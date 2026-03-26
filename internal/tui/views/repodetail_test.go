package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestRenderRepoDetailFull(t *testing.T) {
	r := &model.Repo{
		Name: "test-repo",
		Path: "/home/user/test-repo",
		Status: &model.LoopStatus{
			Status:          "running",
			LoopCount:       10,
			CallsMadeThisHr: 5,
			MaxCallsPerHour: 80,
			Model:           "sonnet",
			SessionSpendUSD: 2.50,
			BudgetStatus:    "ok",
			LastAction:      "built binary",
			Timestamp:       time.Now(),
		},
		Circuit: &model.CircuitBreakerState{
			State:                 "CLOSED",
			ConsecutiveNoProgress: 1,
			TotalOpens:            0,
			LastChange:            time.Now(),
		},
		Progress: &model.Progress{
			Iteration:    3,
			CompletedIDs: []string{"task-1", "task-2"},
			Status:       "in_progress",
		},
		Config: &model.RalphConfig{
			Values: map[string]string{"PROJECT_NAME": "test"},
		},
	}

	output := RenderRepoDetail(r, 120, nil)

	checks := []string{"test-repo", "Status", "Circuit Breaker", "Configuration", "Progress", "sonnet", "task-1", "PROJECT_NAME"}
	for _, check := range checks {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}

func TestRenderRepoDetailEmpty(t *testing.T) {
	r := &model.Repo{Name: "empty-repo", Path: "/path"}
	output := RenderRepoDetail(r, 80, nil)

	if !strings.Contains(output, "No status data") {
		t.Error("should show 'No status data'")
	}
	if !strings.Contains(output, "No circuit breaker data") {
		t.Error("should show 'No circuit breaker data'")
	}
	if !strings.Contains(output, "No progress data") {
		t.Error("should show 'No progress data'")
	}
}

func TestRenderRepoDetailWithRC(t *testing.T) {
	r := &model.Repo{Name: "rc-repo", Path: "/path", HasRC: true}
	output := RenderRepoDetail(r, 80, nil)
	if !strings.Contains(output, "failed to parse") {
		t.Error("should show parse failure for HasRC without loaded config")
	}
}

func TestRenderRepoDetailParseErrors(t *testing.T) {
	r := &model.Repo{
		Name: "warn-repo",
		Path: "/path",
		RefreshErrors: []error{
			errors.New("failed to read .ralphrc"),
			errors.New("invalid JSON"),
		},
	}
	output := RenderRepoDetail(r, 80, nil)
	// Compact indicator in title line
	if !strings.Contains(output, "2") {
		t.Error("title should show parse error count")
	}
	// Detailed warnings section
	if !strings.Contains(output, "Warnings") {
		t.Error("should render Warnings section")
	}
	if !strings.Contains(output, "failed to read .ralphrc") {
		t.Error("should list first parse error")
	}
	if !strings.Contains(output, "invalid JSON") {
		t.Error("should list second parse error")
	}
}

func TestRenderRepoDetailExitReason(t *testing.T) {
	r := &model.Repo{
		Name: "exited-repo",
		Path: "/path",
		Status: &model.LoopStatus{
			Status:     "stopped",
			ExitReason: "budget exceeded",
			Timestamp:  time.Now(),
		},
	}
	output := RenderRepoDetail(r, 80, nil)
	if !strings.Contains(output, "budget exceeded") {
		t.Error("should show exit reason")
	}
}
