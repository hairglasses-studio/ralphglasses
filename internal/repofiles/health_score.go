package repofiles

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// HealthScore is the composite health assessment for a repository.
type HealthScore struct {
	Overall    float64                   `json:"overall"`    // 0.0-1.0
	Dimensions map[string]DimensionScore `json:"dimensions"` // keyed by dimension name
	Issues     []HealthIssue             `json:"issues"`
	Timestamp  time.Time                 `json:"timestamp"`
}

// DimensionScore is a single health dimension measurement.
type DimensionScore struct {
	Name    string  `json:"name"`
	Score   float64 `json:"score"`   // 0.0-1.0
	Weight  float64 `json:"weight"`  // contribution to overall
	Details string  `json:"details"` // human-readable explanation
}

// HealthIssue is a detected problem with a suggested fix.
type HealthIssue struct {
	Severity  string `json:"severity"`  // info, warn, error
	Dimension string `json:"dimension"` // which dimension this belongs to
	Message   string `json:"message"`
	Fix       string `json:"fix"`
}

// dimensionWeights defines the weighted contribution of each dimension
// to the overall score.
var dimensionWeights = map[string]float64{
	"config":    0.3,
	"git":       0.2,
	"tests":     0.2,
	"docs":      0.1,
	"deps":      0.1,
	"structure": 0.1,
}

// ScoreRepo computes a comprehensive health score for the repository at repoPath.
func ScoreRepo(ctx context.Context, repoPath string) (*HealthScore, error) {
	info, err := os.Stat(repoPath)
	if err != nil {
		return nil, fmt.Errorf("repo path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("repo path is not a directory: %s", repoPath)
	}

	hs := &HealthScore{
		Dimensions: make(map[string]DimensionScore, len(dimensionWeights)),
		Timestamp:  time.Now(),
	}

	scorers := []struct {
		name string
		fn   func(ctx context.Context, repoPath string, hs *HealthScore) DimensionScore
	}{
		{"config", scoreConfig},
		{"git", scoreGit},
		{"tests", scoreTests},
		{"docs", scoreDocs},
		{"deps", scoreDeps},
		{"structure", scoreStructure},
	}

	for _, s := range scorers {
		dim := s.fn(ctx, repoPath, hs)
		dim.Name = s.name
		dim.Weight = dimensionWeights[s.name]
		hs.Dimensions[s.name] = dim
	}

	// Compute weighted overall score
	var total float64
	for _, dim := range hs.Dimensions {
		total += dim.Score * dim.Weight
	}
	hs.Overall = total

	// Ensure Issues is never nil for clean JSON
	if hs.Issues == nil {
		hs.Issues = []HealthIssue{}
	}

	return hs, nil
}

// scoreConfig checks .ralphrc completeness: provider, model, budget, tool permissions.
func scoreConfig(_ context.Context, repoPath string, hs *HealthScore) DimensionScore {
	rcPath := filepath.Join(repoPath, ".ralphrc")
	values := readKVFile(rcPath)

	if len(values) == 0 {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "error",
			Dimension: "config",
			Message:   ".ralphrc missing or empty",
			Fix:       "Run ralphglasses_repo_scaffold to create default config",
		})
		return DimensionScore{Details: "no .ralphrc found"}
	}

	checks := 0
	total := 0

	// Provider / model
	total++
	if _, ok := values["PRIMARY_MODEL"]; ok {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "warn",
			Dimension: "config",
			Message:   "no PRIMARY_MODEL configured",
			Fix:       "Add PRIMARY_MODEL=\"sonnet\" to .ralphrc",
		})
	}

	// Budget
	total++
	if _, ok := values["RALPH_SESSION_BUDGET"]; ok {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "warn",
			Dimension: "config",
			Message:   "no RALPH_SESSION_BUDGET set",
			Fix:       "Add RALPH_SESSION_BUDGET=100 to prevent runaway spending",
		})
	}

	// Circuit breaker
	total++
	cbKeys := []string{"CB_NO_PROGRESS_THRESHOLD", "CB_SAME_ERROR_THRESHOLD", "CB_COOLDOWN_MINUTES"}
	cbPresent := 0
	for _, k := range cbKeys {
		if _, ok := values[k]; ok {
			cbPresent++
		}
	}
	if cbPresent == len(cbKeys) {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "info",
			Dimension: "config",
			Message:   fmt.Sprintf("circuit breaker partially configured (%d/%d keys)", cbPresent, len(cbKeys)),
			Fix:       "Add CB_NO_PROGRESS_THRESHOLD, CB_SAME_ERROR_THRESHOLD, CB_COOLDOWN_MINUTES",
		})
	}

	// Tool permissions / ALLOWED_TOOLS
	total++
	if _, ok := values["ALLOWED_TOOLS"]; ok {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "warn",
			Dimension: "config",
			Message:   "no ALLOWED_TOOLS configured",
			Fix:       "Add ALLOWED_TOOLS to constrain what ralph can execute",
		})
	}

	// Quality gates
	total++
	projectType := detectProjectType(repoPath)
	qgKeys := qualityGateKeys(projectType)
	qgPresent := 0
	for _, k := range qgKeys {
		if _, ok := values[k]; ok {
			qgPresent++
		}
	}
	if qgPresent == len(qgKeys) {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "info",
			Dimension: "config",
			Message:   fmt.Sprintf("quality gates partially configured (%d/%d)", qgPresent, len(qgKeys)),
			Fix:       fmt.Sprintf("Add quality gate keys for %s project", projectType),
		})
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d config checks passed", checks, total),
	}
}

// scoreGit checks working tree cleanliness, untracked files, and branch status.
func scoreGit(ctx context.Context, repoPath string, hs *HealthScore) DimensionScore {
	gitDir := filepath.Join(repoPath, ".git")
	if _, err := os.Stat(gitDir); err != nil {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "error",
			Dimension: "git",
			Message:   "not a git repository",
			Fix:       "Run git init to initialize the repository",
		})
		return DimensionScore{Details: "not a git repo"}
	}

	checks := 0
	total := 0

	// Clean working tree (no modified files)
	total++
	statusOut := runGitCmd(ctx, repoPath, "status", "--porcelain")
	if statusOut == "" {
		checks++
	} else {
		lines := strings.Split(strings.TrimSpace(statusOut), "\n")
		modified := 0
		untracked := 0
		for _, line := range lines {
			if strings.HasPrefix(line, "??") {
				untracked++
			} else {
				modified++
			}
		}
		if modified > 0 {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "warn",
				Dimension: "git",
				Message:   fmt.Sprintf("%d modified/staged files in working tree", modified),
				Fix:       "Commit or stash uncommitted changes",
			})
		}
		if untracked > 0 {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "git",
				Message:   fmt.Sprintf("%d untracked files", untracked),
				Fix:       "Add untracked files to .gitignore or commit them",
			})
		}
	}

	// No untracked files
	total++
	if !strings.Contains(statusOut, "??") {
		checks++
	}

	// On main/master branch or has recent commits
	total++
	branch := strings.TrimSpace(runGitCmd(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD"))
	if branch == "main" || branch == "master" {
		checks++
	} else if branch != "" {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "info",
			Dimension: "git",
			Message:   fmt.Sprintf("on branch %q, not main/master", branch),
			Fix:       "Consider merging to main branch",
		})
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d git checks passed", checks, total),
	}
}

// scoreTests checks for test files and basic test health.
func scoreTests(ctx context.Context, repoPath string, hs *HealthScore) DimensionScore {
	checks := 0
	total := 0

	// Has test files
	total++
	hasTests := false
	testPatterns := []string{"*_test.go", "**/*_test.go", "test_*.py", "**/test_*.py", "*.test.js", "**/*.test.js", "*.test.ts", "**/*.test.ts"}
	for _, pattern := range testPatterns {
		matches, _ := filepath.Glob(filepath.Join(repoPath, pattern))
		if len(matches) > 0 {
			hasTests = true
			break
		}
	}
	// Also check nested directories for Go test files
	if !hasTests {
		_ = filepath.WalkDir(repoPath, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				// Skip hidden dirs and vendor
				base := d.Name()
				if strings.HasPrefix(base, ".") || base == "vendor" || base == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(d.Name(), "_test.go") ||
				strings.HasPrefix(d.Name(), "test_") ||
				strings.Contains(d.Name(), ".test.") ||
				strings.Contains(d.Name(), ".spec.") {
				hasTests = true
				return filepath.SkipAll
			}
			return nil
		})
	}

	if hasTests {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "warn",
			Dimension: "tests",
			Message:   "no test files found",
			Fix:       "Add test files for your project",
		})
	}

	// Tests pass (only check if we can identify the project type and run quickly)
	total++
	projectType := detectProjectType(repoPath)
	if hasTests && projectType == "go" {
		testOut := runCmdWithTimeout(ctx, repoPath, 30*time.Second, "go", "test", "./...")
		if testOut.err == nil {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "error",
				Dimension: "tests",
				Message:   "tests are failing",
				Fix:       "Run tests locally and fix failures",
			})
		}
	} else if hasTests {
		// For non-Go projects, give partial credit if tests exist
		checks++
	}

	// Coverage > 60% (only for Go projects with test files)
	total++
	if hasTests && projectType == "go" {
		coverOut := runCmdWithTimeout(ctx, repoPath, 30*time.Second, "go", "test", "-cover", "./...")
		if coverOut.err == nil {
			output := coverOut.stdout
			// Parse "coverage: XX.X% of statements" lines
			coverageOK := true
			lines := strings.SplitSeq(output, "\n")
			for line := range lines {
				if idx := strings.Index(line, "coverage:"); idx >= 0 {
					var pct float64
					if _, err := fmt.Sscanf(line[idx:], "coverage: %f%%", &pct); err == nil {
						if pct < 60.0 {
							coverageOK = false
						}
					}
				}
			}
			if coverageOK {
				checks++
			} else {
				hs.Issues = append(hs.Issues, HealthIssue{
					Severity:  "info",
					Dimension: "tests",
					Message:   "some packages have test coverage below 60%",
					Fix:       "Add more tests to improve coverage",
				})
			}
		}
	} else {
		// Non-Go or no tests: give partial credit if tests exist
		if hasTests {
			checks++
		}
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d test checks passed", checks, total),
	}
}

// scoreDocs checks for README, CLAUDE.md, or similar documentation.
func scoreDocs(_ context.Context, repoPath string, hs *HealthScore) DimensionScore {
	checks := 0
	total := 0

	// Has README
	total++
	readmeNames := []string{"README.md", "README", "README.txt", "readme.md"}
	hasReadme := false
	for _, name := range readmeNames {
		if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
			hasReadme = true
			break
		}
	}
	if hasReadme {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "warn",
			Dimension: "docs",
			Message:   "no README found",
			Fix:       "Add a README.md with project description and setup instructions",
		})
	}

	// Has CLAUDE.md or similar AI-config file
	total++
	aiDocs := []string{"CLAUDE.md", "AGENTS.md", "GEMINI.md", "CODEX.md"}
	hasAIDoc := false
	for _, name := range aiDocs {
		if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
			hasAIDoc = true
			break
		}
	}
	if hasAIDoc {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "info",
			Dimension: "docs",
			Message:   "no CLAUDE.md or similar AI agent config",
			Fix:       "Add CLAUDE.md with project-specific instructions for AI agents",
		})
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d doc checks passed", checks, total),
	}
}

// scoreDeps checks dependency health: go.mod tidy, go vuln check.
func scoreDeps(ctx context.Context, repoPath string, hs *HealthScore) DimensionScore {
	projectType := detectProjectType(repoPath)

	checks := 0
	total := 0

	if projectType == "go" {
		// go.mod exists and is tidy
		total++
		goMod := filepath.Join(repoPath, "go.mod")
		if _, err := os.Stat(goMod); err == nil {
			// Check if go mod tidy would change anything
			tidyOut := runCmdWithTimeout(ctx, repoPath, 30*time.Second, "go", "mod", "verify")
			if tidyOut.err == nil {
				checks++
			} else {
				hs.Issues = append(hs.Issues, HealthIssue{
					Severity:  "warn",
					Dimension: "deps",
					Message:   "go mod verify failed",
					Fix:       "Run go mod tidy to clean up dependencies",
				})
			}
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "error",
				Dimension: "deps",
				Message:   "go.mod not found for Go project",
				Fix:       "Run go mod init to create go.mod",
			})
		}

		// No vulnerable deps (govulncheck)
		total++
		vulnOut := runCmdWithTimeout(ctx, repoPath, 30*time.Second, "govulncheck", "./...")
		if vulnOut.err == nil {
			checks++
		} else {
			// govulncheck may not be installed; give benefit of doubt
			if isCommandNotFound(vulnOut.err) {
				checks++ // Don't penalize for missing tool
				hs.Issues = append(hs.Issues, HealthIssue{
					Severity:  "info",
					Dimension: "deps",
					Message:   "govulncheck not installed, skipping vulnerability scan",
					Fix:       "Install with: go install golang.org/x/vuln/cmd/govulncheck@latest",
				})
			} else {
				hs.Issues = append(hs.Issues, HealthIssue{
					Severity:  "warn",
					Dimension: "deps",
					Message:   "vulnerability scan found issues",
					Fix:       "Run govulncheck ./... and update affected dependencies",
				})
			}
		}
	} else if projectType == "node" {
		total++
		pkgJSON := filepath.Join(repoPath, "package.json")
		if _, err := os.Stat(pkgJSON); err == nil {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "error",
				Dimension: "deps",
				Message:   "package.json not found for Node project",
				Fix:       "Run npm init to create package.json",
			})
		}
		total++
		lockFile := filepath.Join(repoPath, "package-lock.json")
		if _, err := os.Stat(lockFile); err == nil {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "deps",
				Message:   "no package-lock.json found",
				Fix:       "Run npm install to generate lockfile",
			})
		}
	} else {
		// Unknown project type: check for any manifest
		total++
		manifests := []string{"go.mod", "package.json", "Cargo.toml", "pyproject.toml", "requirements.txt"}
		for _, m := range manifests {
			if _, err := os.Stat(filepath.Join(repoPath, m)); err == nil {
				checks++
				break
			}
		}
		if checks == 0 {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "deps",
				Message:   "no dependency manifest found",
				Fix:       "Add a dependency manifest (go.mod, package.json, etc.)",
			})
		}
	}

	if total == 0 {
		return DimensionScore{Score: 1.0, Details: "no dependency checks applicable"}
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d dependency checks passed", checks, total),
	}
}

// scoreStructure checks for standard project layout (cmd/ or main.go, etc.).
func scoreStructure(_ context.Context, repoPath string, hs *HealthScore) DimensionScore {
	projectType := detectProjectType(repoPath)

	checks := 0
	total := 0

	if projectType == "go" {
		// Has cmd/ or main.go
		total++
		hasEntry := false
		if _, err := os.Stat(filepath.Join(repoPath, "cmd")); err == nil {
			hasEntry = true
		}
		if _, err := os.Stat(filepath.Join(repoPath, "main.go")); err == nil {
			hasEntry = true
		}
		if hasEntry {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "structure",
				Message:   "no cmd/ directory or main.go found",
				Fix:       "Add cmd/ directory or main.go as entry point",
			})
		}

		// Has internal/ or pkg/
		total++
		hasLib := false
		if _, err := os.Stat(filepath.Join(repoPath, "internal")); err == nil {
			hasLib = true
		}
		if _, err := os.Stat(filepath.Join(repoPath, "pkg")); err == nil {
			hasLib = true
		}
		if hasLib {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "structure",
				Message:   "no internal/ or pkg/ directory",
				Fix:       "Organize code into internal/ or pkg/ packages",
			})
		}
	} else if projectType == "node" {
		// Has src/ or index.js
		total++
		hasEntry := false
		for _, name := range []string{"src", "index.js", "index.ts", "main.js", "main.ts"} {
			if _, err := os.Stat(filepath.Join(repoPath, name)); err == nil {
				hasEntry = true
				break
			}
		}
		if hasEntry {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "info",
				Dimension: "structure",
				Message:   "no src/ directory or entry point found",
				Fix:       "Add src/ directory or index.js as entry point",
			})
		}
	} else {
		// Generic: check for some source files
		total++
		hasSrc := false
		entries, _ := os.ReadDir(repoPath)
		for _, e := range entries {
			if !e.IsDir() {
				ext := filepath.Ext(e.Name())
				if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".rs" || ext == ".java" {
					hasSrc = true
					break
				}
			}
		}
		if !hasSrc {
			// Check subdirectories
			for _, e := range entries {
				if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
					subEntries, _ := os.ReadDir(filepath.Join(repoPath, e.Name()))
					for _, se := range subEntries {
						ext := filepath.Ext(se.Name())
						if ext == ".go" || ext == ".py" || ext == ".js" || ext == ".ts" || ext == ".rs" || ext == ".java" {
							hasSrc = true
							break
						}
					}
					if hasSrc {
						break
					}
				}
			}
		}
		if hasSrc {
			checks++
		} else {
			hs.Issues = append(hs.Issues, HealthIssue{
				Severity:  "warn",
				Dimension: "structure",
				Message:   "no source code files found",
				Fix:       "Add source code to the repository",
			})
		}
	}

	// .gitignore exists
	total++
	if _, err := os.Stat(filepath.Join(repoPath, ".gitignore")); err == nil {
		checks++
	} else {
		hs.Issues = append(hs.Issues, HealthIssue{
			Severity:  "info",
			Dimension: "structure",
			Message:   "no .gitignore file",
			Fix:       "Add .gitignore to exclude build artifacts and sensitive files",
		})
	}

	if total == 0 {
		return DimensionScore{Score: 1.0, Details: "no structure checks applicable"}
	}

	score := float64(checks) / float64(total)
	return DimensionScore{
		Score:   score,
		Details: fmt.Sprintf("%d/%d structure checks passed", checks, total),
	}
}

// cmdResult holds stdout and error from a command execution.
type cmdResult struct {
	stdout string
	err    error
}

// runCmdWithTimeout executes a command with a timeout, returning stdout and any error.
func runCmdWithTimeout(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) cmdResult {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return cmdResult{stdout: string(out), err: err}
}

// runGitCmd runs a git command in the given directory and returns stdout.
func runGitCmd(ctx context.Context, dir string, args ...string) string {
	result := runCmdWithTimeout(ctx, dir, 10*time.Second, "git", args...)
	return result.stdout
}

// isCommandNotFound returns true if the error indicates the command was not found.
func isCommandNotFound(err error) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if ok := false; !ok {
		// Check if exec.ErrNotFound or "not found" in error message
		if err == exec.ErrNotFound {
			return true
		}
		msg := err.Error()
		return strings.Contains(msg, "not found") || strings.Contains(msg, "no such file")
	}
	_ = exitErr
	return false
}
