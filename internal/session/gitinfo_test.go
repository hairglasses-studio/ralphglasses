package session

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func initGitInfoRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git setup %v: %v\n%s", args, err, out)
		}
	}

	// Create an initial commit with a backdated author date so it falls
	// outside the default test time window. This ensures GitDiffWindow
	// never tries to use root-commit^ which would fail.
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "file.txt"},
		{"git", "commit", "-m", "initial commit", "--date=2020-01-01T00:00:00Z"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_COMMITTER_DATE=2020-01-01T00:00:00Z")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %v: %v\n%s", args, err, out)
		}
	}

	return dir
}

func TestGitLogSince(t *testing.T) {
	dir := initGitInfoRepo(t)

	// Add a recent commit (initGitInfoRepo backdates the initial commit)
	if err := os.WriteFile(filepath.Join(dir, "recent.txt"), []byte("recent\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "recent.txt"},
		{"git", "commit", "-m", "recent commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %v: %v\n%s", args, err, out)
		}
	}

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)

	commits, err := GitLogSince(dir, since, until)
	if err != nil {
		t.Fatalf("GitLogSince: %v", err)
	}
	if len(commits) != 1 {
		t.Fatalf("expected 1 commit, got %d", len(commits))
	}
	if commits[0]["subject"] != "recent commit" {
		t.Errorf("subject = %q, want 'recent commit'", commits[0]["subject"])
	}
	if commits[0]["hash"] == "" {
		t.Error("expected non-empty hash")
	}
}

func TestGitLogSince_NoCommits(t *testing.T) {
	dir := initGitInfoRepo(t)

	// Window in the past where no commits exist
	since := time.Now().Add(-48 * time.Hour)
	until := time.Now().Add(-47 * time.Hour)

	commits, err := GitLogSince(dir, since, until)
	if err != nil {
		t.Fatalf("GitLogSince: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("expected 0 commits, got %d", len(commits))
	}
}

func TestGitLogSince_InvalidDir(t *testing.T) {
	_, err := GitLogSince("/nonexistent/path", time.Now(), time.Now())
	if err == nil {
		t.Error("expected error for invalid directory")
	}
}

func TestGitDiffWindow(t *testing.T) {
	dir := initGitInfoRepo(t)

	// The first commit is the initial. We need a second commit so the diff
	// window has a parent ref (oldest^) that actually exists.
	// Add a second commit right away to give us a clean base.
	if err := os.WriteFile(filepath.Join(dir, "base.txt"), []byte("base\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "base.txt"},
		{"git", "commit", "-m", "base commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %v: %v\n%s", args, err, out)
		}
	}

	// Now make a third commit with changes
	if err := os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"git", "add", "file.txt"},
		{"git", "commit", "-m", "third commit"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit %v: %v\n%s", args, err, out)
		}
	}

	since := time.Now().Add(-1 * time.Hour)
	until := time.Now().Add(1 * time.Hour)

	// stat-only
	_, stat, truncated, err := GitDiffWindow(dir, since, until, true, 0)
	if err != nil {
		t.Fatalf("GitDiffWindow stat: %v", err)
	}
	if stat["files_changed"] < 1 {
		t.Errorf("expected at least 1 file changed, got %d", stat["files_changed"])
	}
	if truncated {
		t.Error("stat-only should not be truncated")
	}

	// full diff
	diffText, stat2, _, err := GitDiffWindow(dir, since, until, false, 0)
	if err != nil {
		t.Fatalf("GitDiffWindow full: %v", err)
	}
	if diffText == "" {
		t.Error("expected non-empty diff text")
	}
	if stat2["files_changed"] < 1 {
		t.Error("expected at least 1 file changed in full diff")
	}

	// truncated diff
	_, _, trunc, err := GitDiffWindow(dir, since, until, false, 1)
	if err != nil {
		t.Fatalf("GitDiffWindow truncated: %v", err)
	}
	if !trunc {
		t.Error("expected truncated=true with maxLines=1")
	}
}

func TestGitDiffWindow_NoCommits(t *testing.T) {
	dir := initGitInfoRepo(t)

	since := time.Now().Add(-48 * time.Hour)
	until := time.Now().Add(-47 * time.Hour)

	_, stat, _, err := GitDiffWindow(dir, since, until, true, 0)
	if err != nil {
		t.Fatalf("GitDiffWindow: %v", err)
	}
	if stat["files_changed"] != 0 {
		t.Errorf("expected 0 files changed, got %d", stat["files_changed"])
	}
}
