package worktree

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// MergeResult holds the outcome of a merge-back operation.
type MergeResult struct {
	Merged        bool     // true if the merge completed successfully
	ConflictFiles []string // files with conflicts (populated when Merged is false)
	CommitHash    string   // merge commit SHA (populated when Merged is true)
}

// MergeBack merges branch into targetBranch within the repository at repoPath
// using a no-fast-forward merge. If the merge produces conflicts, it aborts the
// merge and returns the list of conflicting files in MergeResult.ConflictFiles.
//
// The caller is responsible for ensuring targetBranch is checked out at repoPath
// (or a worktree thereof) before calling MergeBack.
func MergeBack(ctx context.Context, repoPath, branch, targetBranch string) (*MergeResult, error) {
	// Ensure we are on the target branch.
	checkoutCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "checkout", targetBranch)
	if out, err := checkoutCmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worktree: checkout %q: %w: %s", targetBranch, err, strings.TrimSpace(string(out)))
	}

	// Attempt the merge.
	mergeCmd := exec.CommandContext(ctx, "git", "-C", repoPath, "merge", "--no-ff", branch)
	mergeOut, mergeErr := mergeCmd.CombinedOutput()

	if mergeErr == nil {
		// Merge succeeded — grab the commit hash.
		hash, err := revParseHEAD(ctx, repoPath)
		if err != nil {
			return nil, fmt.Errorf("worktree: merge succeeded but rev-parse failed: %w", err)
		}
		return &MergeResult{
			Merged:     true,
			CommitHash: hash,
		}, nil
	}

	// Check if the failure is due to conflicts.
	hasConflict, conflictFiles, checkErr := HasConflicts(ctx, repoPath)
	if checkErr != nil {
		// Cannot determine conflict state — abort and propagate.
		abortMerge(ctx, repoPath)
		return nil, fmt.Errorf("worktree: merge %q into %q failed: %w: %s", branch, targetBranch, mergeErr, strings.TrimSpace(string(mergeOut)))
	}

	if hasConflict {
		// Abort the merge to leave the repo in a clean state.
		abortMerge(ctx, repoPath)
		return &MergeResult{
			Merged:        false,
			ConflictFiles: conflictFiles,
		}, nil
	}

	// Non-conflict merge failure (e.g. invalid branch name).
	abortMerge(ctx, repoPath)
	return nil, fmt.Errorf("worktree: merge %q into %q: %w: %s", branch, targetBranch, mergeErr, strings.TrimSpace(string(mergeOut)))
}

// HasConflicts checks whether the repository at repoPath currently has
// unresolved merge conflicts. It parses `git status --porcelain` looking for
// "UU", "AA", "DD", "AU", "UA", "DU", "UD" status codes.
func HasConflicts(ctx context.Context, repoPath string) (bool, []string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return false, nil, fmt.Errorf("worktree: status: %w: %s", err, stderr)
	}

	var conflicts []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		if isConflictStatus(xy) {
			conflicts = append(conflicts, strings.TrimSpace(line[3:]))
		}
	}
	if err := scanner.Err(); err != nil {
		return false, nil, fmt.Errorf("worktree: parse status: %w", err)
	}

	return len(conflicts) > 0, conflicts, nil
}

// isConflictStatus returns true for git status XY codes that indicate a
// merge conflict. See `git status --porcelain` documentation.
func isConflictStatus(xy string) bool {
	switch xy {
	case "UU", "AA", "DD", "AU", "UA", "DU", "UD":
		return true
	}
	return false
}

// abortMerge runs `git merge --abort` to clean up after a failed merge.
// Errors are ignored since this is best-effort cleanup.
func abortMerge(ctx context.Context, repoPath string) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "merge", "--abort")
	_ = cmd.Run()
}

// revParseHEAD returns the current HEAD commit SHA.
func revParseHEAD(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("worktree: rev-parse HEAD: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}
