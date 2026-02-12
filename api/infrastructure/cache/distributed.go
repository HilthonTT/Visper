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
func NewDistributedCache(redisClient *redis.Client, keyPrefix string, localOptions Options) *DistributedCache {
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

// ZAdd adds a member to a sorted set
func (dc *DistributedCache) ZAdd(ctx context.Context, key string, members ...redis.Z) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZAdd(ctx, redisKey, members...).Err()
}

// ZRange returns members from a sorted set by index range
func (dc *DistributedCache) ZRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRange(ctx, redisKey, start, stop).Result()
}

// ZRevRange returns members from a sorted set in reverse order
func (dc *DistributedCache) ZRevRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRevRange(ctx, redisKey, start, stop).Result()
}

// ZRangeByScore returns members from a sorted set by score range
func (dc *DistributedCache) ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRangeByScore(ctx, redisKey, opt).Result()
}

// ZRem removes members from a sorted set
func (dc *DistributedCache) ZRem(ctx context.Context, key string, members ...interface{}) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRem(ctx, redisKey, members...).Err()
}

// ZRemRangeByScore removes members from a sorted set by score range
func (dc *DistributedCache) ZRemRangeByScore(ctx context.Context, key, min, max string) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRemRangeByScore(ctx, redisKey, min, max).Err()
}

// ZCard returns the number of members in a sorted set
func (dc *DistributedCache) ZCard(ctx context.Context, key string) (int64, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZCard(ctx, redisKey).Result()
}

// SAdd adds members to a set
func (dc *DistributedCache) SAdd(ctx context.Context, key string, members ...interface{}) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.SAdd(ctx, redisKey, members...).Err()
}

// SRem removes members from a set
func (dc *DistributedCache) SRem(ctx context.Context, key string, members ...interface{}) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.SRem(ctx, redisKey, members...).Err()
}

// SMembers returns all members of a set
func (dc *DistributedCache) SMembers(ctx context.Context, key string) ([]string, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.SMembers(ctx, redisKey).Result()
}

// Pipeline returns a Redis pipeline for batch operations
func (dc *DistributedCache) Pipeline() redis.Pipeliner {
	return dc.redis.Pipeline()
}

// ExecPipeline executes a pipeline with key prefix handling
func (dc *DistributedCache) ExecPipeline(ctx context.Context, pipe redis.Pipeliner) error {
	_, err := pipe.Exec(ctx)
	return err
}

// GetRedisKey returns the prefixed Redis key
func (dc *DistributedCache) GetRedisKey(key string) string {
	return dc.keyPrefix + key
}

// Close closes both caches
func (dc *DistributedCache) Close() error {
	dc.local.Close()
	return dc.redis.Close()
}
