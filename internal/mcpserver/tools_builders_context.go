package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildContextGroup() ToolGroup {
	return ToolGroup{
		Name:        "context",
		Description: "Context window budget monitoring: track token usage per session",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_context_budget",
				mcp.WithDescription(descriptions.DescRalphglassesContextBudget),
				mcp.WithString("session_id", mcp.Description("Session ID (omit to return all sessions)")),
			), s.handleContextBudget},
		},
	}
}
