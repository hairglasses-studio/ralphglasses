package views

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
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

func TestRenderRepoDetailParseErrors(t *testing.T) {
	r := &model.Repo{
		Name: "broken-repo",
		Path: "/path",
		RefreshErrors: []error{
			errors.New("invalid JSON in loop_status.json"),
			errors.New("bad timestamp in circuit_breaker.json"),
		},
	}
	output := RenderRepoDetail(r, 120, nil)

	// Status bar line should contain the compact warning indicator
	if !strings.Contains(output, "2 parse error(s)") {
		t.Error("title line should show '2 parse error(s)' warning indicator")
	}
	// Detailed warnings section should still be present
	if !strings.Contains(output, "Warnings") {
		t.Error("should show detailed Warnings section")
	}
	if !strings.Contains(output, "invalid JSON") {
		t.Error("should show individual error details")
	}
}

func TestRenderRepoDetailIncludesOllamaInventorySummary(t *testing.T) {
	r := &model.Repo{
		Name: "ollama-repo",
		Path: "/path",
	}
	health := &RepoDetailHealth{
		OllamaInventory: &session.OllamaInventory{
			BaseURL:               "http://127.0.0.1:11434",
			Reachable:             true,
			RequiredModels:        []string{"code-primary", "code-heavy"},
			ReadyRequiredModels:   []string{"code-primary"},
			MissingRequiredModels: []string{"code-heavy"},
			ManagedAliases: []session.OllamaAliasInventory{
				{Alias: "code-fast", Status: "missing_alias"},
			},
		},
	}

	output := RenderRepoDetail(r, 120, health)
	for _, check := range []string{"Local Models", "http://127.0.0.1:11434", "1/2 ready", "code-heavy", "code-fast"} {
		if !strings.Contains(output, check) {
			t.Errorf("output missing %q", check)
		}
	}
}
