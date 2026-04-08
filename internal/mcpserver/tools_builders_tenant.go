package mcpserver

import (
	"github.com/hairglasses-studio/ralphglasses/internal/mcpserver/descriptions"
	"github.com/mark3labs/mcp-go/mcp"
)

func (s *Server) buildTenantGroup() ToolGroup {
	return ToolGroup{
		Name:        "tenant",
		Description: "Workspace tenant administration: list, create, status, rotate trigger token, and batch role leaderboards",
		Tools: []ToolEntry{
			{mcp.NewTool("ralphglasses_tenant_list",
				mcp.WithDescription(descriptions.DescRalphglassesTenantList),
			), s.handleTenantList},
			{mcp.NewTool("ralphglasses_tenant_create",
				mcp.WithDescription(descriptions.DescRalphglassesTenantCreate),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
				mcp.WithString("display_name", mcp.Description("Display name")),
				mcp.WithString("allowed_repo_roots", mcp.Description("Comma-separated allowed repo roots")),
				mcp.WithNumber("budget_cap_usd", mcp.Description("Budget cap in USD")),
			), s.handleTenantCreate},
			{mcp.NewTool("ralphglasses_tenant_status",
				mcp.WithDescription(descriptions.DescRalphglassesTenantStatus),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
			), s.handleTenantStatus},
			{mcp.NewTool("ralphglasses_tenant_rotate_trigger_token",
				mcp.WithDescription(descriptions.DescRalphglassesTenantRotateTriggerToken),
				mcp.WithString("tenant_id", mcp.Required(), mcp.Description("Tenant ID")),
			), s.handleTenantRotateTriggerToken},
			{mcp.NewTool("ralphglasses_tenant_role_leaderboards",
				mcp.WithDescription(descriptions.DescRalphglassesTenantRoleLeaderboards),
				mcp.WithString("tenant_id", mcp.Description("Optional tenant ID; omit to return all tenants")),
				mcp.WithNumber("limit", mcp.Description("Maximum role entries per tenant (default: 10)")),
				mcp.WithBoolean("include_ended", mcp.Description("Include persisted ended sessions (default: true)")),
			), s.handleTenantRoleLeaderboards},
		},
	}
}
