package cache

import (
	"hash/fnv"
	"time"
)

const DefaultShards = 32

// ShardedCache distributes items across multiple shards to reduce lock contention
type ShardedCache struct {
	shards     []*Cache
	shardCount int
}

func NewShardedCache(options Options, shardCount int) *ShardedCache {
	if shardCount <= 0 {
		shardCount = DefaultShards
	}

	sc := &ShardedCache{
		shards:     make([]*Cache, shardCount),
		shardCount: shardCount,
	}

	for i := 0; i < shardCount; i++ {
		sc.shards[i] = NewCache(options)
	}

	return sc
}

// getShard returns the shard for a given key
func (sc *ShardedCache) getShard(key string) *Cache {
	hasher := fnv.New32a()
	hasher.Write([]byte(key))
	shardIndex := int(hasher.Sum32()) % sc.shardCount
	return sc.shards[shardIndex]
}

// Set adds an item to the cache
func (sc *ShardedCache) Set(key string, value any, expiration time.Duration) {
	shard := sc.getShard(key)
	shard.Set(key, value, expiration)
}

// Get retrieves an item from the cache
func (sc *ShardedCache) Get(key string) (any, bool) {
	shard := sc.getShard(key)
	return shard.Get(key)
}

// Delete removes an item from the cache
func (sc *ShardedCache) Delete(key string) {
	shard := sc.getShard(key)
	shard.Delete(key)
}

// Flush removes all items from all shards
func (sc *ShardedCache) Flush() {
	for _, shard := range sc.shards {
		shard.Flush()
	}
}

// Count returns the total number of items across all shards
func (sc *ShardedCache) Count() int {
	count := 0
	for _, shard := range sc.shards {
		count += shard.Count()
	}
	return count
}

// GetStats returns combined stats from all shards
func (sc *ShardedCache) GetStats() Stats {
	var stats Stats
	for _, shard := range sc.shards {
		shardStats := shard.GetStats()
		stats.Hits += shardStats.Hits
		stats.Misses += shardStats.Misses
		stats.Evictions += shardStats.Evictions
		stats.TotalItems += shardStats.TotalItems
	}
	return stats
}

// Close closes all shards
func (sc *ShardedCache) Close() {
	for _, shard := range sc.shards {
		shard.Close()
	}
}
