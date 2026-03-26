package e2e

import (
	"strings"
	"testing"
	"time"
)

func TestFormatGateReport_MixedVerdicts(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 10,
		Overall:     VerdictFail,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictPass, BaselineVal: 1.0, CurrentVal: 0.95, DeltaPct: -5.0},
			{Metric: "total_latency", Verdict: VerdictWarn, BaselineVal: 5000, CurrentVal: 6500, DeltaPct: 30.0},
			{Metric: "completion_rate", Verdict: VerdictPass, CurrentVal: 0.92},
			{Metric: "verify_pass_rate", Verdict: VerdictFail, CurrentVal: 0.40},
			{Metric: "error_rate", Verdict: VerdictPass, CurrentVal: 0.05},
		},
	}

	out := FormatGateReport(report)

	// Header must include sample count and overall verdict.
	if !strings.Contains(out, "samples=10") {
		t.Errorf("missing sample count in output:\n%s", out)
	}
	if !strings.Contains(out, "FAIL") {
		t.Errorf("missing overall FAIL in output:\n%s", out)
	}

	// Each metric must appear.
	for _, metric := range []string{"cost_per_iteration", "total_latency", "completion_rate", "verify_pass_rate", "error_rate"} {
		if !strings.Contains(out, metric) {
			t.Errorf("missing metric %q in output:\n%s", metric, out)
		}
	}

	// Verdict indicators must appear.
	if !strings.Contains(out, "PASS") {
		t.Errorf("missing PASS indicator:\n%s", out)
	}
	if !strings.Contains(out, "WARN") {
		t.Errorf("missing WARN indicator:\n%s", out)
	}

	// Delta formatting.
	if !strings.Contains(out, "-5.0%") {
		t.Errorf("missing negative delta in output:\n%s", out)
	}
	if !strings.Contains(out, "+30.0%") {
		t.Errorf("missing positive delta in output:\n%s", out)
	}
}

func TestFormatGateReport_NilReport(t *testing.T) {
	t.Parallel()

	out := FormatGateReport(nil)
	if out != "(no report)" {
		t.Errorf("nil report: got %q, want %q", out, "(no report)")
	}
}

func TestFormatGateReport_EmptyResults(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 0,
		Overall:     VerdictSkip,
	}

	out := FormatGateReport(report)
	if !strings.Contains(out, "(no results)") {
		t.Errorf("empty results: expected '(no results)' in output:\n%s", out)
	}
}

func TestFormatGateReportMarkdown_ValidTable(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", Verdict: VerdictPass, BaselineVal: 1.0, CurrentVal: 0.80, DeltaPct: -20.0},
			{Metric: "completion_rate", Verdict: VerdictPass, CurrentVal: 0.95},
		},
	}

	out := FormatGateReportMarkdown(report)

	// Must have markdown table header.
	if !strings.Contains(out, "| Metric |") {
		t.Errorf("missing markdown table header:\n%s", out)
	}
	// Must have separator row.
	if !strings.Contains(out, "|--------|") {
		t.Errorf("missing markdown separator row:\n%s", out)
	}
	// Metrics appear in table rows.
	if !strings.Contains(out, "| cost_per_iteration |") {
		t.Errorf("missing cost metric row:\n%s", out)
	}
	if !strings.Contains(out, "| completion_rate |") {
		t.Errorf("missing completion rate row:\n%s", out)
	}
	// Overall summary line.
	if !strings.Contains(out, "samples: 5") {
		t.Errorf("missing sample count in markdown:\n%s", out)
	}
}

func TestFormatGateReportMarkdown_NilReport(t *testing.T) {
	t.Parallel()

	out := FormatGateReportMarkdown(nil)
	if !strings.Contains(out, "No report") {
		t.Errorf("nil report markdown: got %q", out)
	}
}

func TestCompareGateReports_Trends(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictWarn,
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.20},
			{Metric: "completion_rate", CurrentVal: 0.80},
			{Metric: "error_rate", CurrentVal: 0.15},
			{Metric: "total_latency", CurrentVal: 6000},
		},
	}

	current := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 0.90},  // improved (lower)
			{Metric: "completion_rate", CurrentVal: 0.95},     // improved (higher)
			{Metric: "error_rate", CurrentVal: 0.15},          // unchanged
			{Metric: "total_latency", CurrentVal: 7000},       // degraded (higher)
		},
	}

	trends := CompareGateReports(prev, current)

	if len(trends) != 4 {
		t.Fatalf("expected 4 trends, got %d", len(trends))
	}

	expected := map[string]string{
		"cost_per_iteration": "improved",
		"completion_rate":    "improved",
		"error_rate":         "unchanged",
		"total_latency":      "degraded",
	}

	for _, tr := range trends {
		want, ok := expected[tr.Metric]
		if !ok {
			t.Errorf("unexpected metric %q in trends", tr.Metric)
			continue
		}
		if tr.Direction != want {
			t.Errorf("metric %s: direction = %q, want %q (prev=%.2f curr=%.2f)",
				tr.Metric, tr.Direction, want, tr.PrevValue, tr.CurrValue)
		}
	}
}

func TestCompareGateReports_NilInputs(t *testing.T) {
	t.Parallel()

	report := &GateReport{
		Timestamp:   time.Now(),
		SampleCount: 5,
		Overall:     VerdictPass,
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},
		},
	}

	if trends := CompareGateReports(nil, report); trends != nil {
		t.Errorf("nil prev: expected nil trends, got %v", trends)
	}
	if trends := CompareGateReports(report, nil); trends != nil {
		t.Errorf("nil current: expected nil trends, got %v", trends)
	}
	if trends := CompareGateReports(nil, nil); trends != nil {
		t.Errorf("both nil: expected nil trends, got %v", trends)
	}
}

func TestCompareGateReports_DisjointMetrics(t *testing.T) {
	t.Parallel()

	prev := &GateReport{
		Results: []GateResult{
			{Metric: "cost_per_iteration", CurrentVal: 1.0},
		},
	}
	current := &GateReport{
		Results: []GateResult{
			{Metric: "error_rate", CurrentVal: 0.10},
		},
	}

	trends := CompareGateReports(prev, current)
	if len(trends) != 0 {
		t.Errorf("disjoint metrics: expected 0 trends, got %d", len(trends))
	}
}

func TestCompareGateReports_EmptyReports(t *testing.T) {
	t.Parallel()

	prev := &GateReport{}
	current := &GateReport{}

	trends := CompareGateReports(prev, current)
	if len(trends) != 0 {
		t.Errorf("empty reports: expected 0 trends, got %d", len(trends))
	}
}
