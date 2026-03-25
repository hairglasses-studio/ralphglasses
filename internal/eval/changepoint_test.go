package eval

import (
	"math"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// makeObservations creates n observations with metric values set via the provided function.
// The setter receives each observation pointer and index for customization.
func makeObservations(n int, setter func(*session.LoopObservation, int)) []session.LoopObservation {
	obs := make([]session.LoopObservation, n)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range obs {
		obs[i].Timestamp = base.Add(time.Duration(i) * time.Minute)
		if setter != nil {
			setter(&obs[i], i)
		}
	}
	return obs
}

func TestDetectChangepointsTooFew(t *testing.T) {
	obs := makeObservations(5, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = float64(i)
	})
	metric := func(o session.LoopObservation) float64 { return o.TotalCostUSD }
	result := DetectChangepoints(obs, metric, "cost")
	if result != nil {
		t.Errorf("expected nil for fewer than 10 observations, got %d changepoints", len(result))
	}
}

func TestDetectChangepointsConstant(t *testing.T) {
	obs := makeObservations(20, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 5.0
	})
	metric := func(o session.LoopObservation) float64 { return o.TotalCostUSD }
	result := DetectChangepoints(obs, metric, "cost")
	if result != nil {
		t.Errorf("expected nil for constant values, got %d changepoints", len(result))
	}
}

func TestDetectChangepointsStepFunction(t *testing.T) {
	obs := makeObservations(100, func(o *session.LoopObservation, i int) {
		if i < 50 {
			o.TotalCostUSD = 1.0
		} else {
			o.TotalCostUSD = 5.0
		}
	})
	metric := func(o session.LoopObservation) float64 { return o.TotalCostUSD }
	result := DetectChangepoints(obs, metric, "cost")

	if len(result) == 0 {
		t.Fatal("expected at least 1 changepoint for step function, got 0")
	}

	// The most significant changepoint should be near index 50.
	cp := result[0]
	if cp.Index < 40 || cp.Index > 70 {
		t.Errorf("expected changepoint near index 50, got index %d", cp.Index)
	}
	if cp.Direction != "increase" {
		t.Errorf("expected direction 'increase', got %q", cp.Direction)
	}
	if cp.Significance <= 0 || cp.Significance > 1 {
		t.Errorf("expected significance in (0, 1], got %f", cp.Significance)
	}
	if cp.AfterMean <= cp.BeforeMean {
		t.Errorf("expected AfterMean > BeforeMean, got %f <= %f", cp.AfterMean, cp.BeforeMean)
	}
	if cp.MetricName != "cost" {
		t.Errorf("expected metric name 'cost', got %q", cp.MetricName)
	}
}

func TestDetectChangepointsGradual(t *testing.T) {
	// Linearly increasing values — CUSUM is designed for abrupt changes,
	// so it may or may not detect a changepoint. This test verifies no panic
	// and that any detected changepoints are well-formed.
	obs := makeObservations(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = float64(i) * 0.1
	})
	metric := func(o session.LoopObservation) float64 { return o.TotalCostUSD }
	result := DetectChangepoints(obs, metric, "cost")

	// Just verify well-formedness if any are returned.
	for _, cp := range result {
		if cp.Significance < 0 || cp.Significance > 1 {
			t.Errorf("significance out of range: %f", cp.Significance)
		}
		if cp.Direction != "increase" && cp.Direction != "decrease" {
			t.Errorf("unexpected direction: %q", cp.Direction)
		}
	}
}

func TestStandardMetrics(t *testing.T) {
	obs := session.LoopObservation{
		VerifyPassed:    true,
		TotalCostUSD:    1.23,
		TotalLatencyMs:  4567,
		Confidence:      0.95,
		DifficultyScore: 0.7,
	}

	metrics := StandardMetrics()

	tests := []struct {
		name     string
		expected float64
	}{
		{"completion_rate", 1.0},
		{"cost", 1.23},
		{"latency", 4567.0},
		{"confidence", 0.95},
		{"difficulty", 0.7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, ok := metrics[tt.name]
			if !ok {
				t.Fatalf("metric %q not found in StandardMetrics()", tt.name)
			}
			got := fn(obs)
			if math.Abs(got-tt.expected) > 1e-9 {
				t.Errorf("expected %f, got %f", tt.expected, got)
			}
		})
	}

	// Verify completion_rate returns 0 when VerifyPassed is false.
	obsFailed := session.LoopObservation{VerifyPassed: false}
	if v := metrics["completion_rate"](obsFailed); v != 0.0 {
		t.Errorf("expected completion_rate 0.0 for failed, got %f", v)
	}
}

func TestDetectAllChangepoints(t *testing.T) {
	obs := makeObservations(100, func(o *session.LoopObservation, i int) {
		if i < 50 {
			o.TotalCostUSD = 1.0
			o.TotalLatencyMs = 100
		} else {
			o.TotalCostUSD = 5.0
			o.TotalLatencyMs = 500
		}
		o.Confidence = 0.5
		o.DifficultyScore = 0.5
		o.VerifyPassed = true
	})

	result := DetectAllChangepoints(obs)

	// Cost and latency should have changepoints due to step function.
	if _, ok := result["cost"]; !ok {
		t.Error("expected changepoints for 'cost' metric")
	}
	if _, ok := result["latency"]; !ok {
		t.Error("expected changepoints for 'latency' metric")
	}

	// Constant metrics should have no changepoints (nil, so not in map).
	if cps, ok := result["confidence"]; ok && len(cps) > 0 {
		t.Errorf("expected no changepoints for constant 'confidence', got %d", len(cps))
	}
}

func TestDetectChangepointsDecrease(t *testing.T) {
	obs := makeObservations(100, func(o *session.LoopObservation, i int) {
		if i < 50 {
			o.TotalCostUSD = 5.0
		} else {
			o.TotalCostUSD = 1.0
		}
	})
	metric := func(o session.LoopObservation) float64 { return o.TotalCostUSD }
	result := DetectChangepoints(obs, metric, "cost")

	if len(result) == 0 {
		t.Fatal("expected at least 1 changepoint for decreasing step, got 0")
	}

	cp := result[0]
	if cp.Direction != "decrease" {
		t.Errorf("expected direction 'decrease', got %q", cp.Direction)
	}
	if cp.AfterMean >= cp.BeforeMean {
		t.Errorf("expected AfterMean < BeforeMean, got %f >= %f", cp.AfterMean, cp.BeforeMean)
	}
}
