package eval

import (
	"math"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- helper ---

func makeRegObs(n int, setter func(*session.LoopObservation, int)) []session.LoopObservation {
	obs := make([]session.LoopObservation, n)
	base := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	for i := range obs {
		obs[i].Timestamp = base.Add(time.Duration(i) * time.Minute)
		if setter != nil {
			setter(&obs[i], i)
		}
	}
	return obs
}

// --- RegressionDetector constructor ---

func TestNewRegressionDetectorDefaults(t *testing.T) {
	d := NewRegressionDetector(nil)
	if d.Thresholds == nil {
		t.Fatal("expected non-nil thresholds with nil input")
	}
	for _, name := range []string{"completion_rate", "cost", "latency", "confidence"} {
		if _, ok := d.Thresholds[name]; !ok {
			t.Errorf("expected default threshold for %q", name)
		}
	}
}

func TestNewRegressionDetectorCustom(t *testing.T) {
	custom := map[string]MetricThreshold{
		"my_metric": {
			Extract:           func(o session.LoopObservation) float64 { return o.TotalCostUSD },
			Direction:         LowerIsBetter,
			RelativeThreshold: 0.10,
		},
	}
	d := NewRegressionDetector(custom)
	if len(d.Thresholds) != 1 {
		t.Fatalf("expected 1 threshold, got %d", len(d.Thresholds))
	}
	if _, ok := d.Thresholds["my_metric"]; !ok {
		t.Fatal("expected custom threshold 'my_metric'")
	}
}

// --- Empty inputs ---

func TestDetectRegressionsEmptyBaseline(t *testing.T) {
	d := NewRegressionDetector(nil)
	candidate := makeRegObs(10, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 1.0
	})
	report := d.DetectRegressions(nil, candidate)
	if !report.Passed {
		t.Error("expected Passed=true with empty baseline")
	}
	if len(report.Regressions) != 0 {
		t.Errorf("expected 0 regressions, got %d", len(report.Regressions))
	}
}

func TestDetectRegressionsEmptyCandidate(t *testing.T) {
	d := NewRegressionDetector(nil)
	baseline := makeRegObs(10, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 1.0
	})
	report := d.DetectRegressions(baseline, nil)
	if !report.Passed {
		t.Error("expected Passed=true with empty candidate")
	}
}

func TestDetectRegressionsBothEmpty(t *testing.T) {
	d := NewRegressionDetector(nil)
	report := d.DetectRegressions(nil, nil)
	if !report.Passed {
		t.Error("expected Passed=true with both empty")
	}
	if report.Summary != "No regressions detected." {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

// --- No regressions ---

func TestDetectRegressionsNoRegression(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})

	report := d.DetectRegressions(baseline, candidate)
	if !report.Passed {
		t.Errorf("expected Passed=true, got regressions=%d, failures=%d, perf=%d",
			len(report.Regressions), len(report.NewFailures), len(report.PerformanceRegressions))
	}
	if report.Summary != "No regressions detected." {
		t.Errorf("unexpected summary: %s", report.Summary)
	}
}

// --- Metric regressions: higher-is-better ---

func TestMetricRegressionCompletionRate(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = true // 100% pass rate
	})
	candidate := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = i < 80 // 80% pass rate, 20% drop
	})

	report := d.DetectRegressions(baseline, candidate)

	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "completion_rate" {
			found = true
			if r.Direction != "degraded" {
				t.Errorf("expected direction 'degraded', got %q", r.Direction)
			}
			if r.CandidateValue >= r.BaselineValue {
				t.Errorf("expected candidate < baseline, got %f >= %f", r.CandidateValue, r.BaselineValue)
			}
			if r.RelativeChange >= 0 {
				t.Errorf("expected negative relative change for drop, got %f", r.RelativeChange)
			}
		}
	}
	if !found {
		t.Error("expected completion_rate regression to be detected")
	}
}

// --- Metric regressions: lower-is-better ---

func TestMetricRegressionCostIncrease(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10
		o.VerifyPassed = true
		o.Confidence = 0.90
		o.TotalLatencyMs = 100
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.15 // 50% increase, exceeds 20% threshold
		o.VerifyPassed = true
		o.Confidence = 0.90
		o.TotalLatencyMs = 100
	})

	report := d.DetectRegressions(baseline, candidate)

	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "cost" {
			found = true
			if r.CandidateValue <= r.BaselineValue {
				t.Errorf("expected candidate > baseline for cost regression, got %f <= %f",
					r.CandidateValue, r.BaselineValue)
			}
			// Relative change should be positive (cost went up).
			if r.RelativeChange < 0.20 {
				t.Errorf("expected relative change >= 0.20, got %f", r.RelativeChange)
			}
		}
	}
	if !found {
		t.Error("expected cost regression to be detected")
	}
}

func TestMetricRegressionLatencyIncrease(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalLatencyMs = 1000
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.Confidence = 0.90
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalLatencyMs = 1500 // 50% increase, exceeds 25% threshold
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.Confidence = 0.90
	})

	report := d.DetectRegressions(baseline, candidate)

	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "latency" {
			found = true
		}
	}
	if !found {
		t.Error("expected latency regression to be detected")
	}
}

// --- Metric regressions: absolute threshold ---

func TestMetricRegressionAbsoluteThreshold(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.95
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.88 // -0.07, exceeds absolute threshold of 0.05
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
	})

	report := d.DetectRegressions(baseline, candidate)

	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "confidence" {
			found = true
			if math.Abs(r.AbsoluteChange-(-0.07)) > 0.001 {
				t.Errorf("expected absolute change ~-0.07, got %f", r.AbsoluteChange)
			}
		}
	}
	if !found {
		t.Error("expected confidence regression to be detected")
	}
}

func TestMetricRegressionBelowAbsoluteThreshold(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.95
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.92 // -0.03, below absolute threshold of 0.05
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
	})

	report := d.DetectRegressions(baseline, candidate)
	for _, r := range report.Regressions {
		if r.MetricName == "confidence" {
			t.Error("did not expect confidence regression below threshold")
		}
	}
}

// --- Metric improvement (not regression) ---

func TestMetricImprovement(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.20
		o.TotalLatencyMs = 500
		o.VerifyPassed = i < 40 // 80%
		o.Confidence = 0.85
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10 // cost decreased (improvement)
		o.TotalLatencyMs = 300 // latency decreased (improvement)
		o.VerifyPassed = i < 48 // 96% (improvement)
		o.Confidence = 0.95    // confidence increased (improvement)
	})

	report := d.DetectRegressions(baseline, candidate)
	if len(report.Regressions) != 0 {
		t.Errorf("expected 0 metric regressions for improvements, got %d", len(report.Regressions))
		for _, r := range report.Regressions {
			t.Errorf("  unexpected regression: %s (baseline=%f, candidate=%f)", r.MetricName, r.BaselineValue, r.CandidateValue)
		}
	}
}

// --- Below-threshold changes ---

func TestMetricBelowRelativeThreshold(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 1.00
		o.VerifyPassed = true
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 1.10 // 10% increase, below 20% cost threshold
		o.VerifyPassed = true
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})

	report := d.DetectRegressions(baseline, candidate)
	for _, r := range report.Regressions {
		if r.MetricName == "cost" {
			t.Error("did not expect cost regression below 20% threshold")
		}
	}
}

// --- New failures ---

func TestNewFailuresDetected(t *testing.T) {
	baseline := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", TaskType: "bugfix", VerifyPassed: true},
		{LoopID: "loop1", TaskTitle: "task-b", TaskType: "refactor", VerifyPassed: true},
		{LoopID: "loop1", TaskTitle: "task-c", TaskType: "feature", VerifyPassed: false},
	}
	candidate := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", TaskType: "bugfix", VerifyPassed: true},
		{LoopID: "loop1", TaskTitle: "task-b", TaskType: "refactor", VerifyPassed: false, Error: "compile error"},
		{LoopID: "loop1", TaskTitle: "task-c", TaskType: "feature", VerifyPassed: false}, // was already failing
	}

	d := NewRegressionDetector(nil)
	report := d.DetectRegressions(baseline, candidate)

	if len(report.NewFailures) != 1 {
		t.Fatalf("expected 1 new failure, got %d", len(report.NewFailures))
	}
	nf := report.NewFailures[0]
	if nf.TaskTitle != "task-b" {
		t.Errorf("expected failure for task-b, got %s", nf.TaskTitle)
	}
	if nf.Error != "compile error" {
		t.Errorf("expected error 'compile error', got %q", nf.Error)
	}
	if nf.TaskType != "refactor" {
		t.Errorf("expected task type 'refactor', got %q", nf.TaskType)
	}
}

func TestNewFailuresNoDuplicates(t *testing.T) {
	baseline := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: true},
	}
	// Same task fails twice in candidate.
	candidate := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: false},
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: false},
	}

	d := NewRegressionDetector(nil)
	report := d.DetectRegressions(baseline, candidate)

	if len(report.NewFailures) != 1 {
		t.Errorf("expected 1 deduped failure, got %d", len(report.NewFailures))
	}
}

func TestNoNewFailuresWhenAlreadyFailing(t *testing.T) {
	baseline := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: false},
	}
	candidate := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: false},
	}

	d := NewRegressionDetector(nil)
	report := d.DetectRegressions(baseline, candidate)

	if len(report.NewFailures) != 0 {
		t.Errorf("expected 0 new failures for already-failing task, got %d", len(report.NewFailures))
	}
}

func TestNoNewFailuresAllPassing(t *testing.T) {
	baseline := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: true},
	}
	candidate := []session.LoopObservation{
		{LoopID: "loop1", TaskTitle: "task-a", VerifyPassed: true},
	}

	d := NewRegressionDetector(nil)
	report := d.DetectRegressions(baseline, candidate)

	if len(report.NewFailures) != 0 {
		t.Errorf("expected 0 new failures when all pass, got %d", len(report.NewFailures))
	}
}

// --- Performance regressions (statistical) ---

func TestPerformanceRegressionDetected(t *testing.T) {
	// Use a custom single-metric detector for isolation.
	thresholds := map[string]MetricThreshold{
		"cost": {
			Extract:           func(o session.LoopObservation) float64 { return o.TotalCostUSD },
			Direction:         LowerIsBetter,
			RelativeThreshold: 0.50, // high threshold so metric regression doesn't fire
		},
	}
	d := NewRegressionDetector(thresholds)

	// Baseline: low cost, tight distribution.
	baseline := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10
	})
	// Candidate: noticeably higher cost.
	candidate := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.13
	})

	report := d.DetectRegressions(baseline, candidate)

	// With constant values, std=0, so z-test returns p=1 (no evidence).
	// We need variance for the z-test to be meaningful.
	// Retry with some variance.
	baseline = makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10 + float64(i%5)*0.01
	})
	candidate = makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.20 + float64(i%5)*0.01 // shifted up by 0.10
	})

	report = d.DetectRegressions(baseline, candidate)

	found := false
	for _, pr := range report.PerformanceRegressions {
		if pr.MetricName == "cost" {
			found = true
			if pr.Direction != "degraded" {
				t.Errorf("expected direction 'degraded', got %q", pr.Direction)
			}
			if pr.PValue >= 0.05 {
				t.Errorf("expected p-value < 0.05, got %f", pr.PValue)
			}
			if pr.CandidateMean <= pr.BaselineMean {
				t.Errorf("expected candidate mean > baseline mean, got %f <= %f",
					pr.CandidateMean, pr.BaselineMean)
			}
		}
	}
	if !found {
		t.Error("expected performance regression for cost")
	}
}

func TestPerformanceRegressionNotFiredOnImprovement(t *testing.T) {
	thresholds := map[string]MetricThreshold{
		"cost": {
			Extract:           func(o session.LoopObservation) float64 { return o.TotalCostUSD },
			Direction:         LowerIsBetter,
			RelativeThreshold: 0.50,
		},
	}
	d := NewRegressionDetector(thresholds)

	baseline := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.20 + float64(i%5)*0.01
	})
	candidate := makeRegObs(100, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10 + float64(i%5)*0.01 // cost went down
	})

	report := d.DetectRegressions(baseline, candidate)
	for _, pr := range report.PerformanceRegressions {
		if pr.MetricName == "cost" {
			t.Error("should not flag cost improvement as performance regression")
		}
	}
}

func TestPerformanceRegressionInsufficientData(t *testing.T) {
	d := NewRegressionDetector(nil)

	// Only 1 observation each; z-test needs >= 2.
	baseline := makeRegObs(1, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10
	})
	candidate := makeRegObs(1, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.50
	})

	report := d.DetectRegressions(baseline, candidate)
	if len(report.PerformanceRegressions) != 0 {
		t.Errorf("expected no perf regressions with 1 obs each, got %d", len(report.PerformanceRegressions))
	}
}

// --- Passed flag ---

func TestReportPassedFlag(t *testing.T) {
	d := NewRegressionDetector(nil)

	// Scenario with regression.
	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})
	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.VerifyPassed = i < 20 // 40% pass rate, big drop
		o.TotalCostUSD = 0.10
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})

	report := d.DetectRegressions(baseline, candidate)
	if report.Passed {
		t.Error("expected Passed=false when regressions exist")
	}
}

// --- Summary ---

func TestSummaryContent(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := []session.LoopObservation{
		{LoopID: "l1", TaskTitle: "t1", VerifyPassed: true, TotalCostUSD: 0.10, TotalLatencyMs: 100, Confidence: 0.90},
	}
	candidate := []session.LoopObservation{
		{LoopID: "l1", TaskTitle: "t1", VerifyPassed: false, TotalCostUSD: 5.0, TotalLatencyMs: 100, Confidence: 0.90},
	}

	report := d.DetectRegressions(baseline, candidate)

	if report.Passed {
		t.Fatal("expected Passed=false")
	}
	if report.Summary == "" {
		t.Fatal("expected non-empty summary")
	}
	// Should mention "new failure" since task-t1 went from pass to fail.
	if len(report.NewFailures) == 0 {
		t.Error("expected at least one new failure")
	}
}

// --- Timestamp ---

func TestReportTimestamp(t *testing.T) {
	d := NewRegressionDetector(nil)
	before := time.Now()
	report := d.DetectRegressions(nil, nil)
	after := time.Now()

	if report.Timestamp.Before(before) || report.Timestamp.After(after) {
		t.Errorf("report timestamp %v not between %v and %v", report.Timestamp, before, after)
	}
}

// --- normalCDFComplement ---

func TestNormalCDFComplement(t *testing.T) {
	tests := []struct {
		z        float64
		expected float64
		tol      float64
	}{
		{0, 0.5, 0.01},
		{1.96, 0.025, 0.005},
		{2.576, 0.005, 0.002},
		{3.0, 0.00135, 0.001},
	}
	for _, tt := range tests {
		got := normalCDFComplement(tt.z)
		if math.Abs(got-tt.expected) > tt.tol {
			t.Errorf("normalCDFComplement(%f) = %f, want ~%f (tol %f)", tt.z, got, tt.expected, tt.tol)
		}
	}
}

func TestNormalCDFComplementNegative(t *testing.T) {
	// P(Z > -1.96) should be close to 0.975.
	got := normalCDFComplement(-1.96)
	if math.Abs(got-0.975) > 0.01 {
		t.Errorf("normalCDFComplement(-1.96) = %f, want ~0.975", got)
	}
}

// --- twoSampleZTest ---

func TestTwoSampleZTestIdentical(t *testing.T) {
	z, p := twoSampleZTest(5.0, 1.0, 100, 5.0, 1.0, 100)
	if z != 0 {
		t.Errorf("expected z=0 for identical distributions, got %f", z)
	}
	if p < 0.99 {
		t.Errorf("expected p~1.0 for identical distributions, got %f", p)
	}
}

func TestTwoSampleZTestDifferent(t *testing.T) {
	// Large difference in means with small variance should yield significant p.
	z, p := twoSampleZTest(1.0, 0.5, 100, 3.0, 0.5, 100)
	if p >= 0.05 {
		t.Errorf("expected p < 0.05 for large difference, got %f (z=%f)", p, z)
	}
}

func TestTwoSampleZTestZeroVariance(t *testing.T) {
	z, p := twoSampleZTest(5.0, 0, 10, 5.0, 0, 10)
	if z != 0 || p != 1 {
		t.Errorf("expected z=0, p=1 for zero variance, got z=%f, p=%f", z, p)
	}
}

func TestTwoSampleZTestEmptySamples(t *testing.T) {
	z, p := twoSampleZTest(5.0, 1.0, 0, 5.0, 1.0, 100)
	if z != 0 || p != 1 {
		t.Errorf("expected z=0, p=1 for empty sample, got z=%f, p=%f", z, p)
	}
}

// --- isRegression ---

func TestIsRegressionHigherIsBetter(t *testing.T) {
	thresh := MetricThreshold{
		Direction:         HigherIsBetter,
		RelativeThreshold: 0.10,
	}

	// 15% decrease (regression).
	if !isRegression(-0.15, -0.15, thresh) {
		t.Error("expected regression for 15% decrease with 10% threshold (higher-is-better)")
	}

	// 5% decrease (below threshold).
	if isRegression(-0.05, -0.05, thresh) {
		t.Error("did not expect regression for 5% decrease with 10% threshold")
	}

	// 15% increase (improvement).
	if isRegression(0.15, 0.15, thresh) {
		t.Error("did not expect regression for increase when higher-is-better")
	}
}

func TestIsRegressionLowerIsBetter(t *testing.T) {
	thresh := MetricThreshold{
		Direction:         LowerIsBetter,
		RelativeThreshold: 0.20,
	}

	// 25% increase (regression).
	if !isRegression(0.25, 0.25, thresh) {
		t.Error("expected regression for 25% increase with 20% threshold (lower-is-better)")
	}

	// 10% increase (below threshold).
	if isRegression(0.10, 0.10, thresh) {
		t.Error("did not expect regression for 10% increase with 20% threshold")
	}

	// 25% decrease (improvement).
	if isRegression(-0.25, -0.25, thresh) {
		t.Error("did not expect regression for decrease when lower-is-better")
	}
}

// --- DefaultThresholds ---

func TestDefaultThresholds(t *testing.T) {
	thresholds := DefaultThresholds()

	tests := []struct {
		name      string
		direction Direction
	}{
		{"completion_rate", HigherIsBetter},
		{"cost", LowerIsBetter},
		{"latency", LowerIsBetter},
		{"confidence", HigherIsBetter},
	}

	for _, tt := range tests {
		thresh, ok := thresholds[tt.name]
		if !ok {
			t.Errorf("missing default threshold for %q", tt.name)
			continue
		}
		if thresh.Direction != tt.direction {
			t.Errorf("%s: expected direction %d, got %d", tt.name, tt.direction, thresh.Direction)
		}
		if thresh.Extract == nil {
			t.Errorf("%s: Extract function is nil", tt.name)
		}
		// At least one threshold must be set.
		if thresh.AbsoluteThreshold == 0 && thresh.RelativeThreshold == 0 {
			t.Errorf("%s: no threshold configured", tt.name)
		}
	}
}

// --- Combined regression scenario ---

func TestMultipleRegressionTypes(t *testing.T) {
	d := NewRegressionDetector(nil)

	baseline := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.LoopID = "loop1"
		o.TaskTitle = "task-x"
		o.TaskType = "bugfix"
		o.VerifyPassed = true
		o.TotalCostUSD = 0.10 + float64(i%3)*0.01
		o.TotalLatencyMs = 100
		o.Confidence = 0.90
	})

	candidate := makeRegObs(50, func(o *session.LoopObservation, i int) {
		o.LoopID = "loop1"
		o.TaskTitle = "task-x"
		o.TaskType = "bugfix"
		o.VerifyPassed = i < 30 // 60% pass rate (was 100%)
		o.TotalCostUSD = 0.50 + float64(i%3)*0.01 // big cost increase
		o.TotalLatencyMs = 100
		o.Confidence = 0.80 // -0.10 confidence drop
	})

	report := d.DetectRegressions(baseline, candidate)

	if report.Passed {
		t.Fatal("expected Passed=false with multiple regressions")
	}

	// Should detect metric regressions.
	if len(report.Regressions) == 0 {
		t.Error("expected metric regressions")
	}

	// Should detect new failures (task-x went from pass to fail for some).
	if len(report.NewFailures) == 0 {
		t.Error("expected new failures")
	}

	// Summary should be non-empty and mention regressions.
	if report.Summary == "" || report.Summary == "No regressions detected." {
		t.Errorf("unexpected summary for failing report: %q", report.Summary)
	}
}

// --- Custom threshold with both absolute and relative ---

func TestCustomThresholdBothAbsoluteAndRelative(t *testing.T) {
	thresholds := map[string]MetricThreshold{
		"score": {
			Extract: func(o session.LoopObservation) float64 {
				return o.Confidence
			},
			Direction:         HigherIsBetter,
			AbsoluteThreshold: 0.10,
			RelativeThreshold: 0.05,
		},
	}
	d := NewRegressionDetector(thresholds)

	// Relative threshold triggers (6% drop) but absolute doesn't (0.06 < 0.10).
	baseline := makeRegObs(20, func(o *session.LoopObservation, i int) {
		o.Confidence = 1.0
	})
	candidate := makeRegObs(20, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.94 // 6% relative drop, 0.06 absolute
	})

	report := d.DetectRegressions(baseline, candidate)
	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "score" {
			found = true
		}
	}
	if !found {
		t.Error("expected regression when relative threshold is exceeded even if absolute is not")
	}
}

func TestCustomThresholdAbsoluteTriggersAlone(t *testing.T) {
	thresholds := map[string]MetricThreshold{
		"score": {
			Extract: func(o session.LoopObservation) float64 {
				return o.Confidence
			},
			Direction:         HigherIsBetter,
			AbsoluteThreshold: 0.05,
			RelativeThreshold: 0.50, // very high relative threshold
		},
	}
	d := NewRegressionDetector(thresholds)

	baseline := makeRegObs(20, func(o *session.LoopObservation, i int) {
		o.Confidence = 1.0
	})
	candidate := makeRegObs(20, func(o *session.LoopObservation, i int) {
		o.Confidence = 0.90 // 0.10 absolute, 10% relative
	})

	report := d.DetectRegressions(baseline, candidate)
	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "score" {
			found = true
		}
	}
	if !found {
		t.Error("expected regression when absolute threshold is exceeded even if relative is not")
	}
}

// --- Zero baseline value edge case ---

func TestZeroBaselineValue(t *testing.T) {
	thresholds := map[string]MetricThreshold{
		"cost": {
			Extract:           func(o session.LoopObservation) float64 { return o.TotalCostUSD },
			Direction:         LowerIsBetter,
			AbsoluteThreshold: 0.05,
		},
	}
	d := NewRegressionDetector(thresholds)

	baseline := makeRegObs(10, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.0
	})
	candidate := makeRegObs(10, func(o *session.LoopObservation, i int) {
		o.TotalCostUSD = 0.10
	})

	report := d.DetectRegressions(baseline, candidate)
	found := false
	for _, r := range report.Regressions {
		if r.MetricName == "cost" {
			found = true
			// With baseline=0, relative change would be Inf or NaN;
			// absolute threshold should still catch it.
			if math.Abs(r.AbsoluteChange-0.10) > 1e-9 {
				t.Errorf("expected absolute change ~0.10, got %f", r.AbsoluteChange)
			}
		}
	}
	if !found {
		t.Error("expected cost regression from zero baseline with absolute threshold")
	}
}
