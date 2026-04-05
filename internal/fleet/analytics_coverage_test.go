package fleet

import (
	"testing"
	"time"
)

func TestPercentile_EmptySlice(t *testing.T) {
	t.Parallel()
	got := percentile(nil, 50)
	if got != 0 {
		t.Errorf("percentile(nil, 50) = %f, want 0", got)
	}
}

func TestPercentile_SingleElement(t *testing.T) {
	t.Parallel()
	got := percentile([]float64{42.0}, 50)
	if got != 42.0 {
		t.Errorf("percentile([42], 50) = %f, want 42", got)
	}
}

func TestPercentile_P99(t *testing.T) {
	t.Parallel()
	// 100 values from 1..100.
	vals := make([]float64, 100)
	for i := range vals {
		vals[i] = float64(i + 1)
	}
	got := percentile(vals, 99)
	if got < 99 {
		t.Errorf("percentile(1..100, 99) = %f, want >= 99", got)
	}
}

func TestPercentile_P0(t *testing.T) {
	t.Parallel()
	vals := []float64{10, 20, 30}
	got := percentile(vals, 0)
	if got != 10 {
		t.Errorf("percentile([10,20,30], 0) = %f, want 10", got)
	}
}

func TestRecordFailureAt(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(100, time.Hour)

	now := time.Now()
	fa.recordFailureAt(now, "worker-1", "timeout")
	fa.recordFailureAt(now, "worker-2", "oom")

	snap := fa.Snapshot(time.Hour)
	if snap.TotalFailures != 2 {
		t.Errorf("expected 2 failures, got %d", snap.TotalFailures)
	}
}

func TestRecordFailureAt_Overflow(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(5, time.Hour)

	now := time.Now()
	for range 10 {
		fa.recordFailureAt(now, "w", "err")
	}

	// Internal slice should be capped at maxSamples.
	fa.mu.RLock()
	defer fa.mu.RUnlock()
	if len(fa.failures) > 5 {
		t.Errorf("failures should be capped at 5, got %d", len(fa.failures))
	}
}

func TestCostForecast_InsufficientData(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(100, time.Hour)

	// Zero completions.
	if got := fa.CostForecast(time.Hour); got != 0 {
		t.Errorf("forecast with no data = %f, want 0", got)
	}

	// Single completion.
	fa.RecordCompletion("w1", "claude", 100*time.Millisecond, 0.50)
	if got := fa.CostForecast(time.Hour); got != 0 {
		t.Errorf("forecast with 1 sample = %f, want 0", got)
	}
}

func TestCostForecast_WithData(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(100, time.Hour)

	// Record completions with known spread.
	now := time.Now()
	fa.mu.Lock()
	fa.completions = append(fa.completions,
		completionSample{Timestamp: now.Add(-30 * time.Minute), WorkerID: "w1", Provider: "claude", CostUSD: 1.0},
		completionSample{Timestamp: now.Add(-15 * time.Minute), WorkerID: "w1", Provider: "claude", CostUSD: 1.0},
		completionSample{Timestamp: now, WorkerID: "w1", Provider: "claude", CostUSD: 1.0},
	)
	fa.mu.Unlock()

	forecast := fa.CostForecast(time.Hour)
	if forecast <= 0 {
		t.Errorf("forecast should be positive, got %f", forecast)
	}
}

func TestSnapshot_WithFailures(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(100, time.Hour)

	fa.RecordCompletion("w1", "claude", 100*time.Millisecond, 0.10)
	fa.RecordFailure("w2", "connection refused")

	snap := fa.Snapshot(time.Hour)
	if snap.TotalCompletions != 1 {
		t.Errorf("completions = %d, want 1", snap.TotalCompletions)
	}
	if snap.TotalFailures != 1 {
		t.Errorf("failures = %d, want 1", snap.TotalFailures)
	}
	if snap.FailureRate < 0.49 || snap.FailureRate > 0.51 {
		t.Errorf("failure rate = %f, want ~0.5", snap.FailureRate)
	}
}

func TestSnapshot_OutsideWindow(t *testing.T) {
	t.Parallel()
	fa := NewFleetAnalytics(100, time.Hour)

	// Add old completions.
	fa.mu.Lock()
	old := time.Now().Add(-2 * time.Hour)
	fa.completions = append(fa.completions,
		completionSample{Timestamp: old, WorkerID: "w1", Provider: "claude", CostUSD: 5.0},
	)
	fa.mu.Unlock()

	snap := fa.Snapshot(time.Hour)
	if snap.TotalCompletions != 0 {
		t.Errorf("old completions should be outside window, got %d", snap.TotalCompletions)
	}
}
