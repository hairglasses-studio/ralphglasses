package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderObservationView_Empty(t *testing.T) {
	data := ObservationViewData{
		RepoName:     "empty-repo",
		Observations: nil,
	}

	out := RenderObservationView(data, 120, 40)
	if !strings.Contains(out, "No loop observations") {
		t.Error("should show no-observations message")
	}
	if !strings.Contains(out, "empty-repo") {
		t.Error("should show repo name in title")
	}
}

func TestRenderObservationView_WithData(t *testing.T) {
	obs := []session.LoopObservation{
		{
			Timestamp:        time.Now().Add(-10 * time.Minute),
			IterationNumber:  1,
			PlannerTokensOut: 500,
			WorkerTokensOut:  1000,
			TotalCostUSD:     0.50,
			TotalLatencyMs:   5000,
			FilesChanged:     3,
		},
		{
			Timestamp:        time.Now().Add(-5 * time.Minute),
			IterationNumber:  2,
			PlannerTokensOut: 600,
			WorkerTokensOut:  1200,
			TotalCostUSD:     0.75,
			TotalLatencyMs:   8000,
			FilesChanged:     5,
		},
	}

	data := ObservationViewData{
		RepoName:     "active-repo",
		Observations: obs,
	}

	out := RenderObservationView(data, 120, 40)

	checks := []string{
		"Observation Sparklines",
		"active-repo",
		"Iteration Metrics",
		"2 iterations",
		"Tokens",
		"Cost",
		"Duration",
		"Files",
		"Summary",
		"Total tokens",
		"Total cost",
		"Total duration",
		"Total files",
		"Velocity", // 2 observations triggers velocity calculation
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderObservationView_SingleObservation(t *testing.T) {
	obs := []session.LoopObservation{
		{
			Timestamp:        time.Now(),
			IterationNumber:  1,
			PlannerTokensOut: 100,
			WorkerTokensOut:  200,
			TotalCostUSD:     0.25,
			TotalLatencyMs:   3000,
			FilesChanged:     1,
		},
	}

	data := ObservationViewData{
		RepoName:     "single-obs",
		Observations: obs,
	}

	out := RenderObservationView(data, 100, 30)

	// With only one observation, velocity should NOT be shown
	if strings.Contains(out, "Velocity") {
		t.Error("should not show velocity with only 1 observation")
	}
	if !strings.Contains(out, "1 iterations") {
		t.Error("should show 1 iteration count")
	}
}

func TestRenderObservationView_NarrowWidth(t *testing.T) {
	obs := []session.LoopObservation{
		{TotalCostUSD: 0.10, TotalLatencyMs: 1000, PlannerTokensOut: 50, WorkerTokensOut: 100, FilesChanged: 1},
		{TotalCostUSD: 0.20, TotalLatencyMs: 2000, PlannerTokensOut: 60, WorkerTokensOut: 120, FilesChanged: 2},
	}

	data := ObservationViewData{
		RepoName:     "narrow-repo",
		Observations: obs,
	}

	// Should not panic at narrow width
	out := RenderObservationView(data, 50, 20)
	if out == "" {
		t.Error("should produce output even at narrow width")
	}
}

func TestRenderObservationView_SummaryTotals(t *testing.T) {
	obs := []session.LoopObservation{
		{TotalCostUSD: 1.00, TotalLatencyMs: 2000, PlannerTokensOut: 100, WorkerTokensOut: 200, FilesChanged: 2},
		{TotalCostUSD: 2.00, TotalLatencyMs: 3000, PlannerTokensOut: 150, WorkerTokensOut: 250, FilesChanged: 3},
	}

	data := ObservationViewData{
		RepoName:     "totals-repo",
		Observations: obs,
	}

	out := RenderObservationView(data, 120, 40)

	// Total cost should be 3.00
	if !strings.Contains(out, "$3.000") {
		t.Error("total cost should be $3.000")
	}
	// Average cost should be 1.50
	if !strings.Contains(out, "$1.500") {
		t.Error("average cost should be $1.500")
	}
}
