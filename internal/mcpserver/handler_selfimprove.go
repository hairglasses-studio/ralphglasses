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

	// Wire self-learning subsystems (same pattern as handler_loop.go)
	ralphDir := filepath.Join(r.Path, ".ralph")
	if !s.SessMgr.HasReflexion() {
		s.SessMgr.SetReflexionStore(session.NewReflexionStore(ralphDir))
	}
	if !s.SessMgr.HasEpisodicMemory() {
		s.SessMgr.SetEpisodicMemory(session.NewEpisodicMemory(ralphDir, 500, 0))
	}
	if !s.SessMgr.HasCascadeRouter() {
		cfg := session.DefaultCascadeConfig()
		s.SessMgr.SetCascadeRouter(session.NewCascadeRouter(cfg, nil, nil, ralphDir))
	}
	if !s.SessMgr.HasCurriculumSorter() {
		var episodic session.EpisodicSource
		if em := s.SessMgr.GetEpisodicMemory(); em != nil {
			episodic = em
		}
		s.SessMgr.SetCurriculumSorter(session.NewCurriculumSorter(nil, episodic))
	}

	// Wire prompt enhancer
	if s.SessMgr.Enhancer == nil {
		s.SessMgr.Enhancer = s.getEngine()
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return codedError(ErrLoopStart, fmt.Sprintf("start self-improvement loop: %v", err)), nil
	}

	return mcp.NewToolResultText(fmt.Sprintf(
		"Self-improvement loop started: id=%s repo=%s budget=$%.0f max_iterations=%d",
		run.ID, repoName,
		profile.PlannerBudgetUSD+profile.WorkerBudgetUSD, profile.MaxIterations,
	)), nil
}
