package e2e

import "testing"

// Scenario defines a reproducible loop cycle test case.
type Scenario struct {
	Name            string                      // unique scenario identifier
	Category        string                      // bug_fix, refactor, test, docs, feature
	RepoSetup       func(t *testing.T) string   // creates temp repo, returns path
	PlannerResponse string                      // JSON the mock planner returns
	WorkerBehavior  func(worktree string) error // what mock worker writes to worktree
	VerifyCommands  []string                    // commands run to verify worker output
	ExpectedStatus  string                      // "idle" (success) or "failed"
	MockCostUSD     float64                     // cost to set on mock sessions
	MockTurnCount   int                         // turns to set on mock sessions
	Constraints     Constraints                 // regression limits
}

// Constraints define acceptable bounds for a scenario.
type Constraints struct {
	MaxCostUSD        float64
	MaxDurationSec    float64
	MinCompletionRate float64
}
