package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// Stress/edge scenarios
// ---------------------------------------------------------------------------

// BudgetExhaustion: session hits budget limit, verifies graceful stop.
func BudgetExhaustion() Scenario {
	return Scenario{
		Name:     "budget-exhaustion",
		Category: "stress",
		Provider: session.ProviderClaude,
		Tags:     []string{"stress", "budget", "cost"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":  "package main\n\nfunc main() {}\n",
				".ralphrc": "PROJECT_NAME=\"e2e-budget\"\nRALPH_SESSION_BUDGET=\"0.50\"\n",
			})
		},
		PlannerResponse: plannerJSON("Expensive refactor", "Perform a large-scale refactoring that will exceed the $0.50 budget"),
		WorkerBehavior:  nil,
		VerifyCommands:  []string{"test -f main.go"},
		ExpectedStatus:  "failed",
		MockCostUSD:     0.55,
		MockTurnCount:   8,
		MockFailure:     "budget exceeded: $0.55 > $0.50 limit",
		Constraints:     Constraints{MaxCostUSD: 0.60, MaxDurationSec: 30, MinCompletionRate: 0.0},
	}
}

// TimeoutCascade: multiple workers timeout simultaneously.
func TimeoutCascade() Scenario {
	return Scenario{
		Name:     "timeout-cascade",
		Category: "stress",
		Provider: session.ProviderClaude,
		Tags:     []string{"stress", "timeout", "team"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"worker_a.go": "package main\n\n// placeholder A\n",
				"worker_b.go": "package main\n\n// placeholder B\n",
				"worker_c.go": "package main\n\n// placeholder C\n",
				".ralphrc":    "PROJECT_NAME=\"e2e-timeout\"\nCLAUDE_TIMEOUT_MINUTES=\"1\"\n",
			})
		},
		PlannerResponse: plannerJSON("Parallel worker task", "Have three workers each modify their respective files; all will timeout"),
		WorkerBehavior:  nil,
		VerifyCommands: []string{
			"test -f worker_a.go",
		},
		ExpectedStatus: "failed",
		MockCostUSD:    0.45,
		MockTurnCount:  3,
		MockFailure:    "context deadline exceeded",
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 120, MinCompletionRate: 0.0},
	}
}

// CircuitBreakerTrip: repeated failures trip the circuit breaker, verify recovery.
func CircuitBreakerTrip() Scenario {
	return Scenario{
		Name:     "circuit-breaker-trip",
		Category: "stress",
		Provider: session.ProviderClaude,
		Tags:     []string{"stress", "circuit-breaker", "recovery"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
				".ralph/.circuit_breaker_state": `{"state":"CLOSED","failure_count":4,"success_count":0,"last_failure":"repeated API errors"}`,
				".ralphrc": "PROJECT_NAME=\"e2e-cb\"\nCB_FAILURE_THRESHOLD=\"5\"\nCB_HALF_OPEN_AFTER=\"10\"\n",
			})
		},
		PlannerResponse: plannerJSON("Trigger circuit breaker", "One more failure should trip the breaker to OPEN state"),
		WorkerBehavior:  nil,
		VerifyCommands:  []string{"test -f main.go"},
		ExpectedStatus:  "failed",
		MockCostUSD:     0.05,
		MockTurnCount:   1,
		MockFailure:     "circuit breaker OPEN: too many failures",
		Constraints:     Constraints{MaxCostUSD: 0.5, MaxDurationSec: 15, MinCompletionRate: 0.0},
	}
}

// ConcurrentFileConflict: two workers edit the same file, verify conflict detection.
func ConcurrentFileConflict() Scenario {
	return Scenario{
		Name:     "concurrent-file-conflict",
		Category: "stress",
		Provider: session.ProviderClaude,
		Tags:     []string{"stress", "conflict", "context-store"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"shared.go": "package main\n\nvar Shared = \"original\"\n",
				".ralph/context_store.json": `{"entries":[{"session_id":"sess-001","files":["shared.go"],"started_at":"2026-03-23T00:00:00Z"}]}`,
			})
		},
		PlannerResponse: plannerJSON("Modify shared resource", "Update the Shared variable in shared.go (another session already has it locked)"),
		WorkerBehavior:  nil,
		VerifyCommands:  []string{"test -f shared.go"},
		ExpectedStatus:  "failed",
		MockCostUSD:     0.10,
		MockTurnCount:   2,
		MockFailure:     "file conflict: shared.go edited by concurrent session",
		Constraints:     Constraints{MaxCostUSD: 0.5, MaxDurationSec: 15, MinCompletionRate: 0.0},
	}
}

// CheckpointRecovery: session crashes mid-work, verify checkpoint restore.
func CheckpointRecovery() Scenario {
	return Scenario{
		Name:     "checkpoint-recovery",
		Category: "stress",
		Provider: session.ProviderClaude,
		Tags:     []string{"stress", "checkpoint", "recovery"},
		RepoSetup: func(t *testing.T) string {
			dir := setupRepo(t, map[string]string{
				"main.go":    "package main\n\nfunc main() {\n\tprintln(\"step1\")\n}\n",
				"step2.go":   "package main\n\n// step2 not started\n",
				".ralph/progress.json": `{"iteration":3,"completed_ids":["1.1.1","1.1.2"],"status":"running"}`,
			})
			// Create a checkpoint tag simulating previous successful work
			gitRun(t, dir, "tag", "checkpoint-iter-2")
			return dir
		},
		PlannerResponse: plannerJSON("Continue from checkpoint", "Resume work from iteration 3; step 1+2 already done, do step 3"),
		WorkerBehavior: func(worktree string) error {
			// Worker successfully completes step 3 after recovery
			return os.WriteFile(filepath.Join(worktree, "step2.go"),
				[]byte("package main\n\n// step2 completed after recovery\nfunc Step2() string { return \"done\" }\n"), 0o644)
		},
		VerifyCommands: []string{
			"grep -q 'step2 completed' step2.go",
			"git tag -l checkpoint-iter-2",
		},
		ExpectedStatus: "idle",
		MockCostUSD:    0.20,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.5, MaxDurationSec: 45, MinCompletionRate: 0.75},
	}
}
