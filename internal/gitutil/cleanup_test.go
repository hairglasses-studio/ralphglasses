package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// makeDir creates a subdirectory and optionally backdates it.
func makeDir(t *testing.T, base, name string, age time.Duration) string {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if age > 0 {
		past := time.Now().Add(-age)
		if err := os.Chtimes(dir, past, past); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// setupRepoWorktrees creates the standard worktree directory structure under a
// temp dir acting as repoPath.
func setupRepoWorktrees(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	os.MkdirAll(filepath.Join(repo, ".claude", "worktrees"), 0755)
	os.MkdirAll(filepath.Join(repo, ".ralph", "worktrees", "loops"), 0755)
	return repo
}

func TestCleanupStaleWorktrees_RemovesOldAgentDirs(t *testing.T) {
	repo := setupRepoWorktrees(t)
	makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-aaa111", 2*time.Hour)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_RemovesOldLoopDirs(t *testing.T) {
	repo := setupRepoWorktrees(t)
	makeDir(t, filepath.Join(repo, ".ralph", "worktrees", "loops"), "some-uuid-loop", 2*time.Hour)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Errorf("expected 1 removed, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_SkipsLockedDirs(t *testing.T) {
	repo := setupRepoWorktrees(t)
	dir := makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-locked1", 2*time.Hour)
	if err := os.WriteFile(filepath.Join(dir, ".lock"), []byte(""), 0644); err != nil {
		t.Fatal(err)
	}

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (locked), got %d", removed)
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		t.Error("locked directory should not have been removed")
	}
}

func TestCleanupStaleWorktrees_SkipsYoungDirs(t *testing.T) {
	repo := setupRepoWorktrees(t)
	makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-young1", 0)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (young), got %d", removed)
	}
}

func TestCleanupStaleWorktrees_SkipsNonAgentDirs(t *testing.T) {
	repo := setupRepoWorktrees(t)
	// In .claude/worktrees, non-agent-* dirs should be skipped.
	makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "worktree-abc", 2*time.Hour)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (non-agent), got %d", removed)
	}
}

func TestCleanupStaleWorktrees_MixedScenario(t *testing.T) {
	repo := setupRepoWorktrees(t)

	// Old agent dir (should be removed).
	makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-stale1", 2*time.Hour)

	// Old agent dir with lock (should be kept).
	locked := makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-locked", 2*time.Hour)
	os.WriteFile(filepath.Join(locked, ".lock"), []byte(""), 0644)

	// Young agent dir (should be kept).
	makeDir(t, filepath.Join(repo, ".claude", "worktrees"), "agent-young1", 0)

	// Old loop dir (should be removed).
	makeDir(t, filepath.Join(repo, ".ralph", "worktrees", "loops"), "uuid-old", 2*time.Hour)

	// Young loop dir (should be kept).
	makeDir(t, filepath.Join(repo, ".ralph", "worktrees", "loops"), "uuid-new", 0)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 2 {
		t.Errorf("expected 2 removed, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_EmptyDir(t *testing.T) {
	repo := setupRepoWorktrees(t)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed for empty dir, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_MissingSubdirs(t *testing.T) {
	// repoPath exists but has no .claude or .ralph subdirectories.
	repo := t.TempDir()

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed for missing subdirs, got %d", removed)
	}
}

func TestCleanupStaleWorktrees_LockedLoopDir(t *testing.T) {
	repo := setupRepoWorktrees(t)
	dir := makeDir(t, filepath.Join(repo, ".ralph", "worktrees", "loops"), "uuid-locked", 2*time.Hour)
	os.WriteFile(filepath.Join(dir, ".lock"), []byte(""), 0644)

	removed, err := CleanupStaleWorktrees(repo, 1*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if removed != 0 {
		t.Errorf("expected 0 removed (locked loop), got %d", removed)
	}
}

func TestCleanupOrphanedBranches(t *testing.T) {
	// We need a real git repo for git worktree prune to work.
	repo := t.TempDir()
	cmd := exec.Command("git", "init", repo)
	if err := cmd.Run(); err != nil {
		t.Skipf("git not available: %v", err)
	}

	count, err := CleanupOrphanedBranches(repo)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Errorf("expected 0 count, got %d", count)
	}
}

func TestDefaultMaxAge(t *testing.T) {
	if DefaultMaxAge != 24*time.Hour {
		t.Errorf("expected DefaultMaxAge to be 24h, got %v", DefaultMaxAge)
	}
}
