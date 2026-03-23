package e2e

import (
	"context"
	"testing"
)

func TestE2EAllScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	ctx := context.Background()
	scenarios := AllScenarios()

	for _, s := range scenarios {
		s := s
		t.Run(s.Name, func(t *testing.T) {
			h := NewHarness(t)
			status, err := h.RunScenario(ctx, s)

			if s.ExpectedStatus == "idle" {
				// Success scenario — StepLoop should not return an error
				if err != nil {
					t.Errorf("expected success, got error: %v", err)
				}
				if status != "idle" {
					t.Errorf("status = %q, want idle", status)
				}
			} else {
				// Failure scenario — we expect the iteration to fail
				if status != "failed" {
					t.Errorf("status = %q, want failed", status)
				}
			}
		})
	}
}

func TestE2EScenarioCatalogComplete(t *testing.T) {
	scenarios := AllScenarios()
	if len(scenarios) != 6 {
		t.Errorf("expected 6 scenarios, got %d", len(scenarios))
	}

	names := make(map[string]bool)
	categories := make(map[string]bool)
	for _, s := range scenarios {
		if names[s.Name] {
			t.Errorf("duplicate scenario name: %s", s.Name)
		}
		names[s.Name] = true
		categories[s.Category] = true

		if s.RepoSetup == nil {
			t.Errorf("scenario %s has nil RepoSetup", s.Name)
		}
		if s.PlannerResponse == "" {
			t.Errorf("scenario %s has empty PlannerResponse", s.Name)
		}
		if s.WorkerBehavior == nil {
			t.Errorf("scenario %s has nil WorkerBehavior", s.Name)
		}
		if s.ExpectedStatus == "" {
			t.Errorf("scenario %s has empty ExpectedStatus", s.Name)
		}
	}

	// Ensure we cover multiple categories
	if len(categories) < 4 {
		t.Errorf("expected at least 4 categories, got %d: %v", len(categories), categories)
	}
}
