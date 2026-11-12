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

// nextSpan advances the clock by one span and runs the span boundary work.
func nextSpan(th *Throttler, clock *fakeClock) {
	clock.Advance(th.span)
	th.endSpan(context.Background())
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

// Regression: the local limit used to apply to a single span, so a client
// could send MaxRequests every span (Spans times the limit per interval).
func TestLimitCoversWholeInterval(t *testing.T) {
	th, clock := newTestThrottler(Config{MaxRequests: 6, Interval: time.Minute, Spans: 6})
	for i := range 6 {
		if ok, _ := th.Allow("ip", 1); !ok {
			t.Fatalf("request %d denied under limit", i)
		}
	}
	for span := 1; span < 6; span++ {
		nextSpan(th, clock)
		if ok, _ := th.Allow("ip", 1); ok {
			t.Fatalf("span %d: request allowed although the interval's quota is used up", span)
		}
	}
	nextSpan(th, clock) // the span holding the original requests slides out
	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("request denied after the window slid past the old requests")
	}
}

func TestCooldownAndRetryAfter(t *testing.T) {
	th, clock := newTestThrottler(Config{MaxRequests: 1, Interval: 2 * time.Second, Spans: 2, Cooldown: 10 * time.Second})

	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("first request should pass")
	}
	ok, retry := th.Allow("ip", 1)
	if ok || retry != 10*time.Second {
		t.Fatalf("second request: ok=%v retry=%v, want denied for 10s", ok, retry)
	}
	if ok, _ := th.Allow("other", 1); !ok {
		t.Fatal("other keys must not be affected")
	}

	clock.Advance(4 * time.Second)
	if ok, retry := th.Allow("ip", 1); ok || retry != 6*time.Second {
		t.Fatalf("in cooldown: ok=%v retry=%v, want denied for 6s", ok, retry)
	}

	// The window has long drained; only the cooldown remained.
	for range 6 {
		nextSpan(th, clock)
	}
	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("request should pass after cooldown")
	}
}

func TestDeniedRequestsDoNotConsumeQuota(t *testing.T) {
	th, clock := newTestThrottler(Config{MaxRequests: 2, Interval: time.Minute, Spans: 6, Cooldown: 0})
	th.Allow("ip", 1)
	th.Allow("ip", 1)
	for range 50 {
		th.Allow("ip", 1) // denied
	}
	for range 6 {
		nextSpan(th, clock)
	}
	if ok, _ := th.Allow("ip", 1); !ok {
		t.Fatal("denied requests kept the key over its limit")
	}
}

func TestEndSpanForgetsIdleKeys(t *testing.T) {
	th, clock := newTestThrottler(Config{MaxRequests: 1, Interval: time.Minute, Spans: 3, Cooldown: time.Second})
	th.Allow("a", 1)
	th.Allow("a", 1) // blocked
	th.Allow("b", 1)
	for range 3 {
		nextSpan(th, clock)
	}
	th.mu.Lock()
	defer th.mu.Unlock()
	if len(th.windows) != 0 || len(th.blocked) != 0 || len(th.pending) != 0 {
		t.Fatalf("state not cleaned: windows=%v blocked=%v pending=%v", th.windows, th.blocked, th.pending)
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
