// E2.3: Tiered Knowledge Composer — composes TieredMemory with ContextInjector
// to provide a unified knowledge retrieval layer that checks a hot cache first,
// then falls back to graph-based context injection.
//
// Informed by Codified Context (ArXiv 2602.20478): tiered caching reduces
// retrieval latency for frequently accessed code context.
package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"sync/atomic"

	"github.com/hairglasses-studio/ralphglasses/internal/graph"
)

// MemoryStore is a simple key-value interface satisfied by hot-tier stores
// (e.g., patterns.SharedMemory via patterns.WarmStore). Defined here to
// avoid import cycles with internal/session/patterns.
type MemoryStore interface {
	Get(key string) (string, error)
	Put(key, value string) error
}

// TieredKnowledge composes a hot memory cache with graph-based context
// injection. Queries check the in-memory cache first, falling back to
// the knowledge graph for uncached lookups.
type TieredKnowledge struct {
	hot   MemoryStore
	graph *graph.ContextInjector

	mu       sync.RWMutex
	cache    map[string][]graph.CodeChunk // key = query hash
	hitCount map[string]int               // track access frequency for auto-promotion
	maxCache int                          // maximum cached queries

	// Stats tracked with atomics for lock-free reads.
	cacheHits  atomic.Int64
	cacheMisses atomic.Int64
}

// NewTieredKnowledge creates a knowledge composer with the given hot store
// and graph-based context injector. Either may be nil (nil stores are skipped).
func NewTieredKnowledge(hot MemoryStore, injector *graph.ContextInjector) *TieredKnowledge {
	return &TieredKnowledge{
		hot:      hot,
		graph:    injector,
		cache:    make(map[string][]graph.CodeChunk),
		hitCount: make(map[string]int),
		maxCache: 100,
	}
}

// Query retrieves code chunks relevant to a task description.
// It checks the in-memory cache first, then delegates to the graph-based
// context injector. Results are cached for subsequent lookups.
// maxChunks limits the number of returned chunks (default 20).
func (tk *TieredKnowledge) Query(taskDesc string, maxChunks int) ([]graph.CodeChunk, error) {
	if maxChunks <= 0 {
		maxChunks = 20
	}

	key := queryHash(taskDesc)

	// Check cache first.
	tk.mu.Lock()
	if cached, ok := tk.cache[key]; ok {
		tk.hitCount[key]++
		tk.mu.Unlock()
		tk.cacheHits.Add(1)
		if len(cached) > maxChunks {
			return cached[:maxChunks], nil
		}
		return cached, nil
	}
	tk.mu.Unlock()

	tk.cacheMisses.Add(1)

	// Fall back to graph-based context injection.
	if tk.graph == nil {
		return nil, nil
	}

	chunks := tk.graph.RelevantContext(taskDesc, maxChunks, 2)

	// Cache the result.
	tk.mu.Lock()
	defer tk.mu.Unlock()

	// Evict oldest entries if cache is full.
	if len(tk.cache) >= tk.maxCache {
		tk.evictLocked()
	}

	tk.cache[key] = chunks
	tk.hitCount[key] = 1

	return chunks, nil
}

// Invalidate removes a cached entry by its original query string.
func (tk *TieredKnowledge) Invalidate(queryStr string) {
	key := queryHash(queryStr)
	tk.mu.Lock()
	defer tk.mu.Unlock()
	delete(tk.cache, key)
	delete(tk.hitCount, key)
}

// Stats returns cache hit/miss counts.
func (tk *TieredKnowledge) Stats() map[string]int {
	return map[string]int{
		"cache_hits":   int(tk.cacheHits.Load()),
		"cache_misses": int(tk.cacheMisses.Load()),
		"cache_size":   tk.cacheSize(),
	}
}

// cacheSize returns the current number of cached entries.
func (tk *TieredKnowledge) cacheSize() int {
	tk.mu.RLock()
	defer tk.mu.RUnlock()
	return len(tk.cache)
}

// evictLocked removes the least-accessed cache entry. Caller must hold tk.mu.
func (tk *TieredKnowledge) evictLocked() {
	var minKey string
	minHits := int(^uint(0) >> 1) // max int

	for k, hits := range tk.hitCount {
		if hits < minHits {
			minHits = hits
			minKey = k
		}
	}

	if minKey != "" {
		delete(tk.cache, minKey)
		delete(tk.hitCount, minKey)
	}
}

// queryHash returns a deterministic hash for a query string, used as cache key.
func queryHash(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:16]) // 128-bit prefix is sufficient for cache keys
}
