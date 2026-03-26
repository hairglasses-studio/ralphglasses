package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestFormatGateReport_AllPass(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 10,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictPass, BaselineVal: 1.0, CurrentVal: 0.90, DeltaPct: -10.0},
			{Metric: "completion_rate", Verdict: VerdictPass, BaselineVal: 0.85, CurrentVal: 0.95, DeltaPct: 11.8},
			{Metric: "error_rate", Verdict: VerdictPass, BaselineVal: 0.10, CurrentVal: 0.02, DeltaPct: -80.0},
		},
	}

	out := FormatGateReport(report)
	if !strings.Contains(out, "PASS") {
		t.Errorf("all-pass report should contain PASS indicator:\n%s", out)
	}
	// Should not contain FAIL or WARN
	if strings.Contains(out, "FAIL") {
		t.Errorf("all-pass report should not contain FAIL:\n%s", out)
	}
	if strings.Contains(out, "WARN") {
		t.Errorf("all-pass report should not contain WARN:\n%s", out)
	}
}

func TestFormatGateReport_AllFail(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 3,
		Overall:     VerdictFail,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictFail, BaselineVal: 1.0, CurrentVal: 3.0, DeltaPct: 200.0},
			{Metric: "completion_rate", Verdict: VerdictFail, BaselineVal: 0.90, CurrentVal: 0.20, DeltaPct: -77.8},
		},
	}

	out := FormatGateReport(report)
	if !strings.Contains(out, "FAIL") {
		t.Errorf("all-fail report should contain FAIL indicator:\n%s", out)
	}
	// Should not contain PASS (except possibly in the table header)
	// Count PASS occurrences — only the overall should not be PASS.
	passCount := strings.Count(out, "PASS")
	if passCount > 0 {
		t.Errorf("all-fail report should not contain PASS, found %d occurrences:\n%s", passCount, out)
	}
}

func TestFormatGateReportMarkdown_EmptyResults(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 0,
		Overall:     VerdictSkip,
	}

	out := FormatGateReportMarkdown(report)
	if !strings.Contains(out, "| Metric |") {
		t.Errorf("markdown should have table header even with empty results:\n%s", out)
	}
	if !strings.Contains(out, "no results") {
		t.Errorf("markdown should indicate no results:\n%s", out)
	}
}

func TestFormatGateReportMarkdown_AllPass(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictPass, BaselineVal: 1.0, CurrentVal: 0.80, DeltaPct: -20.0},
		},
	}

	out := FormatGateReportMarkdown(report)
	if !strings.Contains(out, "PASS") {
		t.Errorf("markdown all-pass should contain PASS:\n%s", out)
	}
	// Verify it has pipe-delimited table rows
	lines := strings.Split(out, "\n")
	tableRows := 0
	for _, line := range lines {
		if strings.HasPrefix(line, "|") && strings.Contains(line, "|") {
			tableRows++
		}
	}
	if tableRows < 3 { // header + separator + at least 1 data row
		t.Errorf("expected at least 3 table rows in markdown, got %d:\n%s", tableRows, out)
	}
}

func TestCompareGateReports_IdenticalReports(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},
			{Metric: "completion_rate", CurrentVal: 0.90},
			{Metric: "error_rate", CurrentVal: 0.05},
		},
	}

	trends := CompareGateReports(report, report)
	for _, tr := range trends {
		if tr.Direction != "unchanged" {
			t.Errorf("identical reports: metric %s should be unchanged, got %q", tr.Metric, tr.Direction)
		}
	}
}

func TestCompareGateReports_AllImproved(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 2.0},
			{Metric: "completion_rate", CurrentVal: 0.50},
		},
	}
	current := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},  // lower is better
			{Metric: "completion_rate", CurrentVal: 0.90},     // higher is better
		},
	}

	trends := CompareGateReports(prev, current)
	if len(trends) != 2 {
		t.Fatalf("expected 2 trends, got %d", len(trends))
	}
	for _, tr := range trends {
		if tr.Direction != "improved" {
			t.Errorf("metric %s: expected improved, got %q (prev=%.2f, curr=%.2f)",
				tr.Metric, tr.Direction, tr.PrevValue, tr.CurrValue)
		}
	}
}

func TestCompareGateReports_AllDegraded(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},
			{Metric: "completion_rate", CurrentVal: 0.90},
		},
	}
	current := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 3.0},  // higher cost = degraded
			{Metric: "completion_rate", CurrentVal: 0.40},     // lower rate = degraded
		},
	}

	trends := CompareGateReports(prev, current)
	if len(trends) != 2 {
		t.Fatalf("expected 2 trends, got %d", len(trends))
	}
	for _, tr := range trends {
		if tr.Direction != "degraded" {
			t.Errorf("metric %s: expected degraded, got %q (prev=%.2f, curr=%.2f)",
				tr.Metric, tr.Direction, tr.PrevValue, tr.CurrValue)
		}
	}
}

func TestCompareGateReports_PartialOverlap(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},
			{Metric: "old_metric", CurrentVal: 5.0},
		},
	}
	current := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 0.8},
			{Metric: "new_metric", CurrentVal: 10.0},
		},
	}

	trends := CompareGateReports(prev, current)
	// Only cost_per_iteration appears in both
	if len(trends) != 1 {
		t.Fatalf("expected 1 trend for overlapping metric, got %d", len(trends))
	}
	if trends[0].Metric != "cost_per_iteration" {
		t.Errorf("expected cost_per_iteration trend, got %s", trends[0].Metric)
	}
}

func TestFormatGateReport_SingleResult(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 1,
		Overall:     VerdictWarn,
		Results: []GateResult{
			{Metric: "total_latency", Verdict: VerdictWarn, BaselineVal: 3000, CurrentVal: 4500, DeltaPct: 50.0},
		},
	}

	out := FormatGateReport(report)
	if !strings.Contains(out, "total_latency") {
		t.Errorf("should contain metric name:\n%s", out)
	}
	if !strings.Contains(out, "WARN") {
		t.Errorf("should contain WARN indicator:\n%s", out)
	}
	if !strings.Contains(out, "+50.0%") {
		t.Errorf("should contain delta:\n%s", out)
	}
}

func TestFormatFloat_ZeroValue(t *testing.T) {
	t.Parallel()

	result := formatFloat(0)
	if result != "0" {
		t.Errorf("formatFloat(0) = %q, want %q", result, "0")
	}
}

func TestFormatDelta_ZeroValue(t *testing.T) {
	t.Parallel()

	result := formatDelta(0)
	if result != "-" {
		t.Errorf("formatDelta(0) = %q, want %q", result, "-")
	}
}

func TestVerdictIndicator_UnknownVerdict(t *testing.T) {
	t.Parallel()

	result := verdictIndicator("custom_verdict")
	if result != "custom_verdict" {
		t.Errorf("verdictIndicator for unknown = %q, want %q", result, "custom_verdict")
	}
}
