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

func newTestThrottler(cfg Config) (*Throttler, *fakeClock) {
	clock := &fakeClock{t: time.Unix(1_700_000_000, 0)}
	th := New(cfg, nil)
	th.now = clock.Now
	return th, clock
}

func TestConfigNormalize(t *testing.T) {
	tests := []struct {
		name      string
		cfg       Config
		wantSpans int
		wantSpan  time.Duration
	}{
		{"defaults for zero config", Config{}, 6, 10 * time.Second},
		{"sub-second spans", Config{MaxRequests: 1, Interval: time.Second, Spans: 4}, 4, 250 * time.Millisecond},
		{"more spans than nanoseconds", Config{MaxRequests: 1, Interval: 2, Spans: 10}, 1, 2},
		{"interval not divisible by spans", Config{MaxRequests: 1, Interval: time.Second, Spans: 7}, 7, 142857142},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, span := tt.cfg.normalize()
			if cfg.Spans != tt.wantSpans || span != tt.wantSpan {
				t.Fatalf("got spans=%d span=%v, want %d %v", cfg.Spans, span, tt.wantSpans, tt.wantSpan)
			}
			if want := span * time.Duration(cfg.Spans); cfg.Interval != want {
				t.Fatalf("interval %v is not spans*span (%v); span positions drift from interval boundaries", cfg.Interval, want)
			}
		})
	}
}

func TestAllow(t *testing.T) {
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
		{"zero weight counts as one", 2, 0, 3, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			th, _ := newTestThrottler(Config{MaxRequests: tt.max, Interval: time.Minute, Spans: 6, Cooldown: time.Second})
			got := 0
			for range tt.calls {
				if ok, _ := th.Allow("k", tt.weight); ok {
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
	th, clock := newTestThrottler(Config{MaxRequests: 1, Interval: time.Minute, Spans: 1, Cooldown: 10 * time.Second})
	ctx := context.Background()

	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("first request should pass")
	}
	if ok, retry := th.Allow("ip", 1); ok || retry != 10*time.Second {
		t.Fatalf("second request: ok=%v retry=%v, want throttled for 10s", ok, retry)
	}
	if ok, _ := th.Allow("other", 1); !ok {
		t.Fatal("other keys must not be affected")
	}

	clock.Advance(5 * time.Second)
	if ok, _ := th.Allow("ip", 1); ok {
		t.Fatal("still in cooldown")
	}

	clock.Advance(time.Minute)
	// Resets counters; the span total for "ip" (2) exceeded the local
	// fallback threshold, so it is blocked for one more cooldown.
	th.endSpan(ctx)
	if ok, _ := th.Allow("ip", 1); ok {
		t.Fatal("over-limit key should be re-blocked at span end")
	}

	clock.Advance(time.Minute)
	th.endSpan(ctx) // nothing counted this span; cleans expired blocks
	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("request should pass after cooldown and span reset")
	}
	th.mu.Lock()
	defer th.mu.Unlock()
	if len(th.blocked) != 0 {
		t.Fatalf("blocked map not cleaned: %v", th.blocked)
	}
}

func TestEndSpanLocalFallbackBlocks(t *testing.T) {
	// No Redis: a key that exceeds MaxRequests/Spans within a span is blocked.
	th, _ := newTestThrottler(Config{MaxRequests: 60, Interval: time.Minute, Spans: 6, Cooldown: time.Minute})
	for range 11 {
		th.Allow("busy", 1)
	}
	for range 5 {
		th.Allow("quiet", 1)
	}
	th.endSpan(context.Background())

	if ok, _ := th.Allow("busy", 1); ok {
		t.Fatal("busy key should be blocked after span")
	}
	if ok, _ := th.Allow("quiet", 1); !ok {
		t.Fatal("quiet key should not be blocked")
	}
}

func TestNodeEstimateSharesLimit(t *testing.T) {
	th, _ := newTestThrottler(Config{MaxRequests: 10, Interval: time.Minute, Spans: 6})
	th.nodes.set(5) // e.g. learned from Redis: five proxies share the limit
	allowed := 0
	for range 10 {
		if ok, _ := th.Allow("ip", 1); ok {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d, want 2 (10 / 5 nodes)", allowed)
	}
}

func TestStartStopsOnCancel(t *testing.T) {
	th := New(Config{MaxRequests: 1, Interval: 60 * time.Millisecond, Spans: 3}, nil)
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

func TestHierarchyAllow(t *testing.T) {
	cfg := Config{MaxRequests: 2, Interval: time.Minute, Spans: 1, Cooldown: time.Minute}
	ipT, _ := newTestThrottler(cfg)
	cfg.MaxRequests = 3
	globalT, _ := newTestThrottler(cfg)
	h := NewHierarchy(
		&Level{Name: "ip", Key: func(r *http.Request) string { return r.RemoteAddr }, Throttler: ipT},
		&Level{Name: "global", Key: func(*http.Request) string { return "global" }, Throttler: globalT},
	)

	tests := []struct {
		remote    string
		wantLevel string // empty: allowed
	}{
		{"1.1.1.1", ""},
		{"1.1.1.1", ""},
		{"1.1.1.1", "ip"}, // per-IP limit; must not consume global quota
		{"2.2.2.2", ""},
		{"3.3.3.3", "global"},
	}
	for i, tt := range tests {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.RemoteAddr = tt.remote
		d := h.Allow(r)
		if d.Allowed != (tt.wantLevel == "") || d.Level != tt.wantLevel {
			t.Fatalf("step %d: got %+v, want level %q", i, d, tt.wantLevel)
		}
		if !d.Allowed && d.RetryAfter <= 0 {
			t.Fatalf("step %d: denied without RetryAfter", i)
		}
	}
}
