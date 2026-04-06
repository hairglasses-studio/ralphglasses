package promptdj

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// BenchmarkRoute_FullDecisionTree measures the complete 10-phase PromptDJ
// routing decision path — the hot path for every prompt dispatched.
func BenchmarkRoute_FullDecisionTree(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0 // disable enhancement to measure pure routing
	cfg.LogDecisions = false  // disable I/O for benchmark purity

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, "")

	ctx := context.Background()
	req := RoutingRequest{
		Prompt: "Implement a concurrent-safe LRU cache in Go with TTL expiration and automatic cleanup goroutine",
		Repo:   "mcpkit",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(ctx, req)
	}
}

// BenchmarkRoute_PreClassified measures routing when classification and scoring
// are pre-computed — the fast path when the caller already knows the task type.
func BenchmarkRoute_PreClassified(b *testing.B) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0
	cfg.LogDecisions = false

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, "")

	ctx := context.Background()
	req := RoutingRequest{
		Prompt:   "Review this code for security vulnerabilities in the auth middleware",
		Repo:     "mcpkit",
		TaskType: enhancer.TaskTypeAnalysis,
		Score:    85,
		Tags:     []string{"security", "go"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		router.Route(ctx, req)
	}
}

// BenchmarkComputeConfidence measures the weighted confidence score calculation.
func BenchmarkComputeConfidence(b *testing.B) {
	c := ConfidenceComponents{
		ClassificationConf: 0.85,
		QualityScore:       0.75,
		AffinityStrength:   0.80,
		HistoricalSuccess:  0.70,
		LatencyHealth:      1.0,
		DomainSpecificity:  1.0,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeConfidence(c, true, false, 0)
	}
}

// BenchmarkAffinityLookup measures the affinity matrix lookup path.
func BenchmarkAffinityLookup(b *testing.B) {
	matrix := NewAffinityMatrix()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		matrix.Lookup(enhancer.TaskTypeCode, QualityHigh)
	}
}

// BenchmarkInferDomainTags measures domain tag inference from prompt text.
func BenchmarkInferDomainTags(b *testing.B) {
	prompt := "Write a Go MCP tool handler with proper error handling and thread safety using sync.RWMutex"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		inferDomainTags(prompt, "mcpkit")
	}
}

// BenchmarkApplyDomainBoosts measures affinity weight adjustment for domain tags.
func BenchmarkApplyDomainBoosts(b *testing.B) {
	matrix := NewAffinityMatrix()
	entries := matrix.Lookup(enhancer.TaskTypeCode, QualityHigh)
	tags := []string{"go", "mcp", "testing"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		applyDomainBoosts(entries, tags)
	}
}
