package cache

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
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

	// writes counts Set/Delete calls. Get only refills the local cache when
	// no write happened while it read Redis, so a read that raced a write
	// can't put the old value back locally (where it would outlive a delete
	// for localTTL). fillMu makes "check writes, then fill" and "count the
	// write, then update the local copy" atomic with respect to each other.
	writes atomic.Uint64
	fillMu sync.Mutex
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
		dc.invalidateLocal(key)
		return err
	}

	dc.fillMu.Lock()
	dc.writes.Add(1)
	dc.local.Set(key, data, dc.localTTLFor(ttl))
	dc.fillMu.Unlock()
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

	writesBefore := dc.writes.Load()
	redisKey := dc.keyPrefix + key
	pipe := dc.redis.Pipeline()
	getCmd := pipe.Get(ctx, redisKey)
	ttlCmd := pipe.PTTL(ctx, redisKey)
	_, _ = pipe.Exec(ctx) // errors are read from the commands below
	data, err := getCmd.Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
		}
		return false, err
	}

	if err := json.Unmarshal(data, valuePtr); err != nil {
		return false, err
	}

	// Cap the local copy at the key's remaining Redis TTL, as Set does, so it
	// is not served after Redis has expired the key.
	if ttl, err := ttlCmd.Result(); err == nil {
		dc.fillMu.Lock()
		if dc.writes.Load() == writesBefore {
			dc.local.Set(key, data, dc.localTTLFor(ttl))
		}
		dc.fillMu.Unlock()
	}

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
	redisKey := dc.keyPrefix + key
	err := dc.redis.Del(ctx, redisKey).Err()
	// Drop the local copy after Redis, and count the write, so a concurrent
	// Get can't refill it with the deleted value.
	dc.invalidateLocal(key)
	return err
}

// invalidateLocal drops key's local copy and counts a write.
func (dc *DistributedCache) invalidateLocal(key string) {
	dc.fillMu.Lock()
	dc.writes.Add(1)
	dc.local.Delete(key)
	dc.fillMu.Unlock()
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

// ZRem removes members from a sorted set and returns how many were removed.
func (dc *DistributedCache) ZRem(ctx context.Context, key string, members ...any) (int64, error) {
	redisKey := dc.keyPrefix + key
	return dc.redis.ZRem(ctx, redisKey, members...).Result()
}

// zReplaceScript swaps a sorted-set member for another one, but only if the
// old member is still present.
var zReplaceScript = redis.NewScript(`
if redis.call('ZREM', KEYS[1], ARGV[1]) == 1 then
	redis.call('ZADD', KEYS[1], ARGV[3], ARGV[2])
	return 1
end
return 0
`)

// ZReplace atomically replaces oldMember with newMember (at score) in a sorted
// set. It reports false, changing nothing, if oldMember is no longer a member.
func (dc *DistributedCache) ZReplace(ctx context.Context, key, oldMember, newMember string, score float64) (bool, error) {
	redisKey := dc.keyPrefix + key
	n, err := zReplaceScript.Run(ctx, dc.redis, []string{redisKey},
		oldMember, newMember, strconv.FormatFloat(score, 'f', -1, 64)).Int()
	if err != nil {
		return false, err
	}
	return n == 1, nil
}

// zAppendNextScript adds a member at score ARGV[2] only if the set's highest
// score is ARGV[2]-1 (or the set is empty and ARGV[2] is 1), then drops every
// lower score and deletes KEYS[2].
var zAppendNextScript = redis.NewScript(`
local top = redis.call('ZRANGE', KEYS[1], -1, -1, 'WITHSCORES')
local current = 0
if #top == 2 then current = tonumber(top[2]) end
local next = tonumber(ARGV[2])
if next ~= current + 1 then return current end
redis.call('ZADD', KEYS[1], next, ARGV[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', '(' .. next)
redis.call('DEL', KEYS[2])
return -1
`)

// ZAppendNext atomically adds member to a sorted set at score next, if next
// follows the set's highest score (an empty set counts as 0), keeping only
// that member and deleting alsoDelete. It returns ok and, when not ok, the
// set's current highest score.
func (dc *DistributedCache) ZAppendNext(ctx context.Context, key, member string, next int64, alsoDelete string) (bool, int64, error) {
	n, err := zAppendNextScript.Run(ctx, dc.redis,
		[]string{dc.keyPrefix + key, dc.keyPrefix + alsoDelete},
		member, strconv.FormatInt(next, 10)).Int64()
	if err != nil {
		return false, 0, err
	}
	if n == -1 {
		dc.invalidateLocal(alsoDelete)
		return true, next, nil
	}
	return false, n, nil
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

// sAddIfRecordScript adds ARGV[1] to the set KEYS[2] only if the JSON record
// at KEYS[1] exists and, for each following (field, value) pair of ARGV,
// has that string value. It returns -1 if the record is missing, 0 if a
// field differs, and 1 once the member is added.
var sAddIfRecordScript = redis.NewScript(`
local raw = redis.call('GET', KEYS[1])
if not raw then return -1 end
if #ARGV > 1 then
	local record = cjson.decode(raw)
	for i = 2, #ARGV, 2 do
		if record[ARGV[i]] ~= ARGV[i + 1] then return 0 end
	end
end
redis.call('SADD', KEYS[2], ARGV[1])
return 1
`)

// SAddIf results.
const (
	SAddRecordMissing = -1
	SAddFieldMismatch = 0
	SAddAdded         = 1
)

// SAddIf atomically adds member to the set setKey only if the JSON record at
// recordKey exists (read from Redis, never the local cache) and its string
// fields equal want. It returns SAddAdded, SAddRecordMissing or
// SAddFieldMismatch.
func (dc *DistributedCache) SAddIf(ctx context.Context, recordKey, setKey, member string, want map[string]string) (int, error) {
	args := make([]any, 0, 1+2*len(want))
	args = append(args, member)
	for field, value := range want {
		args = append(args, field, value)
	}
	return sAddIfRecordScript.Run(ctx, dc.redis,
		[]string{dc.keyPrefix + recordKey, dc.keyPrefix + setKey}, args...).Int()
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

// Close stops the local cache. The Redis client is shared and owned by the
// caller (see CloseRedis), so it is not closed here.
func (dc *DistributedCache) Close() {
	dc.local.Close()
}
