package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// Cost scenarios
// ---------------------------------------------------------------------------

// CostTrackingAccuracy: verify ledger entries match provider-reported costs.
func CostTrackingAccuracy() Scenario {
	return Scenario{
		Name:     "cost-tracking-accuracy",
		Category: "cost",
		Provider: session.ProviderClaude,
		Tags:     []string{"cost", "ledger", "accuracy"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"cost test\")\n}\n",
				".ralph/cost_ledger.jsonl": `{"session_id":"prev-001","provider":"claude","cost_usd":0.10,"turns":3}` + "\n",
			})
		},
		PlannerResponse: plannerJSON("Tracked cost operation", "Perform a small edit and verify the cost is accurately recorded in the ledger"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"cost tracked\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{
			"grep -q 'cost tracked' main.go",
			"test -f .ralph/cost_ledger.jsonl",
		},
		ExpectedStatus: "idle",
		MockCostUSD:    0.15,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
	}
}

// FleetBudgetEnforcement: fleet-wide budget cap stops new work.
func FleetBudgetEnforcement() Scenario {
	return Scenario{
		Name:     "fleet-budget-enforcement",
		Category: "cost",
		Provider: session.ProviderClaude,
		Tags:     []string{"cost", "fleet", "budget"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":  "package main\n\nfunc main() {}\n",
				".ralphrc": "PROJECT_NAME=\"e2e-fleet-budget\"\nFLEET_BUDGET_USD=\"1.00\"\n",
				".ralph/cost_ledger.jsonl": `{"session_id":"s1","provider":"claude","cost_usd":0.40,"turns":5}` + "\n" +
					`{"session_id":"s2","provider":"gemini","cost_usd":0.30,"turns":4}` + "\n" +
					`{"session_id":"s3","provider":"codex","cost_usd":0.25,"turns":3}` + "\n",
			})
		},
		PlannerResponse: plannerJSON("Blocked by fleet budget", "Attempt work that should be rejected because fleet budget ($1.00) is nearly exhausted ($0.95 spent)"),
		WorkerBehavior:  nil,
		VerifyCommands:  []string{"test -f main.go"},
		ExpectedStatus:  "failed",
		MockCostUSD:     0.00,
		MockTurnCount:   0,
		MockFailure:     "fleet budget exceeded: $0.95/$1.00",
		Constraints:     Constraints{MaxCostUSD: 0.10, MaxDurationSec: 10, MinCompletionRate: 0.0},
	}
}
