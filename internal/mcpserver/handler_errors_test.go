package mcpserver

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestErrorContext_MissingSessionID(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleErrorContext(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing session_id")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrInvalidParams) {
		t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
	}
}

func TestErrorContext_SessionNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleErrorContext(context.Background(), makeRequest(map[string]any{
		"session_id": "nonexistent-session-id",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent session")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrSessionNotFound) {
		t.Errorf("error_code = %q, want %q", code, ErrSessionNotFound)
	}
}

func TestErrorContext_EmptyContext(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Create a session in the manager.
	sess := &session.Session{ID: "test-sess-1", Status: session.StatusRunning}
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleErrorContext(context.Background(), makeRequest(map[string]any{
		"session_id": "test-sess-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}

	var resp struct {
		SessionID         string `json:"session_id"`
		ConsecutiveErrors int    `json:"consecutive_errors"`
		ShouldEscalate    bool   `json:"should_escalate"`
		TotalErrors       int    `json:"total_errors"`
		RecentErrors      string `json:"recent_errors"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.SessionID != "test-sess-1" {
		t.Errorf("session_id = %q, want %q", resp.SessionID, "test-sess-1")
	}
	if resp.ConsecutiveErrors != 0 {
		t.Errorf("consecutive_errors = %d, want 0", resp.ConsecutiveErrors)
	}
	if resp.ShouldEscalate {
		t.Error("should_escalate = true, want false")
	}
	if resp.TotalErrors != 0 {
		t.Errorf("total_errors = %d, want 0", resp.TotalErrors)
	}
	if resp.RecentErrors != "" {
		t.Errorf("recent_errors = %q, want empty string", resp.RecentErrors)
	}
}

func TestErrorContext_WithErrors(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := &session.Session{ID: "test-sess-2", Status: session.StatusRunning}
	srv.SessMgr.AddSessionForTesting(sess)

	// Pre-populate the error context.
	ec := srv.SessMgr.GetErrorContext("test-sess-2")
	ec.RecordError("build failed: missing import", session.ErrCatBuild, 1)
	ec.RecordError("test failed: expected 42", session.ErrCatTest, 2)
	ec.RecordError("lint: unused variable", session.ErrCatLint, 3)

	result, err := srv.handleErrorContext(context.Background(), makeRequest(map[string]any{
		"session_id": "test-sess-2",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}

	var resp struct {
		SessionID         string `json:"session_id"`
		ConsecutiveErrors int    `json:"consecutive_errors"`
		ShouldEscalate    bool   `json:"should_escalate"`
		TotalErrors       int    `json:"total_errors"`
		RecentErrors      string `json:"recent_errors"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ConsecutiveErrors != 3 {
		t.Errorf("consecutive_errors = %d, want 3", resp.ConsecutiveErrors)
	}
	if !resp.ShouldEscalate {
		t.Error("should_escalate = false, want true (3 >= default threshold 3)")
	}
	if resp.TotalErrors != 3 {
		t.Errorf("total_errors = %d, want 3", resp.TotalErrors)
	}
	if resp.RecentErrors == "" {
		t.Error("recent_errors should not be empty with recorded errors")
	}
}

func TestErrorContext_AfterSuccess(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := &session.Session{ID: "test-sess-3", Status: session.StatusRunning}
	srv.SessMgr.AddSessionForTesting(sess)

	ec := srv.SessMgr.GetErrorContext("test-sess-3")
	ec.RecordError("err1", session.ErrCatRuntime, 1)
	ec.RecordError("err2", session.ErrCatRuntime, 2)
	ec.RecordSuccess()

	result, err := srv.handleErrorContext(context.Background(), makeRequest(map[string]any{
		"session_id": "test-sess-3",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error: %s", getResultText(result))
	}

	var resp struct {
		ConsecutiveErrors int  `json:"consecutive_errors"`
		ShouldEscalate    bool `json:"should_escalate"`
		TotalErrors       int  `json:"total_errors"`
	}
	if err := json.Unmarshal([]byte(getResultText(result)), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ConsecutiveErrors != 0 {
		t.Errorf("consecutive_errors = %d after success, want 0", resp.ConsecutiveErrors)
	}
	if resp.ShouldEscalate {
		t.Error("should_escalate = true after success, want false")
	}
	if resp.TotalErrors != 2 {
		t.Errorf("total_errors = %d, want 2 (errors persist after success)", resp.TotalErrors)
	}
}
