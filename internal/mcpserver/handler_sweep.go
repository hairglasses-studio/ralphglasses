package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Built-in audit prompt template. REPO_PLACEHOLDER is replaced per-repo.
const sweepAuditTemplate = `<role>You are a senior software architect performing a deep codebase audit of REPO_PLACEHOLDER, a repository in the hairglasses-studio ecosystem (74 repos, primarily Go/Python/Node.js, built on the mcpkit MCP framework).</role>

<context>
Repository path: ~/hairglasses-studio/REPO_PLACEHOLDER
Ecosystem context: This repo may depend on mcpkit (Go MCP framework), hg-mcp (tool server), claudekit (terminal customization), or ralphglasses (orchestration TUI). Check go.mod, package.json, or requirements.txt for actual dependencies.
Use the largest context window available for the selected provider. Read every source file, test file, config file, and documentation file in the repo before producing findings.
</context>

<scratchpad>
Before writing findings, answer these questions internally:
- What is the repo's primary purpose and who are its consumers?
- What build system, test framework, and CI pipeline does it use?
- Which files have the highest cyclomatic complexity or deepest nesting?
- Where are error handling gaps (unchecked returns, swallowed errors, missing context wrapping)?
- What test coverage patterns exist and which critical paths lack tests?
- Are there dead code paths, unused exports, or stale dependencies?
- How well do AGENTS.md, README.md, and provider-specific docs reflect the actual codebase state?
- What cross-repo integration points could break silently?
</scratchpad>

<instructions>
Read the entire repository contents, then produce an audit covering these areas in order of impact:

1. Architecture health: package boundaries, dependency direction, circular imports, layering violations
2. Reliability gaps: error handling, nil checks, race conditions, resource leaks, missing timeouts
3. Test quality: coverage gaps on critical paths, missing edge case tests, test helpers that hide failures
4. Code hygiene: dead code, duplicated logic, overly complex functions (cyclomatic complexity above 10), inconsistent naming
5. Documentation drift: places where AGENTS.md, README, provider docs, or inline comments contradict the actual code
6. Dependency health: outdated deps, unused imports, version pinning issues, local replace directives that need updating
</instructions>

<constraints>
- Every finding references a specific file path and line number range
- Rank findings by effort-to-impact ratio (high impact + low effort first)
- Limit to 15 findings maximum, quality over quantity
- Include a concrete code fix or refactoring approach for each finding
- Skip cosmetic issues (formatting, whitespace, import ordering)
</constraints>

<output_format>
## REPO_PLACEHOLDER Audit Report

### Summary
One paragraph: overall health assessment and the single highest-priority improvement.

### Findings

For each finding:

#### [N] Title (Severity: high/medium/low)
- **File(s)**: path/to/file.go:42-67
- **Issue**: What is wrong and why it matters
- **Fix**: Specific code change or refactoring approach
- **Effort**: small (< 30 min) / medium (1-3 hours) / large (half day+)

### Instruction Accuracy
List any sections of AGENTS.md or provider-specific docs that are outdated or missing, with suggested corrections.

### Recommended Next Actions
Top 3 improvements ranked by effort-to-impact ratio, as actionable one-line descriptions.
</output_format>`

// Built-in fix prompt template. REPO_PLACEHOLDER is replaced per-repo.
const sweepFixTemplate = `<role>You are a senior software engineer fixing audit findings in REPO_PLACEHOLDER. Every fix must compile, pass tests, and be committed individually.</role>

<context>
Repository: ~/hairglasses-studio/REPO_PLACEHOLDER
Audit file: .claude/audit-2026-04-03.md or docs-generated audit notes contain all findings with file paths, line numbers, and fix descriptions.
Read the audit file first, then fix every item in priority order (HIGH first, then MEDIUM, then LOW).
</context>

<instructions>
1. Read .claude/audit-2026-04-03.md completely
2. Read AGENTS.md and project docs for build/test/lint commands
3. For each finding, in order:
   a. Read the referenced file(s) at the specified line numbers
   b. Apply the fix described in the audit
   c. Run the build command (go build ./... or equivalent)
   d. Run the test command (go test ./... -count=1 or equivalent)
   e. If tests pass, commit with message: "fix: [finding-N] <description>"
   f. If tests fail, diagnose and fix the test failure, then commit
   g. If the fix requires architectural decisions beyond what the audit describes, skip it and note why
4. After all findings are addressed, update .claude/audit-2026-04-03.md marking completed items with [x]
5. Run the full test suite one final time to confirm no regressions
</instructions>

<constraints>
- Every commit must compile and pass tests
- One commit per finding (do not batch unrelated fixes)
- Do not refactor code beyond what the finding specifies
- Skip findings that require human judgment and document why
- For documentation fixes, verify the correction against actual code before committing
- Do not modify test assertions to make tests pass — fix the code instead
</constraints>

<output_format>
When finished, print a summary:
- Total findings: N
- Fixed: N (list commit hashes)
- Skipped: N (list reasons)
- Test suite: PASS/FAIL
</output_format>`

func (s *Server) handleSweepGenerate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	taskType := p.OptionalString("task_type", "audit")
	targetProvider := p.OptionalString("target_provider", "openai")
	customPrompt := p.OptionalString("custom_prompt", "")

	var basePrompt string
	if customPrompt != "" {
		basePrompt = customPrompt
	} else if taskType == "fix" {
		basePrompt = sweepFixTemplate
	} else {
		basePrompt = sweepAuditTemplate
	}

	// Run through the enhancer pipeline.
	cfg := enhancer.Config{}
	mode := enhancer.ModeLocal
	tp := enhancer.ProviderName(targetProvider)
	eResult := enhancer.EnhanceHybrid(context.Background(), basePrompt, enhancer.TaskType(taskType), cfg, s.getEngine(), mode, tp)

	// Score the result.
	analysis := enhancer.Analyze(eResult.Enhanced)

	return jsonResult(map[string]any{
		"prompt":           eResult.Enhanced,
		"quality_score":    analysis.Score,
		"grade":            scoreGrade(analysis),
		"task_type":        eResult.TaskType,
		"stages_run":       eResult.StagesRun,
		"improvements":     eResult.Improvements,
		"estimated_tokens": eResult.EstimatedTokens,
		"cost_tier":        eResult.CostTier,
		"source":           eResult.Source,
	}), nil
}

func (s *Server) handleSweepLaunch(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	prompt, errResult := p.RequireString("prompt")
	if errResult != nil {
		return errResult, nil
	}

	reposParam := p.OptionalString("repos", "active")
	limit := int(p.OptionalNumber("limit", 10))
	model := p.OptionalString("model", session.ProviderDefaults(session.ProviderCodex))
	permMode := p.OptionalString("permission_mode", "plan")
	enhanceMode := p.OptionalString("enhance_prompt", "local")
	budgetUSD := p.OptionalNumber("budget_usd", 0.50)
	effort := p.OptionalString("effort", "")
	allowedTools := p.OptionalString("allowed_tools", "")
	maxSweepBudget := p.OptionalNumber("max_sweep_budget_usd", 100.0)
	maxTurns := int(p.OptionalNumber("max_turns", 50))
	sessionPersistence := p.OptionalBool("session_persistence", false)

	// Ensure repos are scanned.
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	// Resolve target repos.
	targetRepos, err := s.resolveSweepRepos(reposParam, limit)
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	if len(targetRepos) == 0 {
		return codedError(ErrInvalidParams, "no repos matched"), nil
	}

	// Validate model name against known provider prefixes.
	for _, w := range session.ValidateLoopConfig(session.LoopConfig{
		Provider: session.ProviderCodex, Model: model,
	}) {
		if w.Field == "model" {
			return codedError(ErrInvalidParams, fmt.Sprintf("model validation: %s", w.Message)), nil
		}
	}

	// Pre-launch cost estimation.
	repoCount := len(targetRepos)
	estimatedPerSession := 1.0

	if s.SessMgr != nil && s.SessMgr.HasCostPredictor() {
		estimatedPerSession = s.SessMgr.GetCostPredictor().Predict("sweep", "codex")
	}
	if estimatedPerSession <= 1.0 {
		pc := config.DefaultProviderCosts()
		inRate := pc.InputPerMToken["codex"]
		outRate := pc.OutputPerMToken["codex"]
		estTurns := float64(maxTurns) * 0.6
		tokPerTurn := 8000.0
		estimatedPerSession = (tokPerTurn * estTurns / 1_000_000) * (inRate + outRate)
	}

	// Auto-size per-session budget: at least 1.5x estimate.
	if budgetUSD < estimatedPerSession*1.5 {
		budgetUSD = estimatedPerSession * 1.5
	}

	// Check sweep total against cap.
	totalEstimated := estimatedPerSession * float64(repoCount)
	if totalEstimated > maxSweepBudget {
		return codedError(ErrInvalidParams, fmt.Sprintf(
			"estimated sweep cost $%.2f (%d repos × $%.2f) exceeds max_sweep_budget_usd $%.2f",
			totalEstimated, repoCount, estimatedPerSession, maxSweepBudget)), nil
	}

	sweepID := fmt.Sprintf("sweep-%s", uuid.New().String()[:8])
	sweepPool := session.NewBudgetPool(maxSweepBudget)

	// Default read-only tools for plan mode.
	var tools []string
	if allowedTools != "" {
		tools = strings.Split(allowedTools, ",")
	} else if permMode == "plan" {
		tools = []string{"Bash(readonly:true)", "Read", "Glob", "Grep"}
	}

	// Fan out session launches.
	ctx, cancel := context.WithCancel(context.Background())
	taskID := s.Tasks.Create("sweep_launch", cancel, map[string]any{
		"sweep_id": sweepID,
		"repos":    len(targetRepos),
	})

	go func() {
		var launched []map[string]any
		var errors []string

		for _, r := range targetRepos {
			// Substitute REPO_PLACEHOLDER with actual repo name.
			repoPrompt := strings.ReplaceAll(prompt, "REPO_PLACEHOLDER", r.Name)

			opts := session.LaunchOptions{
				Provider:             session.DefaultPrimaryProvider(),
				RepoPath:             r.Path,
				Prompt:               repoPrompt,
				Model:                model,
				MaxBudgetUSD:         budgetUSD,
				MaxTurns:             maxTurns,
				PermissionMode:       permMode,
				SweepID:              sweepID,
				AllowedTools:         tools,
				NoSessionPersistence: !sessionPersistence,
				SessionID:            fmt.Sprintf("%s-%s", sweepID, r.Name),
			}
			if effort != "" {
				opts.Effort = effort
			}

			// Auto-enhance prompt if requested.
			if enhanceMode != "" && enhanceMode != "none" {
				cfg := enhancer.LoadConfig(r.Path)
				if enhancer.ShouldEnhance(repoPrompt, cfg) {
					m := enhancer.ValidMode(enhanceMode)
					if m == "" {
						m = enhancer.ModeLocal
					}
					eResult := enhancer.EnhanceHybrid(ctx, repoPrompt, "", cfg, s.getEngine(), m, enhancer.ProviderOpenAI)
					opts.Prompt = eResult.Enhanced
				}
			}

			// Check sweep pool before launch.
			if err := sweepPool.Allocate(r.Name, budgetUSD); err != nil {
				errors = append(errors, fmt.Sprintf("%s: sweep budget cap reached", r.Name))
				break
			}

			sess, err := s.SessMgr.Launch(context.Background(), opts)
			if err != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", r.Name, err))
				continue
			}

			launched = append(launched, map[string]any{
				"session_id": sess.ID,
				"repo":       r.Name,
				"status":     sess.Status,
			})
		}

		result := map[string]any{
			"sweep_id":              sweepID,
			"launched":              launched,
			"errors":                errors,
			"total":                 len(launched),
			"estimated_per_session": estimatedPerSession,
			"estimated_total":       totalEstimated,
			"budget_per_session":    budgetUSD,
			"max_sweep_budget_usd":  maxSweepBudget,
		}
		s.Tasks.Complete(taskID, result)
	}()

	return jsonResult(map[string]any{
		"task_id":               taskID,
		"sweep_id":              sweepID,
		"repos":                 len(targetRepos),
		"status":                "launching",
		"estimated_per_session": estimatedPerSession,
		"estimated_total":       totalEstimated,
		"budget_per_session":    budgetUSD,
		"max_sweep_budget_usd":  maxSweepBudget,
	}), nil
}

func (s *Server) handleSweepStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	verbose := p.OptionalBool("verbose", false)

	sessions := s.sweepSessions(sweepID)
	if len(sessions) == 0 {
		return emptyResult("sweep_sessions"), nil
	}

	var totalCost float64
	var completed, running, errored, stalled int
	var items []map[string]any

	stalledIDs := make(map[string]bool)
	for _, id := range s.SessMgr.DetectStalls(session.DefaultStallThreshold) {
		stalledIDs[id] = true
	}

	for _, sess := range sessions {
		sess.Lock()
		status := sess.Status
		spent := sess.SpentUSD
		repo := sess.RepoName
		turns := sess.TurnCount
		lastAct := sess.LastActivity
		lastOut := sess.LastOutput
		sessID := sess.ID
		sess.Unlock()

		totalCost += spent

		switch {
		case status == session.StatusCompleted:
			completed++
		case status == session.StatusRunning || status == session.StatusLaunching:
			running++
		case status == session.StatusErrored:
			errored++
		}
		if stalledIDs[sessID] {
			stalled++
		}

		item := map[string]any{
			"session_id": sessID,
			"repo":       repo,
			"status":     status,
			"spent_usd":  spent,
			"turns":      turns,
			"idle_sec":   int(time.Since(lastAct).Seconds()),
			"stalled":    stalledIDs[sessID],
		}
		if verbose && lastOut != "" {
			item["last_output"] = lastOut
		}
		items = append(items, item)
	}

	total := len(sessions)
	pct := 0.0
	if total > 0 {
		pct = float64(completed) / float64(total) * 100
	}

	return jsonResult(map[string]any{
		"sweep_id":       sweepID,
		"total":          total,
		"running":        running,
		"completed":      completed,
		"errored":        errored,
		"stalled":        stalled,
		"completion_pct": pct,
		"total_cost_usd": totalCost,
		"sessions":       items,
	}), nil
}

func (s *Server) handleSweepNudge(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	thresholdMin := p.OptionalNumber("stale_threshold_min", 5)
	action := p.OptionalString("action", "restart")

	threshold := time.Duration(thresholdMin) * time.Minute
	sessions := s.sweepSessions(sweepID)

	var nudged, skipped int
	var details []map[string]any

	for _, sess := range sessions {
		sess.Lock()
		status := sess.Status
		idle := time.Since(sess.LastActivity)
		repo := sess.RepoName
		repoPath := sess.RepoPath
		prompt := sess.Prompt
		model := sess.Model
		sessID := sess.ID
		budget := sess.BudgetUSD
		permMode := sess.PermissionMode
		sess.Unlock()

		isRunning := status == session.StatusRunning || status == session.StatusLaunching
		if !isRunning || idle < threshold {
			continue
		}

		switch action {
		case "restart":
			_ = s.SessMgr.Stop(sessID)

			newOpts := session.LaunchOptions{
				Provider:       session.DefaultPrimaryProvider(),
				RepoPath:       repoPath,
				Prompt:         prompt,
				Model:          model,
				MaxBudgetUSD:   budget,
				PermissionMode: permMode,
				SweepID:        sweepID,
			}
			newSess, err := s.SessMgr.Launch(context.Background(), newOpts)
			if err != nil {
				details = append(details, map[string]any{
					"repo":   repo,
					"action": "restart_failed",
					"error":  err.Error(),
				})
				continue
			}
			nudged++
			details = append(details, map[string]any{
				"repo":           repo,
				"action":         "restarted",
				"old_session_id": sessID,
				"new_session_id": newSess.ID,
				"idle_min":       int(idle.Minutes()),
			})

		case "skip":
			skipped++
			details = append(details, map[string]any{
				"repo":     repo,
				"action":   "skipped",
				"idle_min": int(idle.Minutes()),
			})
		}
	}

	return jsonResult(map[string]any{
		"sweep_id": sweepID,
		"nudged":   nudged,
		"skipped":  skipped,
		"details":  details,
	}), nil
}

func (s *Server) handleSweepSchedule(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	sweepID, errResult := p.RequireString("sweep_id")
	if errResult != nil {
		return errResult, nil
	}
	intervalMin := p.OptionalNumber("interval_minutes", 5)
	autoNudge := p.OptionalBool("auto_nudge", false)
	maxChecks := int(p.OptionalNumber("max_checks", 0))
	maxCostCap := p.OptionalNumber("max_sweep_budget_usd", 0)

	interval := time.Duration(intervalMin) * time.Minute
	if interval < time.Minute {
		interval = time.Minute
	}

	ctx, cancel := context.WithCancel(context.Background())
	taskID := s.Tasks.Create("sweep_schedule", cancel, map[string]any{
		"sweep_id":     sweepID,
		"interval_min": intervalMin,
		"auto_nudge":   autoNudge,
	})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		checks := 0
		for {
			select {
			case <-ctx.Done():
				s.Tasks.Complete(taskID, map[string]any{
					"checks_completed": checks,
					"reason":           "canceled",
				})
				return
			case <-ticker.C:
				checks++

				sessions := s.sweepSessions(sweepID)
				if len(sessions) == 0 {
					s.Tasks.Complete(taskID, map[string]any{
						"checks_completed": checks,
						"reason":           "no sessions found",
					})
					return
				}

				var running, completed, stalled int
				var totalCost float64
				stalledIDs := s.SessMgr.DetectStalls(session.DefaultStallThreshold)
				stalledSet := make(map[string]bool, len(stalledIDs))
				for _, id := range stalledIDs {
					stalledSet[id] = true
				}

				for _, sess := range sessions {
					sess.Lock()
					st := sess.Status
					totalCost += sess.SpentUSD
					sessID := sess.ID
					sess.Unlock()

					switch {
					case st == session.StatusCompleted:
						completed++
					case st == session.StatusRunning || st == session.StatusLaunching:
						running++
					}
					if stalledSet[sessID] {
						stalled++
					}
				}

				// Auto-nudge stalled sessions.
				if autoNudge && stalled > 0 {
					nudgeReq := mcp.CallToolRequest{}
					nudgeReq.Params.Name = "ralphglasses_sweep_nudge"
					nudgeReq.Params.Arguments = map[string]any{
						"sweep_id": sweepID,
						"action":   "restart",
					}
					_, _ = s.handleSweepNudge(ctx, nudgeReq)
				}

				// Update task progress.
				progress := 0.0
				total := len(sessions)
				if total > 0 {
					progress = float64(completed) / float64(total)
				}
				s.Tasks.SetProgress(taskID, progress)

				// Cost cap abort: stop all sessions if total spend exceeds cap.
				// TODO: replace polling with event-driven cost check
				if maxCostCap > 0 && totalCost >= maxCostCap {
					for _, sess := range sessions {
						sess.Lock()
						st := sess.Status
						sid := sess.ID
						sess.Unlock()
						if st == session.StatusRunning || st == session.StatusLaunching {
							_ = s.SessMgr.Stop(sid)
						}
					}
					s.Tasks.Complete(taskID, map[string]any{
						"checks_completed": checks,
						"total_cost_usd":   totalCost,
						"reason":           fmt.Sprintf("cost cap $%.2f reached (spent $%.2f)", maxCostCap, totalCost),
					})
					return
				}

				// All done?
				if running == 0 {
					s.Tasks.Complete(taskID, map[string]any{
						"checks_completed": checks,
						"total_cost_usd":   totalCost,
						"reason":           "all sessions finished",
					})
					return
				}

				// Max checks reached?
				if maxChecks > 0 && checks >= maxChecks {
					s.Tasks.Complete(taskID, map[string]any{
						"checks_completed": checks,
						"total_cost_usd":   totalCost,
						"reason":           "max checks reached",
					})
					return
				}
			}
		}
	}()

	return jsonResult(map[string]any{
		"task_id":      taskID,
		"sweep_id":     sweepID,
		"interval_min": intervalMin,
		"auto_nudge":   autoNudge,
		"max_checks":   maxChecks,
		"status":       "scheduled",
	}), nil
}

// sweepSessions returns all in-memory sessions with the given sweep ID.
func (s *Server) sweepSessions(sweepID string) []*session.Session {
	all := s.SessMgr.List("")
	var result []*session.Session
	for _, sess := range all {
		sess.Lock()
		sid := sess.SweepID
		sess.Unlock()
		if sid == sweepID {
			result = append(result, sess)
		}
	}
	return result
}

// resolveSweepRepos parses the repos parameter into a list of repos.
func (s *Server) resolveSweepRepos(reposParam string, limit int) ([]*repoRef, error) {
	allRepos := s.reposCopy()

	switch reposParam {
	case "all":
		return toRepoRefs(allRepos, limit), nil
	case "active":
		return toRepoRefs(allRepos, limit), nil
	default:
		// Parse JSON array of repo names.
		var names []string
		if err := json.Unmarshal([]byte(reposParam), &names); err != nil {
			return nil, fmt.Errorf("repos must be a JSON array of names, \"active\", or \"all\": %w", err)
		}
		var refs []*repoRef
		for _, name := range names {
			r := s.findRepo(name)
			if r == nil {
				return nil, fmt.Errorf("repo not found: %s", name)
			}
			refs = append(refs, &repoRef{Name: r.Name, Path: r.Path})
			if limit > 0 && len(refs) >= limit {
				break
			}
		}
		return refs, nil
	}
}

type repoRef struct {
	Name string
	Path string
}

// scoreGrade extracts the overall grade from an AnalyzeResult.
func scoreGrade(a enhancer.AnalyzeResult) string {
	if a.ScoreReport != nil {
		return a.ScoreReport.Grade
	}
	switch {
	case a.Score >= 90:
		return "A"
	case a.Score >= 80:
		return "B"
	case a.Score >= 65:
		return "C"
	case a.Score >= 50:
		return "D"
	default:
		return "F"
	}
}

func toRepoRefs(repos []*model.Repo, limit int) []*repoRef {
	var refs []*repoRef
	for _, r := range repos {
		refs = append(refs, &repoRef{Name: r.Name, Path: r.Path})
		if limit > 0 && len(refs) >= limit {
			break
		}
	}
	return refs
}
