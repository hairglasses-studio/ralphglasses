package knowledge

import (
	"errors"
	"sync"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/graph"
)

// stubMemory is a simple in-memory MemoryStore for testing.
type stubMemory struct {
	mu   sync.RWMutex
	data map[string]string
}

func newStubMemory() *stubMemory {
	return &stubMemory{data: make(map[string]string)}
}

func (m *stubMemory) Get(key string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.data[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

func (m *stubMemory) Put(key, value string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = value
	return nil
}

func TestNewTieredKnowledge(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)
	if tk == nil {
		t.Fatal("expected non-nil TieredKnowledge")
	}
	if tk.maxCache != 100 {
		t.Errorf("expected maxCache=100, got %d", tk.maxCache)
	}
}

func TestQueryCacheHitMiss(t *testing.T) {
	gs := graph.NewGraphStore()
	gs.AddNode(&graph.Node{ID: "n1", Kind: graph.KindFunction, Name: "HandleRequest", Package: "handler"})
	qe := graph.NewQueryEngine(gs)
	ci := graph.NewContextInjector(gs, qe)

	tk := NewTieredKnowledge(newStubMemory(), ci)

	// First query: cache miss.
	chunks, err := tk.Query("HandleRequest function", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats := tk.Stats()
	if stats["cache_misses"] != 1 {
		t.Errorf("expected 1 cache miss, got %d", stats["cache_misses"])
	}

	// Second identical query: cache hit.
	chunks2, err := tk.Query("HandleRequest function", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	stats = tk.Stats()
	if stats["cache_hits"] != 1 {
		t.Errorf("expected 1 cache hit, got %d", stats["cache_hits"])
	}

	// Results should be the same length.
	if len(chunks) != len(chunks2) {
		t.Errorf("expected same chunk count, got %d vs %d", len(chunks), len(chunks2))
	}
}

func TestInvalidate(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)

	// Populate cache directly.
	key := queryHash("test query")
	tk.mu.Lock()
	tk.cache[key] = []graph.CodeChunk{{Name: "foo"}}
	tk.hitCount[key] = 1
	tk.mu.Unlock()

	if tk.cacheSize() != 1 {
		t.Fatalf("expected cache size 1, got %d", tk.cacheSize())
	}

	tk.Invalidate("test query")

	if tk.cacheSize() != 0 {
		t.Errorf("expected cache size 0 after invalidate, got %d", tk.cacheSize())
	}
}

func TestQueryMaxChunksLimit(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)

	// Populate cache with more chunks than we will request.
	key := queryHash("many chunks")
	chunks := make([]graph.CodeChunk, 30)
	for i := range chunks {
		chunks[i] = graph.CodeChunk{Name: "chunk"}
	}
	tk.mu.Lock()
	tk.cache[key] = chunks
	tk.hitCount[key] = 1
	tk.mu.Unlock()

	result, err := tk.Query("many chunks", 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 5 {
		t.Errorf("expected 5 chunks, got %d", len(result))
	}
}

func TestQueryNilGraph(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)

	// With nil graph, should return nil without error.
	chunks, err := tk.Query("anything", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if chunks != nil {
		t.Errorf("expected nil chunks with nil graph, got %v", chunks)
	}
}

func TestEviction(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)
	tk.maxCache = 3

	// Fill cache to capacity.
	for i := range 3 {
		key := queryHash("query" + string(rune('a'+i)))
		tk.mu.Lock()
		tk.cache[key] = []graph.CodeChunk{{Name: "chunk"}}
		tk.hitCount[key] = i + 1 // ascending hit counts
		tk.mu.Unlock()
	}

	if tk.cacheSize() != 3 {
		t.Fatalf("expected cache size 3, got %d", tk.cacheSize())
	}

	// Adding a 4th entry should evict the least-accessed one.
	tk.mu.Lock()
	tk.evictLocked()
	tk.mu.Unlock()

	if tk.cacheSize() != 2 {
		t.Errorf("expected cache size 2 after eviction, got %d", tk.cacheSize())
	}
}

func TestStats(t *testing.T) {
	tk := NewTieredKnowledge(nil, nil)

	stats := tk.Stats()
	if stats["cache_hits"] != 0 || stats["cache_misses"] != 0 || stats["cache_size"] != 0 {
		t.Errorf("expected zeroed stats, got %v", stats)
	}
}

func TestConcurrentAccess(t *testing.T) {
	gs := graph.NewGraphStore()
	qe := graph.NewQueryEngine(gs)
	ci := graph.NewContextInjector(gs, qe)
	tk := NewTieredKnowledge(newStubMemory(), ci)

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, _ = tk.Query("concurrent test query", 5)
			tk.Stats()
			if n%3 == 0 {
				tk.Invalidate("concurrent test query")
			}
		}(i)
	}
	wg.Wait()
}
