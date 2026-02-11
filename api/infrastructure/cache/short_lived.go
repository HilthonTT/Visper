package cache

import (
	"sync"
	"time"
)

var itemPool = sync.Pool{
	New: func() any {
		return new(Item)
	},
}

// getItem gets an item from the pool
func getItem(value any, expiration int64) *Item {
	item := itemPool.Get().(*Item)
	item.Value = value
	item.Expiration = expiration
	item.Created = time.Now()
	item.LastAccess = time.Now()
	item.AccessCount = 0
	return item
}

// releaseItem returns an Item to the pool
func releaseItem(item *Item) {
	item.Value = nil
	itemPool.Put(item)
}
