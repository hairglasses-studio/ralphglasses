package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/awesome"
	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Server holds state for the MCP server.
type Server struct {
	mu           sync.RWMutex
	ScanPath     string
	Repos        []*model.Repo
	ProcMgr      *process.Manager
	SessMgr      *session.Manager
	EventBus     *events.Bus
	HTTPClient   *http.Client
	Engine       *enhancer.HybridEngine
	engineOnce   sync.Once
	ToolRecorder *ToolCallRecorder
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
		mcp.WithString("no_journal", mcp.Description("Skip improvement journal injection: true/false (default: false)")),
		mcp.WithString("enhance_prompt", mcp.Description("Auto-enhance the prompt before launch: local (deterministic), llm (Claude API), auto (try LLM, fallback). Omit to skip enhancement")),
		mcp.WithString("target_provider", mcp.Description("Target LLM provider for prompt enhancement: claude, gemini, openai (defaults to session provider)")),
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

	// Improvement journal tools

	srv.AddTool(mcp.NewTool("ralphglasses_journal_read",
		mcp.WithDescription("Read improvement journal entries for a repo with synthesized context"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithNumber("limit", mcp.Description("Max entries to return (default 10)")),
	), s.handleJournalRead)

	srv.AddTool(mcp.NewTool("ralphglasses_journal_write",
		mcp.WithDescription("Manually write an improvement note to a repo's journal"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("worked", mcp.Description("Comma-separated items that worked")),
		mcp.WithString("failed", mcp.Description("Comma-separated items that failed")),
		mcp.WithString("suggest", mcp.Description("Comma-separated suggestions")),
		mcp.WithString("session_id", mcp.Description("Associated session ID (optional)")),
	), s.handleJournalWrite)

	srv.AddTool(mcp.NewTool("ralphglasses_journal_prune",
		mcp.WithDescription("Compact improvement journal to prevent unbounded growth"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithNumber("keep", mcp.Description("Number of entries to keep (default 100)")),
		mcp.WithString("dry_run", mcp.Description("Preview only, don't modify: true/false (default: true)")),
	), s.handleJournalPrune)

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

	srv.AddTool(mcp.NewTool("ralphglasses_loop_start",
		mcp.WithDescription("Create a multi-provider planner/worker perpetual development loop for a repo"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("planner_model", mcp.Description("Planner model (default: o1-pro)")),
		mcp.WithString("worker_model", mcp.Description("Worker model (default: gpt-5.4-xhigh)")),
		mcp.WithString("verifier_model", mcp.Description("Verifier model metadata (default: gpt-5.4-xhigh)")),
		mcp.WithString("planner_provider", mcp.Description("Planner provider: claude, gemini, codex (default: codex)")),
		mcp.WithString("worker_provider", mcp.Description("Worker provider: claude, gemini, codex (default: codex)")),
		mcp.WithString("verifier_provider", mcp.Description("Verifier provider: claude, gemini, codex (default: codex)")),
		mcp.WithString("verify_commands", mcp.Description("Newline-separated verification commands (default: ./scripts/dev/ci.sh)")),
		mcp.WithNumber("retry_limit", mcp.Description("Maximum consecutive failed iterations before step is refused")),
		mcp.WithNumber("max_concurrent_workers", mcp.Description("Maximum concurrent workers (currently only 1 supported)")),
		mcp.WithString("worktree_policy", mcp.Description("Worktree isolation policy (default: git)")),
	), s.handleLoopStart)

	srv.AddTool(mcp.NewTool("ralphglasses_loop_status",
		mcp.WithDescription("Get status for a perpetual development loop"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
	), s.handleLoopStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_loop_step",
		mcp.WithDescription("Execute one planner/worker/verifier iteration for a loop"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
	), s.handleLoopStep)

	srv.AddTool(mcp.NewTool("ralphglasses_loop_stop",
		mcp.WithDescription("Stop a perpetual development loop"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
	), s.handleLoopStop)

	srv.AddTool(mcp.NewTool("ralphglasses_snapshot",
		mcp.WithDescription("Save or list fleet state snapshots"),
		mcp.WithString("action", mcp.Description("Action: save (default) or list")),
		mcp.WithString("name", mcp.Description("Snapshot name (auto-generated if omitted)")),
	), s.handleSnapshot)

	srv.AddTool(mcp.NewTool("ralphglasses_agent_compose",
		mcp.WithDescription("Create a composite agent by layering multiple existing agent definitions"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the composite agent")),
		mcp.WithString("agents", mcp.Required(), mcp.Description("Comma-separated agent names to compose")),
		mcp.WithString("provider", mcp.Description("Provider: claude (default), gemini, codex")),
		mcp.WithString("model", mcp.Description("Override model for composite agent")),
	), s.handleAgentCompose)

	srv.AddTool(mcp.NewTool("ralphglasses_session_stop_all",
		mcp.WithDescription("Stop all running LLM sessions — emergency cost cutoff"),
	), s.handleSessionStopAll)

	// Awesome-list research tools

	srv.AddTool(mcp.NewTool("ralphglasses_awesome_fetch",
		mcp.WithDescription("Fetch and parse an awesome-list README into structured entries with categories"),
		mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
	), s.handleAwesomeFetch)

	srv.AddTool(mcp.NewTool("ralphglasses_awesome_analyze",
		mcp.WithDescription("Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities"),
		mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
		mcp.WithNumber("max_workers", mcp.Description("Concurrent README fetches (default 5)")),
	), s.handleAwesomeAnalyze)

	srv.AddTool(mcp.NewTool("ralphglasses_awesome_diff",
		mcp.WithDescription("Compare current awesome-list against previous fetch (new/removed entries)"),
		mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path where previous index is saved")),
		mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
	), s.handleAwesomeDiff)

	srv.AddTool(mcp.NewTool("ralphglasses_awesome_report",
		mcp.WithDescription("Generate formatted report from saved analysis results"),
		mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path where analysis is saved")),
		mcp.WithString("format", mcp.Description("Output format: json or markdown (default: markdown)")),
	), s.handleAwesomeReport)

	srv.AddTool(mcp.NewTool("ralphglasses_awesome_sync",
		mcp.WithDescription("Full pipeline: fetch awesome-list → diff → analyze new entries → report → save"),
		mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path to save results")),
		mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
		mcp.WithString("full_rescan", mcp.Description("Re-analyze all entries, not just new: true/false (default: false)")),
		mcp.WithNumber("max_workers", mcp.Description("Concurrent README fetches (default 5)")),
	), s.handleAwesomeSync)

	// Prompt enhancement tools

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_analyze",
		mcp.WithDescription("Score a prompt across 10 quality dimensions (clarity, specificity, structure, examples, etc.) with letter grades and actionable suggestions"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to analyze")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
		mcp.WithString("target_provider", mcp.Description("Target model provider for scoring suggestions: claude (default), gemini, openai")),
	), s.handlePromptAnalyze)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_enhance",
		mcp.WithDescription("Run the 13-stage prompt enhancement pipeline (specificity, positive reframing, XML structure, context reorder, format enforcement, etc.)"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to enhance")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
		mcp.WithString("mode", mcp.Description("Enhancement mode: local (default, deterministic), llm (Claude/Gemini/OpenAI API), auto (try LLM, fallback to local)")),
		mcp.WithString("repo", mcp.Description("Repo name to load .prompt-improver.yaml config from")),
		mcp.WithString("target_provider", mcp.Description("Target model provider — controls structure style and scoring: claude (default), gemini, openai")),
	), s.handlePromptEnhance)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_lint",
		mcp.WithDescription("Deep-lint a prompt for anti-patterns: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to lint")),
	), s.handlePromptLint)

	srv.AddTool(mcp.NewTool("ralphglasses_prompt_improve",
		mcp.WithDescription("LLM-powered prompt improvement using Claude, Gemini, or OpenAI with domain-specific meta-prompts"),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to improve")),
		mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
		mcp.WithBoolean("thinking_enabled", mcp.Description("Include thinking scaffolding in the improved prompt")),
		mcp.WithString("feedback", mcp.Description("Optional feedback to guide the improvement direction")),
		mcp.WithString("provider", mcp.Description("LLM provider for improvement: claude (default, requires ANTHROPIC_API_KEY), gemini (requires GOOGLE_API_KEY), openai (requires OPENAI_API_KEY)")),
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

	// Marathon monitoring tools

	srv.AddTool(mcp.NewTool("ralphglasses_session_tail",
		mcp.WithDescription("Tail session output with cursor — returns only new lines since last call"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
		mcp.WithString("cursor", mcp.Description("Cursor from previous response (omit for latest)")),
		mcp.WithNumber("lines", mcp.Description("Max lines to return (default 30, max 100)")),
	), s.handleSessionTail)

	srv.AddTool(mcp.NewTool("ralphglasses_session_diff",
		mcp.WithDescription("Git changes made during a session's execution window"),
		mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
		mcp.WithString("stat_only", mcp.Description("true/false (default: true)")),
		mcp.WithNumber("max_lines", mcp.Description("Truncate diff at N lines (default 200)")),
	), s.handleSessionDiff)

	srv.AddTool(mcp.NewTool("ralphglasses_marathon_dashboard",
		mcp.WithDescription("Compact marathon status: burn rate, stale sessions, team progress, alerts"),
		mcp.WithNumber("stale_threshold_min", mcp.Description("Minutes idle before flagged stale (default 5)")),
	), s.handleMarathonDashboard)

	srv.AddTool(mcp.NewTool("ralphglasses_session_errors",
		mcp.WithDescription("Aggregated error view: parse failures, API errors, budget warnings"),
		mcp.WithString("repo", mcp.Description("Filter by repo name")),
		mcp.WithString("severity", mcp.Description("Filter: critical, warning, info")),
		mcp.WithNumber("limit", mcp.Description("Max errors (default 50)")),
	), s.handleSessionErrors)

	// Remote Control (rc) tools — optimized for mobile via Claude Android/iOS app

	srv.AddTool(mcp.NewTool("ralphglasses_rc_status",
		mcp.WithDescription("Compact fleet overview for mobile: active sessions, costs, alerts in readable text"),
	), s.handleRCStatus)

	srv.AddTool(mcp.NewTool("ralphglasses_rc_send",
		mcp.WithDescription("Send prompt to repo — auto-stops existing session, launches new. The 'input' tool for remote control."),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("What to tell the agent")),
		mcp.WithString("provider", mcp.Description("claude (default), gemini, codex")),
		mcp.WithString("model", mcp.Description("Override model")),
		mcp.WithNumber("budget", mcp.Description("Max budget USD (default: 5)")),
		mcp.WithString("resume", mcp.Description("true to resume last session instead of fresh start")),
	), s.handleRCSend)

	srv.AddTool(mcp.NewTool("ralphglasses_rc_read",
		mcp.WithDescription("Read recent output from most active session. Combines tail + status for mobile."),
		mcp.WithString("id", mcp.Description("Session ID (omit for most recently active)")),
		mcp.WithString("cursor", mcp.Description("Cursor from previous call — only new output")),
		mcp.WithNumber("lines", mcp.Description("Max lines (default 10, max 30)")),
	), s.handleRCRead)

	srv.AddTool(mcp.NewTool("ralphglasses_event_poll",
		mcp.WithDescription("Poll for new fleet events since last check. Cursor-based for efficient mobile polling."),
		mcp.WithString("cursor", mcp.Description("Cursor from previous response (omit for recent)")),
		mcp.WithNumber("limit", mcp.Description("Max events (default 20, max 50)")),
		mcp.WithString("type", mcp.Description("Filter by event type (e.g. session.started, cost.update)")),
	), s.handleEventPoll)

	srv.AddTool(mcp.NewTool("ralphglasses_rc_act",
		mcp.WithDescription("Quick fleet action: stop, stop_all, pause, resume, retry. Single tool for all control actions."),
		mcp.WithString("action", mcp.Required(), mcp.Description("Action: stop, stop_all, pause, resume, retry")),
		mcp.WithString("target", mcp.Description("Session ID or repo name (required except stop_all)")),
	), s.handleRCAct)

	// Distributed fleet tools

	srv.AddTool(mcp.NewTool("ralphglasses_fleet_submit",
		mcp.WithDescription("Submit work to the distributed fleet queue for execution on any worker"),
		mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
		mcp.WithString("prompt", mcp.Required(), mcp.Description("Task prompt")),
		mcp.WithString("provider", mcp.Description("claude (default), gemini, codex")),
		mcp.WithNumber("budget", mcp.Description("Max budget USD (default: 5)")),
		mcp.WithNumber("priority", mcp.Description("Priority 0-10 (default: 5, higher = first)")),
	), s.handleFleetSubmit)

	srv.AddTool(mcp.NewTool("ralphglasses_fleet_budget",
		mcp.WithDescription("View or set the fleet-wide budget. Shows spent, remaining, and active work."),
		mcp.WithNumber("limit", mcp.Description("New budget limit in USD (omit to just view)")),
	), s.handleFleetBudget)

	srv.AddTool(mcp.NewTool("ralphglasses_fleet_workers",
		mcp.WithDescription("List registered fleet workers with status, capacity, and spend"),
	), s.handleFleetWorkers)

	// Self-improvement / HITL tools

	srv.AddTool(mcp.NewTool("ralphglasses_hitl_score",
		mcp.WithDescription("Current human-in-the-loop score: manual interventions vs autonomous actions, with trend"),
		mcp.WithNumber("hours", mcp.Description("Time window in hours (default: 24)")),
	), s.handleHITLScore)

	srv.AddTool(mcp.NewTool("ralphglasses_hitl_history",
		mcp.WithDescription("Recent HITL events: manual stops, auto-recoveries, config changes, etc."),
		mcp.WithNumber("hours", mcp.Description("Time window in hours (default: 24)")),
		mcp.WithNumber("limit", mcp.Description("Max events (default: 50)")),
	), s.handleHITLHistory)

	srv.AddTool(mcp.NewTool("ralphglasses_autonomy_level",
		mcp.WithDescription("View or set the autonomy level (0=observe, 1=auto-recover, 2=auto-optimize, 3=full-autonomy)"),
		mcp.WithString("level", mcp.Description("New level: 0-3 or name (omit to view current)")),
	), s.handleAutonomyLevel)

	srv.AddTool(mcp.NewTool("ralphglasses_autonomy_decisions",
		mcp.WithDescription("Recent autonomous decisions with rationale, inputs, and outcomes"),
		mcp.WithNumber("limit", mcp.Description("Max decisions (default: 20)")),
	), s.handleAutonomyDecisions)

	srv.AddTool(mcp.NewTool("ralphglasses_autonomy_override",
		mcp.WithDescription("Override/reverse an autonomous decision and record human intervention"),
		mcp.WithString("decision_id", mcp.Required(), mcp.Description("Decision ID to override")),
		mcp.WithString("details", mcp.Description("Why this was overridden")),
	), s.handleAutonomyOverride)

	srv.AddTool(mcp.NewTool("ralphglasses_feedback_profiles",
		mcp.WithDescription("View feedback profiles: per-task-type and per-provider performance data from journal analysis"),
		mcp.WithString("type", mcp.Description("Filter: prompt, provider, or omit for both")),
	), s.handleFeedbackProfiles)

	srv.AddTool(mcp.NewTool("ralphglasses_provider_recommend",
		mcp.WithDescription("Recommend best provider + model + budget for a task based on feedback profiles and cost normalization"),
		mcp.WithString("task", mcp.Required(), mcp.Description("Task description (e.g. 'fix lint errors', 'add search feature')")),
	), s.handleProviderRecommend)

	// Tool benchmarking

	srv.AddTool(mcp.NewTool("ralphglasses_tool_benchmark",
		mcp.WithDescription("Per-tool performance benchmarks: latency percentiles, success rates, and regression detection"),
		mcp.WithString("tool", mcp.Description("Filter to a specific tool name")),
		mcp.WithString("compare", mcp.Description("Include regression analysis vs previous baseline: true/false (default: false)")),
		mcp.WithNumber("hours", mcp.Description("Time window in hours (default 24)")),
	), s.handleToolBenchmark)
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

// errCode returns a structured error result with an error_code field.
// error_code values: "invalid_params", "not_found", "internal_error"
func errCode(code, msg string) *mcp.CallToolResult {
	data, _ := json.Marshal(map[string]string{
		"error":      msg,
		"error_code": code,
	})
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(data),
		}},
	}
}

func invalidParams(msg string) *mcp.CallToolResult { return errCode("invalid_params", msg) }
func notFound(msg string) *mcp.CallToolResult      { return errCode("not_found", msg) }
func internalErr(msg string) *mcp.CallToolResult   { return errCode("internal_error", msg) }

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

func splitLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
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

// Handlers

func (s *Server) handleScan(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if err := s.scan(); err != nil {
		return errResult(fmt.Sprintf("scan failed: %v", err)), nil
	}
	repos := s.reposCopy()
	if s.EventBus != nil {
		s.EventBus.Publish(events.Event{
			Type: events.ScanComplete,
			Data: map[string]any{"repo_count": len(repos)},
		})
	}
	return textResult(fmt.Sprintf("Found %d ralph-enabled repos", len(repos))), nil
}

func (s *Server) handleList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	repos := s.reposCopy()
	for _, r := range repos {
		model.RefreshRepo(r)
	}

	type repoSummary struct {
		Name    string `json:"name"`
		Status  string `json:"status"`
		Loop    int    `json:"loop_count"`
		Calls   string `json:"calls"`
		Circuit string `json:"circuit"`
		Running bool   `json:"managed"`
	}

	summaries := make([]repoSummary, len(repos))
	for i, r := range repos {
		loop := 0
		if r.Status != nil {
			loop = r.Status.LoopCount
		}
		summaries[i] = repoSummary{
			Name:    r.Name,
			Status:  r.StatusDisplay(),
			Loop:    loop,
			Calls:   r.CallsDisplay(),
			Circuit: r.CircuitDisplay(),
			Running: s.ProcMgr.IsRunning(r.Path),
		}
	}
	return jsonResult(summaries), nil
}

func (s *Server) handleStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	model.RefreshRepo(r)

	detail := map[string]any{
		"name":    r.Name,
		"path":    r.Path,
		"managed": s.ProcMgr.IsRunning(r.Path),
		"paused":  s.ProcMgr.IsPaused(r.Path),
	}
	if pid := s.ProcMgr.PidForRepo(r.Path); pid != 0 {
		detail["pid"] = pid
	}
	if r.Status != nil {
		detail["status"] = r.Status
	}
	if r.Circuit != nil {
		detail["circuit_breaker"] = r.Circuit
	}
	if r.Progress != nil {
		detail["progress"] = r.Progress
	}
	if r.Config != nil {
		detail["config"] = r.Config.Values
	}
	return jsonResult(detail), nil
}

func (s *Server) handleStart(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Start(r.Path); err != nil {
		return internalErr(fmt.Sprintf("start failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Started ralph loop for %s", name)), nil
}

func (s *Server) handleStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}
	if err := s.ProcMgr.Stop(r.Path); err != nil {
		return internalErr(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped ralph loop for %s", name)), nil
}

func (s *Server) handleStopAll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.ProcMgr.StopAll()
	return textResult("All managed loops stopped"), nil
}

func (s *Server) handlePause(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	paused, err := s.ProcMgr.TogglePause(r.Path)
	if err != nil {
		return errResult(fmt.Sprintf("pause toggle failed: %v", err)), nil
	}
	if paused {
		return textResult(fmt.Sprintf("Paused loop for %s", name)), nil
	}
	return textResult(fmt.Sprintf("Resumed loop for %s", name)), nil
}

func (s *Server) handleLogs(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	maxLines := int(getNumberArg(req, "lines", 50))
	if maxLines > 500 {
		maxLines = 500
	}

	allLines, err := process.ReadFullLog(r.Path)
	if err != nil {
		return errResult(fmt.Sprintf("read log: %v", err)), nil
	}

	start := 0
	if len(allLines) > maxLines {
		start = len(allLines) - maxLines
	}
	return textResult(strings.Join(allLines[start:], "\n")), nil
}

func (s *Server) handleConfig(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	if r.Config == nil {
		return errResult(fmt.Sprintf("no .ralphrc found for %s", name)), nil
	}

	key := getStringArg(req, "key")
	value := getStringArg(req, "value")

	// List all
	if key == "" {
		return jsonResult(r.Config.Values), nil
	}

	// Set value
	if value != "" {
		r.Config.Values[key] = value
		if err := r.Config.Save(); err != nil {
			return errResult(fmt.Sprintf("save config: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Set %s=%s for %s", key, value, name)), nil
	}

	// Get value
	v := r.Config.Get(key, "")
	if v == "" {
		return errResult(fmt.Sprintf("key not found: %s", key)), nil
	}
	return textResult(fmt.Sprintf("%s=%s", key, v)), nil
}

// Roadmap handlers

func (s *Server) handleRoadmapParse(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}
	return jsonResult(rm), nil
}

func (s *Server) handleRoadmapAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	rmPath := roadmap.ResolvePath(path, file)

	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return errResult(fmt.Sprintf("analyze: %v", err)), nil
	}
	return jsonResult(analysis), nil
}

func (s *Server) handleRoadmapResearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	topics := getStringArg(req, "topics")
	limit := int(getNumberArg(req, "limit", 10))

	results, err := roadmap.Research(ctx, s.HTTPClient, path, topics, limit)
	if err != nil {
		return errResult(fmt.Sprintf("research: %v", err)), nil
	}
	return jsonResult(results), nil
}

func (s *Server) handleRoadmapExpand(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	style := getStringArg(req, "style")
	researchTopics := getStringArg(req, "research")

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	analysis, err := roadmap.Analyze(rm, path)
	if err != nil {
		return errResult(fmt.Sprintf("analyze: %v", err)), nil
	}

	var research *roadmap.ResearchResults
	if researchTopics != "" {
		research, _ = roadmap.Research(ctx, s.HTTPClient, path, researchTopics, 10)
	}

	expansion, err := roadmap.Expand(rm, analysis, research, style)
	if err != nil {
		return errResult(fmt.Sprintf("expand: %v", err)), nil
	}
	return jsonResult(expansion), nil
}

func (s *Server) handleRoadmapExport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}
	file := getStringArg(req, "file")
	format := getStringArg(req, "format")
	phase := getStringArg(req, "phase")
	section := getStringArg(req, "section")
	maxTasks := int(getNumberArg(req, "max_tasks", 20))
	respectDeps := getStringArg(req, "respect_deps") != "false"

	rmPath := roadmap.ResolvePath(path, file)
	rm, err := roadmap.Parse(rmPath)
	if err != nil {
		return errResult(fmt.Sprintf("parse roadmap: %v", err)), nil
	}

	output, err := roadmap.Export(rm, format, phase, section, maxTasks, respectDeps)
	if err != nil {
		return errResult(fmt.Sprintf("export: %v", err)), nil
	}
	return textResult(output), nil
}

// Repo file handlers

func (s *Server) handleRepoScaffold(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}

	opts := repofiles.ScaffoldOptions{
		ProjectType: getStringArg(req, "project_type"),
		ProjectName: getStringArg(req, "project_name"),
		Force:       getStringArg(req, "force") == "true",
	}

	result, err := repofiles.Scaffold(path, opts)
	if err != nil {
		return errResult(fmt.Sprintf("scaffold: %v", err)), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handleRepoOptimize(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	path := getStringArg(req, "path")
	if path == "" {
		return errResult("path required"), nil
	}
	if err := ValidatePath(path, s.ScanPath); err != nil {
		return invalidParams(fmt.Sprintf("invalid path: %v", err)), nil
	}

	opts := repofiles.OptimizeOptions{
		DryRun: getStringArg(req, "dry_run") != "false",
		Focus:  getStringArg(req, "focus"),
	}

	result, err := repofiles.Optimize(path, opts)
	if err != nil {
		return errResult(fmt.Sprintf("optimize: %v", err)), nil
	}
	return jsonResult(result), nil
}

// Session handlers

func (s *Server) handleSessionLaunch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return invalidParams("prompt required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.ProviderClaude
	}
	if err := session.ValidateProvider(provider); err != nil {
		return invalidParams(fmt.Sprintf("invalid provider %q: %v", provider, err)), nil
	}

	opts := session.LaunchOptions{
		Provider:     provider,
		RepoPath:     r.Path,
		Prompt:       prompt,
		Model:        getStringArg(req, "model"),
		MaxBudgetUSD: getNumberArg(req, "max_budget_usd", 0),
		MaxTurns:     int(getNumberArg(req, "max_turns", 0)),
		Agent:        getStringArg(req, "agent"),
		SystemPrompt: getStringArg(req, "system_prompt"),
		SessionName:  getStringArg(req, "session_name"),
		Worktree:     getStringArg(req, "worktree"),
	}
	if tools := getStringArg(req, "allowed_tools"); tools != "" {
		opts.AllowedTools = strings.Split(tools, ",")
	}

	// Inject improvement context from journal
	if getStringArg(req, "no_journal") != "true" {
		journal, _ := session.ReadRecentJournal(r.Path, 5)
		if len(journal) > 0 {
			journalCtx := session.SynthesizeContext(journal)
			if journalCtx != "" {
				opts.Prompt = journalCtx + "\n\n---\n\n" + opts.Prompt
			}
		}
	}

	// Auto-enhance prompt if requested
	enhanceMode := getStringArg(req, "enhance_prompt")
	if enhanceMode != "" {
		cfg := enhancer.LoadConfig(r.Path)
		if enhancer.ShouldEnhance(prompt, cfg) {
			mode := enhancer.ValidMode(enhanceMode)
			if mode == "" {
				mode = enhancer.ModeLocal
			}
			targetProvider := enhancer.ProviderName(getStringArg(req, "target_provider"))
			if targetProvider == "" {
				targetProvider = mapSessionProvider(provider)
			}
			eResult := enhancer.EnhanceHybrid(ctx, prompt, "", cfg, s.getEngine(), mode, targetProvider)
			opts.Prompt = eResult.Enhanced
		}
	}

	sess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		return internalErr(fmt.Sprintf("launch failed: %v", err)), nil
	}

	result := map[string]any{
		"session_id": sess.ID,
		"provider":   sess.Provider,
		"repo":       sess.RepoName,
		"status":     sess.Status,
		"model":      sess.Model,
		"budget_usd": sess.BudgetUSD,
	}
	if warnings := session.UnsupportedOptionsWarnings(provider, opts); len(warnings) > 0 {
		result["warnings"] = warnings
	}
	if enhanceMode != "" && opts.Prompt != prompt {
		result["prompt_enhanced"] = true
		result["original_prompt"] = prompt
		if s.EventBus != nil {
			s.EventBus.Publish(events.Event{
				Type: events.PromptEnhanced,
				Data: map[string]any{"session_id": sess.ID, "repo": name},
			})
		}
	}

	return jsonResult(result), nil
}

func (s *Server) handleSessionList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")
	statusFilter := getStringArg(req, "status")

	var repoPath string
	if repoFilter != "" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return errResult(fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		r := s.findRepo(repoFilter)
		if r != nil {
			repoPath = r.Path
		}
	}

	sessions := s.SessMgr.List(repoPath)

	type sessionSummary struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		Model    string  `json:"model,omitempty"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
		Agent    string  `json:"agent,omitempty"`
		Team     string  `json:"team,omitempty"`
	}

	var summaries []sessionSummary
	for _, sess := range sessions {
		sess.Lock()
		status := string(sess.Status)
		provider := string(sess.Provider)
		spent := sess.SpentUSD
		turns := sess.TurnCount
		sess.Unlock()

		if statusFilter != "" && status != statusFilter {
			continue
		}
		if providerFilter != "" && provider != providerFilter {
			continue
		}

		summaries = append(summaries, sessionSummary{
			ID:       sess.ID,
			Provider: provider,
			Repo:     sess.RepoName,
			Status:   status,
			Model:    sess.Model,
			SpentUSD: spent,
			Turns:    turns,
			Agent:    sess.AgentName,
			Team:     sess.TeamName,
		})
	}

	if summaries == nil {
		summaries = []sessionSummary{}
	}
	return jsonResult(summaries), nil
}

func (s *Server) handleSessionStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return invalidParams("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return notFound(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	detail := map[string]any{
		"id":                  sess.ID,
		"provider":            sess.Provider,
		"provider_session_id": sess.ProviderSessionID,
		"repo":                sess.RepoName,
		"repo_path":           sess.RepoPath,
		"status":              sess.Status,
		"prompt":              sess.Prompt,
		"model":               sess.Model,
		"agent":               sess.AgentName,
		"team":                sess.TeamName,
		"budget_usd":          sess.BudgetUSD,
		"spent_usd":           sess.SpentUSD,
		"turns":               sess.TurnCount,
		"max_turns":           sess.MaxTurns,
		"launched_at":         sess.LaunchedAt,
		"last_activity":       sess.LastActivity,
		"exit_reason":         sess.ExitReason,
		"last_output":         sess.LastOutput,
		"error":               sess.Error,
	}
	if sess.EndedAt != nil {
		detail["ended_at"] = sess.EndedAt
	}
	sess.Unlock()

	return jsonResult(detail), nil
}

func (s *Server) handleSessionResume(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	sessionID := getStringArg(req, "session_id")
	if sessionID == "" {
		return errResult("session_id required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.ProviderClaude
	}
	prompt := getStringArg(req, "prompt")
	sess, err := s.SessMgr.Resume(ctx, r.Path, provider, sessionID, prompt)
	if err != nil {
		return errResult(fmt.Sprintf("resume failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"session_id":   sess.ID,
		"resumed_from": sessionID,
		"repo":         sess.RepoName,
		"status":       sess.Status,
	}), nil
}

func (s *Server) handleSessionStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	if err := s.SessMgr.Stop(id); err != nil {
		return errResult(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped session %s", id)), nil
}

func (s *Server) handleSessionBudget(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	newBudget := getNumberArg(req, "budget", 0)
	if newBudget > 0 {
		sess.Lock()
		sess.BudgetUSD = newBudget
		sess.Unlock()
	}

	sess.Lock()
	info := map[string]any{
		"session_id": sess.ID,
		"budget_usd": sess.BudgetUSD,
		"spent_usd":  sess.SpentUSD,
		"remaining":  sess.BudgetUSD - sess.SpentUSD,
		"turns":      sess.TurnCount,
		"status":     sess.Status,
	}
	sess.Unlock()

	return jsonResult(info), nil
}

// Team handlers

func (s *Server) handleTeamCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	teamName := getStringArg(req, "name")
	if teamName == "" {
		return errResult("team name required"), nil
	}
	tasksStr := getStringArg(req, "tasks")
	if tasksStr == "" {
		return errResult("tasks required (newline-separated)"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
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
		return errResult(fmt.Sprintf("create team failed: %v", err)), nil
	}
	return jsonResult(team), nil
}

func (s *Server) handleTeamStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return errResult("team name required"), nil
	}

	team, ok := s.SessMgr.GetTeam(name)
	if !ok {
		return errResult(fmt.Sprintf("team not found: %s", name)), nil
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
		return errResult("team name required"), nil
	}
	task := getStringArg(req, "task")
	if task == "" {
		return errResult("task description required"), nil
	}

	taskProvider := session.Provider(getStringArg(req, "provider"))
	count, err := s.SessMgr.DelegateTask(name, session.TeamTask{
		Description: task,
		Provider:    taskProvider,
		Status:      "pending",
	})
	if err != nil {
		return errResult(err.Error()), nil
	}

	return textResult(fmt.Sprintf("Added task to team %s (%d total tasks)", name, count)), nil
}

// Agent handlers

func (s *Server) handleAgentDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	agentName := getStringArg(req, "name")
	if agentName == "" {
		return errResult("agent name required"), nil
	}
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
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
		return errResult(fmt.Sprintf("write agent: %v", err)), nil
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
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
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
			return errResult(fmt.Sprintf("list agents: %v", err)), nil
		}
	}

	if agents == nil {
		agents = []session.AgentDef{}
	}
	return jsonResult(agents), nil
}

// Alert thresholds for fleet status.
const (
	fleetStaleThreshold      = 1 * time.Hour
	fleetBudgetWarnThreshold = 0.90
	fleetNoProgressThreshold = 3
)

func (s *Server) handleFleetStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Auto-scan if needed
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	// Refresh all repos
	for _, r := range s.Repos {
		model.RefreshRepo(r)
	}

	// Gather sessions and teams
	allSessions := s.SessMgr.List("")
	allTeams := s.SessMgr.ListTeams()

	// --- Build per-repo summaries ---
	type repoSummary struct {
		Name            string  `json:"name"`
		Status          string  `json:"status"`
		Managed         bool    `json:"managed"`
		Paused          bool    `json:"paused"`
		LoopCount       int     `json:"loop_count"`
		Calls           string  `json:"calls"`
		Circuit         string  `json:"circuit"`
		SpendUSD        float64 `json:"spend_usd"`
		Model           string  `json:"model,omitempty"`
		LastUpdate      string  `json:"last_update"`
		SessionsRunning int     `json:"sessions_running"`
		SessionsTotal   int     `json:"sessions_total"`
		CompletedTasks  int     `json:"completed_tasks"`
		TotalTasks      int     `json:"total_tasks"`
	}

	repos := make([]repoSummary, 0, len(s.Repos))
	var totalLoopSpend float64
	var runningLoops, pausedLoops, openCircuits int

	for _, r := range s.Repos {
		managed := s.ProcMgr.IsRunning(r.Path)
		paused := s.ProcMgr.IsPaused(r.Path)

		loopCount := 0
		var spendUSD float64
		var mdl string
		if r.Status != nil {
			loopCount = r.Status.LoopCount
			spendUSD = r.Status.SessionSpendUSD
			mdl = r.Status.Model
		}
		totalLoopSpend += spendUSD

		if managed && !paused {
			runningLoops++
		}
		if paused {
			pausedLoops++
		}

		circuitStr := r.CircuitDisplay()
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			openCircuits++
		}

		// Count sessions for this repo
		var sessRunning, sessTotal int
		for _, sess := range allSessions {
			if sess.RepoPath == r.Path {
				sessTotal++
				sess.Lock()
				st := sess.Status
				sess.Unlock()
				if st == session.StatusRunning || st == session.StatusLaunching {
					sessRunning++
				}
			}
		}

		// Progress tasks
		var completedTasks, totalTasks int
		if r.Progress != nil {
			completedTasks = len(r.Progress.CompletedIDs)
			// Total tasks = completed + remaining iterations implied by log
			totalTasks = completedTasks
		}

		repos = append(repos, repoSummary{
			Name:            r.Name,
			Status:          r.StatusDisplay(),
			Managed:         managed,
			Paused:          paused,
			LoopCount:       loopCount,
			Calls:           r.CallsDisplay(),
			Circuit:         circuitStr,
			SpendUSD:        spendUSD,
			Model:           mdl,
			LastUpdate:      r.UpdatedDisplay(),
			SessionsRunning: sessRunning,
			SessionsTotal:   sessTotal,
			CompletedTasks:  completedTasks,
			TotalTasks:      totalTasks,
		})
	}

	// --- Build session summaries ---
	type sessionSummary struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		Model    string  `json:"model,omitempty"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
		Agent    string  `json:"agent,omitempty"`
		Team     string  `json:"team,omitempty"`
	}

	// Provider breakdown accumulators
	type providerStats struct {
		Sessions int     `json:"sessions"`
		Running  int     `json:"running"`
		SpendUSD float64 `json:"spend_usd"`
	}
	providerMap := map[string]*providerStats{}

	var totalSessionSpend float64
	var runningSessions int
	sessions := make([]sessionSummary, 0, len(allSessions))

	for _, sess := range allSessions {
		sess.Lock()
		status := string(sess.Status)
		provider := string(sess.Provider)
		spent := sess.SpentUSD
		turns := sess.TurnCount
		isRunning := sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching
		sess.Unlock()

		totalSessionSpend += spent
		if isRunning {
			runningSessions++
		}

		// Provider stats
		ps, ok := providerMap[provider]
		if !ok {
			ps = &providerStats{}
			providerMap[provider] = ps
		}
		ps.Sessions++
		ps.SpendUSD += spent
		if isRunning {
			ps.Running++
		}

		sessions = append(sessions, sessionSummary{
			ID:       sess.ID,
			Provider: provider,
			Repo:     sess.RepoName,
			Status:   status,
			Model:    sess.Model,
			SpentUSD: spent,
			Turns:    turns,
			Agent:    sess.AgentName,
			Team:     sess.TeamName,
		})
	}

	// --- Build team summaries ---
	type teamSummary struct {
		Name           string `json:"name"`
		Repo           string `json:"repo"`
		Status         string `json:"status"`
		TasksTotal     int    `json:"tasks_total"`
		TasksCompleted int    `json:"tasks_completed"`
		TasksPending   int    `json:"tasks_pending"`
	}

	teams := make([]teamSummary, 0, len(allTeams))
	for _, t := range allTeams {
		var completed, pending int
		for _, task := range t.Tasks {
			switch task.Status {
			case "completed":
				completed++
			case "pending":
				pending++
			}
		}
		teams = append(teams, teamSummary{
			Name:           t.Name,
			Repo:           filepath.Base(t.RepoPath),
			Status:         string(t.Status),
			TasksTotal:     len(t.Tasks),
			TasksCompleted: completed,
			TasksPending:   pending,
		})
	}

	// --- Generate alerts ---
	type alert struct {
		Severity string `json:"severity"`
		Repo     string `json:"repo"`
		Message  string `json:"message"`
	}

	var alerts []alert

	for _, r := range s.Repos {
		// Circuit breaker OPEN → critical
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			alerts = append(alerts, alert{
				Severity: "critical",
				Repo:     r.Name,
				Message:  fmt.Sprintf("Circuit breaker OPEN: %s", r.Circuit.Reason),
			})
		}

		// Loop stale → warning
		managed := s.ProcMgr.IsRunning(r.Path)
		if managed && r.Status != nil && !r.Status.Timestamp.IsZero() {
			if time.Since(r.Status.Timestamp) > fleetStaleThreshold {
				alerts = append(alerts, alert{
					Severity: "warning",
					Repo:     r.Name,
					Message:  fmt.Sprintf("Loop stale: last update %s", r.UpdatedDisplay()),
				})
			}
		}

		// Budget near limit → warning
		if r.Config != nil && r.Status != nil {
			if budgetStr, ok := r.Config.Values["RALPH_SESSION_BUDGET"]; ok {
				budget, err := strconv.ParseFloat(budgetStr, 64)
				if err == nil && budget > 0 {
					ratio := r.Status.SessionSpendUSD / budget
					if ratio >= fleetBudgetWarnThreshold {
						alerts = append(alerts, alert{
							Severity: "warning",
							Repo:     r.Name,
							Message:  fmt.Sprintf("Budget at %d%%: $%.2f/$%.2f", int(ratio*100), r.Status.SessionSpendUSD, budget),
						})
					}
				}
			}
		}

		// No-progress streak → warning
		if r.Circuit != nil && r.Circuit.State != "OPEN" && r.Circuit.ConsecutiveNoProgress >= fleetNoProgressThreshold {
			alerts = append(alerts, alert{
				Severity: "warning",
				Repo:     r.Name,
				Message:  fmt.Sprintf("No-progress streak: %d consecutive iterations", r.Circuit.ConsecutiveNoProgress),
			})
		}

		// Loop paused → info
		if s.ProcMgr.IsPaused(r.Path) {
			alerts = append(alerts, alert{
				Severity: "info",
				Repo:     r.Name,
				Message:  "Loop paused",
			})
		}
	}

	// Session errored → info
	for _, sess := range allSessions {
		sess.Lock()
		st := sess.Status
		errMsg := sess.Error
		sess.Unlock()
		if st == session.StatusErrored {
			msg := fmt.Sprintf("Session %s errored", sess.ID)
			if errMsg != "" {
				msg += ": " + errMsg
			}
			alerts = append(alerts, alert{
				Severity: "info",
				Repo:     sess.RepoName,
				Message:  msg,
			})
		}
	}

	if alerts == nil {
		alerts = []alert{}
	}

	// --- Assemble response ---
	totalSpend := totalLoopSpend + totalSessionSpend
	loopRuns := s.SessMgr.ListLoops()

	type loopSummary struct {
		ID             string `json:"id"`
		Repo           string `json:"repo"`
		Status         string `json:"status"`
		Iterations     int    `json:"iterations"`
		LastError      string `json:"last_error,omitempty"`
		PlannerModel   string `json:"planner_model"`
		WorkerModel    string `json:"worker_model"`
		WorktreePolicy string `json:"worktree_policy,omitempty"`
	}

	loops := make([]loopSummary, 0, len(loopRuns))
	runningLoopRuns := 0
	for _, run := range loopRuns {
		run.Lock()
		if run.Status == "running" {
			runningLoopRuns++
		}
		loops = append(loops, loopSummary{
			ID:             run.ID,
			Repo:           run.RepoName,
			Status:         run.Status,
			Iterations:     len(run.Iterations),
			LastError:      run.LastError,
			PlannerModel:   run.Profile.PlannerModel,
			WorkerModel:    run.Profile.WorkerModel,
			WorktreePolicy: run.Profile.WorktreePolicy,
		})
		run.Unlock()
	}

	result := map[string]any{
		"summary": map[string]any{
			"total_repos":             len(s.Repos),
			"running_loops":           runningLoops,
			"paused_loops":            pausedLoops,
			"total_sessions":          len(allSessions),
			"running_sessions":        runningSessions,
			"total_loop_runs":         len(loopRuns),
			"running_loop_runs":       runningLoopRuns,
			"total_loop_spend_usd":    totalLoopSpend,
			"total_session_spend_usd": totalSessionSpend,
			"total_spend_usd":         totalSpend,
			"open_circuits":           openCircuits,
			"providers":               providerMap,
		},
		"repos":    repos,
		"sessions": sessions,
		"teams":    teams,
		"loops":    loops,
		"alerts":   alerts,
	}

	return jsonResult(result), nil
}

// --- Event Bus & New Tool Handlers ---

func (s *Server) handleEventList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.EventBus == nil {
		return errResult("event bus not initialized"), nil
	}

	typeFilter := events.EventType(getStringArg(req, "type"))
	repoFilter := getStringArg(req, "repo")
	limit := int(getNumberArg(req, "limit", 50))
	sinceStr := getStringArg(req, "since")

	var evts []events.Event
	if sinceStr != "" {
		t, err := time.Parse(time.RFC3339, sinceStr)
		if err != nil {
			return errResult(fmt.Sprintf("invalid since timestamp: %v", err)), nil
		}
		evts = s.EventBus.HistorySince(t)
	} else {
		evts = s.EventBus.History(typeFilter, limit)
	}

	// Apply filters
	var filtered []events.Event
	for _, e := range evts {
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		if repoFilter != "" && e.RepoName != repoFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[len(filtered)-limit:]
	}

	type eventOut struct {
		Type      string         `json:"type"`
		Timestamp string         `json:"timestamp"`
		RepoName  string         `json:"repo_name,omitempty"`
		SessionID string         `json:"session_id,omitempty"`
		Provider  string         `json:"provider,omitempty"`
		Data      map[string]any `json:"data,omitempty"`
	}
	out := make([]eventOut, len(filtered))
	for i, e := range filtered {
		out[i] = eventOut{
			Type:      string(e.Type),
			Timestamp: e.Timestamp.Format(time.RFC3339),
			RepoName:  e.RepoName,
			SessionID: e.SessionID,
			Provider:  e.Provider,
			Data:      e.Data,
		}
	}
	return jsonResult(out), nil
}

func (s *Server) handleFleetAnalytics(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")

	sessions := s.SessMgr.List("")

	type providerStats struct {
		Sessions    int     `json:"sessions"`
		Running     int     `json:"running"`
		TotalSpend  float64 `json:"total_spend_usd"`
		AvgCostTurn float64 `json:"avg_cost_per_turn"`
		TotalTurns  int     `json:"total_turns"`
	}

	providers := make(map[string]*providerStats)
	repos := make(map[string]float64)

	for _, sess := range sessions {
		sess.Lock()
		provider := string(sess.Provider)
		repoName := sess.RepoName
		spent := sess.SpentUSD
		turns := sess.TurnCount
		status := sess.Status
		sess.Unlock()

		if repoFilter != "" && repoName != repoFilter {
			continue
		}
		if providerFilter != "" && provider != providerFilter {
			continue
		}

		ps, ok := providers[provider]
		if !ok {
			ps = &providerStats{}
			providers[provider] = ps
		}
		ps.Sessions++
		ps.TotalSpend += spent
		ps.TotalTurns += turns
		if status == session.StatusRunning || status == session.StatusLaunching {
			ps.Running++
		}
		repos[repoName] += spent
	}

	for _, ps := range providers {
		if ps.TotalTurns > 0 {
			ps.AvgCostTurn = ps.TotalSpend / float64(ps.TotalTurns)
		}
	}

	result := map[string]any{
		"providers":      providers,
		"repos":          repos,
		"total_sessions": len(sessions),
	}
	return jsonResult(result), nil
}

func (s *Server) handleSessionCompare(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id1 := getStringArg(req, "id1")
	id2 := getStringArg(req, "id2")
	if id1 == "" || id2 == "" {
		return errResult("both id1 and id2 are required"), nil
	}

	s1, ok1 := s.SessMgr.Get(id1)
	s2, ok2 := s.SessMgr.Get(id2)
	if !ok1 {
		return errResult(fmt.Sprintf("session not found: %s", id1)), nil
	}
	if !ok2 {
		return errResult(fmt.Sprintf("session not found: %s", id2)), nil
	}

	extract := func(sess *session.Session) map[string]any {
		sess.Lock()
		defer sess.Unlock()
		dur := time.Since(sess.LaunchedAt)
		if sess.EndedAt != nil {
			dur = sess.EndedAt.Sub(sess.LaunchedAt)
		}
		costPerTurn := 0.0
		if sess.TurnCount > 0 {
			costPerTurn = sess.SpentUSD / float64(sess.TurnCount)
		}
		turnsPerMin := 0.0
		if dur.Minutes() > 0 {
			turnsPerMin = float64(sess.TurnCount) / dur.Minutes()
		}
		return map[string]any{
			"id":            sess.ID,
			"provider":      string(sess.Provider),
			"status":        string(sess.Status),
			"model":         sess.Model,
			"spent_usd":     sess.SpentUSD,
			"turns":         sess.TurnCount,
			"duration":      dur.String(),
			"cost_per_turn": costPerTurn,
			"turns_per_min": turnsPerMin,
		}
	}

	return jsonResult(map[string]any{
		"session_1": extract(s1),
		"session_2": extract(s2),
	}), nil
}

func (s *Server) handleSessionOutput(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}
	lines := int(getNumberArg(req, "lines", 20))
	if lines > 100 {
		lines = 100
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	sess.Unlock()

	if len(history) > lines {
		history = history[len(history)-lines:]
	}

	return jsonResult(map[string]any{
		"session_id": id,
		"lines":      len(history),
		"output":     history,
	}), nil
}

func (s *Server) handleRepoHealth(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}
	model.RefreshRepo(r)

	score := 100
	var issues []string

	// Circuit breaker
	cbState := "CLOSED"
	if r.Circuit != nil {
		cbState = r.Circuit.State
		if cbState == "OPEN" {
			score -= 30
			issues = append(issues, fmt.Sprintf("circuit breaker OPEN: %s", r.Circuit.Reason))
		} else if cbState == "HALF_OPEN" {
			score -= 10
			issues = append(issues, "circuit breaker HALF_OPEN")
		}
	}

	// Staleness
	if r.Status != nil && !r.Status.Timestamp.IsZero() {
		age := time.Since(r.Status.Timestamp)
		if age > time.Hour {
			score -= 15
			issues = append(issues, fmt.Sprintf("status stale (%.0f min)", age.Minutes()))
		}
	}

	// Budget
	if r.Status != nil && r.Status.BudgetStatus == "exceeded" {
		score -= 20
		issues = append(issues, "budget exceeded")
	}

	// Active sessions
	activeSessions := 0
	erroredSessions := 0
	totalSpend := 0.0
	for _, sess := range s.SessMgr.List("") {
		sess.Lock()
		if sess.RepoName == name || filepath.Base(sess.RepoPath) == name {
			if sess.Status == session.StatusRunning {
				activeSessions++
			}
			if sess.Status == session.StatusErrored {
				erroredSessions++
				score -= 5
			}
			totalSpend += sess.SpentUSD
		}
		sess.Unlock()
	}

	if erroredSessions > 0 {
		issues = append(issues, fmt.Sprintf("%d errored sessions", erroredSessions))
	}

	// CLAUDE.md health
	claudeMDPath := filepath.Join(r.Path, "CLAUDE.md")
	var claudeMDFindings []enhancer.ClaudeMDResult
	if claudeResults, err := enhancer.CheckClaudeMD(claudeMDPath); err == nil {
		claudeMDFindings = claudeResults
		warningCount := 0
		for _, finding := range claudeResults {
			if finding.Severity == "warn" {
				warningCount++
			}
		}
		if warningCount > 3 {
			score -= 10
			issues = append(issues, fmt.Sprintf("CLAUDE.md: %d warnings", warningCount))
		}
	}

	if score < 0 {
		score = 0
	}

	return jsonResult(map[string]any{
		"repo":              name,
		"health_score":      score,
		"circuit_breaker":   cbState,
		"active_sessions":   activeSessions,
		"errored_sessions":  erroredSessions,
		"total_spend_usd":   totalSpend,
		"loop_running":      s.ProcMgr.IsRunning(r.Path),
		"issues":            issues,
		"claudemd_findings": claudeMDFindings,
	}), nil
}

func (s *Server) handleSessionRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	opts := session.LaunchOptions{
		Provider:     sess.Provider,
		RepoPath:     sess.RepoPath,
		Prompt:       sess.Prompt,
		Model:        sess.Model,
		MaxBudgetUSD: sess.BudgetUSD,
		MaxTurns:     sess.MaxTurns,
		Agent:        sess.AgentName,
		TeamName:     sess.TeamName,
	}
	sess.Unlock()

	// Apply overrides
	if m := getStringArg(req, "model"); m != "" {
		opts.Model = m
	}
	if b := getNumberArg(req, "max_budget_usd", 0); b > 0 {
		opts.MaxBudgetUSD = b
	}

	newSess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		return errResult(fmt.Sprintf("retry failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"original_id": id,
		"new_id":      newSess.ID,
		"provider":    string(newSess.Provider),
		"status":      "launched",
	}), nil
}

func (s *Server) handleConfigBulk(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	key := getStringArg(req, "key")
	if key == "" {
		return errResult("key required"), nil
	}
	value := getStringArg(req, "value")
	reposStr := getStringArg(req, "repos")

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}

	allRepos := s.reposCopy()
	var targetNames map[string]bool
	if reposStr != "" {
		targetNames = make(map[string]bool)
		for _, name := range strings.Split(reposStr, ",") {
			targetNames[strings.TrimSpace(name)] = true
		}
	}

	results := make(map[string]any)
	for _, r := range allRepos {
		if targetNames != nil && !targetNames[r.Name] {
			continue
		}
		model.RefreshRepo(r)
		if r.Config == nil {
			results[r.Name] = "no .ralphrc"
			continue
		}
		if value == "" {
			results[r.Name] = r.Config.Values[key]
		} else {
			r.Config.Values[key] = value
			if err := r.Config.Save(); err != nil {
				results[r.Name] = fmt.Sprintf("save error: %v", err)
			} else {
				results[r.Name] = "updated"
				if s.EventBus != nil {
					s.EventBus.Publish(events.Event{
						Type:     events.ConfigChanged,
						RepoPath: r.Path,
						RepoName: r.Name,
						Data:     map[string]any{"key": key, "value": value},
					})
				}
			}
		}
	}
	return jsonResult(results), nil
}

func (s *Server) handleWorkflowDefine(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	yamlStr := getStringArg(req, "yaml")
	if repoName == "" || name == "" || yamlStr == "" {
		return errResult("repo, name, and yaml are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.ParseWorkflow(name, []byte(yamlStr))
	if err != nil {
		return errResult(fmt.Sprintf("invalid workflow YAML: %v", err)), nil
	}

	if err := session.SaveWorkflow(r.Path, wf); err != nil {
		return errResult(fmt.Sprintf("save failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":  wf.Name,
		"steps": len(wf.Steps),
		"saved": true,
	}), nil
}

func (s *Server) handleWorkflowRun(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	name := getStringArg(req, "name")
	if repoName == "" || name == "" {
		return errResult("repo and name are required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	wf, err := session.LoadWorkflow(r.Path, name)
	if err != nil {
		return errResult(fmt.Sprintf("load workflow: %v", err)), nil
	}

	run, err := s.SessMgr.RunWorkflow(ctx, r.Path, *wf)
	if err != nil {
		return errResult(fmt.Sprintf("run workflow: %v", err)), nil
	}

	run.Lock()
	result := map[string]any{
		"run_id":     run.ID,
		"workflow":   run.Name,
		"repo_path":  run.RepoPath,
		"status":     run.Status,
		"created_at": run.CreatedAt,
		"updated_at": run.UpdatedAt,
		"steps":      append([]session.WorkflowStepResult(nil), run.Steps...),
	}
	run.Unlock()

	return jsonResult(result), nil
}

func (s *Server) handleLoopStart(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
	}

	profile := session.DefaultLoopProfile()
	if value := getStringArg(req, "planner_model"); value != "" {
		profile.PlannerModel = value
	}
	if value := getStringArg(req, "worker_model"); value != "" {
		profile.WorkerModel = value
	}
	if value := getStringArg(req, "verifier_model"); value != "" {
		profile.VerifierModel = value
	}
	if value := getStringArg(req, "worktree_policy"); value != "" {
		profile.WorktreePolicy = value
	}
	if value := int(getNumberArg(req, "retry_limit", float64(profile.RetryLimit))); value != profile.RetryLimit {
		profile.RetryLimit = value
	}
	if value := int(getNumberArg(req, "max_concurrent_workers", float64(profile.MaxConcurrentWorkers))); value != profile.MaxConcurrentWorkers {
		profile.MaxConcurrentWorkers = value
	}
	if commands := splitLines(getStringArg(req, "verify_commands")); len(commands) > 0 {
		profile.VerifyCommands = commands
	}
	if pp := getStringArg(req, "planner_provider"); pp != "" {
		profile.PlannerProvider = session.Provider(pp)
	}
	if wp := getStringArg(req, "worker_provider"); wp != "" {
		profile.WorkerProvider = session.Provider(wp)
	}
	if vp := getStringArg(req, "verifier_provider"); vp != "" {
		profile.VerifierProvider = session.Provider(vp)
	}

	// Wire prompt enhancer into session manager for loop integration
	if s.SessMgr.Enhancer == nil {
		s.SessMgr.Enhancer = s.getEngine()
	}

	run, err := s.SessMgr.StartLoop(ctx, r.Path, profile)
	if err != nil {
		return errResult(fmt.Sprintf("start loop: %v", err)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return errResult(fmt.Sprintf("loop not found: %s", id)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStep(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	if err := s.SessMgr.StepLoop(ctx, id); err != nil {
		return errResult(fmt.Sprintf("step loop: %v", err)), nil
	}

	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return errResult(fmt.Sprintf("loop not found after step: %s", id)), nil
	}
	return jsonResult(loopResult(run)), nil
}

func (s *Server) handleLoopStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("loop id required"), nil
	}

	if err := s.SessMgr.StopLoop(id); err != nil {
		return errResult(fmt.Sprintf("stop loop: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped loop %s", id)), nil
}

func loopResult(run *session.LoopRun) map[string]any {
	run.Lock()
	defer run.Unlock()

	iterations := append([]session.LoopIteration(nil), run.Iterations...)
	return map[string]any{
		"id":         run.ID,
		"repo":       run.RepoName,
		"repo_path":  run.RepoPath,
		"status":     run.Status,
		"last_error": run.LastError,
		"profile":    run.Profile,
		"iterations": iterations,
		"created_at": run.CreatedAt,
		"updated_at": run.UpdatedAt,
	}
}

func (s *Server) handleSnapshot(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := getStringArg(req, "action")
	if action == "" {
		action = "save"
	}
	name := getStringArg(req, "name")

	if action == "list" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return errResult(fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		allRepos := s.reposCopy()
		var snapshots []string
		for _, r := range allRepos {
			snaps, _ := filepath.Glob(filepath.Join(r.Path, ".ralph", "snapshots", "*.json"))
			for _, snap := range snaps {
				snapshots = append(snapshots, filepath.Base(snap))
			}
		}
		return jsonResult(map[string]any{"snapshots": snapshots}), nil
	}

	// Save snapshot
	if name == "" {
		name = fmt.Sprintf("snapshot-%s", time.Now().Format("20060102-150405"))
	}

	allSessions := s.SessMgr.List("")
	type sessionSnap struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
	}
	var sessSnaps []sessionSnap
	for _, sess := range allSessions {
		sess.Lock()
		sessSnaps = append(sessSnaps, sessionSnap{
			ID:       sess.ID,
			Provider: string(sess.Provider),
			Repo:     sess.RepoName,
			Status:   string(sess.Status),
			SpentUSD: sess.SpentUSD,
			Turns:    sess.TurnCount,
		})
		sess.Unlock()
	}

	teams := s.SessMgr.ListTeams()
	snapshot := map[string]any{
		"name":      name,
		"timestamp": time.Now().Format(time.RFC3339),
		"sessions":  sessSnaps,
		"teams":     teams,
	}

	data, _ := json.MarshalIndent(snapshot, "", "  ")

	// Save to first repo's .ralph/snapshots/
	if s.reposNil() {
		_ = s.scan()
	}
	allRepos := s.reposCopy()
	if len(allRepos) > 0 {
		dir := filepath.Join(allRepos[0].Path, ".ralph", "snapshots")
		_ = os.MkdirAll(dir, 0o755)
		_ = os.WriteFile(filepath.Join(dir, name+".json"), data, 0o644)
	}

	return jsonResult(snapshot), nil
}

func (s *Server) handleAgentCompose(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo name required"), nil
	}
	if err := ValidateRepoName(repoName); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	name := getStringArg(req, "name")
	if name == "" {
		return errResult("composite agent name required"), nil
	}
	agentsStr := getStringArg(req, "agents")
	if agentsStr == "" {
		return errResult("agents list required (comma-separated)"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", repoName)), nil
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
		return errResult(fmt.Sprintf("compose agents: %v", err)), nil
	}

	// Apply model override
	if m := getStringArg(req, "model"); m != "" {
		composite.Model = m
	}

	if err := session.WriteAgent(r.Path, composite); err != nil {
		return errResult(fmt.Sprintf("write composite agent: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"name":     composite.Name,
		"provider": string(composite.Provider),
		"composed": agentNames,
		"tools":    composite.Tools,
		"model":    composite.Model,
	}), nil
}

func (s *Server) handleSessionStopAll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	// Count running sessions before stopping
	sessions := s.SessMgr.List("")
	running := 0
	for _, sess := range sessions {
		sess.Lock()
		if sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching {
			running++
		}
		sess.Unlock()
	}

	s.SessMgr.StopAll()

	return textResult(fmt.Sprintf("Stopped %d running session(s)", running)), nil
}

// --- Journal handlers ---

func (s *Server) handleJournalRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	limit := int(getNumberArg(req, "limit", 10))
	entries, err := session.ReadRecentJournal(r.Path, limit)
	if err != nil {
		return errResult(fmt.Sprintf("read journal: %v", err)), nil
	}

	synthesis := session.SynthesizeContext(entries)

	return jsonResult(map[string]any{
		"entries":   entries,
		"count":     len(entries),
		"synthesis": synthesis,
	}), nil
}

func (s *Server) handleJournalWrite(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	entry := session.JournalEntry{
		Timestamp: time.Now(),
		SessionID: getStringArg(req, "session_id"),
		RepoName:  r.Name,
	}
	if w := getStringArg(req, "worked"); w != "" {
		entry.Worked = splitCSV(w)
	}
	if f := getStringArg(req, "failed"); f != "" {
		entry.Failed = splitCSV(f)
	}
	if sg := getStringArg(req, "suggest"); sg != "" {
		entry.Suggest = splitCSV(sg)
	}

	if err := session.WriteJournalEntryManual(r.Path, entry); err != nil {
		return errResult(fmt.Sprintf("write journal: %v", err)), nil
	}

	if s.EventBus != nil {
		s.EventBus.Publish(events.Event{
			Type:      events.JournalWritten,
			RepoName:  r.Name,
			RepoPath:  r.Path,
			SessionID: entry.SessionID,
		})
	}

	return jsonResult(map[string]any{
		"status":  "written",
		"repo":    r.Name,
		"worked":  len(entry.Worked),
		"failed":  len(entry.Failed),
		"suggest": len(entry.Suggest),
	}), nil
}

func (s *Server) handleJournalPrune(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	keep := int(getNumberArg(req, "keep", 100))
	dryRun := getStringArg(req, "dry_run") != "false"

	// Read current count
	entries, err := session.ReadRecentJournal(r.Path, 100000)
	if err != nil {
		return errResult(fmt.Sprintf("read journal: %v", err)), nil
	}

	wouldPrune := len(entries) - keep
	if wouldPrune < 0 {
		wouldPrune = 0
	}

	if dryRun {
		return jsonResult(map[string]any{
			"dry_run":     true,
			"total":       len(entries),
			"keep":        keep,
			"would_prune": wouldPrune,
		}), nil
	}

	pruned, err := session.PruneJournal(r.Path, keep)
	if err != nil {
		return errResult(fmt.Sprintf("prune journal: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"dry_run":   false,
		"pruned":    pruned,
		"remaining": len(entries) - pruned,
	}), nil
}

func splitCSV(s string) []string {
	var result []string
	for _, item := range strings.Split(s, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

// Awesome-list handlers

func (s *Server) handleAwesomeFetch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return errResult(fmt.Sprintf("fetch: %v", err)), nil
	}
	return jsonResult(idx), nil
}

func (s *Server) handleAwesomeAnalyze(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repo := getStringArg(req, "repo")
	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return errResult(fmt.Sprintf("fetch: %v", err)), nil
	}

	maxWorkers := int(getNumberArg(req, "max_workers", 5))
	analysis, err := awesome.Analyze(ctx, s.HTTPClient, idx.Entries, awesome.AnalyzeOptions{
		MaxWorkers: maxWorkers,
	})
	if err != nil {
		return errResult(fmt.Sprintf("analyze: %v", err)), nil
	}
	analysis.Source = idx.Source
	return jsonResult(analysis), nil
}

func (s *Server) handleAwesomeDiff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return errResult("save_to required"), nil
	}
	repo := getStringArg(req, "repo")

	idx, err := awesome.Fetch(ctx, s.HTTPClient, repo)
	if err != nil {
		return errResult(fmt.Sprintf("fetch: %v", err)), nil
	}

	prev, _ := awesome.LoadIndex(saveTo)
	diff := awesome.Diff(prev, idx)
	return jsonResult(diff), nil
}

func (s *Server) handleAwesomeReport(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return errResult("save_to required"), nil
	}

	analysis, err := awesome.LoadAnalysis(saveTo)
	if err != nil {
		return errResult(fmt.Sprintf("load analysis: %v", err)), nil
	}

	report := awesome.GenerateReport(analysis)
	format := getStringArg(req, "format")
	if format == "json" {
		return jsonResult(report), nil
	}
	return textResult(awesome.FormatMarkdown(report)), nil
}

func (s *Server) handleAwesomeSync(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	saveTo := getStringArg(req, "save_to")
	if saveTo == "" {
		return errResult("save_to required"), nil
	}

	opts := awesome.SyncOptions{
		Repo:       getStringArg(req, "repo"),
		SaveTo:     saveTo,
		FullRescan: getStringArg(req, "full_rescan") == "true",
		MaxWorkers: int(getNumberArg(req, "max_workers", 5)),
	}

	result, err := awesome.Sync(ctx, s.HTTPClient, opts)
	if err != nil {
		return errResult(fmt.Sprintf("sync: %v", err)), nil
	}
	return jsonResult(result), nil
}

// --- Marathon monitoring handlers ---

func (s *Server) handleSessionTail(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}
	lines := int(getNumberArg(req, "lines", 30))
	if lines > 100 {
		lines = 100
	}
	if lines < 1 {
		lines = 30
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	totalCount := sess.TotalOutputCount
	status := sess.Status
	lastActivity := sess.LastActivity
	sess.Unlock()

	cursorStr := getStringArg(req, "cursor")
	var output []string

	if cursorStr != "" {
		cursor, err := strconv.Atoi(cursorStr)
		if err != nil {
			return errResult(fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
		// cursor is the TotalOutputCount at the time of last call.
		// New lines since cursor = totalCount - cursor.
		newLines := totalCount - cursor
		if newLines <= 0 {
			output = nil
		} else {
			startIdx := len(history) - newLines
			if startIdx < 0 {
				startIdx = 0
			}
			output = history[startIdx:]
			if len(output) > lines {
				output = output[len(output)-lines:]
			}
		}
	} else {
		// No cursor: return last N lines
		if len(history) > lines {
			output = history[len(history)-lines:]
		} else {
			output = history
		}
	}

	idleSeconds := time.Since(lastActivity).Seconds()

	return jsonResult(map[string]any{
		"session_id":     id,
		"status":         string(status),
		"output":         output,
		"lines_returned": len(output),
		"next_cursor":    strconv.Itoa(totalCount),
		"is_active":      status == session.StatusRunning || status == session.StatusLaunching,
		"idle_seconds":   int(idleSeconds),
	}), nil
}

func (s *Server) handleSessionDiff(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	repoPath := sess.RepoPath
	repoName := sess.RepoName
	launchedAt := sess.LaunchedAt
	endedAt := sess.EndedAt
	sess.Unlock()

	until := time.Now()
	if endedAt != nil {
		until = *endedAt
	}

	statOnly := getStringArg(req, "stat_only") != "false"
	maxLines := int(getNumberArg(req, "max_lines", 200))

	commits, err := session.GitLogSince(repoPath, launchedAt, until)
	if err != nil {
		return errResult(fmt.Sprintf("git log: %v", err)), nil
	}

	diffText, stat, truncated, err := session.GitDiffWindow(repoPath, launchedAt, until, statOnly, maxLines)
	if err != nil {
		return errResult(fmt.Sprintf("git diff: %v", err)), nil
	}

	duration := until.Sub(launchedAt).Round(time.Second).String()

	result := map[string]any{
		"session_id": id,
		"repo":       repoName,
		"window": map[string]any{
			"started":  launchedAt.Format(time.RFC3339),
			"ended":    until.Format(time.RFC3339),
			"duration": duration,
		},
		"commits":   commits,
		"stat":      stat,
		"truncated": truncated,
	}
	if diffText != "" {
		result["diff"] = diffText
	}

	return jsonResult(result), nil
}

func (s *Server) handleMarathonDashboard(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	staleMin := getNumberArg(req, "stale_threshold_min", 5)
	staleThreshold := time.Duration(staleMin) * time.Minute

	allSessions := s.SessMgr.List("")
	allTeams := s.SessMgr.ListTeams()

	var (
		totalUSD     float64
		runningCount int
		staleCount   int
		erroredCount int
		staleList    []map[string]any
		alerts       []map[string]any
		byProvider   = make(map[string]float64)
	)

	now := time.Now()

	for _, sess := range allSessions {
		sess.Lock()
		totalUSD += sess.SpentUSD
		byProvider[string(sess.Provider)] += sess.SpentUSD

		isRunning := sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching
		if isRunning {
			runningCount++
			idle := now.Sub(sess.LastActivity)
			if idle > staleThreshold {
				staleCount++
				staleList = append(staleList, map[string]any{
					"id":           sess.ID,
					"repo":         sess.RepoName,
					"idle_minutes": int(idle.Minutes()),
				})
				alerts = append(alerts, map[string]any{
					"severity": "warning",
					"type":     "stale_session",
					"message":  fmt.Sprintf("Session %s idle %.0f min", sess.ID[:min(8, len(sess.ID))], idle.Minutes()),
				})
			}
		}

		if sess.Status == session.StatusErrored {
			erroredCount++
			alerts = append(alerts, map[string]any{
				"severity": "critical",
				"type":     "session_error",
				"message":  fmt.Sprintf("Session %s errored: %s", sess.ID[:min(8, len(sess.ID))], truncateForAlert(sess.Error, 80)),
			})
		}

		if sess.BudgetUSD > 0 && sess.SpentUSD/sess.BudgetUSD >= 0.80 {
			alerts = append(alerts, map[string]any{
				"severity": "warning",
				"type":     "budget_warning",
				"message":  fmt.Sprintf("Session %s at %.0f%% budget ($%.2f/$%.2f)", sess.ID[:min(8, len(sess.ID))], sess.SpentUSD/sess.BudgetUSD*100, sess.SpentUSD, sess.BudgetUSD),
			})
		}
		sess.Unlock()
	}

	// Burn rate: total spend / total hours of running sessions
	var burnRate float64
	var hoursEst float64
	var totalBudget float64
	for _, sess := range allSessions {
		sess.Lock()
		if sess.Status == session.StatusRunning {
			elapsed := now.Sub(sess.LaunchedAt).Hours()
			if elapsed > 0 && sess.SpentUSD > 0 {
				burnRate += sess.SpentUSD / elapsed
			}
		}
		totalBudget += sess.BudgetUSD
		sess.Unlock()
	}
	remaining := totalBudget - totalUSD
	if remaining < 0 {
		remaining = 0
	}
	if burnRate > 0 {
		hoursEst = remaining / burnRate
	}

	// Team summaries
	var teamSummaries []map[string]any
	var tasksCompleted, tasksTotal int
	for _, team := range allTeams {
		completed := 0
		for _, t := range team.Tasks {
			tasksTotal++
			if t.Status == "completed" {
				completed++
				tasksCompleted++
			}
		}
		teamSummaries = append(teamSummaries, map[string]any{
			"name":      team.Name,
			"status":    string(team.Status),
			"tasks":     len(team.Tasks),
			"completed": completed,
		})
	}

	return jsonResult(map[string]any{
		"timestamp": now.Format(time.RFC3339),
		"cost": map[string]any{
			"total_usd":   totalUSD,
			"burn_rate":   burnRate,
			"remaining":   remaining,
			"hours_est":   hoursEst,
			"by_provider": byProvider,
		},
		"sessions": map[string]any{
			"total":      len(allSessions),
			"running":    runningCount,
			"stale":      staleCount,
			"errored":    erroredCount,
			"stale_list": staleList,
		},
		"teams": map[string]any{
			"summary":         teamSummaries,
			"tasks_completed": tasksCompleted,
			"tasks_total":     tasksTotal,
		},
		"alerts": alerts,
	}), nil
}

func (s *Server) handleSessionErrors(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	severityFilter := getStringArg(req, "severity")
	limit := int(getNumberArg(req, "limit", 50))
	if limit < 1 {
		limit = 50
	}

	allSessions := s.SessMgr.List("")

	type errorEntry struct {
		SessionID string `json:"session_id"`
		Repo      string `json:"repo"`
		Provider  string `json:"provider"`
		Severity  string `json:"severity"`
		Type      string `json:"type"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}

	var errors []errorEntry
	byType := make(map[string]int)
	bySeverity := make(map[string]int)
	healthySessions := 0
	sessionsWithErrors := 0

	for _, sess := range allSessions {
		sess.Lock()
		repo := sess.RepoName
		provider := string(sess.Provider)
		hasError := false

		if repoFilter != "" && repo != repoFilter {
			sess.Unlock()
			continue
		}

		ts := sess.LastActivity.Format(time.RFC3339)

		// Critical: errored sessions
		if sess.Error != "" || sess.Status == session.StatusErrored {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "critical",
				Type:      "session_error",
				Message:   truncateForAlert(firstNonEmptyStr(sess.Error, sess.ExitReason, "unknown error"), 200),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["session_error"]++
			bySeverity["critical"]++
		}

		// Warning: stream parse errors
		if sess.StreamParseErrors > 0 {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "warning",
				Type:      "stream_parse",
				Message:   fmt.Sprintf("%d parse errors", sess.StreamParseErrors),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["stream_parse"]++
			bySeverity["warning"]++
		}

		// Warning: budget warning
		if sess.BudgetUSD > 0 && sess.SpentUSD/sess.BudgetUSD >= 0.80 {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "warning",
				Type:      "budget_warning",
				Message:   fmt.Sprintf("%.0f%% of budget used ($%.2f/$%.2f)", sess.SpentUSD/sess.BudgetUSD*100, sess.SpentUSD, sess.BudgetUSD),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["budget_warning"]++
			bySeverity["warning"]++
		}

		// Info: stopped with reason
		if sess.Status == session.StatusStopped && sess.ExitReason != "" {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "info",
				Type:      "session_stopped",
				Message:   truncateForAlert(sess.ExitReason, 200),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["session_stopped"]++
			bySeverity["info"]++
		}

		if hasError {
			sessionsWithErrors++
		} else {
			healthySessions++
		}
		sess.Unlock()
	}

	// Filter by severity
	if severityFilter != "" {
		var filtered []errorEntry
		for _, e := range errors {
			if e.Severity == severityFilter {
				filtered = append(filtered, e)
			}
		}
		errors = filtered
	}

	// Sort: critical first, then warning, then info
	severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
	for i := 0; i < len(errors); i++ {
		for j := i + 1; j < len(errors); j++ {
			if severityOrder[errors[i].Severity] > severityOrder[errors[j].Severity] {
				errors[i], errors[j] = errors[j], errors[i]
			}
		}
	}

	// Cap at limit
	if len(errors) > limit {
		errors = errors[:limit]
	}

	return jsonResult(map[string]any{
		"total_errors":         len(errors),
		"by_type":              byType,
		"by_severity":          bySeverity,
		"errors":               errors,
		"sessions_with_errors": sessionsWithErrors,
		"healthy_sessions":     healthySessions,
	}), nil
}

func truncateForAlert(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// --- Remote Control (RC) helpers ---

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func formatCost(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}

// resolveTarget finds a session by ID or by repo name (most recent running session).
func (s *Server) resolveTarget(target string) (*session.Session, error) {
	if target == "" {
		return nil, fmt.Errorf("target required")
	}
	// Try as session ID first
	if sess, ok := s.SessMgr.Get(target); ok {
		return sess, nil
	}
	// Try as repo name
	sessions := s.SessMgr.FindByRepo(target)
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no session found for %q", target)
	}
	// Prefer running session, otherwise most recent
	var best *session.Session
	for _, sess := range sessions {
		sess.Lock()
		st := sess.Status
		la := sess.LastActivity
		sess.Unlock()
		if st == session.StatusRunning || st == session.StatusLaunching {
			if best == nil {
				best = sess
			} else {
				best.Lock()
				bestLA := best.LastActivity
				best.Unlock()
				if la.After(bestLA) {
					best = sess
				}
			}
		}
	}
	if best == nil {
		// No running session, use most recent
		best = sessions[0]
		for _, sess := range sessions[1:] {
			sess.Lock()
			la := sess.LastActivity
			sess.Unlock()
			best.Lock()
			bestLA := best.LastActivity
			best.Unlock()
			if la.After(bestLA) {
				best = sess
			}
		}
	}
	return best, nil
}

// mostActiveSession returns the most recently active session (prefers running).
func (s *Server) mostActiveSession() *session.Session {
	all := s.SessMgr.List("")
	if len(all) == 0 {
		return nil
	}
	var best *session.Session
	var bestRunning bool
	var bestTime time.Time

	for _, sess := range all {
		sess.Lock()
		st := sess.Status
		la := sess.LastActivity
		sess.Unlock()

		isRunning := st == session.StatusRunning || st == session.StatusLaunching
		if best == nil ||
			(isRunning && !bestRunning) ||
			(isRunning == bestRunning && la.After(bestTime)) {
			best = sess
			bestRunning = isRunning
			bestTime = la
		}
	}
	return best
}

func summarizeEvent(e events.Event) string {
	switch e.Type {
	case events.SessionStarted:
		return fmt.Sprintf("[start] %s/%s session %s", e.RepoName, e.Provider, shortID(e.SessionID))
	case events.SessionEnded:
		return fmt.Sprintf("[end] %s session %s", e.RepoName, shortID(e.SessionID))
	case events.SessionStopped:
		return fmt.Sprintf("[stop] %s session %s", e.RepoName, shortID(e.SessionID))
	case events.CostUpdate:
		cost := ""
		if v, ok := e.Data["cost_usd"]; ok {
			if f, ok := v.(float64); ok {
				cost = formatCost(f)
			}
		}
		return fmt.Sprintf("[cost] %s %s", e.RepoName, cost)
	case events.BudgetExceeded:
		return fmt.Sprintf("[BUDGET] %s exceeded budget", e.RepoName)
	case events.LoopStarted:
		return fmt.Sprintf("[loop] %s started", e.RepoName)
	case events.LoopStopped:
		return fmt.Sprintf("[loop] %s stopped", e.RepoName)
	case events.TeamCreated:
		return fmt.Sprintf("[team] %s created", e.RepoName)
	default:
		return fmt.Sprintf("[%s] %s", e.Type, e.RepoName)
	}
}

// --- Remote Control (RC) handlers ---

func (s *Server) handleRCStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all := s.SessMgr.List("")

	var running, recent []*session.Session
	var totalCost float64
	var alerts []string
	now := time.Now()

	for _, sess := range all {
		sess.Lock()
		st := sess.Status
		cost := sess.SpentUSD
		la := sess.LastActivity
		repoName := sess.RepoName
		idleMin := now.Sub(la).Minutes()
		sess.Unlock()

		totalCost += cost

		if st == session.StatusRunning || st == session.StatusLaunching {
			running = append(running, sess)
			if idleMin > 8 {
				alerts = append(alerts, fmt.Sprintf("Session %s on %s idle %s", shortID(sess.ID), repoName, formatDuration(now.Sub(la))))
			}
		} else if now.Sub(la) < 30*time.Minute {
			recent = append(recent, sess)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d running | %s total", len(running), formatCost(totalCost))
	if len(alerts) > 0 {
		fmt.Fprintf(&b, " | %d alert(s)", len(alerts))
	}
	b.WriteString("\n")

	for _, sess := range running {
		sess.Lock()
		repoName := sess.RepoName
		provider := sess.Provider
		cost := sess.SpentUSD
		turns := sess.TurnCount
		idle := now.Sub(sess.LastActivity)
		sess.Unlock()
		fmt.Fprintf(&b, "\n[running] %s/%s  %s  %dt  %s idle",
			repoName, provider, formatCost(cost), turns, formatDuration(idle))
	}

	for _, sess := range recent {
		sess.Lock()
		repoName := sess.RepoName
		provider := sess.Provider
		cost := sess.SpentUSD
		turns := sess.TurnCount
		st := sess.Status
		sess.Unlock()
		fmt.Fprintf(&b, "\n[%s] %s/%s  %s  %dt", st, repoName, provider, formatCost(cost), turns)
	}

	if len(running) == 0 && len(recent) == 0 {
		b.WriteString("\nNo active or recent sessions.")
	}

	if len(alerts) > 0 {
		b.WriteString("\n\nAlerts:")
		for _, a := range alerts {
			fmt.Fprintf(&b, "\n  %s", a)
		}
	}

	return textResult(b.String()), nil
}

func (s *Server) handleRCSend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return invalidParams("repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return invalidParams(fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	prompt := SanitizeString(getStringArg(req, "prompt"))
	if prompt == "" {
		return invalidParams("prompt required"), nil
	}

	if s.reposNil() {
		if err := s.scan(); err != nil {
			return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return notFound(fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.ProviderClaude
	}
	if err := session.ValidateProvider(provider); err != nil {
		return invalidParams(fmt.Sprintf("invalid provider: %v", err)), nil
	}

	// Check for resume
	if getStringArg(req, "resume") == "true" {
		existing := s.SessMgr.FindByRepo(name)
		for _, sess := range existing {
			sess.Lock()
			psid := sess.ProviderSessionID
			sess.Unlock()
			if psid != "" {
				resumed, err := s.SessMgr.Resume(ctx, r.Path, provider, psid, prompt)
				if err != nil {
					return internalErr(fmt.Sprintf("resume failed: %v", err)), nil
				}
				return textResult(fmt.Sprintf("Resumed %s session on %s (id: %s)", provider, name, shortID(resumed.ID))), nil
			}
		}
		// No resumable session found, fall through to fresh launch
	}

	// Stop existing running sessions on this repo
	existing := s.SessMgr.FindByRepo(name)
	for _, sess := range existing {
		sess.Lock()
		st := sess.Status
		sid := sess.ID
		sess.Unlock()
		if st == session.StatusRunning || st == session.StatusLaunching {
			_ = s.SessMgr.Stop(sid)
		}
	}

	budget := getNumberArg(req, "budget", 5.0)
	opts := session.LaunchOptions{
		Provider:     provider,
		RepoPath:     r.Path,
		Prompt:       prompt,
		Model:        getStringArg(req, "model"),
		MaxBudgetUSD: budget,
	}

	// Inject journal context
	journal, _ := session.ReadRecentJournal(r.Path, 5)
	if len(journal) > 0 {
		journalCtx := session.SynthesizeContext(journal)
		if journalCtx != "" {
			opts.Prompt = journalCtx + "\n\n---\n\n" + opts.Prompt
		}
	}

	sess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		return internalErr(fmt.Sprintf("launch failed: %v", err)), nil
	}

	return textResult(fmt.Sprintf("Launched %s session on %s (budget: %s, id: %s)",
		provider, name, formatCost(budget), shortID(sess.ID))), nil
}

func (s *Server) handleRCRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	var sess *session.Session

	if id != "" {
		var ok bool
		sess, ok = s.SessMgr.Get(id)
		if !ok {
			return notFound(fmt.Sprintf("session not found: %s", id)), nil
		}
	} else {
		sess = s.mostActiveSession()
		if sess == nil {
			return textResult("No sessions."), nil
		}
	}

	lines := int(getNumberArg(req, "lines", 10))
	if lines > 30 {
		lines = 30
	}
	if lines < 1 {
		lines = 10
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	totalCount := sess.TotalOutputCount
	status := sess.Status
	repoName := sess.RepoName
	provider := sess.Provider
	cost := sess.SpentUSD
	turns := sess.TurnCount
	lastActivity := sess.LastActivity
	sess.Unlock()

	cursorStr := getStringArg(req, "cursor")
	var output []string

	if cursorStr != "" {
		cursor, err := strconv.Atoi(cursorStr)
		if err != nil {
			return invalidParams(fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
		newLines := totalCount - cursor
		if newLines <= 0 {
			output = nil
		} else {
			startIdx := len(history) - newLines
			if startIdx < 0 {
				startIdx = 0
			}
			output = history[startIdx:]
			if len(output) > lines {
				output = output[len(output)-lines:]
			}
		}
	} else {
		if len(history) > lines {
			output = history[len(history)-lines:]
		} else {
			output = history
		}
	}

	idle := time.Since(lastActivity)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s/%s | %s | %dt | %s idle\n",
		status, repoName, provider, formatCost(cost), turns, formatDuration(idle))

	if len(output) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(output, "\n"))
	} else if cursorStr != "" {
		b.WriteString("\n(no new output)")
	}

	fmt.Fprintf(&b, "\n\ncursor:%d", totalCount)

	return textResult(b.String()), nil
}

func (s *Server) handleEventPoll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.EventBus == nil {
		return errResult("event bus not initialized"), nil
	}

	cursorStr := getStringArg(req, "cursor")
	cursor := 0
	if cursorStr != "" {
		var err error
		cursor, err = strconv.Atoi(cursorStr)
		if err != nil {
			return invalidParams(fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
	}

	limit := int(getNumberArg(req, "limit", 20))
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 20
	}

	typeFilter := events.EventType(getStringArg(req, "type"))

	evts, newCursor := s.EventBus.HistoryAfterCursor(cursor, limit*2) // fetch extra for filtering

	// Apply type filter
	var filtered []events.Event
	for _, e := range evts {
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	type compactEvent struct {
		Type      string `json:"type"`
		Time      string `json:"time"`
		Repo      string `json:"repo,omitempty"`
		SessionID string `json:"session_id,omitempty"`
		Summary   string `json:"summary"`
	}

	out := make([]compactEvent, len(filtered))
	for i, e := range filtered {
		out[i] = compactEvent{
			Type:      string(e.Type),
			Time:      e.Timestamp.Format("15:04:05"),
			Repo:      e.RepoName,
			SessionID: shortID(e.SessionID),
			Summary:   summarizeEvent(e),
		}
	}

	return jsonResult(map[string]any{
		"events":   out,
		"count":    len(out),
		"cursor":   strconv.Itoa(newCursor),
		"has_more": len(evts) > limit,
	}), nil
}

func (s *Server) handleRCAct(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := getStringArg(req, "action")
	if action == "" {
		return invalidParams("action required"), nil
	}
	target := getStringArg(req, "target")

	switch action {
	case "stop":
		sess, err := s.resolveTarget(target)
		if err != nil {
			return notFound(err.Error()), nil
		}
		sess.Lock()
		sid := sess.ID
		repoName := sess.RepoName
		cost := sess.SpentUSD
		turns := sess.TurnCount
		sess.Unlock()

		if err := s.SessMgr.Stop(sid); err != nil {
			return internalErr(fmt.Sprintf("stop failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Stopped session %s on %s (%s, %dt)",
			shortID(sid), repoName, formatCost(cost), turns)), nil

	case "stop_all":
		all := s.SessMgr.List("")
		count := 0
		for _, sess := range all {
			sess.Lock()
			st := sess.Status
			sess.Unlock()
			if st == session.StatusRunning || st == session.StatusLaunching {
				count++
			}
		}
		s.SessMgr.StopAll()
		return textResult(fmt.Sprintf("Stopped %d session(s)", count)), nil

	case "pause":
		if target == "" {
			return invalidParams("target required for pause"), nil
		}
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return internalErr(fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		r := s.findRepo(target)
		if r == nil {
			return notFound(fmt.Sprintf("repo not found: %s", target)), nil
		}
		nowPaused, err := s.ProcMgr.TogglePause(r.Path)
		if err != nil {
			return internalErr(fmt.Sprintf("pause toggle failed: %v", err)), nil
		}
		if nowPaused {
			return textResult(fmt.Sprintf("Paused loop on %s", target)), nil
		}
		return textResult(fmt.Sprintf("Resumed loop on %s", target)), nil

	case "resume":
		if target == "" {
			return invalidParams("target required for resume"), nil
		}
		sess, err := s.resolveTarget(target)
		if err != nil {
			return notFound(err.Error()), nil
		}
		sess.Lock()
		psid := sess.ProviderSessionID
		repoPath := sess.RepoPath
		provider := sess.Provider
		sess.Unlock()
		if psid == "" {
			return invalidParams("session has no provider session ID to resume"), nil
		}
		newSess, err := s.SessMgr.Resume(ctx, repoPath, provider, psid, "")
		if err != nil {
			return internalErr(fmt.Sprintf("resume failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Resumed session %s", shortID(newSess.ID))), nil

	case "retry":
		sess, err := s.resolveTarget(target)
		if err != nil {
			return notFound(err.Error()), nil
		}
		sess.Lock()
		opts := session.LaunchOptions{
			Provider:     sess.Provider,
			RepoPath:     sess.RepoPath,
			Prompt:       sess.Prompt,
			Model:        sess.Model,
			MaxBudgetUSD: sess.BudgetUSD,
			MaxTurns:     sess.MaxTurns,
			Agent:        sess.AgentName,
			TeamName:     sess.TeamName,
		}
		sess.Unlock()
		newSess, err := s.SessMgr.Launch(ctx, opts)
		if err != nil {
			return internalErr(fmt.Sprintf("retry failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Retried → new session %s", shortID(newSess.ID))), nil

	default:
		return invalidParams(fmt.Sprintf("unknown action: %s (expected: stop, stop_all, pause, resume, retry)", action)), nil
	}
}

func (s *Server) handleToolBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.ToolRecorder == nil {
		return errResult("tool benchmarking not configured"), nil
	}

	hours := getNumberArg(req, "hours", 24)
	since := time.Now().Add(-time.Duration(hours) * time.Hour)

	entries, err := s.ToolRecorder.LoadEntries(since)
	if err != nil {
		return internalErr(fmt.Sprintf("loading benchmark data: %v", err)), nil
	}

	toolFilter := getStringArg(req, "tool")
	if toolFilter != "" {
		filtered := entries[:0]
		for _, e := range entries {
			if e.ToolName == toolFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	summaries := Summarize(entries)

	// Build sorted list for stable output.
	summaryList := make([]*ToolBenchmarkSummary, 0, len(summaries))
	for _, s := range summaries {
		summaryList = append(summaryList, s)
	}

	result := map[string]any{
		"summaries":    summaryList,
		"window_hours": hours,
		"total_calls":  len(entries),
	}

	compare := getStringArg(req, "compare")
	if compare == "true" {
		// Baseline: previous window of same duration.
		baselineSince := since.Add(-time.Duration(hours) * time.Hour)
		baselineEntries, err := s.ToolRecorder.LoadEntries(baselineSince)
		if err == nil {
			// Filter baseline to only entries before 'since'.
			baselineFiltered := baselineEntries[:0]
			for _, e := range baselineEntries {
				if e.Timestamp.Before(since) {
					baselineFiltered = append(baselineFiltered, e)
				}
			}
			baselineSummaries := Summarize(baselineFiltered)
			regressions := CompareRuns(baselineSummaries, summaries)
			result["regressions"] = regressions
		}
	}

	return jsonResult(result), nil
}
