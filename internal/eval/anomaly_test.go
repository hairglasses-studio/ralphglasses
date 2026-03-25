package eval

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func makeTimestamps(n int) []time.Time {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	ts := make([]time.Time, n)
	for i := range ts {
		ts[i] = t0.Add(time.Duration(i) * time.Minute)
	}
	return ts
}

func TestAnomalyConstant(t *testing.T) {
	det := NewSlidingWindowAnomaly(5, 2.5)
	values := make([]float64, 30)
	for i := range values {
		values[i] = 10.0
	}
	anomalies := det.Detect(values, makeTimestamps(30))
	if len(anomalies) != 0 {
		t.Fatalf("expected 0 anomalies for constant data, got %d", len(anomalies))
	}
}

func TestAnomalySingleSpike(t *testing.T) {
	det := NewSlidingWindowAnomaly(5, 2.5)
	values := make([]float64, 30)
	for i := range values {
		values[i] = 10.0
	}
	values[10] = 100.0 // big spike
	anomalies := det.Detect(values, makeTimestamps(30))
	if len(anomalies) != 1 {
		t.Fatalf("expected 1 anomaly, got %d", len(anomalies))
	}
	if anomalies[0].Index != 10 {
		t.Fatalf("expected anomaly at index 10, got %d", anomalies[0].Index)
	}
	if anomalies[0].Direction != "high" {
		t.Fatalf("expected direction high, got %s", anomalies[0].Direction)
	}
}

func TestAnomalyMultipleSpikes(t *testing.T) {
	det := NewSlidingWindowAnomaly(5, 2.5)
	values := make([]float64, 30)
	for i := range values {
		values[i] = 10.0
	}
	values[10] = 100.0
	values[20] = 100.0
	anomalies := det.Detect(values, makeTimestamps(30))
	if len(anomalies) < 2 {
		t.Fatalf("expected at least 2 anomalies, got %d", len(anomalies))
	}
}

func TestAnomalyTooFewPoints(t *testing.T) {
	det := NewSlidingWindowAnomaly(20, 2.5)
	values := make([]float64, 10) // fewer than window size
	anomalies := det.Detect(values, makeTimestamps(10))
	if anomalies != nil {
		t.Fatalf("expected nil for too few points, got %d anomalies", len(anomalies))
	}
}

func TestAnomalyZeroStddev(t *testing.T) {
	det := NewSlidingWindowAnomaly(5, 2.5)
	values := make([]float64, 10)
	for i := range values {
		values[i] = 5.0
	}
	values[7] = 6.0 // different value after constant window
	anomalies := det.Detect(values, makeTimestamps(10))
	if len(anomalies) == 0 {
		t.Fatal("expected anomaly when stddev is zero and value differs")
	}
	found := false
	for _, a := range anomalies {
		if a.Index == 7 {
			found = true
			if !math.IsInf(a.ZScore, 1) {
				t.Fatalf("expected +Inf z-score for zero-stddev anomaly, got %f", a.ZScore)
			}
		}
	}
	if !found {
		t.Fatal("expected anomaly at index 7")
	}
}

func TestDetectFromObservations(t *testing.T) {
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	observations := make([]session.LoopObservation, 30)
	for i := range observations {
		observations[i] = session.LoopObservation{
			Timestamp:    t0.Add(time.Duration(i) * time.Minute),
			TotalCostUSD: 0.10,
		}
	}
	// Insert a cost spike
	observations[25].TotalCostUSD = 5.0

	anomalies, err := DetectFromObservations(observations, "total_cost_usd")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(anomalies) == 0 {
		t.Fatal("expected at least one anomaly for cost spike")
	}

	// Verify the spike was detected
	found := false
	for _, a := range anomalies {
		if a.Index == 25 {
			found = true
			if a.Direction != "high" {
				t.Fatalf("expected high direction, got %s", a.Direction)
			}
		}
	}
	if !found {
		t.Fatal("expected anomaly at index 25")
	}

	// Unknown metric should error
	_, err = DetectFromObservations(observations, "nonexistent")
	if err == nil {
		t.Fatal("expected error for unknown metric")
	}
}

func TestAnomalyMismatchedLengths(t *testing.T) {
	det := NewSlidingWindowAnomaly(5, 2.5)

	values := make([]float64, 20)
	timestamps := makeTimestamps(10) // different length

	result := det.Detect(values, timestamps)
	if result != nil {
		t.Fatalf("expected nil for mismatched lengths, got %d anomalies", len(result))
	}

	// Also test the reverse: fewer values than timestamps.
	result = det.Detect(values[:10], makeTimestamps(20))
	if result != nil {
		t.Fatalf("expected nil for mismatched lengths (reverse), got %d anomalies", len(result))
	}
}

func TestDetectFromObservationsUnknownMetric(t *testing.T) {
	observations := make([]session.LoopObservation, 5)

	_, err := DetectFromObservations(observations, "bogus_metric")
	if err == nil {
		t.Fatal("expected error for unknown metric name")
	}

	errMsg := err.Error()
	// Error message should mention the invalid metric name.
	if !strings.Contains(errMsg, "bogus_metric") {
		t.Errorf("error should mention the invalid metric name, got: %s", errMsg)
	}

	// Error message should list at least some valid metric names.
	for _, valid := range []string{"total_cost_usd", "confidence", "files_changed"} {
		if !strings.Contains(errMsg, valid) {
			t.Errorf("error should list valid metric %q, got: %s", valid, errMsg)
		}
	}
}
