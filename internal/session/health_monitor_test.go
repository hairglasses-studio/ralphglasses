package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestHealthMonitor_NilSafe(t *testing.T) {
	var hm *HealthMonitor
	signals := hm.Evaluate("/nonexistent")
	if signals != nil {
		t.Fatalf("expected nil from nil monitor, got %v", signals)
	}
}

func TestHealthMonitor_EvaluateFunc(t *testing.T) {
	hm := &HealthMonitor{
		EvaluateFunc: func(repoPath string) []HealthSignal {
			return []HealthSignal{{Metric: "test", Value: 1.0}}
		},
	}
	signals := hm.Evaluate("/tmp")
	if len(signals) != 1 || signals[0].Metric != "test" {
		t.Fatalf("unexpected signals: %v", signals)
	}
}

func TestHealthMonitor_DefaultThresholds(t *testing.T) {
	dt := DefaultHealthThresholds()
	if dt.MinCompletionRate != 0.70 {
		t.Fatalf("unexpected MinCompletionRate: %f", dt.MinCompletionRate)
	}
	if dt.MaxIdleTime != time.Hour {
		t.Fatalf("unexpected MaxIdleTime: %v", dt.MaxIdleTime)
	}
}

func TestHealthMonitor_EmptyRepo(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755)

	hm := NewHealthMonitor(DefaultHealthThresholds())
	signals := hm.Evaluate(dir)

	// With no observations or cycles, should detect idle.
	found := false
	for _, s := range signals {
		if s.Metric == "idle_time" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected idle_time signal for empty repo")
	}
}

func TestHealthMonitor_LowCompletionRate(t *testing.T) {
	dir := t.TempDir()
	obsPath := filepath.Join(dir, ".ralph", "cost_observations.json")
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755)

	// Write 10 observations, only 3 passed.
	var obs []LoopObservation
	for i := 0; i < 10; i++ {
		obs = append(obs, LoopObservation{
			Timestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			VerifyPassed: i < 3,
			TotalCostUSD: 0.01,
		})
	}
	writeObservations(t, obsPath, obs)

	hm := NewHealthMonitor(DefaultHealthThresholds())
	signals := hm.Evaluate(dir)

	found := false
	for _, s := range signals {
		if s.Metric == "completion_rate" {
			found = true
			if s.Value >= 0.70 {
				t.Fatalf("expected rate < 0.70, got %f", s.Value)
			}
		}
	}
	if !found {
		t.Fatal("expected completion_rate signal")
	}
}

func TestHealthMonitor_HighCostRate(t *testing.T) {
	dir := t.TempDir()
	obsPath := filepath.Join(dir, ".ralph", "cost_observations.json")
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755)

	// 100 observations, all recent, $1 each = very high rate.
	var obs []LoopObservation
	for i := 0; i < 100; i++ {
		obs = append(obs, LoopObservation{
			Timestamp:    time.Now().Add(-time.Duration(i) * time.Second),
			VerifyPassed: true,
			TotalCostUSD: 1.0,
		})
	}
	writeObservations(t, obsPath, obs)

	hm := NewHealthMonitor(DefaultHealthThresholds())
	signals := hm.Evaluate(dir)

	found := false
	for _, s := range signals {
		if s.Metric == "cost_rate_per_hour" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected cost_rate_per_hour signal for expensive runs")
	}
}

func TestHealthMonitor_NoSignalsWhenHealthy(t *testing.T) {
	dir := t.TempDir()
	obsPath := filepath.Join(dir, ".ralph", "cost_observations.json")
	os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755)

	// All passing, cheap, recent.
	var obs []LoopObservation
	for i := 0; i < 10; i++ {
		obs = append(obs, LoopObservation{
			Timestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
			VerifyPassed: true,
			TotalCostUSD: 0.001,
		})
	}
	writeObservations(t, obsPath, obs)

	hm := NewHealthMonitor(DefaultHealthThresholds())
	signals := hm.Evaluate(dir)

	// Should have no completion_rate or cost_rate signals.
	for _, s := range signals {
		if s.Metric == "completion_rate" || s.Metric == "cost_rate_per_hour" {
			t.Fatalf("unexpected signal for healthy repo: %+v", s)
		}
	}
}

func writeObservations(t *testing.T, path string, obs []LoopObservation) {
	t.Helper()
	for _, o := range obs {
		if err := WriteObservation(path, o); err != nil {
			t.Fatal(err)
		}
	}
}
