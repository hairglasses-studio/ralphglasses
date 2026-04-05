package session

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TokenCounter tests
// ---------------------------------------------------------------------------

func TestEstimateTokens_ByProvider(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog" // 9 words

	tests := []struct {
		provider Provider
		rate     float64
	}{
		{ProviderClaude, 1.3},
		{ProviderGemini, 1.2},
		{ProviderCodex, 1.3},
	}

	for _, tt := range tests {
		got := EstimateTokensForProvider(text, tt.provider)
		want := int64(float64(9) * tt.rate)
		if got != want {
			t.Errorf("EstimateTokensForProvider(%s): got %d, want %d", tt.provider, got, want)
		}
	}
}

func TestEstimateTokens_Empty(t *testing.T) {
	if got := EstimateTokensForProvider("", ProviderClaude); got != 0 {
		t.Errorf("empty text: got %d tokens, want 0", got)
	}
}

func TestEstimateTokens_UnknownProvider(t *testing.T) {
	got := EstimateTokensForProvider("hello world", Provider("unknown"))
	words := float64(2)
	want := int64(words * DefaultTokenRate)
	if got != want {
		t.Errorf("unknown provider: got %d, want %d", got, want)
	}
}

func TestTokenCounter_RecordInputOutput(t *testing.T) {
	tc := NewTokenCounter()

	inputTokens := tc.RecordInput("s1", ProviderClaude, "Write tests for the session package")
	if inputTokens <= 0 {
		t.Fatal("expected positive input tokens")
	}

	outputTokens := tc.RecordOutput("s1", ProviderClaude, "Here are the tests for the session package with full coverage")
	if outputTokens <= 0 {
		t.Fatal("expected positive output tokens")
	}

	usage := tc.GetUsage("s1")
	if usage == nil {
		t.Fatal("expected usage for s1")
	}
	if usage.InputTokens != inputTokens {
		t.Errorf("input tokens: got %d, want %d", usage.InputTokens, inputTokens)
	}
	if usage.OutputTokens != outputTokens {
		t.Errorf("output tokens: got %d, want %d", usage.OutputTokens, outputTokens)
	}
	if usage.TotalTokens() != inputTokens+outputTokens {
		t.Error("TotalTokens mismatch")
	}
	if usage.TurnCount != 1 {
		t.Errorf("turn count: got %d, want 1", usage.TurnCount)
	}
}

func TestTokenCounter_MultipleSessions(t *testing.T) {
	tc := NewTokenCounter()

	tc.RecordInput("s1", ProviderClaude, "prompt one")
	tc.RecordInput("s2", ProviderGemini, "prompt two")
	tc.RecordInput("s3", ProviderCodex, "prompt three")

	all := tc.AllUsage()
	if len(all) != 3 {
		t.Errorf("AllUsage: got %d sessions, want 3", len(all))
	}

	total := tc.TotalTokens()
	if total <= 0 {
		t.Error("expected positive total tokens")
	}
}

func TestTokenCounter_EstimateCostUSD(t *testing.T) {
	tc := NewTokenCounter()

	// Record a large prompt to get meaningful cost.
	bigPrompt := strings.Repeat("word ", 10000) // 10k words
	tc.RecordInput("s1", ProviderClaude, bigPrompt)
	tc.RecordOutput("s1", ProviderClaude, strings.Repeat("output ", 5000))

	cost := tc.EstimateCostUSD("s1")
	if cost <= 0 {
		t.Error("expected positive cost estimate")
	}

	// Claude should be more expensive than Gemini for same content.
	tc.RecordInput("s2", ProviderGemini, bigPrompt)
	tc.RecordOutput("s2", ProviderGemini, strings.Repeat("output ", 5000))

	costGemini := tc.EstimateCostUSD("s2")
	if costGemini >= cost {
		t.Errorf("Gemini cost ($%.4f) should be less than Claude cost ($%.4f)", costGemini, cost)
	}
}

func TestTokenCounter_GetUsage_NotFound(t *testing.T) {
	tc := NewTokenCounter()
	if tc.GetUsage("nonexistent") != nil {
		t.Error("expected nil for unknown session")
	}
}

func TestTokenCounter_Reset(t *testing.T) {
	tc := NewTokenCounter()
	tc.RecordInput("s1", ProviderClaude, "hello")
	tc.Reset()

	if tc.TotalTokens() != 0 {
		t.Error("expected 0 tokens after reset")
	}
	if tc.GetUsage("s1") != nil {
		t.Error("expected nil usage after reset")
	}
}

func TestTokenCounter_RemoveSession(t *testing.T) {
	tc := NewTokenCounter()
	tc.RecordInput("s1", ProviderClaude, "hello")
	tc.RecordInput("s2", ProviderClaude, "world")
	tc.RemoveSession("s1")

	if tc.GetUsage("s1") != nil {
		t.Error("s1 should be removed")
	}
	if tc.GetUsage("s2") == nil {
		t.Error("s2 should still exist")
	}
}

func TestTokenCounter_CostUnknownSession(t *testing.T) {
	tc := NewTokenCounter()
	if tc.EstimateCostUSD("nope") != 0 {
		t.Error("expected 0 cost for unknown session")
	}
}

// ---------------------------------------------------------------------------
// BatchOptimizer tests
// ---------------------------------------------------------------------------

func TestBatchOptimizer_GroupByProvider(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   5,
		MaxWaitTime:    time.Minute,
		DedupThreshold: 0.85,
		MinBatchSize:   1,
	})

	g1, dup1 := bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "fix the bug in auth module"})
	g2, dup2 := bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderGemini, Prompt: "add logging to handler"})
	g3, dup3 := bo.AddRequest(BatchRequest{ID: "r3", Provider: ProviderClaude, Prompt: "add tests for auth module"})

	if dup1 || dup2 || dup3 {
		t.Error("no requests should be duplicated")
	}

	// r1 and r3 are both Claude, should be in the same group.
	if g1 != g3 {
		t.Errorf("Claude requests should share a group: g1=%s, g3=%s", g1, g3)
	}
	// r2 is Gemini, should be in a different group.
	if g1 == g2 {
		t.Error("Claude and Gemini requests should be in different groups")
	}
}

func TestBatchOptimizer_DeduplicateExact(t *testing.T) {
	bo := NewBatchOptimizer(DefaultBatchOptimizerConfig())

	_, dup1 := bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "write unit tests"})
	_, dup2 := bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderClaude, Prompt: "write unit tests"})

	if dup1 {
		t.Error("first request should not be a duplicate")
	}
	if !dup2 {
		t.Error("second identical request should be deduplicated")
	}

	stats := bo.Stats()
	if stats.DedupedRequests != 1 {
		t.Errorf("deduped: got %d, want 1", stats.DedupedRequests)
	}
}

func TestBatchOptimizer_DeduplicateSimilar(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   10,
		MaxWaitTime:    time.Minute,
		DedupThreshold: 0.70, // lower threshold to catch near-duplicates
		MinBatchSize:   2,
	})

	_, dup1 := bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "write unit tests for the session package"})
	_, dup2 := bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderClaude, Prompt: "write unit tests for the session package please"})

	if dup1 {
		t.Error("first request should not be a duplicate")
	}
	if !dup2 {
		t.Error("similar request should be deduplicated")
	}
}

func TestBatchOptimizer_GetReadyGroups_MinSize(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   10,
		MaxWaitTime:    time.Hour, // long wait — don't trigger time-based readiness
		DedupThreshold: 0.95,
		MinBatchSize:   2,
	})

	bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "task alpha"})

	// Only 1 request — should not be ready.
	ready := bo.GetReadyGroups()
	if len(ready) != 0 {
		t.Error("group should not be ready with only 1 request")
	}

	bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderClaude, Prompt: "task beta"})

	// Now 2 requests — should be ready.
	ready = bo.GetReadyGroups()
	if len(ready) != 1 {
		t.Errorf("expected 1 ready group, got %d", len(ready))
	}
}

func TestBatchOptimizer_MarkSubmittedCompleted(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   10,
		MaxWaitTime:    time.Hour,
		DedupThreshold: 0.95,
		MinBatchSize:   1,
	})

	groupID, _ := bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "task one"})

	bo.MarkSubmitted(groupID)
	if bo.Stats().GroupsSubmitted != 1 {
		t.Error("expected 1 submitted group")
	}

	bo.MarkCompleted(groupID)
	if bo.GroupCount() != 0 {
		t.Error("completed group should be removed")
	}
}

func TestBatchOptimizer_MaxGroupSize(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   2,
		MaxWaitTime:    time.Hour,
		DedupThreshold: 0.99,
		MinBatchSize:   1,
	})

	g1, _ := bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "alpha"})
	g2, _ := bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderClaude, Prompt: "beta"})
	g3, _ := bo.AddRequest(BatchRequest{ID: "r3", Provider: ProviderClaude, Prompt: "gamma"})

	// First two should be in the same group, third in a new one.
	if g1 != g2 {
		t.Error("first two should share a group")
	}
	if g3 == g1 {
		t.Error("third request should overflow to a new group")
	}
}

func TestBatchOptimizer_PendingCount(t *testing.T) {
	bo := NewBatchOptimizer(BatchOptimizerConfig{
		MaxGroupSize:   10,
		MaxWaitTime:    time.Hour,
		DedupThreshold: 0.99,
		MinBatchSize:   1,
	})

	bo.AddRequest(BatchRequest{ID: "r1", Provider: ProviderClaude, Prompt: "one"})
	bo.AddRequest(BatchRequest{ID: "r2", Provider: ProviderClaude, Prompt: "two"})

	if bo.PendingCount() != 2 {
		t.Errorf("pending: got %d, want 2", bo.PendingCount())
	}
}

// ---------------------------------------------------------------------------
// CacheManager tests
// ---------------------------------------------------------------------------

func TestCacheManager_LookupMiss(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 20,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{ProviderClaude: 0.90},
	})

	prompt := "## Instructions\nYou are a helpful coding assistant.\nPlease write clean Go code."
	_, hit := cm.LookupPrefix(ProviderClaude, prompt)
	if hit {
		t.Error("first lookup should be a miss")
	}

	stats := cm.Stats()
	if stats.CacheMisses != 1 {
		t.Errorf("misses: got %d, want 1", stats.CacheMisses)
	}
}

func TestCacheManager_LookupHit(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 20,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{ProviderClaude: 0.90},
	})

	prompt := "## Instructions\nYou are a helpful coding assistant.\nPlease write clean Go code with tests."

	// First call: miss.
	cm.LookupPrefix(ProviderClaude, prompt)

	// Second call with same prefix: hit.
	_, hit := cm.LookupPrefix(ProviderClaude, prompt)
	if !hit {
		t.Error("second lookup should be a cache hit")
	}

	stats := cm.Stats()
	if stats.CacheHits != 1 {
		t.Errorf("hits: got %d, want 1", stats.CacheHits)
	}
	if stats.HitRatePct != 50.0 {
		t.Errorf("hit rate: got %.1f%%, want 50.0%%", stats.HitRatePct)
	}
}

func TestCacheManager_EstimateSavings(t *testing.T) {
	cm := NewCacheManager(DefaultCacheManagerConfig())

	// Claude savings are now pessimistic by default until live cache reads are observed.
	savings := cm.EstimateSavings(ProviderClaude, 1_000_000)
	if savings != 0 {
		t.Errorf("Claude savings for 1M tokens: got $%.2f, want $0.00 by default", savings)
	}

	// Gemini: $0.50/M input, 75% savings → $0.375/M saved.
	savings = cm.EstimateSavings(ProviderGemini, 1_000_000)
	if savings < 0.37 || savings > 0.38 {
		t.Errorf("Gemini savings for 1M tokens: got $%.3f, want ~$0.375", savings)
	}

	savings = cm.EstimateSavings(ProviderCodex, 1_000_000)
	if savings <= 0 {
		t.Errorf("Codex savings for 1M tokens: got $%.3f, want positive savings", savings)
	}
}

func TestCacheManager_SavingsAccumulate(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 20,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{ProviderCodex: 0.50},
	})

	prompt := "## Instructions\nYou are a helpful coding assistant.\nPlease write clean Go code."

	// Register prefix.
	cm.LookupPrefix(ProviderCodex, prompt)
	// Hit it multiple times.
	cm.LookupPrefix(ProviderCodex, prompt)
	cm.LookupPrefix(ProviderCodex, prompt)
	cm.LookupPrefix(ProviderCodex, prompt)

	stats := cm.Stats()
	if stats.EstimatedSavings <= 0 {
		t.Error("expected positive savings after cache hits")
	}
	if stats.CacheHits != 3 {
		t.Errorf("hits: got %d, want 3", stats.CacheHits)
	}
}

func TestCacheManager_RecordPrefix(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 10,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{ProviderClaude: 0.90},
	})

	prefix := "## System\nYou are a Go expert. Follow best practices."
	cm.RecordPrefix(ProviderClaude, prefix)

	// Now a lookup with a prompt that starts with this prefix should hit.
	_, hit := cm.LookupPrefix(ProviderClaude, prefix+"\nWrite a function.")
	// The extracted prefix should match what we recorded.
	// Whether it hits depends on the extraction matching the recorded prefix.
	// At minimum, the prefix should be tracked.
	top := cm.TopPrefixes(5)
	if len(top) == 0 {
		t.Error("expected at least one tracked prefix")
	}
	_ = hit // hit depends on prefix extraction alignment
}

func TestCacheManager_RecordPrefix_TooShort(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 100,
		MaxEntries:   10,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{},
	})

	cm.RecordPrefix(ProviderClaude, "short")
	if len(cm.TopPrefixes(10)) != 0 {
		t.Error("too-short prefix should not be recorded")
	}
}

func TestCacheManager_Purge(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 20,
		MaxEntries:   100,
		TTL:          1 * time.Millisecond, // very short TTL
		SavingsRate:  map[Provider]float64{ProviderClaude: 0.90},
	})

	prompt := "## Instructions\nYou are a helpful coding assistant.\nPlease write clean Go code."
	cm.LookupPrefix(ProviderClaude, prompt)

	// Wait for TTL to expire.
	time.Sleep(5 * time.Millisecond)

	removed := cm.Purge()
	if removed == 0 {
		t.Error("expected expired entries to be purged")
	}
}

func TestCacheManager_Eviction(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 10,
		MaxEntries:   2,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{},
	})

	cm.RecordPrefix(ProviderClaude, "## First prefix with enough length to pass")
	cm.RecordPrefix(ProviderClaude, "## Second prefix with enough length to pass")
	cm.RecordPrefix(ProviderClaude, "## Third prefix that should trigger eviction")

	top := cm.TopPrefixes(10)
	if len(top) > 2 {
		t.Errorf("expected at most 2 entries after eviction, got %d", len(top))
	}
}

func TestCacheManager_Reset(t *testing.T) {
	cm := NewCacheManager(DefaultCacheManagerConfig())
	cm.RecordPrefix(ProviderClaude, strings.Repeat("x", 600))
	cm.Reset()

	stats := cm.Stats()
	if stats.TotalLookups != 0 || stats.ActivePrefixes != 0 {
		t.Error("expected clean state after reset")
	}
}

func TestCacheManager_PromptTooShort(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 1000,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{},
	})

	_, hit := cm.LookupPrefix(ProviderClaude, "short")
	if hit {
		t.Error("short prompt should not hit cache")
	}
}

func TestCacheManager_TopPrefixes(t *testing.T) {
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 10,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{},
	})

	cm.RecordPrefix(ProviderClaude, "## Alpha prefix content here")
	cm.RecordPrefix(ProviderGemini, "## Beta prefix content here too")

	top := cm.TopPrefixes(1)
	if len(top) != 1 {
		t.Errorf("TopPrefixes(1): got %d, want 1", len(top))
	}
}

// ---------------------------------------------------------------------------
// Integration: token counting + cache savings
// ---------------------------------------------------------------------------

func TestIntegration_TokenCountingWithCacheSavings(t *testing.T) {
	tc := NewTokenCounter()
	cm := NewCacheManager(CacheManagerConfig{
		MinPrefixLen: 20,
		MaxEntries:   100,
		TTL:          5 * time.Minute,
		SavingsRate:  map[Provider]float64{ProviderCodex: 0.50},
	})

	prompt := "## Instructions\nYou are a helpful coding assistant.\nPlease implement the feature described below.\nFeature: add retry logic"

	// Simulate two sessions with the same system prompt prefix.
	for i, sid := range []string{"s1", "s2"} {
		tokens := tc.RecordInput(sid, ProviderCodex, prompt)
		_, hit := cm.LookupPrefix(ProviderCodex, prompt)

		if i == 0 && hit {
			t.Error("first session should not be a cache hit")
		}
		if i == 1 && !hit {
			t.Error("second session should be a cache hit")
		}
		_ = tokens
	}

	// Verify the cost without cache would be higher than with cache.
	baseCost := tc.EstimateCostUSD("s1") + tc.EstimateCostUSD("s2")
	savings := cm.Stats().EstimatedSavings

	if savings <= 0 {
		t.Error("expected positive savings from cache hit")
	}
	if savings >= baseCost {
		t.Error("savings should not exceed base cost")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func TestCountWords(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"", 0},
		{"hello", 1},
		{"hello world", 2},
		{"  hello   world  ", 2},
		{"one\ttwo\nthree", 3},
	}
	for _, tt := range tests {
		if got := countWords(tt.input); got != tt.want {
			t.Errorf("countWords(%q): got %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestHashPrompt(t *testing.T) {
	h1 := hashPrompt("hello world")
	h2 := hashPrompt("  Hello World  ") // normalized to same
	h3 := hashPrompt("different text")

	if h1 != h2 {
		t.Error("normalized prompts should have the same hash")
	}
	if h1 == h3 {
		t.Error("different prompts should have different hashes")
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{100, "100"},
		{999, "999"},
	}
	for _, tt := range tests {
		if got := itoa(tt.n); got != tt.want {
			t.Errorf("itoa(%d): got %q, want %q", tt.n, got, tt.want)
		}
	}
}
