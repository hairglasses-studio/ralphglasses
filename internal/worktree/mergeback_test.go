package worktree

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// setupWorktreeRepo creates a main repo with an initial commit and a worktree
// on a feature branch with one additional commit. Returns the repo path,
// worktree path, main branch name, and feature branch name.
func setupWorktreeRepo(t *testing.T) (repoPath, wtPath, mainBranch, featureBranch string) {
	t.Helper()

	repoPath = initTestRepo(t)
	ctx := context.Background()
	mainBranch = runGit(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	featureBranch = "feature-mergeback"

	base, _ := filepath.EvalSymlinks(t.TempDir())
	wtPath = filepath.Join(base, "wt-mergeback")
	if err := Create(ctx, repoPath, wtPath, featureBranch); err != nil {
		t.Fatalf("Create worktree: %v", err)
	}

	// Add a file in the worktree.
	if err := os.WriteFile(filepath.Join(wtPath, "feature.txt"), []byte("feature content\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtPath, "add", ".")
	runGit(t, wtPath, "commit", "-m", "add feature.txt")

	return repoPath, wtPath, mainBranch, featureBranch
}

func TestMergeBackFnCleanMerge(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	result, err := MergeBackFn(ctx, wt, mainBranch)
	if err != nil {
		t.Fatalf("MergeBackFn: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got conflicts: %v", result.ConflictFiles)
	}
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}

	// Verify feature.txt exists on the main branch after merge.
	runGit(t, repoPath, "checkout", mainBranch)
	if _, err := os.Stat(filepath.Join(repoPath, "feature.txt")); err != nil {
		t.Errorf("feature.txt should exist after merge: %v", err)
	}
}

func TestMergeBackFnConflictDetection(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	// Create a conflicting change on the main branch.
	runGit(t, repoPath, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("main version\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "conflicting feature.txt on main")

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	result, err := MergeBackFn(ctx, wt, mainBranch)
	if err != nil {
		t.Fatalf("MergeBackFn: %v", err)
	}
	if result.Success {
		t.Fatal("expected conflict, got success")
	}
	if len(result.ConflictFiles) == 0 {
		t.Fatal("expected at least one conflict file")
	}

	foundFeature := false
	for _, f := range result.ConflictFiles {
		if f == "feature.txt" {
			foundFeature = true
		}
	}
	if !foundFeature {
		t.Errorf("expected feature.txt in conflicts, got %v", result.ConflictFiles)
	}

	// Verify the repo is clean (merge was aborted).
	hasConflict, _, checkErr := HasConflicts(ctx, repoPath)
	if checkErr != nil {
		t.Fatalf("HasConflicts: %v", checkErr)
	}
	if hasConflict {
		t.Error("repo should be clean after aborted merge")
	}
}

func TestDetectConflicts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	// No conflict case first.
	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	conflicts, err := DetectConflicts(ctx, wt, mainBranch)
	if err != nil {
		t.Fatalf("DetectConflicts (no conflict): %v", err)
	}
	if len(conflicts) != 0 {
		t.Errorf("expected no conflicts, got %v", conflicts)
	}

	// Now create a conflict on main.
	runGit(t, repoPath, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("conflict from main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "conflicting change on main")

	conflicts, err = DetectConflicts(ctx, wt, mainBranch)
	if err != nil {
		t.Fatalf("DetectConflicts (with conflict): %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatal("expected at least one conflict")
	}

	// Verify repo is restored to its original state.
	currentBr := runGit(t, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if currentBr != mainBranch {
		t.Errorf("repo should be back on %q, got %q", mainBranch, currentBr)
	}
}

func TestCopyBack(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, _, _ := setupWorktreeRepo(t)

	// Add a nested file in the worktree.
	nestedDir := filepath.Join(wtPath, "sub", "dir")
	if err := os.MkdirAll(nestedDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "nested.txt"), []byte("nested\n"), 0644); err != nil {
		t.Fatal(err)
	}

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	targetDir := t.TempDir()

	err := CopyBack(wt, targetDir, "feature.txt", "sub/dir/nested.txt")
	if err != nil {
		t.Fatalf("CopyBack: %v", err)
	}

	// Verify copied files.
	content, err := os.ReadFile(filepath.Join(targetDir, "feature.txt"))
	if err != nil {
		t.Fatalf("read feature.txt: %v", err)
	}
	if string(content) != "feature content\n" {
		t.Errorf("feature.txt content = %q", content)
	}

	content, err = os.ReadFile(filepath.Join(targetDir, "sub", "dir", "nested.txt"))
	if err != nil {
		t.Fatalf("read nested.txt: %v", err)
	}
	if string(content) != "nested\n" {
		t.Errorf("nested.txt content = %q", content)
	}
}

func TestCopyBackNoFiles(t *testing.T) {
	wt := &Worktree{Path: "/tmp", RepoPath: "/tmp"}
	err := CopyBack(wt, "/tmp/target")
	if err == nil {
		t.Fatal("expected error when no files specified")
	}
	if !strings.Contains(err.Error(), "no files specified") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCopyBackMissingSource(t *testing.T) {
	wt := &Worktree{Path: t.TempDir(), RepoPath: t.TempDir()}
	err := CopyBack(wt, t.TempDir(), "nonexistent.txt")
	if err == nil {
		t.Fatal("expected error for missing source file")
	}
}

func TestDiffSummaryFn(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, _, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	// Add more changes in the worktree for a richer diff.
	if err := os.WriteFile(filepath.Join(wtPath, "added.txt"), []byte("new file\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wtPath, "README.md"), []byte("modified readme\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, wtPath, "add", ".")
	runGit(t, wtPath, "commit", "-m", "add and modify files")

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	summary, err := DiffSummaryFn(ctx, wt)
	if err != nil {
		t.Fatalf("DiffSummaryFn: %v", err)
	}

	// We should see added files.
	if len(summary.AddedFiles) == 0 && len(summary.ModifiedFiles) == 0 {
		t.Error("expected at least some added or modified files")
	}

	// feature.txt and added.txt should appear somewhere.
	allFiles := append(summary.AddedFiles, summary.ModifiedFiles...)
	foundFeature := false
	foundAdded := false
	for _, f := range allFiles {
		if f == "feature.txt" {
			foundFeature = true
		}
		if f == "added.txt" {
			foundAdded = true
		}
	}
	if !foundFeature {
		t.Errorf("expected feature.txt in diff summary, got added=%v modified=%v", summary.AddedFiles, summary.ModifiedFiles)
	}
	if !foundAdded {
		t.Errorf("expected added.txt in diff summary, got added=%v modified=%v", summary.AddedFiles, summary.ModifiedFiles)
	}

	// Stats should have entries.
	if len(summary.Stats) == 0 {
		t.Error("expected non-empty stats")
	}

	// Verify stats have positive line counts.
	for _, s := range summary.Stats {
		if s.Path == "" {
			t.Error("stat entry with empty path")
		}
	}
}

func TestMergeBackFnWithSquash(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	result, err := MergeBackFn(ctx, wt, mainBranch, WithSquash(), WithMessage("squashed merge"))
	if err != nil {
		t.Fatalf("MergeBackFn with squash: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got conflicts: %v", result.ConflictFiles)
	}

	// Verify the commit message.
	runGit(t, repoPath, "checkout", mainBranch)
	lastMsg := runGit(t, repoPath, "log", "-1", "--format=%s")
	if lastMsg != "squashed merge" {
		t.Errorf("expected commit message 'squashed merge', got %q", lastMsg)
	}
}

func TestMergeBackFnWithDryRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}

	// Get HEAD before dry-run.
	headBefore := runGit(t, repoPath, "rev-parse", "HEAD")

	result, err := MergeBackFn(ctx, wt, mainBranch, WithDryRun())
	if err != nil {
		t.Fatalf("MergeBackFn with dry-run: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success (no conflicts), got conflicts: %v", result.ConflictFiles)
	}

	// HEAD should not have changed.
	headAfter := runGit(t, repoPath, "rev-parse", "HEAD")
	if headBefore != headAfter {
		t.Errorf("dry-run should not change HEAD: before=%s after=%s", headBefore, headAfter)
	}
}

func TestMergeBackFnDryRunWithConflicts(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}

	repoPath, wtPath, mainBranch, _ := setupWorktreeRepo(t)
	ctx := context.Background()

	// Create conflict on main.
	runGit(t, repoPath, "checkout", mainBranch)
	if err := os.WriteFile(filepath.Join(repoPath, "feature.txt"), []byte("main conflict\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runGit(t, repoPath, "add", ".")
	runGit(t, repoPath, "commit", "-m", "conflict on main")

	wt := &Worktree{Path: wtPath, RepoPath: repoPath}
	result, err := MergeBackFn(ctx, wt, mainBranch, WithDryRun())
	if err != nil {
		t.Fatalf("MergeBackFn dry-run with conflicts: %v", err)
	}
	if result.Success {
		t.Fatal("expected conflict detection in dry-run")
	}
	if len(result.ConflictFiles) == 0 {
		t.Fatal("expected at least one conflict file")
	}
}
