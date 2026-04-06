package mcpserver

import (
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildContextGroup() ToolGroup {
	return ToolGroup{
		Name:        "context",
		Description: "Context window budget monitoring: track token usage per session",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_context_budget",
				mcp.WithDescription("Get context window budget status for a session or all sessions. Returns used tokens, limit, utilization percent, and threshold status (ok/warning/critical)."),
				mcp.WithString("session_id", mcp.Description("Session ID (omit to return all sessions)")),
			), s.handleContextBudget},
		},
	}
}
