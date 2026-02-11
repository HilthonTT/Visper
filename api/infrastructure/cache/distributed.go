package cache

import (
	"context"
	"encoding/json"
	"time"

	"github.com/redis/go-redis/v9"
)

// DistributedCache combines local and Redis caching
type DistributedCache struct {
	local       *Cache
	redis       *redis.Client
	keyPrefix   string
	localTTL    time.Duration
	redisKeyTTL time.Duration
}

// NewDistributedCache creates a new distributed cache
func NewDistributedCache(redisAddr, keyPrefix string, localOptions Options) *DistributedCache {
	redisClient := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	return &DistributedCache{
		local:       NewCache(localOptions),
		redis:       redisClient,
		keyPrefix:   keyPrefix,
		localTTL:    5 * time.Minute, // Local cache expires faster than Redis
		redisKeyTTL: 30 * time.Minute,
	}
}

// Set adds an item to both local and Redis caches
func (dc *DistributedCache) Set(key string, value any, ttl time.Duration) error {
	localTTL := ttl
	if ttl > dc.localTTL {
		localTTL = dc.localTTL
	}
	dc.local.Set(key, value, localTTL)

	// Marshal values for Redis
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	// Set in Redis
	redisKey := dc.keyPrefix + key
	ctx := context.Background()
	return dc.redis.Set(ctx, redisKey, data, ttl).Err()
}

// Get retrieves an item, checking local cache first
func (dc *DistributedCache) Get(key string, valuePtr any) (bool, error) {
	// Check local cache first
	if val, found := dc.local.Get(key); found {
		// Unmarshal into the provided pointer
		data, err := json.Marshal(val)
		if err != nil {
			return false, err
		}

		return true, json.Unmarshal(data, valuePtr)
	}

	// Check Redis
	redisKey := dc.keyPrefix + key
	ctx := context.Background()
	data, err := dc.redis.Get(ctx, redisKey).Bytes()
	if err != nil {
		if err == redis.Nil {
			return false, nil
		}
		return false, err
	}

	// Unmarshal the data
	if err := json.Unmarshal(data, valuePtr); err != nil {
		return false, err
	}

	// Update local cache
	dc.local.Set(key, valuePtr, dc.localTTL)

	return true, nil
}

// Delete removes an item from both caches
func (dc *DistributedCache) Delete(key string) error {
	// Delete from local cache
	dc.local.Delete(key)

	// Delete from Redis
	redisKey := dc.keyPrefix + key
	ctx := context.Background()
	return dc.redis.Del(ctx, redisKey).Err()
}

// Flush clears both caches
func (dc *DistributedCache) Flush() error {
	// Flush local cache
	dc.local.Flush()

	// Flush Redis keys with our prefix
	ctx := context.Background()
	iter := dc.redis.Scan(ctx, 0, dc.keyPrefix+"*", 100).Iterator()

	for iter.Next(ctx) {
		if err := dc.redis.Del(ctx, iter.Val()).Err(); err != nil {
			return err
		}
	}

	return iter.Err()
}

// Close closes both caches
func (dc *DistributedCache) Close() error {
	dc.local.Close()
	return dc.redis.Close()
}
