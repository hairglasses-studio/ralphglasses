package mcpserver

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Team handlers

func (s *Server) handleTeamCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	teamName := getStringArg(req, "name")
	if teamName == "" {
		return codedError(ErrInvalidParams, "team name required"), nil
	}
	tasksStr := getStringArg(req, "tasks")
	if tasksStr == "" {
		return codedError(ErrInvalidParams, "tasks required (newline-separated)"), nil
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

	teamProvider := session.Provider(getStringArg(req, "provider"))
	if teamProvider == "" {
		teamProvider = session.ProviderClaude
	}

	workerProvider := session.Provider(getStringArg(req, "worker_provider"))

	config := session.TeamConfig{
		Name:           teamName,
		Provider:       teamProvider,
		WorkerProvider: workerProvider,
		RepoPath:       r.Path,
		LeadAgent:      getStringArg(req, "lead_agent"),
		Tasks:          tasks,
		Model:          getStringArg(req, "model"),
		MaxBudgetUSD:   getNumberArg(req, "max_budget_usd", 0),
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

	team, ok := s.SessMgr.GetTeam(name)
	if !ok {
		return codedError(ErrTeamNotFound, fmt.Sprintf("team not found: %s", name)), nil
	}

	// Enrich with lead session info
	result := map[string]any{
		"name":    team.Name,
		"repo":    team.RepoPath,
		"status":  team.Status,
		"tasks":   team.Tasks,
		"created": team.CreatedAt,
	}

	if lead, ok := s.SessMgr.Get(team.LeadID); ok {
		lead.Lock()
		result["lead_session"] = map[string]any{
			"id":        lead.ID,
			"status":    lead.Status,
			"spent_usd": lead.SpentUSD,
			"turns":     lead.TurnCount,
			"output":    lead.LastOutput,
		}
		lead.Unlock()
	}

	return jsonResult(result), nil
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
	count, err := s.SessMgr.DelegateTask(name, session.TeamTask{
		Description: task,
		Provider:    taskProvider,
		Status:      "pending",
	})
	if err != nil {
		return codedError(ErrTeamNotFound, err.Error()), nil
	}

	return textResult(fmt.Sprintf("Added task to team %s (%d total tasks)", name, count)), nil
}

// Agent handlers

func (s *Server) handleAgentDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
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
		provider = session.ProviderClaude
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
		location = fmt.Sprintf("%s/AGENTS.md (## %s)", r.Path, agentName)
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
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
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
		for _, p := range []session.Provider{session.ProviderClaude, session.ProviderGemini, session.ProviderCodex} {
			found, err := session.DiscoverAgents(r.Path, p)
			if err != nil {
				continue
			}
			agents = append(agents, found...)
		}
	} else {
		provider := session.Provider(providerStr)
		if provider == "" {
			provider = session.ProviderClaude
		}
		var err error
		agents, err = session.DiscoverAgents(r.Path, provider)
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("list agents: %v", err)), nil
		}
	}

	if agents == nil {
		agents = []session.AgentDef{}
	}
	return jsonResult(agents), nil
}

func (s *Server) handleAgentCompose(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
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
		provider = session.ProviderClaude
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
