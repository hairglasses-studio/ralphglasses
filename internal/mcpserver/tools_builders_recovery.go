package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildRecoveryGroup() ToolGroup {
	return ToolGroup{
		Name:        "recovery",
		Description: "Emergency session recovery: triage killed sessions, salvage partial output, generate recovery plans, batch re-launch, write incident reports, discover orphaned sessions",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_session_triage",
				mcp.WithDescription(descriptions.DescRalphglassesSessionTriage),
				mcp.WithString("since", mcp.Description("Start of time window (RFC3339 or relative: '2h', '30m', '1d'). Default: 1h")),
				mcp.WithString("until", mcp.Description("End of time window (RFC3339 or relative). Default: now")),
				mcp.WithString("repo", mcp.Description("Filter by repo name (omit for all)")),
				mcp.WithString("status", mcp.Description("Comma-separated statuses: interrupted, errored, stopped (default: interrupted,errored)")),
			), s.handleSessionTriage},
			{mcp.NewTool("ralphglasses_session_salvage",
				mcp.WithDescription(descriptions.DescRalphglassesSessionSalvage),
				mcp.WithString("id", mcp.Required(), mcp.Description("Session ID to salvage")),
				mcp.WithBoolean("generate_prompt", mcp.Description("Generate a recovery prompt that continues where the session left off (default: true)")),
				mcp.WithString("save_to_docs", mcp.Description("Domain to save salvaged findings to docs/research/<domain>/ (omit to skip)")),
			), s.handleSessionSalvage},
			{mcp.NewTool("ralphglasses_recovery_plan",
				mcp.WithDescription(descriptions.DescRalphglassesRecoveryPlan),
				mcp.WithString("session_ids", mcp.Description("Comma-separated session IDs (omit to auto-discover from time window)")),
				mcp.WithString("since", mcp.Description("Time window for auto-discovery (RFC3339 or relative). Default: 1h")),
				mcp.WithNumber("budget_cap_usd", mcp.Description("Max total budget for retry operations (default: 50.0)")),
				mcp.WithString("strategy", mcp.Description("Strategy: conservative, aggressive, cost-aware (default: cost-aware)")),
			), s.handleRecoveryPlan},
			{mcp.NewTool("ralphglasses_recovery_execute",
				mcp.WithDescription(descriptions.DescRalphglassesRecoveryExecute),
				mcp.WithString("plan_json", mcp.Required(), mcp.Description("JSON recovery plan (the 'actions' array from recovery_plan output)")),
				mcp.WithNumber("budget_cap_usd", mcp.Description("Total sweep budget cap (default: 50.0)")),
				mcp.WithNumber("concurrency", mcp.Description("Max simultaneous re-launches (default: 5)")),
				mcp.WithString("model_override", mcp.Description("Override model for all retries (e.g., downgrade to save cost)")),
			), s.handleRecoveryExecute},
			{mcp.NewTool("ralphglasses_incident_report",
				mcp.WithDescription(descriptions.DescRalphglassesIncidentReport),
				mcp.WithString("title", mcp.Required(), mcp.Description("Incident title (kebab-case, used as filename)")),
				mcp.WithString("cause", mcp.Description("Root cause description (e.g., 'hyprland-hy3-plugin-crash')")),
				mcp.WithString("session_ids", mcp.Description("Comma-separated affected session IDs (omit to auto-discover)")),
				mcp.WithString("since", mcp.Description("Incident window start (RFC3339 or relative). Default: 1h")),
				mcp.WithString("recovery_sweep_id", mcp.Description("Sweep ID from recovery_execute, if recovery was run")),
			), s.handleIncidentReport},
			{mcp.NewTool("ralphglasses_session_discover",
				mcp.WithDescription(descriptions.DescRalphglassesSessionDiscover),
				mcp.WithString("scan_path", mcp.Description("Base directory to scan (default: configured scan path)")),
				mcp.WithBoolean("include_claude_projects", mcp.Description("Also scan ~/.claude/projects/ for session metadata (default: true)")),
				mcp.WithBoolean("check_processes", mcp.Description("Check if discovered sessions still have running processes (default: true)")),
			), s.handleSessionDiscover},
		},
	}
}
