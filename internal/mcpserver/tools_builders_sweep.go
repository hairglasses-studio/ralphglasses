package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildSweepGroup() ToolGroup {
	return ToolGroup{
		Name:        "sweep",
		Description: "Cross-repo audit sweeps: generate optimized prompts, fan-out sessions, monitor, nudge stalled sessions, schedule recurring checks",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_sweep_generate",
				mcp.WithDescription("Generate an optimized audit prompt using the 13-stage enhancer pipeline. Returns enhanced prompt text, quality score, and stages applied."),
				mcp.WithString("task_type", mcp.Description("Task type for prompt template: audit (default), review, improve")),
				mcp.WithString("target_provider", mcp.Description("Target model provider for structure style: claude (default), gemini, openai")),
				mcp.WithString("custom_prompt", mcp.Description("Custom base prompt instead of built-in template. Will be enhanced through the pipeline.")),
			), s.handleSweepGenerate},
			{mcp.NewTool("ralphglasses_sweep_launch",
				mcp.WithDescription("Launch an enhanced prompt against multiple repos as parallel sessions. Returns a sweep_id for tracking all sessions as a group."),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("The prompt to run (use sweep_generate output or provide your own). Use REPO_PLACEHOLDER for repo name substitution.")),
				mcp.WithString("repos", mcp.Description("JSON array of repo names, or \"active\" for recently-active repos, or \"all\". Default: active")),
				mcp.WithNumber("limit", mcp.Description("Max repos to launch against (default 10)")),
				mcp.WithString("model", mcp.Description("Model to use: opus (default), sonnet, haiku")),
				mcp.WithString("permission_mode", mcp.Description("Claude permission mode: plan (default, read-only), auto, default")),
				mcp.WithString("enhance_prompt", mcp.Description("Enhance each repo's prompt before launch: local (default), llm, auto, none")),
				mcp.WithNumber("budget_usd", mcp.Description("Per-session budget in USD (default: 5.0)")),
				mcp.WithString("effort", mcp.Description("Effort level: low, medium, high, max")),
				mcp.WithString("allowed_tools", mcp.Description("Comma-separated allowed tools (default: read-only tools for plan mode)")),
			), s.handleSweepLaunch},
			{mcp.NewTool("ralphglasses_sweep_status",
				mcp.WithDescription("Dashboard for a sweep: per-repo status, total cost, completion percentage, stalled sessions, and optional output tails."),
				mcp.WithString("sweep_id", mcp.Required(), mcp.Description("Sweep ID returned by sweep_launch")),
				mcp.WithBoolean("verbose", mcp.Description("Include last output lines per session (default false)")),
			), s.handleSweepStatus},
			{mcp.NewTool("ralphglasses_sweep_nudge",
				mcp.WithDescription("Detect and restart stalled sessions in a sweep. Identifies sessions idle beyond threshold and restarts them with the same prompt."),
				mcp.WithString("sweep_id", mcp.Required(), mcp.Description("Sweep ID to nudge")),
				mcp.WithNumber("stale_threshold_min", mcp.Description("Minutes idle before a session is considered stalled (default 5)")),
				mcp.WithString("action", mcp.Description("Action for stalled sessions: restart (default), skip")),
			), s.handleSweepNudge},
			{mcp.NewTool("ralphglasses_sweep_schedule",
				mcp.WithDescription("Set up recurring status checks for a sweep at configurable intervals. Optionally auto-nudges stalled sessions. Returns a task_id for cancellation."),
				mcp.WithString("sweep_id", mcp.Required(), mcp.Description("Sweep ID to monitor")),
				mcp.WithNumber("interval_minutes", mcp.Description("Check interval in minutes (default 5)")),
				mcp.WithBoolean("auto_nudge", mcp.Description("Automatically restart stalled sessions (default false)")),
				mcp.WithNumber("max_checks", mcp.Description("Stop after N checks, 0 for unlimited (default 0)")),
			), s.handleSweepSchedule},
		},
	}
}
