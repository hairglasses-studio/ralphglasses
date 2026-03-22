package mcpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Server holds state for the MCP server.
type Server struct {
	mu         sync.RWMutex
	ScanPath   string
	Repos      []*model.Repo
	ProcMgr    *process.Manager
	SessMgr    *session.Manager
	EventBus   *events.Bus
	HTTPClient *http.Client
	Engine     *enhancer.HybridEngine
	engineOnce sync.Once
}

// NewServer creates a new MCP server instance.
func NewServer(scanPath string) *Server {
	return &Server{
		ScanPath:   scanPath,
		ProcMgr:    process.NewManager(),
		SessMgr:    session.NewManager(),
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewServerWithBus creates a new MCP server instance with an event bus.
func NewServerWithBus(scanPath string, bus *events.Bus) *Server {
	return &Server{
		ScanPath:   scanPath,
		ProcMgr:    process.NewManagerWithBus(bus),
		SessMgr:    session.NewManagerWithBus(bus),
		EventBus:   bus,
		HTTPClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Register adds all ralphglasses tools to the MCP server.
func (s *Server) Register(srv *server.MCPServer) {
	srv.AddTool(mcp.NewTool("ralphglasses_scan",
		mcp.WithDescription("Scan for ralph-enabled repos and return their current status"),
	), s.handleScan)

	srv.AddTool(mcp.NewTool("ralphglasses_list",
		mcp.WithDescription("List all discovered repos with status summary"),
	), s.handleList)

	srv.AddTool(mcp.NewTool("ralphglasses_fleet_status",
		mcp.WithDescription("Fleet-wide dashboard: aggregate status, costs, health, and alerts across all repos and sessions in one call"),
	), s.handleFleetStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_status",
		mcp.WithDescription("Get detailed status for a specific repo including loop status, circuit breaker, progress, and config"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name (basename of directory)")),
	), s.handleStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_start",
		mcp.WithDescription("Start a ralph loop for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to start loop for")),
	), s.handleStart)

	srv.AddTool(mcp.NewTool("ralphglasses_stop",
		mcp.WithDescription("Stop a running ralph loop for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to stop loop for")),
	), s.handleStop)

	srv.AddTool(mcp.NewTool("ralphglasses_stop_all",
		mcp.WithDescription("Stop all running ralph loops"),
	), s.handleStopAll)

	srv.AddTool(mcp.NewTool("ralphglasses_pause",
		mcp.WithDescription("Pause or resume a running ralph loop"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to pause/resume")),
	), s.handlePause)

	srv.AddTool(mcp.NewTool("ralphglasses_logs",
		mcp.WithDescription("Get recent log lines from a repo's ralph log"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 50, max 500)")),
	), s.handleLogs)

	srv.AddTool(mcp.NewTool("ralphglasses_config",
		mcp.WithDescription("Get or set .ralphrc config values for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("key", mcp.Description("Config key to get/set (omit to list all)")),
		mcp.WithString("value", mcp.Description("Value to set (omit to get current value)")),
	), s.handleConfig)

	// Roadmap automation tools

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_parse",
		mcp.WithDescription("Parse ROADMAP.md into structured JSON (phases, sections, tasks, deps, completion stats)"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root or direct .md path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
	), s.handleRoadmapParse)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_analyze",
		mcp.WithDescription("Compare roadmap vs codebase — find gaps, stale checkboxes, ready tasks, orphaned code"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
	), s.handleRoadmapAnalyze)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_research",
		mcp.WithDescription("Search GitHub for relevant repos and tools that unlock new capabilities"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("topics", mcp.Description("Search topics (inferred from go.mod/README if omitted)")),
		mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
	), s.handleRoadmapResearch)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_expand",
		mcp.WithDescription("Generate proposed roadmap expansions from analysis gaps and research findings"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
		mcp.WithString("research", mcp.Description("Research topics to include (runs research internally)")),
		mcp.WithString("style", mcp.Description("Expansion style: conservative, balanced, aggressive (default: balanced)")),
	), s.handleRoadmapExpand)

	srv.AddTool(mcp.NewTool("ralphglasses_roadmap_export",
		mcp.WithDescription("Export roadmap items as structured task specs for ralph loop consumption"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
		mcp.WithString("format", mcp.Description("Output format: rdcycle, fix_plan, progress (default: rdcycle)")),
		mcp.WithString("phase", mcp.Description("Filter by phase name (default: all)")),
		mcp.WithString("section", mcp.Description("Filter by section name (default: all)")),
		mcp.WithNumber("max_tasks", mcp.Description("Max tasks to export (default 20)")),
		mcp.WithString("respect_deps", mcp.Description("Skip tasks with unmet deps (default: true)")),
	), s.handleRoadmapExport)

	// Repo file management tools

	srv.AddTool(mcp.NewTool("ralphglasses_repo_scaffold",
		mcp.WithDescription("Create/initialize ralph config files (.ralph/, .ralphrc, PROMPT.md, AGENT.md, fix_plan.md) for a repo"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("project_type", mcp.Description("Project type override (auto-detected from go.mod, package.json, etc.)")),
		mcp.WithString("project_name", mcp.Description("Project name override (defaults to directory name)")),
		mcp.WithString("force", mcp.Description("Overwrite existing files: true/false (default: false)")),
	), s.handleRepoScaffold)

	srv.AddTool(mcp.NewTool("ralphglasses_repo_optimize",
		mcp.WithDescription("Analyze and optimize ralph config files — detect misconfigs, missing settings, stale plans"),
		mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
		mcp.WithString("focus", mcp.Description("Focus area: config, prompt, plan, all (default: all)")),
		mcp.WithString("dry_run", mcp.Description("Report only, don't modify: true/false (default: true)")),
	), s.handleRepoOptimize)

	// Claude Code session management tools

	srv.AddTool(mcp.NewTool("ralphglasses_session_launch",
		mcp.WithDescription("Launch a headless LLM CLI session (claude/gemini/codex) for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Prompt/task to send")),
		mcp.WithString("provider", mcp.Description("LLM provider: claude (default), gemini, codex")),
		mcp.WithString("model", mcp.Description("Model to use")),
		mcp.WithNumber("max_budget_usd", mcp.Description("Maximum spend in USD")),
		mcp.WithNumber("max_turns", mcp.Description("Maximum conversation turns")),
		mcp.WithString("agent", mcp.Description("Agent name (from .claude/agents/)")),
		mcp.WithString("allowed_tools", mcp.Description("Comma-separated allowed tools (e.g. Bash,Read,Edit)")),
		mcp.WithString("system_prompt", mcp.Description("Additional system prompt to append")),
		mcp.WithString("session_name", mcp.Description("Human-readable session name")),
		mcp.WithString("worktree", mcp.Description("Git worktree isolation (true for auto, or branch name)")),
		mcp.WithString("enhance_prompt", mcp.Description("Auto-enhance the prompt before launch: local (deterministic), llm (Claude API), auto (try LLM, fallback). Omit to skip enhancement")),
	), s.handleSessionLaunch)

	srv.AddTool(mcp.NewTool("ralphglasses_session_list",
		mcp.WithDescription("List all tracked LLM sessions with status, cost, and turns"),
		mcp.WithString("repo", mcp.Description("Filter by repo name (omit for all)")),
		mcp.WithString("provider", mcp.Description("Filter by provider: claude, gemini, codex (omit for all)")),
		mcp.WithString("status", mcp.Description("Filter by status: running, completed, errored, stopped")),
	), s.handleSessionList)

	srv.AddTool(mcp.NewTool("ralphglasses_session_status",
		mcp.WithDescription("Get detailed status for a Claude Code session including output, cost, and turns"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
	), s.handleSessionStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_session_resume",
		mcp.WithDescription("Resume a previous LLM CLI session"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("session_id", mcp.Required(), mcp.Description("Provider session ID to resume (from session status)")),
		mcp.WithString("provider", mcp.Description("LLM provider: claude (default), gemini, codex")),
		mcp.WithString("prompt", mcp.Description("Follow-up prompt (optional)")),
	), s.handleSessionResume)

	srv.AddTool(mcp.NewTool("ralphglasses_session_stop",
		mcp.WithDescription("Stop a running Claude Code session"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to stop")),
	), s.handleSessionStop)

	srv.AddTool(mcp.NewTool("ralphglasses_session_budget",
		mcp.WithDescription("Get cost/budget info for a session, or update budget"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
		mcp.WithNumber("budget", mcp.Description("New budget in USD (omit to just query)")),
	), s.handleSessionBudget)

	// Agent team tools

	srv.AddTool(mcp.NewTool("ralphglasses_team_create",
		mcp.WithDescription("Create an agent team with a lead session that delegates tasks to teammates"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
		mcp.WithString("tasks", mcp.Required(), mcp.Description("Newline-separated task descriptions")),
		mcp.WithString("provider", mcp.Description("LLM provider for lead: claude (default), gemini, codex")),
		mcp.WithString("worker_provider", mcp.Description("Default LLM provider for worker tasks: claude, gemini, codex")),
		mcp.WithString("lead_agent", mcp.Description("Agent definition for the lead (from .claude/agents/)")),
		mcp.WithString("model", mcp.Description("Model for lead session")),
		mcp.WithNumber("max_budget_usd", mcp.Description("Total budget for the team")),
	), s.handleTeamCreate)

	srv.AddTool(mcp.NewTool("ralphglasses_team_status",
		mcp.WithDescription("Get team status including lead session, tasks, and progress"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
	), s.handleTeamStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_team_delegate",
		mcp.WithDescription("Add a new task to an existing team"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
		mcp.WithString("task", mcp.Required(), mcp.Description("Task description to delegate")),
		mcp.WithString("provider", mcp.Description("LLM provider override for this task: claude, gemini, codex")),
	), s.handleTeamDelegate)

	// Agent definition tools

	srv.AddTool(mcp.NewTool("ralphglasses_agent_define",
		mcp.WithDescription("Create or update an agent definition for a repo (supports all providers)"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Agent name")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Agent system prompt / instructions (markdown)")),
		mcp.WithString("provider", mcp.Description("Target provider: claude (default, .claude/agents/), gemini (.gemini/agents/), codex (AGENTS.md)")),
		mcp.WithString("description", mcp.Description("Agent description")),
		mcp.WithString("model", mcp.Description("Model override (sonnet, opus, haiku)")),
		mcp.WithString("tools", mcp.Description("Comma-separated allowed tools")),
		mcp.WithNumber("max_turns", mcp.Description("Max turns for this agent")),
	), s.handleAgentDefine)

	srv.AddTool(mcp.NewTool("ralphglasses_agent_list",
		mcp.WithDescription("List available agent definitions for a repo (supports all providers)"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("provider", mcp.Description("Filter by provider: claude (default), gemini, codex, or 'all'")),
	), s.handleAgentList)

	// Event bus tools

	srv.AddTool(mcp.NewTool("ralphglasses_event_list",
		mcp.WithDescription("Query recent fleet events from the event bus"),
		mcp.WithString("type", mcp.Description("Filter by event type (e.g. session.started, cost.update)")),
		mcp.WithString("repo", mcp.Description("Filter by repo name")),
		mcp.WithNumber("limit", mcp.Description("Max events to return (default 50)")),
		mcp.WithString("since", mcp.Description("ISO timestamp filter")),
	), s.handleEventList)

	// Fleet analytics tools

	srv.AddTool(mcp.NewTool("ralphglasses_fleet_analytics",
		mcp.WithDescription("Cost breakdown by provider/repo/time-period with trend analysis"),
		mcp.WithString("repo", mcp.Description("Filter by repo name")),
		mcp.WithString("provider", mcp.Description("Filter by provider")),
	), s.handleFleetAnalytics)

	srv.AddTool(mcp.NewTool("ralphglasses_session_compare",
		mcp.WithDescription("Compare two sessions by ID: cost, turns, duration, provider efficiency"),
		mcp.WithString("id1", mcp.Required(), mcp.Description("First session ID")),
		mcp.WithString("id2", mcp.Required(), mcp.Description("Second session ID")),
	), s.handleSessionCompare)

	srv.AddTool(mcp.NewTool("ralphglasses_session_output",
		mcp.WithDescription("Get recent output from a session's output history"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
		mcp.WithNumber("lines", mcp.Description("Number of output lines (default 20, max 100)")),
	), s.handleSessionOutput)

	srv.AddTool(mcp.NewTool("ralphglasses_repo_health",
		mcp.WithDescription("Composite health check: circuit breaker, budget, staleness, errors, active sessions"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
	), s.handleRepoHealth)

	srv.AddTool(mcp.NewTool("ralphglasses_session_retry",
		mcp.WithDescription("Re-launch a failed session with same params, optional overrides"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to retry")),
		mcp.WithString("model", mcp.Description("Override model")),
		mcp.WithNumber("max_budget_usd", mcp.Description("Override budget")),
	), s.handleSessionRetry)

	srv.AddTool(mcp.NewTool("ralphglasses_config_bulk",
		mcp.WithDescription("Get/set .ralphrc values across multiple repos"),
		mcp.WithString("key", mcp.Required(), mcp.Description("Config key to get/set")),
		mcp.WithString("value", mcp.Description("Value to set (omit to query)")),
		mcp.WithString("repos", mcp.Description("Comma-separated repo names (default: all)")),
	), s.handleConfigBulk)

	srv.AddTool(mcp.NewTool("ralphglasses_workflow_define",
		mcp.WithDescription("Define a multi-step workflow as YAML"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name")),
		mcp.WithString("yaml", mcp.Required(), mcp.Description("Workflow YAML definition")),
	), s.handleWorkflowDefine)

	srv.AddTool(mcp.NewTool("ralphglasses_workflow_run",
		mcp.WithDescription("Execute a defined workflow, launching sessions per step"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name")),
	), s.handleWorkflowRun)

	srv.AddTool(mcp.NewTool("ralphglasses_snapshot",
		mcp.WithDescription("Save or list fleet state snapshots"),
		mcp.WithString("action", mcp.Description("Action: save (default) or list")),
		mcp.WithString("name", mcp.Description("Snapshot name (auto-generated if omitted)")),
	), s.handleSnapshot)

	// Prompt enhancement tools

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_analyze",
		mcp.WithDescription("Score a prompt across 10 quality dimensions (clarity, specificity, structure, examples, etc.) with letter grades and actionable suggestions"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to analyze")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
	), s.handlePromptAnalyze)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_enhance",
		mcp.WithDescription("Run the 13-stage prompt enhancement pipeline (specificity, positive reframing, XML structure, context reorder, format enforcement, etc.)"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to enhance")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
		mcp.WithString("mode", mcp.Description("Enhancement mode: local (default, deterministic), llm (Claude API), auto (try LLM, fallback to local)")),
		mcp.WithString("repo", mcp.Description("Repo name to load .prompt-improver.yaml config from")),
	), s.handlePromptEnhance)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_lint",
		mcp.WithDescription("Deep-lint a prompt for anti-patterns: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to lint")),
	), s.handlePromptLint)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_improve",
		mcp.WithDescription("LLM-powered prompt improvement using Claude with domain-specific meta-prompts (requires ANTHROPIC_API_KEY)"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to improve")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
		mcp.WithBoolean("thinking_enabled", mcp.Description("Include thinking scaffolding in the improved prompt")),
		mcp.WithString("feedback", mcp.Description("Optional feedback to guide the improvement direction")),
	), s.handlePromptImprove)

	// Prompt utility tools

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_templates",
		mcp.WithDescription("List available prompt templates with descriptions and required variables"),
	), s.handlePromptTemplates)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_template_fill",
		mcp.WithDescription("Fill a prompt template with variable values"),
		mcp.WithString("name", mcp.Required(), mcp.Description("Template name")),
		mcp.WithString("vars", mcp.Required(), mcp.Description("JSON object of variable key-value pairs")),
	), s.handlePromptTemplateFill)

	srv.AddTool(mcp.NewTool("ralphglasses_claudemd_check",
		mcp.WithDescription("Health-check a repo's CLAUDE.md for common issues (length, inline code, overtrigger language, missing headers)"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
	), s.handleClaudeMDCheck)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_classify",
		mcp.WithDescription("Classify a prompt's task type (code, troubleshooting, analysis, creative, workflow, general)"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to classify")),
	), s.handlePromptClassify)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_should_enhance",
		mcp.WithDescription("Check whether a prompt would benefit from enhancement"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to check")),
		mcp.WithString("repo", mcp.Description("Repo name for loading .prompt-improver.yaml config")),
	), s.handlePromptShouldEnhance)
}

func (s *Server) scan() error {
	repos, err := discovery.Scan(s.ScanPath)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.Repos = repos
	s.mu.Unlock()
	return nil
}

func (s *Server) findRepo(name string) *model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, r := range s.Repos {
		if r.Name == name {
			return r
		}
	}
	return nil
}

func (s *Server) reposCopy() []*model.Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make([]*model.Repo, len(s.Repos))
	copy(cp, s.Repos)
	return cp
}

func (s *Server) reposNil() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.Repos == nil
}

func textResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: text,
		}},
	}
}

func errResult(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: msg,
		}},
	}
}

func jsonResult(v any) *mcp.CallToolResult {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return errResult(fmt.Sprintf("json marshal: %v", err))
	}
	return textResult(string(data))
}

func argsMap(req mcp.CallToolRequest) map[string]any {
	if m, ok := req.Params.Arguments.(map[string]any); ok {
		return m
	}
	return nil
}

func getStringArg(req mcp.CallToolRequest, key string) string {
	m := argsMap(req)
	if m == nil {
		return ""
	}
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func getNumberArg(req mcp.CallToolRequest, key string, defaultVal float64) float64 {
	m := argsMap(req)
	if m == nil {
		return defaultVal
	}
	if v, ok := m[key]; ok {
		if n, ok := v.(float64); ok {
			return n
		}
	}
	return defaultVal
}

func getBoolArg(req mcp.CallToolRequest, key string) bool {
	m := argsMap(req)
	if m == nil {
		return false
	}
	if v, ok := m[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return false
}
