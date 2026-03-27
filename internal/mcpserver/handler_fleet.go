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
		return jsonResult(map[string]any{
			"status":     "not_configured",
			"fleet_mode": false,
			"message":    "fleet coordinator not active — start with 'ralphglasses mcp --fleet'",
			"items":      []any{},
			"count":      0,
		}), nil
	}

	repo := getStringArg(req, "repo")
	prompt := getStringArg(req, "prompt")
	provider := getStringArg(req, "provider")
	budget := getNumberArg(req, "budget_usd", 5)
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
		return jsonResult(map[string]any{
			"status":     "not_configured",
			"fleet_mode": false,
			"message":    "fleet coordinator not active — start with 'ralphglasses mcp --fleet'",
			"items":      []any{},
			"count":      0,
		}), nil
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
		return jsonResult(map[string]any{
			"status":     "not_configured",
			"fleet_mode": false,
			"message":    "fleet coordinator not active — start with 'ralphglasses mcp --fleet'",
			"items":      []any{},
			"count":      0,
		}), nil
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
	if events == nil {
		events = []session.HITLEvent{}
	}
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

	action := getStringArg(req, "action")
	if action == "" {
		action = "get"
	}
	profileType := getStringArg(req, "type")

	// Auto-seed when explicitly requested or when profiles are empty on a get.
	// force=true when action=="seed" so existing profiles are replaced.
	if action == "seed" || (action == "get" && s.FeedbackAnalyzer.IsEmpty()) {
		s.autoSeedFeedbackProfiles(action == "seed")
	}

	result := map[string]any{}

	if profileType == "" || profileType == "prompt" {
		result["prompt_profiles"] = s.FeedbackAnalyzer.AllPromptProfiles()
	}
	if profileType == "" || profileType == "provider" {
		result["provider_profiles"] = s.FeedbackAnalyzer.AllProviderProfiles()
	}
	result["seeded"] = action == "seed" || s.feedbackWasAutoSeeded

	return fleetJSON(result)
}

// autoSeedFeedbackProfiles tries to seed profiles from observation JSONL files
// across all known repos. Falls back to journal entries when no observations
// are available (FINDING-103). When force is true, existing profiles are
// cleared before seeding so that an explicit action=seed always re-reads data.
func (s *Server) autoSeedFeedbackProfiles(force bool) {
	if s.FeedbackAnalyzer == nil {
		return
	}
	if !force && !s.FeedbackAnalyzer.IsEmpty() {
		return
	}

	if force {
		s.FeedbackAnalyzer.Reset()
	}

	s.mu.RLock()
	repos := s.Repos
	s.mu.RUnlock()

	var allObs []session.LoopObservation
	var allJournal []session.JournalEntry
	for _, r := range repos {
		obsPath := session.ObservationPath(r.Path)
		obs, err := session.LoadObservations(obsPath, time.Time{})
		if err == nil && len(obs) > 0 {
			allObs = append(allObs, obs...)
		}
		// Also collect journal entries as a fallback data source.
		entries, err := session.ReadRecentJournal(r.Path, 100)
		if err == nil && len(entries) > 0 {
			allJournal = append(allJournal, entries...)
		}
	}

	if len(allObs) > 0 {
		if err := s.FeedbackAnalyzer.SeedFromObservations(allObs); err == nil {
			s.feedbackWasAutoSeeded = true
		}
	}

	// FINDING-103: Fall back to journal entries when observations yield no profiles.
	if s.FeedbackAnalyzer.IsEmpty() && len(allJournal) > 0 {
		s.FeedbackAnalyzer.Ingest(allJournal)
		s.feedbackWasAutoSeeded = true
	}
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

	// FINDING-104: Fall back to model-based cost estimation when profile data
	// yields zero budget so callers always get a usable estimate.
	result := map[string]any{
		"provider":             rec.Provider,
		"model":                rec.Model,
		"estimated_budget_usd": rec.EstimatedBudget,
		"confidence":           rec.Confidence,
		"task_type":            rec.TaskType,
		"rationale":            rec.Rationale,
	}
	if rec.NormalizedCost > 0 {
		result["normalized_cost_usd"] = rec.NormalizedCost
	}

	if rec.EstimatedBudget == 0 {
		// Use model-based cost estimation as fallback.
		rates := session.DefaultCostRates()
		est := estimateSessionCost(
			string(rec.Provider), rec.Model,
			5000,  // default prompt tokens
			2000,  // default output tokens per turn
			5,     // default turns
			"session", 1,
			rates, nil,
		)
		result["estimated_budget_usd"] = est.Estimate.MidUSD
		result["source"] = "model_estimate"
	}

	return fleetJSON(result)
}

func (s *Server) handleFleetDLQ(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil {
		return jsonResult(map[string]any{
			"status":     "not_configured",
			"fleet_mode": false,
			"message":    "fleet coordinator not active — start with 'ralphglasses mcp --fleet'",
			"items":      []any{},
			"count":      0,
		}), nil
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
