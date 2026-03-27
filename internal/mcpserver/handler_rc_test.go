package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Helper function tests ---

func TestShortID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefgh", "abcdefgh"},
		{"abcdefghi", "abcdefgh"},
		{"abcdefghijklmnop", "abcdefgh"},
		{"abc", "abc"},
		{"", ""},
	}
	for _, tt := range tests {
		got := shortID(tt.input)
		if got != tt.want {
			t.Errorf("shortID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatDuration(t *testing.T) {
	t.Parallel()
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1m"},
		{5 * time.Minute, "5m"},
		{59 * time.Minute, "59m"},
		{2 * time.Hour, "2h"},
		{90 * time.Minute, "1h"},
	}
	for _, tt := range tests {
		got := formatDuration(tt.d)
		if got != tt.want {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestFormatCost(t *testing.T) {
	t.Parallel()
	tests := []struct {
		usd  float64
		want string
	}{
		{0.0, "$0.00"},
		{1.5, "$1.50"},
		{10.123, "$10.12"},
	}
	for _, tt := range tests {
		got := formatCost(tt.usd)
		if got != tt.want {
			t.Errorf("formatCost(%v) = %q, want %q", tt.usd, got, tt.want)
		}
	}
}

// --- RC handler tests ---

func TestHandleRCStatus_NoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "0 running") {
		t.Errorf("expected '0 running' in output, got: %s", text)
	}
	if !strings.Contains(text, "No active or recent sessions") {
		t.Errorf("expected 'No active or recent sessions' in output, got: %s", text)
	}
}

func TestHandleRCStatus_WithRunningSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 1.23
		s.TurnCount = 5
		s.LastActivity = time.Now()
	})

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "1 running") {
		t.Errorf("expected '1 running' in output, got: %s", text)
	}
	if !strings.Contains(text, "[running]") {
		t.Errorf("expected '[running]' session line in output, got: %s", text)
	}
}

func TestHandleRCSend_MissingRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "repo name required") {
		t.Errorf("expected 'repo name required' in error, got: %s", text)
	}
}

func TestHandleRCSend_MissingPrompt(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing prompt")
	}
	text := getResultText(result)
	if !strings.Contains(text, "prompt required") {
		t.Errorf("expected 'prompt required' in error, got: %s", text)
	}
}

func TestHandleRCSend_InvalidRepoName(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "../escape",
		"prompt": "test",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid repo name")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid repo name") {
		t.Errorf("expected 'invalid repo name' in error, got: %s", text)
	}
}

func TestHandleRCRead_NoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"status":"empty"`) {
		t.Errorf("expected empty status JSON in output, got: %s", text)
	}
	if !strings.Contains(text, `"item_type":"rc_messages"`) {
		t.Errorf("expected item_type rc_messages in empty result, got: %s", text)
	}
}

func TestHandleRCRead_MissingSession(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id": "nonexistent-session-id",
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing session")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SESSION_NOT_FOUND") {
		t.Errorf("expected 'SESSION_NOT_FOUND' error code, got: %s", text)
	}
}

func TestHandleRCRead_WithSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 2.50
		s.TurnCount = 3
		s.OutputHistory = []string{"line 1", "line 2", "line 3"}
		s.TotalOutputCount = 3
	})

	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id": sid,
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "[running]") {
		t.Errorf("expected '[running]' in output, got: %s", text)
	}
	if !strings.Contains(text, "$2.50") {
		t.Errorf("expected '$2.50' in output, got: %s", text)
	}
	if !strings.Contains(text, "line 1") {
		t.Errorf("expected output history lines, got: %s", text)
	}
	if !strings.Contains(text, "cursor:3") {
		t.Errorf("expected 'cursor:3' in output, got: %s", text)
	}
}

func TestHandleRCRead_InvalidCursor(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", nil)

	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id":     sid,
		"cursor": "not-a-number",
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid cursor")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid cursor") {
		t.Errorf("expected 'invalid cursor' in error, got: %s", text)
	}
}

func TestHandleRCAct_MissingAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing action")
	}
	text := getResultText(result)
	if !strings.Contains(text, "action required") {
		t.Errorf("expected 'action required' in error, got: %s", text)
	}
}

func TestHandleRCAct_UnknownAction(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "explode",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for unknown action")
	}
	text := getResultText(result)
	if !strings.Contains(text, "unknown action") {
		t.Errorf("expected 'unknown action' in error, got: %s", text)
	}
}

func TestHandleRCAct_StopMissingTarget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop",
		"target": "",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing target on stop")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SESSION_NOT_FOUND") {
		t.Errorf("expected SESSION_NOT_FOUND error, got: %s", text)
	}
}

func TestHandleRCAct_PauseMissingTarget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "pause",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing target on pause")
	}
	text := getResultText(result)
	if !strings.Contains(text, "target required") {
		t.Errorf("expected 'target required' in error, got: %s", text)
	}
}

func TestHandleRCAct_ResumeMissingTarget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "resume",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing target on resume")
	}
	text := getResultText(result)
	if !strings.Contains(text, "target required") {
		t.Errorf("expected 'target required' in error, got: %s", text)
	}
}

func TestHandleRCAct_StopAll(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
	})

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop_all",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped 1 session") {
		t.Errorf("expected 'Stopped 1 session' in output, got: %s", text)
	}
}

func TestHandleRCAct_StopBySessionID(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 0.50
		s.TurnCount = 2
	})

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop",
		"target": sid,
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped session") {
		t.Errorf("expected 'Stopped session' in output, got: %s", text)
	}
}

// --- resolveTarget tests ---

func TestResolveTarget_Empty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, err := srv.resolveTarget("")
	if err == nil {
		t.Fatal("expected error for empty target")
	}
	if !strings.Contains(err.Error(), "target required") {
		t.Errorf("expected 'target required' error, got: %v", err)
	}
}

func TestResolveTarget_ByID(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", nil)

	sess, err := srv.resolveTarget(sid)
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if sess.ID != sid {
		t.Errorf("expected session ID %s, got %s", sid, sess.ID)
	}
}

func TestResolveTarget_NotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, err := srv.resolveTarget("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent target")
	}
	if !strings.Contains(err.Error(), "no session found") {
		t.Errorf("expected 'no session found' error, got: %v", err)
	}
}

func TestResolveTarget_ByRepoName(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now()
	})

	sess, err := srv.resolveTarget("test-repo")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if sess.ID != sid {
		t.Errorf("expected session ID %s, got %s", sid, sess.ID)
	}
}

// --- summarizeEvent tests ---

func TestSummarizeEvent_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		event    events.Event
		contains string
	}{
		{
			"session_started",
			events.Event{Type: events.SessionStarted, RepoName: "myrepo", Provider: "claude", SessionID: "abcdefghij"},
			"[start] myrepo/claude session abcdefgh",
		},
		{
			"session_ended",
			events.Event{Type: events.SessionEnded, RepoName: "myrepo", SessionID: "abcdefghij"},
			"[end] myrepo session abcdefgh",
		},
		{
			"session_stopped",
			events.Event{Type: events.SessionStopped, RepoName: "myrepo", SessionID: "abcdefghij"},
			"[stop] myrepo session abcdefgh",
		},
		{
			"cost_update_with_data",
			events.Event{Type: events.CostUpdate, RepoName: "myrepo", Data: map[string]any{"cost_usd": 1.23}},
			"$1.23",
		},
		{
			"cost_update_no_data",
			events.Event{Type: events.CostUpdate, RepoName: "myrepo"},
			"[cost] myrepo",
		},
		{
			"budget_exceeded",
			events.Event{Type: events.BudgetExceeded, RepoName: "myrepo"},
			"[BUDGET] myrepo exceeded budget",
		},
		{
			"loop_started",
			events.Event{Type: events.LoopStarted, RepoName: "myrepo"},
			"[loop] myrepo started",
		},
		{
			"loop_stopped",
			events.Event{Type: events.LoopStopped, RepoName: "myrepo"},
			"[loop] myrepo stopped",
		},
		{
			"team_created",
			events.Event{Type: events.TeamCreated, RepoName: "myrepo"},
			"[team] myrepo created",
		},
		{
			"tool_called_with_data",
			events.Event{Type: events.ToolCalled, Data: map[string]any{"tool": "prompt_analyze", "latency_ms": float64(45)}},
			"[tool.called] prompt_analyze (45ms)",
		},
		{
			"tool_called_name_only",
			events.Event{Type: events.ToolCalled, Data: map[string]any{"name": "session_launch"}},
			"[tool.called] session_launch",
		},
		{
			"tool_called_no_data",
			events.Event{Type: events.ToolCalled},
			"[tool.called] unknown",
		},
		{
			"scan_complete_with_count",
			events.Event{Type: events.ScanComplete, Data: map[string]any{"repo_count": float64(7)}},
			"[scan.complete] found 7 repos",
		},
		{
			"scan_complete_no_data",
			events.Event{Type: events.ScanComplete},
			"[scan.complete] scan finished",
		},
		{
			"loop_iterated_with_step_status",
			events.Event{Type: events.LoopIterated, Data: map[string]any{"step": float64(3), "status": "pass"}},
			"[loop.iterated] step 3: pass",
		},
		{
			"loop_iterated_step_only",
			events.Event{Type: events.LoopIterated, Data: map[string]any{"step": float64(5)}},
			"[loop.iterated] step 5",
		},
		{
			"loop_iterated_no_data",
			events.Event{Type: events.LoopIterated},
			"[loop.iterated] iteration",
		},
		{
			"loop_regression",
			events.Event{Type: events.LoopRegression, RepoName: "myrepo"},
			"[loop.regression] myrepo",
		},
		{
			"prompt_enhanced",
			events.Event{Type: events.PromptEnhanced, RepoName: "myrepo"},
			"[prompt.enhanced] myrepo",
		},
		{
			"session_error_with_msg",
			events.Event{Type: events.SessionError, RepoName: "myrepo", Data: map[string]any{"error": "timeout"}},
			"[session.error] myrepo: timeout",
		},
		{
			"session_error_no_data",
			events.Event{Type: events.SessionError, RepoName: "myrepo"},
			"[session.error] myrepo",
		},
		{
			"unknown_type",
			events.Event{Type: "custom.event", RepoName: "myrepo"},
			"[custom.event] myrepo",
		},
		{
			"unknown_type_with_data",
			events.Event{Type: "custom.event", RepoName: "myrepo", Data: map[string]any{"key1": "val1"}},
			"key1=val1",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := summarizeEvent(tt.event)
			if !strings.Contains(got, tt.contains) {
				t.Errorf("summarizeEvent() = %q, want to contain %q", got, tt.contains)
			}
		})
	}
}

func TestSummarizeEvent_CostUpdateNonFloat(t *testing.T) {
	t.Parallel()
	// cost_usd is present but not a float64
	e := events.Event{Type: events.CostUpdate, RepoName: "myrepo", Data: map[string]any{"cost_usd": "not-a-number"}}
	got := summarizeEvent(e)
	if !strings.Contains(got, "[cost] myrepo") {
		t.Errorf("summarizeEvent() = %q, want to contain '[cost] myrepo'", got)
	}
}

// --- handleRCStatus extended tests ---

func TestHandleRCStatus_IdleAlert(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create a running session that has been idle for > 8 minutes
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.SpentUSD = 0.50
		s.TurnCount = 2
		s.LastActivity = time.Now().Add(-10 * time.Minute)
	})

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "1 alert") {
		t.Errorf("expected alert for idle session, got: %s", text)
	}
	if !strings.Contains(text, "Alerts:") {
		t.Errorf("expected 'Alerts:' section, got: %s", text)
	}
	if !strings.Contains(text, "idle") {
		t.Errorf("expected 'idle' in alert message, got: %s", text)
	}
}

func TestHandleRCStatus_RecentStoppedSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create a stopped session with recent activity (within 30 min)
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.SpentUSD = 1.00
		s.TurnCount = 3
		s.LastActivity = time.Now().Add(-5 * time.Minute)
	})

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCStatus: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "[stopped]") {
		t.Errorf("expected '[stopped]' for recent session, got: %s", text)
	}
}

func TestHandleRCStatus_LaunchingSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusLaunching
		s.LastActivity = time.Now()
	})

	result, err := srv.handleRCStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCStatus: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "1 running") {
		t.Errorf("expected launching counted as running, got: %s", text)
	}
}

// --- handleRCSend extended tests ---

func TestHandleRCSend_RepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "nonexistent-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for repo not found")
	}
	text := getResultText(result)
	if !strings.Contains(text, "REPO_NOT_FOUND") {
		t.Errorf("expected REPO_NOT_FOUND error code, got: %s", text)
	}
}

func TestHandleRCSend_InvalidProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":     "test-repo",
		"prompt":   "do something",
		"provider": "invalid-provider-xyz",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid provider")
	}
	text := getResultText(result)
	if !strings.Contains(text, "INVALID_PARAMS") {
		t.Errorf("expected INVALID_PARAMS error code, got: %s", text)
	}
}

func TestHandleRCSend_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SCAN_FAILED") {
		t.Errorf("expected SCAN_FAILED error code, got: %s", text)
	}
}

func TestHandleRCSend_DefaultProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Trigger scan so test-repo is found
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// No provider specified — should default to claude, then attempt launch
	// Launch will fail (no real claude CLI) but we exercise the provider default + stop existing + launch paths
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	// Launch will fail in test environment, but we exercised the code path
	_ = result
}

func TestHandleRCSend_WithExistingRunningSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Trigger scan
	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Inject a running session on test-repo that should be stopped
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
	})

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something else",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	// The existing session should be stopped even if launch fails
	_ = result
}

func TestHandleRCSend_WithBudget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
		"budget_usd": float64(20),
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	_ = result
}

func TestHandleRCSend_WithResumeNoExisting(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Resume with no existing sessions — should fall through to fresh launch
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
		"resume": "true",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	_ = result
}

func TestHandleRCSend_WithResumeExistingSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	// Inject a session with a provider session ID
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.ProviderSessionID = "provider-sess-123"
		s.Status = session.StatusStopped
	})

	// Resume should find the session with provider session ID
	result, err := srv.handleRCSend(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "continue work",
		"resume": "true",
	}))
	if err != nil {
		t.Fatalf("handleRCSend: %v", err)
	}
	// Resume may fail since it's trying to actually restart a process
	_ = result
}

// --- handleRCRead extended tests ---

func TestHandleRCRead_WithCursor(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.OutputHistory = []string{"line1", "line2", "line3", "line4", "line5"}
		s.TotalOutputCount = 10
	})

	// Cursor at 8 means 2 new lines (10 - 8)
	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id":     sid,
		"cursor": "8",
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "line4") || !strings.Contains(text, "line5") {
		t.Errorf("expected last 2 lines from cursor, got: %s", text)
	}
	if !strings.Contains(text, "cursor:10") {
		t.Errorf("expected 'cursor:10', got: %s", text)
	}
}

func TestHandleRCRead_CursorAtEnd(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.OutputHistory = []string{"line1"}
		s.TotalOutputCount = 5
	})

	// Cursor at 5 means no new lines
	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id":     sid,
		"cursor": "5",
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "(no new output)") {
		t.Errorf("expected '(no new output)', got: %s", text)
	}
}

func TestHandleRCRead_MostActiveSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create a session without specifying an ID; handleRCRead should pick the most active
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.OutputHistory = []string{"auto-selected"}
		s.TotalOutputCount = 1
		s.LastActivity = time.Now()
	})

	// No id provided; should fall back to mostActiveSession
	result, err := srv.handleRCRead(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "auto-selected") {
		t.Errorf("expected auto-selected output from most active session, got: %s", text)
	}
}

func TestHandleRCRead_LinesClamp(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "output"
	}
	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.OutputHistory = lines
		s.TotalOutputCount = 50
	})

	// Request more than 30 (max)
	result, err := srv.handleRCRead(context.Background(), makeRequest(map[string]any{
		"id":    sid,
		"lines": float64(50),
	}))
	if err != nil {
		t.Fatalf("handleRCRead: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	// Lines should be clamped to 30; the result should contain exactly 30 output lines
	text := getResultText(result)
	// Count occurrences of "output" in the content after the header line
	parts := strings.Split(text, "\n")
	outputCount := 0
	for _, p := range parts {
		if strings.TrimSpace(p) == "output" {
			outputCount++
		}
	}
	if outputCount > 30 {
		t.Errorf("expected at most 30 output lines, got %d", outputCount)
	}
}

// --- handleRCAct extended tests ---

func TestHandleRCAct_RetryNonexistentTarget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "retry",
		"target": "nonexistent-session",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent target on retry")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SESSION_NOT_FOUND") {
		t.Errorf("expected SESSION_NOT_FOUND error, got: %s", text)
	}
}

func TestHandleRCAct_ResumeNoProviderSessionID(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.ProviderSessionID = "" // No provider session to resume
	})

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "resume",
		"target": sid,
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for session without provider session ID")
	}
	text := getResultText(result)
	if !strings.Contains(text, "no provider session ID") {
		t.Errorf("expected 'no provider session ID' error, got: %s", text)
	}
}

func TestHandleRCAct_StopNonexistentTarget(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop",
		"target": "does-not-exist",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for nonexistent target on stop")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SESSION_NOT_FOUND") {
		t.Errorf("expected SESSION_NOT_FOUND error, got: %s", text)
	}
}

func TestHandleRCAct_PauseRepoNotFound(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "pause",
		"target": "nonexistent-repo",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for pause on nonexistent repo")
	}
	text := getResultText(result)
	if !strings.Contains(text, "REPO_NOT_FOUND") {
		t.Errorf("expected REPO_NOT_FOUND error, got: %s", text)
	}
}

func TestHandleRCAct_RetryWithSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	sid := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Prompt = "do something"
		s.BudgetUSD = 5.0
	})

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "retry",
		"target": sid,
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	// Launch may fail in test mode, but we exercise the retry path
	_ = result
}

func TestHandleRCAct_PauseWithValidRepo(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	_, _ = srv.handleScan(context.Background(), makeRequest(nil))

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "pause",
		"target": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	// TogglePause may fail, but exercises the path through repo lookup
	_ = result
}

func TestHandleRCAct_PauseScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "pause",
		"target": "test-repo",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails for pause")
	}
	text := getResultText(result)
	if !strings.Contains(text, "SCAN_FAILED") {
		t.Errorf("expected SCAN_FAILED error, got: %s", text)
	}
}

func TestHandleRCAct_StopAllNoSessions(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleRCAct(context.Background(), makeRequest(map[string]any{
		"action": "stop_all",
	}))
	if err != nil {
		t.Fatalf("handleRCAct: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped 0 session") {
		t.Errorf("expected 'Stopped 0 session(s)', got: %s", text)
	}
}

// --- resolveTarget extended tests ---

func TestResolveTarget_PrefersRunning(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create stopped session (older)
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now().Add(-5 * time.Minute)
	})

	// Create running session (more recent)
	runningSID := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now()
	})

	sess, err := srv.resolveTarget("test-repo")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if sess.ID != runningSID {
		t.Errorf("expected running session ID %s, got %s", runningSID, sess.ID)
	}
}

func TestResolveTarget_MostRecentRunning(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create two running sessions with different activity times
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now().Add(-3 * time.Minute)
	})

	newerSID := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now()
	})

	sess, err := srv.resolveTarget("test-repo")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if sess.ID != newerSID {
		t.Errorf("expected most recent running session ID %s, got %s", newerSID, sess.ID)
	}
}

func TestResolveTarget_MostRecentStopped(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Create two stopped sessions, no running sessions
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now().Add(-10 * time.Minute)
	})

	newerSID := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now().Add(-1 * time.Minute)
	})

	sess, err := srv.resolveTarget("test-repo")
	if err != nil {
		t.Fatalf("resolveTarget: %v", err)
	}
	if sess.ID != newerSID {
		t.Errorf("expected most recent stopped session ID %s, got %s", newerSID, sess.ID)
	}
}

// --- mostActiveSession tests ---

func TestMostActiveSession_Empty(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	sess := srv.mostActiveSession()
	if sess != nil {
		t.Fatal("expected nil for empty session list")
	}
}

func TestMostActiveSession_PrefersRunning(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	// Stopped session with more recent activity
	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now()
	})

	// Running session with older activity
	runningSID := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusRunning
		s.LastActivity = time.Now().Add(-5 * time.Minute)
	})

	sess := srv.mostActiveSession()
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.ID != runningSID {
		t.Errorf("expected running session preferred, got %s", sess.ID)
	}
}

func TestMostActiveSession_MostRecentAmongStopped(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)

	injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now().Add(-10 * time.Minute)
	})

	newerSID := injectTestSession(t, srv, root+"/test-repo", func(s *session.Session) {
		s.Status = session.StatusStopped
		s.LastActivity = time.Now()
	})

	sess := srv.mostActiveSession()
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if sess.ID != newerSID {
		t.Errorf("expected most recent stopped session, got %s", sess.ID)
	}
}

// --- Event poll tests ---

func TestHandleEventPoll_NilBus(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.EventBus = nil

	result, err := srv.handleEventPoll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleEventPoll: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when event bus is nil")
	}
	text := getResultText(result)
	if !strings.Contains(text, "NOT_RUNNING") {
		t.Errorf("expected NOT_RUNNING error code, got: %s", text)
	}
}

func TestHandleEventPoll_InvalidCursorRC(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	srv.EventBus = events.NewBus(100)

	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"cursor": "not-a-number",
	}))
	if err != nil {
		t.Fatalf("handleEventPoll: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for invalid cursor")
	}
	text := getResultText(result)
	if !strings.Contains(text, "invalid cursor") {
		t.Errorf("expected 'invalid cursor' in error, got: %s", text)
	}
}

func TestHandleEventPoll_WithEvents(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "test-repo", SessionID: "abc123456"})
	bus.Publish(events.Event{Type: events.CostUpdate, RepoName: "test-repo", SessionID: "abc123456", Data: map[string]any{"cost_usd": 1.0}})

	result, err := srv.handleEventPoll(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handleEventPoll: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "session.started") {
		t.Errorf("expected session.started event, got: %s", text)
	}
	if !strings.Contains(text, "cursor") {
		t.Errorf("expected cursor in response, got: %s", text)
	}
}

func TestHandleEventPoll_TypeFilterRC(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "test-repo"})
	bus.Publish(events.Event{Type: events.CostUpdate, RepoName: "test-repo"})
	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "other-repo"})

	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"type": "cost.update",
	}))
	if err != nil {
		t.Fatalf("handleEventPoll: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"count":1`) {
		t.Errorf("expected 1 event after filter, got: %s", text)
	}
}

func TestHandleEventPoll_LimitClamp(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus

	// Publish many events
	for i := 0; i < 60; i++ {
		bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "test-repo"})
	}

	// Request with limit > 50, should be clamped to 50
	result, err := srv.handleEventPoll(context.Background(), makeRequest(map[string]any{
		"limit": float64(100),
	}))
	if err != nil {
		t.Fatalf("handleEventPoll: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	// Count should be at most 50
	if strings.Contains(text, `"count":60`) {
		t.Errorf("expected count clamped to 50 or less, got: %s", text)
	}
}

func TestHandleEventPoll_WithValidCursor(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	bus := events.NewBus(100)
	srv.EventBus = bus

	bus.Publish(events.Event{Type: events.SessionStarted, RepoName: "old-event"})
	bus.Publish(events.Event{Type: events.SessionEnded, RepoName: "new-event"})

	// First poll to get cursor
	result1, _ := srv.handleEventPoll(context.Background(), makeRequest(nil))
	text1 := getResultText(result1)
	if !strings.Contains(text1, "cursor") {
		t.Fatalf("expected cursor in first poll, got: %s", text1)
	}
}
