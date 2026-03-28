package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) handleFindingToTask(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	findingID := getStringArg(req, "finding_id")
	if findingID == "" {
		return codedError(ErrInvalidParams, "finding_id required"), nil
	}
	scratchpadName := getStringArg(req, "scratchpad_name")
	if scratchpadName == "" {
		return codedError(ErrInvalidParams, "scratchpad_name required"), nil
	}

	// TODO: Read scratchpad, find finding by ID, generate task spec
	result := map[string]any{
		"finding_id":       findingID,
		"scratchpad":       scratchpadName,
		"title":            fmt.Sprintf("Task from %s", findingID),
		"description":      "Auto-generated task stub",
		"difficulty_score":  0.5,
		"provider_hint":    "claude",
		"estimated_cost":   0.10,
		"status":           "stub",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleBaseline(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	if repo == "" {
		return codedError(ErrInvalidParams, "repo required"), nil
	}
	metricsStr := getStringArg(req, "metrics")

	// Parse requested metrics or use defaults.
	metricNames := []string{"test_pass_rate", "coverage_pct", "vet_clean", "build_ok", "lint_score"}
	if metricsStr != "" {
		metricNames = strings.Split(metricsStr, ",")
		for i := range metricNames {
			metricNames[i] = strings.TrimSpace(metricNames[i])
		}
	}

	baselineID := fmt.Sprintf("baseline-%s-%d", repo, time.Now().Unix())

	// Build zero-value snapshot for each metric.
	snapshot := make(map[string]float64, len(metricNames))
	for _, m := range metricNames {
		snapshot[m] = 0
	}

	// TODO: run `go test -count=1`, parse coverage, record to `.ralph/cycle_baselines/`
	result := map[string]any{
		"baseline_id": baselineID,
		"repo":        repo,
		"metrics":     snapshot,
		"captured_at": time.Now().UTC().Format(time.RFC3339),
		"status":      "stub",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCyclePlan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	previousCycleID := getStringArg(req, "previous_cycle_id")
	maxTasks := int(getNumberArg(req, "max_tasks", 10))
	budget := getNumberArg(req, "budget", 5.0)

	planID := fmt.Sprintf("plan-%d", time.Now().Unix())

	// TODO: read scratchpads, filter unresolved findings, sort by recurrence and severity
	result := map[string]any{
		"plan_id":           planID,
		"previous_cycle_id": previousCycleID,
		"tasks":             []any{},
		"constraints": map[string]any{
			"max_tasks":  maxTasks,
			"budget_usd": budget,
		},
		"status": "stub",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleMerge(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	worktreePaths := getStringArg(req, "worktree_paths")
	if worktreePaths == "" {
		return codedError(ErrInvalidParams, "worktree_paths required"), nil
	}
	conflictStrategy := getStringArg(req, "conflict_strategy")
	if conflictStrategy == "" {
		conflictStrategy = "manual"
	}

	paths := splitCSV(worktreePaths)

	// TODO: git merge-tree or sequential merge with conflict detection
	result := map[string]any{
		"merge_status":      "pending",
		"worktree_count":    len(paths),
		"worktree_paths":    paths,
		"conflict_strategy": conflictStrategy,
		"conflicts":         []any{},
		"status":            "stub",
	}
	return jsonResult(result), nil
}

func (s *Server) handleCycleSchedule(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	cronExpr := getStringArg(req, "cron_expr")
	if cronExpr == "" {
		return codedError(ErrInvalidParams, "cron_expr required"), nil
	}
	cycleConfig := getStringArg(req, "cycle_config")

	scheduleID := fmt.Sprintf("sched-%d", time.Now().Unix())
	nextRun := time.Now().Add(24 * time.Hour).UTC().Format(time.RFC3339)

	// TODO: write schedule to `.ralph/schedules/`, implement cron parsing
	result := map[string]any{
		"schedule_id":  scheduleID,
		"cron_expr":    cronExpr,
		"cycle_config": cycleConfig,
		"next_run":     nextRun,
		"status":       "created",
	}
	return jsonResult(result), nil
}
