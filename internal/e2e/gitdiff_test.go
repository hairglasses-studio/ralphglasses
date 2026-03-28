package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// gitEnvForTest returns an isolated git environment for deterministic tests.
func gitEnvForTest(dir string) []string {
	return []string{
		"HOME=" + dir,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@test.com",
		"PATH=" + os.Getenv("PATH"),
	}
}

// initTestGitRepo creates a git repo with an initial committed file.
func initTestGitRepo(t *testing.T, dir string) {
	t.Helper()
	env := gitEnvForTest(dir)
	cmds := [][]string{
		{"git", "init"},
		{"git", "commit", "--allow-empty", "-m", "init"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
	// Add a real file and commit it
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "add readme"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v failed: %v\n%s", args, err, out)
		}
	}
}

func runGitInDir(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitEnvForTest(dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// ---------------------------------------------------------------------------
// GitDiffPaths
// ---------------------------------------------------------------------------

func TestGitDiffPaths_NoDiff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	paths, err := GitDiffPaths(dir)
	if err != nil {
		t.Fatalf("GitDiffPaths: %v", err)
	}
	if len(paths) != 0 {
		t.Errorf("expected 0 diff paths in clean repo, got %d: %v", len(paths), paths)
	}
}

func TestGitDiffPaths_WithChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	// Modify a tracked file
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# changed"), 0644); err != nil {
		t.Fatal(err)
	}

	paths, err := GitDiffPaths(dir)
	if err != nil {
		t.Fatalf("GitDiffPaths: %v", err)
	}
	if len(paths) != 1 {
		t.Errorf("expected 1 diff path, got %d: %v", len(paths), paths)
	}
	if len(paths) > 0 && paths[0] != "README.md" {
		t.Errorf("expected README.md, got %q", paths[0])
	}
}

func TestGitDiffPaths_MultipleFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	// Add and commit multiple files, then modify them
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("original"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	runGitInDir(t, dir, "add", ".")
	runGitInDir(t, dir, "commit", "-m", "add files")

	// Modify both
	for _, name := range []string{"a.txt", "b.txt"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("modified"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	paths, err := GitDiffPaths(dir)
	if err != nil {
		t.Fatalf("GitDiffPaths: %v", err)
	}
	if len(paths) != 2 {
		t.Errorf("expected 2 diff paths, got %d: %v", len(paths), paths)
	}
}

func TestGitDiffPaths_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	_, err := GitDiffPaths(dir)
	if err == nil {
		t.Error("expected error for non-git directory")
	}
}

// ---------------------------------------------------------------------------
// GitDiffStats
// ---------------------------------------------------------------------------

func TestGitDiffStats_NoDiff(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	files, added, removed := GitDiffStats(dir)
	if files != 0 || added != 0 || removed != 0 {
		t.Errorf("expected (0,0,0) for clean repo, got (%d,%d,%d)", files, added, removed)
	}
}

func TestGitDiffStats_WithChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initTestGitRepo(t, dir)

	// Modify a tracked file — add lines
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# changed\nnew line\n"), 0644); err != nil {
		t.Fatal(err)
	}

	files, added, removed := GitDiffStats(dir)
	if files == 0 {
		t.Error("expected at least 1 file changed")
	}
	if added == 0 && removed == 0 {
		t.Error("expected non-zero added or removed lines")
	}
}

func TestGitDiffStats_NotAGitRepo(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	files, added, removed := GitDiffStats(dir)
	// Should return zeros for non-git directory (graceful error handling)
	if files != 0 || added != 0 || removed != 0 {
		t.Errorf("expected (0,0,0) for non-git dir, got (%d,%d,%d)", files, added, removed)
	}
}
