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

	t.Run("non-existent session", func(t *testing.T) {
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
		if code != string(ErrSessionNotFound) {
			t.Errorf("error_code = %q, want %q", code, ErrSessionNotFound)
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
