package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// AllScenarios returns all e2e scenarios including multi-provider, stress, and cost scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		// Original scenarios
		TrivialFix(),
		MultiFileRefactor(),
		TestAddition(),
		DocsUpdate(),
		FeatureAddition(),
		VerifyFailure(),

		// Multi-provider scenarios
		GeminiWorkerBasic(),
		CodexWorkerBasic(),
		MultiProviderTeam(),
		ProviderFailover(),

		// Stress/edge scenarios
		BudgetExhaustion(),
		TimeoutCascade(),
		CircuitBreakerTrip(),
		ConcurrentFileConflict(),
		CheckpointRecovery(),

		// Cost scenarios
		CostTrackingAccuracy(),
		FleetBudgetEnforcement(),
	}
}

// CoreScenarios returns only the original 6 scenarios for quick regression checks.
func CoreScenarios() []Scenario {
	return []Scenario{
		TrivialFix(),
		MultiFileRefactor(),
		TestAddition(),
		DocsUpdate(),
		FeatureAddition(),
		VerifyFailure(),
	}
}

// ScenariosByTag returns scenarios matching any of the given tags.
func ScenariosByTag(tags ...string) []Scenario {
	tagSet := make(map[string]bool, len(tags))
	for _, t := range tags {
		tagSet[t] = true
	}
	var result []Scenario
	for _, s := range AllScenarios() {
		for _, st := range s.Tags {
			if tagSet[st] {
				result = append(result, s)
				break
			}
		}
	}
	return result
}

// plannerJSON builds a valid planner JSON response for a single task.
func plannerJSON(title, prompt string) string {
	task := struct {
		Title  string `json:"title"`
		Prompt string `json:"prompt"`
	}{Title: title, Prompt: prompt}
	data, _ := json.Marshal(task)
	return string(data)
}

// setupRepo creates a temp git repo with .ralph/, .ralphrc, ROADMAP.md, and an initial commit.
func setupRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()

	// Base files
	base := map[string]string{
		"README.md":  "# test repo\n",
		".ralphrc":   "PROJECT_NAME=\"e2e-test\"\n",
		"ROADMAP.md": "# Roadmap\n\n## Phase 1\n\n### 1.1\n- [ ] 1.1.1 — Fix bug in main\n",
	}
	for k, v := range base {
		if err := os.WriteFile(filepath.Join(dir, k), []byte(v), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(dir, ".ralph"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Extra files
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gitRun(t, dir, "init")
	gitRun(t, dir, "config", "user.email", "test@e2e.test")
	gitRun(t, dir, "config", "user.name", "E2E Test")
	gitRun(t, dir, "config", "commit.gpgsign", "false")
	gitRun(t, dir, "add", ".")
	gitRun(t, dir, "commit", "-m", "initial")
	return dir
}

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// TrivialFix: single file edit, simple verify.
func TrivialFix() Scenario {
	return Scenario{
		Name:     "trivial-fix",
		Category: "bug_fix",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
			})
		},
		PlannerResponse: plannerJSON("Fix greeting typo", "Change the greeting in main.go from 'hello' to 'hello world'"),
		WorkerBehavior: func(worktree string) error {
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"hello world\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.15,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.9},
	}
}

// MultiFileRefactor: 3 files changed, rename + update refs.
func MultiFileRefactor() Scenario {
	return Scenario{
		Name:     "multi-file-refactor",
		Category: "refactor",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"pkg/config/config.go": "package config\n\nvar OldName = \"value\"\n",
				"pkg/config/loader.go": "package config\n\nfunc Load() string { return OldName }\n",
				"cmd/app/main.go":      "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"app\") }\n",
			})
		},
		PlannerResponse: plannerJSON("Refactor config naming", "Rename OldName to NewName in config package and update all references"),
		WorkerBehavior: func(worktree string) error {
			files := map[string]string{
				"pkg/config/config.go": "package config\n\nvar NewName = \"value\"\n",
				"pkg/config/loader.go": "package config\n\nfunc Load() string { return NewName }\n",
				"cmd/app/main.go":      "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"app v2\") }\n",
			}
			for name, content := range files {
				p := filepath.Join(worktree, name)
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
		VerifyCommands: []string{"grep -q NewName pkg/config/config.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.35,
		MockTurnCount:  5,
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 60, MinCompletionRate: 0.8},
	}
}

// TestAddition: add test file, verify passes.
func TestAddition() Scenario {
	return Scenario{
		Name:     "test-addition",
		Category: "test",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"math.go": "package main\n\nfunc Add(a, b int) int { return a + b }\n",
			})
		},
		PlannerResponse: plannerJSON("Add unit tests for math", "Create math_test.go with tests for the Add function"),
		WorkerBehavior: func(worktree string) error {
			content := `package main

import "testing"

func TestAdd(t *testing.T) {
	if Add(2, 3) != 5 {
		t.Fatal("expected 5")
	}
}
`
			return os.WriteFile(filepath.Join(worktree, "math_test.go"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"test -f math_test.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.20,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.5, MaxDurationSec: 45, MinCompletionRate: 0.85},
	}
}

// DocsUpdate: update README.
func DocsUpdate() Scenario {
	return Scenario{
		Name:     "docs-update",
		Category: "docs",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, nil)
		},
		PlannerResponse: plannerJSON("Update documentation", "Add installation section to README.md"),
		WorkerBehavior: func(worktree string) error {
			content := "# test repo\n\n## Installation\n\n```bash\ngo install .\n```\n"
			return os.WriteFile(filepath.Join(worktree, "README.md"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"grep -q Installation README.md"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.10,
		MockTurnCount:  2,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 20, MinCompletionRate: 0.95},
	}
}

// FeatureAddition: new file + integration.
func FeatureAddition() Scenario {
	return Scenario{
		Name:     "feature-addition",
		Category: "feature",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Add health endpoint", "Create a health.go file with a HealthCheck function and update main.go to call it"),
		WorkerBehavior: func(worktree string) error {
			if err := os.WriteFile(filepath.Join(worktree, "health.go"),
				[]byte("package main\n\nfunc HealthCheck() string { return \"ok\" }\n"), 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(HealthCheck())\n}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f health.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.40,
		MockTurnCount:  6,
		Constraints:    Constraints{MaxCostUSD: 2.5, MaxDurationSec: 60, MinCompletionRate: 0.8},
	}
}

// VerifyFailure: worker output that fails verification.
func VerifyFailure() Scenario {
	return Scenario{
		Name:     "verify-failure",
		Category: "bug_fix",
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Fix critical bug", "Fix the validation logic in validator.go"),
		WorkerBehavior: func(worktree string) error {
			// Worker creates the wrong file — verification will fail
			return os.WriteFile(filepath.Join(worktree, "wrong.go"),
				[]byte("package main\n\n// oops wrong file\n"), 0o644)
		},
		VerifyCommands: []string{"test -f validator.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.25,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.0},
	}
}

// ---------------------------------------------------------------------------
// Multi-provider scenarios
// ---------------------------------------------------------------------------

// GeminiWorkerBasic: single Gemini worker session doing basic code generation.
func GeminiWorkerBasic() Scenario {
	return Scenario{
		Name:     "gemini-worker-basic",
		Category: "feature",
		Provider: session.ProviderGemini,
		Tags:     []string{"multi-provider", "gemini"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {}\n",
			})
		},
		PlannerResponse: plannerJSON("Generate utility functions", "Create a utils.go file with string helper functions using Gemini"),
		WorkerBehavior: func(worktree string) error {
			content := "package main\n\nimport \"strings\"\n\n// Capitalize returns s with the first letter uppercased.\nfunc Capitalize(s string) string {\n\tif s == \"\" {\n\t\treturn s\n\t}\n\treturn strings.ToUpper(s[:1]) + s[1:]\n}\n"
			return os.WriteFile(filepath.Join(worktree, "utils.go"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"test -f utils.go", "grep -q Capitalize utils.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.08,
		MockTurnCount:  3,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 30, MinCompletionRate: 0.85},
	}
}

// CodexWorkerBasic: single Codex worker session doing focused refactoring.
func CodexWorkerBasic() Scenario {
	return Scenario{
		Name:     "codex-worker-basic",
		Category: "refactor",
		Provider: session.ProviderCodex,
		Tags:     []string{"multi-provider", "codex"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"handler.go": "package main\n\nimport \"fmt\"\n\nfunc handleRequest(method string, path string, body string) {\n\tfmt.Println(method, path, body)\n}\n",
			})
		},
		PlannerResponse: plannerJSON("Refactor handler to use struct", "Refactor handleRequest to accept a Request struct instead of individual parameters"),
		WorkerBehavior: func(worktree string) error {
			content := "package main\n\nimport \"fmt\"\n\n// Request holds HTTP request parameters.\ntype Request struct {\n\tMethod string\n\tPath   string\n\tBody   string\n}\n\nfunc handleRequest(r Request) {\n\tfmt.Println(r.Method, r.Path, r.Body)\n}\n"
			return os.WriteFile(filepath.Join(worktree, "handler.go"), []byte(content), 0o644)
		},
		VerifyCommands: []string{"grep -q 'type Request struct' handler.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.12,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 45, MinCompletionRate: 0.8},
	}
}

// MultiProviderTeam: Claude lead orchestrating Gemini + Codex workers.
func MultiProviderTeam() Scenario {
	return Scenario{
		Name:     "multi-provider-team",
		Category: "feature",
		Provider: session.ProviderClaude,
		Tags:     []string{"multi-provider", "team", "claude", "gemini", "codex"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go":       "package main\n\nfunc main() {}\n",
				"api/server.go": "package api\n\n// Server is a placeholder.\ntype Server struct{}\n",
			})
		},
		PlannerResponse: plannerJSON("Build API with multi-provider team",
			"Claude: architect API design. Gemini: generate endpoint handlers. Codex: refactor server struct with dependency injection."),
		WorkerBehavior: func(worktree string) error {
			// Simulates output from three providers working on different files
			files := map[string]string{
				"api/server.go": "package api\n\nimport \"net/http\"\n\n// Server handles HTTP requests with injected dependencies.\ntype Server struct {\n\tMux    *http.ServeMux\n\tAddr   string\n}\n\n// New creates a Server with defaults.\nfunc New(addr string) *Server {\n\treturn &Server{Mux: http.NewServeMux(), Addr: addr}\n}\n",
				"api/health.go": "package api\n\nimport \"net/http\"\n\n// HandleHealth returns 200 OK.\nfunc (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {\n\tw.WriteHeader(http.StatusOK)\n\tw.Write([]byte(`{\"status\":\"ok\"}`))\n}\n",
				"api/routes.go": "package api\n\n// RegisterRoutes wires all endpoint handlers.\nfunc (s *Server) RegisterRoutes() {\n\ts.Mux.HandleFunc(\"/health\", s.HandleHealth)\n}\n",
			}
			for name, content := range files {
				p := filepath.Join(worktree, name)
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					return err
				}
				if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
					return err
				}
			}
			return nil
		},
		VerifyCommands: []string{
			"test -f api/server.go",
			"test -f api/health.go",
			"test -f api/routes.go",
			"grep -q RegisterRoutes api/routes.go",
		},
		ExpectedStatus: "idle",
		MockCostUSD:    0.85,
		MockTurnCount:  12,
		Constraints:    Constraints{MaxCostUSD: 5.0, MaxDurationSec: 120, MinCompletionRate: 0.7},
	}
}

// ProviderFailover: primary provider fails, system should failover to secondary.
func ProviderFailover() Scenario {
	return Scenario{
		Name:     "provider-failover",
		Category: "bug_fix",
		Provider: session.ProviderGemini,
		Tags:     []string{"multi-provider", "failover", "stress"},
		RepoSetup: func(t *testing.T) string {
			return setupRepo(t, map[string]string{
				"main.go": "package main\n\nfunc main() {\n\tprintln(\"before fix\")\n}\n",
				// .ralphrc with failover config
				".ralphrc": "PROJECT_NAME=\"e2e-failover\"\nFAILOVER_PROVIDER=\"claude\"\n",
			})
		},
		PlannerResponse: plannerJSON("Apply hotfix with failover", "Fix the output message; if Gemini fails, Claude should take over"),
		WorkerBehavior: func(worktree string) error {
			// Simulates the secondary provider (Claude) completing the work after Gemini failure
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\nfunc main() {\n\tprintln(\"after fix\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'after fix' main.go"},
		ExpectedStatus: "idle",
		MockCostUSD:    0.30,
		MockTurnCount:  6,
		Constraints:    Constraints{MaxCostUSD: 2.0, MaxDurationSec: 60, MinCompletionRate: 0.7},
	}
}

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
		WorkerBehavior: func(worktree string) error {
			// Worker writes partial output before budget kills it
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\n// partial work before budget stop\nfunc main() {\n\tprintln(\"partial\")\n}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f main.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.55, // exceeds budget
		MockTurnCount:  8,
		Constraints:    Constraints{MaxCostUSD: 0.60, MaxDurationSec: 30, MinCompletionRate: 0.0},
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
		WorkerBehavior: func(worktree string) error {
			// Simulates partial/no output from timed-out workers
			return os.WriteFile(filepath.Join(worktree, "worker_a.go"),
				[]byte("package main\n\n// timeout: only A got partial output\nfunc WorkerA() {}\n"), 0o644)
			// worker_b.go and worker_c.go unchanged — simulating timeout
		},
		VerifyCommands: []string{
			"grep -q WorkerA worker_a.go",
			"grep -q 'placeholder B' worker_b.go",
			"grep -q 'placeholder C' worker_c.go",
		},
		ExpectedStatus: "failed",
		MockCostUSD:    0.45,
		MockTurnCount:  3,
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
		WorkerBehavior: func(worktree string) error {
			// Worker fails, pushing circuit breaker over threshold
			return fmt.Errorf("simulated API error: 429 rate limited")
		},
		VerifyCommands: []string{"test -f main.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.05,
		MockTurnCount:  1,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 15, MinCompletionRate: 0.0},
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
		WorkerBehavior: func(worktree string) error {
			// Worker attempts to modify a file already claimed by another session
			return os.WriteFile(filepath.Join(worktree, "shared.go"),
				[]byte("package main\n\nvar Shared = \"conflicting edit\"\n"), 0o644)
		},
		VerifyCommands: []string{"grep -q 'conflicting edit' shared.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.10,
		MockTurnCount:  2,
		Constraints:    Constraints{MaxCostUSD: 0.5, MaxDurationSec: 15, MinCompletionRate: 0.0},
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
		WorkerBehavior: func(worktree string) error {
			// Worker should not run — fleet budget enforcement blocks launch
			return os.WriteFile(filepath.Join(worktree, "main.go"),
				[]byte("package main\n\n// should not reach here\nfunc main() {}\n"), 0o644)
		},
		VerifyCommands: []string{"test -f main.go"},
		ExpectedStatus: "failed",
		MockCostUSD:    0.00, // no cost incurred — blocked before launch
		MockTurnCount:  0,
		Constraints:    Constraints{MaxCostUSD: 0.10, MaxDurationSec: 10, MinCompletionRate: 0.0},
	}
}
