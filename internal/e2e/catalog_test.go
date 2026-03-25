package e2e

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// validProviders lists all recognized provider values (empty string = claude default).
var validProviders = map[session.Provider]bool{
	"":                     true, // default = claude
	session.ProviderClaude: true,
	session.ProviderGemini: true,
	session.ProviderCodex:  true,
}

// validCategories lists all recognized scenario categories.
var validCategories = map[string]bool{
	"bug_fix":        true,
	"refactor":       true,
	"test":           true,
	"docs":           true,
	"feature":        true,
	"multi_provider": true,
	"stress":         true,
	"cost":           true,
	"self_learning":  true,
}

func TestAllScenariosHaveValidFields(t *testing.T) {
	scenarios := AllScenarios()
	if len(scenarios) == 0 {
		t.Fatal("AllScenarios() returned no scenarios")
	}

	seen := make(map[string]bool)
	for _, s := range scenarios {
		t.Run(s.Name, func(t *testing.T) {
			// Non-empty Name
			if s.Name == "" {
				t.Error("scenario has empty Name")
			}

			// Unique Name
			if seen[s.Name] {
				t.Errorf("duplicate scenario name: %q", s.Name)
			}
			seen[s.Name] = true

			// Non-empty Category
			if s.Category == "" {
				t.Error("scenario has empty Category")
			}
			if !validCategories[s.Category] {
				t.Errorf("unrecognized category %q", s.Category)
			}

			// Valid Provider (empty is OK — means claude default)
			if !validProviders[s.Provider] {
				t.Errorf("invalid provider %q", s.Provider)
			}

			// RepoSetup must be set
			if s.RepoSetup == nil {
				t.Error("scenario has nil RepoSetup")
			}

			// PlannerResponse must be non-empty
			if s.PlannerResponse == "" {
				t.Error("scenario has empty PlannerResponse")
			}

			// WorkerBehavior must be set (unless MockFailure simulates an infrastructure error)
			if s.WorkerBehavior == nil && s.MockFailure == "" {
				t.Error("scenario has nil WorkerBehavior")
			}

			// ExpectedStatus must be "idle" or "failed"
			if s.ExpectedStatus != "idle" && s.ExpectedStatus != "failed" {
				t.Errorf("ExpectedStatus must be 'idle' or 'failed', got %q", s.ExpectedStatus)
			}

			// MockCostUSD must be non-negative
			if s.MockCostUSD < 0 {
				t.Errorf("MockCostUSD must be >= 0, got %f", s.MockCostUSD)
			}

			// MockTurnCount must be non-negative
			if s.MockTurnCount < 0 {
				t.Errorf("MockTurnCount must be >= 0, got %d", s.MockTurnCount)
			}

			// Constraints.MaxCostUSD must be positive (or zero for blocked scenarios)
			if s.Constraints.MaxCostUSD < 0 {
				t.Errorf("Constraints.MaxCostUSD must be >= 0, got %f", s.Constraints.MaxCostUSD)
			}

			// Constraints.MaxDurationSec must be positive
			if s.Constraints.MaxDurationSec <= 0 {
				t.Errorf("Constraints.MaxDurationSec must be > 0, got %f", s.Constraints.MaxDurationSec)
			}

			// Constraints.MinCompletionRate must be in [0, 1]
			if s.Constraints.MinCompletionRate < 0 || s.Constraints.MinCompletionRate > 1 {
				t.Errorf("Constraints.MinCompletionRate must be in [0,1], got %f", s.Constraints.MinCompletionRate)
			}
		})
	}
}

func TestCoreScenariosSubset(t *testing.T) {
	core := CoreScenarios()
	all := AllScenarios()

	if len(core) != 6 {
		t.Errorf("CoreScenarios() should return 6, got %d", len(core))
	}
	if len(all) <= len(core) {
		t.Errorf("AllScenarios() (%d) should have more scenarios than CoreScenarios() (%d)", len(all), len(core))
	}

	allNames := make(map[string]bool)
	for _, s := range all {
		allNames[s.Name] = true
	}
	for _, s := range core {
		if !allNames[s.Name] {
			t.Errorf("core scenario %q not found in AllScenarios()", s.Name)
		}
	}
}

func TestScenariosByTag(t *testing.T) {
	multi := ScenariosByTag("multi-provider")
	if len(multi) < 4 {
		t.Errorf("expected at least 4 multi-provider scenarios, got %d", len(multi))
	}

	stress := ScenariosByTag("stress")
	if len(stress) < 5 {
		t.Errorf("expected at least 5 stress scenarios, got %d", len(stress))
	}

	cost := ScenariosByTag("cost")
	if len(cost) < 2 {
		t.Errorf("expected at least 2 cost scenarios, got %d", len(cost))
	}

	selfLearning := ScenariosByTag("self-learning")
	if len(selfLearning) < 5 {
		t.Errorf("expected at least 5 self-learning scenarios, got %d", len(selfLearning))
	}

	// Empty tag returns nothing
	empty := ScenariosByTag("nonexistent-tag")
	if len(empty) != 0 {
		t.Errorf("expected 0 scenarios for nonexistent tag, got %d", len(empty))
	}
}

func TestAllScenariosCount(t *testing.T) {
	scenarios := AllScenarios()
	// 6 original + 4 multi-provider + 5 stress + 2 cost + 5 self-learning + 3 integration = 25
	if len(scenarios) != 25 {
		t.Errorf("expected 25 scenarios, got %d", len(scenarios))
	}
}

func TestNewScenariosHaveTags(t *testing.T) {
	// All non-core scenarios should have at least one tag
	core := make(map[string]bool)
	for _, s := range CoreScenarios() {
		core[s.Name] = true
	}

	for _, s := range AllScenarios() {
		if core[s.Name] {
			continue // original scenarios don't require tags
		}
		if len(s.Tags) == 0 {
			t.Errorf("new scenario %q has no tags", s.Name)
		}
	}
}
