package e2e

import (
	"context"
	"testing"
)

func TestNewHarness(t *testing.T) {
	h := NewHarness(t)
	if h == nil {
		t.Fatal("NewHarness returned nil")
	}
	if h.Manager == nil {
		t.Error("Manager should not be nil")
	}
	if h.stateDir == "" {
		t.Error("stateDir should not be empty")
	}
	if h.t == nil {
		t.Error("testing.T should not be nil")
	}
}

func TestRunAll_EmptyScenarios(t *testing.T) {
	h := NewHarness(t)
	ctx := context.Background()
	results := h.RunAll(ctx, nil)
	if len(results) != 0 {
		t.Errorf("RunAll with nil scenarios should return empty results, got %d", len(results))
	}

	results = h.RunAll(ctx, []Scenario{})
	if len(results) != 0 {
		t.Errorf("RunAll with empty scenarios should return empty results, got %d", len(results))
	}
}

func TestScenarioResult_ObservationJSON(t *testing.T) {
	r := ScenarioResult{
		Scenario: Scenario{
			Name:          "test-scenario",
			Category:      "bug_fix",
			MockCostUSD:   0.05,
			MockTurnCount: 3,
		},
		Status: "idle",
		Error:  nil,
	}

	jsonStr := r.ObservationJSON()
	if jsonStr == "" {
		t.Fatal("ObservationJSON returned empty string")
	}
	// Should contain key fields
	for _, expected := range []string{"e2e-test-scenario", "bug_fix", "idle"} {
		if !containsStr(jsonStr, expected) {
			t.Errorf("ObservationJSON should contain %q, got: %s", expected, jsonStr)
		}
	}
}

func TestScenarioResult_ObservationJSON_FailedStatus(t *testing.T) {
	r := ScenarioResult{
		Scenario: Scenario{
			Name:     "fail-scenario",
			Category: "refactor",
		},
		Status: "failed",
	}
	jsonStr := r.ObservationJSON()
	if !containsStr(jsonStr, "failed") {
		t.Errorf("ObservationJSON should contain 'failed', got: %s", jsonStr)
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
