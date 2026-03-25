package fleet

import (
	"hash/fnv"
	"sync"
)

const maxRecentPrompts = 10

// CacheScorer tracks recent prompt hashes per worker to boost cache-friendly
// work assignment. Workers that recently processed a similar prompt prefix are
// more likely to benefit from provider-side prompt caching.
type CacheScorer struct {
	mu            sync.Mutex
	recentPrompts map[string][]uint64 // workerID → ring of prompt prefix hashes
}

// NewCacheScorer creates a CacheScorer with an empty history.
func NewCacheScorer() *CacheScorer {
	return &CacheScorer{
		recentPrompts: make(map[string][]uint64),
	}
}

// RecordPrompt stores a hash of the prompt prefix for the given worker.
// Only the first 500 characters are hashed. The per-worker list is capped at
// maxRecentPrompts entries; the oldest entry is evicted when full.
func (cs *CacheScorer) RecordPrompt(workerID string, prompt string) {
	h := hashPrefix(prompt)

	cs.mu.Lock()
	defer cs.mu.Unlock()

	ring := cs.recentPrompts[workerID]
	if len(ring) >= maxRecentPrompts {
		// Evict oldest (front of slice).
		ring = ring[1:]
	}
	cs.recentPrompts[workerID] = append(ring, h)
}

// CacheBoost returns a multiplier for queue scoring. If the prompt prefix hash
// matches any recent prompt for the worker, returns 2.0 (likely cache hit);
// otherwise returns 1.0 (no boost).
func (cs *CacheScorer) CacheBoost(workerID string, prompt string) float64 {
	h := hashPrefix(prompt)

	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, stored := range cs.recentPrompts[workerID] {
		if stored == h {
			return 2.0
		}
	}
	return 1.0
}

// hashPrefix returns an FNV-1a hash of the first min(500, len(prompt)) characters.
func hashPrefix(prompt string) uint64 {
	prefix := prompt
	if len(prefix) > 500 {
		prefix = prefix[:500]
	}
	hasher := fnv.New64a()
	hasher.Write([]byte(prefix))
	return hasher.Sum64()
}
