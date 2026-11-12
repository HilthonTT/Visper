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
	counts  map[string]int // current-span counts, synced to Redis at span end
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
		counts:  make(map[string]int),
		blocked: make(map[string]time.Time),
		nodes:   newNodeEstimator(),
	}
}

// Name returns the throttler's configured name.
func (t *Throttler) Name() string { return t.cfg.Name }

// Allow records weight requests for key and reports whether they are
// allowed. When they are not, it also returns the cooldown to wait.
func (t *Throttler) Allow(key string, weight int) (bool, time.Duration) {
	if weight <= 0 {
		weight = 1
	}
	limit := max(1, int(float64(t.cfg.MaxRequests)/math.Max(1, t.nodes.get())))
	now := t.now()

	// A single critical section avoids a check-then-increment race.
	t.mu.Lock()
	defer t.mu.Unlock()

	if until, ok := t.blocked[key]; ok {
		if now.Before(until) {
			return false, t.cfg.Cooldown
		}
		delete(t.blocked, key)
	}

	t.counts[key] += weight
	if t.counts[key] > limit {
		t.blocked[key] = now.Add(t.cfg.Cooldown)
		return false, t.cfg.Cooldown
	}
	return true, 0
}

// Start aligns to span boundaries, then syncs with Redis once per span
// until ctx is done.
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

// endSpan runs at a span boundary: it resets the span counts, drops expired
// blocks and pushes the finished span's counts to Redis.
func (t *Throttler) endSpan(ctx context.Context) {
	now := t.now()

	t.mu.Lock()
	counts := t.counts
	t.counts = make(map[string]int)
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
	if len(counts) == 0 || t.sync(ctx, counts, interval) {
		return
	}

	// Redis unavailable: fall back to a per-span local threshold.
	threshold := max(1, t.cfg.MaxRequests/t.cfg.Spans)
	for key, n := range counts {
		if n > threshold {
			t.block(key)
		}
	}
}

func (t *Throttler) block(key string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blocked[key] = t.now().Add(t.cfg.Cooldown)
}
