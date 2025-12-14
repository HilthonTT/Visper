package ratelimiter

import (
	"context"
	"sync"
	"time"
)

type inMemoryEntry struct {
	value     int
	expiresAt time.Time
}

type InMemory struct {
	cache     map[string]inMemoryEntry
	mu        sync.RWMutex
	stopClean chan struct{}
	cleanOnce sync.Once
}

func NewInMemory() GetterSetter {
	im := &InMemory{
		cache:     make(map[string]inMemoryEntry),
		stopClean: make(chan struct{}),
	}

	go im.cleanupExpired(context.Background())

	return im
}

func (i *InMemory) Get(key string) (int, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()

	entry, ok := i.cache[key]
	if !ok {
		return 0, ErrCacheMiss
	}

	if !entry.expiresAt.IsZero() && time.Now().After(entry.expiresAt) {
		return 0, ErrCacheMiss
	}

	return entry.value, nil
}

func (i *InMemory) Set(key string, value int) error {
	return i.SetWithExpiration(key, value, 0)
}

func (i *InMemory) SetWithExpiration(key string, value int, expiration time.Duration) error {
	var expiresAt time.Time
	if expiration > 0 {
		expiresAt = time.Now().Add(expiration)
	}

	i.mu.Lock()

	i.cache[key] = inMemoryEntry{
		value:     value,
		expiresAt: expiresAt,
	}

	i.mu.Unlock()

	return nil
}

func (i *InMemory) cleanupExpired(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			i.removeExpired()
		case <-i.stopClean:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (i *InMemory) removeExpired() {
	now := time.Now()

	i.mu.Lock()
	defer i.mu.Unlock()

	for key, entry := range i.cache {
		if !entry.expiresAt.IsZero() && now.After(entry.expiresAt) {
			delete(i.cache, key)
		}
	}
}

func (i *InMemory) Close() error {
	i.cleanOnce.Do(func() {
		close(i.stopClean)
	})
	return nil
}
