package throttling

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time          { return c.t }
func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

func newTestThrottler(cfg ThrottleConfig) (*Throttler, *fakeClock) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	th := NewThrottler(cfg, nil)
	th.now = clock.Now
	return th, clock
}

func TestNewThrottlerSanitizesConfig(t *testing.T) {
	tests := []struct {
		name      string
		cfg       ThrottleConfig
		wantSpans int
		wantSpan  time.Duration
	}{
		{"defaults for zero config", ThrottleConfig{}, 6, 10 * time.Second},
		{"sub-second spans", ThrottleConfig{MaxRequests: 1, Interval: time.Second, Spans: 4}, 4, 250 * time.Millisecond},
		{"more spans than nanoseconds", ThrottleConfig{MaxRequests: 1, Interval: 2, Spans: 10}, 1, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th := NewThrottler(tt.cfg, nil)
			if th.config.Spans != tt.wantSpans || th.spanDuration != tt.wantSpan {
				t.Fatalf("got spans=%d span=%v, want %d %v", th.config.Spans, th.spanDuration, tt.wantSpans, tt.wantSpan)
			}
		})
	}
}

func TestIncrementAndCheck(t *testing.T) {
	tests := []struct {
		name    string
		max     int
		weight  int
		calls   int
		allowed int
	}{
		{"under limit", 5, 1, 5, 5},
		{"over limit blocks", 3, 1, 10, 3},
		{"weighted requests", 10, 4, 5, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, _ := newTestThrottler(ThrottleConfig{MaxRequests: tt.max, Interval: time.Minute, Spans: 6, Cooldown: time.Second})
			got := 0
			for range tt.calls {
				if th.IncrementAndCheck("k", tt.weight) {
					got++
				}
			}
			if got != tt.allowed {
				t.Fatalf("allowed %d, want %d", got, tt.allowed)
			}
		})
	}
}

func TestCooldownExpires(t *testing.T) {
	th, clock := newTestThrottler(ThrottleConfig{MaxRequests: 1, Interval: time.Minute, Spans: 1, Cooldown: 10 * time.Second})
	ctx := context.Background()

	if !th.IncrementAndCheck("ip", 1) {
		t.Fatal("first request should pass")
	}
	if th.IncrementAndCheck("ip", 1) {
		t.Fatal("second request should be throttled")
	}
	if !th.IncrementAndCheck("other", 1) {
		t.Fatal("other keys must not be affected")
	}

	clock.Advance(5 * time.Second)
	if th.IncrementAndCheck("ip", 1) {
		t.Fatal("still in cooldown")
	}

	clock.Advance(time.Minute)
	// Resets counters; the span total for "ip" (2) exceeded the local
	// fallback threshold, so it is blocked for one more cooldown.
	th.processSpan(ctx)
	if th.IncrementAndCheck("ip", 1) {
		t.Fatal("over-limit key should be re-blocked at span end")
	}

	clock.Advance(time.Minute)
	th.processSpan(ctx) // nothing counted this span; cleans expired blocks
	if !th.IncrementAndCheck("ip", 1) {
		t.Fatal("request should pass after cooldown and span reset")
	}
	th.mu.RLock()
	defer th.mu.RUnlock()
	if len(th.blockedRoutes) != 0 {
		t.Fatalf("blocked map not cleaned: %v", th.blockedRoutes)
	}
}

func TestProcessSpanLocalFallbackBlocks(t *testing.T) {
	// No Redis: a key that exceeds MaxRequests/Spans within a span is blocked.
	th, _ := newTestThrottler(ThrottleConfig{MaxRequests: 60, Interval: time.Minute, Spans: 6, Cooldown: time.Minute})
	for range 11 {
		th.IncrementAndCheck("busy", 1)
	}
	for range 5 {
		th.IncrementAndCheck("quiet", 1)
	}
	th.processSpan(context.Background())

	if th.IncrementAndCheck("busy", 1) {
		t.Fatal("busy key should be blocked after span")
	}
	if !th.IncrementAndCheck("quiet", 1) {
		t.Fatal("quiet key should not be blocked")
	}
}

func TestStartStopsOnCancel(t *testing.T) {
	th := NewThrottler(ThrottleConfig{MaxRequests: 1, Interval: 60 * time.Millisecond, Spans: 3}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- th.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not return after cancel")
	}
}

func TestHierarchicalCheckRequest(t *testing.T) {
	cfg := ThrottleConfig{MaxRequests: 2, Interval: time.Minute, Spans: 1, Cooldown: time.Minute}
	ipT, _ := newTestThrottler(cfg)
	cfg.MaxRequests = 3
	globalT, _ := newTestThrottler(cfg)
	ht := NewHierarchicalThrottler(
		&ThrottleLevel{Name: "ip", KeyExtractor: func(r *http.Request) string { return r.RemoteAddr }, Throttler: ipT},
		&ThrottleLevel{Name: "global", KeyExtractor: func(*http.Request) string { return "global" }, Throttler: globalT},
	)

	tests := []struct {
		remote    string
		wantOK    bool
		wantLevel string
	}{
		{"1.1.1.1", true, ""},
		{"1.1.1.1", true, ""},
		{"1.1.1.1", false, "ip"}, // per-IP limit; must not consume global quota
		{"2.2.2.2", true, ""},
		{"3.3.3.3", false, "global"},
	}
	for i, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remote
		ok, level := ht.CheckRequest(r)
		if ok != tt.wantOK {
			t.Fatalf("step %d: ok=%v, want %v", i, ok, tt.wantOK)
		}
		if !ok && level.Name != tt.wantLevel {
			t.Fatalf("step %d: denied by %q, want %q", i, level.Name, tt.wantLevel)
		}
	}
}
