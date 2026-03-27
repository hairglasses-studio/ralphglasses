package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// InitFleetTools initializes fleet infrastructure on the Server.
func (s *Server) InitFleetTools(coord *fleet.Coordinator, client *fleet.Client, hitl *session.HITLTracker, decisions *session.DecisionLog, feedback *session.FeedbackAnalyzer) {
	s.FleetCoordinator = coord
	s.FleetClient = client
	s.HITLTracker = hitl
	s.DecisionLog = decisions
	s.FeedbackAnalyzer = feedback
}

// InitSelfImprovement initializes HITL and feedback infrastructure (works without fleet).
func (s *Server) InitSelfImprovement(stateDir string, autonomyLevel int) {
	if s.HITLTracker == nil {
		s.HITLTracker = session.NewHITLTracker(stateDir)
	}
	if s.DecisionLog == nil {
		s.DecisionLog = session.NewDecisionLog(stateDir, session.AutonomyLevel(autonomyLevel))
	}
	if s.FeedbackAnalyzer == nil {
		s.FeedbackAnalyzer = session.NewFeedbackAnalyzer(stateDir, 5)
	}
	if s.AutoOptimizer == nil {
		s.AutoOptimizer = session.NewAutoOptimizer(s.FeedbackAnalyzer, s.DecisionLog, s.HITLTracker, nil)
	}
}

// WireAutoOptimizer attaches the auto-optimizer to a session manager and creates
// the AutoRecovery handler. Call after InitSelfImprovement.
func (s *Server) WireAutoOptimizer(mgr *session.Manager) {
	if s.AutoOptimizer == nil || mgr == nil {
		return
	}
	recovery := session.NewAutoRecovery(mgr, s.DecisionLog, s.HITLTracker, session.DefaultAutoRecoveryConfig())
	s.AutoOptimizer = session.NewAutoOptimizer(s.FeedbackAnalyzer, s.DecisionLog, s.HITLTracker, recovery)
	mgr.SetAutoOptimizer(s.AutoOptimizer)
}

func (s *Server) handleFleetSubmit(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil && s.FleetClient == nil {
		return codedError(ErrFleetNotRunning, "fleet server not active — start with 'ralphglasses mcp --fleet'"), nil
	}

	repo := getStringArg(req, "repo")
	prompt := getStringArg(req, "prompt")
	provider := getStringArg(req, "provider")
	budget := getNumberArg(req, "budget", 5)
	priority := int(getNumberArg(req, "priority", 5))

	if repo == "" || prompt == "" {
		return codedError(ErrInvalidParams, "repo and prompt required"), nil
	}

	item := &fleet.WorkItem{
		Type:         fleet.WorkTypeSession,
		RepoName:     repo,
		Prompt:       prompt,
		Provider:     session.Provider(provider),
		MaxBudgetUSD: budget,
		Priority:     priority,
		MaxRetries:   2,
	}

	if s.FleetCoordinator != nil {
		if err := s.FleetCoordinator.SubmitWork(item); err != nil {
			return codedError(ErrInternal, err.Error()), nil
		}
		return fleetJSON(map[string]any{
			"work_item_id": item.ID,
			"status":       "pending",
			"queue":        "local_coordinator",
		})
	}

	id, err := s.FleetClient.SubmitWork(context.Background(), *item)
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	return fleetJSON(map[string]any{
		"work_item_id": id,
		"status":       "pending",
		"queue":        "remote_coordinator",
	})
}

func (s *Server) handleFleetBudget(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil && s.FleetClient == nil {
		return codedError(ErrFleetNotRunning, "fleet server not active — start with 'ralphglasses mcp --fleet'"), nil
	}

	newLimit := getNumberArg(req, "limit", 0)

	if s.FleetCoordinator != nil {
		if newLimit > 0 {
			s.FleetCoordinator.SetBudgetLimit(newLimit)
		}
		state := s.FleetCoordinator.GetFleetState()
		return fleetJSON(map[string]any{
			"budget_usd":  state.BudgetUSD,
			"spent_usd":   state.TotalSpentUSD,
			"remaining":   state.BudgetUSD - state.TotalSpentUSD,
			"active_work": state.ActiveWork,
			"queue_depth": state.QueueDepth,
		})
	}

	state, err := s.FleetClient.FleetState(context.Background())
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	return fleetJSON(map[string]any{
		"budget_usd":  state.BudgetUSD,
		"spent_usd":   state.TotalSpentUSD,
		"remaining":   state.BudgetUSD - state.TotalSpentUSD,
		"active_work": state.ActiveWork,
		"queue_depth": state.QueueDepth,
	})
}

func (s *Server) handleFleetWorkers(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil && s.FleetClient == nil {
		return codedError(ErrFleetNotRunning, "fleet server not active — start with 'ralphglasses mcp --fleet'"), nil
	}

	action := getStringArg(req, "action")
	workerID := getStringArg(req, "worker_id")

	// Handle worker actions (coordinator-only)
	if action != "" {
		if s.FleetCoordinator == nil {
			return codedError(ErrInvalidParams, "worker actions require local coordinator"), nil
		}
		if workerID == "" {
			return codedError(ErrInvalidParams, "worker_id required for action"), nil
		}

		var err error
		switch action {
		case "pause":
			err = s.FleetCoordinator.PauseWorker(workerID)
		case "resume":
			err = s.FleetCoordinator.ResumeWorker(workerID)
		case "drain":
			err = s.FleetCoordinator.DrainWorker(workerID)
		default:
			return codedError(ErrInvalidParams, fmt.Sprintf("unknown action %q (use pause, resume, or drain)", action)), nil
		}
		if err != nil {
			return codedError(ErrInternal, err.Error()), nil
		}

		result := map[string]any{
			"status":    "ok",
			"action":    action,
			"worker_id": workerID,
		}
		if action == "drain" {
			result["drained"] = s.FleetCoordinator.IsWorkerDrained(workerID)
		}
		return fleetJSON(result)
	}

	// Default: list workers
	if s.FleetCoordinator != nil {
		state := s.FleetCoordinator.GetFleetState()
		return fleetJSON(map[string]any{
			"workers":     state.Workers,
			"total":       len(state.Workers),
			"active_work": state.ActiveWork,
		})
	}

	state, err := s.FleetClient.FleetState(context.Background())
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}
	return fleetJSON(map[string]any{
		"workers":     state.Workers,
		"total":       len(state.Workers),
		"active_work": state.ActiveWork,
	})
}

func (s *Server) handleHITLScore(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.HITLTracker == nil {
		return codedError(ErrNotRunning, "HITL tracking not initialized — run InitSelfImprovement first"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	window := time.Duration(hours * float64(time.Hour))

	snap := s.HITLTracker.CurrentScore(window)
	return fleetJSON(snap)
}

func (s *Server) handleHITLHistory(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.HITLTracker == nil {
		return codedError(ErrNotRunning, "HITL tracking not initialized"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	limit := int(getNumberArg(req, "limit", 50))
	window := time.Duration(hours * float64(time.Hour))

	events := s.HITLTracker.History(window, limit)
	return fleetJSON(map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func (s *Server) handleAutonomyLevel(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.DecisionLog == nil {
		return codedError(ErrNotRunning, "autonomy system not initialized"), nil
	}

	levelStr := getStringArg(req, "level")
	if levelStr != "" {
		var level session.AutonomyLevel
		switch levelStr {
		case "0", "observe":
			level = session.LevelObserve
		case "1", "auto-recover":
			level = session.LevelAutoRecover
		case "2", "auto-optimize":
			level = session.LevelAutoOptimize
		case "3", "full-autonomy":
			level = session.LevelFullAutonomy
		default:
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid level: %s (use 0-3 or name)", levelStr)), nil
		}
		s.DecisionLog.SetLevel(level)
	}

	current := s.DecisionLog.Level()
	return fleetJSON(map[string]any{
		"level":      int(current),
		"level_name": current.String(),
		"stats":      s.DecisionLog.Stats(),
	})
}

func (s *Server) handleAutonomyDecisions(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.DecisionLog == nil {
		return codedError(ErrNotRunning, "autonomy system not initialized"), nil
	}

	limit := int(getNumberArg(req, "limit", 20))

	decisions := s.DecisionLog.Recent(limit)
	return fleetJSON(map[string]any{
		"decisions": decisions,
		"count":     len(decisions),
		"stats":     s.DecisionLog.Stats(),
	})
}

func (s *Server) handleAutonomyOverride(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.DecisionLog == nil {
		return codedError(ErrNotRunning, "autonomy system not initialized"), nil
	}

	decisionID := getStringArg(req, "decision_id")
	if decisionID == "" {
		return codedError(ErrInvalidParams, "decision_id required"), nil
	}

	details := getStringArg(req, "details")
	if details == "" {
		details = "manually overridden by user"
	}

	s.DecisionLog.RecordOutcome(decisionID, session.DecisionOutcome{
		EvaluatedAt: time.Now(),
		Overridden:  true,
		Details:     details,
	})

	if s.HITLTracker != nil {
		s.HITLTracker.RecordManual(session.MetricAutoDecisionOverride, "", "", "overrode decision "+decisionID)
	}

	return fleetJSON(map[string]string{"status": "overridden", "decision_id": decisionID})
}

func (s *Server) handleFeedbackProfiles(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FeedbackAnalyzer == nil {
		return codedError(ErrNotRunning, "feedback analyzer not initialized"), nil
	}

	profileType := getStringArg(req, "type")

	result := map[string]any{}

	if profileType == "" || profileType == "prompt" {
		result["prompt_profiles"] = s.FeedbackAnalyzer.AllPromptProfiles()
	}
	if profileType == "" || profileType == "provider" {
		result["provider_profiles"] = s.FeedbackAnalyzer.AllProviderProfiles()
	}

	return fleetJSON(result)
}

func (s *Server) handleProviderRecommend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.AutoOptimizer == nil {
		return codedError(ErrNotRunning, "auto-optimizer not initialized — run InitSelfImprovement first"), nil
	}

	task := getStringArg(req, "task")
	if task == "" {
		return codedError(ErrInvalidParams, "task description required"), nil
	}

	rec := s.AutoOptimizer.RecommendProvider(task)
	return fleetJSON(rec)
}

func (s *Server) handleFleetDLQ(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil {
		return codedError(ErrFleetNotRunning, "fleet server not active — start with 'ralphglasses mcp --fleet'"), nil
	}

	action := getStringArg(req, "action")
	if action == "" {
		action = "list"
	}

	switch action {
	case "list":
		items := s.FleetCoordinator.ListDLQ()
		return fleetJSON(map[string]any{
			"items": items,
			"count": len(items),
		})
	case "retry":
		itemID := getStringArg(req, "item_id")
		if itemID == "" {
			return codedError(ErrInvalidParams, "item_id required for retry action"), nil
		}
		if err := s.FleetCoordinator.RetryFromDLQ(itemID); err != nil {
			return codedError(ErrInternal, err.Error()), nil
		}
		return fleetJSON(map[string]string{"status": "retried", "item_id": itemID})
	case "purge":
		n := s.FleetCoordinator.PurgeDLQ()
		return fleetJSON(map[string]any{"status": "purged", "count": n})
	case "depth":
		return fleetJSON(map[string]any{"dlq_depth": s.FleetCoordinator.DLQDepth()})
	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown action %q — use list, retry, purge, or depth", action)), nil
	}
}

// fleetJSON marshals v and returns it as a text content result.
func fleetJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return codedError(ErrInternal, "marshal error: " + err.Error()), nil
	}
	return textResult(string(data)), nil
}
