package mcpserver

import (
	"context"
	"fmt"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/mark3labs/mcp-go/mcp"
)

// handleCycleCreate creates a new CycleRun via the Manager.
func (s *Server) handleCycleCreate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo required"), nil
	}
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	name := getStringArg(req, "name")
	if name == "" {
		name = "cycle"
	}
	objective := getStringArg(req, "objective")
	if objective == "" {
		return codedError(ErrInvalidParams, "objective required"), nil
	}

	var criteria []string
	if c := getStringArg(req, "criteria"); c != "" {
		criteria = splitCSV(c)
	}

	cycle, err := s.SessMgr.CreateCycle(repoPath, name, objective, criteria)
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"cycle_id":  cycle.ID,
		"name":      cycle.Name,
		"phase":     string(cycle.Phase),
		"objective": cycle.Objective,
		"status":    "created",
	}), nil
}

// handleCycleAdvance transitions a cycle to the next phase.
func (s *Server) handleCycleAdvance(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	cycleID := getStringArg(req, "cycle_id")
	var cycle *session.CycleRun
	var err error
	if cycleID != "" {
		cycle, err = s.SessMgr.GetCycle(repoPath, cycleID)
	} else {
		cycle, err = s.SessMgr.GetActiveCycle(repoPath)
	}
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	if cycle == nil {
		return codedError(ErrInvalidParams, "no active cycle found"), nil
	}

	previousPhase := cycle.Phase
	if err := s.SessMgr.AdvanceCycle(cycle); err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"cycle_id":       cycle.ID,
		"previous_phase": string(previousPhase),
		"phase":          string(cycle.Phase),
		"status":         "advanced",
	}), nil
}

// handleCycleStatus returns the current state of a cycle.
func (s *Server) handleCycleStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	cycleID := getStringArg(req, "cycle_id")
	var cycle *session.CycleRun
	var err error
	if cycleID != "" {
		cycle, err = s.SessMgr.GetCycle(repoPath, cycleID)
	} else {
		cycle, err = s.SessMgr.GetActiveCycle(repoPath)
	}
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	if cycle == nil {
		return jsonResult(map[string]any{
			"status":  "none",
			"message": "no active cycle",
		}), nil
	}

	taskSummary := make([]map[string]any, len(cycle.Tasks))
	for i, t := range cycle.Tasks {
		taskSummary[i] = map[string]any{
			"title":   t.Title,
			"status":  t.Status,
			"source":  t.Source,
			"loop_id": t.LoopID,
		}
	}

	result := map[string]any{
		"cycle_id":  cycle.ID,
		"name":      cycle.Name,
		"phase":     string(cycle.Phase),
		"objective": cycle.Objective,
		"tasks":     taskSummary,
		"loop_ids":  cycle.LoopIDs,
		"findings":  len(cycle.Findings),
		"error":     cycle.Error,
		"created":   cycle.CreatedAt.Format("2006-01-02T15:04:05Z"),
		"updated":   cycle.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
	if cycle.Synthesis != nil {
		result["synthesis"] = map[string]any{
			"summary":        cycle.Synthesis.Summary,
			"accomplished":   cycle.Synthesis.Accomplished,
			"remaining":      cycle.Synthesis.Remaining,
			"next_objective": cycle.Synthesis.NextObjective,
		}
	}
	result["status"] = "ok"
	return jsonResult(result), nil
}

// handleCycleFail marks a cycle as failed.
func (s *Server) handleCycleFail(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	cycleID := getStringArg(req, "cycle_id")
	errMsg := getStringArg(req, "error")
	if errMsg == "" {
		errMsg = "manually failed"
	}

	var cycle *session.CycleRun
	var err error
	if cycleID != "" {
		cycle, err = s.SessMgr.GetCycle(repoPath, cycleID)
	} else {
		cycle, err = s.SessMgr.GetActiveCycle(repoPath)
	}
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	if cycle == nil {
		return codedError(ErrInvalidParams, "no active cycle found"), nil
	}

	if err := s.SessMgr.FailCycle(cycle, errMsg); err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	return jsonResult(map[string]any{
		"cycle_id": cycle.ID,
		"phase":    string(cycle.Phase),
		"error":    cycle.Error,
		"status":   "failed",
	}), nil
}

// handleCycleList lists all cycles for a repo.
func (s *Server) handleCycleList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	cycles, err := s.SessMgr.ListCycles(repoPath)
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	limit := int(getNumberArg(req, "limit", 20))
	if limit > 0 && len(cycles) > limit {
		cycles = cycles[:limit]
	}

	items := make([]map[string]any, len(cycles))
	for i, c := range cycles {
		items[i] = map[string]any{
			"cycle_id":   c.ID,
			"name":       c.Name,
			"phase":      string(c.Phase),
			"objective":  c.Objective,
			"tasks":      len(c.Tasks),
			"findings":   len(c.Findings),
			"created":    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"updated":    c.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
		if c.Error != "" {
			items[i]["error"] = c.Error
		}
	}

	return jsonResult(map[string]any{
		"cycles": items,
		"total":  len(items),
		"status": "ok",
	}), nil
}

// handleCycleSynthesize sets the synthesis on a cycle and advances to complete.
func (s *Server) handleCycleSynthesize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	cycleID := getStringArg(req, "cycle_id")
	var cycle *session.CycleRun
	var err error
	if cycleID != "" {
		cycle, err = s.SessMgr.GetCycle(repoPath, cycleID)
	} else {
		cycle, err = s.SessMgr.GetActiveCycle(repoPath)
	}
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	if cycle == nil {
		return codedError(ErrInvalidParams, "no active cycle found"), nil
	}

	summary := getStringArg(req, "summary")
	if summary == "" {
		return codedError(ErrInvalidParams, "summary required"), nil
	}

	synthesis := session.CycleSynthesis{
		Summary:       summary,
		Accomplished:  splitCSV(getStringArg(req, "accomplished")),
		Remaining:     splitCSV(getStringArg(req, "remaining")),
		NextObjective: getStringArg(req, "next_objective"),
		Patterns:      splitCSV(getStringArg(req, "patterns")),
	}

	if err := s.SessMgr.SetCycleSynthesis(cycle, synthesis); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("set synthesis: %v", err)), nil
	}

	// Advance to complete.
	if err := s.SessMgr.AdvanceCycle(cycle); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("advance to complete: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"cycle_id":       cycle.ID,
		"phase":          string(cycle.Phase),
		"synthesis":      synthesis,
		"status":         "synthesized",
	}), nil
}

// handleCycleRun drives a full R&D cycle synchronously through all phases.
func (s *Server) handleCycleRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo_path")
	if repo == "" {
		repo = getStringArg(req, "repo")
	}
	if repo == "" {
		return codedError(ErrInvalidParams, "repo_path required"), nil
	}
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	name := getStringArg(req, "name")
	if name == "" {
		name = "cycle"
	}
	objective := getStringArg(req, "objective")
	if objective == "" {
		return codedError(ErrInvalidParams, "objective required"), nil
	}

	var criteria []string
	if c := getStringArg(req, "criteria"); c != "" {
		criteria = splitCSV(c)
	}

	maxTasks := int(getNumberArg(req, "max_tasks", 10))

	cycle, err := s.SessMgr.RunCycle(ctx, repoPath, name, objective, criteria, maxTasks)
	if err != nil {
		result := map[string]any{
			"status": "failed",
			"error":  err.Error(),
		}
		if cycle != nil {
			result["cycle_id"] = cycle.ID
			result["phase"] = string(cycle.Phase)
		}
		return jsonResult(result), nil
	}

	taskSummary := make([]map[string]any, len(cycle.Tasks))
	for i, t := range cycle.Tasks {
		taskSummary[i] = map[string]any{
			"title":   t.Title,
			"status":  t.Status,
			"loop_id": t.LoopID,
		}
	}

	result := map[string]any{
		"cycle_id":  cycle.ID,
		"name":      cycle.Name,
		"phase":     string(cycle.Phase),
		"objective": cycle.Objective,
		"tasks":     taskSummary,
		"findings":  len(cycle.Findings),
		"status":    "complete",
	}
	if cycle.Synthesis != nil {
		result["synthesis"] = map[string]any{
			"summary":        cycle.Synthesis.Summary,
			"accomplished":   cycle.Synthesis.Accomplished,
			"remaining":      cycle.Synthesis.Remaining,
			"next_objective": cycle.Synthesis.NextObjective,
			"patterns":       cycle.Synthesis.Patterns,
		}
	}
	return jsonResult(result), nil
}
