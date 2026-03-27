package mcpserver

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// errorResponse is used to parse the structured error JSON from codedError.
type errorResponse struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
}

// parseErrorCode extracts the error_code from a CallToolResult that IsError.
func parseErrorCode(t *testing.T, result string) string {
	t.Helper()
	var resp errorResponse
	if err := json.Unmarshal([]byte(result), &resp); err != nil {
		t.Fatalf("failed to parse error response: %v\nraw: %s", err, result)
	}
	return resp.ErrorCode
}

func TestHandleSessionLaunch(t *testing.T) {
	t.Parallel()

	t.Run("missing repo param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"prompt": "do something",
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

	t.Run("missing prompt param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"repo": "test-repo",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing prompt")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("invalid provider", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"repo":     "test-repo",
			"prompt":   "do something",
			"provider": "invalid-provider-xyz",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid provider")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrProviderUnavailable) {
			t.Errorf("error_code = %q, want %q", code, ErrProviderUnavailable)
		}
	})

	t.Run("invalid repo name", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"repo":   "../escape-attempt",
			"prompt": "do something",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid repo name")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrRepoNameInvalid) {
			t.Errorf("error_code = %q, want %q", code, ErrRepoNameInvalid)
		}
	})

	t.Run("repo not found", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"repo":   "nonexistent-repo",
			"prompt": "do something",
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

	t.Run("invalid output_schema", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionLaunch(context.Background(), makeRequest(map[string]any{
			"repo":          "test-repo",
			"prompt":        "do something",
			"output_schema": "not valid json{{{",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid output_schema")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})
}

func TestHandleSessionStatus(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionStatus(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionStatus(context.Background(), makeRequest(map[string]any{
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

	t.Run("existing session", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Model = "opus"
			s.BudgetUSD = 10.0
			s.SpentUSD = 2.5
			s.TurnCount = 5
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
		if !strings.Contains(text, id) {
			t.Errorf("expected session ID in response, got: %s", text)
		}
		if !strings.Contains(text, `"model":"opus"`) {
			t.Errorf("expected model opus in response, got: %s", text)
		}
		if !strings.Contains(text, `"turns":5`) {
			t.Errorf("expected turns 5 in response, got: %s", text)
		}
	})
}

func TestHandleSessionList(t *testing.T) {
	t.Parallel()

	t.Run("empty list", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionList(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "[]") {
			t.Errorf("expected empty array, got: %s", text)
		}
	})

	t.Run("with sessions", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, nil)

		result, err := srv.handleSessionList(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, id) {
			t.Errorf("expected session ID %s in list, got: %s", id, text)
		}
	})

	t.Run("filter by status", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusStopped
		})

		result, err := srv.handleSessionList(context.Background(), makeRequest(map[string]any{
			"status": "stopped",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, `"status":"stopped"`) {
			t.Errorf("expected stopped session in filtered list, got: %s", text)
		}
		if strings.Contains(text, `"status":"running"`) {
			t.Errorf("should not contain running sessions after filter, got: %s", text)
		}
	})
}

func TestHandleSessionStop(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionStop(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionStop(context.Background(), makeRequest(map[string]any{
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

func TestHandleSessionStopAll(t *testing.T) {
	t.Parallel()

	t.Run("no running sessions", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionStopAll(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "Stopped 0") {
			t.Errorf("expected 0 stopped, got: %s", text)
		}
	})

	t.Run("with running sessions", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusRunning
		})
		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.Status = session.StatusStopped
		})

		result, err := srv.handleSessionStopAll(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "Stopped 2") {
			t.Errorf("expected 2 stopped, got: %s", text)
		}
	})
}

func TestHandleSessionErrors(t *testing.T) {
	t.Parallel()

	t.Run("no sessions returns empty", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, `"total_errors":0`) {
			t.Errorf("expected 0 total errors, got: %s", text)
		}
	})

	t.Run("empty errors is array not null", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		// Verify errors is [] not null (FINDING-89)
		if strings.Contains(text, `"errors":null`) {
			t.Errorf("errors field is null, should be empty array []: %s", text)
		}
		if !strings.Contains(text, `"errors":[]`) {
			t.Errorf("expected errors:[], got: %s", text)
		}
	})

	t.Run("budget warning at 80 percent", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.BudgetUSD = 10.0
			s.SpentUSD = 8.5 // 85%
		})

		result, err := srv.handleSessionErrors(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		text := getResultText(result)
		if !strings.Contains(text, "budget_warning") {
			t.Errorf("expected budget_warning, got: %s", text)
		}
	})
}

func TestHandleSessionBudget(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionBudget(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionBudget(context.Background(), makeRequest(map[string]any{
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

	t.Run("get and set budget", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.BudgetUSD = 5.0
			s.SpentUSD = 1.0
		})

		// Set new budget
		result, err := srv.handleSessionBudget(context.Background(), makeRequest(map[string]any{
			"id":     id,
			"budget": float64(20),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, `"budget_usd":20`) {
			t.Errorf("expected updated budget 20, got: %s", text)
		}
		if !strings.Contains(text, `"remaining":19`) {
			t.Errorf("expected remaining 19, got: %s", text)
		}
	})
}

func TestHandleSessionOutput(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionOutput(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{
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

	t.Run("returns output history", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.OutputHistory = []string{"line-a", "line-b", "line-c"}
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
		if !strings.Contains(text, "line-a") || !strings.Contains(text, "line-c") {
			t.Errorf("expected output lines, got: %s", text)
		}
		if !strings.Contains(text, `"lines":3`) {
			t.Errorf("expected 3 lines, got: %s", text)
		}
	})
}

func TestHandleSessionTailErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionTail(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
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

	t.Run("returns tail with cursor", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.OutputHistory = []string{"x1", "x2", "x3"}
			s.TotalOutputCount = 10
		})

		result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
			"id": id,
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, `"next_cursor":"10"`) {
			t.Errorf("expected next_cursor 10, got: %s", text)
		}
		if !strings.Contains(text, `"is_active":true`) {
			t.Errorf("expected is_active true for running session, got: %s", text)
		}
	})

	t.Run("invalid cursor", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.OutputHistory = []string{"a"}
			s.TotalOutputCount = 1
		})

		result, err := srv.handleSessionTail(context.Background(), makeRequest(map[string]any{
			"id":     id,
			"cursor": "not-a-number",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for invalid cursor")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})
}

func TestHandleSessionDiffCodes(t *testing.T) {
	t.Parallel()

	t.Run("missing session_id", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionDiff(context.Background(), makeRequest(nil))
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

		result, err := srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{
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

	t.Run("valid session with git repo", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.LaunchedAt = time.Now().Add(-30 * time.Minute)
		})

		result, err := srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{
			"id": id,
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, "window") {
			t.Errorf("expected window field, got: %s", text)
		}
		if !strings.Contains(text, "commits") {
			t.Errorf("expected commits field, got: %s", text)
		}
	})
}

func TestHandleSessionCompare(t *testing.T) {
	t.Parallel()

	t.Run("missing both params", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionCompare(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing params")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("missing id2", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
			"id1": "session-a",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing id2")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("id1 not found no sessions", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{
			"id1": "not-found",
			"id2": "also-not-found",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for non-existent sessions")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrNoActiveSessions) {
			t.Errorf("error_code = %q, want %q", code, ErrNoActiveSessions)
		}
	})

	t.Run("valid comparison", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)
		repoPath := filepath.Join(root, "test-repo")

		id1 := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.SpentUSD = 1.5
			s.TurnCount = 10
		})
		id2 := injectTestSession(t, srv, repoPath, func(s *session.Session) {
			s.SpentUSD = 3.0
			s.TurnCount = 20
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
		if !strings.Contains(text, "session_1") || !strings.Contains(text, "session_2") {
			t.Errorf("expected session_1 and session_2 in response, got: %s", text)
		}
		if !strings.Contains(text, `"cost_per_turn"`) {
			t.Errorf("expected cost_per_turn field, got: %s", text)
		}
	})
}

func TestSessionToolsNoActiveSessions(t *testing.T) {
	t.Parallel()

	handlers := []struct {
		name string
		call func(*Server) (*mcp.CallToolResult, error)
	}{
		{"session_status", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionStatus(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_output", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_budget", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionBudget(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_tail", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionTail(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_diff", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_retry", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionRetry(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_stop", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionStop(context.Background(), makeRequest(map[string]any{"id": "nonexistent"}))
		}},
		{"session_compare", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionCompare(context.Background(), makeRequest(map[string]any{"id1": "a", "id2": "b"}))
		}},
	}

	for _, h := range handlers {
		h := h
		t.Run(h.name+"_no_sessions", func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)

			result, err := h.call(srv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error when no sessions exist")
			}
			code := parseErrorCode(t, getResultText(result))
			if code != string(ErrNoActiveSessions) {
				t.Errorf("error_code = %q, want %q", code, ErrNoActiveSessions)
			}
			text := getResultText(result)
			if !strings.Contains(text, "ralphglasses_session_launch") {
				t.Errorf("expected guidance to use session_launch, got: %s", text)
			}
		})
	}
}

func TestSessionToolsWithInvalidID(t *testing.T) {
	t.Parallel()

	handlers := []struct {
		name string
		call func(*Server) (*mcp.CallToolResult, error)
	}{
		{"session_status", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionStatus(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
		{"session_output", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionOutput(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
		{"session_budget", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionBudget(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
		{"session_tail", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionTail(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
		{"session_diff", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionDiff(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
		{"session_retry", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleSessionRetry(context.Background(), makeRequest(map[string]any{"id": "wrong-id"}))
		}},
	}

	for _, h := range handlers {
		h := h
		t.Run(h.name+"_invalid_id", func(t *testing.T) {
			t.Parallel()
			srv, root := setupTestServer(t)
			repoPath := filepath.Join(root, "test-repo")

			// Inject a session so the manager is not empty
			injectTestSession(t, srv, repoPath, nil)

			result, err := h.call(srv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error for invalid session ID")
			}
			code := parseErrorCode(t, getResultText(result))
			if code != string(ErrSessionNotFound) {
				t.Errorf("error_code = %q, want %q", code, ErrSessionNotFound)
			}
			text := getResultText(result)
			if !strings.Contains(text, "ralphglasses_session_list") {
				t.Errorf("expected guidance to use session_list, got: %s", text)
			}
		})
	}
}

func TestFleetToolsNotRunning(t *testing.T) {
	t.Parallel()

	handlers := []struct {
		name string
		call func(*Server) (*mcp.CallToolResult, error)
	}{
		{"fleet_submit", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleFleetSubmit(context.Background(), makeRequest(map[string]any{"repo": "r", "prompt": "p"}))
		}},
		{"fleet_budget", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleFleetBudget(context.Background(), makeRequest(nil))
		}},
		{"fleet_workers", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleFleetWorkers(context.Background(), makeRequest(nil))
		}},
		{"fleet_dlq", func(srv *Server) (*mcp.CallToolResult, error) {
			return srv.handleFleetDLQ(context.Background(), makeRequest(nil))
		}},
	}

	for _, h := range handlers {
		h := h
		t.Run(h.name, func(t *testing.T) {
			t.Parallel()
			srv, _ := setupTestServer(t)

			result, err := h.call(srv)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.IsError {
				t.Fatal("expected error when fleet not running")
			}
			code := parseErrorCode(t, getResultText(result))
			if code != string(ErrFleetNotRunning) {
				t.Errorf("error_code = %q, want %q", code, ErrFleetNotRunning)
			}
			text := getResultText(result)
			if !strings.Contains(text, "ralphglasses mcp --fleet") {
				t.Errorf("expected guidance to start fleet, got: %s", text)
			}
		})
	}
}
