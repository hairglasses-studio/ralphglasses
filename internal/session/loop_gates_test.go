package session

import "testing"

func TestInitBaselineFromFirstObservation_Empty(t *testing.T) {
	b := InitBaselineFromFirstObservation(nil)
	if b != nil {
		t.Errorf("expected nil for empty observations, got %+v", b)
	}
}

func TestInitBaselineFromFirstObservation_Single(t *testing.T) {
	obs := []LoopObservation{
		{
			PlannerLatencyMs: 100,
			WorkerLatencyMs:  200,
			TotalLatencyMs:   350,
			TotalCostUSD:     0.50,
			FilesChanged:     3,
			LinesAdded:       42,
		},
	}
	b := InitBaselineFromFirstObservation(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.AvgPlannerLatencyMs != 100 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 100", b.AvgPlannerLatencyMs)
	}
	if b.AvgWorkerLatencyMs != 200 {
		t.Errorf("AvgWorkerLatencyMs = %d, want 200", b.AvgWorkerLatencyMs)
	}
	if b.AvgTotalLatencyMs != 350 {
		t.Errorf("AvgTotalLatencyMs = %d, want 350", b.AvgTotalLatencyMs)
	}
	if b.AvgTotalCostUSD != 0.50 {
		t.Errorf("AvgTotalCostUSD = %f, want 0.50", b.AvgTotalCostUSD)
	}
	if b.AvgFilesChanged != 3 {
		t.Errorf("AvgFilesChanged = %d, want 3", b.AvgFilesChanged)
	}
	if b.AvgLinesAdded != 42 {
		t.Errorf("AvgLinesAdded = %d, want 42", b.AvgLinesAdded)
	}
	if b.SampleCount != 1 {
		t.Errorf("SampleCount = %d, want 1", b.SampleCount)
	}
}

func TestBaselineFromObservations_Empty(t *testing.T) {
	b := BaselineFromObservations(nil)
	if b != nil {
		t.Errorf("expected nil for empty observations, got %+v", b)
	}
}

func TestBaselineFromObservations_Multiple(t *testing.T) {
	obs := []LoopObservation{
		{
			PlannerLatencyMs: 100,
			WorkerLatencyMs:  200,
			TotalLatencyMs:   300,
			TotalCostUSD:     1.00,
			FilesChanged:     4,
			LinesAdded:       40,
		},
		{
			PlannerLatencyMs: 200,
			WorkerLatencyMs:  400,
			TotalLatencyMs:   600,
			TotalCostUSD:     3.00,
			FilesChanged:     6,
			LinesAdded:       60,
		},
	}
	b := BaselineFromObservations(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	if b.AvgPlannerLatencyMs != 150 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 150", b.AvgPlannerLatencyMs)
	}
	if b.AvgWorkerLatencyMs != 300 {
		t.Errorf("AvgWorkerLatencyMs = %d, want 300", b.AvgWorkerLatencyMs)
	}
	if b.AvgTotalCostUSD != 2.00 {
		t.Errorf("AvgTotalCostUSD = %f, want 2.00", b.AvgTotalCostUSD)
	}
	if b.SampleCount != 2 {
		t.Errorf("SampleCount = %d, want 2", b.SampleCount)
	}
}

func TestInitBaselineFromFirstObservation_IgnoresRest(t *testing.T) {
	obs := []LoopObservation{
		{PlannerLatencyMs: 100, TotalCostUSD: 1.00},
		{PlannerLatencyMs: 900, TotalCostUSD: 9.00},
	}
	b := InitBaselineFromFirstObservation(obs)
	if b == nil {
		t.Fatal("expected non-nil baseline")
	}
	// Should only use the first observation.
	if b.AvgPlannerLatencyMs != 100 {
		t.Errorf("AvgPlannerLatencyMs = %d, want 100 (first only)", b.AvgPlannerLatencyMs)
	}
	if b.AvgTotalCostUSD != 1.00 {
		t.Errorf("AvgTotalCostUSD = %f, want 1.00 (first only)", b.AvgTotalCostUSD)
	}
}
