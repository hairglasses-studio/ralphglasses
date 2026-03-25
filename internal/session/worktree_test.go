package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupLoopWorktrees(t *testing.T) {
	tmp := t.TempDir()
	loopID := "loop-abc123"
	dir := filepath.Join(tmp, ".ralph", "worktrees", "loops", loopID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Place a file inside to confirm recursive removal.
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := CleanupLoopWorktrees(tmp, loopID); err != nil {
		t.Fatalf("CleanupLoopWorktrees returned error: %v", err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected dir to be removed, got stat err: %v", err)
	}
}

func TestCleanupLoopWorktrees_NonExistent(t *testing.T) {
	tmp := t.TempDir()
	// Should succeed even when the directory doesn't exist.
	if err := CleanupLoopWorktrees(tmp, "no-such-loop"); err != nil {
		t.Fatalf("expected nil error for non-existent dir, got: %v", err)
	}
}

func TestCleanupStaleWorktrees(t *testing.T) {
	tmp := t.TempDir()
	base := filepath.Join(tmp, ".ralph", "worktrees", "loops")

	// Create an "old" directory and a "new" directory.
	oldDir := filepath.Join(base, "old-loop")
	newDir := filepath.Join(base, "new-loop")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Set the old directory's mod time to 48 hours ago.
	oldTime := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	cleaned, err := CleanupStaleWorktrees(tmp, 24*time.Hour)
	if err != nil {
		t.Fatalf("CleanupStaleWorktrees returned error: %v", err)
	}
	if cleaned != 1 {
		t.Fatalf("expected 1 cleaned, got %d", cleaned)
	}

	// Old should be gone, new should remain.
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("old dir should be removed")
	}
	if _, err := os.Stat(newDir); err != nil {
		t.Fatalf("new dir should still exist: %v", err)
	}
}

func TestCleanupStaleWorktrees_NoDir(t *testing.T) {
	tmp := t.TempDir()
	// Base directory doesn't exist — should return 0 with no error.
	cleaned, err := CleanupStaleWorktrees(tmp, 24*time.Hour)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
	if cleaned != 0 {
		t.Fatalf("expected 0 cleaned, got %d", cleaned)
	}
}
