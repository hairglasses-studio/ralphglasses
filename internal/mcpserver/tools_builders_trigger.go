package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildTriggerGroup() ToolGroup {
	return ToolGroup{
		Name:        "trigger",
		Description: "External agent triggering and cron-based scheduling",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_trigger_webhook",
				mcp.WithDescription(descriptions.DescRalphglassesTriggerWebhook),
				mcp.WithString("prompt", mcp.Required(), mcp.Description("Task prompt for the agent session")),
				mcp.WithString("tenant_id", mcp.Description("Workspace tenant ID (default: _default)")),
				mcp.WithString("agent_type", mcp.Required(), mcp.Description("Agent type to trigger"),
					mcp.Enum("ralph", "loop", "cycle"),
				),
				mcp.WithString("provider", mcp.Description("Cycle provider override: claude, codex, gemini, or auto/omit for runtime selection")),
				mcp.WithNumber("priority", mcp.Description("Priority 1-10, higher = more urgent (default: 5)")),
				mcp.WithString("model", mcp.Description("Model override for the session")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget in USD for the session")),
				mcp.WithNumber("max_turns", mcp.Description("Maximum conversation turns")),
				mcp.WithBoolean("launch", mcp.Description("Launch the session immediately (default: false, just queue)")),
				mcp.WithString("repo", mcp.Description("Repo name (required when launch=true)")),
			), s.handleTriggerWebhook},
			{mcp.NewTool("ralphglasses_schedule_create",
				mcp.WithDescription(descriptions.DescRalphglassesScheduleCreate),
				mcp.WithString("action", mcp.Description("Action: create (default), list, enable, disable")),
				mcp.WithString("repo", mcp.Description("Repo name or path for canonical repo-local schedules")),
				mcp.WithString("prompt", mcp.Description("Task prompt (required for create)")),
				mcp.WithString("cron_expression", mcp.Description("Cron expression e.g. '0 */6 * * *' (required for create)")),
				mcp.WithString("agent_type", mcp.Description("Agent type: ralph (default), loop, cycle")),
				mcp.WithBoolean("enabled", mcp.Description("Whether the schedule is active (default: true)")),
				mcp.WithString("provider", mcp.Description("Provider override for repo-local schedules")),
				mcp.WithString("model", mcp.Description("Model override for repo-local schedules")),
				mcp.WithNumber("budget_usd", mcp.Description("Budget override for repo-local schedules")),
				mcp.WithNumber("max_turns", mcp.Description("Max turns override for repo-local schedules")),
				mcp.WithString("name", mcp.Description("Cycle name for cycle schedules")),
				mcp.WithString("objective", mcp.Description("Cycle objective override for cycle schedules")),
				mcp.WithString("criteria", mcp.Description("Comma-separated cycle success criteria")),
				mcp.WithNumber("max_tasks", mcp.Description("Max planned tasks for cycle schedules")),
				mcp.WithNumber("priority", mcp.Description("Queue priority for repo-local schedules")),
				mcp.WithString("id", mcp.Description("Schedule ID (required for enable/disable)")),
			), s.handleScheduleCreate},
		},
	}
}
