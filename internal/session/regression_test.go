package session

import (
	"sync"
	"testing"
)

func TestRegressionDetector_NoBaseline(t *testing.T) {
	rd := NewRegressionDetector(0.05)
	r := rd.Check("coverage", 80.0)
	if r != nil {
		t.Fatalf("expected nil regression when no baseline set, got %+v", r)
	}
}

func TestRegressionDetector_NoRegression(t *testing.T) {
	rd := NewRegressionDetector(0.05)
	rd.SetBaseline("coverage", 80.0)

	// Value equal to baseline — no regression.
	r := rd.Check("coverage", 80.0)
	if r != nil {
		t.Fatalf("expected nil for value equal to baseline, got %+v", r)
	}

	// Value above baseline — no regression.
	r = rd.Check("coverage", 85.0)
	if r != nil {
		t.Fatalf("expected nil for value above baseline, got %+v", r)
	}

	// Small drop within threshold (2% < 5%).
	r = rd.Check("coverage", 78.5)
	if r != nil {
		t.Fatalf("expected nil for drop within threshold, got %+v", r)
	}
}

func TestRegressionDetector_DetectsRegression(t *testing.T) {
	rd := NewRegressionDetector(0.05) // 5% threshold
	rd.SetBaseline("coverage", 100.0)

	// 10% drop should trigger.
	r := rd.Check("coverage", 90.0)
	if r == nil {
		t.Fatal("expected regression for 10% drop")
	}
	if r.Metric != "coverage" {
		t.Errorf("metric = %q, want %q", r.Metric, "coverage")
	}
	if r.Baseline != 100.0 {
		t.Errorf("baseline = %f, want 100.0", r.Baseline)
	}
	if r.Current != 90.0 {
		t.Errorf("current = %f, want 90.0", r.Current)
	}
	// Drop should be 0.10 (10%).
	if r.DropPercent < 0.099 || r.DropPercent > 0.101 {
		t.Errorf("drop = %f, want ~0.10", r.DropPercent)
	}
	if r.DetectedAt.IsZero() {
		t.Error("DetectedAt should not be zero")
	}
}

func TestRegressionDetector_ExactThreshold(t *testing.T) {
	rd := NewRegressionDetector(0.05) // 5% threshold
	rd.SetBaseline("latency", 200.0)

	// Exactly 5% drop (200 → 190). drop == threshold, should NOT trigger (> not >=).
	r := rd.Check("latency", 190.0)
	if r != nil {
		t.Fatalf("expected nil at exact threshold boundary, got %+v", r)
	}

	// Just past threshold.
	r = rd.Check("latency", 189.0)
	if r == nil {
		t.Fatal("expected regression just past threshold")
	}
}

func TestRegressionDetector_CheckAll(t *testing.T) {
	rd := NewRegressionDetector(0.10) // 10% threshold
	rd.SetBaseline("coverage", 80.0)
	rd.SetBaseline("speed", 100.0)
	rd.SetBaseline("memory", 500.0)

	results := rd.CheckAll(map[string]float64{
		"coverage": 78.0,  // 2.5% drop — ok
		"speed":    85.0,  // 15% drop — regression
		"memory":   440.0, // 12% drop — regression
		"unknown":  42.0,  // no baseline — ignored
	})

	if len(results) != 2 {
		t.Fatalf("expected 2 regressions, got %d: %+v", len(results), results)
	}

	found := map[string]bool{}
	for _, r := range results {
		found[r.Metric] = true
	}
	if !found["speed"] {
		t.Error("expected regression for speed")
	}
	if !found["memory"] {
		t.Error("expected regression for memory")
	}
}

func TestRegressionDetector_ConcurrentAccess(t *testing.T) {
	rd := NewRegressionDetector(0.05)
	var wg sync.WaitGroup

	// Concurrent writers.
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rd.SetBaseline("metric", float64(100+n))
		}(i)
	}

	// Concurrent readers.
	for i := range 50 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			rd.Check("metric", float64(n))
		}(i)
	}

	// Concurrent CheckAll.
	for range 20 {
		wg.Go(func() {
			rd.CheckAll(map[string]float64{"metric": 50.0})
		})
	}

	wg.Wait()
}
