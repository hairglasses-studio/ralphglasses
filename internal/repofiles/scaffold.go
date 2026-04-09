package repofiles

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScaffoldResult describes what was created/updated.
type ScaffoldResult struct {
	Created  []string `json:"created"`
	Skipped  []string `json:"skipped"`
	RepoPath string   `json:"repo_path"`
}

// ScaffoldOptions controls what gets created.
type ScaffoldOptions struct {
	Force       bool   // Overwrite existing files
	Minimal     bool   // Generate the minimal .ralphrc variant
	ProjectType string // go, node, python, etc.
	ProjectName string // Derived from directory name if empty
}

// Scaffold creates or initializes ralph supplemental files for a repo.
func Scaffold(repoPath string, opts ScaffoldOptions) (*ScaffoldResult, error) {
	if opts.ProjectName == "" {
		opts.ProjectName = filepath.Base(repoPath)
	}
	if opts.ProjectType == "" {
		opts.ProjectType = detectProjectType(repoPath)
	}

	result := &ScaffoldResult{RepoPath: repoPath}

	// Ensure .ralph/ directory exists
	ralphDir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(filepath.Join(ralphDir, "logs"), 0755); err != nil {
		return nil, fmt.Errorf("create .ralph dir: %w", err)
	}

	// Files to create
	files := map[string]func(string, ScaffoldOptions) string{
		filepath.Join(repoPath, ".ralphrc"):                      generateRalphRC,
		filepath.Join(repoPath, "AGENTS.md"):                     generateAgentsMD,
		filepath.Join(repoPath, ".agents", "roles", "README.md"): generateRoleCatalogReadme,
		filepath.Join(repoPath, ".codex", "config.toml"):       generateCodexConfig,
		filepath.Join(ralphDir, "PROMPT.md"):                     generatePrompt,
		filepath.Join(ralphDir, "AGENT.md"):                      generateAgent,
		filepath.Join(ralphDir, "fix_plan.md"):                   generateFixPlan,
	}

	for path, generator := range files {
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				result.Skipped = append(result.Skipped, relPath(repoPath, path))
				continue
			}
		}

		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("create parent dir for %s: %w", relPath(repoPath, path), err)
		}
		content := generator(opts.ProjectName, opts)
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return nil, fmt.Errorf("write %s: %w", relPath(repoPath, path), err)
		}
		result.Created = append(result.Created, relPath(repoPath, path))
	}

	return result, nil
}

func detectProjectType(repoPath string) string {
	indicators := map[string]string{
		"go.mod":           "go",
		"package.json":     "node",
		"Cargo.toml":       "rust",
		"pyproject.toml":   "python",
		"requirements.txt": "python",
		"pom.xml":          "java",
		"build.gradle":     "java",
	}
	for file, lang := range indicators {
		if _, err := os.Stat(filepath.Join(repoPath, file)); err == nil {
			return lang
		}
	}
	return "unknown"
}

func relPath(base, full string) string {
	rel, err := filepath.Rel(base, full)
	if err != nil {
		return full
	}
	return rel
}

func generateRalphRC(projectName string, opts ScaffoldOptions) string {
	if opts.Minimal {
		return fmt.Sprintf(`PROJECT_NAME="%s"
PROJECT_TYPE="%s"
PROVIDER="codex"
MODEL="gpt-5.4"
MAX_CALLS_PER_HOUR=80
`, projectName, opts.ProjectType)
	}

	buildCmd, testCmd, vetCmd := buildCommands(opts.ProjectType)

	return fmt.Sprintf(`PROJECT_NAME="%s"
PROJECT_TYPE="%s"
PROVIDER="codex"
MODEL="gpt-5.4"
MAX_CALLS_PER_HOUR=80
CLAUDE_TIMEOUT_MINUTES=20
CLAUDE_OUTPUT_FORMAT="json"
SESSION_CONTINUITY=true
SESSION_EXPIRY_HOURS=24
MARATHON_MODE=true
MARATHON_DURATION_HOURS=12
MARATHON_CHECKPOINT_INTERVAL=3
CB_NO_PROGRESS_THRESHOLD=4
CB_SAME_ERROR_THRESHOLD=5
CB_PERMISSION_DENIAL_THRESHOLD=2
CB_COOLDOWN_MINUTES=15
CB_AUTO_RESET=true
LOG_RETENTION_DAYS=7
PRIMARY_MODEL="gpt-5.4"
CACHE_SAFE_CLAUDE_RESUME=true
CACHE_ASSUMED_SAVINGS_CLAUDE=0.0
BATCH_SIMILAR_TASKS=true
MAX_TASKS_PER_BATCH=3
MAX_LINES_PER_BATCH=600
QG_BUILD_CMD="%s"
QG_TEST_CMD="%s"
QG_VET_CMD="%s"
RALPH_SESSION_BUDGET=100
FAST_MODE_ENABLED=true
FAST_MODE_PHASES="execution,test,docs,mechanical"
STANDARD_MODE_PHASES="analysis,planning,debug,refactor,architecture"
FAST_MODE_DEFAULT=false
`, projectName, opts.ProjectType, buildCmd, testCmd, vetCmd)
}

func generateAgentsMD(projectName string, opts ScaffoldOptions) string {
	build, test, vet := buildCommands(opts.ProjectType)
	return fmt.Sprintf(`# %s — Codex Instructions

Primary command-and-control provider: Codex.

## Build

`+"```bash"+`
%s
%s
%s
`+"```"+`

## Working Rules

- Read the codebase before changing it.
- Prefer the smallest defensible change.
- Run the relevant verification commands after edits.
- Keep unrelated files untouched.
- Treat resumed Claude sessions as cache-unsafe unless live cache reads prove otherwise.

## Role And Skill Surfaces

- Project instructions live in `+"`AGENTS.md`"+`.
- Shared workflows live in `+"`.agents/skills/`"+`.
- Shared fleet roles live in `+"`.agents/roles/*.json`"+`.
- Native role projections live in `+"`.codex/agents/*.toml`"+`, `+"`.claude/agents/*.md`"+`, and `+"`.gemini/agents/*.md`"+`.
- `+"`.gemini/commands/`"+` is reserved for shortcut compatibility prompts, not canonical roles.
- Project Codex config lives in `+"`.codex/config.toml`"+`.
- Regenerate role projections with `+"`python3 scripts/sync-provider-roles.py`"+`.
`, projectName, build, test, vet)
}

func generateRoleCatalogReadme(projectName string, opts ScaffoldOptions) string {
	return fmt.Sprintf(`# %s Role Catalog

This directory is the provider-neutral source of truth for reusable fleet roles.

## Purpose

- Put reusable role metadata in `+"`*.json`"+` manifests here.
- Keep provider-specific wording in `+"`provider_overrides`"+` when needed.
- Project native role files into `+"`.codex/agents/`"+`, `+"`.claude/agents/`"+`, and `+"`.gemini/agents/`"+`.

## Related Surfaces

- `+"`.agents/skills/`"+` for provider-neutral workflows and skills
- `+"`.gemini/commands/`"+` for Gemini shortcut compatibility prompts only

## Regeneration

Run `+"`python3 scripts/sync-provider-roles.py`"+` after editing manifests.
`, projectName)
}

func generateCodexConfig(projectName string, opts ScaffoldOptions) string {
	return `model = "gpt-5.4"
approval_policy = "on-request"
sandbox_mode = "workspace-write"
web_search = "cached"
model_reasoning_effort = "medium"
personality = "pragmatic"

[agents]
max_threads = 6
max_depth = 1
job_max_runtime_seconds = 1800
`
}

func buildCommands(projectType string) (build, test, vet string) {
	switch projectType {
	case "go":
		return "go build ./...", "go test ./...", "go vet ./..."
	case "node":
		return "npm run build", "npm test", "npm run lint"
	case "rust":
		return "cargo build", "cargo test", "cargo clippy"
	case "python":
		return "python -m py_compile", "pytest", "ruff check ."
	default:
		return "make build", "make test", "make lint"
	}
}

func generatePrompt(projectName string, opts ScaffoldOptions) string {
	return fmt.Sprintf(`# Ralph Development Instructions

## Context
You are Ralph, an autonomous AI development agent working on the **%s** project.

**Project Type:** %s

## Current Objectives
- Follow tasks in fix_plan.md
- Implement one task per loop
- Write tests for new functionality
- Update fix_plan.md progress after each task

## Key Principles
- ONE task per loop - focus on the most important thing
- Search the codebase before assuming something isn't implemented
- Write comprehensive tests with clear documentation
- Update fix_plan.md with your learnings
- Commit working changes with descriptive messages

## Protected Files (DO NOT MODIFY)
The following files and directories are part of Ralph's infrastructure.
NEVER delete, move, rename, or overwrite these under any circumstances:
- .ralph/ (entire directory and all contents)
- .ralphrc (project configuration)

## Testing Guidelines
- LIMIT testing to ~20%% of your total effort per loop
- PRIORITIZE: Implementation > Documentation > Tests
- Only write tests for NEW functionality you implement

## Build & Run
See AGENT.md for build and run instructions.

## Status Reporting (CRITICAL)

At the end of your response, ALWAYS include this status block:

`+"```"+`
---RALPH_STATUS---
STATUS: IN_PROGRESS | COMPLETE | BLOCKED
TASKS_COMPLETED_THIS_LOOP: <number>
FILES_MODIFIED: <number>
TESTS_STATUS: PASSING | FAILING | NOT_RUN
WORK_TYPE: IMPLEMENTATION | TESTING | DOCUMENTATION | REFACTORING
EXIT_SIGNAL: false | true
RECOMMENDATION: <one line summary of what to do next>
---END_RALPH_STATUS---
`+"```"+`

## Current Task
Follow fix_plan.md and choose the most important item to implement next.
`, projectName, opts.ProjectType)
}

func generateAgent(projectName string, opts ScaffoldOptions) string {
	build, test, _ := buildCommands(opts.ProjectType)
	return fmt.Sprintf(`# Ralph Agent Configuration

## Build Instructions

`+"```bash"+`
# Build the project
%s
`+"```"+`

## Test Instructions

`+"```bash"+`
# Run tests
%s
`+"```"+`

## Run Instructions

`+"```bash"+`
# Start/run the project
%s
`+"```"+`

## Notes
- Update this file when build process changes
- Add environment setup instructions as needed
- Include any pre-requisites or dependencies
`, build, test, runCommand(opts.ProjectType))
}

func runCommand(projectType string) string {
	switch projectType {
	case "go":
		return "go run ."
	case "node":
		return "npm start"
	case "rust":
		return "cargo run"
	case "python":
		return "python -m main"
	default:
		return "make run"
	}
}

func generateFixPlan(projectName string, opts ScaffoldOptions) string {
	return fmt.Sprintf(`# Fix Plan — %s

## High Priority
- [ ] Review codebase and understand architecture
- [ ] Identify critical bugs and issues
- [ ] Set up quality gates (%s)

## Medium Priority
- [ ] Implement core features from ROADMAP.md
- [ ] Add test coverage for critical paths
- [ ] Update documentation

## Low Priority
- [ ] Performance optimization
- [ ] Code cleanup and refactoring

## Completed
- [x] Project enabled for Ralph

## Notes
- Focus on MVP functionality first
- Ensure each feature is properly tested
- Update this file after each major milestone
`, projectName, opts.ProjectType)
}

// ReadClaudeMD reads the CLAUDE.md file from a repo, if it exists.
func ReadClaudeMD(repoPath string) string {
	data, err := os.ReadFile(filepath.Join(repoPath, "CLAUDE.md"))
	if err != nil {
		return ""
	}
	return string(data)
}

// ReadRoadmap reads the first N lines of ROADMAP.md from a repo.
func ReadRoadmap(repoPath string, maxLines int) string {
	f, err := os.Open(filepath.Join(repoPath, "ROADMAP.md"))
	if err != nil {
		return ""
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() && len(lines) < maxLines {
		lines = append(lines, scanner.Text())
	}
	return strings.Join(lines, "\n")
}
