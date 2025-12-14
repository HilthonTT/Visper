package ratelimiter

import (
	"errors"
	"math"
	"net/http"
	"sync"
	"time"
)

const (
	bucketKeyPrefix   = "rl:bucket:"
	lastFillKeyPrefix = "rl:fill:"
	defaultSourceKey  = "X-RateLimit-Key"
)

type Limiter interface {
	Allow(sourceKey string) bool
	GetSourceKey(r *http.Request) string
	Remaining(sourceKey string) int
	GetMaxBurst() int
}

type RateLimiter struct {
	maxRatePerMillisecond float64
	maxBurst              int
	cache                 GetterSetter
	cacheTTL              time.Duration
	sourceHeaderKey       string
	// Per-key locks to ensure atomic operations for each source
	locks sync.Map // map[string]*sync.Mutex
}

func (rl *RateLimiter) getLock(sourceKey string) *sync.Mutex {
	lock, _ := rl.locks.LoadOrStore(sourceKey, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (rl *RateLimiter) getBucketKeyFor(sourceKey string) string {
	return bucketKeyPrefix + sourceKey
}

func (rl *RateLimiter) getLastFillKeyFor(sourceKey string) string {
	return lastFillKeyPrefix + sourceKey
}

type bucketState struct {
	tokens   int
	lastFill int64 // Unix milliseconds
}

func (rl *RateLimiter) getState(sourceKey string) bucketState {
	bucketKey := rl.getBucketKeyFor(sourceKey)
	lastFillKey := rl.getLastFillKeyFor(sourceKey)

	bucket, bucketErr := rl.cache.Get(bucketKey)
	lastFill, fillErr := rl.cache.Get(lastFillKey)

	if errors.Is(bucketErr, ErrCacheMiss) || errors.Is(fillErr, ErrCacheMiss) {
		return bucketState{
			tokens:   rl.maxBurst,
			lastFill: time.Now().UnixMilli(),
		}
	}

	// On cache error (not miss), fail open with full bucket
	if bucketErr != nil || fillErr != nil {
		return bucketState{
			tokens:   rl.maxBurst,
			lastFill: time.Now().UnixMilli(),
		}
	}

	return bucketState{
		tokens:   bucket,
		lastFill: int64(lastFill),
	}
}

func (rl *RateLimiter) setState(sourceKey string, state bucketState) {
	bucketKey := rl.getBucketKeyFor(sourceKey)
	lastFillKey := rl.getLastFillKeyFor(sourceKey)

	_ = rl.cache.SetWithExpiration(bucketKey, state.tokens, rl.cacheTTL)
	_ = rl.cache.SetWithExpiration(lastFillKey, int(state.lastFill), rl.cacheTTL)
}

func (rl *RateLimiter) refillTokens(state bucketState, now int64) bucketState {
	elapsed := now - state.lastFill
	if elapsed <= 0 {
		return state // No time has passed
	}

	tokensToAdd := float64(elapsed) + rl.maxRatePerMillisecond
	newTokens := float64(state.tokens) + tokensToAdd

	if newTokens > float64(rl.maxBurst) {
		return bucketState{
			tokens:   rl.maxBurst,
			lastFill: now,
		}
	}

	return bucketState{
		tokens:   int(math.Floor(newTokens)), // Only count whole tokens
		lastFill: now,
	}
}

func (rl *RateLimiter) Remaining(sourceKey string) int {
	lock := rl.getLock(sourceKey)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().UnixMilli()
	state := rl.getState(sourceKey)
	newState := rl.refillTokens(state, now)

	// Only update cache if state changed
	if newState.tokens != state.tokens || newState.lastFill != state.lastFill {
		rl.setState(sourceKey, newState)
	}

	return newState.tokens
}

func (rl *RateLimiter) GetMaxBurst() int {
	return rl.maxBurst
}

func (rl *RateLimiter) Allow(sourceKey string) bool {
	lock := rl.getLock(sourceKey)
	lock.Lock()
	defer lock.Unlock()

	now := time.Now().UnixMilli()
	state := rl.getState(sourceKey)
	newState := rl.refillTokens(state, now)

	// Check if we have tokens available
	if newState.tokens > 0 {
		newState.tokens--
		rl.setState(sourceKey, newState)
		return true
	}

	// No tokens available - still update state if refill occurred
	if newState.lastFill != state.lastFill {
		rl.setState(sourceKey, newState)
	}

	return false
}

func (rl *RateLimiter) GetSourceKey(r *http.Request) string {
	if key := r.Header.Get(rl.sourceHeaderKey); key != "" {
		return key
	}

	// Fall back to IP address
	return r.RemoteAddr
}

type Options struct {
	MaxRatePerSecond int
	MaxBurst         int
	Cache            GetterSetter
	CacheTTL         time.Duration
	SourceHeaderKey  string
}

func New(options Options) Limiter {
	if options.Cache == nil {
		options.Cache = NewInMemory()
	}

	if options.CacheTTL == 0 {
		options.CacheTTL = 10 * time.Second
	}

	if options.MaxBurst <= 0 {
		options.MaxBurst = options.MaxRatePerSecond // Reasonable default
	}

	if options.SourceHeaderKey == "" {
		options.SourceHeaderKey = defaultSourceKey
	}

	return &RateLimiter{
		maxRatePerMillisecond: float64(options.MaxRatePerSecond) / 1000.0,
		maxBurst:              options.MaxBurst,
		cache:                 options.Cache,
		cacheTTL:              options.CacheTTL,
		sourceHeaderKey:       options.SourceHeaderKey,
	}
}
