package mcpserver

import (
	"context"
	"strings"
	"testing"
	"time"

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
	if !strings.Contains(text, `"status": "empty"`) {
		t.Errorf("expected empty status JSON in output, got: %s", text)
	}
	if !strings.Contains(text, `"item_type": "rc_messages"`) {
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
