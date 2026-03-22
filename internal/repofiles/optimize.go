package repofiles

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OptimizeResult describes what was analyzed and proposed.
type OptimizeResult struct {
	RepoPath      string           `json:"repo_path"`
	ProjectType   string           `json:"project_type"`
	Issues        []OptimizeIssue  `json:"issues"`
	Optimizations []OptimizeAction `json:"optimizations"`
	Applied       int              `json:"applied"`
}

// OptimizeIssue is a detected configuration problem.
type OptimizeIssue struct {
	File       string `json:"file"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"` // info, warning, error
	Suggestion string `json:"suggestion"`
}

// OptimizeAction is an optimization that was applied.
type OptimizeAction struct {
	File    string `json:"file"`
	Action  string `json:"action"`
	Applied bool   `json:"applied"`
}

// OptimizeOptions controls optimization behavior.
type OptimizeOptions struct {
	DryRun bool   // Report issues but don't modify files
	Focus  string // "config", "prompt", "plan", "all"
}

// Optimize analyzes and optionally fixes ralph config files.
func Optimize(repoPath string, opts OptimizeOptions) (*OptimizeResult, error) {
	if opts.Focus == "" {
		opts.Focus = "all"
	}

	projectType := detectProjectType(repoPath)
	result := &OptimizeResult{
		RepoPath:    repoPath,
		ProjectType: projectType,
	}

	if opts.Focus == "all" || opts.Focus == "config" {
		optimizeRalphRC(repoPath, projectType, opts, result)
	}

	if opts.Focus == "all" || opts.Focus == "prompt" {
		optimizePrompt(repoPath, projectType, opts, result)
	}

	if opts.Focus == "all" || opts.Focus == "plan" {
		optimizeFixPlan(repoPath, opts, result)
	}

	// Check for missing ALLOWED_TOOLS
	optimizeAllowedTools(repoPath, projectType, opts, result)

	return result, nil
}

func optimizeRalphRC(repoPath, projectType string, opts OptimizeOptions, result *OptimizeResult) {
	rcPath := filepath.Join(repoPath, ".ralphrc")
	values := readKVFile(rcPath)

	if len(values) == 0 {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralphrc",
			Issue:      "File missing or empty",
			Severity:   "error",
			Suggestion: "Run ralphglasses_repo_scaffold to create default config",
		})
		return
	}

	// Check PROJECT_TYPE matches detected
	if pt, ok := values["PROJECT_TYPE"]; ok && pt != projectType {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralphrc",
			Issue:      fmt.Sprintf("PROJECT_TYPE=%q doesn't match detected type %q", pt, projectType),
			Severity:   "warning",
			Suggestion: fmt.Sprintf("Set PROJECT_TYPE=%q", projectType),
		})
	}

	// Check quality gates match project type
	qgKeys := qualityGateKeys(projectType)
	for _, key := range qgKeys {
		if _, ok := values[key]; !ok {
			result.Issues = append(result.Issues, OptimizeIssue{
				File:       ".ralphrc",
				Issue:      fmt.Sprintf("Missing quality gate: %s", key),
				Severity:   "info",
				Suggestion: fmt.Sprintf("Add %s=true to enable %s quality gate", key, key),
			})
		}
	}

	// Check budget is set
	if _, ok := values["RALPH_SESSION_BUDGET"]; !ok {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralphrc",
			Issue:      "No RALPH_SESSION_BUDGET set",
			Severity:   "warning",
			Suggestion: "Add RALPH_SESSION_BUDGET=100 to prevent runaway spending",
		})
	}

	// Check circuit breaker thresholds
	for _, key := range []string{"CB_NO_PROGRESS_THRESHOLD", "CB_SAME_ERROR_THRESHOLD", "CB_COOLDOWN_MINUTES"} {
		if _, ok := values[key]; !ok {
			result.Issues = append(result.Issues, OptimizeIssue{
				File:       ".ralphrc",
				Issue:      fmt.Sprintf("Missing circuit breaker config: %s", key),
				Severity:   "info",
				Suggestion: "Circuit breaker prevents infinite loops of failure",
			})
		}
	}

	// Check fast mode config
	if _, ok := values["FAST_MODE_ENABLED"]; !ok {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralphrc",
			Issue:      "Fast mode not configured",
			Severity:   "info",
			Suggestion: "Add FAST_MODE_ENABLED=true for phase-aware model switching",
		})
	}
}

func optimizePrompt(repoPath, projectType string, opts OptimizeOptions, result *OptimizeResult) {
	promptPath := filepath.Join(repoPath, ".ralph", "PROMPT.md")
	content, err := os.ReadFile(promptPath)
	if err != nil {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/PROMPT.md",
			Issue:      "File missing",
			Severity:   "error",
			Suggestion: "Run ralphglasses_repo_scaffold to create default prompt",
		})
		return
	}

	text := string(content)

	// Check for status reporting block
	if !strings.Contains(text, "RALPH_STATUS") {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/PROMPT.md",
			Issue:      "Missing RALPH_STATUS reporting block",
			Severity:   "warning",
			Suggestion: "Add status reporting template so ralph loop can parse progress",
		})
	}

	// Check for protected files section
	if !strings.Contains(text, "Protected Files") && !strings.Contains(text, "DO NOT MODIFY") {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/PROMPT.md",
			Issue:      "Missing protected files section",
			Severity:   "warning",
			Suggestion: "Add protected files list to prevent ralph from deleting its own config",
		})
	}

	// Check for project-specific context
	if !strings.Contains(strings.ToLower(text), projectType) {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/PROMPT.md",
			Issue:      fmt.Sprintf("No mention of project type %q", projectType),
			Severity:   "info",
			Suggestion: "Add project-specific build/test instructions",
		})
	}

	// Check for fix_plan reference
	if !strings.Contains(text, "fix_plan") {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/PROMPT.md",
			Issue:      "No reference to fix_plan.md",
			Severity:   "info",
			Suggestion: "Reference fix_plan.md so ralph knows where to find tasks",
		})
	}
}

func optimizeFixPlan(repoPath string, opts OptimizeOptions, result *OptimizeResult) {
	planPath := filepath.Join(repoPath, ".ralph", "fix_plan.md")
	content, err := os.ReadFile(planPath)
	if err != nil {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/fix_plan.md",
			Issue:      "File missing",
			Severity:   "error",
			Suggestion: "Run ralphglasses_repo_scaffold to create default fix plan",
		})
		return
	}

	text := string(content)

	// Count tasks
	todoCount := strings.Count(text, "- [ ]")
	doneCount := strings.Count(text, "- [x]")

	if todoCount == 0 {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/fix_plan.md",
			Issue:      "No open tasks",
			Severity:   "warning",
			Suggestion: "Add tasks from ROADMAP.md using ralphglasses_roadmap_export",
		})
	}

	if doneCount > 0 && todoCount == 0 {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/fix_plan.md",
			Issue:      "All tasks completed — ralph will have nothing to do",
			Severity:   "warning",
			Suggestion: "Export next batch of tasks from roadmap",
		})
	}

	// Check if fix_plan references roadmap
	if !strings.Contains(strings.ToLower(text), "roadmap") {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralph/fix_plan.md",
			Issue:      "Fix plan doesn't reference ROADMAP.md",
			Severity:   "info",
			Suggestion: "Link tasks back to roadmap phases for traceability",
		})
	}
}

func optimizeAllowedTools(repoPath, projectType string, opts OptimizeOptions, result *OptimizeResult) {
	rcPath := filepath.Join(repoPath, ".ralphrc")
	values := readKVFile(rcPath)

	tools, ok := values["ALLOWED_TOOLS"]
	if !ok {
		result.Issues = append(result.Issues, OptimizeIssue{
			File:       ".ralphrc",
			Issue:      "No ALLOWED_TOOLS configured",
			Severity:   "warning",
			Suggestion: "Add ALLOWED_TOOLS to constrain what ralph can execute",
		})
		return
	}

	// Check for project-type-specific tools
	requiredPatterns := map[string][]string{
		"go":     {"go build", "go test", "go vet"},
		"node":   {"npm", "node"},
		"rust":   {"cargo"},
		"python": {"python", "pytest"},
	}

	if patterns, ok := requiredPatterns[projectType]; ok {
		for _, p := range patterns {
			if !strings.Contains(tools, p) {
				result.Issues = append(result.Issues, OptimizeIssue{
					File:       ".ralphrc",
					Issue:      fmt.Sprintf("ALLOWED_TOOLS missing %q pattern for %s project", p, projectType),
					Severity:   "info",
					Suggestion: fmt.Sprintf("Add Bash(%s *) to ALLOWED_TOOLS", p),
				})
			}
		}
	}
}

func qualityGateKeys(projectType string) []string {
	switch projectType {
	case "go":
		return []string{"QG_GO_BUILD", "QG_GO_VET", "QG_GO_TEST"}
	case "node":
		return []string{"QG_BUILD_CMD", "QG_TEST_CMD"}
	default:
		return []string{"QG_BUILD_CMD", "QG_TEST_CMD"}
	}
}

func readKVFile(path string) map[string]string {
	values := make(map[string]string)
	f, err := os.Open(path)
	if err != nil {
		return values
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"")
	}
	return values
}
