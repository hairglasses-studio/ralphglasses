package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildA2AGroup() ToolGroup {
	return ToolGroup{
		Name:        "a2a",
		Description: "A2A protocol integration: discover agents, send tasks, check status, export agent card",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_a2a_discover",
				mcp.WithDescription("Discover an A2A agent by fetching its agent card from /.well-known/agent.json. Returns skills, capabilities, and metadata."),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
			), s.handleA2ADiscover},
			{mcp.NewTool("ralphglasses_a2a_send",
				mcp.WithDescription("Send a task to a remote A2A agent. Returns task ID and initial state."),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
				mcp.WithString("message", mcp.Required(), mcp.Description("Task message to send")),
				mcp.WithString("task_id", mcp.Description("Optional task ID (auto-generated if empty)")),
			), s.handleA2ASend},
			{mcp.NewTool("ralphglasses_a2a_status",
				mcp.WithDescription("Check the status of a previously sent A2A task."),
				mcp.WithString("url", mcp.Required(), mcp.Description("Base URL of the A2A agent")),
				mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID to check")),
			), s.handleA2AStatus},
			{mcp.NewTool("ralphglasses_a2a_agent_card",
				mcp.WithDescription("Generate and return this server's A2A agent card (our capabilities as an A2A agent)."),
			), s.handleA2AAgentCard},
		},
	}
}
