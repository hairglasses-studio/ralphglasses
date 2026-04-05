package fewshot

import (
	"container/list"
	"sync"
	"time"
)

// Cache is an LRU cache for retrieval results, following the EnhancerCache pattern.
type Cache struct {
	mu      sync.RWMutex
	items   map[string]*list.Element
	order   *list.List
	maxSize int
	ttl     time.Duration
	hits    int64
	misses  int64
}

type cacheEntry struct {
	key       string
	result    *RetrievalResult
	createdAt time.Time
}

// NewCache creates a new LRU cache with the given capacity and TTL.
func NewCache(maxSize int, ttl time.Duration) *Cache {
	if maxSize <= 0 {
		maxSize = 256
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &Cache{
		items:   make(map[string]*list.Element),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a cached result. Returns nil, false if not found or expired.
func (c *Cache) Get(key string) (*RetrievalResult, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)
	if time.Since(entry.createdAt) > c.ttl {
		c.order.Remove(elem)
		delete(c.items, key)
		c.misses++
		return nil, false
	}

	c.order.MoveToFront(elem)
	c.hits++
	// Return a copy to prevent mutation
	result := *entry.result
	return &result, true
}

// Put stores a result in the cache, evicting LRU entries if at capacity.
func (c *Cache) Put(key string, result *RetrievalResult) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		elem.Value.(*cacheEntry).result = result
		elem.Value.(*cacheEntry).createdAt = time.Now()
		return
	}

	// Evict LRU if at capacity
	for c.order.Len() >= c.maxSize {
		back := c.order.Back()
		if back == nil {
			break
		}
		c.order.Remove(back)
		delete(c.items, back.Value.(*cacheEntry).key)
	}

	entry := &cacheEntry{key: key, result: result, createdAt: time.Now()}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// Clear empties the cache.
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items = make(map[string]*list.Element)
	c.order.Init()
}

// Stats returns cache hit/miss statistics.
func (c *Cache) Stats() (hits, misses int64, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.hits, c.misses, c.order.Len()
}
