package fleet

import (
	"errors"
	"sync"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- NodeCapabilities method tests ---

func TestHasProvider(t *testing.T) {
	nc := NodeCapabilities{
		Providers: []session.Provider{session.ProviderClaude, session.ProviderGemini},
	}
	if !nc.HasProvider(session.ProviderClaude) {
		t.Error("expected HasProvider(claude) = true")
	}
	if !nc.HasProvider(session.ProviderGemini) {
		t.Error("expected HasProvider(gemini) = true")
	}
	if nc.HasProvider(session.ProviderCodex) {
		t.Error("expected HasProvider(codex) = false")
	}
}

func TestHasProvider_Empty(t *testing.T) {
	nc := NodeCapabilities{}
	if nc.HasProvider(session.ProviderClaude) {
		t.Error("expected HasProvider on empty providers = false")
	}
}

func TestHasLanguage(t *testing.T) {
	nc := NodeCapabilities{
		SupportedLanguages: []string{"go", "python", "rust"},
	}
	if !nc.HasLanguage("go") {
		t.Error("expected HasLanguage(go) = true")
	}
	if nc.HasLanguage("java") {
		t.Error("expected HasLanguage(java) = false")
	}
}

func TestHasLanguage_Empty(t *testing.T) {
	nc := NodeCapabilities{}
	if nc.HasLanguage("go") {
		t.Error("expected HasLanguage on empty = false")
	}
}

// --- Satisfies tests ---

func TestSatisfies_AllFieldsMatch(t *testing.T) {
	nc := NodeCapabilities{
		Providers:          []session.Provider{session.ProviderClaude, session.ProviderGemini},
		GPUCount:           2,
		MemoryGB:           64,
		MaxConcurrent:      8,
		SupportedLanguages: []string{"go", "python"},
	}
	req := TaskRequirements{
		Providers:     []session.Provider{session.ProviderClaude},
		MinGPUs:       1,
		MinMemoryGB:   32,
		MinConcurrent: 4,
		Languages:     []string{"go"},
	}
	if !nc.Satisfies(req) {
		t.Error("expected node to satisfy requirements")
	}
}

func TestSatisfies_ZeroRequirements(t *testing.T) {
	nc := NodeCapabilities{NodeID: "n1"}
	req := TaskRequirements{} // no requirements
	if !nc.Satisfies(req) {
		t.Error("zero-value requirements should match any node")
	}
}

func TestSatisfies_ProviderMismatch(t *testing.T) {
	nc := NodeCapabilities{
		Providers: []session.Provider{session.ProviderGemini},
	}
	req := TaskRequirements{
		Providers: []session.Provider{session.ProviderClaude, session.ProviderCodex},
	}
	if nc.Satisfies(req) {
		t.Error("expected provider mismatch to fail")
	}
}

func TestSatisfies_ProviderAnyOf(t *testing.T) {
	nc := NodeCapabilities{
		Providers: []session.Provider{session.ProviderCodex},
	}
	req := TaskRequirements{
		Providers: []session.Provider{session.ProviderClaude, session.ProviderCodex},
	}
	if !nc.Satisfies(req) {
		t.Error("node supports codex which is one of the required providers")
	}
}

func TestSatisfies_InsufficientGPUs(t *testing.T) {
	nc := NodeCapabilities{GPUCount: 1}
	req := TaskRequirements{MinGPUs: 2}
	if nc.Satisfies(req) {
		t.Error("expected GPU requirement to fail")
	}
}

func TestSatisfies_InsufficientMemory(t *testing.T) {
	nc := NodeCapabilities{MemoryGB: 16}
	req := TaskRequirements{MinMemoryGB: 32}
	if nc.Satisfies(req) {
		t.Error("expected memory requirement to fail")
	}
}

func TestSatisfies_InsufficientConcurrent(t *testing.T) {
	nc := NodeCapabilities{MaxConcurrent: 2}
	req := TaskRequirements{MinConcurrent: 4}
	if nc.Satisfies(req) {
		t.Error("expected concurrent requirement to fail")
	}
}

func TestSatisfies_MissingLanguage(t *testing.T) {
	nc := NodeCapabilities{
		SupportedLanguages: []string{"go", "python"},
	}
	req := TaskRequirements{
		Languages: []string{"go", "rust"},
	}
	if nc.Satisfies(req) {
		t.Error("expected missing language (rust) to fail")
	}
}

func TestSatisfies_AllLanguagesRequired(t *testing.T) {
	nc := NodeCapabilities{
		SupportedLanguages: []string{"go", "python", "rust"},
	}
	req := TaskRequirements{
		Languages: []string{"go", "rust"},
	}
	if !nc.Satisfies(req) {
		t.Error("node supports both go and rust")
	}
}

func TestSatisfies_ExactBoundary(t *testing.T) {
	nc := NodeCapabilities{
		GPUCount:      2,
		MemoryGB:      32,
		MaxConcurrent: 4,
	}
	req := TaskRequirements{
		MinGPUs:       2,
		MinMemoryGB:   32,
		MinConcurrent: 4,
	}
	if !nc.Satisfies(req) {
		t.Error("exact boundary values should satisfy requirements")
	}
}

// --- CapabilityMatcher tests ---

func sampleNodes() []NodeCapabilities {
	return []NodeCapabilities{
		{
			NodeID:             "node-gpu-1",
			Providers:          []session.Provider{session.ProviderClaude, session.ProviderGemini},
			GPUCount:           2,
			MemoryGB:           128,
			MaxConcurrent:      8,
			SupportedLanguages: []string{"go", "python", "rust", "typescript"},
		},
		{
			NodeID:             "node-cpu-1",
			Providers:          []session.Provider{session.ProviderClaude},
			GPUCount:           0,
			MemoryGB:           32,
			MaxConcurrent:      4,
			SupportedLanguages: []string{"go", "python"},
		},
		{
			NodeID:             "node-gpu-2",
			Providers:          []session.Provider{session.ProviderClaude, session.ProviderGemini, session.ProviderCodex},
			GPUCount:           4,
			MemoryGB:           256,
			MaxConcurrent:      16,
			SupportedLanguages: []string{"go", "python", "rust", "typescript", "java"},
		},
	}
}

func setupMatcher() *CapabilityMatcher {
	cm := NewCapabilityMatcher()
	for _, n := range sampleNodes() {
		cm.Register(n)
	}
	return cm
}

func TestNewCapabilityMatcher(t *testing.T) {
	cm := NewCapabilityMatcher()
	if cm == nil {
		t.Fatal("expected non-nil matcher")
	}
	all := cm.All()
	if len(all) != 0 {
		t.Fatalf("expected empty matcher, got %d nodes", len(all))
	}
}

func TestRegisterAndGet(t *testing.T) {
	cm := NewCapabilityMatcher()
	caps := NodeCapabilities{
		NodeID:    "n1",
		GPUCount:  2,
		MemoryGB:  64,
		Providers: []session.Provider{session.ProviderClaude},
	}
	cm.Register(caps)

	got, ok := cm.Get("n1")
	if !ok {
		t.Fatal("expected node n1 to be registered")
	}
	if got.GPUCount != 2 {
		t.Errorf("got GPUCount=%d, want 2", got.GPUCount)
	}
	if got.MemoryGB != 64 {
		t.Errorf("got MemoryGB=%f, want 64", got.MemoryGB)
	}
}

func TestRegister_UpdateExisting(t *testing.T) {
	cm := NewCapabilityMatcher()
	cm.Register(NodeCapabilities{NodeID: "n1", GPUCount: 1})
	cm.Register(NodeCapabilities{NodeID: "n1", GPUCount: 4}) // update

	got, ok := cm.Get("n1")
	if !ok {
		t.Fatal("expected node to exist")
	}
	if got.GPUCount != 4 {
		t.Errorf("got GPUCount=%d after update, want 4", got.GPUCount)
	}
}

func TestGet_NotFound(t *testing.T) {
	cm := NewCapabilityMatcher()
	_, ok := cm.Get("nonexistent")
	if ok {
		t.Error("expected ok=false for unregistered node")
	}
}

func TestRemove(t *testing.T) {
	cm := setupMatcher()
	cm.Remove("node-cpu-1")

	_, ok := cm.Get("node-cpu-1")
	if ok {
		t.Error("expected node-cpu-1 to be removed")
	}

	all := cm.All()
	if len(all) != 2 {
		t.Errorf("expected 2 remaining nodes, got %d", len(all))
	}
}

func TestRemove_Nonexistent(t *testing.T) {
	cm := setupMatcher()
	cm.Remove("nonexistent") // should not panic
	if len(cm.All()) != 3 {
		t.Error("removing nonexistent node should not affect registry")
	}
}

func TestAll_SortedByID(t *testing.T) {
	cm := setupMatcher()
	all := cm.All()
	if len(all) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(all))
	}
	// Should be sorted: node-cpu-1, node-gpu-1, node-gpu-2
	expected := []string{"node-cpu-1", "node-gpu-1", "node-gpu-2"}
	for i, want := range expected {
		if all[i].NodeID != want {
			t.Errorf("All()[%d].NodeID = %s, want %s", i, all[i].NodeID, want)
		}
	}
}

func TestMatch_ByProvider(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{
		Providers: []session.Provider{session.ProviderCodex},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matched))
	}
	if matched[0].NodeID != "node-gpu-2" {
		t.Errorf("expected node-gpu-2, got %s", matched[0].NodeID)
	}
}

func TestMatch_ByGPU(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{MinGPUs: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 nodes with >= 2 GPUs, got %d", len(matched))
	}
	ids := map[string]bool{}
	for _, n := range matched {
		ids[n.NodeID] = true
	}
	if !ids["node-gpu-1"] || !ids["node-gpu-2"] {
		t.Errorf("expected node-gpu-1 and node-gpu-2, got %v", ids)
	}
}

func TestMatch_ByMemory(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{MinMemoryGB: 100})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 nodes with >= 100 GB, got %d", len(matched))
	}
}

func TestMatch_ByConcurrent(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{MinConcurrent: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("expected 1 node with >= 10 concurrent, got %d", len(matched))
	}
	if matched[0].NodeID != "node-gpu-2" {
		t.Errorf("expected node-gpu-2, got %s", matched[0].NodeID)
	}
}

func TestMatch_ByLanguage(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{
		Languages: []string{"rust", "typescript"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 2 {
		t.Fatalf("expected 2 nodes with rust+typescript, got %d", len(matched))
	}
}

func TestMatch_CombinedRequirements(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{
		Providers:     []session.Provider{session.ProviderGemini},
		MinGPUs:       3,
		MinMemoryGB:   200,
		MinConcurrent: 10,
		Languages:     []string{"java"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 1 {
		t.Fatalf("expected exactly 1 match, got %d", len(matched))
	}
	if matched[0].NodeID != "node-gpu-2" {
		t.Errorf("expected node-gpu-2, got %s", matched[0].NodeID)
	}
}

func TestMatch_NoResults(t *testing.T) {
	cm := setupMatcher()
	_, err := cm.Match(TaskRequirements{MinGPUs: 100})
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("expected ErrNoMatch, got %v", err)
	}
}

func TestMatch_EmptyRegistry(t *testing.T) {
	cm := NewCapabilityMatcher()
	_, err := cm.Match(TaskRequirements{})
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("expected ErrNoMatch on empty registry, got %v", err)
	}
}

func TestMatch_EmptyRequirements(t *testing.T) {
	cm := setupMatcher()
	matched, err := cm.Match(TaskRequirements{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(matched) != 3 {
		t.Errorf("empty requirements should match all 3 nodes, got %d", len(matched))
	}
}

func TestMatchIDs(t *testing.T) {
	cm := setupMatcher()
	ids, err := cm.MatchIDs(TaskRequirements{
		Providers: []session.Provider{session.ProviderGemini},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	// Should be sorted
	if ids[0] != "node-gpu-1" || ids[1] != "node-gpu-2" {
		t.Errorf("expected [node-gpu-1 node-gpu-2], got %v", ids)
	}
}

func TestMatchIDs_NoMatch(t *testing.T) {
	cm := setupMatcher()
	_, err := cm.MatchIDs(TaskRequirements{MinGPUs: 100})
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("expected ErrNoMatch, got %v", err)
	}
}

func TestRankByCapacity(t *testing.T) {
	cm := setupMatcher()
	ranked, err := cm.RankByCapacity(TaskRequirements{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(ranked))
	}
	// node-gpu-2 (16 concurrent) > node-gpu-1 (8) > node-cpu-1 (4)
	if ranked[0].NodeID != "node-gpu-2" {
		t.Errorf("expected node-gpu-2 first, got %s", ranked[0].NodeID)
	}
	if ranked[1].NodeID != "node-gpu-1" {
		t.Errorf("expected node-gpu-1 second, got %s", ranked[1].NodeID)
	}
	if ranked[2].NodeID != "node-cpu-1" {
		t.Errorf("expected node-cpu-1 third, got %s", ranked[2].NodeID)
	}
}

func TestRankByCapacity_TiebreakByGPU(t *testing.T) {
	cm := NewCapabilityMatcher()
	cm.Register(NodeCapabilities{NodeID: "a", MaxConcurrent: 8, GPUCount: 1, MemoryGB: 64})
	cm.Register(NodeCapabilities{NodeID: "b", MaxConcurrent: 8, GPUCount: 4, MemoryGB: 32})

	ranked, err := cm.RankByCapacity(TaskRequirements{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Same concurrent, b has more GPUs
	if ranked[0].NodeID != "b" {
		t.Errorf("expected b first (more GPUs), got %s", ranked[0].NodeID)
	}
}

func TestRankByCapacity_TiebreakByMemory(t *testing.T) {
	cm := NewCapabilityMatcher()
	cm.Register(NodeCapabilities{NodeID: "a", MaxConcurrent: 8, GPUCount: 2, MemoryGB: 64})
	cm.Register(NodeCapabilities{NodeID: "b", MaxConcurrent: 8, GPUCount: 2, MemoryGB: 128})

	ranked, err := cm.RankByCapacity(TaskRequirements{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Same concurrent and GPUs, b has more memory
	if ranked[0].NodeID != "b" {
		t.Errorf("expected b first (more memory), got %s", ranked[0].NodeID)
	}
}

func TestRankByCapacity_WithRequirements(t *testing.T) {
	cm := setupMatcher()
	ranked, err := cm.RankByCapacity(TaskRequirements{MinGPUs: 1})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ranked) != 2 {
		t.Fatalf("expected 2 GPU nodes, got %d", len(ranked))
	}
	// node-gpu-2 (4 GPUs, 16 concurrent) should be first
	if ranked[0].NodeID != "node-gpu-2" {
		t.Errorf("expected node-gpu-2 first, got %s", ranked[0].NodeID)
	}
}

func TestRankByCapacity_NoMatch(t *testing.T) {
	cm := setupMatcher()
	_, err := cm.RankByCapacity(TaskRequirements{MinGPUs: 100})
	if !errors.Is(err, ErrNoMatch) {
		t.Errorf("expected ErrNoMatch, got %v", err)
	}
}

// --- Concurrency test ---

func TestCapabilityMatcher_ConcurrentAccess(t *testing.T) {
	cm := NewCapabilityMatcher()
	var wg sync.WaitGroup

	// Concurrent registrations.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			nodeID := "node-" + string(rune('A'+id%26))
			cm.Register(NodeCapabilities{
				NodeID:        nodeID,
				GPUCount:      id % 4,
				MemoryGB:      float64(16 * (id%4 + 1)),
				MaxConcurrent: (id % 8) + 1,
				Providers:     []session.Provider{session.ProviderClaude},
			})
		}(i)
	}

	// Concurrent reads interleaved.
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = cm.All()
			_, _ = cm.Match(TaskRequirements{MinGPUs: 1})
			_, _ = cm.Get("node-A")
		}()
	}

	wg.Wait()

	// Verify the registry is consistent.
	all := cm.All()
	if len(all) == 0 {
		t.Fatal("expected at least some nodes after concurrent registration")
	}
}
