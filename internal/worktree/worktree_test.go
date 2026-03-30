package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initTestRepo creates a bare-minimum git repo with one commit, suitable for
// worktree operations. Returns the repo path. The repo is created inside
// t.TempDir() so cleanup is automatic.
func initTestRepo(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	// Resolve symlinks so paths match git's resolved output (macOS: /var -> /private/var).
	dir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	repo := filepath.Join(dir, "repo")

	cmds := [][]string{
		{"git", "init", repo},
		{"git", "-C", repo, "config", "user.email", "test@test.com"},
		{"git", "-C", repo, "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Env = gitEnv(t)
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
		cmd.Env = gitEnv(t)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}

	return repo
}

// gitEnv returns a minimal environment for git commands that avoids
// interference from the user's global git config.
func gitEnv(t *testing.T) []string {
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

// runGit is a test helper that runs a git command and fails the test on error.
func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Env = gitEnv(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestCreate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt1")
	err := Create(ctx, repo, wtPath, "feature-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Verify the worktree directory exists.
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree directory not created: %v", statErr)
	}

	// Verify the branch was created.
	branch := runGit(t, wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	if branch != "feature-1" {
		t.Errorf("expected branch feature-1, got %q", branch)
	}
}

func TestCreateDuplicateBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	wtPath1 := filepath.Join(t.TempDir(), "wt1")
	if err := Create(ctx, repo, wtPath1, "dup-branch"); err != nil {
		t.Fatalf("first Create: %v", err)
	}

	// Second create with the same branch name should fail.
	wtPath2 := filepath.Join(t.TempDir(), "wt2")
	err := Create(ctx, repo, wtPath2, "dup-branch")
	if err == nil {
		t.Fatal("expected error creating worktree with duplicate branch, got nil")
	}
	if !strings.Contains(err.Error(), "worktree: create") {
		t.Errorf("error should contain 'worktree: create', got: %v", err)
	}
}

func TestList(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	// List with just the main worktree.
	entries, err := List(ctx, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) < 1 {
		t.Fatal("expected at least 1 worktree entry (main)")
	}

	// Verify main entry.
	main := entries[0]
	if main.Path != repo {
		t.Errorf("main worktree path = %q, want %q", main.Path, repo)
	}
	if main.HEAD == "" {
		t.Error("main worktree HEAD is empty")
	}
	if main.IsDetached {
		t.Error("main worktree should not be detached")
	}

	// Add a worktree and list again.
	wtBase, _ := filepath.EvalSymlinks(t.TempDir())
	wtPath := filepath.Join(wtBase, "wt-list")
	if err := Create(ctx, repo, wtPath, "list-branch"); err != nil {
		t.Fatalf("Create for list test: %v", err)
	}

	entries, err = List(ctx, repo)
	if err != nil {
		t.Fatalf("List after add: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 worktree entries, got %d", len(entries))
	}

	// Find the added worktree.
	var found bool
	for _, e := range entries {
		if e.Path == wtPath {
			found = true
			if !strings.HasSuffix(e.Branch, "list-branch") {
				t.Errorf("expected branch ending in list-branch, got %q", e.Branch)
			}
		}
	}
	if !found {
		t.Errorf("added worktree %q not found in list", wtPath)
	}
}

func TestRemove(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt-remove")
	if err := Create(ctx, repo, wtPath, "remove-branch"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Remove without force.
	err := Remove(ctx, repo, wtPath, false)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}

	// Directory should be gone.
	if _, statErr := os.Stat(wtPath); !os.IsNotExist(statErr) {
		t.Fatalf("worktree directory still exists after remove")
	}

	// Listing should show only the main worktree.
	entries, err := List(ctx, repo)
	if err != nil {
		t.Fatalf("List after remove: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 worktree after remove, got %d", len(entries))
	}
}

func TestRemoveForce(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	wtPath := filepath.Join(t.TempDir(), "wt-force")
	if err := Create(ctx, repo, wtPath, "force-branch"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Create an untracked file to make the worktree dirty.
	if err := os.WriteFile(filepath.Join(wtPath, "dirty.txt"), []byte("dirty"), 0644); err != nil {
		t.Fatal(err)
	}

	// Force remove should succeed even with dirty worktree.
	err := Remove(ctx, repo, wtPath, true)
	if err != nil {
		t.Fatalf("Remove --force: %v", err)
	}
}

func TestRemoveNonExistent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	err := Remove(ctx, repo, "/nonexistent/worktree", false)
	if err == nil {
		t.Fatal("expected error removing non-existent worktree")
	}
}

func TestPrune(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	// Create a worktree and then manually delete its directory.
	wtPath := filepath.Join(t.TempDir(), "wt-prune")
	if err := Create(ctx, repo, wtPath, "prune-branch"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Manually remove the directory (simulating a stale worktree).
	if err := os.RemoveAll(wtPath); err != nil {
		t.Fatal(err)
	}

	// Before prune, list should still show the stale entry.
	entries, err := List(ctx, repo)
	if err != nil {
		t.Fatalf("List before prune: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries before prune (including stale), got %d", len(entries))
	}

	// Prune should clean up the stale reference.
	if err := Prune(ctx, repo); err != nil {
		t.Fatalf("Prune: %v", err)
	}

	// After prune, only the main worktree should remain.
	entries, err = List(ctx, repo)
	if err != nil {
		t.Fatalf("List after prune: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after prune, got %d", len(entries))
	}
}

func TestCreateListRemoveLifecycle(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	// Create multiple worktrees.
	paths := make([]string, 3)
	for i := 0; i < 3; i++ {
		paths[i] = filepath.Join(t.TempDir(), "wt-lifecycle")
		branch := "lifecycle-" + strings.Repeat("a", i+1) // lifecycle-a, lifecycle-aa, lifecycle-aaa
		if err := Create(ctx, repo, paths[i], branch); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}

	// List should show 4 entries (main + 3 worktrees).
	entries, err := List(ctx, repo)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(entries))
	}

	// Remove all added worktrees.
	for _, p := range paths {
		if err := Remove(ctx, repo, p, false); err != nil {
			t.Fatalf("Remove %q: %v", p, err)
		}
	}

	// List should show only main.
	entries, err = List(ctx, repo)
	if err != nil {
		t.Fatalf("List after removal: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after removing all, got %d", len(entries))
	}
}

func TestContextCancellation(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// All operations should fail with context error.
	err := Create(ctx, repo, filepath.Join(t.TempDir(), "wt"), "cancel-branch")
	if err == nil {
		t.Error("Create with cancelled context should fail")
	}

	_, err = List(ctx, repo)
	if err == nil {
		t.Error("List with cancelled context should fail")
	}

	err = Prune(ctx, repo)
	if err == nil {
		t.Error("Prune with cancelled context should fail")
	}
}

func TestParsePorcelain(t *testing.T) {
	// Test parsing with known porcelain output.
	input := `worktree /home/user/repo
HEAD abc123def456
branch refs/heads/main

worktree /home/user/repo-wt
HEAD 789012fed345
branch refs/heads/feature

`
	entries, err := parsePorcelain([]byte(input))
	if err != nil {
		t.Fatalf("parsePorcelain: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].Path != "/home/user/repo" {
		t.Errorf("entry 0 path = %q", entries[0].Path)
	}
	if entries[0].HEAD != "abc123def456" {
		t.Errorf("entry 0 HEAD = %q", entries[0].HEAD)
	}
	if entries[0].Branch != "refs/heads/main" {
		t.Errorf("entry 0 branch = %q", entries[0].Branch)
	}

	if entries[1].Path != "/home/user/repo-wt" {
		t.Errorf("entry 1 path = %q", entries[1].Path)
	}
	if entries[1].Branch != "refs/heads/feature" {
		t.Errorf("entry 1 branch = %q", entries[1].Branch)
	}
}

func TestParsePorcelainDetached(t *testing.T) {
	input := `worktree /repo
HEAD aaa111
detached

`
	entries, err := parsePorcelain([]byte(input))
	if err != nil {
		t.Fatalf("parsePorcelain: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].IsDetached {
		t.Error("expected detached = true")
	}
	if entries[0].Branch != "" {
		t.Errorf("detached entry should have empty branch, got %q", entries[0].Branch)
	}
}

func TestParsePorcelainBare(t *testing.T) {
	input := `worktree /repo.git
HEAD bbb222
bare

`
	entries, err := parsePorcelain([]byte(input))
	if err != nil {
		t.Fatalf("parsePorcelain: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if !entries[0].IsBare {
		t.Error("expected bare = true")
	}
}

func TestParsePorcelainNoTrailingNewline(t *testing.T) {
	// Some git versions omit the trailing blank line.
	input := `worktree /repo
HEAD ccc333
branch refs/heads/main`

	entries, err := parsePorcelain([]byte(input))
	if err != nil {
		t.Fatalf("parsePorcelain: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Path != "/repo" {
		t.Errorf("path = %q", entries[0].Path)
	}
}

func TestParsePorcelainEmpty(t *testing.T) {
	entries, err := parsePorcelain([]byte(""))
	if err != nil {
		t.Fatalf("parsePorcelain empty: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(entries))
	}
}

// --- Merge tests ---

func TestMergeBackSuccess(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	// Determine the main branch name.
	mainBranch := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	// Create a feature branch with a commit.
	runGit(t, repo, "checkout", "-b", "feature-merge")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("feature\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "add feature")

	// Switch back to main.
	runGit(t, repo, "checkout", mainBranch)

	result, err := MergeBack(ctx, repo, "feature-merge", mainBranch)
	if err != nil {
		t.Fatalf("MergeBack: %v", err)
	}
	if !result.Merged {
		t.Fatalf("expected successful merge, got conflicts: %v", result.ConflictFiles)
	}
	if result.CommitHash == "" {
		t.Error("expected non-empty commit hash")
	}
	if len(result.ConflictFiles) != 0 {
		t.Errorf("expected no conflict files, got %v", result.ConflictFiles)
	}

	// Verify the feature file exists on main after merge.
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Errorf("feature.txt should exist after merge: %v", err)
	}
}

func TestMergeBackConflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	mainBranch := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	// Create conflicting changes on two branches.
	// First, modify README.md on a feature branch.
	runGit(t, repo, "checkout", "-b", "conflict-branch")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("feature version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "feature change to README")

	// Then, modify README.md on main.
	runGit(t, repo, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "add", ".")
	runGit(t, repo, "commit", "-m", "main change to README")

	result, err := MergeBack(ctx, repo, "conflict-branch", mainBranch)
	if err != nil {
		t.Fatalf("MergeBack should not return error on conflict, got: %v", err)
	}
	if result.Merged {
		t.Fatal("expected merge to fail due to conflict")
	}
	if len(result.ConflictFiles) == 0 {
		t.Fatal("expected at least one conflict file")
	}

	// README.md should be in the conflict list.
	var foundReadme bool
	for _, f := range result.ConflictFiles {
		if f == "README.md" {
			foundReadme = true
		}
	}
	if !foundReadme {
		t.Errorf("expected README.md in conflict files, got %v", result.ConflictFiles)
	}

	// Verify the merge was aborted — repo should be clean.
	hasConflict, _, err := HasConflicts(ctx, repo)
	if err != nil {
		t.Fatalf("HasConflicts after abort: %v", err)
	}
	if hasConflict {
		t.Error("repo should be clean after merge abort")
	}
}

func TestMergeBackNonExistentBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	mainBranch := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD")

	_, err := MergeBack(ctx, repo, "nonexistent-branch", mainBranch)
	if err == nil {
		t.Fatal("expected error merging non-existent branch")
	}
}

func TestHasConflictsCleanRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	hasConflict, files, err := HasConflicts(ctx, repo)
	if err != nil {
		t.Fatalf("HasConflicts: %v", err)
	}
	if hasConflict {
		t.Errorf("clean repo should not have conflicts, got files: %v", files)
	}
}

func TestCreateFromRef(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repo := initTestRepo(t)
	ctx := context.Background()

	// Get the current HEAD commit.
	head := runGit(t, repo, "rev-parse", "HEAD")

	wtPath := filepath.Join(t.TempDir(), "wt-fromref")
	err := CreateFromRef(ctx, repo, wtPath, head)
	if err != nil {
		t.Fatalf("CreateFromRef: %v", err)
	}

	// The worktree should exist and be in detached HEAD state.
	if _, statErr := os.Stat(wtPath); statErr != nil {
		t.Fatalf("worktree directory not created: %v", statErr)
	}

	// Verify the HEAD matches.
	wtHead := runGit(t, wtPath, "rev-parse", "HEAD")
	if wtHead != head {
		t.Errorf("worktree HEAD = %q, want %q", wtHead, head)
	}
}
