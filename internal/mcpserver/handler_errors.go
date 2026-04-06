package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleErrorContext returns the error context for a session, including
// consecutive error count, escalation status, and formatted recent errors
// suitable for LLM context injection (12-Factor Agents Factor 9).
func (s *Server) handleErrorContext(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	if err := pp.Required("session_id"); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	sessionID := pp.String("session_id")

	// Verify the session exists.
	if _, ok := s.SessMgr.Get(sessionID); !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %q not found", sessionID)), nil
	}

	ec := s.SessMgr.GetErrorContext(sessionID)

	type response struct {
		SessionID        string `json:"session_id"`
		ConsecutiveErrors int   `json:"consecutive_errors"`
		ShouldEscalate   bool   `json:"should_escalate"`
		TotalErrors      int    `json:"total_errors"`
		RecentErrors     string `json:"recent_errors"`
	}

	resp := response{
		SessionID:        sessionID,
		ConsecutiveErrors: ec.ConsecutiveErrors(),
		ShouldEscalate:   ec.ShouldEscalate(),
		TotalErrors:      ec.TotalErrors(),
		RecentErrors:     ec.FormatForContext(),
	}

	data, _ := json.Marshal(resp)
	return &mcp.CallToolResult{
		Content: []mcp.Content{mcp.TextContent{
			Type: "text",
			Text: string(data),
		}},
	}, nil
}
