package session

import (
	"testing"
)

func TestCostPredictor_NewAndRecord(t *testing.T) {
	dir := t.TempDir()
	cp := NewCostPredictor(dir)

	if cp.ObservationCount() != 0 {
		t.Fatalf("expected 0 observations, got %d", cp.ObservationCount())
	}

	cp.Record(CostObservation{TaskType: "bug_fix", Provider: "claude", CostUSD: 0.50, TurnCount: 10})
	cp.Record(CostObservation{TaskType: "bug_fix", Provider: "claude", CostUSD: 1.50, TurnCount: 20})

	if cp.ObservationCount() != 2 {
		t.Fatalf("expected 2 observations, got %d", cp.ObservationCount())
	}
}

func TestCostPredictor_Predict(t *testing.T) {
	dir := t.TempDir()
	cp := NewCostPredictor(dir)

	// No data — returns default
	if got := cp.Predict("bug_fix", "claude"); got != 1.0 {
		t.Errorf("expected default 1.0, got %f", got)
	}

	cp.Record(CostObservation{TaskType: "bug_fix", Provider: "claude", CostUSD: 0.50})
	cp.Record(CostObservation{TaskType: "bug_fix", Provider: "claude", CostUSD: 1.50})

	// Average of 0.50 and 1.50 = 1.00
	if got := cp.Predict("bug_fix", "claude"); got != 1.0 {
		t.Errorf("expected 1.0, got %f", got)
	}

	// Different key — no data
	if got := cp.Predict("feature", "gemini"); got != 1.0 {
		t.Errorf("expected default 1.0 for unknown key, got %f", got)
	}
}

func TestCostPredictor_Persistence(t *testing.T) {
	dir := t.TempDir()

	cp1 := NewCostPredictor(dir)
	cp1.Record(CostObservation{TaskType: "test", Provider: "gemini", CostUSD: 0.25})

	// Reload from disk
	cp2 := NewCostPredictor(dir)
	if cp2.ObservationCount() != 1 {
		t.Fatalf("expected 1 observation after reload, got %d", cp2.ObservationCount())
	}
	if got := cp2.Predict("test", "gemini"); got != 0.25 {
		t.Errorf("expected 0.25 after reload, got %f", got)
	}
}

func TestCostPredictor_EmptyStateDir(t *testing.T) {
	cp := NewCostPredictor("")
	cp.Record(CostObservation{TaskType: "a", Provider: "b", CostUSD: 1.0})
	if cp.ObservationCount() != 1 {
		t.Fatalf("expected 1 observation in memory, got %d", cp.ObservationCount())
	}
}
