package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleAutomationPolicy(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.SessMgr == nil {
		return codedError(ErrNotRunning, "session manager not initialized"), nil
	}

	pp := NewParamParserFromRequest(req)
	repo, errResult := pp.StringErr("repo")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	action := pp.OptionalString("action", "get")
	ctrl := s.SessMgr.EnsureSubscriptionAutomation(repoPath)
	if ctrl == nil {
		return codedError(ErrInternal, "automation controller unavailable"), nil
	}

	switch action {
	case "get":
		return fleetJSON(map[string]any{
			"policy":         ctrl.Policy(),
			"status":         ctrl.Status(),
			"recommendation": ctrl.RecommendPolicy(),
		})
	case "recommend":
		return fleetJSON(ctrl.RecommendPolicy())
	case "set":
		policy := ctrl.Policy()
		args := argsMap(req)
		if _, ok := args["enabled"]; ok {
			policy.Enabled = pp.OptionalBool("enabled", policy.Enabled)
		}
		if _, ok := args["provider"]; ok {
			policy.Provider = session.Provider(pp.OptionalString("provider", string(policy.Provider)))
		}
		if _, ok := args["timezone"]; ok {
			policy.Timezone = pp.OptionalString("timezone", policy.Timezone)
		}
		if _, ok := args["reset_cron"]; ok {
			policy.ResetCron = pp.OptionalString("reset_cron", policy.ResetCron)
		}
		if _, ok := args["reset_anchor"]; ok {
			policy.ResetAnchor = pp.OptionalString("reset_anchor", policy.ResetAnchor)
		}
		if _, ok := args["reset_window_hours"]; ok {
			policy.ResetWindowHours = int(getNumberArg(req, "reset_window_hours", float64(policy.ResetWindowHours)))
		}
		if _, ok := args["window_budget_usd"]; ok {
			policy.WindowBudgetUSD = getNumberArg(req, "window_budget_usd", policy.WindowBudgetUSD)
		}
		if _, ok := args["target_utilization_pct"]; ok {
			policy.TargetUtilizationPct = getNumberArg(req, "target_utilization_pct", policy.TargetUtilizationPct)
		}
		if _, ok := args["resume_backoff_minutes"]; ok {
			policy.ResumeBackoffMinutes = int(getNumberArg(req, "resume_backoff_minutes", float64(policy.ResumeBackoffMinutes)))
		}
		if _, ok := args["default_model"]; ok {
			policy.DefaultModel = pp.OptionalString("default_model", policy.DefaultModel)
		}
		if _, ok := args["default_task_budget_usd"]; ok {
			policy.DefaultTaskBudgetUSD = getNumberArg(req, "default_task_budget_usd", policy.DefaultTaskBudgetUSD)
		}
		if _, ok := args["default_task_max_turns"]; ok {
			policy.DefaultTaskMaxTurns = int(getNumberArg(req, "default_task_max_turns", float64(policy.DefaultTaskMaxTurns)))
		}

		if err := ctrl.SetPolicy(policy); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid automation policy: %v", err)), nil
		}
		return fleetJSON(map[string]any{
			"policy":         ctrl.Policy(),
			"status":         ctrl.Status(),
			"recommendation": ctrl.RecommendPolicy(),
		})
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unsupported action: %s", action)), nil
	}
}

func (s *Server) handleAutomationQueue(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.SessMgr == nil {
		return codedError(ErrNotRunning, "session manager not initialized"), nil
	}

	pp := NewParamParserFromRequest(req)
	repo, errResult := pp.StringErr("repo")
	if errResult != nil {
		return errResult, nil
	}
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	action := pp.OptionalString("action", "list")
	ctrl := s.SessMgr.EnsureSubscriptionAutomation(repoPath)
	if ctrl == nil {
		return codedError(ErrInternal, "automation controller unavailable"), nil
	}

	switch action {
	case "list":
		return fleetJSON(map[string]any{
			"queue":  ctrl.ListQueue(),
			"count":  len(ctrl.ListQueue()),
			"status": ctrl.Status(),
		})
	case "enqueue":
		prompt, errResult := pp.StringErr("prompt")
		if errResult != nil {
			return errResult, nil
		}
		item, err := ctrl.Enqueue(session.AutomationQueueItem{
			Prompt:    prompt,
			Provider:  session.Provider(pp.OptionalString("provider", string(session.ProviderCodex))),
			Model:     pp.OptionalString("model", ""),
			BudgetUSD: getNumberArg(req, "budget_usd", 0),
			MaxTurns:  int(getNumberArg(req, "max_turns", 0)),
			Priority:  int(getNumberArg(req, "priority", 5)),
			Source:    pp.OptionalString("source", "manual"),
		})
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("enqueue failed: %v", err)), nil
		}
		return fleetJSON(map[string]any{
			"item":   item,
			"status": ctrl.Status(),
		})
	case "remove":
		id, errResult := pp.StringErr("id")
		if errResult != nil {
			return errResult, nil
		}
		if !ctrl.RemoveQueueItem(id) {
			return codedError(ErrInvalidParams, fmt.Sprintf("queue item not found: %s", id)), nil
		}
		return fleetJSON(map[string]any{
			"removed_id": id,
			"status":     ctrl.Status(),
		})
	case "reprioritize":
		id, errResult := pp.StringErr("id")
		if errResult != nil {
			return errResult, nil
		}
		item, err := ctrl.ReprioritizeQueueItem(id, int(getNumberArg(req, "priority", 5)))
		if err != nil {
			return codedError(ErrInvalidParams, err.Error()), nil
		}
		return fleetJSON(map[string]any{
			"item":   item,
			"status": ctrl.Status(),
		})
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unsupported action: %s", action)), nil
	}
}
