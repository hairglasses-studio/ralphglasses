package views

import (
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestRenderObservationViewEmpty(t *testing.T) {
	data := ObservationViewData{
		RepoName:     "test-repo",
		Observations: nil,
	}
	out := RenderObservationView(data, 120, 40)
	if !strings.Contains(out, "No loop observations") {
		t.Error("empty view should show 'No loop observations'")
	}
	if !strings.Contains(out, "test-repo") {
		t.Error("empty view should show repo name")
	}
}

func TestRenderObservationViewPopulated(t *testing.T) {
	now := time.Now()
	data := ObservationViewData{
		RepoName: "my-repo",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				Timestamp:       now.Add(-2 * time.Minute),
				PlannerTokensOut: 100,
				WorkerTokensOut:  200,
				TotalCostUSD:    0.05,
				TotalLatencyMs:  3000,
				FilesChanged:    2,
				VerifyPassed:    true,
				Status:          "idle",
			},
			{
				IterationNumber: 2,
				Timestamp:       now.Add(-1 * time.Minute),
				PlannerTokensOut: 150,
				WorkerTokensOut:  250,
				TotalCostUSD:    0.08,
				TotalLatencyMs:  4500,
				FilesChanged:    3,
				VerifyPassed:    false,
				Status:          "idle",
			},
			{
				IterationNumber: 3,
				Timestamp:       now,
				PlannerTokensOut: 120,
				WorkerTokensOut:  180,
				TotalCostUSD:    0.06,
				TotalLatencyMs:  2500,
				FilesChanged:    1,
				VerifyPassed:    true,
				Status:          "idle",
			},
		},
	}
	out := RenderObservationView(data, 120, 40)

	checks := []string{
		"my-repo",
		"Iteration Metrics",
		"3 iterations",
		"Tokens",
		"Cost",
		"Duration",
		"Files",
		"Summary",
		"Total tokens",
		"Total cost",
		"Total duration",
		"Total files",
		"Velocity",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderObservationViewNoVelocityWithOneObs(t *testing.T) {
	data := ObservationViewData{
		RepoName: "single-obs",
		Observations: []session.LoopObservation{
			{
				IterationNumber: 1,
				Timestamp:       time.Now(),
				TotalCostUSD:    0.01,
				TotalLatencyMs:  1000,
				FilesChanged:    1,
				Status:          "idle",
			},
		},
	}
	out := RenderObservationView(data, 80, 30)
	// Velocity requires >= 2 observations
	if strings.Contains(out, "Velocity") {
		t.Error("should not show velocity with only 1 observation")
	}
	// Should still show summary
	if !strings.Contains(out, "Summary") {
		t.Error("should still show summary section")
	}
}
