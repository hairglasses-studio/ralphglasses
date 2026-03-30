package eval

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestAnomalyMetrics_ReturnsAllKeys(t *testing.T) {
	m := AnomalyMetrics()
	expectedKeys := []string{
		"total_cost_usd", "planner_cost_usd", "worker_cost_usd",
		"total_latency_ms", "planner_latency_ms", "worker_latency_ms",
		"verify_latency_ms", "files_changed", "lines_added",
		"lines_removed", "confidence", "difficulty_score",
	}
	for _, key := range expectedKeys {
		if _, ok := m[key]; !ok {
			t.Errorf("AnomalyMetrics() missing key %q", key)
		}
	}
}

func TestAnomalyMetrics_ExtractorFunctions(t *testing.T) {
	obs := session.LoopObservation{
		TotalCostUSD:     1.23,
		PlannerCostUSD:   0.50,
		WorkerCostUSD:    0.73,
		TotalLatencyMs:   1000,
		PlannerLatencyMs: 400,
		WorkerLatencyMs:  600,
		VerifyLatencyMs:  200,
		FilesChanged:     5,
		LinesAdded:       100,
		LinesRemoved:     20,
		Confidence:       0.9,
		DifficultyScore:  0.7,
	}

	m := AnomalyMetrics()
	tests := []struct {
		key  string
		want float64
	}{
		{"total_cost_usd", 1.23},
		{"planner_cost_usd", 0.50},
		{"worker_cost_usd", 0.73},
		{"total_latency_ms", 1000.0},
		{"planner_latency_ms", 400.0},
		{"worker_latency_ms", 600.0},
		{"verify_latency_ms", 200.0},
		{"files_changed", 5.0},
		{"lines_added", 100.0},
		{"lines_removed", 20.0},
		{"confidence", 0.9},
		{"difficulty_score", 0.7},
	}
	for _, tt := range tests {
		fn := m[tt.key]
		got := fn(obs)
		if got != tt.want {
			t.Errorf("AnomalyMetrics()[%q](obs) = %f, want %f", tt.key, got, tt.want)
		}
	}
}

func TestDetectFromObservations_UnknownMetric(t *testing.T) {
	_, err := DetectFromObservations(nil, "nonexistent_metric")
	if err == nil {
		t.Error("expected error for unknown metric name")
	}
}

func TestDetectFromObservations_ValidMetric(t *testing.T) {
	now := time.Now()
	obs := []session.LoopObservation{
		{TotalCostUSD: 1.0, Timestamp: now},
		{TotalCostUSD: 1.1, Timestamp: now.Add(1)},
		{TotalCostUSD: 1.05, Timestamp: now.Add(2)},
		{TotalCostUSD: 1.0, Timestamp: now.Add(3)},
		{TotalCostUSD: 100.0, Timestamp: now.Add(4)}, // spike
	}
	anomalies, err := DetectFromObservations(obs, "total_cost_usd")
	if err != nil {
		t.Fatalf("DetectFromObservations error: %v", err)
	}
	_ = anomalies // just verify no panic
}
