package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// handleContextBudget returns context budget status for a single session (by id)
// or for all sessions when session_id is omitted.
func (s *Server) handleContextBudget(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "session_id")

	if id != "" {
		return s.contextBudgetForSession(id)
	}

	return s.contextBudgetForAll()
}

func (s *Server) contextBudgetForSession(id string) (*mcp.CallToolResult, error) {
	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found", id)), nil
	}

	return jsonResult(contextBudgetSummary(sess)), nil
}

func (s *Server) contextBudgetForAll() (*mcp.CallToolResult, error) {
	sessions := s.SessMgr.List("")
	if len(sessions) == 0 {
		return emptyResult("context_budgets"), nil
	}

	summaries := make([]map[string]any, 0, len(sessions))
	for _, sess := range sessions {
		summaries = append(summaries, contextBudgetSummary(sess))
	}

	return jsonResult(summaries), nil
}

func contextBudgetSummary(sess *session.Session) map[string]any {
	sess.Lock()
	provider := string(sess.Provider)
	model := sess.Model
	sessID := sess.ID
	budget := sess.CtxBudget
	sess.Unlock()

	if budget == nil {
		limit := session.ModelLimitForProvider(session.Provider(provider))
		return map[string]any{
			"session_id":  sessID,
			"model":       model,
			"provider":    provider,
			"used_tokens": 0,
			"limit":       limit,
			"percent":     0.0,
			"status":      "ok",
		}
	}

	used, limit, pct := budget.Usage()
	return map[string]any{
		"session_id":  sessID,
		"model":       model,
		"provider":    provider,
		"used_tokens": used,
		"limit":       limit,
		"percent":     pct,
		"status":      budget.Status(),
	}
}
