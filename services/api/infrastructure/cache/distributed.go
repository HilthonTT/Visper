package cache

import (
	"context"
	"encoding/json"
	"errors"
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

// Set adds an item to both local and Redis caches. A ttl <= 0 means the Redis
// key does not expire; the local copy always expires after at most localTTL so
// that changes made by other instances become visible.
func (dc *DistributedCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	redisKey := dc.keyPrefix + key
	if err := dc.redis.Set(ctx, redisKey, data, ttl).Err(); err != nil {
		// Do not keep a local copy of a value that was never persisted.
		dc.local.Delete(key)
		return err
	}

	dc.local.Set(key, data, dc.localTTLFor(ttl))
	return nil
}

// Get retrieves an item, checking local cache first. The value is decoded
// into valuePtr.
func (dc *DistributedCache) Get(ctx context.Context, key string, valuePtr any) (bool, error) {
	// The local cache stores the encoded JSON rather than the caller's value so
	// that callers mutating the decoded struct can never corrupt the cache.
	if val, found := dc.local.Get(key); found {
		if data, ok := val.([]byte); ok {
			return true, json.Unmarshal(data, valuePtr)
		}
		dc.local.Delete(key)
	}

	redisKey := dc.keyPrefix + key
	data, err := dc.redis.Get(ctx, redisKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}

	if err := json.Unmarshal(data, valuePtr); err != nil {
		return false, err
	}

	dc.local.Set(key, data, dc.localTTL)

	return true, nil
}

func (dc *DistributedCache) localTTLFor(ttl time.Duration) time.Duration {
	if ttl <= 0 || ttl > dc.localTTL {
		return dc.localTTL
	}
	return ttl
}

// Delete removes an item from both caches
func (dc *DistributedCache) Delete(ctx context.Context, key string) error {
	// Delete from local cache
	dc.local.Delete(key)

	// Delete from Redis
	redisKey := dc.keyPrefix + key
	return dc.redis.Del(ctx, redisKey).Err()
}

// Flush clears both caches
func (dc *DistributedCache) Flush(ctx context.Context) error {
	// Flush local cache
	dc.local.Flush()

	// Flush Redis keys with our prefix
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
	return dc.redis.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:   redisKey,
		Start: start,
		Stop:  stop,
		Rev:   true,
	}).Result()
}

// ZRangeByScore returns members from a sorted set by score range
func (dc *DistributedCache) ZRangeByScore(ctx context.Context, key string, opt *redis.ZRangeBy) ([]string, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRangeArgs(ctx, redis.ZRangeArgs{
		Key:     redisKey,
		Start:   opt.Min,
		Stop:    opt.Max,
		ByScore: true,
		Offset:  opt.Offset,
		Count:   opt.Count,
	}).Result()
}

// ZRem removes members from a sorted set
func (dc *DistributedCache) ZRem(ctx context.Context, key string, members ...any) error {
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
func (dc *DistributedCache) SAdd(ctx context.Context, key string, members ...any) error {
	redisKey := dc.keyPrefix + key
	return dc.redis.SAdd(ctx, redisKey, members...).Err()
}

// SRem removes members from a set
func (dc *DistributedCache) SRem(ctx context.Context, key string, members ...any) error {
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

// TxPipeline returns a Redis pipeline wrapped in MULTI/EXEC.
func (dc *DistributedCache) TxPipeline() redis.Pipeliner {
	return dc.redis.TxPipeline()
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

// Close stops the local cache. The Redis client is shared and owned by the
// caller (see CloseRedis), so it is not closed here.
func (dc *DistributedCache) Close() {
	dc.local.Close()
}
