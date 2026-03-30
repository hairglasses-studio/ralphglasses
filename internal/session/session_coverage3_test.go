package session

import (
	"os"
	"path/filepath"
	"testing"
)

// ---------------------------------------------------------------------------
// TeamSafetyError.Error (0%)
// ---------------------------------------------------------------------------

func TestTeamSafetyError_Error(t *testing.T) {
	t.Parallel()
	e := &TeamSafetyError{
		Check:   "max_team_size",
		Message: "team exceeds 10 members",
	}
	got := e.Error()
	if got != "team safety: max_team_size: team exceeds 10 members" {
		t.Errorf("Error() = %q", got)
	}
}

func TestTeamSafetyError_Error_Formatting(t *testing.T) {
	t.Parallel()
	e := &TeamSafetyError{Check: "nesting", Message: "depth 4 > 3"}
	s := e.Error()
	if s == "" {
		t.Error("Error() should return non-empty string")
	}
}

// ---------------------------------------------------------------------------
// ListWorktrees (0%)
// ---------------------------------------------------------------------------

func TestListWorktrees_NoDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(wts))
	}
}

func TestListWorktrees_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, ".ralph", "worktrees", "loops")
	os.MkdirAll(base, 0755)

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees, got %d", len(wts))
	}
}

func TestListWorktrees_SkipsFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, ".ralph", "worktrees", "loops")
	os.MkdirAll(base, 0755)
	// Create a file (not directory) under loops -- should be skipped.
	os.WriteFile(filepath.Join(base, "not-a-dir.txt"), []byte("x"), 0644)

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 0 {
		t.Errorf("expected 0 worktrees (files skipped), got %d", len(wts))
	}
}

func TestListWorktrees_WithLoopDirs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	base := filepath.Join(dir, ".ralph", "worktrees", "loops")
	// Create a loop dir with an iteration subdir.
	iterDir := filepath.Join(base, "loop-1", "iter-1")
	os.MkdirAll(iterDir, 0755)
	// Also add a non-dir file inside loop-1 to verify it's skipped.
	os.WriteFile(filepath.Join(base, "loop-1", "status.json"), []byte("{}"), 0644)

	wts, err := ListWorktrees(dir)
	if err != nil {
		t.Fatalf("ListWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Loop != "loop-1" {
		t.Errorf("loop = %q, want loop-1", wts[0].Loop)
	}
	if wts[0].Path != iterDir {
		t.Errorf("path = %q, want %q", wts[0].Path, iterDir)
	}
}

// ---------------------------------------------------------------------------
// CreateWorktree — error paths (0%)
// ---------------------------------------------------------------------------

func TestCreateWorktree_EmptyRepoPath(t *testing.T) {
	t.Parallel()
	_, _, err := CreateWorktree("", "test")
	if err == nil {
		t.Error("CreateWorktree should fail with empty repo path")
	}
}

func TestCreateWorktree_EmptyName(t *testing.T) {
	t.Parallel()
	_, _, err := CreateWorktree(t.TempDir(), "")
	if err == nil {
		t.Error("CreateWorktree should fail with empty name")
	}
}

func TestCreateWorktree_InvalidName(t *testing.T) {
	t.Parallel()
	_, _, err := CreateWorktree(t.TempDir(), "   ")
	if err == nil {
		t.Error("CreateWorktree should fail with whitespace-only name")
	}
}

func TestCreateWorktree_AlreadyExists(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// Pre-create the worktree path so it "already exists".
	wtPath := filepath.Join(dir, ".ralph", "worktrees", "manual", "existing")
	os.MkdirAll(wtPath, 0755)

	_, _, err := CreateWorktree(dir, "existing")
	if err == nil {
		t.Error("CreateWorktree should fail when worktree already exists")
	}
}

// ---------------------------------------------------------------------------
// SetAutonomyLevel — level < LevelAutoOptimize path (partial 0%)
// ---------------------------------------------------------------------------

func TestManager_SetAutonomyLevel_BelowAutoOptimize(t *testing.T) {
	t.Parallel()
	m := NewManager()
	// Setting level below LevelAutoOptimize should not start a supervisor.
	m.SetAutonomyLevel(LevelObserve, t.TempDir())
	if m.SupervisorStatus() != nil {
		t.Error("supervisor should not start at LevelObserve")
	}
}

func TestManager_SetAutonomyLevel_StopsSupervisor(t *testing.T) {
	t.Parallel()
	m := NewManager()
	dir := t.TempDir()
	// Start at a high level then reduce.
	m.SetAutonomyLevel(LevelAutoOptimize, dir)
	// Now reduce level — should stop supervisor.
	m.SetAutonomyLevel(LevelObserve, dir)
	if m.SupervisorStatus() != nil {
		t.Error("supervisor should be stopped after lowering autonomy level")
	}
}
