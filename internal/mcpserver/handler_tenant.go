package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleTenantList(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tenants, err := s.SessMgr.ListTenants(ctx)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("list tenants: %v", err)), nil
	}
	return jsonResult(tenants), nil
}

func (s *Server) handleTenantCreate(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	id, errResult := p.RequireString("tenant_id")
	if errResult != nil {
		return errResult, nil
	}
	tenant := &session.Tenant{
		ID:           id,
		DisplayName:  p.OptionalString("display_name", ""),
		BudgetCapUSD: p.OptionalNumber("budget_cap_usd", 0),
	}
	if roots := splitCSV(p.OptionalString("allowed_repo_roots", "")); len(roots) > 0 {
		tenant.AllowedRepoRoots = roots
	}
	saved, err := s.SessMgr.SaveTenant(ctx, tenant)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("save tenant: %v", err)), nil
	}
	return jsonResult(saved), nil
}

func (s *Server) handleTenantStatus(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	tenant, err := s.SessMgr.GetTenant(ctx, id)
	if err != nil {
		if err == session.ErrTenantNotFound {
			return codedError(ErrInvalidParams, fmt.Sprintf("tenant not found: %s", id)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("get tenant: %v", err)), nil
	}
	totalSpend := 0.0
	if store := s.SessMgr.Store(); store != nil {
		totalSpend, _ = store.AggregateSpend(ctx, id, "")
	}
	return jsonResult(map[string]any{
		"tenant":          tenant,
		"sessions_active": len(s.SessMgr.ListByTenant("", id)),
		"teams":           len(s.SessMgr.ListTeamsForTenant(id)),
		"total_spend_usd": totalSpend,
	}), nil
}

func (s *Server) handleTenantRotateTriggerToken(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	token, tenant, err := s.SessMgr.RotateTenantTriggerToken(ctx, id)
	if err != nil {
		if err == session.ErrTenantNotFound {
			return codedError(ErrInvalidParams, fmt.Sprintf("tenant not found: %s", id)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("rotate trigger token: %v", err)), nil
	}
	return jsonResult(map[string]any{
		"tenant_id":     tenant.ID,
		"trigger_token": token,
	}), nil
}
