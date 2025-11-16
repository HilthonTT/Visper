package ratelimiter

import (
	"sync"
	"sync/atomic"
	"time"
)

type FixedWindowRateLimiter struct {
	counts      sync.Map // string -> *clientData
	limit       int64
	window      time.Duration
	cleanupTick *time.Ticker
	done        chan struct{}
}

type clientData struct {
	count   int64        // atomic
	resetAt atomic.Value // stores time.Time
	mu      sync.Mutex   // only for reset (rare)
}

func NewFixedWindowRateLimiter(limit int, window time.Duration) *FixedWindowRateLimiter {
	rl := &FixedWindowRateLimiter{
		limit:       int64(limit),
		window:      window,
		cleanupTick: time.NewTicker(window),
		done:        make(chan struct{}),
	}
	go rl.startCleanup()
	return rl
}

func (rl *FixedWindowRateLimiter) Allow(ip string) (bool, time.Duration) {
	now := time.Now()
	windowStart := now.Truncate(rl.window)
	nextReset := windowStart.Add(rl.window)

	// Load or create
	val, _ := rl.counts.LoadOrStore(ip, &clientData{})
	data := val.(*clientData)

	// Initialize resetAt if first time
	if data.resetAt.Load() == nil {
		data.resetAt.Store(nextReset)
		atomic.StoreInt64(&data.count, 1)
		return true, 0
	}

	currentReset := data.resetAt.Load().(time.Time)

	if now.Before(currentReset) {
		// Still in current window
		newCount := atomic.AddInt64(&data.count, 1)
		if newCount-1 >= rl.limit {
			atomic.AddInt64(&data.count, -1) // rollback
			return false, time.Until(currentReset)
		}
		return true, 0
	}

	// --- Window expired: reset ---
	data.mu.Lock()
	defer data.mu.Unlock()

	// Double-check after lock
	if currentReset := data.resetAt.Load().(time.Time); now.Before(currentReset) {
		// Another goroutine already handled reset
		newCount := atomic.AddInt64(&data.count, 1)
		if newCount-1 >= rl.limit {
			atomic.AddInt64(&data.count, -1)
			return false, time.Until(currentReset)
		}
		return true, 0
	}

	// Perform reset
	atomic.StoreInt64(&data.count, 1)
	data.resetAt.Store(nextReset)
	return true, 0
}

func (rl *FixedWindowRateLimiter) startCleanup() {
	for {
		select {
		case <-rl.cleanupTick.C:
			rl.cleanup()
		case <-rl.done:
			return
		}
	}
}

func (rl *FixedWindowRateLimiter) cleanup() {
	now := time.Now()
	rl.counts.Range(func(key, value interface{}) bool {
		data := value.(*clientData)
		if resetAt := data.resetAt.Load(); resetAt != nil {
			if now.After(resetAt.(time.Time)) {
				rl.counts.Delete(key)
			}
		}
		return true
	})
}

func (rl *FixedWindowRateLimiter) Close() {
	close(rl.done)
	rl.cleanupTick.Stop()
}
