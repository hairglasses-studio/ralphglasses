package mcpserver

import (
	"context"
	"fmt"
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

	profile := session.SelfImprovementProfile()

	// Apply explicit overrides
	if budgetUSD := getNumberArg(req, "budget_usd", 0); budgetUSD > 0 {
		profile.PlannerBudgetUSD = budgetUSD / 4
		profile.WorkerBudgetUSD = budgetUSD * 3 / 4
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
	wireSubsystems(s, s.SessMgr, ralphDir)

	// Wire prompt enhancer
	if s.SessMgr.Enhancer == nil {
		s.SessMgr.Enhancer = s.getEngine()
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return codedError(ErrLoopStart, fmt.Sprintf("start self-improvement loop: %v", err)), nil
	}

	// Drive the loop to completion in a background goroutine.
	go func() {
		_ = s.SessMgr.RunLoop(context.Background(), run.ID)
	}()

	return mcp.NewToolResultText(fmt.Sprintf(
		"Self-improvement loop started: id=%s repo=%s budget=$%.0f max_iterations=%d",
		run.ID, repoName,
		profile.PlannerBudgetUSD+profile.WorkerBudgetUSD, profile.MaxIterations,
	)), nil
}
