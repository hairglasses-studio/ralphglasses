package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildFleetGroup() ToolGroup {
	return ToolGroup{
		Name:        "fleet",
		Description: "Fleet operations: fleet_status, analytics, submit, budget, workers, dlq, marathon_dashboard",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_fleet_status",
				mcp.WithDescription("Fleet-wide dashboard: aggregate status, costs, health, and alerts across all repos and sessions in one call"),
				mcp.WithNumber("limit", mcp.Description("Max repos to return in full mode (default 50)")),
				mcp.WithNumber("offset", mcp.Description("Pagination offset for repos (default 0)")),
				mcp.WithString("repo", mcp.Description("Filter to a specific repo name")),
				mcp.WithBoolean("summary_only", mcp.Description("Return compact JSON with just repo names, session counts, and total spend instead of full dump")),
			), s.handleFleetStatus},
			{mcp.NewTool("ralphglasses_fleet_analytics",
				mcp.WithDescription("Cost breakdown by provider/repo/time-period with trend analysis. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithString("repo", mcp.Description("Filter by repo name")),
				mcp.WithString("provider", mcp.Description("Filter by provider")),
				mcp.WithString("window", mcp.Description("Time window as Go duration (e.g. '1h', '24h'). Default: 1h")),
			), s.handleFleetAnalytics},
			{mcp.NewTool("ralphglasses_fleet_submit",
				mcp.WithDescription("Submit work for `repo` with `prompt` to the distributed fleet queue. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Task prompt")),
				mcp.WithString("provider", mcp.Description("claude (default), gemini, codex")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget in USD (default: 5.0)")),
				mcp.WithNumber("priority", mcp.Description("Priority 0-10 (default: 5, higher = first)")),
			), s.handleFleetSubmit},
			{mcp.NewTool("ralphglasses_fleet_budget",
				mcp.WithDescription("View or set the fleet-wide budget. Shows spent, remaining, and active work. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithNumber("limit", mcp.Description("New budget limit in USD (omit to just view current budget)")),
			), s.handleFleetBudget},
			{mcp.NewTool("ralphglasses_fleet_workers",
				mcp.WithDescription("List registered fleet workers with status, capacity, and spend. Optionally pause, resume, or drain a worker. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithString("action", mcp.Description("Worker action: pause, resume, or drain (omit to list)")),
				mcp.WithString("worker_id", mcp.Description("Worker ID (required for pause/resume/drain actions)")),
			), s.handleFleetWorkers},
			{mcp.NewTool("ralphglasses_fleet_dlq",
				mcp.WithDescription("Dead letter queue operations for permanently failed fleet work items. Actions: list, retry, purge, depth. Requires fleet coordinator mode."),
				mcp.WithString("action", mcp.Description("Action: list (default), retry, purge, depth")),
				mcp.WithString("item_id", mcp.Description("Work item ID (required for retry action)")),
			), s.handleFleetDLQ},
			{mcp.NewTool("ralphglasses_marathon_dashboard",
				mcp.WithDescription("Compact marathon status: burn rate, stale sessions, team progress, alerts"),
				mcp.WithNumber("stale_threshold_min", mcp.Description("Minutes idle before flagged stale (default 5)")),
			), s.handleMarathonDashboard},
		},
	}
}

func (s *Server) buildFleetHGroup() ToolGroup {
	return ToolGroup{
		Name:        "fleet_h",
		Description: "Fleet intelligence: blackboard coordination, A2A task delegation, cost forecasting",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_blackboard_query",
				mcp.WithDescription("Query blackboard entries by namespace for fleet worker coordination. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithString("namespace", mcp.Required(), mcp.Description("Namespace to query")),
			), s.handleBlackboardQuery},
			{mcp.NewTool("ralphglasses_blackboard_put",
				mcp.WithDescription("Write an entry to the blackboard for fleet coordination. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithString("namespace", mcp.Required(), mcp.Description("Entry namespace")),
				mcp.WithString("key", mcp.Required(), mcp.Description("Entry key")),
				mcp.WithString("value", mcp.Required(), mcp.Description("JSON object value")),
				mcp.WithString("writer_id", mcp.Description("Writer identifier")),
				mcp.WithNumber("ttl_seconds", mcp.Description("Time-to-live in seconds (0 for no expiry)")),
			), s.handleBlackboardPut},
			{mcp.NewTool("ralphglasses_a2a_offers",
				mcp.WithDescription("List open agent-to-agent task delegation offers. Requires fleet server mode (ralphglasses mcp --fleet)."),
			), s.handleA2AOffers},
			{mcp.NewTool("ralphglasses_cost_forecast",
				mcp.WithDescription("Cost burn rate, anomaly detection, and budget exhaustion ETA. Requires fleet server mode (ralphglasses mcp --fleet)."),
				mcp.WithNumber("budget_remaining", mcp.Description("Remaining budget in USD for exhaustion ETA (default: 0)")),
			), s.handleCostForecast},
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
