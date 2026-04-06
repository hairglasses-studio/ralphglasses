package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

// defaultRegistry returns a ToolGroupRegistry populated with all standard
// tool group builders in the canonical ordering. Each builder wraps the
// corresponding build*Group method on Server.
func defaultRegistry() *ToolGroupRegistry {
	r := NewToolGroupRegistry()
	r.Register(NewFuncBuilder("core", (*Server).buildCoreGroup))
	r.Register(NewFuncBuilder("session", (*Server).buildSessionGroup))
	r.Register(NewFuncBuilder("loop", (*Server).buildLoopGroup))
	r.Register(NewFuncBuilder("prompt", (*Server).buildPromptGroup))
	r.Register(NewFuncBuilder("fleet", (*Server).buildFleetGroup))
	r.Register(NewFuncBuilder("repo", (*Server).buildRepoGroup))
	r.Register(NewFuncBuilder("roadmap", (*Server).buildRoadmapGroup))
	r.Register(NewFuncBuilder("team", (*Server).buildTeamGroup))
	r.Register(NewFuncBuilder("awesome", (*Server).buildAwesomeGroup))
	r.Register(NewFuncBuilder("advanced", (*Server).buildAdvancedGroup))
	r.Register(NewFuncBuilder("events", (*Server).buildEventsGroup))
	r.Register(NewFuncBuilder("feedback", (*Server).buildFeedbackGroup))
	r.Register(NewFuncBuilder("eval", (*Server).buildEvalGroup))
	r.Register(NewFuncBuilder("fleet_h", (*Server).buildFleetHGroup))
	r.Register(NewFuncBuilder("observability", (*Server).buildObservabilityGroup))
	r.Register(NewFuncBuilder("rdcycle", (*Server).buildRdcycleGroup))
	r.Register(NewFuncBuilder("plugin", (*Server).buildPluginGroup))
	r.Register(NewFuncBuilder("sweep", (*Server).buildSweepGroup))
	r.Register(NewFuncBuilder("rc", (*Server).buildRCGroup))
	r.Register(NewFuncBuilder("autonomy", (*Server).buildAutonomyGroup))
	r.Register(NewFuncBuilder("workflow", (*Server).buildWorkflowGroup))
	r.Register(NewFuncBuilder("docs", (*Server).buildDocsGroup))
	r.Register(NewFuncBuilder("recovery", (*Server).buildRecoveryGroup))
	r.Register(NewFuncBuilder("promptdj", (*Server).buildPromptDJGroup))
	r.Register(NewFuncBuilder("a2a", (*Server).buildA2AGroup))
	r.Register(NewFuncBuilder("trigger", (*Server).buildTriggerGroup))
	return r
}

// buildToolGroups constructs all tool groups with their tool definitions and handlers.
// It delegates to the default registry, preserving the canonical ordering.
func (s *Server) buildToolGroups() []ToolGroup {
	return defaultRegistry().BuildAllOrdered(s)
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
				mcp.WithBoolean("include_config", mcp.Description("Include full config in status response")),
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
				mcp.WithDescription("Pause or resume a running ralph loop for a `repo`"),
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
				mcp.WithDescription("Get/set .ralphrc `key` values across multiple repos"),
				mcp.WithString("key", mcp.Required(), mcp.Description("Config key to get/set")),
				mcp.WithString("value", mcp.Description("Value to set (omit to query)")),
				mcp.WithString("repos", mcp.Description("Comma-separated repo names (default: all)")),
			), s.handleConfigBulk},
			{mcp.NewTool("ralphglasses_tasks_get",
				mcp.WithDescription("Get status of an async task by `task_id` — poll for long-running operations (loop_start, fleet_submit, self_improve)"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID returned from async tool invocation")),
			), s.handleTasksGet},
			{mcp.NewTool("ralphglasses_tasks_list",
				mcp.WithDescription("List all async tasks with optional state filter"),
				mcp.WithString("state", mcp.Description("Filter by state: running, completed, failed, canceled, input_required")),
			), s.handleTasksList},
			{mcp.NewTool("ralphglasses_tasks_cancel",
				mcp.WithDescription("Cancel a running async task by `task_id`"),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to cancel")),
			), s.handleTasksCancel},
		},
	}
}

func (s *Server) buildPluginGroup() ToolGroup {
	return ToolGroup{
		Name:        "plugin",
		Description: "Plugin management: list, info, enable, disable registered plugins",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_plugin_list",
				mcp.WithDescription("List all registered plugins with name, version, status, and type (builtin/yaml/grpc)"),
			), s.handlePluginList},
			{mcp.NewTool("ralphglasses_plugin_info",
				mcp.WithDescription("Show detailed information for a specific plugin"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name")),
			), s.handlePluginInfo},
			{mcp.NewTool("ralphglasses_plugin_enable",
				mcp.WithDescription("Enable a disabled plugin"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to enable")),
			), s.handlePluginEnable},
			{mcp.NewTool("ralphglasses_plugin_disable",
				mcp.WithDescription("Disable an active plugin"),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to disable")),
			), s.handlePluginDisable},
		},
	}
}
