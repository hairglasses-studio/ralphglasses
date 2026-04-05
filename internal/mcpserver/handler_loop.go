package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleLoopStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	repoName, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrRepoNameInvalid, fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	profile := session.DefaultLoopProfile()
	if value := p.OptionalString("planner_model", ""); value != "" {
		profile.PlannerModel = value
	}
	if value := p.OptionalString("worker_model", ""); value != "" {
		profile.WorkerModel = value
	}
	if value := p.OptionalString("verifier_model", ""); value != "" {
		profile.VerifierModel = value
	}
	if value := p.OptionalString("worktree_policy", ""); value != "" {
		profile.WorktreePolicy = value
	}
	if value := int(p.OptionalNumber("retry_limit", float64(profile.RetryLimit))); value != profile.RetryLimit {
		profile.RetryLimit = value
	}
	if value := int(p.OptionalNumber("max_concurrent_workers", float64(profile.MaxConcurrentWorkers))); value != profile.MaxConcurrentWorkers {
		profile.MaxConcurrentWorkers = value
	}
	if commands := splitLines(p.OptionalString("verify_commands", "")); len(commands) > 0 {
		profile.VerifyCommands = commands
	}
	if pp := p.OptionalString("planner_provider", ""); pp != "" {
		profile.PlannerProvider = session.Provider(pp)
	}
	if wp := p.OptionalString("worker_provider", ""); wp != "" {
		profile.WorkerProvider = session.Provider(wp)
	}
	if vp := p.OptionalString("verifier_provider", ""); vp != "" {
		profile.VerifierProvider = session.Provider(vp)
	}
	if budgetUSD := p.OptionalNumber("budget_usd", 0); budgetUSD > 0 {
		profile.PlannerBudgetUSD = budgetUSD / 3
		profile.WorkerBudgetUSD = budgetUSD * 2 / 3
	}

	// Wire self-learning subsystems when requested (singleton: only create if not already set).
	ralphDir := filepath.Join(r.Path, ".ralph")
	if p.OptionalBool("enable_reflexion", false) {
		profile.EnableReflexion = true
		if !s.SessMgr.HasReflexion() {
			s.SessMgr.SetReflexionStore(session.NewReflexionStore(ralphDir))
		}
	}
	if p.OptionalBool("enable_episodic_memory", false) {
		profile.EnableEpisodicMemory = true
		if !s.SessMgr.HasEpisodicMemory() {
			s.SessMgr.SetEpisodicMemory(session.NewEpisodicMemory(ralphDir, 500, 0))
		}
	}
	if p.OptionalBool("enable_cascade", false) {
		profile.EnableCascade = true
		if !s.SessMgr.HasCascadeRouter() {
			cfg := cascadeConfigFromRepo(ctx, r.Path, ralphDir)
			s.SessMgr.SetCascadeRouter(session.NewCascadeRouter(cfg, nil, nil, ralphDir))
		}
	}
	if p.OptionalBool("enable_uncertainty", false) {
		profile.EnableUncertainty = true
	}
	if p.OptionalBool("enable_curriculum", false) {
		profile.EnableCurriculum = true
		if !s.SessMgr.HasCurriculumSorter() {
			// Wire to real episodic memory if available for richer difficulty scoring.
			var episodic session.EpisodicSource
			if em := s.SessMgr.GetEpisodicMemory(); em != nil {
				episodic = em
			}
			s.SessMgr.SetCurriculumSorter(session.NewCurriculumSorter(nil, episodic))
		}
	}

	// Self-improvement mode: override profile with SelfImprovementProfile defaults.
	if p.OptionalBool("self_improvement", false) {
		profile = session.SelfImprovementProfile()
		// Re-apply explicit overrides on top of self-improvement defaults.
		if value := p.OptionalString("planner_model", ""); value != "" {
			profile.PlannerModel = value
		}
		if value := p.OptionalString("worker_model", ""); value != "" {
			profile.WorkerModel = value
		}
		if budgetUSD := p.OptionalNumber("budget_usd", 0); budgetUSD > 0 {
			profile.PlannerBudgetUSD = budgetUSD / 4
			profile.WorkerBudgetUSD = budgetUSD * 3 / 4
		}
		// Wire all self-learning subsystems (singleton creation).
		wireSubsystems(ctx, s, s.SessMgr, ralphDir)
	}

	// Iteration and duration limits
	if maxIter := int(p.OptionalNumber("max_iterations", 0)); maxIter > 0 {
		profile.MaxIterations = maxIter
	}
	if durationHours := p.OptionalNumber("duration_hours", 0); durationHours > 0 {
		profile.MaxDurationSecs = int(durationHours * 3600)
	}

	if provider, reason := s.rerouteClaudeProviderForCacheHealth(r.Path, profile.PlannerProvider, p.OptionalString("planner_provider", "") != ""); reason != "" {
		profile.PlannerProvider = provider
	}
	if provider, reason := s.rerouteClaudeProviderForCacheHealth(r.Path, profile.WorkerProvider, p.OptionalString("worker_provider", "") != ""); reason != "" {
		profile.WorkerProvider = provider
	}
	if provider, reason := s.rerouteClaudeProviderForCacheHealth(r.Path, profile.VerifierProvider, p.OptionalString("verifier_provider", "") != ""); reason != "" {
		profile.VerifierProvider = provider
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return codedError(ErrLoopStart, fmt.Sprintf("start loop: %v", err)), nil
	}

	// Auto-drive the loop when self-improvement is enabled.
	if profile.SelfImprovement {
		go s.SessMgr.RunLoop(context.Background(), run.ID)
	}

	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "loop id required"), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return codedError(ErrLoopNotFound, fmt.Sprintf("loop not found: %s", id)), nil
	}

	result := loopResult(run)

	// WS-7: Add hygiene metrics to loop status response.
	if s.SessMgr != nil {
		result["consecutive_noops"] = s.SessMgr.ConsecutiveNoOps(id)
		result["total_pruned_this_session"] = s.SessMgr.TotalPrunedThisSession()
		// Journal entry count: derive repo path from run.
		run.Lock()
		repoPath := run.RepoPath
		run.Unlock()
		if repoPath != "" {
			result["journal_entry_count"] = session.CountJournalEntries(repoPath)

			// Wire gate report into loop status (0.6.4.4).
			obsPath := session.ObservationPath(repoPath)
			since := time.Now().Add(-24 * time.Hour)
			if obs, err := session.LoadObservations(obsPath, since); err == nil && len(obs) > 0 {
				blPath := e2e.BaselinePath(repoPath)
				if baseline, err := e2e.LoadBaseline(blPath); err == nil && baseline != nil && baseline.Aggregate != nil {
					report := e2e.EvaluateGates(obs, baseline, e2e.DefaultGateThresholds())
					result["gate_report"] = report
					result["gate_summary"] = e2e.FormatGateReport(report)
				}
			}
		}
	}

	return jsonResult(result), nil
}

func (s *Server) handleLoopStep(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "loop id required"), nil
	}

	if err := s.SessMgr.StepLoop(ctx, id); err != nil {
		return codedError(ErrLoopStart, fmt.Sprintf("step loop: %v", err)), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return codedError(ErrLoopNotFound, fmt.Sprintf("loop not found after step: %s", id)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "loop id required"), nil
	}

	if err := s.SessMgr.StopLoop(id); err != nil {
		return codedError(ErrLoopNotFound, fmt.Sprintf("loop %s not found — use ralphglasses_loop_status to check active loops", id)), nil
	}
	return textResult(fmt.Sprintf("Stopped loop %s", id)), nil
}

func (s *Server) handleLoopPrune(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	olderThanHours := getNumberArg(req, "older_than_hours", 72)
	statusesStr := getStringArg(req, "statuses")
	if statusesStr == "" {
		statusesStr = "pending,failed"
	}
	dryRun := true
	if v, ok := req.GetArguments()["dry_run"]; ok {
		if b, isBool := v.(bool); isBool {
			dryRun = b
		}
	}

	olderThan := time.Duration(olderThanHours) * time.Hour
	statuses := strings.Split(statusesStr, ",")

	// Resolve loop state directory from session manager.
	loopDir := s.SessMgr.LoopStateDir()
	if loopDir == "" {
		return jsonResult(map[string]any{
			"pruned":  0,
			"dry_run": dryRun,
			"message": "no loop state directory configured",
		}), nil
	}

	// If a repo filter is provided, we still prune from the central loop dir
	// but only files whose repo_name matches.
	repoFilter := getStringArg(req, "repo")

	if repoFilter != "" {
		pruned, err := pruneLoopRunsFiltered(loopDir, olderThan, statuses, repoFilter, dryRun)
		if err != nil {
			return codedError(ErrFilesystem, fmt.Sprintf("prune loop runs: %v", err)), nil
		}
		return jsonResult(map[string]any{
			"pruned":  pruned,
			"dry_run": dryRun,
			"repo":    repoFilter,
			"message": fmt.Sprintf("pruned %d loop run files", pruned),
		}), nil
	}

	pruned, err := session.PruneLoopRuns(loopDir, olderThan, statuses, dryRun)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("prune loop runs: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"pruned":  pruned,
		"dry_run": dryRun,
		"message": fmt.Sprintf("pruned %d loop run files", pruned),
	}), nil
}

// pruneLoopRunsFiltered wraps PruneLoopRuns but adds a repo name filter.
// It reads each file to check the repo_name field before considering it for pruning.
func pruneLoopRunsFiltered(loopDir string, olderThan time.Duration, statuses []string, repoFilter string, dryRun bool) (int, error) {
	// Use a temporary filtered directory approach: just call PruneLoopRuns
	// with the standard logic — the repo filter is handled by re-reading
	// the same files and skipping non-matching repos.
	return session.PruneLoopRunsFiltered(loopDir, olderThan, statuses, repoFilter, dryRun)
}

func loopResult(run *session.LoopRun) map[string]any {
	run.Lock()
	defer run.Unlock()

	iterations := append([]session.LoopIteration(nil), run.Iterations...)
	return map[string]any{
		"id":         run.ID,
		"repo":       run.RepoName,
		"repo_path":  run.RepoPath,
		"status":     run.Status,
		"last_error": run.LastError,
		"profile":    run.Profile,
		"iterations": iterations,
		"created_at": run.CreatedAt,
		"updated_at": run.UpdatedAt,
	}
}
