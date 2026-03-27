package session

import (
	"encoding/json"
	"errors"
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

func TestAutoCommitAndMerge_RebaseOnDivergedMain(t *testing.T) {
	t.Parallel()

	// Create a main repo with initial commit.
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
	if err := os.WriteFile(filepath.Join(mainDir, "file.txt"), []byte("initial"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(mainDir, "add", ".")
	run(mainDir, "commit", "-m", "initial")

	// Detect the default branch name.
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = mainDir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create a worktree.
	wtDir := filepath.Join(t.TempDir(), "wt")
	run(mainDir, "worktree", "add", wtDir, "-b", "work")
	run(wtDir, "config", "user.email", "test@test.com")
	run(wtDir, "config", "user.name", "Test")

	// Advance main in the main repo (simulate prior iteration merge).
	run(mainDir, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(mainDir, "other.txt"), []byte("from main"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(mainDir, "add", ".")
	run(mainDir, "commit", "-m", "advance main")

	// Make a non-conflicting change in worktree.
	if err := os.WriteFile(filepath.Join(wtDir, "feature.txt"), []byte("new feature"), 0o644); err != nil {
		t.Fatal(err)
	}

	// AutoCommitAndMerge should succeed via rebase.
	if err := AutoCommitAndMerge(wtDir, mainBranch, "test merge via rebase"); err != nil {
		t.Fatalf("expected rebase to succeed, got: %v", err)
	}

	// Verify main branch has our commit.
	logCmd := exec.Command("git", "log", mainBranch, "--oneline", "-1")
	logCmd.Dir = wtDir
	out, err := logCmd.Output()
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(out), "test merge via rebase") {
		t.Errorf("main branch should have the commit, got: %s", out)
	}
}

func TestAutoCommitAndMerge_RebaseConflictFallback(t *testing.T) {
	t.Parallel()

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
	if err := os.WriteFile(filepath.Join(mainDir, "conflict.txt"), []byte("line1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(mainDir, "add", ".")
	run(mainDir, "commit", "-m", "initial")

	// Detect the default branch name.
	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = mainDir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// Create a worktree.
	wtDir := filepath.Join(t.TempDir(), "wt")
	run(mainDir, "worktree", "add", wtDir, "-b", "work")
	run(wtDir, "config", "user.email", "test@test.com")
	run(wtDir, "config", "user.name", "Test")

	// Conflicting change on main.
	run(mainDir, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(mainDir, "conflict.txt"), []byte("main version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run(mainDir, "add", ".")
	run(mainDir, "commit", "-m", "main change")

	// Conflicting change in worktree.
	if err := os.WriteFile(filepath.Join(wtDir, "conflict.txt"), []byte("worktree version\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err = AutoCommitAndMerge(wtDir, mainBranch, "test merge with conflict")
	if !errors.Is(err, ErrRebaseConflict) {
		t.Fatalf("expected ErrRebaseConflict, got: %v", err)
	}
}

func TestAutoCommitAndMergeTraced_NoChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

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

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit("add", ".")
	gitInit("commit", "-m", "initial")

	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// No changes — trace should report no_staged_files
	trace, err := AutoCommitAndMergeTraced(dir, mainBranch, "test: no changes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trace.Reason != "no_staged_files" {
		t.Errorf("expected reason 'no_staged_files', got %q", trace.Reason)
	}
	if trace.StagedFileCount != 0 {
		t.Errorf("expected 0 staged files, got %d", trace.StagedFileCount)
	}
}

func TestAutoCommitAndMergeTraced_WithChanges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

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

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInit("add", ".")
	gitInit("commit", "-m", "initial")

	cmd := exec.Command("git", "branch", "--show-current")
	cmd.Dir = dir
	branchOut, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	mainBranch := strings.TrimSpace(string(branchOut))

	// Add a file
	if err := os.WriteFile(filepath.Join(dir, "new_test.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	trace, err := AutoCommitAndMergeTraced(dir, mainBranch, "test: with changes")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if trace.Reason != "auto_merged" {
		t.Errorf("expected reason 'auto_merged', got %q", trace.Reason)
	}
	if trace.StagedFileCount != 1 {
		t.Errorf("expected 1 staged file, got %d", trace.StagedFileCount)
	}
}

func TestClassifySelfImprovePaths_TestFilesAreSafe(t *testing.T) {
	paths := []string{
		"internal/session/acceptance_test.go",
		"internal/mcpserver/handler_test.go",
		"cmd/root_test.go",
	}
	safe, review := ClassifySelfImprovePaths(paths)
	if len(review) != 0 {
		t.Errorf("test files should all be safe, got review: %v", review)
	}
	if len(safe) != 3 {
		t.Errorf("expected 3 safe paths, got %d", len(safe))
	}
}

func TestCheckpointExcludes_DoNotFilterGoFiles(t *testing.T) {
	// Verify that checkpointExcludes patterns don't accidentally match
	// legitimate Go source files. Files without "secret" or "credentials"
	// in their basename should never match.
	safeFiles := []string{
		"internal/session/loop.go",
		"cmd/secretsctl/main.go", // "secretsctl" is a dir name, basename is "main.go"
		"internal/auth/provider.go",
	}

	for _, f := range safeFiles {
		for _, excl := range checkpointExcludes {
			base := filepath.Base(f)
			if m, _ := filepath.Match(excl, base); m {
				t.Errorf("checkpointExcludes pattern %q matches legitimate Go file %q (base=%q)", excl, f, base)
			}
		}
	}
}

func TestCheckpointExcludes_KnownOverBroadPatterns(t *testing.T) {
	// Document that *secret* and credentials* patterns can match Go source
	// files with those words in their basename. This is a known risk (A22)
	// that can cause 0-files-staged rejections if a worker creates files
	// like "secret_rotation.go" or "credentials.go".
	knownOverBroad := map[string]string{
		"internal/session/secret_rotation.go": "*secret*",
		"internal/credentials_manager.go":     "credentials*",
		"internal/auth/credentials.go":        "credentials*",
	}

	for file, expectedPattern := range knownOverBroad {
		base := filepath.Base(file)
		matched := false
		for _, excl := range checkpointExcludes {
			if m, _ := filepath.Match(excl, base); m {
				if excl != expectedPattern {
					t.Errorf("file %q matched unexpected pattern %q (expected %q)", file, excl, expectedPattern)
				}
				matched = true
			}
		}
		if !matched {
			t.Errorf("expected file %q to be over-broadly matched by %q, but it wasn't", file, expectedPattern)
		}
	}
}

func TestCheckpointExcludes_MatchSecretFiles(t *testing.T) {
	// Verify that actual secret/credential files ARE matched.
	secretFiles := []string{
		".env",
		".env.local",
		"server.pem",
		"private.key",
		"credentials.json",
		"my_secret.yaml",
	}

	for _, f := range secretFiles {
		matched := false
		for _, excl := range checkpointExcludes {
			if m, _ := filepath.Match(excl, f); m {
				matched = true
				break
			}
		}
		if !matched {
			t.Errorf("expected checkpointExcludes to match secret file %q", f)
		}
	}
}

func TestAcceptanceTrace_JSON(t *testing.T) {
	trace := AcceptanceTrace{
		SafePaths:       []string{"docs/README.md"},
		ReviewPaths:     []string{"cmd/root.go"},
		StagedFileCount: 2,
		Reason:          "auto_merged",
	}
	data, err := json.Marshal(trace)
	if err != nil {
		t.Fatal(err)
	}
	var decoded AcceptanceTrace
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Reason != "auto_merged" {
		t.Errorf("Reason = %q, want %q", decoded.Reason, "auto_merged")
	}
	if decoded.StagedFileCount != 2 {
		t.Errorf("StagedFileCount = %d, want 2", decoded.StagedFileCount)
	}
}

func TestCountStagedFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")
	run("config", "commit.gpgsign", "false")

	// Initial commit
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", ".")
	run("commit", "-m", "initial")

	// No staged files
	if n := countStagedFiles(dir); n != 0 {
		t.Errorf("expected 0 staged files, got %d", n)
	}

	// Stage a file
	if err := os.WriteFile(filepath.Join(dir, "new.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "new.go")
	if n := countStagedFiles(dir); n != 1 {
		t.Errorf("expected 1 staged file, got %d", n)
	}

	// Stage another
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "other.go")
	if n := countStagedFiles(dir); n != 2 {
		t.Errorf("expected 2 staged files, got %d", n)
	}
}
