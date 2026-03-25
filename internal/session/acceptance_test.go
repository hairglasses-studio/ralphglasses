package session

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClassifySelfImprovePaths_AllSafe(t *testing.T) {
	paths := []string{
		"internal/session/loop_test.go",
		"docs/README.md",
		"scripts/ci.sh",
		"internal/tui/app.go",
		"distro/config.yaml",
		"testdata/fixtures/sample.json",
	}
	safe, review := ClassifySelfImprovePaths(paths)
	if len(review) != 0 {
		t.Errorf("expected no review paths, got %v", review)
	}
	if len(safe) != len(paths) {
		t.Errorf("expected %d safe paths, got %d", len(paths), len(safe))
	}
}

func TestClassifySelfImprovePaths_AllReview(t *testing.T) {
	paths := []string{
		"internal/session/loop.go",
		"internal/mcpserver/handler.go",
		"cmd/root.go",
		"go.mod",
		"CLAUDE.md",
	}
	safe, review := ClassifySelfImprovePaths(paths)
	if len(safe) != 0 {
		t.Errorf("expected no safe paths, got %v", safe)
	}
	if len(review) != len(paths) {
		t.Errorf("expected %d review paths, got %d", len(paths), len(review))
	}
}

func TestClassifySelfImprovePaths_Mixed(t *testing.T) {
	paths := []string{
		"internal/session/loop_test.go",  // safe (test file)
		"internal/session/loop.go",       // review
		"docs/architecture.md",           // safe
		"cmd/selftest.go",                // review
		"scripts/test/marathon.bats",     // safe
	}
	safe, review := ClassifySelfImprovePaths(paths)
	if len(safe) != 3 {
		t.Errorf("expected 3 safe paths, got %d: %v", len(safe), safe)
	}
	if len(review) != 2 {
		t.Errorf("expected 2 review paths, got %d: %v", len(review), review)
	}
}

func TestClassifySelfImprovePaths_UnknownDefaultsToReview(t *testing.T) {
	paths := []string{"unknown/path/file.go"}
	safe, review := ClassifySelfImprovePaths(paths)
	if len(safe) != 0 {
		t.Errorf("unknown path should not be safe: %v", safe)
	}
	if len(review) != 1 {
		t.Errorf("unknown path should be review: %v", review)
	}
}

func TestAcceptanceResult_JSON(t *testing.T) {
	r := AcceptanceResult{
		SafePaths:   []string{"a_test.go"},
		ReviewPaths: []string{"cmd/root.go"},
		AutoMerged:  true,
		PRURL:       "https://github.com/org/repo/pull/1",
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AcceptanceResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.PRURL != r.PRURL {
		t.Errorf("PRURL = %q, want %q", decoded.PRURL, r.PRURL)
	}
}

func TestAutoCommitAndMerge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Initialize git repo
	gitInit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	gitInit("init")
	gitInit("config", "user.email", "test@test.com")
	gitInit("config", "user.name", "Test")
	gitInit("config", "commit.gpgsign", "false")

	// Create initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit("add", ".")
	gitInit("commit", "-m", "initial")

	// Detect default branch name
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// Make a change
	if err := os.WriteFile(filepath.Join(dir, "docs_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := AutoCommitAndMerge(dir, mainBranch, "self-improve: add test file"); err != nil {
		t.Fatalf("AutoCommitAndMerge: %v", err)
	}

	// Verify the file is committed
	logCmd := exec.Command("git", "log", "--oneline", "-1")
	logCmd.Dir = dir
	out, err := logCmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "self-improve") {
		t.Errorf("commit message should contain 'self-improve', got: %s", out)
	}
}

func TestIsGitWorktree(t *testing.T) {
	t.Parallel()

	t.Run("normal_repo_is_not_worktree", func(t *testing.T) {
		dir := t.TempDir()
		cmd := exec.Command("git", "init")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git init: %v\n%s", err, out)
		}
		if isGitWorktree(dir) {
			t.Error("normal git repo should not be detected as worktree")
		}
	})

	t.Run("linked_worktree_is_worktree", func(t *testing.T) {
		// Create a main repo with an initial commit so we can add a worktree.
		mainDir := t.TempDir()
		run := func(dir string, args ...string) {
			t.Helper()
			cmd := exec.Command("git", args...)
			cmd.Dir = dir
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("git %v: %v\n%s", args, err, out)
			}
		}
		run(mainDir, "init")
		run(mainDir, "config", "user.email", "test@test.com")
		run(mainDir, "config", "user.name", "Test")
		run(mainDir, "config", "commit.gpgsign", "false")
		if err := os.WriteFile(filepath.Join(mainDir, "f.txt"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		run(mainDir, "add", ".")
		run(mainDir, "commit", "-m", "init")

		// Create a linked worktree
		wtDir := filepath.Join(t.TempDir(), "wt")
		run(mainDir, "worktree", "add", wtDir, "-b", "wt-branch")

		if !isGitWorktree(wtDir) {
			t.Error("linked worktree should be detected as worktree")
		}
	})

	t.Run("non_git_dir_returns_false", func(t *testing.T) {
		dir := t.TempDir()
		if isGitWorktree(dir) {
			t.Error("non-git directory should not be detected as worktree")
		}
	})
}

func TestAutoCommitAndMergeWorktreeDetection(t *testing.T) {
	t.Parallel()

	// Set up a main repo with an initial commit.
	mainDir := t.TempDir()
	run := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(mainDir, "init")
	run(mainDir, "config", "user.email", "test@test.com")
	run(mainDir, "config", "user.name", "Test")
	run(mainDir, "config", "commit.gpgsign", "false")
	if err := os.WriteFile(filepath.Join(mainDir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(mainDir, "add", ".")
	run(mainDir, "commit", "-m", "initial")

	// Detect the default branch name from the main repo.
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = mainDir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create a linked worktree on a new branch.
	wtDir := filepath.Join(t.TempDir(), "wt")
	run(mainDir, "worktree", "add", wtDir, "-b", "wt-work")

	// Propagate git config to worktree (inherits from main, but be safe).
	run(wtDir, "config", "user.email", "test@test.com")
	run(wtDir, "config", "user.name", "Test")

	// Make a change inside the worktree.
	if err := os.WriteFile(filepath.Join(wtDir, "newfile.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// AutoCommitAndMerge should succeed without running `git checkout main`
	// (which would fail in a worktree because main is locked).
	if err := AutoCommitAndMerge(wtDir, mainBranch, "self-improve: worktree test"); err != nil {
		t.Fatalf("AutoCommitAndMerge in worktree: %v", err)
	}

	// Verify main branch was updated (via update-ref) by checking the log.
	logCmd := exec.Command("git", "log", mainBranch, "--oneline", "-1")
	logCmd.Dir = wtDir
	out, err := logCmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "self-improve: worktree test") {
		t.Errorf("main branch should have the commit, got: %s", out)
	}
}
