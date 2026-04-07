package mcpserver

import "github.com/mark3labs/mcp-go/mcp"

func (s *Server) buildTenantGroup() ToolGroup {
	return ToolGroup{
		Name:        "tenant",
		Description: "Workspace tenant administration: list, create, status, rotate trigger token",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_tenant_list",
				mcp.WithDescription("List all configured workspace tenants"),
			), s.handleTenantList},
			{mcp.NewTool("ralphglasses_tenant_create",
				mcp.WithDescription("Create or update a workspace tenant"),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
				mcp.WithString("display_name", mcp.Description("Display name")),
				mcp.WithString("allowed_repo_roots", mcp.Description("Comma-separated allowed repo roots")),
				mcp.WithNumber("budget_cap_usd", mcp.Description("Budget cap in USD")),
			), s.handleTenantCreate},
			{mcp.NewTool("ralphglasses_tenant_status",
				mcp.WithDescription("Get tenant details plus current active session/team counts"),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
			), s.handleTenantStatus},
			{mcp.NewTool("ralphglasses_tenant_rotate_trigger_token",
				mcp.WithDescription("Rotate the bearer token used by the trigger HTTP API for one tenant"),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
			), s.handleTenantRotateTriggerToken},
		},
	}
}
