package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSelfImprove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
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

	// Larger budgets use the cheaper Codex planner profile for longer unattended
	// runs. Smaller budgets use the stronger Codex-first planner profile.
	budgetUSD := getNumberArg(req, "budget_usd", 0)
	var profile session.LoopProfile
	if budgetUSD > 20 {
		profile = session.BudgetOptimizedSelfImprovementProfile(budgetUSD)
	} else {
		profile = session.SelfImprovementProfile()
		if budgetUSD > 0 {
			profile.PlannerBudgetUSD = budgetUSD / 4
			profile.WorkerBudgetUSD = budgetUSD * 3 / 4
		}
	}

	profile.MaxIterations = int(getNumberArg(req, "max_iterations", 5))
	if profile.MaxIterations <= 0 {
		profile.MaxIterations = 5
	}
	durationHours := getNumberArg(req, "duration_hours", 4)
	if durationHours > 0 {
		profile.MaxDurationSecs = int(durationHours * 3600)
	}

	// Wire self-learning subsystems (shared helper).
	ralphDir := filepath.Join(r.Path, ".ralph")
	wireSubsystems(ctx, s, s.SessMgr, ralphDir)

	traceLevel := getStringArg(req, "trace_level")
	if traceLevel == "" {
		traceLevel = "summary"
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return codedError(ErrLoopStart, fmt.Sprintf("start self-improvement loop: %v", err)), nil
	}

	// Drive the loop to completion in a background goroutine.
	go func() {
		if err := s.SessMgr.RunLoop(context.Background(), run.ID); err != nil {
			slog.Error("self-improve RunLoop failed", "error", err, "loop_id", run.ID)
		}
	}()

	appliedBudget := profile.PlannerBudgetUSD + profile.WorkerBudgetUSD

	result := map[string]any{
		"message":            fmt.Sprintf("Self-improvement loop started: id=%s repo=%s budget=$%.0f max_iterations=%d", run.ID, repoName, appliedBudget, profile.MaxIterations),
		"loop_id":            run.ID,
		"repo":               repoName,
		"applied_budget_usd": appliedBudget,
		"max_iterations":     profile.MaxIterations,
	}

	if traceLevel != "none" {
		result["trace"] = map[string]any{
			"planner_budget_usd": profile.PlannerBudgetUSD,
			"worker_budget_usd":  profile.WorkerBudgetUSD,
			"max_iterations":     profile.MaxIterations,
			"max_duration_secs":  profile.MaxDurationSecs,
			"enhancer_wired":     s.SessMgr.Enhancer != nil,
		}
	}

	return jsonResult(result), nil
}
