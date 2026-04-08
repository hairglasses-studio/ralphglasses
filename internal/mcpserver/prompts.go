package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

// RegisterPrompts registers MCP prompt primitives on the given MCP server.
// Prompts expose enhancer templates and structured planning prompts as
// first-class MCP prompt resources that clients can list and invoke.
func RegisterPrompts(srv *server.MCPServer, appSrv *Server) {
	prompts := serverPrompts(appSrv)
	for i := range prompts {
		prompts[i].Handler = instrumentPromptHandler(appSrv, prompts[i].Prompt.Name, prompts[i].Handler)
	}
	srv.AddPrompts(prompts...)
}

func serverPrompts(appSrv *Server) []server.ServerPrompt {
	return []server.ServerPrompt{
		selfImprovementPlannerPrompt(),
		codeReviewPrompt(),
		testGenerationPrompt(),
		bootstrapFirstbootPrompt(),
		providerParityAuditPrompt(appSrv),
		repoTriageBriefPrompt(),
	}
}

// selfImprovementPlannerPrompt returns a prompt for planning self-improvement
// iterations on a repository, parameterized by repo name and focus area.
func selfImprovementPlannerPrompt() server.ServerPrompt {
	prompt := mcp.NewPrompt("self-improvement-planner",
		mcp.WithPromptDescription("Plan a self-improvement iteration for a repository. Produces a structured plan with goals, steps, validation criteria, and rollback strategy."),
		mcp.WithArgument("repo_name",
			mcp.ArgumentDescription("Name of the repository to improve"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("focus_area",
			mcp.ArgumentDescription("Area to focus improvement on (e.g., error-handling, test-coverage, performance, documentation)"),
			mcp.RequiredArgument(),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		repoName := req.Params.Arguments["repo_name"]
		focusArea := req.Params.Arguments["focus_area"]

		if repoName == "" {
			return nil, fmt.Errorf("repo_name is required")
		}
		if focusArea == "" {
			return nil, fmt.Errorf("focus_area is required")
		}

		content := buildSelfImprovementPrompt(repoName, focusArea)

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Self-improvement plan for %s focused on %s", repoName, focusArea),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

// codeReviewPrompt returns a prompt for reviewing code changes in a repository,
// built on top of the enhancer's code_review template.
func codeReviewPrompt() server.ServerPrompt {
	prompt := mcp.NewPrompt("code-review",
		mcp.WithPromptDescription("Review code changes in a repository file. Uses the enhancer code_review template for structured feedback with severity levels."),
		mcp.WithArgument("repo_name",
			mcp.ArgumentDescription("Name of the repository containing the code"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("file_path",
			mcp.ArgumentDescription("Path to the file to review (relative to repo root)"),
			mcp.RequiredArgument(),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		repoName := req.Params.Arguments["repo_name"]
		filePath := req.Params.Arguments["file_path"]

		if repoName == "" {
			return nil, fmt.Errorf("repo_name is required")
		}
		if filePath == "" {
			return nil, fmt.Errorf("file_path is required")
		}

		content := buildCodeReviewPrompt(repoName, filePath)

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Code review for %s in %s", filePath, repoName),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

// testGenerationPrompt returns a prompt for generating tests for a file,
// with an optional coverage target parameter.
func testGenerationPrompt() server.ServerPrompt {
	prompt := mcp.NewPrompt("test-generation",
		mcp.WithPromptDescription("Generate tests for a repository file. Produces a structured test plan with coverage targets, edge cases, and test scaffolding."),
		mcp.WithArgument("repo_name",
			mcp.ArgumentDescription("Name of the repository containing the code"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("file_path",
			mcp.ArgumentDescription("Path to the file to generate tests for (relative to repo root)"),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("coverage_target",
			mcp.ArgumentDescription("Target test coverage percentage (e.g., 80). Defaults to 80 if not provided."),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		repoName := req.Params.Arguments["repo_name"]
		filePath := req.Params.Arguments["file_path"]
		coverageTarget := req.Params.Arguments["coverage_target"]

		if repoName == "" {
			return nil, fmt.Errorf("repo_name is required")
		}
		if filePath == "" {
			return nil, fmt.Errorf("file_path is required")
		}
		if coverageTarget == "" {
			coverageTarget = "80"
		}

		content := buildTestGenerationPrompt(repoName, filePath, coverageTarget)

		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Test generation for %s in %s (target: %s%%)", filePath, repoName, coverageTarget),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

func bootstrapFirstbootPrompt() server.ServerPrompt {
	prompt := mcp.NewPrompt("bootstrap-firstboot",
		mcp.WithPromptDescription("Build a first-boot checklist for validating provider CLIs, MCP wiring, and repo readiness before launching work."),
		mcp.WithArgument("scan_path",
			mcp.ArgumentDescription(fmt.Sprintf("Workspace scan path to validate. Defaults to %s if omitted.", config.DefaultScanPath)),
		),
		mcp.WithArgument("primary_provider",
			mcp.ArgumentDescription("Primary provider to optimize for first (codex, claude, or gemini). Defaults to codex."),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		scanPath := req.Params.Arguments["scan_path"]
		if scanPath == "" {
			scanPath = config.DefaultScanPath
		}
		primaryProvider := req.Params.Arguments["primary_provider"]
		if primaryProvider == "" {
			primaryProvider = "codex"
		}

		content := buildBootstrapFirstbootPrompt(scanPath, primaryProvider)
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("First-boot checklist for %s (primary provider: %s)", scanPath, primaryProvider),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

func providerParityAuditPrompt(appSrv *Server) server.ServerPrompt {
	prompt := mcp.NewPrompt("provider-parity-audit",
		mcp.WithPromptDescription("Audit provider parity for a repository across AGENTS.md, provider config, MCP config, generated skills, and workflow prompts."),
		mcp.WithArgument("repo_name",
			mcp.ArgumentDescription("Repository name to audit."),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("target_provider",
			mcp.ArgumentDescription("Optional provider to emphasize (codex, claude, gemini). Defaults to all."),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		repoName := req.Params.Arguments["repo_name"]
		if repoName == "" {
			return nil, fmt.Errorf("repo_name is required")
		}
		targetProvider := req.Params.Arguments["target_provider"]
		if targetProvider == "" {
			targetProvider = "all"
		}

		content := buildProviderParityAuditPrompt(appSrv, repoName, targetProvider)
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Provider parity audit for %s (target: %s)", repoName, targetProvider),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

func repoTriageBriefPrompt() server.ServerPrompt {
	prompt := mcp.NewPrompt("repo-triage-brief",
		mcp.WithPromptDescription("Build a repo triage brief from status, progress, logs, runtime health, and the recommended next skill/tool path."),
		mcp.WithArgument("repo_name",
			mcp.ArgumentDescription("Repository name to triage."),
			mcp.RequiredArgument(),
		),
		mcp.WithArgument("concern",
			mcp.ArgumentDescription("Optional concern or symptom to emphasize during triage."),
		),
	)

	handler := func(_ context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		repoName := req.Params.Arguments["repo_name"]
		if repoName == "" {
			return nil, fmt.Errorf("repo_name is required")
		}
		concern := req.Params.Arguments["concern"]
		if concern == "" {
			concern = "general repo health"
		}

		content := buildRepoTriageBriefPrompt(repoName, concern)
		return &mcp.GetPromptResult{
			Description: fmt.Sprintf("Repo triage brief for %s (%s)", repoName, concern),
			Messages: []mcp.PromptMessage{
				{
					Role:    mcp.RoleUser,
					Content: mcp.TextContent{Type: "text", Text: content},
				},
			},
		}, nil
	}

	return server.ServerPrompt{Prompt: prompt, Handler: handler}
}

func instrumentPromptHandler(appSrv *Server, name string, next server.PromptHandlerFunc) server.PromptHandlerFunc {
	return func(ctx context.Context, req mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		result, err := next(ctx, req)
		if err == nil && result != nil && appSrv != nil && appSrv.DiscoveryRecorder != nil {
			appSrv.DiscoveryRecorder.RecordPrompt(name)
		}
		return result, err
	}
}

// buildSelfImprovementPrompt constructs a structured self-improvement planning
// prompt. It draws on the workflow_create enhancer template for structure.
func buildSelfImprovementPrompt(repoName, focusArea string) string {
	// Use the workflow_create template as a structural base.
	tmpl := enhancer.GetTemplate("workflow_create")
	var base string
	if tmpl != nil {
		base = enhancer.FillTemplate(tmpl, map[string]string{
			"goal":        fmt.Sprintf("Self-improvement iteration for %s focused on %s", repoName, focusArea),
			"systems":     repoName,
			"constraints": "Changes must pass existing tests; no regressions allowed",
		})
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`You are a self-improvement planner for the %s repository.

## Objective
Plan a focused improvement iteration targeting: %s

## Instructions
1. Analyze the current state of %s in the repository
2. Identify the top 3-5 concrete improvements for %s
3. For each improvement, specify:
   - What to change and why
   - Files likely affected
   - Validation criteria (how to confirm the improvement worked)
   - Risk level (low/medium/high) and rollback strategy
4. Order improvements by impact-to-effort ratio (highest first)
5. Estimate total effort in hours

## Constraints
- All changes must pass existing tests (no regressions)
- Each improvement should be a single, reviewable commit
- Include a validation step after each change
- If any step fails validation, stop and report

## Output Format
### Improvement Plan: %s / %s

#### Summary
One paragraph describing the overall improvement goal and expected outcome.

#### Steps
| # | Improvement | Files | Effort | Risk | Validation |
|---|------------|-------|--------|------|------------|
| 1 | ... | ... | ... | ... | ... |

#### Rollback Strategy
- ...

#### Success Criteria
- ...
`, repoName, focusArea, focusArea, focusArea, repoName, focusArea))

	if base != "" {
		b.WriteString("\n---\n## Reference: Workflow Template\n")
		b.WriteString(base)
	}

	return b.String()
}

// buildCodeReviewPrompt constructs a code review prompt using the enhancer's
// code_review template as a base.
func buildCodeReviewPrompt(repoName, filePath string) string {
	tmpl := enhancer.GetTemplate("code_review")

	var b strings.Builder
	if tmpl != nil {
		filled := enhancer.FillTemplate(tmpl, map[string]string{
			"language": inferLanguage(filePath),
			"focus":    "correctness, error handling, idiomatic patterns, security",
			"code":     fmt.Sprintf("(Contents of %s in repository %s — read the file to review)", filePath, repoName),
		})
		b.WriteString(filled)
	} else {
		// Fallback if template is not found.
		b.WriteString(fmt.Sprintf(`You are a senior code reviewer.

Review the file %s in the %s repository.

Focus on:
- Correctness and edge cases
- Error handling
- Idiomatic patterns
- Security concerns

Provide findings categorized as critical, suggestion, or nitpick.
`, filePath, repoName))
	}

	b.WriteString(fmt.Sprintf("\n\n## Context\n- Repository: %s\n- File: %s\n- Language: %s\n",
		repoName, filePath, inferLanguage(filePath)))

	return b.String()
}

// buildTestGenerationPrompt constructs a test generation prompt.
func buildTestGenerationPrompt(repoName, filePath, coverageTarget string) string {
	lang := inferLanguage(filePath)

	return fmt.Sprintf(`You are a test engineer for the %s repository.

## Objective
Generate comprehensive tests for %s targeting %s%% code coverage.

## Instructions
1. Read and understand the code in %s
2. Identify all public functions, methods, and exported types
3. For each function/method:
   - Write a happy-path test
   - Write edge-case tests (empty input, nil, boundaries, overflow)
   - Write error-path tests (invalid input, failures)
4. Add table-driven tests where multiple inputs share the same logic
5. Include benchmark tests for performance-sensitive functions

## Constraints
- Language: %s
- Test file should follow %s conventions (e.g., _test.go for Go, _test.py for Python)
- Use the repository's existing test patterns and helpers
- Do not mock what you can construct
- Each test must have a clear name describing what it validates
- Target coverage: %s%%

## Output Format
### Test Plan: %s

#### Coverage Analysis
| Function/Method | Current Coverage | Tests Needed |
|----------------|-----------------|--------------|
| ... | ... | ... |

#### Test Cases
For each test, provide:
- Test name
- Description of what it validates
- Setup / input
- Expected outcome
- Edge cases covered

#### Generated Test Code
`+"```%s\n// Tests for %s\n```"+`

#### Coverage Estimate
Expected coverage after adding these tests: %s%%
`, repoName, filePath, coverageTarget, filePath, lang, lang, coverageTarget, filePath, lang, filePath, coverageTarget)
}

func buildBootstrapFirstbootPrompt(scanPath, primaryProvider string) string {
	return fmt.Sprintf(`You are preparing a first-boot and bootstrap checklist for a ralphglasses operator.

## Workspace
- Scan path: %s
- Primary provider: %s

## Objective
Create a step-by-step bootstrap checklist that gets the workspace ready for safe MCP-driven operation.

## Instructions
1. Verify the operator can build and run ralphglasses locally.
2. Verify the primary provider CLI is installed and authenticated.
3. Verify repo-local MCP discovery via .mcp.json and .codex/config.toml.
4. Verify the operator can inspect the MCP contract through server health, tool groups, resources, and prompts before using mutating tools.
5. Identify the first repo to validate and the minimum commands or MCP calls needed to prove readiness.
6. Include failure handling for missing provider CLIs, broken MCP registration, missing AGENTS.md, and stale generated skills.

## Output Format
### First-Boot Checklist
| Step | Goal | Validation | Failure Fix |
|------|------|------------|-------------|
| 1 | ... | ... | ... |

### Recommended First Validation Pass
- Repo to validate:
- Read-only discovery path:
- First mutating action after validation:

### Rollback / Safety Notes
- ...
`, scanPath, primaryProvider)
}

func buildProviderParityAuditPrompt(appSrv *Server, repoName, targetProvider string) string {
	repoSummary := fmt.Sprintf("Repository: %s", repoName)
	if repo, err := resolveRepo(appSrv, repoName); err == nil && repo != nil {
		repoSummary = fmt.Sprintf("Repository: %s\nPath: %s", repo.Name, repo.Path)
	}

	return fmt.Sprintf(`You are auditing provider parity for a ralphglasses-managed repository.

## Scope
%s
- Target provider emphasis: %s

## Audit Surfaces
- AGENTS.md and provider-specific instruction projections
- .mcp.json MCP registration
- .codex/config.toml curated profiles
- generated skills under .agents/skills/* and projected provider/plugin skills
- MCP prompts, resources, and discovery contract

## Instructions
1. Identify which parity surfaces are checked-in sources of truth versus generated projections.
2. Compare Codex, Claude, and Gemini support for this repo's managed surfaces.
3. Call out drift that blocks MCP-first or skill-first workflows.
4. Separate contract drift, documentation drift, and generated-file drift.
5. Recommend the smallest safe changes needed to regain parity without breaking current clients.

## Output Format
### Parity Audit Summary
One paragraph on overall parity health.

### Findings
| Severity | Surface | Drift | Recommended Fix |
|----------|---------|-------|-----------------|
| High | ... | ... | ... |

### MCP-First Coverage
- Already covered:
- Still CLI-only:
- Highest-value next parity win:
`, repoSummary, targetProvider)
}

func buildRepoTriageBriefPrompt(repoName, concern string) string {
	return fmt.Sprintf(`You are preparing a repo triage brief for a ralphglasses-managed repository.

## Scope
- Repository: %s
- Primary concern: %s

## Read-Only Inputs
- ralph:///%s/status
- ralph:///%s/progress
- ralph:///%s/logs
- ralph:///runtime/health
- ralph:///catalog/skills
- ralph:///catalog/workflows

## Optional Tool Follow-Ups
- ralphglasses_status
- ralphglasses_repo_health
- ralphglasses_logs
- ralphglasses_debug_bundle
- ralphglasses_server_health

## Instructions
1. Summarize the current repo state from the read-only inputs before proposing any mutation.
2. Call out the likely next best skill family to use:
   - ralphglasses-repo-admin
   - ralphglasses-recovery-observability
   - ralphglasses-session-ops
   - ralphglasses-self-dev
3. Separate evidence from inference.
4. If logs or runtime health suggest instability, recommend the smallest safe next read-only action before any fix.
5. End with the first one or two MCP tool calls that should follow the triage brief.

## Output Format
### Repo Triage Summary
One paragraph on the current repo state and the main concern.

### Evidence
| Source | Signal | Notes |
|--------|--------|-------|
| status | ... | ... |
| progress | ... | ... |
| logs | ... | ... |
| runtime | ... | ... |

### Recommended Skill And Next Steps
- Best-fit skill:
- Why:
- First MCP call:
- Second MCP call:
`, repoName, concern, repoName, repoName, repoName)
}

// inferLanguage guesses the programming language from a file path extension.
func inferLanguage(filePath string) string {
	lower := strings.ToLower(filePath)
	switch {
	case strings.HasSuffix(lower, ".go"):
		return "Go"
	case strings.HasSuffix(lower, ".py"):
		return "Python"
	case strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".tsx"):
		return "TypeScript"
	case strings.HasSuffix(lower, ".js") || strings.HasSuffix(lower, ".jsx"):
		return "JavaScript"
	case strings.HasSuffix(lower, ".rs"):
		return "Rust"
	case strings.HasSuffix(lower, ".java"):
		return "Java"
	case strings.HasSuffix(lower, ".rb"):
		return "Ruby"
	case strings.HasSuffix(lower, ".sh") || strings.HasSuffix(lower, ".bash"):
		return "Shell"
	default:
		return "unknown"
	}
}
