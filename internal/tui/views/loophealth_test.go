package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderLoopHealthEmpty(t *testing.T) {
	data := LoopHealthData{
		RepoName:     "test-repo",
		Observations: nil,
	}
	out := RenderLoopHealth(data, 120, 40)
	if !strings.Contains(out, "No loop observations") {
		t.Error("empty view should show 'No loop observations'")
	}
	if !strings.Contains(out, "test-repo") {
		t.Error("should show repo name in title")
	}
}

func TestRenderLoopHealthPopulated(t *testing.T) {
	now := time.Now()
	data := LoopHealthData{
		RepoName: "my-repo",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				Timestamp:       now.Add(-2 * time.Minute),
				TotalCostUSD:    0.05,
				TotalLatencyMs:  3000,
				FilesChanged:    2,
				VerifyPassed:    true,
				Status:          "idle",
				TaskType:        "refactor",
			},
			{
				IterationNumber: 2,
				Timestamp:       now.Add(-1 * time.Minute),
				TotalCostUSD:    0.08,
				TotalLatencyMs:  4500,
				FilesChanged:    3,
				VerifyPassed:    false,
				Status:          "idle",
				TaskType:        "bugfix",
			},
		},
		GateReport: &e2e.GateReport{
			Overall:     e2e.VerdictPass,
			SampleCount: 2,
			Results: []e2e.GateResult{
				{Metric: "p95_cost", Verdict: e2e.VerdictPass, BaselineVal: 0.05, DeltaPct: 10.0},
				{Metric: "p95_latency", Verdict: e2e.VerdictWarn, BaselineVal: 3000, DeltaPct: 25.0},
			},
		},
	}
	out := RenderLoopHealth(data, 120, 40)

	checks := []string{
		"my-repo",
		"Regression Gates",
		"Recent Iterations",
		"p95_cost",
		"p95_latency",
		"Status",
		"Cost",
		"Latency",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderLoopHealthNoGateData(t *testing.T) {
	data := LoopHealthData{
		RepoName: "no-gates",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				TotalCostUSD:    0.01,
				TotalLatencyMs:  1000,
				Status:          "idle",
			},
		},
		GateReport: nil,
	}
	out := RenderLoopHealth(data, 100, 30)
	if !strings.Contains(out, "No gate data") {
		t.Error("should show 'No gate data available' when no gate report")
	}
}

func TestExtractMetricSeries(t *testing.T) {
	obs := []session.LoopObservation{
		{TotalCostUSD: 0.01, TotalLatencyMs: 1000},
		{TotalCostUSD: 0.02, TotalLatencyMs: 2000},
		{TotalCostUSD: 0.03, TotalLatencyMs: 3000},
	}

	costs := extractMetricSeries(obs, "cost")
	if len(costs) != 3 {
		t.Fatalf("expected 3 cost values, got %d", len(costs))
	}
	if costs[0] != 0.01 || costs[2] != 0.03 {
		t.Errorf("cost values = %v", costs)
	}

	latencies := extractMetricSeries(obs, "latency")
	if latencies[0] != 1000 || latencies[2] != 3000 {
		t.Errorf("latency values = %v", latencies)
	}

	unknown := extractMetricSeries(obs, "unknown")
	for _, v := range unknown {
		if v != 0 {
			t.Errorf("unknown metric should return 0, got %f", v)
		}
	}
}

func TestFormatDurationHelper(t *testing.T) {
	tests := []struct {
		dur    time.Duration
		expect string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2.0h"},
	}
	for _, tt := range tests {
		got := FormatDuration(tt.dur)
		if got != tt.expect {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.dur, got, tt.expect)
		}
	}
}
