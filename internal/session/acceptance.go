package session

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// ErrRebaseConflict is returned when a rebase onto the main branch encounters
// conflicts that require manual resolution.
var ErrRebaseConflict = errors.New("rebase conflict: manual resolution required")

// tryRebaseOnto attempts to rebase the current branch onto the given base branch.
// If the rebase encounters conflicts, it aborts and returns ErrRebaseConflict.
func tryRebaseOnto(dir, baseBranch string) error {
	if err := gitRun(dir, "rebase", baseBranch); err != nil {
		_ = gitRun(dir, "rebase", "--abort")
		return ErrRebaseConflict
	}
	return nil
}

// AcceptanceResult records the outcome of the self-improvement acceptance gate.
type AcceptanceResult struct {
	SafePaths   []string `json:"safe_paths,omitempty"`
	ReviewPaths []string `json:"review_paths,omitempty"`
	AutoMerged  bool     `json:"auto_merged"`
	PRCreated   bool     `json:"pr_created"`
	PRURL       string   `json:"pr_url,omitempty"`
	Error       string   `json:"error,omitempty"`
}

// selfImproveSafePrefixes are path prefixes that can be auto-merged.
// When AutoMergeAll is true (unattended mode), this list is bypassed —
// all paths are auto-merged if verification passes.
var selfImproveSafePrefixes = []string{
	"docs/",
	"scripts/",
	"internal/tui/",
	"distro/",
	"testdata/",
}

// selfImproveReviewPrefixes are path prefixes that require PR review.
var selfImproveReviewPrefixes = []string{
	"internal/session/",
	"internal/mcpserver/",
	"internal/e2e/",
	"cmd/",
}

// selfImproveReviewExact are exact filenames that require PR review.
var selfImproveReviewExact = []string{
	"go.mod",
	"go.sum",
	"CLAUDE.md",
}

// ClassifySelfImprovePaths splits changed paths into safe (auto-merge) and
// review (needs PR) categories. Test files (*_test.go) are always safe.
// Paths not matching any known prefix default to review.
func ClassifySelfImprovePaths(paths []string) (safe, review []string) {
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			safe = append(safe, p)
			continue
		}
		if isReviewPath(p) {
			review = append(review, p)
			continue
		}
		if isSafePath(p) {
			safe = append(safe, p)
			continue
		}
		// Unknown paths default to review for safety.
		review = append(review, p)
	}
	return safe, review
}

func isSafePath(p string) bool {
	for _, prefix := range selfImproveSafePrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

func isReviewPath(p string) bool {
	for _, prefix := range selfImproveReviewPrefixes {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	for _, exact := range selfImproveReviewExact {
		if p == exact {
			return true
		}
	}
	return false
}

// isGitWorktree returns true if dir is inside a git worktree (not the main
// checkout). It checks whether git-common-dir differs from git-dir, which
// indicates a linked worktree where `git checkout main` would fail because
// main is already checked out by the parent repo.
func isGitWorktree(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return false
	}
	commonDir := strings.TrimSpace(string(out))
	// In a normal repo, --git-common-dir returns ".git".
	// In a worktree, it returns an absolute path to the main repo's .git dir.
	return commonDir != ".git"
}

// AutoCommitAndMerge stages, commits, and fast-forward merges changes from a
// worktree back to the main branch. Uses the same secret exclusions as
// checkpoint.go.
//
// When running inside a git worktree, it avoids `git checkout main` (which
// would fail because main is locked by the parent repo) and instead updates
// the main branch ref directly via `git update-ref`.
func AutoCommitAndMerge(dir, mainBranch, message string) error {
	ts := time.Now().Format("20060102-150405")
	branch := fmt.Sprintf("self-improve-%s", ts)

	// Create feature branch in worktree
	if err := gitRun(dir, "checkout", "-b", branch); err != nil {
		return fmt.Errorf("create branch: %w", err)
	}

	// Stage all changes, excluding sensitive files
	addArgs := []string{"add", "-A", "--"}
	for _, excl := range checkpointExcludes {
		addArgs = append(addArgs, ":(exclude)"+excl)
	}
	if err := gitRun(dir, addArgs...); err != nil {
		return fmt.Errorf("git add: %w", err)
	}

	// Check if anything was staged
	statusCmd := exec.Command("git", "diff", "--cached", "--quiet")
	statusCmd.Dir = dir
	if statusCmd.Run() == nil {
		// Nothing staged — nothing to merge
		_ = gitRun(dir, "checkout", mainBranch)
		_ = gitRun(dir, "branch", "-D", branch)
		return nil
	}

	// Commit
	if err := gitRun(dir, "commit", "-m", message); err != nil {
		return fmt.Errorf("git commit: %w", err)
	}

	if isGitWorktree(dir) {
		// Worktree mode: cannot checkout main (it's locked by the parent repo).
		// Verify we can fast-forward, then update the ref directly.
		mergeBase, err := gitOutput(dir, "merge-base", "HEAD", mainBranch)
		if err != nil {
			return fmt.Errorf("merge-base: %w", err)
		}

		mainRef, err := gitOutput(dir, "rev-parse", mainBranch)
		if err != nil {
			return fmt.Errorf("rev-parse %s: %w", mainBranch, err)
		}

		// Fast-forward is only valid if main is the merge-base (i.e., main
		// hasn't diverged from our branch point).
		if strings.TrimSpace(mergeBase) != strings.TrimSpace(mainRef) {
			// Main has diverged — try rebasing our branch onto main.
			if err := tryRebaseOnto(dir, mainBranch); err != nil {
				return err
			}
			// Rebase succeeded — fall through to update-ref with new HEAD.
		}

		headRef, err := gitOutput(dir, "rev-parse", "HEAD")
		if err != nil {
			return fmt.Errorf("rev-parse HEAD: %w", err)
		}

		// Update main branch ref to point to our commit.
		if err := gitRun(dir, "update-ref", "refs/heads/"+mainBranch, strings.TrimSpace(headRef)); err != nil {
			return fmt.Errorf("update-ref: %w", err)
		}
	} else {
		// Normal repo: switch to main and fast-forward merge.
		if err := gitRun(dir, "checkout", mainBranch); err != nil {
			return fmt.Errorf("checkout main: %w", err)
		}
		if err := gitRun(dir, "merge", "--ff-only", branch); err != nil {
			// ff-only failed — rebase the feature branch onto main and retry.
			if err := gitRun(dir, "checkout", branch); err != nil {
				return fmt.Errorf("checkout branch for rebase: %w", err)
			}
			if rebaseErr := tryRebaseOnto(dir, mainBranch); rebaseErr != nil {
				return rebaseErr
			}
			if err := gitRun(dir, "checkout", mainBranch); err != nil {
				return fmt.Errorf("checkout main after rebase: %w", err)
			}
			if err := gitRun(dir, "merge", "--ff-only", branch); err != nil {
				return fmt.Errorf("ff-merge after rebase: %w", err)
			}
		}
	}

	// Cleanup branch
	_ = gitRun(dir, "branch", "-d", branch)
	return nil
}

// gitOutput executes a git command and returns its trimmed stdout.
func gitOutput(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CreateReviewPR creates a branch, commits changes, pushes, and creates a
// GitHub PR via the gh CLI. Returns the PR URL.
func CreateReviewPR(dir, mainBranch, title string, reviewPaths []string) (string, error) {
	// Verify gh CLI is available
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh CLI not found: %w", err)
	}

	ts := time.Now().Format("20060102-150405")
	branch := fmt.Sprintf("self-improve-review-%s", ts)

	// Create branch
	if err := gitRun(dir, "checkout", "-b", branch); err != nil {
		return "", fmt.Errorf("create branch: %w", err)
	}

	// Stage + commit (reuse checkpoint excludes)
	addArgs := []string{"add", "-A", "--"}
	for _, excl := range checkpointExcludes {
		addArgs = append(addArgs, ":(exclude)"+excl)
	}
	if err := gitRun(dir, addArgs...); err != nil {
		return "", fmt.Errorf("git add: %w", err)
	}

	msg := fmt.Sprintf("self-improve: %s", title)
	if err := gitRun(dir, "commit", "-m", msg); err != nil {
		return "", fmt.Errorf("git commit: %w", err)
	}

	// Push
	if err := gitRun(dir, "push", "-u", "origin", branch); err != nil {
		return "", fmt.Errorf("git push: %w", err)
	}

	// Create PR
	body := "## Auto-generated self-improvement PR\n\nReview-required paths:\n"
	for _, p := range reviewPaths {
		body += fmt.Sprintf("- `%s`\n", p)
	}

	cmd := exec.Command("gh", "pr", "create",
		"--title", title,
		"--body", body,
		"--base", mainBranch,
		"--head", branch,
	)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr create: %w\n%s", err, out)
	}

	return strings.TrimSpace(string(out)), nil
}

// gitRun executes a git command in the given directory.
func gitRun(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}
