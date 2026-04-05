package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

// initTestRepoForIntegration creates a minimal git repo with one commit.
// Returns the resolved repo path. Cleanup is handled by t.TempDir().
func initTestRepoForIntegration(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	// Resolve symlinks so paths match git's resolved output (macOS: /var -> /private/var).
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")

	env := integrationGitEnv(t)

	for _, args := range [][]string{
		{"git", "init", repo},
		{"git", "-C", repo, "config", "user.email", "test@test.com"},
		{"git", "-C", repo, "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	// Create an initial commit so HEAD exists.
	readme := filepath.Join(repo, "README.md")
	if err := os.WriteFile(readme, []byte("# test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "-C", repo, "add", "."},
		{"git", "-C", repo, "commit", "-m", "initial commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	return repo
}

// integrationGitEnv returns a minimal environment for git commands that avoids
// interference from the user's global git config.
func integrationGitEnv(t *testing.T) []string {
	t.Helper()
	return append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"HOME="+t.TempDir(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
	)
}

func TestWorktreeManager_CreateForSession(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	wm := NewWorktreeManager(baseDir)

	wtPath, err := wm.CreateForSession("sess-001", repo)
	if err != nil {
		t.Fatalf("CreateForSession: %v", err)
	}

	// Verify the worktree directory exists.
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree directory not created: %v", statErr)
	}

	// Verify it is under the configured baseDir.
	if !strings.HasPrefix(wtPath, baseDir) {
		t.Errorf("worktree path %q should be under baseDir %q", wtPath, baseDir)
	}

	// Verify the branch is checked out.
	cmd := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	cmd.Env = integrationGitEnv(t)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	branch := strings.TrimSpace(string(out))
	if branch != "ralph/session/sess-001" {
		t.Errorf("expected branch ralph/session/sess-001, got %q", branch)
	}
}

func TestWorktreeManager_CreateForSession_DefaultPath(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	wm := NewWorktreeManager("") // no baseDir override

	wtPath, err := wm.CreateForSession("sess-default", repo)
	if err != nil {
		t.Fatalf("CreateForSession: %v", err)
	}

	expected := filepath.Join(repo, ".ralph", "worktrees", "sessions", "sess-default")
	if wtPath != expected {
		t.Errorf("expected path %q, got %q", expected, wtPath)
	}

	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree directory not created: %v", statErr)
	}
}

func TestWorktreeManager_CreateForSession_DuplicateSession(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	wm := NewWorktreeManager(baseDir)

	_, err := wm.CreateForSession("sess-dup", repo)
	if err != nil {
		t.Fatalf("first CreateForSession: %v", err)
	}

	_, err = wm.CreateForSession("sess-dup", repo)
	if err == nil {
		t.Fatal("expected error for duplicate session, got nil")
	}
	if !strings.Contains(err.Error(), "already has an active worktree") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWorktreeManager_CreateForSession_EmptyInputs(t *testing.T) {
	wm := NewWorktreeManager("")

	_, err := wm.CreateForSession("", "/some/repo")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}

	_, err = wm.CreateForSession("sess-x", "")
	if err == nil {
		t.Fatal("expected error for empty repo path")
	}
}

func TestWorktreeManager_CleanupSession(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	wm := NewWorktreeManager(baseDir)

	wtPath, err := wm.CreateForSession("sess-cleanup", repo)
	if err != nil {
		t.Fatalf("CreateForSession: %v", err)
	}

	// Verify worktree exists.
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree should exist: %v", statErr)
	}

	// Cleanup.
	if err := wm.CleanupSession("sess-cleanup"); err != nil {
		t.Fatalf("CleanupSession: %v", err)
	}

	// Worktree directory should be gone.
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree directory should be removed, stat err: %v", statErr)
	}

	// Should no longer appear in active list.
	active := wm.ListActive()
	for _, a := range active {
		if a.SessionID == "sess-cleanup" {
			t.Fatal("cleaned-up session should not appear in ListActive")
		}
	}
}

func TestWorktreeManager_CleanupSession_NonExistent(t *testing.T) {
	wm := NewWorktreeManager("")

	// Cleaning up a session that was never created should succeed (no-op).
	if err := wm.CleanupSession("no-such-session"); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestWorktreeManager_CleanupSession_EmptyID(t *testing.T) {
	wm := NewWorktreeManager("")

	err := wm.CleanupSession("")
	if err == nil {
		t.Fatal("expected error for empty session ID")
	}
}

func TestWorktreeManager_ListActive(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	wm := NewWorktreeManager(baseDir)

	// Initially empty.
	if active := wm.ListActive(); len(active) != 0 {
		t.Fatalf("expected 0 active worktrees, got %d", len(active))
	}

	// Create two sessions.
	_, err := wm.CreateForSession("sess-a", repo)
	if err != nil {
		t.Fatalf("CreateForSession sess-a: %v", err)
	}
	_, err = wm.CreateForSession("sess-b", repo)
	if err != nil {
		t.Fatalf("CreateForSession sess-b: %v", err)
	}

	active := wm.ListActive()
	if len(active) != 2 {
		t.Fatalf("expected 2 active worktrees, got %d", len(active))
	}

	// Sort for deterministic assertions.
	sort.Slice(active, func(i, j int) bool {
		return active[i].SessionID < active[j].SessionID
	})

	if active[0].SessionID != "sess-a" {
		t.Errorf("expected sess-a, got %q", active[0].SessionID)
	}
	if active[1].SessionID != "sess-b" {
		t.Errorf("expected sess-b, got %q", active[1].SessionID)
	}

	// Verify fields are populated.
	for _, a := range active {
		if a.WorktreePath == "" {
			t.Errorf("session %s: WorktreePath is empty", a.SessionID)
		}
		if a.Branch == "" {
			t.Errorf("session %s: Branch is empty", a.SessionID)
		}
		if a.RepoPath != repo {
			t.Errorf("session %s: RepoPath = %q, want %q", a.SessionID, a.RepoPath, repo)
		}
		if a.CreatedAt.IsZero() {
			t.Errorf("session %s: CreatedAt is zero", a.SessionID)
		}
	}

	// Clean up one, verify list updates.
	_ = wm.CleanupSession("sess-a")
	active = wm.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active after cleanup, got %d", len(active))
	}
	if active[0].SessionID != "sess-b" {
		t.Errorf("expected remaining session sess-b, got %q", active[0].SessionID)
	}
}

func TestWorktreeManager_ConcurrentCreateAndCleanup(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepoForIntegration(t)
	baseDir := filepath.Join(t.TempDir(), "worktrees")
	wm := NewWorktreeManager(baseDir)

	const n = 5
	var wg sync.WaitGroup
	errs := make([]error, n)

	// Create n sessions concurrently.
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('a'+idx))
			_, errs[idx] = wm.CreateForSession(id, repo)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent create %d: %v", i, err)
		}
	}

	active := wm.ListActive()
	if len(active) != n {
		t.Fatalf("expected %d active worktrees, got %d", n, len(active))
	}

	// Cleanup all concurrently.
	for i := range n {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			id := "concurrent-" + string(rune('a'+idx))
			errs[idx] = wm.CleanupSession(id)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("concurrent cleanup %d: %v", i, err)
		}
	}

	if active := wm.ListActive(); len(active) != 0 {
		t.Fatalf("expected 0 active after full cleanup, got %d", len(active))
	}
}
