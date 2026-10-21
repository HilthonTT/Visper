package throttling

import (
	"context"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

const RedisPrefix = "visper-proxy"

type ThrottleConfig struct {
	// Name namespaces the Redis keys so throttlers with different limits
	// never share counters.
	Name        string
	MaxRequests int
	Interval    time.Duration
	Spans       int
	Cooldown    time.Duration
}

func NewDefaultThrottleConfig() ThrottleConfig {
	return ThrottleConfig{
		MaxRequests: 100,
		Interval:    time.Minute,
		Spans:       6,
		Cooldown:    30 * time.Second,
	}
}

type Throttler struct {
	config       ThrottleConfig
	client       redis.UniversalClient // nil means local-only throttling
	spanDuration time.Duration
	now          func() time.Time

	mu            sync.RWMutex
	localCounts   map[string]int
	blockedRoutes map[string]time.Time

	// Adaptive node estimation
	nodesMu           sync.RWMutex
	nodes             float64
	previousSpanTotal int
	currentSpanTotal  int
}

// NewThrottler creates a throttler. client may be nil, in which case limits
// are enforced per process only.
func NewThrottler(config ThrottleConfig, client redis.UniversalClient) *Throttler {
	def := NewDefaultThrottleConfig()
	if config.MaxRequests <= 0 {
		config.MaxRequests = def.MaxRequests
	}
	if config.Interval <= 0 {
		config.Interval = def.Interval
	}
	if config.Spans <= 0 {
		config.Spans = def.Spans
	}
	if config.Cooldown < 0 {
		config.Cooldown = 0
	}
	span := config.Interval / time.Duration(config.Spans)
	if span <= 0 {
		span = config.Interval
		config.Spans = 1
	}
	return &Throttler{
		config:        config,
		client:        client,
		spanDuration:  span,
		now:           time.Now,
		localCounts:   make(map[string]int),
		blockedRoutes: make(map[string]time.Time),
		nodes:         1, // assume single node until we learn otherwise
	}
}

// Cooldown returns how long a key stays blocked once it exceeds its limit.
func (t *Throttler) Cooldown() time.Duration { return t.config.Cooldown }

// Start aligns to span boundaries then ticks every spanDuration.
func (t *Throttler) Start(ctx context.Context) error {
	now := t.now()
	span := int64(t.spanDuration)
	nextSpan := time.Unix(0, (now.UnixNano()/span+1)*span)
	timer := time.NewTimer(nextSpan.Sub(now))
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		t.processSpan(ctx)
	}

	ticker := time.NewTicker(t.spanDuration)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			t.processSpan(ctx)
		}
	}
}

func (t *Throttler) redisKey(route string, intervalNum int64) string {
	if t.config.Name == "" {
		return fmt.Sprintf("%s:%s:%d", RedisPrefix, route, intervalNum)
	}
	return fmt.Sprintf("%s:%s:%s:%d", RedisPrefix, t.config.Name, route, intervalNum)
}

func (t *Throttler) processSpan(ctx context.Context) {
	// Atomically swap out the current counts.
	t.mu.Lock()
	localCounts := t.localCounts
	t.localCounts = make(map[string]int)
	t.mu.Unlock()

	// processSpan runs right at a span boundary; attribute the counts to the
	// span that just ended (its midpoint), not to the one starting now.
	spanMid := t.now().Add(-t.spanDuration / 2).UnixNano()
	intervalNum := spanMid / int64(t.config.Interval)
	spanInInterval := (spanMid / int64(t.spanDuration)) % int64(t.config.Spans)

	t.cleanupBlockedRoutes()
	t.updateNodeEstimation(ctx, localCounts, intervalNum, spanInInterval)

	if len(localCounts) == 0 {
		return
	}

	type entry struct {
		route   string
		incrCmd *redis.IntCmd
	}
	var entries []entry
	pipeErr := redis.ErrClosed

	if t.client != nil {
		// Use a pipeline to batch IncrBy + Expire — halves round-trips.
		pipe := t.client.Pipeline()
		for route, count := range localCounts {
			key := t.redisKey(route, intervalNum)
			incrCmd := pipe.IncrBy(ctx, key, int64(count))
			pipe.Expire(ctx, key, t.config.Interval*2)
			entries = append(entries, entry{route, incrCmd})
		}
		_, pipeErr = pipe.Exec(ctx)
	}

	if pipeErr == nil {
		for _, e := range entries {
			val, err := e.incrCmd.Result()
			if err == nil && val > int64(t.config.MaxRequests) {
				t.blockRoute(e.route)
			}
		}
		return
	}

	// Redis unavailable: fall back to a per-span local threshold.
	localThreshold := max(1, t.config.MaxRequests/t.config.Spans)
	for route, count := range localCounts {
		if count > localThreshold {
			t.blockRoute(route)
		}
	}
}

// updateNodeEstimation compares local traffic to the global Redis total
// from the previous interval to infer how many nodes are active.
func (t *Throttler) updateNodeEstimation(ctx context.Context, localCounts map[string]int, intervalNum, spanInInterval int64) {
	var spanTotal int
	for _, c := range localCounts {
		spanTotal += c
	}

	t.nodesMu.Lock()
	t.currentSpanTotal += spanTotal
	t.nodesMu.Unlock()

	// Only recalculate at the last span of each interval.
	if spanInInterval != int64(t.config.Spans-1) {
		return
	}

	var globalTotal int64
	if t.client != nil {
		prevIntervalNum := intervalNum - 1
		for route := range localCounts {
			if val, err := t.client.Get(ctx, t.redisKey(route, prevIntervalNum)).Int64(); err == nil {
				globalTotal += val
			}
		}
	}

	t.nodesMu.Lock()
	defer t.nodesMu.Unlock()
	if globalTotal > 0 && t.previousSpanTotal > 0 {
		t.nodes = float64(globalTotal) / float64(t.previousSpanTotal)
	}
	t.previousSpanTotal = t.currentSpanTotal
	t.currentSpanTotal = 0
}

// IncrementAndCheck increments by weight (use 1 for normal requests).
// Returns false if the route is currently throttled.
func (t *Throttler) IncrementAndCheck(route string, weight int) bool {
	now := t.now()

	// Early local check using node-estimated threshold.
	t.nodesMu.RLock()
	nodes := math.Max(1, t.nodes)
	t.nodesMu.RUnlock()

	localThreshold := max(1, int(float64(t.config.MaxRequests)/nodes))

	// A single critical section avoids the check-then-increment race of
	// separate RLock/Lock sections.
	t.mu.Lock()
	defer t.mu.Unlock()

	if expiry, blocked := t.blockedRoutes[route]; blocked {
		if now.Before(expiry) {
			return false
		}
		delete(t.blockedRoutes, route)
	}

	t.localCounts[route] += weight
	if t.localCounts[route] > localThreshold {
		t.blockedRoutes[route] = now.Add(t.config.Cooldown)
		return false
	}

	return true
}

func (t *Throttler) blockRoute(route string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.blockedRoutes[route] = t.now().Add(t.config.Cooldown)
}

func (t *Throttler) cleanupBlockedRoutes() {
	t.mu.Lock()
	defer t.mu.Unlock()
	now := t.now()
	for route, expiry := range t.blockedRoutes {
		if !now.Before(expiry) {
			delete(t.blockedRoutes, route)
		}
	}
}
