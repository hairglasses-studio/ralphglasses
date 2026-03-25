package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/fleet"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ToolGroup represents a namespace of related tools.
type ToolGroup struct {
	Name        string
	Description string
	Tools       []ToolEntry
}

// ToolEntry pairs a tool definition with its handler.
type ToolEntry struct {
	Tool    mcp.Tool
	Handler server.ToolHandlerFunc
}

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

	// DeferredLoading controls whether only core tools are registered on startup.
	// When true, RegisterCoreTools is called instead of RegisterAllTools.
	DeferredLoading bool

	// loadedGroups tracks which tool groups have been registered (for deferred loading).
	loadedGroups map[string]bool

	// mcpSrv holds a reference to the MCPServer for deferred group loading.
	mcpSrv *server.MCPServer

	// Fleet and HITL infrastructure (set via InitFleetTools / InitSelfImprovement).
	FleetCoordinator *fleet.Coordinator
	FleetClient      *fleet.Client
	HITLTracker      *session.HITLTracker
	DecisionLog      *session.DecisionLog
	FeedbackAnalyzer *session.FeedbackAnalyzer
	AutoOptimizer    *session.AutoOptimizer
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

// ToolGroupNames lists all valid tool group names in registration order.
var ToolGroupNames = []string{
	"core", "session", "loop", "prompt", "fleet",
	"repo", "roadmap", "team", "awesome", "advanced",
}

// buildToolGroups constructs all tool groups with their tool definitions and handlers.
func (s *Server) buildToolGroups() []ToolGroup {
	return []ToolGroup{
		s.buildCoreGroup(),
		s.buildSessionGroup(),
		s.buildLoopGroup(),
		s.buildPromptGroup(),
		s.buildFleetGroup(),
		s.buildRepoGroup(),
		s.buildRoadmapGroup(),
		s.buildTeamGroup(),
		s.buildAwesomeGroup(),
		s.buildAdvancedGroup(),
	}
}

func (s *Server) buildCoreGroup() ToolGroup {
	return ToolGroup{
		Name:        "core",
		Description: "Essential fleet management: scan, list, start, stop, pause, logs, config",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_scan",
				mcp.WithDescription("Scan for ralph-enabled repos and return their current status"),
			), s.handleScan},
			{mcp.NewTool("ralphglasses_list",
				mcp.WithDescription("List all discovered repos with status summary"),
			), s.handleList},
			{mcp.NewTool("ralphglasses_status",
				mcp.WithDescription("Get detailed status for a specific repo including loop status, circuit breaker, progress, and config"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name (basename of directory)")),
			), s.handleStatus},
			{mcp.NewTool("ralphglasses_start",
				mcp.WithDescription("Start a ralph loop for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to start loop for")),
			), s.handleStart},
			{mcp.NewTool("ralphglasses_stop",
				mcp.WithDescription("Stop a running ralph loop for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to stop loop for")),
			), s.handleStop},
			{mcp.NewTool("ralphglasses_stop_all",
				mcp.WithDescription("Stop all running ralph loops"),
			), s.handleStopAll},
			{mcp.NewTool("ralphglasses_pause",
				mcp.WithDescription("Pause or resume a running ralph loop"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to pause/resume")),
			), s.handlePause},
			{mcp.NewTool("ralphglasses_logs",
				mcp.WithDescription("Get recent log lines from a repo's ralph log"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 50, max 500)")),
			), s.handleLogs},
			{mcp.NewTool("ralphglasses_config",
				mcp.WithDescription("Get or set .ralphrc config values for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("key", mcp.Description("Config key to get/set (omit to list all)")),
				mcp.WithString("value", mcp.Description("Value to set (omit to get current value)")),
			), s.handleConfig},
			{mcp.NewTool("ralphglasses_config_bulk",
				mcp.WithDescription("Get/set .ralphrc values across multiple repos"),
				mcp.WithString("key", mcp.Required(), mcp.Description("Config key to get/set")),
				mcp.WithString("value", mcp.Description("Value to set (omit to query)")),
				mcp.WithString("repos", mcp.Description("Comma-separated repo names (default: all)")),
			), s.handleConfigBulk},
		},
	}
}

func (s *Server) buildSessionGroup() ToolGroup {
	return ToolGroup{
		Name:        "session",
		Description: "LLM session lifecycle: launch, list, status, resume, stop, budget, retry, output, tail, diff, compare, errors",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_session_launch",
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
				mcp.WithBoolean("bare", mcp.Description("Skip hooks/plugins for faster scripted startup")),
				mcp.WithString("effort", mcp.Description("Thinking effort level: low, medium, high, max")),
				mcp.WithString("fallback_model", mcp.Description("Auto-fallback model on overload")),
				mcp.WithString("output_schema", mcp.Description("JSON schema for structured output validation (Claude: --json-schema, Codex: --output-schema)")),
			), s.handleSessionLaunch},
			{mcp.NewTool("ralphglasses_session_list",
				mcp.WithDescription("List all tracked LLM sessions with status, cost, and turns"),
				mcp.WithString("repo", mcp.Description("Filter by repo name (omit for all)")),
				mcp.WithString("provider", mcp.Description("Filter by provider: claude, gemini, codex (omit for all)")),
				mcp.WithString("status", mcp.Description("Filter by status: running, completed, errored, stopped")),
			), s.handleSessionList},
			{mcp.NewTool("ralphglasses_session_status",
				mcp.WithDescription("Get detailed status for a Claude Code session including output, cost, and turns"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
			), s.handleSessionStatus},
			{mcp.NewTool("ralphglasses_session_resume",
				mcp.WithDescription("Resume a previous LLM CLI session"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("session_id", mcp.Required(), mcp.Description("Provider session ID to resume (from session status)")),
				mcp.WithString("provider", mcp.Description("LLM provider: claude (default), gemini, codex")),
				mcp.WithString("prompt", mcp.Description("Follow-up prompt (optional)")),
			), s.handleSessionResume},
			{mcp.NewTool("ralphglasses_session_stop",
				mcp.WithDescription("Stop a running Claude Code session"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to stop")),
			), s.handleSessionStop},
			{mcp.NewTool("ralphglasses_session_stop_all",
				mcp.WithDescription("Stop all running LLM sessions — emergency cost cutoff"),
			), s.handleSessionStopAll},
			{mcp.NewTool("ralphglasses_session_budget",
				mcp.WithDescription("Get cost/budget info for a session, or update budget"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
				mcp.WithNumber("budget", mcp.Description("New budget in USD (omit to just query)")),
			), s.handleSessionBudget},
			{mcp.NewTool("ralphglasses_session_retry",
				mcp.WithDescription("Re-launch a failed session with same params, optional overrides"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to retry")),
				mcp.WithString("model", mcp.Description("Override model")),
				mcp.WithNumber("max_budget_usd", mcp.Description("Override budget")),
			), s.handleSessionRetry},
			{mcp.NewTool("ralphglasses_session_output",
				mcp.WithDescription("Get recent output from a session's output history"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
				mcp.WithNumber("lines", mcp.Description("Number of output lines (default 20, max 100)")),
			), s.handleSessionOutput},
			{mcp.NewTool("ralphglasses_session_tail",
				mcp.WithDescription("Tail session output with cursor — returns only new lines since last call"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
				mcp.WithString("cursor", mcp.Description("Cursor from previous response (omit for latest)")),
				mcp.WithNumber("lines", mcp.Description("Max lines to return (default 30, max 100)")),
			), s.handleSessionTail},
			{mcp.NewTool("ralphglasses_session_diff",
				mcp.WithDescription("Git changes made during a session's execution window"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID")),
				mcp.WithString("stat_only", mcp.Description("true/false (default: true)")),
				mcp.WithNumber("max_lines", mcp.Description("Truncate diff at N lines (default 200)")),
			), s.handleSessionDiff},
			{mcp.NewTool("ralphglasses_session_compare",
				mcp.WithDescription("Compare two sessions by ID: cost, turns, duration, provider efficiency"),
				mcp.WithString("id1", mcp.Required(), mcp.Description("First session ID")),
				mcp.WithString("id2", mcp.Required(), mcp.Description("Second session ID")),
			), s.handleSessionCompare},
			{mcp.NewTool("ralphglasses_session_errors",
				mcp.WithDescription("Aggregated error view: parse failures, API errors, budget warnings"),
				mcp.WithString("repo", mcp.Description("Filter by repo name")),
				mcp.WithString("severity", mcp.Description("Filter: critical, warning, info")),
				mcp.WithNumber("limit", mcp.Description("Max errors (default 50)")),
			), s.handleSessionErrors},
		},
	}
}

func (s *Server) buildLoopGroup() ToolGroup {
	return ToolGroup{
		Name:        "loop",
		Description: "Perpetual development loops: start, status, step, stop, benchmark, baseline, gates, self-test, self-improve",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_loop_start",
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
				mcp.WithBoolean("enable_reflexion", mcp.Description("Enable reflexion loop (failure correction injection)")),
				mcp.WithBoolean("enable_episodic_memory", mcp.Description("Enable episodic memory (successful trajectory recall)")),
				mcp.WithBoolean("enable_cascade", mcp.Description("Enable cascade routing (cheap-then-expensive provider)")),
				mcp.WithBoolean("enable_uncertainty", mcp.Description("Enable uncertainty quantification (confidence scoring)")),
				mcp.WithBoolean("enable_curriculum", mcp.Description("Enable curriculum learning (difficulty-based task sorting)")),
				mcp.WithBoolean("self_improvement", mcp.Description("Enable self-improvement mode with autonomous acceptance gate")),
				mcp.WithNumber("max_iterations", mcp.Description("Maximum loop iterations (0 = unlimited)")),
				mcp.WithNumber("duration_hours", mcp.Description("Maximum loop duration in hours (0 = unlimited)")),
				mcp.WithNumber("budget_usd", mcp.Description("Total budget in USD (split 1/3 planner, 2/3 worker)")),
			), s.handleLoopStart},
			{mcp.NewTool("ralphglasses_loop_status",
				mcp.WithDescription("Get status for a perpetual development loop"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
			), s.handleLoopStatus},
			{mcp.NewTool("ralphglasses_loop_step",
				mcp.WithDescription("Execute one planner/worker/verifier iteration for a loop"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
			), s.handleLoopStep},
			{mcp.NewTool("ralphglasses_loop_stop",
				mcp.WithDescription("Stop a perpetual development loop"),
				mcp.WithString("id", mcp.Required(), mcp.Description("Loop run ID")),
			), s.handleLoopStop},
			{mcp.NewTool("ralphglasses_loop_benchmark",
				mcp.WithDescription("P50/P95 metrics from recent loop observations for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("hours", mcp.Description("Look-back window in hours (default: 48)")),
			), s.handleLoopBenchmark},
			{mcp.NewTool("ralphglasses_loop_baseline",
				mcp.WithDescription("Generate, view, or pin loop performance baseline for a repo"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("action", mcp.Description("Action: view (default), refresh, or pin")),
				mcp.WithNumber("hours", mcp.Description("Window for refresh/pin in hours (default: 48)")),
			), s.handleLoopBaseline},
			{mcp.NewTool("ralphglasses_loop_gates",
				mcp.WithDescription("Evaluate regression gates — returns pass/warn/fail report"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("hours", mcp.Description("Look-back window in hours (default: 24)")),
			), s.handleLoopGates},
			{mcp.NewTool("ralphglasses_self_test",
				mcp.WithDescription("Run recursive self-test iterations against a repository using the ralphglasses loop engine"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Absolute path to the repository to test")),
				mcp.WithNumber("iterations", mcp.Description("Number of self-test iterations (default: 3)")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget cap in USD (default: 5.0)")),
				mcp.WithBoolean("use_snapshot", mcp.Description("Restore repo snapshot between iterations (default: true)")),
			), s.handleSelfTest},
			{mcp.NewTool("ralphglasses_self_improve",
				mcp.WithDescription("Start a self-improvement loop that autonomously improves a repository — auto-merges safe changes, creates PRs for review-required changes"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("max_iterations", mcp.Description("Maximum iterations (default: 5)")),
				mcp.WithNumber("budget_usd", mcp.Description("Total budget in USD (default: 20.0, split 1/4 planner + 3/4 worker)")),
				mcp.WithNumber("duration_hours", mcp.Description("Maximum duration in hours (default: 4)")),
			), s.handleSelfImprove},
		},
	}
}

func (s *Server) buildPromptGroup() ToolGroup {
	return ToolGroup{
		Name:        "prompt",
		Description: "Prompt enhancement: analyze, enhance, lint, improve, classify, should_enhance, templates, template_fill",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_prompt_analyze",
				mcp.WithDescription("Score a prompt across 10 quality dimensions (clarity, specificity, structure, examples, etc.) with letter grades and actionable suggestions"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to analyze")),
				mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
				mcp.WithString("target_provider", mcp.Description("Target model provider for scoring suggestions: claude (default), gemini, openai")),
			), s.handlePromptAnalyze},
			{mcp.NewTool("ralphglasses_prompt_enhance",
				mcp.WithDescription("Run the 13-stage prompt enhancement pipeline (specificity, positive reframing, XML structure, context reorder, format enforcement, etc.)"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to enhance")),
				mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
				mcp.WithString("mode", mcp.Description("Enhancement mode: local (default, deterministic), llm (Claude/Gemini/OpenAI API), auto (try LLM, fallback to local)")),
				mcp.WithString("repo", mcp.Description("Repo name to load .prompt-improver.yaml config from")),
				mcp.WithString("target_provider", mcp.Description("Target model provider — controls structure style and scoring: claude (default), gemini, openai")),
			), s.handlePromptEnhance},
			{mcp.NewTool("ralphglasses_prompt_lint",
				mcp.WithDescription("Deep-lint a prompt for anti-patterns: unmotivated rules, negative framing, aggressive caps, vague quantifiers, injection risks, cache-unfriendly ordering"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to lint")),
			), s.handlePromptLint},
			{mcp.NewTool("ralphglasses_prompt_improve",
				mcp.WithDescription("LLM-powered prompt improvement using Claude, Gemini, or OpenAI with domain-specific meta-prompts"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to improve")),
				mcp.WithString("task_type", mcp.Description("Override auto-detection: code, troubleshooting, analysis, creative, workflow, general")),
				mcp.WithBoolean("thinking_enabled", mcp.Description("Include thinking scaffolding in the improved prompt")),
				mcp.WithString("feedback", mcp.Description("Optional feedback to guide the improvement direction")),
				mcp.WithString("provider", mcp.Description("LLM provider for improvement: claude (default, requires ANTHROPIC_API_KEY), gemini (requires GOOGLE_API_KEY), openai (requires OPENAI_API_KEY)")),
			), s.handlePromptImprove},
			{mcp.NewTool("ralphglasses_prompt_classify",
				mcp.WithDescription("Classify a prompt's task type (code, troubleshooting, analysis, creative, workflow, general)"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to classify")),
			), s.handlePromptClassify},
			{mcp.NewTool("ralphglasses_prompt_should_enhance",
				mcp.WithDescription("Check whether a prompt would benefit from enhancement"),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt text to check")),
				mcp.WithString("repo", mcp.Description("Repo name for loading .prompt-improver.yaml config")),
			), s.handlePromptShouldEnhance},
			{mcp.NewTool("ralphglasses_prompt_templates",
				mcp.WithDescription("List available prompt templates with descriptions and required variables"),
			), s.handlePromptTemplates},
			{mcp.NewTool("ralphglasses_prompt_template_fill",
				mcp.WithDescription("Fill a prompt template with variable values"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Template name")),
				mcp.WithString("vars", mcp.Required(), mcp.Description("JSON object of variable key-value pairs")),
			), s.handlePromptTemplateFill},
		},
	}
}

func (s *Server) buildFleetGroup() ToolGroup {
	return ToolGroup{
		Name:        "fleet",
		Description: "Fleet operations: fleet_status, analytics, submit, budget, workers, marathon_dashboard",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_fleet_status",
				mcp.WithDescription("Fleet-wide dashboard: aggregate status, costs, health, and alerts across all repos and sessions in one call"),
			), s.handleFleetStatus},
			{mcp.NewTool("ralphglasses_fleet_analytics",
				mcp.WithDescription("Cost breakdown by provider/repo/time-period with trend analysis"),
				mcp.WithString("repo", mcp.Description("Filter by repo name")),
				mcp.WithString("provider", mcp.Description("Filter by provider")),
			), s.handleFleetAnalytics},
			{mcp.NewTool("ralphglasses_fleet_submit",
				mcp.WithDescription("Submit work to the distributed fleet queue for execution on any worker"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Task prompt")),
				mcp.WithString("provider", mcp.Description("claude (default), gemini, codex")),
				mcp.WithNumber("budget", mcp.Description("Max budget USD (default: 5)")),
				mcp.WithNumber("priority", mcp.Description("Priority 0-10 (default: 5, higher = first)")),
			), s.handleFleetSubmit},
			{mcp.NewTool("ralphglasses_fleet_budget",
				mcp.WithDescription("View or set the fleet-wide budget. Shows spent, remaining, and active work."),
				mcp.WithNumber("limit", mcp.Description("New budget limit in USD (omit to just view)")),
			), s.handleFleetBudget},
			{mcp.NewTool("ralphglasses_fleet_workers",
				mcp.WithDescription("List registered fleet workers with status, capacity, and spend"),
			), s.handleFleetWorkers},
			{mcp.NewTool("ralphglasses_marathon_dashboard",
				mcp.WithDescription("Compact marathon status: burn rate, stale sessions, team progress, alerts"),
				mcp.WithNumber("stale_threshold_min", mcp.Description("Minutes idle before flagged stale (default 5)")),
			), s.handleMarathonDashboard},
		},
	}
}

func (s *Server) buildRepoGroup() ToolGroup {
	return ToolGroup{
		Name:        "repo",
		Description: "Repo management: health, optimize, scaffold, claudemd_check, snapshot",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_repo_health",
				mcp.WithDescription("Composite health check: circuit breaker, budget, staleness, errors, active sessions"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
			), s.handleRepoHealth},
			{mcp.NewTool("ralphglasses_repo_optimize",
				mcp.WithDescription("Analyze and optimize ralph config files — detect misconfigs, missing settings, stale plans"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("focus", mcp.Description("Focus area: config, prompt, plan, all (default: all)")),
				mcp.WithString("dry_run", mcp.Description("Report only, don't modify: true/false (default: true)")),
			), s.handleRepoOptimize},
			{mcp.NewTool("ralphglasses_repo_scaffold",
				mcp.WithDescription("Create/initialize ralph config files (.ralph/, .ralphrc, PROMPT.md, AGENT.md, fix_plan.md) for a repo"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("project_type", mcp.Description("Project type override (auto-detected from go.mod, package.json, etc.)")),
				mcp.WithString("project_name", mcp.Description("Project name override (defaults to directory name)")),
				mcp.WithString("force", mcp.Description("Overwrite existing files: true/false (default: false)")),
			), s.handleRepoScaffold},
			{mcp.NewTool("ralphglasses_claudemd_check",
				mcp.WithDescription("Health-check a repo's CLAUDE.md for common issues (length, inline code, overtrigger language, missing headers)"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
			), s.handleClaudeMDCheck},
			{mcp.NewTool("ralphglasses_snapshot",
				mcp.WithDescription("Save or list fleet state snapshots"),
				mcp.WithString("action", mcp.Description("Action: save (default) or list")),
				mcp.WithString("name", mcp.Description("Snapshot name (auto-generated if omitted)")),
			), s.handleSnapshot},
		},
	}
}

func (s *Server) buildRoadmapGroup() ToolGroup {
	return ToolGroup{
		Name:        "roadmap",
		Description: "Roadmap automation: parse, analyze, research, expand, export",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_roadmap_parse",
				mcp.WithDescription("Parse ROADMAP.md into structured JSON (phases, sections, tasks, deps, completion stats)"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root or direct .md path")),
				mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
			), s.handleRoadmapParse},
			{mcp.NewTool("ralphglasses_roadmap_analyze",
				mcp.WithDescription("Compare roadmap vs codebase — find gaps, stale checkboxes, ready tasks, orphaned code"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
			), s.handleRoadmapAnalyze},
			{mcp.NewTool("ralphglasses_roadmap_research",
				mcp.WithDescription("Search GitHub for relevant repos and tools that unlock new capabilities"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("topics", mcp.Description("Search topics (inferred from go.mod/README if omitted)")),
				mcp.WithNumber("limit", mcp.Description("Max results (default 10)")),
			), s.handleRoadmapResearch},
			{mcp.NewTool("ralphglasses_roadmap_expand",
				mcp.WithDescription("Generate proposed roadmap expansions from analysis gaps and research findings"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
				mcp.WithString("research", mcp.Description("Research topics to include (runs research internally)")),
				mcp.WithString("style", mcp.Description("Expansion style: conservative, balanced, aggressive (default: balanced)")),
			), s.handleRoadmapExpand},
			{mcp.NewTool("ralphglasses_roadmap_export",
				mcp.WithDescription("Export roadmap items as structured task specs for ralph loop consumption"),
				mcp.WithString("path", mcp.Required(), mcp.Description("Repo root path")),
				mcp.WithString("file", mcp.Description("Override filename (default: ROADMAP.md)")),
				mcp.WithString("format", mcp.Description("Output format: rdcycle, fix_plan, progress (default: rdcycle)")),
				mcp.WithString("phase", mcp.Description("Filter by phase name (default: all)")),
				mcp.WithString("section", mcp.Description("Filter by section name (default: all)")),
				mcp.WithNumber("max_tasks", mcp.Description("Max tasks to export (default 20)")),
				mcp.WithString("respect_deps", mcp.Description("Skip tasks with unmet deps (default: true)")),
			), s.handleRoadmapExport},
		},
	}
}

func (s *Server) buildTeamGroup() ToolGroup {
	return ToolGroup{
		Name:        "team",
		Description: "Agent teams and definitions: team_create, team_status, team_delegate, agent_define, agent_list, agent_compose",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_team_create",
				mcp.WithDescription("Create an agent team with a lead session that delegates tasks to teammates"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
				mcp.WithString("tasks", mcp.Required(), mcp.Description("Newline-separated task descriptions")),
				mcp.WithString("provider", mcp.Description("LLM provider for lead: claude (default), gemini, codex")),
				mcp.WithString("worker_provider", mcp.Description("Default LLM provider for worker tasks: claude, gemini, codex")),
				mcp.WithString("lead_agent", mcp.Description("Agent definition for the lead (from .claude/agents/)")),
				mcp.WithString("model", mcp.Description("Model for lead session")),
				mcp.WithNumber("max_budget_usd", mcp.Description("Total budget for the team")),
			), s.handleTeamCreate},
			{mcp.NewTool("ralphglasses_team_status",
				mcp.WithDescription("Get team status including lead session, tasks, and progress"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
			), s.handleTeamStatus},
			{mcp.NewTool("ralphglasses_team_delegate",
				mcp.WithDescription("Add a new task to an existing team"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Team name")),
				mcp.WithString("task", mcp.Required(), mcp.Description("Task description to delegate")),
				mcp.WithString("provider", mcp.Description("LLM provider override for this task: claude, gemini, codex")),
			), s.handleTeamDelegate},
			{mcp.NewTool("ralphglasses_agent_define",
				mcp.WithDescription("Create or update an agent definition for a repo (supports all providers)"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Agent name")),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Agent system prompt / instructions (markdown)")),
				mcp.WithString("provider", mcp.Description("Target provider: claude (default, .claude/agents/), gemini (.gemini/agents/), codex (AGENTS.md)")),
				mcp.WithString("description", mcp.Description("Agent description")),
				mcp.WithString("model", mcp.Description("Model override (sonnet, opus, haiku)")),
				mcp.WithString("tools", mcp.Description("Comma-separated allowed tools")),
				mcp.WithNumber("max_turns", mcp.Description("Max turns for this agent")),
			), s.handleAgentDefine},
			{mcp.NewTool("ralphglasses_agent_list",
				mcp.WithDescription("List available agent definitions for a repo (supports all providers)"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("provider", mcp.Description("Filter by provider: claude (default), gemini, codex, or 'all'")),
			), s.handleAgentList},
			{mcp.NewTool("ralphglasses_agent_compose",
				mcp.WithDescription("Create a composite agent by layering multiple existing agent definitions"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Name for the composite agent")),
				mcp.WithString("agents", mcp.Required(), mcp.Description("Comma-separated agent names to compose")),
				mcp.WithString("provider", mcp.Description("Provider: claude (default), gemini, codex")),
				mcp.WithString("model", mcp.Description("Override model for composite agent")),
			), s.handleAgentCompose},
		},
	}
}

func (s *Server) buildAwesomeGroup() ToolGroup {
	return ToolGroup{
		Name:        "awesome",
		Description: "Awesome-list research: fetch, analyze, diff, report, sync",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_awesome_fetch",
				mcp.WithDescription("Fetch and parse an awesome-list README into structured entries with categories"),
				mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
			), s.handleAwesomeFetch},
			{mcp.NewTool("ralphglasses_awesome_analyze",
				mcp.WithDescription("Deep-analyze repos: fetch READMEs, score value/complexity vs ralph capabilities"),
				mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
				mcp.WithNumber("max_workers", mcp.Description("Concurrent README fetches (default 5)")),
			), s.handleAwesomeAnalyze},
			{mcp.NewTool("ralphglasses_awesome_diff",
				mcp.WithDescription("Compare current awesome-list against previous fetch (new/removed entries)"),
				mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path where previous index is saved")),
				mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
			), s.handleAwesomeDiff},
			{mcp.NewTool("ralphglasses_awesome_report",
				mcp.WithDescription("Generate formatted report from saved analysis results"),
				mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path where analysis is saved")),
				mcp.WithString("format", mcp.Description("Output format: json or markdown (default: markdown)")),
			), s.handleAwesomeReport},
			{mcp.NewTool("ralphglasses_awesome_sync",
				mcp.WithDescription("Full pipeline: fetch awesome-list → diff → analyze new entries → report → save"),
				mcp.WithString("save_to", mcp.Required(), mcp.Description("Repo path to save results")),
				mcp.WithString("repo", mcp.Description("GitHub repo (default: hesreallyhim/awesome-claude-code)")),
				mcp.WithString("full_rescan", mcp.Description("Re-analyze all entries, not just new: true/false (default: false)")),
				mcp.WithNumber("max_workers", mcp.Description("Concurrent README fetches (default 5)")),
			), s.handleAwesomeSync},
		},
	}
}

func (s *Server) buildAdvancedGroup() ToolGroup {
	return ToolGroup{
		Name:        "advanced",
		Description: "Advanced: RC tools, events, HITL, autonomy, feedback, provider recommend, journals, workflows, tool benchmark",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_rc_status",
				mcp.WithDescription("Compact fleet overview for mobile: active sessions, costs, alerts in readable text"),
			), s.handleRCStatus},
			{mcp.NewTool("ralphglasses_rc_send",
				mcp.WithDescription("Send prompt to repo — auto-stops existing session, launches new. The 'input' tool for remote control."),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("What to tell the agent")),
				mcp.WithString("provider", mcp.Description("claude (default), gemini, codex")),
				mcp.WithString("model", mcp.Description("Override model")),
				mcp.WithNumber("budget", mcp.Description("Max budget USD (default: 5)")),
				mcp.WithString("resume", mcp.Description("true to resume last session instead of fresh start")),
			), s.handleRCSend},
			{mcp.NewTool("ralphglasses_rc_read",
				mcp.WithDescription("Read recent output from most active session. Combines tail + status for mobile."),
				mcp.WithString("id", mcp.Description("Session ID (omit for most recently active)")),
				mcp.WithString("cursor", mcp.Description("Cursor from previous call — only new output")),
				mcp.WithNumber("lines", mcp.Description("Max lines (default 10, max 30)")),
			), s.handleRCRead},
			{mcp.NewTool("ralphglasses_rc_act",
				mcp.WithDescription("Quick fleet action: stop, stop_all, pause, resume, retry. Single tool for all control actions."),
				mcp.WithString("action", mcp.Required(), mcp.Description("Action: stop, stop_all, pause, resume, retry")),
				mcp.WithString("target", mcp.Description("Session ID or repo name (required except stop_all)")),
			), s.handleRCAct},
			{mcp.NewTool("ralphglasses_event_list",
				mcp.WithDescription("Query recent fleet events from the event bus"),
				mcp.WithString("type", mcp.Description("Filter by event type (e.g. session.started, cost.update)")),
				mcp.WithString("repo", mcp.Description("Filter by repo name")),
				mcp.WithNumber("limit", mcp.Description("Max events to return (default 50)")),
				mcp.WithString("since", mcp.Description("ISO timestamp filter")),
			), s.handleEventList},
			{mcp.NewTool("ralphglasses_event_poll",
				mcp.WithDescription("Poll for new fleet events since last check. Cursor-based for efficient mobile polling."),
				mcp.WithString("cursor", mcp.Description("Cursor from previous response (omit for recent)")),
				mcp.WithNumber("limit", mcp.Description("Max events (default 20, max 50)")),
				mcp.WithString("type", mcp.Description("Filter by event type (e.g. session.started, cost.update)")),
			), s.handleEventPoll},
			{mcp.NewTool("ralphglasses_hitl_score",
				mcp.WithDescription("Current human-in-the-loop score: manual interventions vs autonomous actions, with trend"),
				mcp.WithNumber("hours", mcp.Description("Time window in hours (default: 24)")),
			), s.handleHITLScore},
			{mcp.NewTool("ralphglasses_hitl_history",
				mcp.WithDescription("Recent HITL events: manual stops, auto-recoveries, config changes, etc."),
				mcp.WithNumber("hours", mcp.Description("Time window in hours (default: 24)")),
				mcp.WithNumber("limit", mcp.Description("Max events (default: 50)")),
			), s.handleHITLHistory},
			{mcp.NewTool("ralphglasses_autonomy_level",
				mcp.WithDescription("View or set the autonomy level (0=observe, 1=auto-recover, 2=auto-optimize, 3=full-autonomy)"),
				mcp.WithString("level", mcp.Description("New level: 0-3 or name (omit to view current)")),
			), s.handleAutonomyLevel},
			{mcp.NewTool("ralphglasses_autonomy_decisions",
				mcp.WithDescription("Recent autonomous decisions with rationale, inputs, and outcomes"),
				mcp.WithNumber("limit", mcp.Description("Max decisions (default: 20)")),
			), s.handleAutonomyDecisions},
			{mcp.NewTool("ralphglasses_autonomy_override",
				mcp.WithDescription("Override/reverse an autonomous decision and record human intervention"),
				mcp.WithString("decision_id", mcp.Required(), mcp.Description("Decision ID to override")),
				mcp.WithString("details", mcp.Description("Why this was overridden")),
			), s.handleAutonomyOverride},
			{mcp.NewTool("ralphglasses_feedback_profiles",
				mcp.WithDescription("View feedback profiles: per-task-type and per-provider performance data from journal analysis"),
				mcp.WithString("type", mcp.Description("Filter: prompt, provider, or omit for both")),
			), s.handleFeedbackProfiles},
			{mcp.NewTool("ralphglasses_provider_recommend",
				mcp.WithDescription("Recommend best provider + model + budget for a task based on feedback profiles and cost normalization"),
				mcp.WithString("task", mcp.Required(), mcp.Description("Task description (e.g. 'fix lint errors', 'add search feature')")),
			), s.handleProviderRecommend},
			{mcp.NewTool("ralphglasses_tool_benchmark",
				mcp.WithDescription("Per-tool performance benchmarks: latency percentiles, success rates, and regression detection"),
				mcp.WithString("tool", mcp.Description("Filter to a specific tool name")),
				mcp.WithString("compare", mcp.Description("Include regression analysis vs previous baseline: true/false (default: false)")),
				mcp.WithNumber("hours", mcp.Description("Time window in hours (default 24)")),
			), s.handleToolBenchmark},
			{mcp.NewTool("ralphglasses_journal_read",
				mcp.WithDescription("Read improvement journal entries for a repo with synthesized context"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("limit", mcp.Description("Max entries to return (default 10)")),
			), s.handleJournalRead},
			{mcp.NewTool("ralphglasses_journal_write",
				mcp.WithDescription("Manually write an improvement note to a repo's journal"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("worked", mcp.Description("Comma-separated items that worked")),
				mcp.WithString("failed", mcp.Description("Comma-separated items that failed")),
				mcp.WithString("suggest", mcp.Description("Comma-separated suggestions")),
				mcp.WithString("session_id", mcp.Description("Associated session ID (optional)")),
			), s.handleJournalWrite},
			{mcp.NewTool("ralphglasses_journal_prune",
				mcp.WithDescription("Compact improvement journal to prevent unbounded growth"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("keep", mcp.Description("Number of entries to keep (default 100)")),
				mcp.WithString("dry_run", mcp.Description("Preview only, don't modify: true/false (default: true)")),
			), s.handleJournalPrune},
			{mcp.NewTool("ralphglasses_workflow_define",
				mcp.WithDescription("Define a multi-step workflow as YAML"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name")),
				mcp.WithString("yaml", mcp.Required(), mcp.Description("Workflow YAML definition")),
			), s.handleWorkflowDefine},
			{mcp.NewTool("ralphglasses_workflow_run",
				mcp.WithDescription("Execute a defined workflow, launching sessions per step"),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("name", mcp.Required(), mcp.Description("Workflow name")),
			), s.handleWorkflowRun},
		},
	}
}

// Register adds all ralphglasses tools to the MCP server (backward compatible).
func (s *Server) Register(srv *server.MCPServer) {
	if s.DeferredLoading {
		s.RegisterCoreTools(srv)
	} else {
		s.RegisterAllTools(srv)
	}
}

// RegisterCoreTools registers only essential tools plus the deferred loading tools.
func (s *Server) RegisterCoreTools(srv *server.MCPServer) {
	s.mcpSrv = srv
	s.loadedGroups = make(map[string]bool)

	// Register the tool_groups and load_tool_group management tools.
	srv.AddTool(mcp.NewTool("ralphglasses_tool_groups",
		mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
	), s.handleToolGroups)

	srv.AddTool(mcp.NewTool("ralphglasses_load_tool_group",
		mcp.WithDescription("Load all tools in a named group (session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced)"),
		mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
	), s.handleLoadToolGroup)

	// Register core group tools.
	coreGroup := s.buildCoreGroup()
	for _, entry := range coreGroup.Tools {
		srv.AddTool(entry.Tool, entry.Handler)
	}
	s.loadedGroups["core"] = true
}

// RegisterToolGroup registers all tools in a named group. Returns an error if
// the group name is unknown. Safe to call multiple times (idempotent).
func (s *Server) RegisterToolGroup(srv *server.MCPServer, group string) error {
	groups := s.buildToolGroups()
	for _, g := range groups {
		if g.Name == group {
			for _, entry := range g.Tools {
				srv.AddTool(entry.Tool, entry.Handler)
			}
			if s.loadedGroups != nil {
				s.loadedGroups[group] = true
			}
			return nil
		}
	}
	return fmt.Errorf("unknown tool group: %q (valid: %s)", group, strings.Join(ToolGroupNames, ", "))
}

// RegisterAllTools registers every tool across all groups (backward compatibility).
func (s *Server) RegisterAllTools(srv *server.MCPServer) {
	s.mcpSrv = srv
	s.loadedGroups = make(map[string]bool)

	// Register group management tools so they are always available.
	srv.AddTool(mcp.NewTool("ralphglasses_tool_groups",
		mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
	), s.handleToolGroups)

	srv.AddTool(mcp.NewTool("ralphglasses_load_tool_group",
		mcp.WithDescription("Load all tools in a named group (session, loop, prompt, fleet, repo, roadmap, team, awesome, advanced)"),
		mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
	), s.handleLoadToolGroup)

	for _, g := range s.buildToolGroups() {
		for _, entry := range g.Tools {
			srv.AddTool(entry.Tool, entry.Handler)
		}
		s.loadedGroups[g.Name] = true
	}
}

// ToolGroups returns all tool group metadata (for testing and introspection).
func (s *Server) ToolGroups() []ToolGroup {
	return s.buildToolGroups()
}

// handleToolGroups returns available tool groups with their descriptions and tool counts.
func (s *Server) handleToolGroups(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	type groupInfo struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		ToolCount   int      `json:"tool_count"`
		Loaded      bool     `json:"loaded"`
		Tools       []string `json:"tools"`
	}

	groups := s.buildToolGroups()
	out := make([]groupInfo, len(groups))
	for i, g := range groups {
		tools := make([]string, len(g.Tools))
		for j, t := range g.Tools {
			tools[j] = t.Tool.Name
		}
		out[i] = groupInfo{
			Name:        g.Name,
			Description: g.Description,
			ToolCount:   len(g.Tools),
			Loaded:      s.loadedGroups[g.Name],
			Tools:       tools,
		}
	}
	return jsonResult(out), nil
}

// handleLoadToolGroup loads all tools in a named group on demand.
func (s *Server) handleLoadToolGroup(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	group := getStringArg(req, "group")
	if group == "" {
		return invalidParams("group is required"), nil
	}

	if s.loadedGroups[group] {
		return jsonResult(map[string]any{
			"group":   group,
			"status":  "already_loaded",
			"message": fmt.Sprintf("Tool group %q is already loaded", group),
		}), nil
	}

	if s.mcpSrv == nil {
		return internalErr("MCP server reference not set"), nil
	}

	if err := s.RegisterToolGroup(s.mcpSrv, group); err != nil {
		return invalidParams(err.Error()), nil
	}

	// Count tools in the loaded group.
	var count int
	for _, g := range s.buildToolGroups() {
		if g.Name == group {
			count = len(g.Tools)
			break
		}
	}

	return jsonResult(map[string]any{
		"group":      group,
		"status":     "loaded",
		"tool_count": count,
		"message":    fmt.Sprintf("Loaded %d tools from group %q", count, group),
	}), nil
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
