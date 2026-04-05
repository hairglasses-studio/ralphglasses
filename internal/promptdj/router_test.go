package promptdj

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func TestRoute_BasicClassification(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0 // disable auto-enhancement for this test

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())

	req := RoutingRequest{
		Prompt: "Write a Go function that implements a concurrent-safe LRU cache with TTL-based expiration and automatic cleanup goroutine",
	}

	d, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if d.DecisionID == "" {
		t.Error("expected non-empty decision ID")
	}
	if d.TaskType == "" {
		t.Error("expected non-empty task type")
	}
	if d.Provider == "" {
		t.Error("expected non-empty provider")
	}
	if d.Confidence <= 0 {
		t.Errorf("expected positive confidence, got %f", d.Confidence)
	}
	if d.ConfidenceLevel == "" {
		t.Error("expected non-empty confidence level")
	}
	if d.LatencyMs < 0 {
		t.Errorf("expected non-negative latency, got %d", d.LatencyMs)
	}
}

func TestRoute_WithPreClassifiedInput(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())

	req := RoutingRequest{
		Prompt:   "Review this code for security vulnerabilities",
		TaskType: enhancer.TaskTypeAnalysis,
		Score:    85,
		Tags:     []string{"security", "go"},
	}

	d, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if d.TaskType != enhancer.TaskTypeAnalysis {
		t.Errorf("expected task type analysis, got %s", d.TaskType)
	}
	if d.OriginalScore != 85 {
		t.Errorf("expected original score 85, got %d", d.OriginalScore)
	}
	if len(d.DomainTags) == 0 {
		t.Error("expected domain tags to be preserved")
	}
}

func TestRoute_LowQualityTriggersEnhancement(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 50 // enhance if score < 50

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())

	// A deliberately vague prompt
	req := RoutingRequest{
		Prompt: "do stuff with the code and make it better",
		Score:  20, // force low score
	}

	d, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	// With score=20 and threshold=50, enhancement should have been attempted
	// (may or may not change the prompt depending on enhancer behavior)
	if d.OriginalScore != 20 {
		t.Errorf("expected original score 20, got %d", d.OriginalScore)
	}
}

func TestRoute_FallbackChain(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())

	req := RoutingRequest{
		Prompt: "Analyze the performance characteristics of this database query\nand suggest optimization strategies based on the execution plan",
		Score:  75,
	}

	d, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if len(d.FallbackChain) == 0 {
		t.Error("expected non-empty fallback chain")
	}
	// Verify Claude Opus is in the fallback chain as safety net
	hasOpus := false
	for _, fb := range d.FallbackChain {
		if fb.Provider == "claude" {
			hasOpus = true
			break
		}
	}
	if d.Provider != "claude" && !hasOpus {
		t.Error("expected Claude in fallback chain as safety net")
	}
}

func TestRoute_DecisionLogging(t *testing.T) {
	dir := t.TempDir()
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.LogDecisions = true
	cfg.EnhanceThreshold = 0

	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, dir)

	req := RoutingRequest{
		Prompt: "Implement a REST API with authentication middleware\nand rate limiting per-client",
		Score:  70,
	}

	d, err := router.Route(context.Background(), req)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	// Verify decision was logged
	log := router.GetDecisionLog()
	if log == nil {
		t.Fatal("expected non-nil decision log")
	}

	rec, ok := log.GetDecision(d.DecisionID)
	if !ok {
		t.Fatal("decision not found in log")
	}
	if rec.Provider != string(d.Provider) {
		t.Errorf("logged provider %s != decision provider %s", rec.Provider, d.Provider)
	}
}
