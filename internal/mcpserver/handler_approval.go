package mcpserver

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// handleRequestApproval creates a pending approval record and optionally pauses
// the associated session. This implements Factor 7 (Contact Humans with Tool
// Calls) from the 12-factor-agents pattern.
func (s *Server) handleRequestApproval(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	action, errResult := p.RequireString("action")
	if errResult != nil {
		return errResult, nil
	}

	ctx, errResult := p.RequireString("context")
	if errResult != nil {
		return errResult, nil
	}

	urgency, errResult := p.RequireEnum("urgency", []string{"low", "normal", "high", "critical"})
	if errResult != nil {
		return errResult, nil
	}

	sessionID := p.OptionalString("session_id", "")

	store := s.getApprovalStore()
	rec := store.Create(action, ctx, urgency, sessionID)

	// If a session_id was provided, pause that session so it waits for the
	// human decision before continuing.
	paused := false
	if sessionID != "" {
		if sess, ok := s.SessMgr.Get(sessionID); ok {
			sess.Lock()
			sess.Status = "paused"
			sess.Unlock()
			paused = true
		}
	}

	result := map[string]any{
		"approval_id": rec.ID,
		"action":      rec.Action,
		"urgency":     rec.Urgency,
		"status":      string(rec.Status),
		"created_at":  rec.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if sessionID != "" {
		result["session_id"] = sessionID
		result["session_paused"] = paused
	}

	return jsonResult(result), nil
}

// handleResolveApproval resolves a pending approval and optionally resumes the
// session that was paused when the approval was requested.
func (s *Server) handleResolveApproval(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	approvalID, errResult := p.RequireString("approval_id")
	if errResult != nil {
		return errResult, nil
	}

	decision, errResult := p.RequireEnum("decision", []string{"approved", "rejected"})
	if errResult != nil {
		return errResult, nil
	}

	reason := p.OptionalString("reason", "")

	store := s.getApprovalStore()
	rec, err := store.Resolve(approvalID, decision, reason)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("resolve failed: %v", err)), nil
	}

	// If the original request had a session_id and the session is paused,
	// set it back to running so the agent can continue.
	resumed := false
	if rec.SessionID != "" {
		if sess, ok := s.SessMgr.Get(rec.SessionID); ok {
			sess.Lock()
			if sess.Status == "paused" {
				sess.Status = "running"
				resumed = true
			}
			sess.Unlock()
		}
	}

	result := map[string]any{
		"approval_id": rec.ID,
		"action":      rec.Action,
		"decision":    rec.Decision,
		"reason":      rec.Reason,
		"status":      string(rec.Status),
		"resolved_at": rec.ResolvedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
	if rec.SessionID != "" {
		result["session_id"] = rec.SessionID
		result["session_resumed"] = resumed
	}

	return jsonResult(result), nil
}

// handleListApprovals returns all pending approval records (or all records if
// include_resolved is true).
func (s *Server) handleListApprovals(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)
	includeResolved := p.OptionalBool("include_resolved", false)

	store := s.getApprovalStore()
	var records []*ApprovalRecord
	if includeResolved {
		records = store.ListAll()
	} else {
		records = store.List()
	}

	if len(records) == 0 {
		return emptyResult("approval"), nil
	}

	return jsonResult(records), nil
}

// getApprovalStore returns the server's approval store, lazily creating one if
// it does not yet exist. This is safe for concurrent use.
func (s *Server) getApprovalStore() *ApprovalStore {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.approvalStore == nil {
		s.approvalStore = NewApprovalStore()
	}
	return s.approvalStore
}
