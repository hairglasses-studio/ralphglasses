package mcpserver

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleRepoScaffold_EdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("missing path param", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(nil))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for missing path")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("path outside scan root", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
			"path": "/etc/should-not-work",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for path outside scan root")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("path with shell metacharacters", func(t *testing.T) {
		t.Parallel()
		srv, _ := setupTestServer(t)

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
			"path": "/tmp/evil;rm -rf /",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for path with metacharacters")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})

	t.Run("valid scaffold call", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)

		newRepoPath := filepath.Join(root, "new-project")

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
			"path":         newRepoPath,
			"project_type": "go",
			"project_name": "new-project",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, newRepoPath) {
			t.Errorf("expected repo path in result, got: %s", text)
		}
	})

	t.Run("valid scaffold with force", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)

		repoPath := filepath.Join(root, "test-repo")

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
			"path":  repoPath,
			"force": "true",
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.IsError {
			t.Fatalf("unexpected error: %s", getResultText(result))
		}
		text := getResultText(result)
		if !strings.Contains(text, repoPath) {
			t.Errorf("expected repo path in result, got: %s", text)
		}
	})

	t.Run("path traversal attempt", func(t *testing.T) {
		t.Parallel()
		srv, root := setupTestServer(t)

		result, err := srv.handleRepoScaffold(context.Background(), makeRequest(map[string]any{
			"path": filepath.Join(root, "..", "escape-attempt"),
		}))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.IsError {
			t.Fatal("expected error for path traversal")
		}
		code := parseErrorCode(t, getResultText(result))
		if code != string(ErrInvalidParams) {
			t.Errorf("error_code = %q, want %q", code, ErrInvalidParams)
		}
	})
}
