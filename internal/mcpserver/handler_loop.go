package mcpserver

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleLoopStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	profile := session.DefaultLoopProfile()
	if value := getStringArg(req, "planner_model"); value != "" {
		profile.PlannerModel = value
	}
	if value := getStringArg(req, "worker_model"); value != "" {
		profile.WorkerModel = value
	}
	if value := getStringArg(req, "verifier_model"); value != "" {
		profile.VerifierModel = value
	}
	if value := getStringArg(req, "worktree_policy"); value != "" {
		profile.WorktreePolicy = value
	}
	if value := int(getNumberArg(req, "retry_limit", float64(profile.RetryLimit))); value != profile.RetryLimit {
		profile.RetryLimit = value
	}
	if value := int(getNumberArg(req, "max_concurrent_workers", float64(profile.MaxConcurrentWorkers))); value != profile.MaxConcurrentWorkers {
		profile.MaxConcurrentWorkers = value
	}
	if commands := splitLines(getStringArg(req, "verify_commands")); len(commands) > 0 {
		profile.VerifyCommands = commands
	}
	if pp := getStringArg(req, "planner_provider"); pp != "" {
		profile.PlannerProvider = session.Provider(pp)
	}
	if wp := getStringArg(req, "worker_provider"); wp != "" {
		profile.WorkerProvider = session.Provider(wp)
	}
	if vp := getStringArg(req, "verifier_provider"); vp != "" {
		profile.VerifierProvider = session.Provider(vp)
	}

	// Wire self-learning subsystems when requested.
	ralphDir := filepath.Join(r.Path, ".ralph")
	if getBoolArg(req, "enable_reflexion") {
		profile.EnableReflexion = true
		s.SessMgr.SetReflexionStore(session.NewReflexionStore(ralphDir))
	}
	if getBoolArg(req, "enable_episodic_memory") {
		profile.EnableEpisodicMemory = true
		s.SessMgr.SetEpisodicMemory(session.NewEpisodicMemory(ralphDir, 500))
	}
	if getBoolArg(req, "enable_cascade") {
		profile.EnableCascade = true
		cfg := session.DefaultCascadeConfig()
		s.SessMgr.SetCascadeRouter(session.NewCascadeRouter(cfg, nil, nil, ralphDir))
	}
	if getBoolArg(req, "enable_uncertainty") {
		profile.EnableUncertainty = true
	}
	if getBoolArg(req, "enable_curriculum") {
		profile.EnableCurriculum = true
		s.SessMgr.SetCurriculumSorter(session.NewCurriculumSorter(nil, nil))
	}

	// Wire prompt enhancer into session manager for loop integration
	if s.SessMgr.Enhancer == nil {
		s.SessMgr.Enhancer = s.getEngine()
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return errResult(fmt.Sprintf("start loop: %v", err)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return errResult(fmt.Sprintf("loop not found: %s", id)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStep(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	if err := s.SessMgr.StepLoop(ctx, id); err != nil {
		return errResult(fmt.Sprintf("step loop: %v", err)), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return errResult(fmt.Sprintf("loop not found after step: %s", id)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	if err := s.SessMgr.StopLoop(id); err != nil {
		return errResult(fmt.Sprintf("stop loop: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped loop %s", id)), nil
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
