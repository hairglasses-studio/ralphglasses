package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildA2AGroup() ToolGroup {
	return ToolGroup{
		Name:        "a2a",
		Description: "A2A protocol integration: discover agents, send tasks, check status, export agent card",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_a2a_discover",
				mcp.WithDescription(descriptions.DescRalphglassesA2aDiscover),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
			), s.handleA2ADiscover},
			{mcp.NewTool("ralphglasses_a2a_send",
				mcp.WithDescription(descriptions.DescRalphglassesA2aSend),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
				mcp.WithString("message", mcp.Required(), mcp.Description("Task message to send")),
				mcp.WithString("task_id", mcp.Description("Optional task ID (auto-generated if empty)")),
			), s.handleA2ASend},
			{mcp.NewTool("ralphglasses_a2a_status",
				mcp.WithDescription(descriptions.DescRalphglassesA2aStatus),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to check")),
			), s.handleA2AStatus},
			{mcp.NewTool("ralphglasses_a2a_agent_card",
				mcp.WithDescription(descriptions.DescRalphglassesA2aAgentCard),
			), s.handleA2AAgentCard},
		},
	}
}
