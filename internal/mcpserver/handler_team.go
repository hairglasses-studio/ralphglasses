package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Team handlers

func (s *Server) handleTeamCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)

	repoName, errResult := pp.StringErr("repo")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	teamName, errResult := pp.StringErr("name")
	if errResult != nil {
		return errResult, nil
	}
	tasksStr, errResult := pp.StringErr("tasks")
	if errResult != nil {
		return errResult, nil
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

	var tasks []string
	for _, line := range strings.Split(tasksStr, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			tasks = append(tasks, line)
		}
	}

	teamProvider := session.Provider(pp.OptionalString("provider", ""))
	if teamProvider == "" {
		teamProvider = session.DefaultPrimaryProvider()
	}

	workerProvider := session.Provider(pp.String("worker_provider"))
	leadAgent := pp.String("lead_agent")
	if teamProvider == session.ProviderCodex && strings.TrimSpace(leadAgent) != "" {
		return codedError(ErrInvalidParams, "lead_agent is not supported for codex teams"), nil
	}
	if err := session.ValidateLaunchAgent(teamProvider, leadAgent); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("lead_agent: %v", err)), nil
	}

	config := session.TeamConfig{
		Name:             teamName,
		TenantID:         session.NormalizeTenantID(pp.String("tenant_id")),
		Provider:         teamProvider,
		WorkerProvider:   workerProvider,
		RepoPath:         r.Path,
		LeadAgent:        leadAgent,
		Tasks:            tasks,
		Model:            pp.String("model"),
		WorkerModel:      pp.String("worker_model"),
		MaxBudgetUSD:     pp.FloatOr("budget_usd", 0),
		MaxConcurrency:   int(pp.FloatOr("max_concurrency", 0)),
		MaxRetries:       int(pp.FloatOr("max_retries", 0)),
		ExecutionBackend: strings.TrimSpace(pp.String("execution_backend")),
		WorktreePolicy:   strings.TrimSpace(pp.String("worktree_policy")),
		TargetBranch:     strings.TrimSpace(pp.String("target_branch")),
		AutoStart:        pp.OptionalBool("autostart", teamProvider == session.ProviderCodex),
		A2AAgentURL:      strings.TrimSpace(pp.String("a2a_agent_url")),
	}
	dryRun := pp.Bool("dry_run")
	backendConfigured := s.SessMgr.StructuredTeamBackend() != nil || s.FleetCoordinator != nil || s.FleetClient != nil
	if config.ExecutionBackend == "" && teamProvider == session.ProviderCodex {
		if backendConfigured {
			config.ExecutionBackend = session.TeamExecutionBackendFleet
		} else {
			config.ExecutionBackend = session.TeamExecutionBackendLocal
		}
	}
	if config.ExecutionBackend == session.TeamExecutionBackendA2A {
		if config.A2AAgentURL == "" {
			return codedError(ErrInvalidParams, "a2a_agent_url required when execution_backend=a2a"), nil
		}
		if !dryRun && !backendConfigured {
			return fleetNotConfiguredResult(), nil
		}
	}
	if config.ExecutionBackend == session.TeamExecutionBackendFleet && !backendConfigured {
		return fleetNotConfiguredResult(), nil
	}
	if config.WorktreePolicy == "" && teamProvider == session.ProviderCodex {
		config.WorktreePolicy = session.TeamWorktreePolicyPerWorker
	}

	if dryRun {
		// Apply the same default resolution as the real launch path so the
		// preview shows effective values instead of zero/empty defaults.
		effectiveProvider := config.Provider
		if effectiveProvider == "" {
			effectiveProvider = session.DefaultPrimaryProvider()
		}
		effectiveWorkerProvider := config.WorkerProvider
		if effectiveWorkerProvider == "" {
			effectiveWorkerProvider = effectiveProvider
		}
		effectiveModel := config.Model
		if effectiveModel == "" {
			switch effectiveProvider {
			case session.ProviderGemini:
				effectiveModel = "gemini-3.1-flash"
			case session.ProviderCodex:
				effectiveModel = session.ProviderDefaults(session.ProviderCodex)
			default:
				effectiveModel = "claude-sonnet-4-6"
			}
		}
		effectiveBudget := config.MaxBudgetUSD
		if effectiveBudget <= 0 {
			effectiveBudget = 5.0
		}
		effectiveWorkerModel := config.WorkerModel
		if effectiveWorkerModel == "" {
			effectiveWorkerModel = session.ProviderDefaults(effectiveWorkerProvider)
		}
		effectiveWorktreePolicy := config.WorktreePolicy
		if effectiveWorktreePolicy == "" {
			if effectiveWorkerProvider == session.ProviderCodex || effectiveWorkerProvider == session.ProviderGemini {
				effectiveWorktreePolicy = session.TeamWorktreePolicyPerWorker
			} else {
				effectiveWorktreePolicy = session.TeamWorktreePolicyShared
			}
		}
		return jsonResult(map[string]any{
			"dry_run":           true,
			"runtime":           teamRuntimeForProvider(effectiveProvider),
			"name":              config.Name,
			"tenant_id":         config.TenantID,
			"repo":              repoName,
			"provider":          string(effectiveProvider),
			"worker_provider":   string(effectiveWorkerProvider),
			"lead_agent":        config.LeadAgent,
			"model":             effectiveModel,
			"worker_model":      effectiveWorkerModel,
			"budget_usd":        effectiveBudget,
			"max_concurrency":   maxInt(config.MaxConcurrency, 2),
			"max_retries":       maxInt(config.MaxRetries, 2),
			"execution_backend": firstNonBlank(config.ExecutionBackend, session.TeamExecutionBackendLocal),
			"worktree_policy":   effectiveWorktreePolicy,
			"target_branch":     firstNonBlank(config.TargetBranch, "main"),
			"autostart":         config.AutoStart,
			"a2a_agent_url":     config.A2AAgentURL,
			"tasks":             config.Tasks,
			"task_count":        len(config.Tasks),
		}), nil
	}

	team, err := s.SessMgr.LaunchTeam(ctx, config)
	if err != nil {
		return codedError(ErrLaunchFailed, fmt.Sprintf("create team failed: %v", err)), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))

	team, ok := s.SessMgr.GetTeamForTenant(name, tenantID)
	if !ok {
		return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
	}

	// Enrich with lead session info
	result := map[string]any{
		"name":                  team.Name,
		"tenant_id":             team.TenantID,
		"repo":                  team.RepoPath,
		"provider":              team.Provider,
		"worker_provider":       team.WorkerProvider,
		"status":                team.Status,
		"run_state":             team.RunState,
		"runtime":               team.Runtime,
		"tasks":                 team.Tasks,
		"created":               team.CreatedAt,
		"updated":               team.UpdatedAt,
		"planner_session_id":    team.PlannerSessionID,
		"last_planner_summary":  team.LastPlannerSummary,
		"pending_question":      team.PendingQuestion,
		"step_count":            team.StepCount,
		"execution_backend":     team.ExecutionBackend,
		"worktree_policy":       team.WorktreePolicy,
		"autostart":             team.AutoStart,
		"controller_running":    team.ControllerRunning,
		"last_controller_error": team.LastControllerError,
		"target_branch":         team.TargetBranch,
		"integration_branch":    team.IntegrationBranch,
		"integration_path":      team.IntegrationPath,
		"promotion_status":      team.PromotionStatus,
		"a2a_agent_url":         team.A2AAgentURL,
	}

	activeWorkers := 0
	completedTasks := 0
	totalSpent := 0.0
	totalTurns := 0
	for _, task := range team.Tasks {
		if task.Status == session.TeamTaskInProgress {
			activeWorkers++
		}
		if task.Status == session.TeamTaskCompleted {
			completedTasks++
		}
		// Aggregate worker stats
		if task.WorkerSessionID != "" {
			if ws, ok := s.SessMgr.GetForTenant(task.WorkerSessionID, tenantID); ok {
				ws.Lock()
				totalSpent += ws.SpentUSD
				totalTurns += ws.TurnCount
				ws.Unlock()
			}
		}
	}
	result["active_workers"] = activeWorkers
	result["completed_tasks"] = completedTasks
	result["task_count"] = len(team.Tasks)

	// Aggregate planner stats
	if team.PlannerSessionID != "" {
		if ps, ok := s.SessMgr.GetForTenant(team.PlannerSessionID, tenantID); ok {
			ps.Lock()
			totalSpent += ps.SpentUSD
			totalTurns += ps.TurnCount
			ps.Unlock()
		}
	}

	if lead, ok := s.SessMgr.GetForTenant(team.LeadID, tenantID); ok {
		lead.Lock()
		totalSpent += lead.SpentUSD
		totalTurns += lead.TurnCount
		result["lead_session"] = map[string]any{
			"id":        lead.ID,
			"status":    lead.Status,
			"spent_usd": lead.SpentUSD,
			"turns":     lead.TurnCount,
			"output":    lead.LastOutput,
		}
		lead.Unlock()
	}

	result["total_spent_usd"] = totalSpent
	result["total_turn_count"] = totalTurns

	return jsonResult(result), nil
}

func (s *Server) handleTeamStep(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))

	result, err := s.SessMgr.StepTeamForTenant(ctx, tenantID, name)
	if err != nil {
		if err == session.ErrTeamNotFound {
			return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("team step failed: %v", err)), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handleTeamAnswer(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	answer := getStringArg(req, "answer")
	if answer == "" {
		return codedError(ErrInvalidParams, "answer required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))

	team, err := s.SessMgr.AnswerTeamForTenant(tenantID, name, answer, getStringArg(req, "task_id"))
	if err != nil {
		if err == session.ErrTeamNotFound {
			return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
		}
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	team, err := s.SessMgr.StartTeamForTenant(ctx, tenantID, name)
	if err != nil {
		if err == session.ErrTeamNotFound {
			return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("team start failed: %v", err)), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	team, err := s.SessMgr.StopTeamForTenant(tenantID, name)
	if err != nil {
		if err == session.ErrTeamNotFound {
			return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("team stop failed: %v", err)), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamAwait(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	timeoutSeconds := getNumberArg(req, "timeout_seconds", 0)
	pollSeconds := getNumberArg(req, "poll_seconds", 2)
	awaitCtx := ctx
	var cancel context.CancelFunc
	if timeoutSeconds > 0 {
		awaitCtx, cancel = context.WithTimeout(ctx, time.Duration(timeoutSeconds*float64(time.Second)))
		defer cancel()
	}
	team, err := s.SessMgr.AwaitTeamForTenant(awaitCtx, tenantID, name, time.Duration(pollSeconds*float64(time.Second)))
	if err != nil {
		if err == session.ErrTeamNotFound {
			return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("team await failed: %v", err)), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamDelegate(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	task := getStringArg(req, "task")
	if task == "" {
		return codedError(ErrInvalidParams, "task description required"), nil
	}

	taskProvider := session.Provider(getStringArg(req, "provider"))
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	count, err := s.SessMgr.DelegateTaskForTenant(tenantID, name, session.TeamTask{
		Description: task,
		Provider:    taskProvider,
		Status:      session.TeamTaskPending,
	})
	if err != nil {
		return codedError(ErrTeamNotFound, err.Error()), nil
	}

	return textResult(fmt.Sprintf("Added task to team %s (%d total tasks)", name, count)), nil
}

func teamRuntimeForProvider(provider session.Provider) string {
	if provider == session.ProviderCodex {
		return session.TeamRuntimeStructuredCodex
	}
	return session.TeamRuntimeLegacyLead
}

func maxInt(values ...int) int {
	maxValue := 0
	for _, value := range values {
		if value > maxValue {
			maxValue = value
		}
	}
	return maxValue
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

// Agent handlers

func (s *Server) handleAgentDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	agentName := getStringArg(req, "name")
	if agentName == "" {
		return codedError(ErrInvalidParams, "agent name required"), nil
	}
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
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

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.DefaultPrimaryProvider()
	}

	def := session.AgentDef{
		Name:        agentName,
		Provider:    provider,
		Description: getStringArg(req, "description"),
		Model:       getStringArg(req, "model"),
		Prompt:      prompt,
		MaxTurns:    int(getNumberArg(req, "max_turns", 0)),
	}
	if tools := getStringArg(req, "tools"); tools != "" {
		def.Tools = strings.Split(tools, ",")
	}

	if err := session.WriteAgent(r.Path, def); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write agent: %v", err)), nil
	}

	var location string
	switch provider {
	case session.ProviderGemini:
		location = fmt.Sprintf("%s/.gemini/agents/%s.md", r.Path, agentName)
	case session.ProviderCodex:
		location = fmt.Sprintf("%s/.codex/agents/%s.toml", r.Path, agentName)
	default:
		location = fmt.Sprintf("%s/.claude/agents/%s.md", r.Path, agentName)
	}
	return textResult(fmt.Sprintf("Created agent definition: %s", location)), nil
}

func (s *Server) handleAgentList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
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

	providerStr := getStringArg(req, "provider")

	var agents []session.AgentDef
	if providerStr == "all" {
		// Discover agents for all providers
		for _, p := range []session.Provider{session.ProviderCodex, session.ProviderGemini, session.ProviderClaude} {
			found, err := session.DiscoverAgents(r.Path, p)
			if err != nil {
				continue
			}
			agents = append(agents, found...)
		}
	} else {
		provider := session.Provider(providerStr)
		if provider == "" {
			provider = session.DefaultPrimaryProvider()
		}
		var err error
		agents, err = session.DiscoverAgents(r.Path, provider)
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("list agents: %v", err)), nil
		}
	}

	if len(agents) == 0 {
		return emptyResult("agents"), nil
	}
	return jsonResult(agents), nil
}

func (s *Server) handleAgentCompose(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	name := getStringArg(req, "name")
	if name == "" {
		return codedError(ErrInvalidParams, "composite agent name required"), nil
	}
	agentsStr := getStringArg(req, "agents")
	if agentsStr == "" {
		return codedError(ErrInvalidParams, "agents list required (comma-separated)"), nil
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

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.DefaultPrimaryProvider()
	}

	var agentNames []string
	for _, n := range strings.Split(agentsStr, ",") {
		n = strings.TrimSpace(n)
		if n != "" {
			agentNames = append(agentNames, n)
		}
	}

	composite, err := session.ComposeAgents(r.Path, agentNames, provider, name)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("compose agents: %v", err)), nil
	}

	// Apply model override
	if m := getStringArg(req, "model"); m != "" {
		composite.Model = m
	}

	if err := session.WriteAgent(r.Path, composite); err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("write composite agent: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":     composite.Name,
		"provider": string(composite.Provider),
		"composed": agentNames,
		"tools":    composite.Tools,
		"model":    composite.Model,
	}), nil
}
