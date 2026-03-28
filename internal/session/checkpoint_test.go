package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitEnv returns a minimal, isolated environment for git commands in tests.
// This prevents parallel test git processes from sharing global config.
func gitEnv(dir string) []string {
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

// initGitRepo creates a git repo in dir with an initial commit.
func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	env := gitEnv(dir)
	initCmd := exec.Command("git", "init", dir)
	initCmd.Env = env
	if err := initCmd.Run(); err != nil {
		t.Fatalf("git init: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# test"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"-C", dir, "add", "."},
		{"-C", dir, "commit", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Env = env
		if err := cmd.Run(); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
}

func gitLog(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "log", "--oneline")
	cmd.Env = gitEnv(dir)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	return string(out)
}

func gitTags(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", dir, "tag", "-l")
	cmd.Env = gitEnv(dir)
	out, err := cmd.Output()
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

func TestCreateCheckpoint_ExcludesPrecision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initGitRepo(t, dir)

	// Legitimate Go source files that were previously over-excluded by
	// the broad "credentials*" and "*secret*" globs.
	legitimateFiles := []string{
		"secret_rotation.go",
		"credentials_handler.go",
		"secret_test.go",
		"lib/secret_manager.go",
	}

	// Actual secret/credential files that must still be excluded.
	secretFiles := []string{
		"credentials.json",
		"credentials.yaml",
		"credentials.yml",
		"credentials.xml",
		"credentials.toml",
		"my-secret.json",
		"app-secret.yaml",
		"db-secret.yml",
		"config.secret",
		".env.secret",
	}

	for _, f := range append(legitimateFiles, secretFiles...) {
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

	dtCmd := exec.Command("git", "-C", dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	dtCmd.Env = gitEnv(dir)
	out, err := dtCmd.Output()
	if err != nil {
		t.Fatalf("git diff-tree: %v", err)
	}
	committed := string(out)

	// Legitimate Go files MUST be committed (no longer over-excluded).
	for _, f := range legitimateFiles {
		if !strings.Contains(committed, f) {
			t.Errorf("legitimate file %q should be committed but was excluded.\nCommitted:\n%s", f, committed)
		}
	}

	// Actual secret files MUST NOT be committed.
	for _, f := range secretFiles {
		if strings.Contains(committed, f) {
			t.Errorf("secret file %q was committed but should have been excluded.\nCommitted:\n%s", f, committed)
		}
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
	dtCmd2 := exec.Command("git", "-C", dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	dtCmd2.Env = gitEnv(dir)
	out2, err2 := dtCmd2.Output()
	if err2 != nil {
		t.Fatalf("git diff-tree: %v", err2)
	}
	committed := string(out2)

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
	statusCmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	statusCmd.Env = gitEnv(dir)
	statusOut, err := statusCmd.Output()
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
