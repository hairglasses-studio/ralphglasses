package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func fleetNotConfiguredPayload() map[string]any {
	message := "fleet coordinator not configured"
	prereqs := []string{
		"Start a coordinator with: ralphglasses serve --coordinator --port 9473",
		"Point MCP clients at it with RALPH_FLEET_URL=http://<host>:9473",
	}
	if url := strings.TrimSpace(os.Getenv("RALPH_FLEET_URL")); url != "" {
		message = fmt.Sprintf("fleet coordinator at %s is not connected", url)
		prereqs = append(prereqs, "Verify the coordinator is reachable and healthy")
	}
	return map[string]any{
		"status":        "not_configured",
		"fleet_mode":    false,
		"message":       message,
		"items":         []any{},
		"count":         0,
		"prerequisites": prereqs,
	}
}

func fleetNotConfiguredResult() *mcp.CallToolResult {
	return jsonResult(fleetNotConfiguredPayload())
}

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
		return fleetNotConfiguredResult(), nil
	}

	p := NewParams(req)

	repo, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	prompt, errResult := p.RequireString("prompt")
	if errResult != nil {
		return errResult, nil
	}
	provider := p.OptionalString("provider", "")
	budget := p.OptionalNumber("budget_usd", 5)
	priority := int(p.OptionalNumber("priority", 5))

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
		return fleetNotConfiguredResult(), nil
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
		return fleetNotConfiguredResult(), nil
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

		// Also tell the Manager to start/stop the supervisor at the new level.
		if s.SessMgr != nil {
			repoPath := getStringArg(req, "repo_path")
			s.SessMgr.SetAutonomyLevel(level, repoPath)
		}
	}

	current := s.DecisionLog.Level()
	return fleetJSON(map[string]any{
		"level":      int(current),
		"level_name": current.String(),
		"stats":      s.DecisionLog.Stats(),
	})
}

func (s *Server) handleSupervisorStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.SessMgr == nil {
		payload := map[string]any{
			"running":      false,
			"error":        "no manager",
			"productivity": session.EmptyProductivitySnapshot(),
		}
		if s.DecisionLog != nil {
			payload["decision_log"] = s.DecisionLog.Snapshot(5)
		}
		return fleetJSON(payload)
	}
	status := s.SessMgr.SupervisorStatus()
	if status == nil {
		payload := map[string]any{
			"running":      false,
			"message":      "supervisor not active",
			"productivity": s.SessMgr.ProductivitySnapshot("", time.Time{}),
		}
		if s.DecisionLog != nil {
			payload["decision_log"] = s.DecisionLog.Snapshot(5)
		}
		return fleetJSON(payload)
	}
	payload := map[string]any{
		"running":                status.Running,
		"repo_path":              status.RepoPath,
		"tick_count":             status.TickCount,
		"last_cycle_launch":      status.LastCycleLaunch,
		"started_at":             status.StartedAt,
		"automation":             status.Automation,
		"research_daemon_active": status.ResearchDaemonActive,
		"research_daemon":        status.ResearchDaemonStats,
		"crash_recovery_active":  status.CrashRecoveryActive,
		"crash_recovery_policy":  status.CrashRecoveryPolicy,
		"productivity":           status.Productivity,
	}
	if s.DecisionLog != nil {
		payload["decision_log"] = s.DecisionLog.Snapshot(5)
	}
	return fleetJSON(payload)
}

func (s *Server) handleAutonomyDecisions(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.DecisionLog == nil {
		return codedError(ErrNotRunning, "autonomy system not initialized"), nil
	}

	limit := int(getNumberArg(req, "limit", 20))

	decisions := s.DecisionLog.Recent(limit)
	return fleetJSON(map[string]any{
		"decisions":          decisions,
		"decision_summaries": s.DecisionLog.RecentSummaries(limit),
		"count":              len(decisions),
		"stats":              s.DecisionLog.Stats(),
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

	// FINDING-220/262: Cold-start bootstrap — when FeedbackAnalyzer lacks
	// sufficient non-Claude observations, bypass it and use cost-based tier
	// selection from CascadeRouter. This breaks the circular dependency where
	// provider_recommend always returns Claude because no Gemini data exists.
	coldStart := s.FeedbackAnalyzer == nil || !s.FeedbackAnalyzer.HasMultiProviderData(5)

	var rec session.ProviderRecommendation

	if coldStart && s.SessMgr != nil && s.SessMgr.HasCascadeRouter() {
		cr := s.SessMgr.GetCascadeRouter()
		taskType := session.ClassifyTask(task)
		tier := cr.SelectTier(taskType, 0)

		rec = session.ProviderRecommendation{
			Provider:   tier.Provider,
			Model:      tier.Model,
			TaskType:   taskType,
			Confidence: "low",
			Rationale: fmt.Sprintf("cold-start heuristic: %s tier (%s) for %s tasks (complexity %d) — no multi-provider feedback data yet",
				tier.Label, tier.Model, taskType, tier.MaxComplexity),
			FallbackChain:         s.AutoOptimizer.BuildSmartFailoverChain(task).Providers,
			CapabilityConstraints: session.ProviderCapabilityConstraints(tier.Provider),
			DataSource:            "heuristic",
		}
	} else {
		rec = s.AutoOptimizer.RecommendProvider(task)
	}

	// FINDING-104: Fall back to model-based cost estimation when profile data
	// yields zero budget so callers always get a usable estimate.
	result := map[string]any{
		"provider":             rec.Provider,
		"model":                rec.Model,
		"estimated_budget_usd": rec.EstimatedBudget,
		"confidence":           rec.Confidence,
		"task_type":            rec.TaskType,
		"rationale":            rec.Rationale,
		"fallback_chain":       rec.FallbackChain,
		"data_source":          rec.DataSource,
	}
	if rec.NormalizedCost > 0 {
		result["normalized_cost_usd"] = rec.NormalizedCost
	}
	if len(rec.CapabilityConstraints) > 0 {
		result["capability_constraints"] = rec.CapabilityConstraints
	}
	if result["data_source"] == "" {
		if coldStart {
			result["data_source"] = "heuristic"
		} else {
			result["data_source"] = "feedback_data"
		}
	}

	if rec.EstimatedBudget == 0 {
		// Use model-based cost estimation as fallback.
		rates := session.DefaultCostRates()
		est := estimateSessionCost(
			string(rec.Provider), rec.Model,
			5000, // default prompt tokens
			2000, // default output tokens per turn
			5,    // default turns
			"session", 1,
			rates, nil,
		)
		result["estimated_budget_usd"] = est.Estimate.MidUSD
		result["source"] = "model_estimate"
	}

	return fleetJSON(result)
}

func (s *Server) handleProviderCapabilities(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	provider := session.Provider(strings.TrimSpace(getStringArg(req, "provider")))
	if provider == "" {
		return fleetJSON(map[string]any{
			"providers": session.ProviderCapabilityMatrices(),
		})
	}

	matrix, ok := session.ProviderCapabilityMatrixFor(provider)
	if !ok {
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown provider %q (valid: claude, codex, gemini)", provider)), nil
	}
	return fleetJSON(matrix)
}

func (s *Server) handleProviderCompare(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	task := strings.TrimSpace(getStringArg(req, "task"))
	taskType := ""
	if task != "" {
		taskType = session.ClassifyTask(task)
	}

	providers := session.PrimaryProviders()
	comparison := make([]map[string]any, 0, len(providers))
	for _, provider := range providers {
		matrix, ok := session.ProviderCapabilityMatrixFor(provider)
		if !ok {
			continue
		}

		entry := map[string]any{
			"provider":             matrix.Provider,
			"binary":               matrix.Binary,
			"default_model":        matrix.DefaultModel,
			"project_instructions": matrix.ProjectInstructions,
			"repo_config_path":     matrix.RepoConfigPath,
			"agent_config_path":    matrix.AgentConfigPath,
			"capabilities":         matrix.Capabilities,
			"constraints":          session.ProviderCapabilityConstraints(provider),
		}
		if taskType != "" {
			if profile := providerProfileSnapshot(s.FeedbackAnalyzer, provider, taskType); profile != nil {
				entry["feedback_profile"] = profile
			}
		}
		comparison = append(comparison, entry)
	}

	result := map[string]any{
		"providers": comparison,
	}
	if task != "" {
		result["task"] = task
		result["task_type"] = taskType
		if s.AutoOptimizer != nil {
			result["recommendation"] = s.AutoOptimizer.RecommendProvider(task)
		}
	}

	return fleetJSON(result)
}

func providerProfileSnapshot(feedback *session.FeedbackAnalyzer, provider session.Provider, taskType string) map[string]any {
	if feedback == nil || taskType == "" {
		return nil
	}
	if profile, ok := feedback.GetProviderProfile(string(provider), taskType); ok {
		return map[string]any{
			"provider":        profile.Provider,
			"task_type":       profile.TaskType,
			"sample_count":    profile.SampleCount,
			"avg_cost_usd":    profile.AvgCostUSD,
			"avg_turns":       profile.AvgTurns,
			"completion_rate": profile.CompletionRate,
			"cost_per_turn":   profile.CostPerTurn,
			"trusted":         true,
		}
	}
	for _, profile := range feedback.AllProviderProfiles() {
		if profile.Provider != string(provider) || profile.TaskType != taskType {
			continue
		}
		return map[string]any{
			"provider":        profile.Provider,
			"task_type":       profile.TaskType,
			"sample_count":    profile.SampleCount,
			"avg_cost_usd":    profile.AvgCostUSD,
			"avg_turns":       profile.AvgTurns,
			"completion_rate": profile.CompletionRate,
			"cost_per_turn":   profile.CostPerTurn,
			"trusted":         false,
		}
	}
	return nil
}

func (s *Server) handleFleetDLQ(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.FleetCoordinator == nil {
		payload := fleetNotConfiguredPayload()
		payload["prerequisites"] = append(payload["prerequisites"].([]string), "Fleet DLQ requires a local coordinator (remote client not supported)")
		return jsonResult(payload), nil
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

func (s *Server) handleFleetSchedule(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tasksJSON := getStringArg(req, "tasks")
	if tasksJSON == "" {
		return codedError(ErrInvalidParams, "tasks parameter is required (JSON array of task objects)"), nil
	}

	var tasks []fleet.TaskNode
	if err := json.Unmarshal([]byte(tasksJSON), &tasks); err != nil {
		return codedError(ErrInvalidParams, "invalid tasks JSON: "+err.Error()), nil
	}

	if len(tasks) == 0 {
		return fleetJSON(map[string]any{
			"schedule": &fleet.SchedulePlan{},
			"message":  "no tasks provided",
		})
	}

	graph := fleet.NewTaskGraph()
	for _, t := range tasks {
		graph.AddNode(t)
	}

	// Detect cycles before building schedule.
	if cycles := graph.DetectCycles(); len(cycles) > 0 {
		return fleetJSON(map[string]any{
			"error":  "dependency cycle detected",
			"cycles": cycles,
		})
	}

	plan, err := fleet.BuildSchedule(graph)
	if err != nil {
		return codedError(ErrInternal, err.Error()), nil
	}

	critPath := graph.CriticalPath()
	var critIDs []string
	for _, n := range critPath {
		critIDs = append(critIDs, n.ID)
	}

	return fleetJSON(map[string]any{
		"schedule":      plan,
		"critical_path": critIDs,
		"total_tasks":   plan.TotalTasks,
		"depth":         plan.Depth,
	})
}

// fleetJSON marshals v and returns it as a text content result.
func fleetJSON(v any) (*mcp.CallToolResult, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return codedError(ErrInternal, "marshal error: "+err.Error()), nil
	}
	return textResult(string(data)), nil
}
