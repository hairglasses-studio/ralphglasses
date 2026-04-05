package promptdj

import (
	"context"
	"testing"
)

func TestABRoutingRunner_Basic(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	cfg.EnhanceThreshold = 0 // disable enhancement for test speed
	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())
	runner := NewABRoutingRunner(router)

	test := ABRoutingTest{
		Name: "threshold-comparison",
		StrategyA: RoutingStrategy{
			Name:             "conservative",
			EnhanceThreshold: 70,
			MinConfidence:    0.7,
		},
		StrategyB: RoutingStrategy{
			Name:             "aggressive",
			EnhanceThreshold: 30,
			MinConfidence:    0.3,
		},
		Prompts: []string{
			"Write a Go function that implements a concurrent-safe LRU cache",
			"Analyze the performance characteristics of this database query and suggest optimizations",
			"Create a REST API with authentication middleware and rate limiting",
		},
	}

	result, err := runner.Run(context.Background(), test)
	if err != nil {
		t.Fatalf("A/B test failed: %v", err)
	}
	if result.TestName != "threshold-comparison" {
		t.Errorf("expected test name, got %s", result.TestName)
	}
	if result.Winner == "" {
		t.Error("expected a winner (A, B, or tie)")
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		t.Errorf("confidence out of range: %.3f", result.Confidence)
	}
	if result.StrategyA.Name != "conservative" {
		t.Errorf("expected strategy A name, got %s", result.StrategyA.Name)
	}
}

func TestABRoutingRunner_EmptyPrompts(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Enabled = true
	router := NewPromptDJRouter(nil, nil, nil, nil, nil, cfg, t.TempDir())
	runner := NewABRoutingRunner(router)

	_, err := runner.Run(context.Background(), ABRoutingTest{
		Name:    "empty",
		Prompts: nil,
	})
	if err == nil {
		t.Error("expected error for empty prompts")
	}
}
