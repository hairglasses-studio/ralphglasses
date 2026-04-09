package e2e

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
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

// writeObsFile writes mock observations as JSONL to the given path.
func writeObsFile(t *testing.T, dir string, obs []session.LoopObservation) string {
	t.Helper()
	logsDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	obsPath := filepath.Join(logsDir, "loop_observations.jsonl")
	f, err := os.Create(obsPath)
	if err != nil {
		t.Fatalf("create obs file: %v", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, o := range obs {
		if err := enc.Encode(o); err != nil {
			t.Fatalf("encode obs: %v", err)
		}
	}
	return obsPath
}

func TestEvaluateFromObservations_NoFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// No observation file exists — should return skip, not error
	report, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Overall != VerdictSkip {
		t.Errorf("overall = %s, want skip", report.Overall)
	}
}

func TestEvaluateFromObservations_EmptyFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	// Create an empty observation file
	logsDir := filepath.Join(dir, ".ralph", "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logsDir, "loop_observations.jsonl"), nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	report, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.Overall != VerdictSkip {
		t.Errorf("overall = %s, want skip", report.Overall)
	}
}

func TestEvaluateFromObservations_WithData(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.8, TotalLatencyMs: 4000, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.9, TotalLatencyMs: 4500, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.7, TotalLatencyMs: 3500, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.85, TotalLatencyMs: 4200, Status: "idle", VerifyPassed: true},
		{Timestamp: now, TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.95, TotalLatencyMs: 4800, Status: "idle", VerifyPassed: true},
	}
	writeObsFile(t, dir, obs)

	// First call: no saved baseline — should get skip + baseline persisted
	report, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if report.Overall != VerdictSkip {
		t.Errorf("first call overall = %s, want skip (no prior baseline)", report.Overall)
	}

	// Verify baseline was saved
	blPath := filepath.Join(dir, ".ralph", "loop_baseline.json")
	if _, err := os.Stat(blPath); err != nil {
		t.Fatalf("baseline file not created: %v", err)
	}

	// Second call: saved baseline exists — should get a real verdict
	report2, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if report2.Overall == VerdictSkip {
		t.Errorf("second call overall = skip, want a real evaluation verdict")
	}
	if report2.SampleCount != 5 {
		t.Errorf("sample count = %d, want 5", report2.SampleCount)
	}
	// With identical data vs baseline, expect pass
	if report2.Overall != VerdictPass {
		t.Errorf("second call overall = %s, want pass", report2.Overall)
		for _, r := range report2.Results {
			t.Logf("  %s: %s (current=%.3f baseline=%.3f delta=%.1f%%)", r.Metric, r.Verdict, r.CurrentVal, r.BaselineVal, r.DeltaPct)
		}
	}
}

func TestEvaluateFromObservations_ExcludesStandaloneSessions(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	now := time.Now()
	obs := []session.LoopObservation{
		{Timestamp: now, Mode: "standalone", TaskTitle: "smoke test", PlannerProvider: "codex", TotalCostUSD: 5.0, TotalLatencyMs: 1000, Status: "failed", Error: "launch failed"},
		{Timestamp: now, Mode: "mock", TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.8, TotalLatencyMs: 4000, Status: "idle", VerifyPassed: true},
		{Timestamp: now, Mode: "mock", TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.9, TotalLatencyMs: 4500, Status: "idle", VerifyPassed: true},
		{Timestamp: now, Mode: "mock", TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.7, TotalLatencyMs: 3500, Status: "idle", VerifyPassed: true},
		{Timestamp: now, Mode: "mock", TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.85, TotalLatencyMs: 4200, Status: "idle", VerifyPassed: true},
		{Timestamp: now, Mode: "mock", TaskTitle: "task-a", PlannerProvider: "claude", TotalCostUSD: 0.95, TotalLatencyMs: 4800, Status: "idle", VerifyPassed: true},
	}
	writeObsFile(t, dir, obs)

	report, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("first call error: %v", err)
	}
	if report.Overall != VerdictSkip {
		t.Fatalf("first call overall = %s, want skip", report.Overall)
	}

	report2, err := EvaluateFromObservations(dir, MockGateThresholds(), 0)
	if err != nil {
		t.Fatalf("second call error: %v", err)
	}
	if report2.SampleCount != 5 {
		t.Fatalf("sample count = %d, want 5 (standalone excluded)", report2.SampleCount)
	}
	if report2.Overall != VerdictPass {
		t.Fatalf("overall = %s, want pass", report2.Overall)
	}
}

func TestFormatReport_ValidMarkdown(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	report := &GateReport{
		Timestamp:   ts,
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictPass, BaselineVal: 1.0, CurrentVal: 0.85, DeltaPct: -15.0},
			{Metric: "completion_rate", Verdict: VerdictWarn, BaselineVal: 0, CurrentVal: 0.80, DeltaPct: 0},
		},
	}

	md := FormatReport(report)

	// Check header
	if !strings.Contains(md, "## Gate Report") {
		t.Error("missing markdown header")
	}
	if !strings.Contains(md, ts.Format(time.RFC3339)) {
		t.Error("missing timestamp in header")
	}

	// Check overall verdict
	if !strings.Contains(md, "Overall: pass") {
		t.Error("missing overall verdict")
	}

	// Check table columns
	if !strings.Contains(md, "| Metric | Verdict | Baseline | Current | Delta |") {
		t.Error("missing table header row")
	}

	// Check table rows
	if !strings.Contains(md, "cost_per_iteration") {
		t.Error("missing cost_per_iteration row")
	}
	if !strings.Contains(md, "completion_rate") {
		t.Error("missing completion_rate row")
	}
}

func TestFormatReport_EmptyResults(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 0,
		Overall:     VerdictSkip,
		Results:     nil,
	}

	md := FormatReport(report)

	if !strings.Contains(md, "## Gate Report") {
		t.Error("missing markdown header")
	}
	// Should have table headers but no data rows
	if !strings.Contains(md, "| Metric | Verdict | Baseline | Current | Delta |") {
		t.Error("missing table header row")
	}
	// No result rows means the table header separator is the last line with |
	lines := strings.Split(strings.TrimSpace(md), "\n")
	lastPipeLine := ""
	for _, l := range lines {
		if strings.Contains(l, "|") {
			lastPipeLine = l
		}
	}
	if !strings.Contains(lastPipeLine, "---") {
		t.Log("last pipe line:", lastPipeLine)
	}
}

func TestFormatReport_Nil(t *testing.T) {
	t.Parallel()

	md := FormatReport(nil)
	if md != "" {
		t.Errorf("nil report should produce empty string, got %q", md)
	}
}

func TestFormatReportJSON_RoundTrip(t *testing.T) {
	t.Parallel()

	ts := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	original := &GateReport{
		Timestamp:   ts,
		SampleCount: 3,
		Overall:     VerdictWarn,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictWarn, BaselineVal: 1.0, CurrentVal: 1.35, DeltaPct: 35.0},
		},
	}

	data, err := FormatReportJSON(original)
	if err != nil {
		t.Fatalf("FormatReportJSON: %v", err)
	}

	var decoded GateReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Overall != original.Overall {
		t.Errorf("overall = %s, want %s", decoded.Overall, original.Overall)
	}
	if decoded.SampleCount != original.SampleCount {
		t.Errorf("sample_count = %d, want %d", decoded.SampleCount, original.SampleCount)
	}
	if len(decoded.Results) != len(original.Results) {
		t.Fatalf("results len = %d, want %d", len(decoded.Results), len(original.Results))
	}
	if decoded.Results[0].Metric != original.Results[0].Metric {
		t.Errorf("metric = %q, want %q", decoded.Results[0].Metric, original.Results[0].Metric)
	}
	if decoded.Results[0].DeltaPct != original.Results[0].DeltaPct {
		t.Errorf("delta_pct = %.1f, want %.1f", decoded.Results[0].DeltaPct, original.Results[0].DeltaPct)
	}
}

func TestCompareGateVerdicts_Trends(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictFail},
			{Metric: "completion_rate", Verdict: VerdictWarn},
			{Metric: "error_rate", Verdict: VerdictPass},
			{Metric: "verify_pass_rate", Verdict: VerdictWarn},
		},
	}
	current := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictWarn}, // improved (fail->warn)
			{Metric: "completion_rate", Verdict: VerdictPass},    // improved (warn->pass)
			{Metric: "error_rate", Verdict: VerdictFail},         // degraded (pass->fail)
			{Metric: "verify_pass_rate", Verdict: VerdictWarn},   // unchanged
		},
	}

	trends := CompareGateVerdicts(prev, current)
	if len(trends) != 4 {
		t.Fatalf("expected 4 trends, got %d", len(trends))
	}

	expected := map[string]string{
		"cost_per_iteration": "improved",
		"completion_rate":    "improved",
		"error_rate":         "degraded",
		"verify_pass_rate":   "unchanged",
	}

	for _, tr := range trends {
		want, ok := expected[tr.Gate]
		if !ok {
			t.Errorf("unexpected gate %q", tr.Gate)
			continue
		}
		if tr.Direction != want {
			t.Errorf("gate %s: direction = %q, want %q (prev=%s curr=%s)",
				tr.Gate, tr.Direction, want, tr.Previous, tr.Current)
		}
	}
}

func TestCompareGateVerdicts_NilInputs(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Results: []GateResult{{Metric: "cost_per_iteration", Verdict: VerdictPass}},
	}
	if trends := CompareGateVerdicts(nil, report); trends != nil {
		t.Errorf("nil prev: expected nil, got %v", trends)
	}
	if trends := CompareGateVerdicts(report, nil); trends != nil {
		t.Errorf("nil current: expected nil, got %v", trends)
	}
}

func TestCompareGateVerdicts_DisjointMetrics(t *testing.T) {
	t.Parallel()

	prev := &GateReport{Results: []GateResult{{Metric: "a", Verdict: VerdictPass}}}
	current := &GateReport{Results: []GateResult{{Metric: "b", Verdict: VerdictFail}}}

	trends := CompareGateVerdicts(prev, current)
	if len(trends) != 0 {
		t.Errorf("disjoint metrics: expected 0 trends, got %d", len(trends))
	}
}

func TestVerdictRank(t *testing.T) {
	if verdictRank(VerdictPass) <= verdictRank(VerdictWarn) {
		t.Error("pass should rank higher than warn")
	}
	if verdictRank(VerdictWarn) <= verdictRank(VerdictFail) {
		t.Error("warn should rank higher than fail")
	}
	if verdictRank(VerdictFail) <= verdictRank(VerdictSkip) {
		t.Error("fail should rank higher than skip")
	}
}

func TestFormatReportJSON_EmptyReport(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp: time.Now(),
		Overall:   VerdictSkip,
	}

	data, err := FormatReportJSON(report)
	if err != nil {
		t.Fatalf("FormatReportJSON: %v", err)
	}

	var decoded GateReport
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Overall != VerdictSkip {
		t.Errorf("overall = %s, want skip", decoded.Overall)
	}
	if len(decoded.Results) != 0 {
		t.Errorf("results len = %d, want 0", len(decoded.Results))
	}
}

func TestBaselinePersistence(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	blPath := filepath.Join(dir, "baseline.json")

	original := &LoopBaseline{
		GeneratedAt: time.Now().UTC().Truncate(time.Millisecond),
		WindowHours: 24,
		Entries: map[string]*BaselineStats{
			"fix-bug:claude":  {CostP50: 0.5, CostP95: 1.2, LatencyP50: 3000, LatencyP95: 6000, SampleCount: 8},
			"add-feat:gemini": {CostP50: 0.3, CostP95: 0.8, LatencyP50: 2000, LatencyP95: 4000, SampleCount: 5},
		},
		Aggregate: &BaselineStats{CostP50: 0.4, CostP95: 1.0, LatencyP50: 2500, LatencyP95: 5000, SampleCount: 13},
		Rates:     &BaselineRates{CompletionRate: 0.92, VerifyPassRate: 0.88, ErrorRate: 0.03},
	}

	if err := SaveBaseline(blPath, original); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := LoadBaseline(blPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	// Verify round-trip fidelity
	if loaded.WindowHours != original.WindowHours {
		t.Errorf("window hours = %.0f, want %.0f", loaded.WindowHours, original.WindowHours)
	}
	if len(loaded.Entries) != len(original.Entries) {
		t.Errorf("entries count = %d, want %d", len(loaded.Entries), len(original.Entries))
	}
	for key, origEntry := range original.Entries {
		loadedEntry, ok := loaded.Entries[key]
		if !ok {
			t.Errorf("missing entry for %s", key)
			continue
		}
		if loadedEntry.CostP95 != origEntry.CostP95 {
			t.Errorf("entry %s CostP95 = %.2f, want %.2f", key, loadedEntry.CostP95, origEntry.CostP95)
		}
		if loadedEntry.SampleCount != origEntry.SampleCount {
			t.Errorf("entry %s SampleCount = %d, want %d", key, loadedEntry.SampleCount, origEntry.SampleCount)
		}
	}
	if loaded.Aggregate.CostP95 != original.Aggregate.CostP95 {
		t.Errorf("aggregate CostP95 = %.2f, want %.2f", loaded.Aggregate.CostP95, original.Aggregate.CostP95)
	}
	if loaded.Rates.CompletionRate != original.Rates.CompletionRate {
		t.Errorf("completion rate = %.2f, want %.2f", loaded.Rates.CompletionRate, original.Rates.CompletionRate)
	}
	if loaded.Rates.ErrorRate != original.Rates.ErrorRate {
		t.Errorf("error rate = %.4f, want %.4f", loaded.Rates.ErrorRate, original.Rates.ErrorRate)
	}
}
