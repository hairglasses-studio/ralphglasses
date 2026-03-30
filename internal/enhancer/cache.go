package enhancer

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// PromptCache is an in-memory cache for LLM-improved prompts.
// SHA-256 keyed on prompt+options, 100 entries max, 10min TTL.
type PromptCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	maxSize int
	ttl     time.Duration
}

type cacheEntry struct {
	result    *ImproveResult
	createdAt time.Time
}

// NewPromptCache creates a cache with default settings.
func NewPromptCache() *PromptCache {
	return &PromptCache{
		entries: make(map[string]*cacheEntry),
		maxSize: 100,
		ttl:     10 * time.Minute,
	}
}

// Get retrieves a cached result. Returns nil if miss or expired.
func (c *PromptCache) Get(prompt string, opts ImproveOptions) *ImproveResult {
	key := c.key(prompt, opts)

	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		return nil
	}
	if time.Since(entry.createdAt) > c.ttl {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil
	}
	return entry.result
}

// Put stores a result in the cache. Evicts oldest entry if at capacity.
func (c *PromptCache) Put(prompt string, opts ImproveOptions, result *ImproveResult) {
	key := c.key(prompt, opts)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	c.entries[key] = &cacheEntry{
		result:    result,
		createdAt: time.Now(),
	}
}

func (c *PromptCache) key(prompt string, opts ImproveOptions) string {
	h := sha256.New()
	if opts.Provider != "" {
		h.Write([]byte(string(opts.Provider) + ":"))
	}
	h.Write([]byte(prompt))
	if opts.ThinkingEnabled {
		h.Write([]byte(":thinking"))
	}
	if opts.TaskType != "" {
		h.Write([]byte(":" + string(opts.TaskType)))
	}
	if opts.Feedback != "" {
		h.Write([]byte(":" + opts.Feedback))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (c *PromptCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.createdAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.createdAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// ---------------------------------------------------------------------------
// EnhancerCache — LRU cache for the deterministic enhancement pipeline
// ---------------------------------------------------------------------------

// CacheStats reports cache performance metrics.
type CacheStats struct {
	Hits       int64   `json:"hits"`
	Misses     int64   `json:"misses"`
	HitRate    float64 `json:"hit_rate"`
	Size       int     `json:"size"`
	MaxSize    int     `json:"max_size"`
	Evictions  int64   `json:"evictions"`
}

// enhancerEntry is a single item stored in the LRU list.
type enhancerEntry struct {
	key       string
	result    *EnhanceResult
	createdAt time.Time
}

// EnhancerCache is a thread-safe LRU cache for EnhanceResult values.
// Keys are SHA-256 hashes of the normalized prompt concatenated with the
// target provider, so the same prompt enhanced for different providers
// produces distinct cache entries. Entries expire after the configured TTL.
type EnhancerCache struct {
	mu       sync.RWMutex
	items    map[string]*list.Element // key -> list element
	order    *list.List               // front = most-recently used
	maxSize  int
	ttl      time.Duration
	hits     int64
	misses   int64
	evictions int64
}

// NewEnhancerCache creates a new LRU cache with the given capacity and TTL.
func NewEnhancerCache(maxSize int, ttl time.Duration) *EnhancerCache {
	if maxSize <= 0 {
		maxSize = 256
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &EnhancerCache{
		items:   make(map[string]*list.Element, maxSize),
		order:   list.New(),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// Get retrieves a cached EnhanceResult for the given prompt and provider.
// Returns the result and true on a cache hit, or nil and false on a miss
// (including TTL expiration).
func (c *EnhancerCache) Get(prompt string, provider string) (*EnhanceResult, bool) {
	key := enhancerCacheKey(prompt, provider)

	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		c.misses++
		return nil, false
	}

	entry := elem.Value.(*enhancerEntry)

	// TTL check
	if time.Since(entry.createdAt) > c.ttl {
		c.order.Remove(elem)
		delete(c.items, key)
		c.misses++
		return nil, false
	}

	// Move to front (most-recently used)
	c.order.MoveToFront(elem)
	c.hits++
	return entry.result, true
}

// Put stores an EnhanceResult in the cache. If the cache is at capacity,
// the least-recently used entry is evicted.
func (c *EnhancerCache) Put(prompt string, provider string, result *EnhanceResult) {
	key := enhancerCacheKey(prompt, provider)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update existing entry
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := elem.Value.(*enhancerEntry)
		entry.result = result
		entry.createdAt = time.Now()
		return
	}

	// Evict LRU if at capacity
	for c.order.Len() >= c.maxSize {
		c.evictLRU()
	}

	entry := &enhancerEntry{
		key:       key,
		result:    result,
		createdAt: time.Now(),
	}
	elem := c.order.PushFront(entry)
	c.items[key] = elem
}

// Invalidate removes all cache entries whose key was derived from the given
// prompt (across all providers).
func (c *EnhancerCache) Invalidate(prompt string) {
	normalized := normalizePrompt(prompt)

	c.mu.Lock()
	defer c.mu.Unlock()

	// We need to check all three known providers plus the empty-provider case.
	providers := []string{"", string(ProviderClaude), string(ProviderGemini), string(ProviderOpenAI)}
	for _, p := range providers {
		key := enhancerCacheKeyRaw(normalized, p)
		if elem, ok := c.items[key]; ok {
			c.order.Remove(elem)
			delete(c.items, key)
		}
	}
}

// Clear flushes the entire cache and resets stats.
func (c *EnhancerCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.items = make(map[string]*list.Element, c.maxSize)
	c.order.Init()
	c.hits = 0
	c.misses = 0
	c.evictions = 0
}

// Stats returns a snapshot of cache performance metrics.
func (c *EnhancerCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	var hitRate float64
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}
	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		HitRate:   hitRate,
		Size:      c.order.Len(),
		MaxSize:   c.maxSize,
		Evictions: c.evictions,
	}
}

// evictLRU removes the least-recently used entry. Caller must hold c.mu.
func (c *EnhancerCache) evictLRU() {
	tail := c.order.Back()
	if tail == nil {
		return
	}
	entry := tail.Value.(*enhancerEntry)
	c.order.Remove(tail)
	delete(c.items, entry.key)
	c.evictions++
}

// normalizePrompt trims and collapses whitespace so that cosmetically
// different versions of the same prompt share a cache entry.
func normalizePrompt(prompt string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(prompt)), " ")
}

// enhancerCacheKey computes a SHA-256 cache key from a prompt and provider.
func enhancerCacheKey(prompt string, provider string) string {
	return enhancerCacheKeyRaw(normalizePrompt(prompt), provider)
}

// enhancerCacheKeyRaw computes the SHA-256 key from an already-normalized
// prompt and a provider string.
func enhancerCacheKeyRaw(normalized string, provider string) string {
	h := sha256.New()
	h.Write([]byte(normalized))
	if provider != "" {
		h.Write([]byte(":" + provider))
	}
	return hex.EncodeToString(h.Sum(nil))
}
