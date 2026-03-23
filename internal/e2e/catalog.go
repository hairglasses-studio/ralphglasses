package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// AllScenarios returns the 6 representative e2e scenarios.
func AllScenarios() []Scenario {
	return []Scenario{
		TrivialFix(),
		MultiFileRefactor(),
		TestAddition(),
		DocsUpdate(),
		FeatureAddition(),
		VerifyFailure(),
	}
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
		VerifyCommands: []string{fmt.Sprintf("test -f validator.go")},
		ExpectedStatus: "failed",
		MockCostUSD:    0.25,
		MockTurnCount:  4,
		Constraints:    Constraints{MaxCostUSD: 1.0, MaxDurationSec: 30, MinCompletionRate: 0.0},
	}
}
