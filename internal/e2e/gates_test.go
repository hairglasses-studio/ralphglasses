package e2e

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func makeObs(status string, cost float64, latencyMs int64, verifyPassed bool, errMsg string) session.LoopObservation {
	return session.LoopObservation{
		Timestamp:      time.Now(),
		Status:         status,
		TotalCostUSD:   cost,
		TotalLatencyMs: latencyMs,
		VerifyPassed:   verifyPassed,
		Error:          errMsg,
		Mode:           "mock",
	}
}

func TestEvaluateGates_Pass(t *testing.T) {
	t.Parallel()

	baseline := &LoopBaseline{
		Aggregate: &BaselineStats{
			CostP95:    1.0,
			LatencyP95: 5000,
		},
		Rates: &BaselineRates{
			CompletionRate: 0.9,
			VerifyPassRate: 0.9,
			ErrorRate:      0.05,
		},
	}

	// All observations within thresholds
	obs := []session.LoopObservation{
		makeObs("idle", 0.8, 4000, true, ""),
		makeObs("idle", 0.9, 4500, true, ""),
		makeObs("idle", 0.7, 3500, true, ""),
		makeObs("idle", 0.85, 4200, true, ""),
		makeObs("idle", 0.95, 4800, true, ""),
	}

	thresholds := MockGateThresholds()
	thresholds.MinSamples = 5
	report := EvaluateGates(obs, baseline, thresholds)

	if report.Overall != VerdictPass {
		t.Errorf("overall = %s, want pass", report.Overall)
		for _, r := range report.Results {
			t.Logf("  %s: %s (current=%.3f baseline=%.3f delta=%.1f%%)", r.Metric, r.Verdict, r.CurrentVal, r.BaselineVal, r.DeltaPct)
		}
	}
}

func TestEvaluateGates_Warn(t *testing.T) {
	t.Parallel()

	baseline := &LoopBaseline{
		Aggregate: &BaselineStats{
			CostP95:    1.0,
			LatencyP95: 5000,
		},
	}

	// Cost is ~1.4x baseline P95 (warn threshold = 1.3x)
	obs := []session.LoopObservation{
		makeObs("idle", 1.4, 4000, true, ""),
		makeObs("idle", 1.5, 4500, true, ""),
		makeObs("idle", 1.3, 3500, true, ""),
		makeObs("idle", 1.4, 4200, true, ""),
		makeObs("idle", 1.45, 4800, true, ""),
	}

	report := EvaluateGates(obs, baseline, MockGateThresholds())

	if report.Overall != VerdictWarn {
		t.Errorf("overall = %s, want warn", report.Overall)
		for _, r := range report.Results {
			t.Logf("  %s: %s (current=%.3f baseline=%.3f)", r.Metric, r.Verdict, r.CurrentVal, r.BaselineVal)
		}
	}
}

func TestEvaluateGates_Fail_CompletionRate(t *testing.T) {
	t.Parallel()

	baseline := &LoopBaseline{
		Aggregate: &BaselineStats{CostP95: 1.0, LatencyP95: 5000},
	}

	// 2 of 5 complete → 40% completion rate, below 70% fail threshold
	obs := []session.LoopObservation{
		makeObs("idle", 0.5, 3000, true, ""),
		makeObs("idle", 0.5, 3000, true, ""),
		makeObs("failed", 0.5, 3000, false, "verify failed"),
		makeObs("failed", 0.5, 3000, false, "verify failed"),
		makeObs("failed", 0.5, 3000, false, "verify failed"),
	}

	report := EvaluateGates(obs, baseline, MockGateThresholds())

	if report.Overall != VerdictFail {
		t.Errorf("overall = %s, want fail", report.Overall)
	}

	// Check specific gate
	for _, r := range report.Results {
		if r.Metric == "completion_rate" && r.Verdict != VerdictFail {
			t.Errorf("completion_rate verdict = %s, want fail", r.Verdict)
		}
	}
}

func TestEvaluateGates_Fail_ErrorRate(t *testing.T) {
	t.Parallel()

	baseline := &LoopBaseline{
		Aggregate: &BaselineStats{CostP95: 1.0, LatencyP95: 5000},
	}

	// 3 of 5 have errors → 60% error rate, above 30% fail threshold
	obs := []session.LoopObservation{
		makeObs("idle", 0.5, 3000, true, ""),
		makeObs("idle", 0.5, 3000, true, ""),
		makeObs("idle", 0.5, 3000, true, "err1"),
		makeObs("idle", 0.5, 3000, true, "err2"),
		makeObs("idle", 0.5, 3000, true, "err3"),
	}

	report := EvaluateGates(obs, baseline, MockGateThresholds())

	if report.Overall != VerdictFail {
		t.Errorf("overall = %s, want fail", report.Overall)
	}
}

func TestEvaluateGates_Skip_InsufficientSamples(t *testing.T) {
	t.Parallel()

	baseline := &LoopBaseline{
		Aggregate: &BaselineStats{CostP95: 1.0, LatencyP95: 5000},
	}

	obs := []session.LoopObservation{
		makeObs("idle", 0.5, 3000, true, ""),
	}

	thresholds := DefaultGateThresholds() // MinSamples = 5
	report := EvaluateGates(obs, baseline, thresholds)

	if report.Overall != VerdictSkip {
		t.Errorf("overall = %s, want skip", report.Overall)
	}
}

func TestBuildBaseline(t *testing.T) {
	t.Parallel()

	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "fix-bug", PlannerProvider: "claude", TotalCostUSD: 1.0, TotalLatencyMs: 5000, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "fix-bug", PlannerProvider: "claude", TotalCostUSD: 1.5, TotalLatencyMs: 6000, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "fix-bug", PlannerProvider: "claude", TotalCostUSD: 2.0, TotalLatencyMs: 7000, Status: "failed", VerifyPassed: false, Error: "verify failed"},
	}

	bl := BuildBaseline(obs, 0) // no time filter

	if bl.Aggregate == nil {
		t.Fatal("expected aggregate stats")
	}
	if bl.Aggregate.SampleCount != 3 {
		t.Errorf("sample count = %d, want 3", bl.Aggregate.SampleCount)
	}
	if bl.Rates == nil {
		t.Fatal("expected rates")
	}
	if bl.Rates.CompletionRate < 0.6 || bl.Rates.CompletionRate > 0.7 {
		t.Errorf("completion rate = %.2f, want ~0.67", bl.Rates.CompletionRate)
	}

	// Check per-key entry
	key := "fix-bug:claude"
	entry, ok := bl.Entries[key]
	if !ok {
		t.Fatalf("missing baseline entry for %s", key)
	}
	if entry.SampleCount != 3 {
		t.Errorf("entry sample count = %d, want 3", entry.SampleCount)
	}
}

func TestBaselineSaveLoad(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := dir + "/baseline.json"

	bl := &LoopBaseline{
		GeneratedAt: time.Now().UTC().Truncate(time.Millisecond),
		WindowHours: 48,
		Entries: map[string]*BaselineStats{
			"test:claude": {CostP50: 0.5, CostP95: 1.0, SampleCount: 10},
		},
		Aggregate: &BaselineStats{CostP50: 0.5, CostP95: 1.0, SampleCount: 10},
		Rates:     &BaselineRates{CompletionRate: 0.9, VerifyPassRate: 0.85, ErrorRate: 0.05},
	}

	if err := SaveBaseline(path, bl); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if loaded.WindowHours != 48 {
		t.Errorf("window hours = %.0f, want 48", loaded.WindowHours)
	}
	if loaded.Aggregate.SampleCount != 10 {
		t.Errorf("aggregate samples = %d, want 10", loaded.Aggregate.SampleCount)
	}
}
