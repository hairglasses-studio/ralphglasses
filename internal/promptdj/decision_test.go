package promptdj

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDecisionLog_RecordAndGet(t *testing.T) {
	dir := t.TempDir()
	log := NewDecisionLog(dir)

	d := &RoutingDecision{
		DecisionID:       "test-decision-001",
		Provider:         "claude",
		Model:            "claude-opus",
		TaskType:         "code",
		Complexity:       3,
		OriginalScore:    72,
		Confidence:       0.85,
		EstimatedCostUSD: 0.15,
		Rationale:        "Test routing decision",
		Timestamp:        time.Now(),
	}

	if err := log.RecordDecision(d); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	rec, ok := log.GetDecision("test-decision-001")
	if !ok {
		t.Fatal("decision not found")
	}
	if rec.Provider != "claude" {
		t.Errorf("expected provider claude, got %s", rec.Provider)
	}
	if rec.Complexity != 3 {
		t.Errorf("expected complexity 3, got %d", rec.Complexity)
	}
	if rec.Status != "routed" {
		t.Errorf("expected status routed, got %s", rec.Status)
	}
}

func TestDecisionLog_RecordOutcome(t *testing.T) {
	dir := t.TempDir()
	log := NewDecisionLog(dir)

	d := &RoutingDecision{
		DecisionID: "test-outcome-001",
		Provider:   "gemini",
		Model:      "gemini-flash",
		TaskType:   "workflow",
		Timestamp:  time.Now(),
	}
	if err := log.RecordDecision(d); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	// Record success outcome
	if err := log.RecordOutcome("test-outcome-001", true, 0.05, 3, "completed successfully"); err != nil {
		t.Fatalf("RecordOutcome failed: %v", err)
	}

	rec, ok := log.GetDecision("test-outcome-001")
	if !ok {
		t.Fatal("decision not found after outcome")
	}
	if rec.Status != "succeeded" {
		t.Errorf("expected status succeeded, got %s", rec.Status)
	}
	if rec.ActualCost != 0.05 {
		t.Errorf("expected actual cost 0.05, got %f", rec.ActualCost)
	}
	if rec.ActualTurns != 3 {
		t.Errorf("expected 3 turns, got %d", rec.ActualTurns)
	}
}

func TestDecisionLog_Persistence(t *testing.T) {
	dir := t.TempDir()
	log1 := NewDecisionLog(dir)

	d := &RoutingDecision{
		DecisionID: "persist-001",
		Provider:   "claude",
		TaskType:   "analysis",
		Timestamp:  time.Now(),
	}
	if err := log1.RecordDecision(d); err != nil {
		t.Fatalf("RecordDecision failed: %v", err)
	}

	// Verify JSONL file was created
	jsonlPath := filepath.Join(dir, "promptdj", "decisions.jsonl")
	if _, err := os.Stat(jsonlPath); err != nil {
		t.Fatalf("JSONL file not created: %v", err)
	}

	// Create new log from same directory (simulates restart)
	log2 := NewDecisionLog(dir)
	rec, ok := log2.GetDecision("persist-001")
	if !ok {
		t.Fatal("decision not found after reload")
	}
	if rec.Provider != "claude" {
		t.Errorf("expected provider claude after reload, got %s", rec.Provider)
	}
}

func TestDecisionLog_QueryFilters(t *testing.T) {
	dir := t.TempDir()
	log := NewDecisionLog(dir)

	for _, d := range []*RoutingDecision{
		{DecisionID: "q1", Provider: "claude", TaskType: "code", Timestamp: time.Now()},
		{DecisionID: "q2", Provider: "gemini", TaskType: "workflow", Timestamp: time.Now()},
		{DecisionID: "q3", Provider: "claude", TaskType: "analysis", Timestamp: time.Now()},
	} {
		if err := log.RecordDecision(d); err != nil {
			t.Fatalf("RecordDecision failed: %v", err)
		}
	}

	// Filter by provider
	results := log.QueryDecisions(DecisionFilter{Provider: "claude"})
	if len(results) != 2 {
		t.Errorf("expected 2 claude decisions, got %d", len(results))
	}

	// Filter by task type
	results = log.QueryDecisions(DecisionFilter{TaskType: "workflow"})
	if len(results) != 1 {
		t.Errorf("expected 1 workflow decision, got %d", len(results))
	}

	// Limit
	results = log.QueryDecisions(DecisionFilter{Limit: 1})
	if len(results) != 1 {
		t.Errorf("expected 1 result with limit=1, got %d", len(results))
	}
}
