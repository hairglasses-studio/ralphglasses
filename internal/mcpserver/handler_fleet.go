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

// Fleet and HITL infrastructure on the Server.
// These fields are set when running in fleet mode.
var (
	fleetCoordinator *fleet.Coordinator
	fleetClient      *fleet.Client
	hitlTracker      *session.HITLTracker
	decisionLog      *session.DecisionLog
	feedbackAnalyzer *session.FeedbackAnalyzer
	autoOptimizer    *session.AutoOptimizer
)

// InitFleetTools initializes fleet infrastructure for MCP tools.
func InitFleetTools(coord *fleet.Coordinator, client *fleet.Client, hitl *session.HITLTracker, decisions *session.DecisionLog, feedback *session.FeedbackAnalyzer) {
	fleetCoordinator = coord
	fleetClient = client
	hitlTracker = hitl
	decisionLog = decisions
	feedbackAnalyzer = feedback
}

// InitSelfImprovement initializes HITL and feedback infrastructure (works without fleet).
// When called with a Server, also wires the AutoOptimizer into the session manager.
func InitSelfImprovement(stateDir string, autonomyLevel int) {
	if hitlTracker == nil {
		hitlTracker = session.NewHITLTracker(stateDir)
	}
	if decisionLog == nil {
		decisionLog = session.NewDecisionLog(stateDir, session.AutonomyLevel(autonomyLevel))
	}
	if feedbackAnalyzer == nil {
		feedbackAnalyzer = session.NewFeedbackAnalyzer(stateDir, 5)
	}
	if autoOptimizer == nil {
		autoOptimizer = session.NewAutoOptimizer(feedbackAnalyzer, decisionLog, hitlTracker, nil)
	}
}

// WireAutoOptimizer attaches the auto-optimizer to a session manager and creates
// the AutoRecovery handler. Call after InitSelfImprovement.
func WireAutoOptimizer(mgr *session.Manager) {
	if autoOptimizer == nil || mgr == nil {
		return
	}
	recovery := session.NewAutoRecovery(mgr, decisionLog, hitlTracker, session.DefaultAutoRecoveryConfig())
	autoOptimizer = session.NewAutoOptimizer(feedbackAnalyzer, decisionLog, hitlTracker, recovery)
	mgr.SetAutoOptimizer(autoOptimizer)
}

func (s *Server) handleFleetSubmit(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if fleetCoordinator == nil && fleetClient == nil {
		return errResult("fleet not active — start with 'ralphglasses serve'"), nil
	}

	repo := getStringArg(req, "repo")
	prompt := getStringArg(req, "prompt")
	provider := getStringArg(req, "provider")
	budget := getNumberArg(req, "budget", 5)
	priority := int(getNumberArg(req, "priority", 5))

	if repo == "" || prompt == "" {
		return errResult("repo and prompt required"), nil
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

	if fleetCoordinator != nil {
		if err := fleetCoordinator.SubmitWork(item); err != nil {
			return errResult(err.Error()), nil
		}
		return fleetJSON(map[string]any{
			"work_item_id": item.ID,
			"status":       "pending",
			"queue":        "local_coordinator",
		})
	}

	id, err := fleetClient.SubmitWork(context.Background(), *item)
	if err != nil {
		return errResult(err.Error()), nil
	}
	return fleetJSON(map[string]any{
		"work_item_id": id,
		"status":       "pending",
		"queue":        "remote_coordinator",
	})
}

func (s *Server) handleFleetBudget(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if fleetCoordinator == nil && fleetClient == nil {
		return errResult("fleet not active"), nil
	}

	newLimit := getNumberArg(req, "limit", 0)

	if fleetCoordinator != nil {
		if newLimit > 0 {
			fleetCoordinator.SetBudgetLimit(newLimit)
		}
		state := fleetCoordinator.GetFleetState()
		return fleetJSON(map[string]any{
			"budget_usd":  state.BudgetUSD,
			"spent_usd":   state.TotalSpentUSD,
			"remaining":   state.BudgetUSD - state.TotalSpentUSD,
			"active_work": state.ActiveWork,
			"queue_depth": state.QueueDepth,
		})
	}

	state, err := fleetClient.FleetState(context.Background())
	if err != nil {
		return errResult(err.Error()), nil
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
	if fleetCoordinator == nil && fleetClient == nil {
		return errResult("fleet not active"), nil
	}

	if fleetCoordinator != nil {
		state := fleetCoordinator.GetFleetState()
		return fleetJSON(map[string]any{
			"workers":     state.Workers,
			"total":       len(state.Workers),
			"active_work": state.ActiveWork,
		})
	}

	state, err := fleetClient.FleetState(context.Background())
	if err != nil {
		return errResult(err.Error()), nil
	}
	return fleetJSON(map[string]any{
		"workers":     state.Workers,
		"total":       len(state.Workers),
		"active_work": state.ActiveWork,
	})
}

func (s *Server) handleHITLScore(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if hitlTracker == nil {
		return errResult("HITL tracking not initialized — run InitSelfImprovement first"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	window := time.Duration(hours * float64(time.Hour))

	snap := hitlTracker.CurrentScore(window)
	return fleetJSON(snap)
}

func (s *Server) handleHITLHistory(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if hitlTracker == nil {
		return errResult("HITL tracking not initialized"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	limit := int(getNumberArg(req, "limit", 50))
	window := time.Duration(hours * float64(time.Hour))

	events := hitlTracker.History(window, limit)
	return fleetJSON(map[string]any{
		"events": events,
		"count":  len(events),
	})
}

func (s *Server) handleAutonomyLevel(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if decisionLog == nil {
		return errResult("autonomy system not initialized"), nil
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
			return errResult(fmt.Sprintf("invalid level: %s (use 0-3 or name)", levelStr)), nil
		}
		decisionLog.SetLevel(level)
	}

	current := decisionLog.Level()
	return fleetJSON(map[string]any{
		"level":      int(current),
		"level_name": current.String(),
		"stats":      decisionLog.Stats(),
	})
}

func (s *Server) handleAutonomyDecisions(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if decisionLog == nil {
		return errResult("autonomy system not initialized"), nil
	}

	limit := int(getNumberArg(req, "limit", 20))

	decisions := decisionLog.Recent(limit)
	return fleetJSON(map[string]any{
		"decisions": decisions,
		"count":     len(decisions),
		"stats":     decisionLog.Stats(),
	})
}

func (s *Server) handleAutonomyOverride(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if decisionLog == nil {
		return errResult("autonomy system not initialized"), nil
	}

	decisionID := getStringArg(req, "decision_id")
	if decisionID == "" {
		return errResult("decision_id required"), nil
	}

	details := getStringArg(req, "details")
	if details == "" {
		details = "manually overridden by user"
	}

	decisionLog.RecordOutcome(decisionID, session.DecisionOutcome{
		EvaluatedAt: time.Now(),
		Overridden:  true,
		Details:     details,
	})

	if hitlTracker != nil {
		hitlTracker.RecordManual(session.MetricAutoDecisionOverride, "", "", "overrode decision "+decisionID)
	}

	return fleetJSON(map[string]string{"status": "overridden", "decision_id": decisionID})
}

func (s *Server) handleFeedbackProfiles(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if feedbackAnalyzer == nil {
		return errResult("feedback analyzer not initialized"), nil
	}

	profileType := getStringArg(req, "type")

	result := map[string]any{}

	if profileType == "" || profileType == "prompt" {
		result["prompt_profiles"] = feedbackAnalyzer.AllPromptProfiles()
	}
	if profileType == "" || profileType == "provider" {
		result["provider_profiles"] = feedbackAnalyzer.AllProviderProfiles()
	}

	return fleetJSON(result)
}

func (s *Server) handleProviderRecommend(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if autoOptimizer == nil {
		return errResult("auto-optimizer not initialized — run InitSelfImprovement first"), nil
	}

	task := getStringArg(req, "task")
	if task == "" {
		return errResult("task description required"), nil
	}

	rec := autoOptimizer.RecommendProvider(task)
	return fleetJSON(rec)
}

// fleetJSON marshals v and returns it as a text content result.
func fleetJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult("marshal error: " + err.Error()), nil
	}
	return textResult(string(data)), nil
}
