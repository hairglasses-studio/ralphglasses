package gitutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupStaleWorktrees_RemovesOldAgentDirs(t *testing.T) {
	base := t.TempDir()

	// Create an old agent directory (older than maxAge).
	old := filepath.Join(base, "agent-aaa111")
	if err := os.Mkdir(old, 0755); err != nil {
		t.Fatal(err)
	}
	// Backdate its modification time.
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Error("expected old directory to be removed")
	}
}

func TestCleanupStaleWorktrees_SkipsLockedDirs(t *testing.T) {
	base := t.TempDir()

	// Create an old agent directory with a .lock file.
	locked := filepath.Join(base, "agent-locked1")
	if err := os.Mkdir(locked, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, ".lock"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(locked, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (locked), got %d", removed)
	}
	if _, err := os.Stat(locked); os.IsNotExist(err) {
		t.Error("locked directory should not have been removed")
	}
}

func TestCleanupStaleWorktrees_SkipsYoungDirs(t *testing.T) {
	base := t.TempDir()

	// Create a fresh agent directory (younger than maxAge).
	young := filepath.Join(base, "agent-young1")
	if err := os.Mkdir(young, 0755); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (young), got %d", removed)
	}
}

func TestCleanupStaleWorktrees_SkipsNonAgentDirs(t *testing.T) {
	base := t.TempDir()

	// Create an old non-agent directory.
	other := filepath.Join(base, "worktree-abc")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(other, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (non-agent), got %d", removed)
	}
}

func TestCleanupStaleWorktrees_MixedScenario(t *testing.T) {
	base := t.TempDir()
	past := time.Now().Add(-2 * time.Hour)

	// Old agent dir (should be removed).
	stale := filepath.Join(base, "agent-stale1")
	if err := os.Mkdir(stale, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(stale, past, past); err != nil {
		t.Fatal(err)
	}

	// Old agent dir with lock (should be kept).
	locked := filepath.Join(base, "agent-locked")
	if err := os.Mkdir(locked, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(locked, ".lock"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(locked, past, past); err != nil {
		t.Fatal(err)
	}

	// Young agent dir (should be kept).
	young := filepath.Join(base, "agent-young1")
	if err := os.Mkdir(young, 0755); err != nil {
		t.Fatal(err)
	}

	// Non-agent dir (should be kept).
	other := filepath.Join(base, "other-dir")
	if err := os.Mkdir(other, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(other, past, past); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_EmptyDir(t *testing.T) {
	base := t.TempDir()

	removed, err := CleanupStaleWorktrees(base, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed for empty dir, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_NonexistentDir(t *testing.T) {
	_, err := CleanupStaleWorktrees("/nonexistent/path/xyz", 1*time.Hour)
	if err == nil {
		t.Error("expected error for nonexistent directory")
	}
}
