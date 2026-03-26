package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderLoopHealth_Empty(t *testing.T) {
	data := LoopHealthData{
		RepoName:     "empty-repo",
		Observations: nil,
	}

	out := RenderLoopHealth(data, 120, 40)
	if !strings.Contains(out, "No loop observations") {
		t.Error("should show no-observations message")
	}
	if !strings.Contains(out, "empty-repo") {
		t.Error("should show repo name")
	}
}

func TestRenderLoopHealth_WithObservations(t *testing.T) {
	obs := []session.LoopObservation{
		{
			Timestamp:       time.Now().Add(-10 * time.Minute),
			IterationNumber: 1,
			TotalCostUSD:    0.50,
			TotalLatencyMs:  5000,
			VerifyPassed:    true,
			Status:          "idle",
			FilesChanged:    3,
			TaskType:        "feature",
		},
		{
			Timestamp:       time.Now().Add(-5 * time.Minute),
			IterationNumber: 2,
			TotalCostUSD:    0.75,
			TotalLatencyMs:  8000,
			VerifyPassed:    false,
			Status:          "idle",
			FilesChanged:    5,
			TaskType:        "bugfix",
		},
	}

	data := LoopHealthData{
		RepoName:     "active-repo",
		Observations: obs,
	}

	out := RenderLoopHealth(data, 120, 40)

	checks := []string{
		"Loop Health",
		"active-repo",
		"Recent Iterations",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderLoopHealth_WithGateReport(t *testing.T) {
	obs := []session.LoopObservation{
		{IterationNumber: 1, TotalCostUSD: 0.50, TotalLatencyMs: 5000, Status: "idle"},
	}

	data := LoopHealthData{
		RepoName:     "gated-repo",
		Observations: obs,
		GateReport: &e2e.GateReport{
			Overall:     e2e.VerdictPass,
			SampleCount: 10,
			Results: []e2e.GateResult{
				{Metric: "p95_cost", Verdict: e2e.VerdictPass, BaselineVal: 0.50, DeltaPct: -5.0},
				{Metric: "p95_latency", Verdict: e2e.VerdictWarn, BaselineVal: 8000, DeltaPct: 15.0},
			},
		},
	}

	out := RenderLoopHealth(data, 120, 40)

	if !strings.Contains(out, "Regression Gates") {
		t.Error("should show regression gates section")
	}
	if !strings.Contains(out, "p95_cost") {
		t.Error("should show cost metric")
	}
	if !strings.Contains(out, "p95_latency") {
		t.Error("should show latency metric")
	}
}

func TestRenderLoopHealth_NoGateData(t *testing.T) {
	obs := []session.LoopObservation{
		{IterationNumber: 1, TotalCostUSD: 0.10, Status: "idle"},
	}

	data := LoopHealthData{
		RepoName:     "no-gates",
		Observations: obs,
		GateReport:   nil,
	}

	out := RenderLoopHealth(data, 120, 40)
	if !strings.Contains(out, "No gate data") {
		t.Error("should show 'No gate data available'")
	}
}

func TestExtractMetricSeries(t *testing.T) {
	obs := []session.LoopObservation{
		{TotalCostUSD: 0.10, TotalLatencyMs: 1000},
		{TotalCostUSD: 0.20, TotalLatencyMs: 2000},
		{TotalCostUSD: 0.30, TotalLatencyMs: 3000},
	}

	costs := extractMetricSeries(obs, "cost")
	if len(costs) != 3 {
		t.Fatalf("expected 3 cost values, got %d", len(costs))
	}
	if costs[0] != 0.10 || costs[1] != 0.20 || costs[2] != 0.30 {
		t.Errorf("cost values = %v, want [0.10 0.20 0.30]", costs)
	}

	latencies := extractMetricSeries(obs, "latency")
	if latencies[0] != 1000 || latencies[1] != 2000 || latencies[2] != 3000 {
		t.Errorf("latency values = %v, want [1000 2000 3000]", latencies)
	}

	unknown := extractMetricSeries(obs, "unknown")
	for _, v := range unknown {
		if v != 0 {
			t.Errorf("unknown metric should return zeros, got %f", v)
		}
	}
}

func TestFormatDurationFunc(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2.0h"},
	}

	for _, tt := range tests {
		got := FormatDuration(tt.dur)
		if got != tt.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tt.dur, got, tt.want)
		}
	}
}
