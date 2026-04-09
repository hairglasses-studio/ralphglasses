package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func managementToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ralphglasses_tool_groups",
			mcp.WithDescription("List available tool groups for deferred loading, or search the live workflow and skill catalog when query/include flags are provided."),
			mcp.WithString("query", mcp.Description("Optional search query across tool groups, workflow names, skill names, descriptions, and key tools")),
			mcp.WithString("tool_group", mcp.Description("Optional tool-group filter (for example \"repo\", \"fleet\", or \"management\")")),
			mcp.WithBoolean("include_workflows", mcp.Description("Include matching workflow catalog entries in the response")),
			mcp.WithBoolean("include_skills", mcp.Description("Include matching skill catalog entries in the response")),
			mcp.WithNumber("limit", mcp.Description("Optional per-section result limit for filtered search responses")),
		),
		mcp.NewTool("ralphglasses_load_tool_group",
			mcp.WithDescription(loadToolGroupDescription()),
			mcp.WithString("group", mcp.Required(), mcp.Description("Tool group name to load")),
		),
		mcp.NewTool("ralphglasses_skill_export",
			mcp.WithDescription("Generate SKILL.md documentation from all registered tool groups. Returns markdown or JSON."),
			mcp.WithString("format", mcp.Description("Output format: \"markdown\" (default) or \"json\"")),
			mcp.WithString("group", mcp.Description("Filter to a specific tool group (for example \"core\", \"session\", or \"management\")")),
		),
		mcp.NewTool("ralphglasses_server_health",
			mcp.WithDescription("Show the active ralphglasses MCP contract shape, including available tool groups, loaded groups, and resource/prompt coverage."),
		),
		mcp.NewTool("ralphglasses_autobuild_ledger_append",
			mcp.WithDescription("Append an entry to the machine-readable autobuild execution ledger and emit telemetry."),
			mcp.WithString("patch_id", mcp.Required(), mcp.Description("Unique identifier for the autobuild tranche")),
			mcp.WithString("status", mcp.Required(), mcp.Description("Current status: \"planned\", \"in_progress\", \"completed\", \"blocked\", \"cancelled\", \"deferred\"")),
			mcp.WithString("trigger_type", mcp.Description("Signal type: \"adoption\", \"integrity\", \"ci\", \"manual\", \"other\"")),
			mcp.WithString("trigger_source", mcp.Description("Signal source resource or path")),
			mcp.WithString("trigger_summary", mcp.Description("Why this tranche was opened")),
			mcp.WithBoolean("remote_main_verified", mcp.Description("Whether the trigger signal was verified against remote main")),
			mcp.WithString("recommended_entry_surface", mcp.Description("Resource, doc, command, or tool to start with")),
			mcp.WithString("changes", mcp.Description("Comma-separated list of changes applied")),
			mcp.WithString("acceptance_condition", mcp.Description("Comma-separated list of conditions for completion")),
			mcp.WithString("stop_condition", mcp.Description("Comma-separated list of boundaries for the tranche")),
			mcp.WithString("repo_owned_scope", mcp.Description("Comma-separated list of items in scope")),
			mcp.WithString("closure_state", mcp.Description("Final state: \"completed\", \"blocked\", \"cancelled\", \"deferred\"")),
			mcp.WithString("closure_summary", mcp.Description("High-signal outcome summary")),
			mcp.WithString("prevented_failure_class", mcp.Description("Comma-separated failure classes prevented")),
			mcp.WithString("next_recommended_patch", mcp.Description("ID of the next recommended patch")),
		),
	}
}

func managementToolNames() []string {
	defs := managementToolDefinitions()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	return names
}

// ManagementTools returns the always-available management/discovery tools that
// are registered ahead of deferred group loading.
func (s *Server) ManagementTools() []ToolEntry {
	defs := managementToolDefinitions()
	entries := make([]ToolEntry, 0, len(defs))
	for _, def := range defs {
		var handler func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error)
		switch def.Name {
		case "ralphglasses_tool_groups":
			handler = s.handleToolGroups
		case "ralphglasses_load_tool_group":
			handler = s.handleLoadToolGroup
		case "ralphglasses_skill_export":
			handler = s.handleSkillExport
		case "ralphglasses_server_health":
			handler = s.handleServerHealth
		case "ralphglasses_autobuild_ledger_append":
			handler = s.handleAutobuildLedgerAppend
		default:
			continue
		}
		entries = append(entries, ToolEntry{
			Tool:    def,
			Handler: handler,
		})
	}
	return entries
}
