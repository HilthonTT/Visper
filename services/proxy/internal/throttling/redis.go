package throttling

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/redis/go-redis/v9"
)

func (t *Throttler) redisKey(key string, interval int64) string {
	if t.cfg.Name == "" {
		return fmt.Sprintf("%s:%s:%d", RedisPrefix, key, interval)
	}
	return fmt.Sprintf("%s:%s:%s:%d", RedisPrefix, t.cfg.Name, key, interval)
}

// sync adds counts to the global per-interval counters in Redis and blocks
// every key whose global total exceeds the limit. When Redis is unreachable
// the local sliding windows keep enforcing each node's share of the limit.
func (t *Throttler) sync(ctx context.Context, counts map[string]int, interval int64) {
	if t.client == nil || len(counts) == 0 {
		return
	}

	keys := make([]string, 0, len(counts))
	cmds := make([]*redis.IntCmd, 0, len(counts))
	pipe := t.client.Pipeline()
	for key, n := range counts {
		rk := t.redisKey(key, interval)
		keys = append(keys, key)
		cmds = append(cmds, pipe.IncrBy(ctx, rk, int64(n)))
		pipe.Expire(ctx, rk, 2*t.cfg.Interval)
	}
	_, err := pipe.Exec(ctx)
	if t.redisErr.report(t.cfg.Name, err) {
		return
	}
	for i, cmd := range cmds {
		if total, err := cmd.Result(); err == nil && total > int64(t.cfg.MaxRequests) {
			t.block(keys[i])
		}
	}
}

// updateNodeEstimate infers how many proxy instances share the limit by
// comparing this node's traffic in the previous interval with the global
// Redis total over the same keys. It recalculates once per interval, after
// the interval's last span.
func (t *Throttler) updateNodeEstimate(ctx context.Context, counts map[string]int, interval int64, lastSpan bool) {
	t.nodes.record(counts)
	if !lastSpan {
		return
	}
	prevKeys, prevTotal := t.nodes.rollover()
	if t.client == nil || len(prevKeys) == 0 || prevTotal == 0 {
		return
	}

	pipe := t.client.Pipeline()
	cmds := make([]*redis.StringCmd, 0, len(prevKeys))
	for key := range prevKeys {
		cmds = append(cmds, pipe.Get(ctx, t.redisKey(key, interval-1)))
	}
	_, _ = pipe.Exec(ctx) // redis.Nil for missing keys; checked per command

	var globalTotal int64
	for _, cmd := range cmds {
		if n, err := cmd.Int64(); err == nil {
			globalTotal += n
		}
	}
	if globalTotal > 0 {
		t.nodes.set(float64(globalTotal) / float64(prevTotal))
	}
}

// nodeEstimator tracks the keys and totals this node served per interval
// and the resulting estimate of active instances.
type nodeEstimator struct {
	mu        sync.Mutex
	nodes     float64
	curTotal  int
	curKeys   map[string]struct{}
	prevTotal int
	prevKeys  map[string]struct{}
}

func newNodeEstimator() nodeEstimator {
	return nodeEstimator{nodes: 1, curKeys: make(map[string]struct{})}
}

func (e *nodeEstimator) get() float64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nodes
}

func (e *nodeEstimator) set(n float64) {
	e.mu.Lock()
	e.nodes = n
	e.mu.Unlock()
}

func (e *nodeEstimator) record(counts map[string]int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for key, n := range counts {
		e.curTotal += n
		e.curKeys[key] = struct{}{}
	}
}

// rollover closes the current interval and returns the previous one, whose
// global counts are complete by now.
func (e *nodeEstimator) rollover() (map[string]struct{}, int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	keys, total := e.prevKeys, e.prevTotal
	e.prevKeys, e.prevTotal = e.curKeys, e.curTotal
	e.curKeys, e.curTotal = make(map[string]struct{}), 0
	return keys, total
}

// redisHealth logs Redis failures once per outage instead of every span.
type redisHealth struct {
	mu   sync.Mutex
	down bool
}

// report records the outcome of a Redis call and reports whether it failed.
func (h *redisHealth) report(name string, err error) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	switch {
	case err != nil && !h.down:
		log.Printf("throttler %q: redis unavailable, enforcing local limits only: %v", name, err)
	case err == nil && h.down:
		log.Printf("throttler %q: redis available again", name)
	}
	h.down = err != nil
	return err != nil
}
