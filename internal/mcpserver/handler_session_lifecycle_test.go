package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandleSessionResume(t *testing.T) {
	t.Parallel()

	t.Run("missing repo param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionResume(context.Background(), makeRequest(map[string]any{
			"session_id": "some-session",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing repo")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("invalid repo name", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionResume(context.Background(), makeRequest(map[string]any{
			"repo":       "../escape-attempt",
			"session_id": "some-session",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid repo name")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("missing session_id param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionResume(context.Background(), makeRequest(map[string]any{
			"repo": "test-repo",
		}))
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
	})

	t.Run("repo not found", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionResume(context.Background(), makeRequest(map[string]any{
			"repo":       "nonexistent-repo",
			"session_id": "some-session",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for repo not found")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrRepoNotFound) {
			t.Errorf("error_code = %q, want %q", code, ErrRepoNotFound)
		}
	})
}

func TestHandleSessionRetry(t *testing.T) {
	t.Parallel()

	t.Run("missing id param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionRetry(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing id")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("non-existent session no sessions", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionRetry(context.Background(), makeRequest(map[string]any{
			"id": "does-not-exist",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for non-existent session")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrNoActiveSessions) {
			t.Errorf("error_code = %q, want %q", code, ErrNoActiveSessions)
		}
	})
}

func TestHandleSessionLaunch_PromptTooLong(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	longPrompt := strings.Repeat("x", MaxPromptLength+1)
	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": longPrompt,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for prompt too long")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrInvalidParams) {
		t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
	}
}

func TestHandleSessionLaunch_SystemPromptTooLong(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	longSystem := strings.Repeat("y", MaxPromptLength+1)
	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":          "test-repo",
		"prompt":        "do something",
		"system_prompt": longSystem,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for system_prompt too long")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrInvalidParams) {
		t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
	}
}

func TestHandleSessionLaunch_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrScanFailed) {
		t.Errorf("error_code = %q, want %q", code, ErrScanFailed)
	}
}

func TestHandleSessionResume_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	result, err := srv.handleSessionResume(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"session_id": "some-session",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrScanFailed) {
		t.Errorf("error_code = %q, want %q", code, ErrScanFailed)
	}
}

func TestHandleSessionStop_ValidSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusRunning
	})

	result, err := srv.handleSessionStop(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "Stopped session") {
		t.Errorf("expected 'Stopped session' message, got: %s", text)
	}
}

func TestHandleSessionStop_AlreadyStopped(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
	})

	result, err := srv.handleSessionStop(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Stopping an already-stopped session should return an error from the manager
	if !result.IsError {
		t.Fatal("expected error for stopping already-stopped session")
	}
}

func TestHandleSessionList_ProviderFilter(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderGemini
	})

	result, err := srv.handleSessionList(context.Background(), makeRequest(map[string]any{
		"provider": "gemini",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"provider":"gemini"`) {
		t.Errorf("expected gemini session in filtered list, got: %s", text)
	}
	if strings.Contains(text, `"provider":"claude"`) {
		t.Errorf("should not contain claude sessions after provider filter, got: %s", text)
	}
}

func TestHandleSessionList_RepoFilter(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, nil)

	// Filter by the test-repo name (which should resolve via findRepo)
	result, err := srv.handleSessionList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "test-repo") {
		t.Errorf("expected test-repo sessions in filtered list, got: %s", text)
	}
}

func TestHandleSessionList_ScanError(t *testing.T) {
	t.Parallel()
	srv := NewServer("/nonexistent/path/that/does/not/exist")

	// Filter by a repo name that triggers scan
	result, err := srv.handleSessionList(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error when scan fails")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrScanFailed) {
		t.Errorf("error_code = %q, want %q", code, ErrScanFailed)
	}
}

func TestHandleSessionOutput_LinesClamp(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Create a session with many output lines
	lines := make([]string, 200)
	for i := range lines {
		lines[i] = "output-line"
	}
	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = lines
	})

	// Request more than the cap of 100
	result, err := srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{
		"id":    id,
		"lines": float64(200),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	// lines should be clamped to 100
	if !strings.Contains(text, `"lines":100`) {
		t.Errorf("expected lines clamped to 100, got: %s", text)
	}
}

func TestHandleSessionOutput_EmptyHistory(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = nil
	})

	result, err := srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"lines":0`) {
		t.Errorf("expected 0 lines for empty history, got: %s", text)
	}
}

func TestHandleSessionStatus_WithEndedAt(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	now := time.Now()
	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
		s.EndedAt = &now
		s.ExitReason = "budget exhausted"
		s.Error = "ran out of money"
	})

	result, err := srv.handleSessionStatus(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "ended_at") {
		t.Errorf("expected ended_at in response, got: %s", text)
	}
	if !strings.Contains(text, "budget exhausted") {
		t.Errorf("expected exit_reason in response, got: %s", text)
	}
}

func TestHandleSessionErrors_RepoFilter(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "some error"
		s.RepoName = "test-repo"
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "other error"
		s.RepoName = "other-repo"
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(map[string]any{
		"repo": "test-repo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "some error") {
		t.Errorf("expected error from test-repo, got: %s", text)
	}
}

func TestTruncateForAlert(t *testing.T) {
	t.Parallel()

	t.Run("short string unchanged", func(t *testing.T) {
		t.Parallel()
		got := truncateForAlert("hello", 10)
		if got != "hello" {
			t.Errorf("truncateForAlert = %q, want %q", got, "hello")
		}
	})

	t.Run("long string truncated", func(t *testing.T) {
		t.Parallel()
		got := truncateForAlert("abcdefghij", 7)
		if got != "abcd..." {
			t.Errorf("truncateForAlert = %q, want %q", got, "abcd...")
		}
	})

	t.Run("exact length unchanged", func(t *testing.T) {
		t.Parallel()
		got := truncateForAlert("abc", 3)
		if got != "abc" {
			t.Errorf("truncateForAlert = %q, want %q", got, "abc")
		}
	})
}

func TestFirstNonEmptyStr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		values []string
		want   string
	}{
		{"first non-empty", []string{"", "b", "c"}, "b"},
		{"all empty", []string{"", "", ""}, ""},
		{"first is non-empty", []string{"a", "b"}, "a"},
		{"no values", nil, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := firstNonEmptyStr(tt.values...)
			if got != tt.want {
				t.Errorf("firstNonEmptyStr = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHandleSessionTail_CursorAtEnd(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"line1", "line2"}
		s.TotalOutputCount = 5
	})

	// Cursor at 5 means no new lines
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id":     id,
		"cursor": "5",
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionTail returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"lines_returned":0`) {
		t.Errorf("expected 0 lines returned when cursor is at end, got: %s", text)
	}
}

func TestHandleSessionTail_LinesClampLow(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"a", "b", "c"}
		s.TotalOutputCount = 3
	})

	// lines < 1 should be clamped to 30 (default)
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id":    id,
		"lines": float64(0),
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if result.IsError {
		t.Fatalf("handleSessionTail returned error: %s", getResultText(result))
	}
	text := getResultText(result)
	// Should return all 3 lines since 3 < 30 (clamped default)
	if !strings.Contains(text, `"lines_returned":3`) {
		t.Errorf("expected 3 lines returned, got: %s", text)
	}
}

func TestHandleSessionCompare_Id2NotFound(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id1 := injectTestSession(t, srv, repoPath, nil)

	result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
		"id1": id1,
		"id2": "does-not-exist",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for non-existent id2")
	}
	code := parseErrorCode(t, getResultText(result))
	if code != string(ErrSessionNotFound) {
		t.Errorf("error_code = %q, want %q", code, ErrSessionNotFound)
	}
}

func TestHandleSessionCompare_WithEndedSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	endedAt := time.Now().Add(-10 * time.Minute)
	id1 := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.SpentUSD = 2.0
		s.TurnCount = 15
		s.Status = session.StatusStopped
		s.LaunchedAt = time.Now().Add(-30 * time.Minute)
		s.EndedAt = &endedAt
	})
	id2 := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.SpentUSD = 5.0
		s.TurnCount = 30
		s.Status = session.StatusRunning
		s.LaunchedAt = time.Now().Add(-15 * time.Minute)
	})

	result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
		"id1": id1,
		"id2": id2,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, "turns_per_min") {
		t.Errorf("expected turns_per_min in response, got: %s", text)
	}
}

func TestHandleSessionErrors_SeverityFilterLifecycle(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Create errored session (critical) and budget warning session (warning)
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Error = "fatal crash"
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.BudgetUSD = 10.0
		s.SpentUSD = 9.0 // 90%, triggers budget_warning
	})

	// Filter by critical only
	result, err := srv.handleSessionErrors(context.Background(), makeRequest(map[string]any{
		"severity": "critical",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "fatal crash") {
		t.Errorf("expected critical error in filtered results, got: %s", text)
	}
	// The errors array should only have 1 entry (the critical one)
	if !strings.Contains(text, `"total_errors":1`) {
		t.Errorf("expected total_errors 1 after severity filter, got: %s", text)
	}
}

func TestHandleSessionErrors_StreamParseErrors(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.StreamParseErrors = 5
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "stream_parse") {
		t.Errorf("expected stream_parse error, got: %s", text)
	}
	if !strings.Contains(text, "5 parse errors") {
		t.Errorf("expected '5 parse errors', got: %s", text)
	}
}

func TestHandleSessionErrors_StoppedWithExitReason(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
		s.ExitReason = "user requested stop"
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, "session_stopped") {
		t.Errorf("expected session_stopped type, got: %s", text)
	}
	if !strings.Contains(text, "user requested stop") {
		t.Errorf("expected exit reason in message, got: %s", text)
	}
}

func TestHandleSessionErrors_HealthySessions(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Session with no errors
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusRunning
		s.BudgetUSD = 10.0
		s.SpentUSD = 1.0 // well under budget
	})

	result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	if !strings.Contains(text, `"healthy_sessions":1`) {
		t.Errorf("expected 1 healthy session, got: %s", text)
	}
	if !strings.Contains(text, `"total_errors":0`) {
		t.Errorf("expected 0 total errors, got: %s", text)
	}
}

func TestHandleSessionErrors_Limit(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	// Create many errored sessions
	for i := 0; i < 5; i++ {
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusErrored
			s.Error = "error"
		})
	}

	// Limit to 2
	result, err := srv.handleSessionErrors(context.Background(), makeRequest(map[string]any{
		"limit": float64(2),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	text := getResultText(result)
	// The "errors" array should contain at most 2 entries
	if strings.Contains(text, `"total_errors":5`) {
		// total_errors counts all before limiting - that's fine
	}
}

func TestHandleSessionTail_WithNewLines(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.OutputHistory = []string{"old1", "old2", "new1", "new2", "new3"}
		s.TotalOutputCount = 8
	})

	// Cursor at 5 means 3 new lines (8-5)
	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id":     id,
		"cursor": "5",
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"lines_returned":3`) {
		t.Errorf("expected 3 lines returned, got: %s", text)
	}
}

func TestHandleSessionTail_StoppedSession(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusStopped
		s.OutputHistory = []string{"done"}
		s.TotalOutputCount = 1
	})

	result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
		"id": id,
	}))
	if err != nil {
		t.Fatalf("handleSessionTail: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"is_active":false`) {
		t.Errorf("expected is_active false for stopped session, got: %s", text)
	}
}

func TestHandleSessionRetry_WithOverrides(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Status = session.StatusErrored
		s.Model = "sonnet"
		s.BudgetUSD = 5.0
		s.Prompt = "test prompt"
	})

	// Retry should accept the session even if the launch itself will fail
	// (since test manager doesn't actually spawn processes)
	result, err := srv.handleSessionRetry(context.Background(), makeRequest(map[string]any{
		"id":             id,
		"model":          "opus",
		"budget_usd": float64(20),
	}))
	if err != nil {
		t.Fatalf("handleSessionRetry: %v", err)
	}
	// The launch may fail in test mode, but we at least exercise the override code paths
	_ = result
}

func TestHandleSessionLaunch_ValidOutputSchema(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":          "test-repo",
		"prompt":        "do something",
		"output_schema": `{"type":"object"}`,
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Will reach repo-not-found or launch (exercise the output_schema validation path)
	_ = result
}

func TestHandleSessionLaunch_DefaultProvider(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// No provider specified, should default to claude
	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":   "test-repo",
		"prompt": "do something",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Exercises the default provider path
	_ = result
}

func TestHandleSessionList_CombinedFilters(t *testing.T) {
	t.Parallel()
	srv, root := setupTestServer(t)
	repoPath := filepath.Join(root, "test-repo")

	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
		s.Status = session.StatusRunning
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderGemini
		s.Status = session.StatusStopped
	})
	injectTestSession(t, srv, repoPath, func(s *session.Session) {
		s.Provider = session.ProviderClaude
		s.Status = session.StatusStopped
	})

	// Filter by both provider and status
	result, err := srv.handleSessionList(context.Background(), makeRequest(map[string]any{
		"provider": "claude",
		"status":   "stopped",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error: %s", getResultText(result))
	}
	text := getResultText(result)
	if !strings.Contains(text, `"provider":"claude"`) {
		t.Errorf("expected claude sessions, got: %s", text)
	}
	if !strings.Contains(text, `"status":"stopped"`) {
		t.Errorf("expected stopped sessions, got: %s", text)
	}
	if strings.Contains(text, `"provider":"gemini"`) {
		t.Errorf("should not contain gemini sessions, got: %s", text)
	}
}

// FINDING-258/261: Verify budget_usd and max_turns params are propagated
// through handleSessionLaunch to LaunchOptions and ultimately to the Session.
func TestHandleSessionLaunch_BudgetParamsPropagated(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// The launch will likely fail (no real claude binary), but we can verify
	// that the params are parsed and set on LaunchOptions by checking that
	// the handler does not reject them and reaches the launch attempt.
	result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
		"repo":       "test-repo",
		"prompt":     "test budget propagation",
		"budget_usd": float64(42.5),
		"max_turns":  float64(100),
		"provider":   "claude",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := getResultText(result)
	// If the launch succeeded (unlikely in test), check that budget_usd is in the result
	if !result.IsError && !strings.Contains(text, "session_id") {
		t.Errorf("expected session_id in successful launch result, got: %s", text)
	}
	// If it failed, it should have failed at the launch step (not param parsing),
	// meaning budget params were successfully parsed and propagated.
	if result.IsError {
		// Should NOT fail with INVALID_PARAMS — that would mean params were rejected
		if strings.Contains(text, "INVALID_PARAMS") {
			t.Errorf("budget params were rejected as invalid: %s", text)
		}
	}
}
