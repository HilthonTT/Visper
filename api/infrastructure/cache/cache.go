package cache

import (
	"encoding/gob"
	"io"
	"os"
	"sync"
	"time"
)

type EvictionPolicy int

const (
	// LRU evicts the least recently used items
	LRU EvictionPolicy = iota
	// LFU evicts the least frequently used items
	LFU
	// FIFO evicts the oldest items
	FIFO
)

// Item represents a cache item with value and expiration time
type Item struct {
	Value       any
	Expiration  int64
	Created     time.Time
	LastAccess  time.Time
	AccessCount int
}

// IsExpired returns true if the item has expired
func (item Item) IsExpired() bool {
	if item.Expiration == 0 {
		return false
	}
	return time.Now().UnixNano() > item.Expiration
}

// Cache represents an in-memory cache
type Cache struct {
	items           map[string]Item
	mu              sync.RWMutex
	cleanupInterval time.Duration
	maxItems        int
	evictionPolicy  EvictionPolicy
	stopCleanup     chan bool
	onEvicted       func(string, any)
	stats           Stats
}

type Stats struct {
	Hits       int64
	Misses     int64
	Evictions  int64
	TotalItems int64
}

// Options configures the cache
type Options struct {
	CleanupInterval time.Duration
	MaxItems        int
	EvictionPolicy  EvictionPolicy
	OnEvicted       func(string, any)
}

// DefaultOptions returns the default cache options
func DefaultOptions() Options {
	return Options{
		CleanupInterval: 5 * time.Minute,
		MaxItems:        0, // No limit
		EvictionPolicy:  LRU,
		OnEvicted:       nil,
	}
}

// NewCache creates a new cache with the given options
func NewCache(options Options) *Cache {
	cache := &Cache{
		items:           make(map[string]Item),
		cleanupInterval: options.CleanupInterval,
		maxItems:        options.MaxItems,
		evictionPolicy:  options.EvictionPolicy,
		stopCleanup:     make(chan bool),
		onEvicted:       options.OnEvicted,
	}

	// Start the cleanup goroutine
	go cache.startCleanupTimer()

	return cache
}

// startCleanupTimer starts the timer for cleanup
func (c *Cache) startCleanupTimer() {
	ticker := time.NewTicker(c.cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.cleanup()
		case <-c.stopCleanup:
			return
		}
	}
}

// cleanup removes expired items from the cache
func (c *Cache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now().UnixNano()
	for key, item := range c.items {
		if item.Expiration > 0 && now > item.Expiration {
			delete(c.items, key)
		}
	}
}

// evict removes items according to the eviction policy
func (c *Cache) evict() {
	if c.maxItems <= 0 || len(c.items) < c.maxItems {
		return
	}

	var keyToEvict string
	var oldestTime time.Time
	var lowestCount int

	switch c.evictionPolicy {
	case LRU:
		for k, item := range c.items {
			if keyToEvict == "" || item.LastAccess.Before(oldestTime) {
				keyToEvict = k
				oldestTime = item.LastAccess
			}
		}
	case LFU:
		for k, item := range c.items {
			if keyToEvict == "" || item.AccessCount < lowestCount {
				keyToEvict = k
				lowestCount = item.AccessCount
			}
		}
	case FIFO:
		for k, item := range c.items {
			if keyToEvict == "" || item.Created.Before(oldestTime) {
				keyToEvict = k
				oldestTime = item.Created
			}
		}
	}

	if keyToEvict != "" {
		c.deleteItem(keyToEvict)
		c.stats.Evictions++
	}
}

// deleteItem removes an item and calls the onEvicted callback if set
func (c *Cache) deleteItem(key string) {
	if c.onEvicted != nil {
		if item, found := c.items[key]; found {
			c.onEvicted(key, item.Value)
			releaseItem(&item)
		}
	}

	delete(c.items, key)
}

// Set adds an item to the cache with an expiration time
func (c *Cache) Set(key string, value any, expiration time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if we need to evict an item
	_, exists := c.items[key]
	if c.maxItems > 0 && len(c.items) >= c.maxItems && !exists {
		c.evict()
	}

	var exp int64
	if expiration > 0 {
		exp = time.Now().Add(expiration).UnixNano()
	}

	c.items[key] = Item{
		Value:      value,
		Expiration: exp,
	}

	c.stats.TotalItems++
}

// Get retrieves an item from the cache
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		c.stats.Misses++
		return nil, false
	}

	// Check if the item has expired
	if item.IsExpired() {
		c.deleteItem(key)
		c.stats.Misses++
		return nil, false
	}

	// Update access stats
	item.LastAccess = time.Now()
	item.AccessCount++
	c.items[key] = item

	c.stats.Hits++

	return item.Value, true
}

func (c *Cache) GetWithExpiration(key string) (any, time.Time, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	item, found := c.items[key]
	if !found {
		c.stats.Misses++
		return nil, time.Time{}, false
	}

	// Check if the item has expired
	if item.IsExpired() {
		c.deleteItem(key)
		c.stats.Misses++
		return nil, time.Time{}, false
	}

	item.LastAccess = time.Now()
	item.AccessCount++
	c.items[key] = item

	c.stats.Hits++

	var expiration time.Time
	if item.Expiration > 0 {
		expiration = time.Unix(0, item.Expiration)
	}

	return item.Value, expiration, true
}

// Delete removes an item from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.deleteItem(key)
}

// Flush removes all items from the cache
func (c *Cache) Flush() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]Item)
	c.stats = Stats{}
}

// Close stops the cleanup goroutine
func (c *Cache) Close() {
	c.stopCleanup <- true
}

// Count returns the number of items in the cache
func (c *Cache) Count() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.items)
}

// GetStats returns the cache statistics
func (c *Cache) GetStats() Stats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return c.stats
}

// SaveToFile saves the cache to a file
func (c *Cache) SaveToFile(filename string) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return c.saveToWriter(file)
}

// LoadFromFile loads the cache from a file
func (c *Cache) LoadFromFile(filename string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return c.loadFromReader(file)
}

// saveToWriter encodes the cache to a writer
func (c *Cache) saveToWriter(w io.Writer) error {
	enc := gob.NewEncoder(w)

	// Only save unexpired items
	now := time.Now().UnixNano()
	items := make(map[string]Item)

	for k, v := range c.items {
		if v.Expiration == 0 || v.Expiration > now {
			items[k] = v
		}
	}

	return enc.Encode(items)
}

// loadFromReader decodes the cache from a reader
func (c *Cache) loadFromReader(r io.Reader) error {
	dec := gob.NewDecoder(r)
	items := make(map[string]Item)

	if err := dec.Decode(&items); err != nil {
		return err
	}

	// Only load unexpired items
	now := time.Now().UnixNano()
	for k, v := range items {
		if v.Expiration == 0 || v.Expiration > now {
			c.items[k] = v
		}
	}

	return nil
}
