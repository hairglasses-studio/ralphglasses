package e2e

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// Self-learning scenarios
// ---------------------------------------------------------------------------

// ReflexionRetry: verification fails, reflexion extracts a reflection,
// and the reflection store contains the failure analysis after the run.
func ReflexionRetry() Scenario {
	return Scenario{
		Name:     "reflexion-retry",
		Category: "self_learning",
		Tags:     []string{"self-learning", "reflexion"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Fix validation bug", "Add input validation to main.go"),
		WorkerBehavior: func(worktree string) error {
			// Worker creates the wrong file — verification will fail
			return os.WriteFile(filepath.Join(worktree, "wrong.go"),
				[]byte("package main\n\n// wrong file created\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q validation main.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.20,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.0},
		ManagerSetup: func(m *session.Manager) {
			rs := session.NewReflexionStore("")
			m.SetReflexionStore(rs)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableReflexion = true
		},
	}
}

// CascadeEscalation: cheap provider's output fails verification,
// cascade escalates to the expensive provider which succeeds.
func CascadeEscalation() Scenario {
	return Scenario{
		Name:     "cascade-escalation",
		Category: "self_learning",
		Tags:     []string{"self-learning", "cascade"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Add simple test", "Create a test file for main.go"),
		WorkerBehavior: func(worktree string) error {
			// The expensive provider succeeds
			return os.WriteFile(filepath.Join(worktree, "main_test.go"),
				[]byte("package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f main_test.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.10,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 1.5, MaxDurationSec: 45, MinCompletionRate: 0.8},
		ManagerSetup: func(m *session.Manager) {
			cfg := session.CascadeConfig{
				CheapProvider:       session.ProviderGemini,
				ExpensiveProvider:   session.ProviderClaude,
				ConfidenceThreshold: 0.99, // high threshold forces escalation
				MaxCheapBudgetUSD:   0.50,
				MaxCheapTurns:       5,
			}
			cr := session.NewCascadeRouter(cfg, nil, nil, "")
			m.SetCascadeRouter(cr)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableCascade = true
		},
	}
}

// CascadeCheapSuccess: cheap provider succeeds without escalation.
func CascadeCheapSuccess() Scenario {
	return Scenario{
		Name:     "cascade-cheap-success",
		Category: "self_learning",
		Tags:     []string{"self-learning", "cascade"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Add comment", "Add a package comment to main.go"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("// Package main is the entry point.\npackage main\n\nfunc main() {}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'Package main' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.05,
		MockTurnCount:  2,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 20, MinCompletionRate: 0.9},
		ManagerSetup: func(m *session.Manager) {
			cfg := session.CascadeConfig{
				CheapProvider:       session.ProviderGemini,
				ExpensiveProvider:   session.ProviderClaude,
				ConfidenceThreshold: 0.1, // low threshold allows cheap to succeed
				MaxCheapBudgetUSD:   0.50,
				MaxCheapTurns:       5,
			}
			cr := session.NewCascadeRouter(cfg, nil, nil, "")
			m.SetCascadeRouter(cr)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableCascade = true
		},
	}
}

// EpisodicInjection: episodic memory is pre-loaded with a successful episode,
// and StepLoop injects it into the planner/worker prompts.
func EpisodicInjection() Scenario {
	return Scenario{
		Name:     "episodic-injection",
		Category: "self_learning",
		Tags:     []string{"self-learning", "episodic"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Add logging", "Add structured logging to main.go"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nimport \"log\"\n\nfunc main() {\n\tlog.Println(\"started\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'log' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.15,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
		ManagerSetup: func(m *session.Manager) {
			em := session.NewEpisodicMemory("", 100, 0)
			// Pre-load a successful episode about logging
			em.RecordSuccess(session.JournalEntry{
				Provider:  "claude",
				Model:     "mock",
				TaskFocus: "Add structured logging",
				Worked:    []string{"Added slog-based logging"},
				Suggest:   []string{"Use log/slog for structured output"},
			})
			m.SetEpisodicMemory(em)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableEpisodicMemory = true
		},
	}
}

// CurriculumOrdering: multi-task scenario where curriculum sorter
// reorders tasks by difficulty (easy first).
func CurriculumOrdering() Scenario {
	return Scenario{
		Name:     "curriculum-ordering",
		Category: "self_learning",
		Tags:     []string{"self-learning", "curriculum"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":  "package main\n\nfunc main() {}\n",
				"utils.go": "package main\n\n// placeholder\n",
			})
		},
		PlannerResponse: plannerJSON("Add simple test", "Create a basic test file"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main_test.go"),
				[]byte("package main\n\nimport \"testing\"\n\nfunc TestMain(t *testing.T) {}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f main_test.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.10,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
		ManagerSetup: func(m *session.Manager) {
			cs := session.NewCurriculumSorter(nil, nil)
			m.SetCurriculumSorter(cs)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableCurriculum = true
		},
	}
}

// ---------------------------------------------------------------------------
// Cross-subsystem integration scenarios
// ---------------------------------------------------------------------------

// BanditCascadeIntegration: cascade router with bandit hooks + decision model.
// Verifies bandit receives rewards and decision model gets observations after iteration.
func BanditCascadeIntegration() Scenario {
	return Scenario{
		Name:     "bandit-cascade-integration",
		Category: "self_learning",
		Tags:     []string{"self-learning", "bandit", "cascade", "integration"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Add readme", "Create a README.md file"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "README.md"),
				[]byte("# Test Project\n"), 0o644)
		},
		VerifyCommands: []string{"test -f README.md"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.15,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 30, MinCompletionRate: 0.8},
		ManagerSetup: func(m *session.Manager) {
			// Wire cascade router
			cfg := session.DefaultCascadeConfig()
			cr := session.NewCascadeRouter(cfg, nil, nil, "")

			// Wire bandit hooks using Thompson Sampling
			tiers := session.DefaultModelTiers()
			arms := make([]bandit.Arm, len(tiers))
			for i, t := range tiers {
				arms[i] = bandit.Arm{
					ID:       t.Label,
					Provider: string(t.Provider),
					Model:    t.Model,
				}
			}
			ts := bandit.NewThompsonSampling(arms, 50)
			cr.SetBanditHooks(
				func() (string, string) {
					arm := ts.Select(nil)
					return arm.Provider, arm.Model
				},
				func(provider string, reward float64) {
					for _, a := range arms {
						if a.Provider == provider {
							ts.Update(bandit.Reward{ArmID: a.ID, Value: reward})
							return
						}
					}
				},
			)

			// Wire decision model
			dm := session.NewDecisionModel()
			cr.SetDecisionModel(dm)

			m.SetCascadeRouter(cr)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableCascade = true
		},
	}
}

// EpisodicWithEmbedder: episodic memory with trigram embedder produces better
// similarity results for semantically close prompts.
func EpisodicWithEmbedder() Scenario {
	return Scenario{
		Name:     "episodic-with-embedder",
		Category: "self_learning",
		Tags:     []string{"self-learning", "episodic", "embedder", "integration"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Fix linting errors", "Run linter and fix any warnings"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"clean\") }\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q clean main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.10,
		MockTurnCount:  2,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
		ManagerSetup: func(m *session.Manager) {
			em := session.NewEpisodicMemory("", 500, 5)
			em.SetEmbedder(session.NewTrigramEmbedder(128))

			// Pre-seed 3 episodes to exercise retrieval.
			for _, title := range []string{
				"Fix lint warnings in auth module",
				"Add unit tests for parser",
				"Refactor database connection pooling",
			} {
				em.RecordSuccess(session.JournalEntry{
					SessionID:  "seed-" + title[:4],
					Provider:   "claude",
					TaskFocus:  title,
					ExitReason: "completed",
					Worked:     []string{title},
				})
			}

			m.SetEpisodicMemory(em)
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableEpisodicMemory = true
		},
	}
}

// FullSubsystemPipeline: all self-learning subsystems wired together.
// Exercises reflexion -> episodic -> cascade -> curriculum -> cost recording in a single run.
func FullSubsystemPipeline() Scenario {
	return Scenario{
		Name:     "full-subsystem-pipeline",
		Category: "self_learning",
		Tags:     []string{"self-learning", "integration", "pipeline"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":  "package main\n\nfunc main() {}\n",
				"utils.go": "package main\n\n// TODO: add helpers\n",
			})
		},
		PlannerResponse: plannerJSON("Add utility functions", "Implement string helpers in utils.go"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "utils.go"),
				[]byte("package main\n\nimport \"strings\"\n\nfunc toUpper(s string) string { return strings.ToUpper(s) }\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q toUpper utils.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.25,
		MockTurnCount:  5,
		Constraints:    Constraints{MaxCostUSD: 3.0, MaxDurationSec: 60, MinCompletionRate: 0.8},
		ManagerSetup: func(m *session.Manager) {
			// WS1: Reflexion
			m.SetReflexionStore(session.NewReflexionStore(""))

			// WS2: Episodic + embedder
			em := session.NewEpisodicMemory("", 500, 5)
			em.SetEmbedder(session.NewTrigramEmbedder(128))
			m.SetEpisodicMemory(em)

			// WS3: Cascade + bandit + decision model
			cfg := session.DefaultCascadeConfig()
			cr := session.NewCascadeRouter(cfg, nil, nil, "")
			dm := session.NewDecisionModel()
			cr.SetDecisionModel(dm)

			tiers := session.DefaultModelTiers()
			arms := make([]bandit.Arm, len(tiers))
			for i, t := range tiers {
				arms[i] = bandit.Arm{
					ID:       t.Label,
					Provider: string(t.Provider),
					Model:    t.Model,
				}
			}
			ts := bandit.NewThompsonSampling(arms, 50)
			cr.SetBanditHooks(
				func() (string, string) {
					arm := ts.Select(nil)
					return arm.Provider, arm.Model
				},
				func(provider string, reward float64) {
					for _, a := range arms {
						if a.Provider == provider {
							ts.Update(bandit.Reward{ArmID: a.ID, Value: reward})
							return
						}
					}
				},
			)
			m.SetCascadeRouter(cr)

			// WS5: Curriculum
			m.SetCurriculumSorter(session.NewCurriculumSorter(nil, em))
		},
		ProfilePatch: func(p *session.LoopProfile) {
			p.EnableReflexion = true
			p.EnableEpisodicMemory = true
			p.EnableCascade = true
			p.EnableCurriculum = true
		},
	}
}
