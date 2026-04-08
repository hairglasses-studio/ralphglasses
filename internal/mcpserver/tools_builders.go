package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
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
	r.Register(NewFuncBuilder("tenant", (*Server).buildTenantGroup))
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
	r.Register(NewFuncBuilder("approval", (*Server).buildApprovalGroup))
	r.Register(NewFuncBuilder("context", (*Server).buildContextGroup))
	r.Register(NewFuncBuilder("prefetch", (*Server).buildPrefetchGroup))
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
				mcp.WithDescription(descriptions.DescRalphglassesScan),
			), s.handleScan},
			{mcp.NewTool("ralphglasses_list",
				mcp.WithDescription(descriptions.DescRalphglassesList),
			), s.handleList},
			{mcp.NewTool("ralphglasses_status",
				mcp.WithDescription(descriptions.DescRalphglassesStatus),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name (basename of directory)")),
				mcp.WithBoolean("include_config", mcp.Description("Include full config in status response")),
			), s.handleStatus},
			{mcp.NewTool("ralphglasses_start",
				mcp.WithDescription(descriptions.DescRalphglassesStart),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to start loop for")),
			), s.handleStart},
			{mcp.NewTool("ralphglasses_stop",
				mcp.WithDescription(descriptions.DescRalphglassesStop),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to stop loop for")),
			), s.handleStop},
			{mcp.NewTool("ralphglasses_stop_all",
				mcp.WithDescription(descriptions.DescRalphglassesStopAll),
			), s.handleStopAll},
			{mcp.NewTool("ralphglasses_pause",
				mcp.WithDescription(descriptions.DescRalphglassesPause),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name to pause/resume")),
			), s.handlePause},
			{mcp.NewTool("ralphglasses_logs",
				mcp.WithDescription(descriptions.DescRalphglassesLogs),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithNumber("lines", mcp.Description("Number of lines to return (default 50, max 500)")),
			), s.handleLogs},
			{mcp.NewTool("ralphglasses_config",
				mcp.WithDescription(descriptions.DescRalphglassesConfig),
				mcp.WithString("repo", mcp.Required(), mcp.Description("Repo name")),
				mcp.WithString("key", mcp.Description("Config key to get/set (omit to list all)")),
				mcp.WithString("value", mcp.Description("Value to set (omit to get current value)")),
			), s.handleConfig},
			{mcp.NewTool("ralphglasses_config_bulk",
				mcp.WithDescription(descriptions.DescRalphglassesConfigBulk),
				mcp.WithString("key", mcp.Required(), mcp.Description("Config key to get/set")),
				mcp.WithString("value", mcp.Description("Value to set (omit to query)")),
				mcp.WithString("repos", mcp.Description("Comma-separated repo names (default: all)")),
			), s.handleConfigBulk},
			{mcp.NewTool("ralphglasses_doctor",
				mcp.WithDescription(descriptions.DescRalphglassesDoctor),
				mcp.WithString("scan_path", mcp.Description("Override scan root to inspect (defaults to the server scan path)")),
				mcp.WithString("checks", mcp.Description("Comma-separated check names to run (e.g. git,scan_path,api_keys)")),
				mcp.WithBoolean("include_optional", mcp.Description("Include optional provider/API key checks (default: true)")),
			), s.handleDoctor},
			{mcp.NewTool("ralphglasses_validate",
				mcp.WithDescription(descriptions.DescRalphglassesValidate),
				mcp.WithString("scan_path", mcp.Description("Override scan root to validate (defaults to the server scan path)")),
				mcp.WithString("repo", mcp.Description("Single repo name to validate")),
				mcp.WithString("repos", mcp.Description("Comma-separated repo names to validate")),
				mcp.WithBoolean("include_clean", mcp.Description("Include OK repos in the response (default: false)")),
				mcp.WithBoolean("strict", mcp.Description("Treat warnings as errors (default: false)")),
			), s.handleValidate},
			{mcp.NewTool("ralphglasses_config_schema",
				mcp.WithDescription(descriptions.DescRalphglassesConfigSchema),
				mcp.WithString("key", mcp.Description("Optional single key to inspect")),
				mcp.WithBoolean("include_defaults", mcp.Description("Include default metadata when available")),
				mcp.WithBoolean("include_constraints", mcp.Description("Include rendered constraints (default: true)")),
			), s.handleConfigSchema},
			{mcp.NewTool("ralphglasses_debug_bundle",
				mcp.WithDescription(descriptions.DescRalphglassesDebugBundle),
				mcp.WithString("action", mcp.Description("Action: view (default) or save")),
				mcp.WithString("repo", mcp.Description("Optional repo name whose root should anchor the bundle save path")),
				mcp.WithString("sections", mcp.Description("Comma-separated bundle sections to include")),
				mcp.WithString("name", mcp.Description("Optional output filename when action=save")),
			), s.handleDebugBundle},
			{mcp.NewTool("ralphglasses_theme_export",
				mcp.WithDescription(descriptions.DescRalphglassesThemeExport),
				mcp.WithString("format", mcp.Required(), mcp.Description("Export format: ghostty, starship, or k9s")),
				mcp.WithString("theme", mcp.Required(), mcp.Description("Theme name")),
			), s.handleThemeExport},
			{mcp.NewTool("ralphglasses_telemetry_export",
				mcp.WithDescription(descriptions.DescRalphglassesTelemetryExport),
				mcp.WithString("format", mcp.Description("Output format: json (default) or csv")),
				mcp.WithString("since", mcp.Description("RFC3339 or YYYY-MM-DD lower bound")),
				mcp.WithString("until", mcp.Description("RFC3339 or YYYY-MM-DD upper bound")),
				mcp.WithString("repo", mcp.Description("Optional repo filter")),
				mcp.WithString("provider", mcp.Description("Optional provider filter")),
				mcp.WithString("type", mcp.Description("Optional telemetry event type filter")),
				mcp.WithNumber("limit", mcp.Description("Maximum events to return")),
			), s.handleTelemetryExport},
			{mcp.NewTool("ralphglasses_firstboot_profile",
				mcp.WithDescription(descriptions.DescRalphglassesFirstbootProfile),
				mcp.WithString("action", mcp.Description("Action: get (default), set, validate, or mark_done")),
				mcp.WithString("config_dir", mcp.Description("Optional config directory override (defaults to "+ralphpath.ConfigDirDefaultDescription()+")")),
				mcp.WithString("hostname", mcp.Description("Hostname to persist or validate")),
				mcp.WithNumber("autonomy_level", mcp.Description("Autonomy level 0-3")),
				mcp.WithString("coordinator_url", mcp.Description("Fleet coordinator URL")),
				mcp.WithString("anthropic_api_key", mcp.Description("Anthropic API key override")),
				mcp.WithString("google_api_key", mcp.Description("Google API key override")),
				mcp.WithString("openai_api_key", mcp.Description("OpenAI API key override")),
			), s.handleFirstbootProfile},
			{mcp.NewTool("ralphglasses_tasks_get",
				mcp.WithDescription(descriptions.DescRalphglassesTasksGet),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID returned from async tool invocation")),
			), s.handleTasksGet},
			{mcp.NewTool("ralphglasses_tasks_list",
				mcp.WithDescription(descriptions.DescRalphglassesTasksList),
				mcp.WithString("state", mcp.Description("Filter by state: running, completed, failed, canceled, input_required")),
			), s.handleTasksList},
			{mcp.NewTool("ralphglasses_tasks_cancel",
				mcp.WithDescription(descriptions.DescRalphglassesTasksCancel),
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
				mcp.WithDescription(descriptions.DescRalphglassesPluginList),
			), s.handlePluginList},
			{mcp.NewTool("ralphglasses_plugin_info",
				mcp.WithDescription(descriptions.DescRalphglassesPluginInfo),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name")),
			), s.handlePluginInfo},
			{mcp.NewTool("ralphglasses_plugin_enable",
				mcp.WithDescription(descriptions.DescRalphglassesPluginEnable),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to enable")),
			), s.handlePluginEnable},
			{mcp.NewTool("ralphglasses_plugin_disable",
				mcp.WithDescription(descriptions.DescRalphglassesPluginDisable),
				mcp.WithString("name", mcp.Required(), mcp.Description("Plugin name to disable")),
			), s.handlePluginDisable},
		},
	}
}
