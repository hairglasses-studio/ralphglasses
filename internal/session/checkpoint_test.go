package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// initGitRepo creates a git repo in dir with an initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", dir},
		{"-C", dir, "config", "user.email", "test@test.com"},
		{"-C", dir, "config", "user.name", "Test"},
		{"-C", dir, "config", "commit.gpgsign", "false"},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", dir, "add", "."},
		{"-C", dir, "commit", "-m", "initial"},
	} {
		if err := exec.Command("git", args...).Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "log", "--oneline").Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return string(out)
}

func gitTags(t *testing.T, dir string) string {
	t.Helper()
	out, err := exec.Command("git", "-C", dir, "tag", "-l").Output()
	if err != nil {
		t.Fatalf("git tag -l: %v", err)
	}
	return string(out)
}

func TestCreateCheckpoint_CleanWorktree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Clean worktree — should be a no-op returning nil.
	if err := CreateCheckpoint(dir, 1, 0.50, 5); err != nil {
		t.Fatalf("expected nil for clean worktree, got: %v", err)
	}

	// No new commits beyond "initial".
	log := gitLog(t, dir)
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 1 {
		t.Errorf("expected 1 commit (initial), got %d: %s", len(lines), log)
	}

	// No tags.
	tags := strings.TrimSpace(gitTags(t, dir))
	if tags != "" {
		t.Errorf("expected no tags, got: %s", tags)
	}
}

func TestCreateCheckpoint_DirtyWorktree(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Make the worktree dirty.
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := CreateCheckpoint(dir, 5, 1.50, 10); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// Verify new commit exists with expected message.
	log := gitLog(t, dir)
	if !strings.Contains(log, "session checkpoint #5") {
		t.Errorf("commit message not found in log:\n%s", log)
	}

	// Verify tag exists matching expected pattern.
	tags := gitTags(t, dir)
	if !strings.Contains(tags, "session-checkpoint-5-") {
		t.Errorf("expected tag matching session-checkpoint-5-*, got:\n%s", tags)
	}
}

func TestCreateCheckpoint_ExcludesSensitiveFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Create a mix of normal and sensitive files.
	normalFiles := []string{"main.go", "lib/util.go"}
	sensitiveFiles := []string{
		".env",
		".env.production",
		"server.pem",
		"private.key",
		"credentials.json",
		"app-secret.yaml",
		"data.sqlite",
		"local.db",
		"cert.p12",
	}

	// Create subdirs as needed and write files.
	for _, f := range append(normalFiles, sensitiveFiles...) {
		full := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	if err := CreateCheckpoint(dir, 1, 0.10, 2); err != nil {
		t.Fatalf("CreateCheckpoint: %v", err)
	}

	// List files in the last commit.
	out, err := exec.Command("git", "-C", dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD").Output()
	if err != nil {
		t.Fatalf("git diff-tree: %v", err)
	}
	committed := string(out)

	// Normal files should be committed.
	for _, f := range normalFiles {
		if !strings.Contains(committed, f) {
			t.Errorf("expected normal file %q to be committed, but it was not.\nCommitted:\n%s", f, committed)
		}
	}

	// Sensitive files should NOT be committed.
	for _, f := range sensitiveFiles {
		if strings.Contains(committed, f) {
			t.Errorf("sensitive file %q was committed but should have been excluded.\nCommitted:\n%s", f, committed)
		}
	}

	// Sensitive files should still be untracked in the working tree.
	statusOut, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
	if err != nil {
		t.Fatalf("git status: %v", err)
	}
	status := string(statusOut)
	if !strings.Contains(status, ".env") {
		t.Errorf("expected .env to remain untracked, status:\n%s", status)
	}
}

func TestCreateCheckpoint_InvalidPath(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // not a git repo

	err := CreateCheckpoint(dir, 1, 0, 0)
	if err == nil {
		t.Fatal("expected error for non-git directory, got nil")
	}
	if !strings.Contains(err.Error(), "git status") {
		t.Errorf("expected git status error, got: %v", err)
	}
}

func TestCreateCheckpoint_MultipleCheckpoints(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// First dirty + checkpoint cycle.
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CreateCheckpoint(dir, 1, 0.50, 3); err != nil {
		t.Fatalf("first checkpoint: %v", err)
	}

	// Second dirty + checkpoint cycle.
	if err := os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bbb"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := CreateCheckpoint(dir, 2, 1.25, 7); err != nil {
		t.Fatalf("second checkpoint: %v", err)
	}

	// Both commits should exist.
	log := gitLog(t, dir)
	if !strings.Contains(log, "session checkpoint #1") {
		t.Errorf("first checkpoint commit not found in log:\n%s", log)
	}
	if !strings.Contains(log, "session checkpoint #2") {
		t.Errorf("second checkpoint commit not found in log:\n%s", log)
	}

	// Both tags should exist.
	tags := gitTags(t, dir)
	if !strings.Contains(tags, "session-checkpoint-1-") {
		t.Errorf("first tag not found:\n%s", tags)
	}
	if !strings.Contains(tags, "session-checkpoint-2-") {
		t.Errorf("second tag not found:\n%s", tags)
	}
}
