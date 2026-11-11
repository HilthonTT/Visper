package cache

import (
	"testing"
	"time"
)

func TestCacheSetGetExpire(t *testing.T) {
	c := NewCache(Options{CleanupInterval: time.Hour})
	defer c.Close()

	c.Set("a", 1, 0)
	c.Set("b", 2, time.Nanosecond)
	time.Sleep(time.Millisecond)

	if v, ok := c.Get("a"); !ok || v != 1 {
		t.Fatalf("Get(a) = %v, %v; want 1, true", v, ok)
	}
	if _, ok := c.Get("b"); ok {
		t.Fatal("expired item b should not be returned")
	}

	stats := c.GetStats()
	if stats.Hits != 1 || stats.Misses != 1 {
		t.Fatalf("stats = %+v, want 1 hit and 1 miss", stats)
	}
}

func TestCacheEviction(t *testing.T) {
	tests := []struct {
		name    string
		policy  EvictionPolicy
		touch   string // key read before inserting the overflow item
		evicted string
	}{
		{"LRU evicts least recently used", LRU, "first", "second"},
		{"FIFO evicts oldest regardless of use", FIFO, "first", "first"},
		{"LFU evicts least frequently used", LFU, "first", "second"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var evictedKey string
			c := NewCache(Options{
				CleanupInterval: time.Hour,
				MaxItems:        2,
				EvictionPolicy:  tt.policy,
				OnEvicted:       func(k string, _ any) { evictedKey = k },
			})
			defer c.Close()

			c.Set("first", 1, 0)
			time.Sleep(2 * time.Millisecond)
			c.Set("second", 2, 0)
			time.Sleep(2 * time.Millisecond)
			c.Get(tt.touch)

			c.Set("third", 3, 0)

			if evictedKey != tt.evicted {
				t.Fatalf("evicted %q, want %q", evictedKey, tt.evicted)
			}
			if c.Count() != 2 {
				t.Fatalf("Count() = %d, want 2", c.Count())
			}
		})
	}
}

func TestCacheCloseIsIdempotent(t *testing.T) {
	c := NewCache(Options{CleanupInterval: time.Millisecond})
	c.Close()
	c.Close() // used to block forever on an unbuffered channel

	// A zero cleanup interval must not panic (time.NewTicker rejects it).
	NewCache(Options{}).Close()
}

func TestShardedCacheDistributesKeys(t *testing.T) {
	sc := NewShardedCache(Options{CleanupInterval: time.Hour}, 4)
	defer sc.Close()

	for i := range 100 {
		sc.Set(string(rune('a'+i%26))+string(rune(i)), i, 0)
	}
	if sc.Count() != 100 {
		t.Fatalf("Count() = %d, want 100", sc.Count())
	}
}
