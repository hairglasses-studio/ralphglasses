package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func TestHandleWorktreeCleanup_MissingRepo(t *testing.T) {
	srv := newTestServer(t.TempDir())
	result, err := srv.handleWorktreeCleanup(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for missing repo param")
	}
}

func TestHandleWorktreeCleanup_RepoNotFound(t *testing.T) {
	srv := newTestServer(t.TempDir())
	srv.Repos = []*model.Repo{{Name: "other", Path: t.TempDir()}}
	result, err := srv.handleWorktreeCleanup(context.Background(), makeRequest(map[string]any{
		"repo": "nonexistent",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error result for repo not found")
	}
}

func TestHandleWorktreeCleanup_NoWorktrees(t *testing.T) {
	dir := t.TempDir()
	ralphDir := filepath.Join(dir, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srv := newTestServer(t.TempDir())
	srv.Repos = []*model.Repo{{Name: "testrepo", Path: dir}}

	result, err := srv.handleWorktreeCleanup(context.Background(), makeRequest(map[string]any{
		"repo": "testrepo",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}
	text := getResultText(result)
	if text == "" {
		t.Fatal("expected non-empty result text")
	}
}

func TestHandleWorktreeCleanup_CleansOldWorktrees(t *testing.T) {
	dir := t.TempDir()
	worktreeDir := filepath.Join(dir, ".ralph", "worktrees", "loops", "old-loop")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Set the directory modification time to 48 hours ago.
	oldTime := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(worktreeDir, oldTime, oldTime)

	srv := newTestServer(t.TempDir())
	srv.Repos = []*model.Repo{{Name: "testrepo", Path: dir}}

	result, err := srv.handleWorktreeCleanup(context.Background(), makeRequest(map[string]any{
		"repo":          "testrepo",
		"max_age_hours": float64(24),
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected error result: %v", result)
	}

	// Verify the old worktree directory was cleaned up.
	if _, err := os.Stat(worktreeDir); !os.IsNotExist(err) {
		t.Error("expected old worktree directory to be removed")
	}
}
