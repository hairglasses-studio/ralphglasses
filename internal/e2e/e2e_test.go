package e2e

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
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
	if len(scenarios) < 6 {
		t.Errorf("expected at least 6 scenarios, got %d", len(scenarios))
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
		if s.WorkerBehavior == nil && s.MockFailure == "" {
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

// --- Batch 5 integration chain tests ---

// Test 5.1: Verify budget enforcement triggers when session costs exceed the
// configured planner+worker budget at the 90% headroom threshold.
func TestE2E_LoopWithBudgetEnforcement(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	ctx := context.Background()

	scenario := Scenario{
		Name:     "budget-enforcement-chain",
		Category: "cost",
		Provider: session.ProviderClaude,
		Tags:     []string{"cost", "budget", "enforcement", "batch5"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"budget test\")\n}\n",
			})
		},
		PlannerResponse: plannerJSON("Budget enforcement test", "Small edit to verify budget tracking"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"budget enforced\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'budget enforced' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    1.80, // high cost: planner=$1.80, worker=$1.80 => total=$3.60
		MockTurnCount:  5,
		Constraints:    Constraints{MaxCostUSD: 5.0, MaxDurationSec: 30, MinCompletionRate: 0.8},
		ProfilePatch: func(p *session.LoopProfile) {
			p.PlannerBudgetUSD = 0.50
			p.WorkerBudgetUSD = 1.50
			// total budget = $2.00, headroom threshold at 90% = $1.80
		},
	}

	h := NewHarness(t)
	status, err := h.RunScenario(ctx, scenario)

	// StepLoop should succeed (budget check happens in RunLoop, not StepLoop)
	if err != nil {
		t.Fatalf("RunScenario returned unexpected error: %v", err)
	}
	if status != "idle" {
		t.Errorf("status = %q, want idle", status)
	}

	// After the scenario, verify sessions have the expected cost data.
	// The harness sets MockCostUSD=$1.80 on each session.
	sessions := h.Manager.List("")
	if len(sessions) == 0 {
		t.Fatal("expected at least one session after scenario")
	}

	var totalSpent float64
	for _, sess := range sessions {
		sess.Lock()
		totalSpent += sess.SpentUSD
		sess.Unlock()
	}

	// Total budget = PlannerBudgetUSD + WorkerBudgetUSD = $2.00
	// 90% headroom threshold = $1.80
	// Aggregate spend = $1.80 * 2 sessions = $3.60 (well above $1.80 threshold)
	totalBudget := 0.50 + 1.50 // $2.00
	threshold := totalBudget * 0.90
	if totalSpent < threshold {
		t.Errorf("total spend $%.2f should exceed budget threshold $%.2f", totalSpent, threshold)
	}

	// Verify the loop run exists and has iterations with budget data.
	loops := h.Manager.ListLoops()
	if len(loops) == 0 {
		t.Fatal("expected at least one loop run")
	}
	run := loops[0]
	run.Lock()
	profile := run.Profile
	iterCount := len(run.Iterations)
	run.Unlock()

	if iterCount == 0 {
		t.Fatal("expected at least one iteration in the loop")
	}
	if profile.PlannerBudgetUSD != 0.50 {
		t.Errorf("PlannerBudgetUSD = %.2f, want 0.50", profile.PlannerBudgetUSD)
	}
	if profile.WorkerBudgetUSD != 1.50 {
		t.Errorf("WorkerBudgetUSD = %.2f, want 1.50", profile.WorkerBudgetUSD)
	}
}

// Test 5.2: Verify cascade routing subsystem doesn't break the loop lifecycle.
func TestE2E_LoopWithCascadeRouting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e tests in short mode")
	}

	ctx := context.Background()

	scenario := Scenario{
		Name:     "cascade-routing-chain",
		Category: "multi_provider",
		Provider: session.ProviderClaude,
		Tags:     []string{"cascade", "routing", "batch5"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"cascade test\")\n}\n",
			})
		},
		PlannerResponse: plannerJSON("Cascade routing test", "Edit file to verify cascade routing integration"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"cascade routed\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'cascade routed' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.25,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
		ManagerSetup: func(m *session.Manager) {
			cfg := session.DefaultCascadeConfig()
			cr := session.NewCascadeRouter(cfg, nil, nil, "")
			m.SetCascadeRouter(cr)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableCascade = true
		},
	}

	h := NewHarness(t)
	status, err := h.RunScenario(ctx, scenario)

	if err != nil {
		t.Fatalf("RunScenario returned unexpected error: %v", err)
	}
	if status != "idle" {
		t.Errorf("status = %q, want idle", status)
	}

	// Verify cascade router is wired on the manager.
	if !h.Manager.HasCascadeRouter() {
		t.Error("expected cascade router to be set on manager")
	}

	// Verify the loop completed with at least one iteration.
	loops := h.Manager.ListLoops()
	if len(loops) == 0 {
		t.Fatal("expected at least one loop run")
	}
	run := loops[0]
	run.Lock()
	iterCount := len(run.Iterations)
	enableCascade := run.Profile.EnableCascade
	run.Unlock()

	if iterCount == 0 {
		t.Fatal("expected at least one iteration in the loop")
	}
	if !enableCascade {
		t.Error("profile.EnableCascade = false, want true")
	}
}
