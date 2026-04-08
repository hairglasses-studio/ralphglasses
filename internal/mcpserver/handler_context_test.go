package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleContextBudget_NoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success for empty sessions list")
	}
	text := getResultText(result)
	if !strings.Contains(text, "empty") {
		t.Errorf("expected empty result, got: %s", text)
	}
}

func TestHandleContextBudget_SessionNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(map[string]any{
		"session_id": "nonexistent-id",
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

func TestHandleContextBudget_SingleSession(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Inject a test session with a context budget.
	sess := &session.Session{
		ID:       "test-ctx-1",
		Provider: session.ProviderClaude,
		Model:    "claude-opus",
		Status:   session.StatusRunning,
	}
	sess.CtxBudget = session.NewContextBudget(session.DefaultClaudeLimit)
	sess.CtxBudget.Record(100000) // 50% usage
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(map[string]any{
		"session_id": "test-ctx-1",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if data["session_id"] != "test-ctx-1" {
		t.Errorf("session_id = %v, want test-ctx-1", data["session_id"])
	}
	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
	if int(data["used_tokens"].(float64)) != 100000 {
		t.Errorf("used_tokens = %v, want 100000", data["used_tokens"])
	}
	if int(data["limit"].(float64)) != 200000 {
		t.Errorf("limit = %v, want 200000", data["limit"])
	}
}

func TestHandleContextBudget_WarningStatus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := &session.Session{
		ID:       "test-ctx-warn",
		Provider: session.ProviderGemini,
		Model:    "gemini-3.1-flash",
		Status:   session.StatusRunning,
	}
	sess.CtxBudget = session.NewContextBudget(100)
	sess.CtxBudget.Record(85) // 85% > 80% warning threshold
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(map[string]any{
		"session_id": "test-ctx-warn",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if data["status"] != "warning" {
		t.Errorf("status = %v, want warning", data["status"])
	}
}

func TestHandleContextBudget_CriticalStatus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := &session.Session{
		ID:       "test-ctx-crit",
		Provider: session.ProviderCodex,
		Model:    "gpt-5.4",
		Status:   session.StatusRunning,
	}
	sess.CtxBudget = session.NewContextBudget(100)
	sess.CtxBudget.Record(96) // 96% > 95% critical threshold
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(map[string]any{
		"session_id": "test-ctx-crit",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if data["status"] != "critical" {
		t.Errorf("status = %v, want critical", data["status"])
	}
}

func TestHandleContextBudget_NilBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := &session.Session{
		ID:       "test-ctx-nil",
		Provider: session.ProviderClaude,
		Model:    "claude-opus",
		Status:   session.StatusRunning,
	}
	// No CtxBudget set — handler should return defaults.
	srv.SessMgr.AddSessionForTesting(sess)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(map[string]any{
		"session_id": "test-ctx-nil",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if data["status"] != "ok" {
		t.Errorf("status = %v, want ok", data["status"])
	}
	if int(data["limit"].(float64)) != session.DefaultClaudeLimit {
		t.Errorf("limit = %v, want %d", data["limit"], session.DefaultClaudeLimit)
	}
}

func TestHandleContextBudget_AllSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess1 := &session.Session{
		ID:       "test-ctx-all-1",
		Provider: session.ProviderClaude,
		Status:   session.StatusRunning,
	}
	sess1.CtxBudget = session.NewContextBudget(200000)
	sess1.CtxBudget.Record(50000)

	sess2 := &session.Session{
		ID:       "test-ctx-all-2",
		Provider: session.ProviderGemini,
		Status:   session.StatusRunning,
	}
	sess2.CtxBudget = session.NewContextBudget(1000000)
	sess2.CtxBudget.Record(900001) // > 90% but below critical

	srv.SessMgr.AddSessionForTesting(sess1)
	srv.SessMgr.AddSessionForTesting(sess2)

	result, err := srv.handleContextBudget(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}

	var data []map[string]any
	if err := json.Unmarshal([]byte(getResultText(result)), &data); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}

	if len(data) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(data))
	}
}
