package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildApprovalGroup() ToolGroup {
	return ToolGroup{
		Name:        "approval",
		Description: "Human-in-the-loop approval: request, resolve, list pending approvals (Factor 7: Contact Humans with Tool Calls)",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_request_approval",
				mcp.WithDescription(descriptions.DescRalphglassesRequestApproval),
				mcp.WithString("action", mcp.Required(), mcp.Description("What needs approval (e.g. 'merge PR #42', 'deploy to prod')")),
				mcp.WithString("context", mcp.Required(), mcp.Description("Why this needs approval — background and rationale")),
				mcp.WithString("urgency", mcp.Required(), mcp.Description("Urgency level: low, normal, high, critical")),
				mcp.WithString("session_id", mcp.Description("Session ID to pause while awaiting approval (omit to skip pause)")),
			), s.handleRequestApproval},
			{mcp.NewTool("ralphglasses_resolve_approval",
				mcp.WithDescription(descriptions.DescRalphglassesResolveApproval),
				mcp.WithString("approval_id", mcp.Required(), mcp.Description("Approval ID returned from request_approval")),
				mcp.WithString("decision", mcp.Required(), mcp.Description("Decision: approved or rejected")),
				mcp.WithString("reason", mcp.Description("Reason for the decision")),
			), s.handleResolveApproval},
			{mcp.NewTool("ralphglasses_list_approvals",
				mcp.WithDescription(descriptions.DescRalphglassesListApprovals),
				mcp.WithBoolean("include_resolved", mcp.Description("Include already-resolved approvals (default: false)")),
			), s.handleListApprovals},
		},
	}
}
