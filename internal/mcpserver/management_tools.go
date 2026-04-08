package mcpserver

import (
	"context"

	"github.com/mark3labs/mcp-go/mcp"
)

func managementToolDefinitions() []mcp.Tool {
	return []mcp.Tool{
		mcp.NewTool("ralphglasses_tool_groups",
			mcp.WithDescription("List available tool groups for deferred loading. Call ralphglasses_load_tool_group to load a specific group."),
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
