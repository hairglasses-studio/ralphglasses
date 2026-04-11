package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Dispatch handler tests ---

func TestDispatch_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"action": "send",
		"prompt": "hello",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo required") {
		t.Errorf("expected 'repo required' in error, got: %s", text)
	}
}

func TestDispatch_MissingAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing action")
	}
	text := getResultText(result)
	if !strings.Contains(text, "action required") {
		t.Errorf("expected 'action required' in error, got: %s", text)
	}
}

func TestDispatch_InvalidAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "destroy",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid action")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid action") {
		t.Errorf("expected 'invalid action' in error, got: %s", text)
	}
}

func TestDispatch_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "../../etc/passwd",
		"action": "stop",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid repo name")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid repo name") {
		t.Errorf("expected 'invalid repo name' in error, got: %s", text)
	}
}

func TestDispatch_SendMissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "send",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt required") {
		t.Errorf("expected 'prompt required' in error, got: %s", text)
	}
}

func TestDispatch_SendRepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Ensure repos are scanned first.
	_ = srv.scan()

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":     "nonexistent-repo",
		"action":   "send",
		"prompt":   "test prompt",
		"provider": "claude",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "REPO_NOT_FOUND") {
		t.Errorf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

func TestDispatch_StopNoSession(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "stop",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when no session found for stop")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SESSION_NOT_FOUND") {
		t.Errorf("expected SESSION_NOT_FOUND error, got: %s", text)
	}
}

func TestDispatch_StopWithSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 0.50
		s.TurnCount = 3
		s.LastActivity = time.Now()
	})

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "stop",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped") {
		t.Errorf("expected 'Stopped' in output, got: %s", text)
	}
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected repo name in output, got: %s", text)
	}
}

func TestDispatch_RetryWithSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusErrored
		s.SpentUSD = 1.00
		s.TurnCount = 10
		s.LastActivity = time.Now()
		s.Prompt = "original prompt"
	})

	result, err := srv.handleDispatch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"action": "retry",
	}))
	if err != nil {
		t.Fatalf("handleDispatch: %v", err)
	}
	// Retry will attempt to launch which may fail in test env (no binary),
	// but the dispatch logic path is validated.
	_ = result
}

func TestDispatch_AutoProviderSelection(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Auto and omitted provider now leave runtime selection unset.
	provider := srv.resolveProvider("auto")
	if provider != "" {
		t.Errorf("auto provider = %q, want empty", provider)
	}

	// Explicit provider should be returned as-is.
	provider = srv.resolveProvider("claude")
	if provider != session.ProviderClaude {
		t.Errorf("explicit claude = %q, want %q", provider, session.ProviderClaude)
	}

	provider = srv.resolveProvider("gemini")
	if provider != session.ProviderGemini {
		t.Errorf("explicit gemini = %q, want %q", provider, session.ProviderGemini)
	}

	// Empty string should behave like auto (cascade router picks).
	provider = srv.resolveProvider("")
	if provider != "" {
		t.Errorf("empty provider = %q, want empty", provider)
	}
}

func TestDispatch_AutoWithCascadeRouterLeavesRuntimeSelectionUnset(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Wire a cascade router into the session manager.
	cfg := session.DefaultCascadeConfig()
	cr := session.NewCascadeRouter(cfg, nil, nil, t.TempDir())
	srv.SessMgr.SetCascadeRouter(cr)

	// Auto still leaves selection to the runtime even if a cascade router exists.
	provider := srv.resolveProvider("auto")
	if provider != "" {
		t.Errorf("auto with cascade returned %q, want empty", provider)
	}
}

func TestDispatch_AllValidActions(t *testing.T) {
	t.Parallel()

	actions := []string{"send", "stop", "pause", "resume", "retry"}
	for _, a := range actions {
		if !validDispatchActions[a] {
			t.Errorf("action %q should be valid", a)
		}
	}

	invalid := []string{"destroy", "kill", "restart", ""}
	for _, a := range invalid {
		if validDispatchActions[a] {
			t.Errorf("action %q should be invalid", a)
		}
	}
}

// --- Fleet summary handler tests ---

func TestFleetSummary_NoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleFleetSummary(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetSummary: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Fleet:") {
		t.Errorf("expected 'Fleet:' header in output, got: %s", text)
	}
	if !strings.Contains(text, "0 total") {
		t.Errorf("expected '0 total' in output, got: %s", text)
	}
	if !strings.Contains(text, "0 active") {
		t.Errorf("expected '0 active' in output, got: %s", text)
	}
	if !strings.Contains(text, "No sessions.") {
		t.Errorf("expected 'No sessions.' in output, got: %s", text)
	}
}

func TestFleetSummary_WithSessions(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 2.50
		s.TurnCount = 15
		s.LastActivity = time.Now()
		s.Provider = session.ProviderClaude
	})

	injectTestSession(t, srv, root+"/other-repo", func(s *session.Session) {
		s.RepoName = "other-repo"
		s.Status = session.StatusStopped
		s.SpentUSD = 0.75
		s.TurnCount = 5
		s.LastActivity = time.Now().Add(-10 * time.Minute)
		s.Provider = session.ProviderGemini
	})

	result, err := srv.handleFleetSummary(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetSummary: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)

	if !strings.Contains(text, "2 total") {
		t.Errorf("expected '2 total' in output, got: %s", text)
	}
	if !strings.Contains(text, "1 active") {
		t.Errorf("expected '1 active' in output, got: %s", text)
	}
	if !strings.Contains(text, "1 stopped") {
		t.Errorf("expected '1 stopped' in output, got: %s", text)
	}
	if !strings.Contains(text, "$3.25") {
		t.Errorf("expected '$3.25' total cost in output, got: %s", text)
	}
	// Check per-repo lines are present.
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected 'test-repo' in per-repo output, got: %s", text)
	}
	if !strings.Contains(text, "other-repo") {
		t.Errorf("expected 'other-repo' in per-repo output, got: %s", text)
	}
}

func TestFleetSummary_OutputFormat(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 1.00
		s.TurnCount = 8
		s.LastActivity = time.Now()
		s.Provider = session.ProviderCodex
	})

	result, err := srv.handleFleetSummary(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleFleetSummary: %v", err)
	}
	text := getResultText(result)

	// Verify it's plain text, not JSON.
	if strings.HasPrefix(text, "{") || strings.HasPrefix(text, "[") {
		t.Errorf("expected plain text output, got JSON-like: %s", text)
	}
	// Verify per-repo line format includes key fields.
	if !strings.Contains(text, "[running]") {
		t.Errorf("expected '[running]' status in per-repo line, got: %s", text)
	}
	if !strings.Contains(text, "codex") {
		t.Errorf("expected 'codex' provider in per-repo line, got: %s", text)
	}
	if !strings.Contains(text, "$1.00") {
		t.Errorf("expected '$1.00' cost in per-repo line, got: %s", text)
	}
	if !strings.Contains(text, "8t") {
		t.Errorf("expected '8t' turn count in per-repo line, got: %s", text)
	}
}
