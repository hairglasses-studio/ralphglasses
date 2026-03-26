package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// handleLoopAwait blocks until a session or loop reaches a terminal state,
// or until the configured timeout expires. This replaces the common
// "sleep && echo done" anti-pattern with proper polling.
func (s *Server) handleLoopAwait(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "id is required"), nil
	}
	awaitType := getStringArg(req, "type")
	if awaitType == "" {
		return codedError(ErrInvalidParams, "type is required (session or loop)"), nil
	}
	if awaitType != "session" && awaitType != "loop" {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid type %q: must be session or loop", awaitType)), nil
	}

	timeoutSec := getNumberArg(req, "timeout_seconds", 300)
	if timeoutSec <= 0 {
		timeoutSec = 300
	}
	pollSec := getNumberArg(req, "poll_interval_seconds", 10)
	if pollSec < 5 {
		pollSec = 5
	}

	timeout := time.Duration(timeoutSec) * time.Second
	pollInterval := time.Duration(pollSec) * time.Second

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	// Check immediately before first tick.
	if result, done := s.checkAwaitStatus(id, awaitType, start); done {
		return result, nil
	}

	for {
		select {
		case <-ctx.Done():
			// Timeout — return last known state.
			elapsed := time.Since(start).Seconds()
			lastState := s.collectStatus(id, awaitType)
			return jsonResult(map[string]any{
				"status":         "timeout",
				"elapsed_seconds": int(elapsed),
				"final_state":    lastState,
			}), nil
		case <-ticker.C:
			if result, done := s.checkAwaitStatus(id, awaitType, start); done {
				return result, nil
			}
		}
	}
}

// checkAwaitStatus checks if the target has reached a terminal state.
// Returns (result, true) if terminal, (nil, false) if still running.
func (s *Server) checkAwaitStatus(id, awaitType string, start time.Time) (*mcp.CallToolResult, bool) {
	elapsed := time.Since(start).Seconds()

	if awaitType == "session" {
		sess, ok := s.SessMgr.Get(id)
		if !ok {
			return jsonResult(map[string]any{
				"status":          "completed",
				"elapsed_seconds": int(elapsed),
				"final_state":     map[string]any{"error": "session not found", "id": id},
			}), true
		}
		sess.Lock()
		status := sess.Status
		state := map[string]any{
			"id":       sess.ID,
			"status":   string(status),
			"repo":     sess.RepoName,
			"spent_usd": sess.SpentUSD,
			"turns":    sess.TurnCount,
		}
		if sess.Error != "" {
			state["error"] = sess.Error
		}
		sess.Unlock()

		if isTerminalSessionStatus(status) {
			resultStatus := "completed"
			if status == session.StatusErrored {
				resultStatus = "failed"
			}
			return jsonResult(map[string]any{
				"status":          resultStatus,
				"elapsed_seconds": int(elapsed),
				"final_state":     state,
			}), true
		}
		return nil, false
	}

	// Loop type.
	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return jsonResult(map[string]any{
			"status":          "completed",
			"elapsed_seconds": int(elapsed),
			"final_state":     map[string]any{"error": "loop not found", "id": id},
		}), true
	}
	run.Lock()
	loopStatus := run.Status
	state := map[string]any{
		"id":         run.ID,
		"status":     loopStatus,
		"repo":       run.RepoName,
		"iterations": len(run.Iterations),
		"last_error": run.LastError,
	}
	run.Unlock()

	if isTerminalLoopStatus(loopStatus) {
		resultStatus := "completed"
		if loopStatus == "failed" {
			resultStatus = "failed"
		}
		return jsonResult(map[string]any{
			"status":          resultStatus,
			"elapsed_seconds": int(elapsed),
			"final_state":     state,
		}), true
	}
	return nil, false
}

// handleLoopPoll performs a single non-blocking status check for a session
// or loop, returning the current state immediately.
func (s *Server) handleLoopPoll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "id is required"), nil
	}
	awaitType := getStringArg(req, "type")
	if awaitType == "" {
		return codedError(ErrInvalidParams, "type is required (session or loop)"), nil
	}
	if awaitType != "session" && awaitType != "loop" {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid type %q: must be session or loop", awaitType)), nil
	}

	state := s.collectStatus(id, awaitType)
	return jsonResult(state), nil
}

// collectStatus gathers current status for either a session or loop.
func (s *Server) collectStatus(id, awaitType string) map[string]any {
	if awaitType == "session" {
		sess, ok := s.SessMgr.Get(id)
		if !ok {
			return map[string]any{
				"id":     id,
				"type":   "session",
				"status": "not_found",
			}
		}
		sess.Lock()
		state := map[string]any{
			"id":            sess.ID,
			"type":          "session",
			"status":        string(sess.Status),
			"repo":          sess.RepoName,
			"provider":      string(sess.Provider),
			"spent_usd":     sess.SpentUSD,
			"turns":         sess.TurnCount,
			"last_activity": sess.LastActivity.Format(time.RFC3339),
		}
		if sess.Error != "" {
			state["error"] = sess.Error
		}
		sess.Unlock()
		return state
	}

	// Loop type.
	run, ok := s.SessMgr.GetLoop(id)
	if !ok {
		return map[string]any{
			"id":     id,
			"type":   "loop",
			"status": "not_found",
		}
	}
	run.Lock()
	state := map[string]any{
		"id":         run.ID,
		"type":       "loop",
		"status":     run.Status,
		"repo":       run.RepoName,
		"iterations": len(run.Iterations),
		"last_error": run.LastError,
		"updated_at": run.UpdatedAt.Format(time.RFC3339),
	}
	run.Unlock()
	return state
}

// isTerminalSessionStatus returns true for session statuses that indicate
// the session will not produce further output.
func isTerminalSessionStatus(s session.SessionStatus) bool {
	switch s {
	case session.StatusCompleted, session.StatusStopped, session.StatusErrored:
		return true
	default:
		return false
	}
}

// isTerminalLoopStatus returns true for loop statuses that indicate
// the loop has finished executing.
func isTerminalLoopStatus(s string) bool {
	switch s {
	case "completed", "stopped", "failed", "idle", "converged":
		return true
	default:
		return false
	}
}
