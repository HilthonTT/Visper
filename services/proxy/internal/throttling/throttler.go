package throttling

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// Throttler rate-limits arbitrary string keys (client IPs, a global key...).
type Throttler struct {
	cfg    Config
	span   time.Duration
	client redis.UniversalClient // nil means local-only throttling
	now    func() time.Time

	mu      sync.Mutex
	slot    int                // ring position of the current span
	windows map[string]*window // local sliding-window usage per key
	pending map[string]int     // current-span counts not yet synced to Redis
	blocked map[string]time.Time

	nodes    nodeEstimator
	redisErr redisHealth
}

// New creates a throttler. client may be nil, in which case limits are
// enforced per process only.
func New(cfg Config, client redis.UniversalClient) *Throttler {
	cfg, span := cfg.normalize()
	return &Throttler{
		cfg:     cfg,
		span:    span,
		client:  client,
		now:     time.Now,
		windows: make(map[string]*window),
		pending: make(map[string]int),
		blocked: make(map[string]time.Time),
		nodes:   newNodeEstimator(),
	}
}

// Name returns the throttler's configured name.
func (t *Throttler) Name() string { return t.cfg.Name }

// Allow records weight requests for key and reports whether they are
// allowed. When they are not, it also returns how long key stays blocked.
func (t *Throttler) Allow(key string, weight int) (bool, time.Duration) {
	if weight <= 0 {
		weight = 1
	}
	limit := max(1, int(float64(t.cfg.MaxRequests)/math.Max(1, t.nodes.get())))
	now := t.now()

	t.mu.Lock()
	defer t.mu.Unlock()

	if until, ok := t.blocked[key]; ok {
		if now.Before(until) {
			return false, until.Sub(now)
		}
		delete(t.blocked, key)
	}

	w := t.windows[key]
	used := 0
	if w != nil {
		used = w.total
	}
	if used+weight > limit {
		// Denied requests do not consume quota.
		return false, t.blockLocked(key, now)
	}
	if w == nil {
		w = newWindow(t.cfg.Spans)
		t.windows[key] = w
	}
	w.add(t.slot, weight)
	t.pending[key] += weight
	return true, 0
}

// blockLocked blocks key for the cooldown and returns the block duration.
// When the cooldown is shorter than a span the key would be blocked again
// right away by its window anyway, so a zero cooldown still blocks for one
// span to give clients a meaningful Retry-After.
func (t *Throttler) blockLocked(key string, now time.Time) time.Duration {
	d := max(t.cfg.Cooldown, t.span)
	t.blocked[key] = now.Add(d)
	return d
}

// Start aligns to span boundaries, then rotates the window and syncs with
// Redis once per span until ctx is done.
func (t *Throttler) Start(ctx context.Context) error {
	now := t.now()
	span := int64(t.span)
	next := time.Unix(0, (now.UnixNano()/span+1)*span)
	timer := time.NewTimer(next.Sub(now))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		t.endSpan(ctx)
	}

	ticker := time.NewTicker(t.span)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.endSpan(ctx)
		}
	}
}

// endSpan runs at a span boundary: it advances every key's sliding window,
// drops expired blocks and pushes the finished span's counts to Redis.
func (t *Throttler) endSpan(ctx context.Context) {
	now := t.now()

	t.mu.Lock()
	counts := t.pending
	t.pending = make(map[string]int)
	t.slot = (t.slot + 1) % t.cfg.Spans
	for key, w := range t.windows {
		if w.clear(t.slot); w.total <= 0 {
			delete(t.windows, key)
		}
	}
	for key, until := range t.blocked {
		if !now.Before(until) {
			delete(t.blocked, key)
		}
	}
	t.mu.Unlock()

	// Attribute the counts to the span that just ended (its midpoint), not
	// to the one starting now.
	mid := now.Add(-t.span / 2).UnixNano()
	interval := mid / int64(t.cfg.Interval)
	lastSpan := (mid/int64(t.span))%int64(t.cfg.Spans) == int64(t.cfg.Spans-1)

	t.updateNodeEstimate(ctx, counts, interval, lastSpan)
	t.sync(ctx, counts, interval)
}

func (t *Throttler) block(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockLocked(key, t.now())
}
