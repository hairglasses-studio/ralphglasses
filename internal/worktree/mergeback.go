package worktree

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// MergeBackResult holds the outcome of a merge-back operation.
type MergeBackResult struct {
	Success       bool     // true if the merge completed without conflicts
	ConflictFiles []string // files with merge conflicts (populated when Success is false)
	MergedFiles   []string // files successfully merged
	Error         error    // non-nil on unexpected failure (distinct from conflicts)
}

// MergeOption configures the behavior of MergeBack.
type MergeOption func(*mergeConfig)

type mergeConfig struct {
	squash          bool
	message         string
	abortOnConflict bool
	dryRun          bool
}

// WithSquash causes the merge to squash all commits into a single change set
// without creating a merge commit.
func WithSquash() MergeOption {
	return func(c *mergeConfig) { c.squash = true }
}

// WithMessage sets a custom commit message for the merge.
func WithMessage(msg string) MergeOption {
	return func(c *mergeConfig) { c.message = msg }
}

// WithAbortOnConflict causes the merge to be aborted automatically when
// conflicts are detected. When false (the default), the merge is still aborted
// but the conflict list is returned.
func WithAbortOnConflict() MergeOption {
	return func(c *mergeConfig) { c.abortOnConflict = true }
}

// WithDryRun performs conflict detection without actually merging. The
// repository is left in its original state.
func WithDryRun() MergeOption {
	return func(c *mergeConfig) { c.dryRun = true }
}

// MergeBackFn merges changes from the worktree's branch back into targetBranch.
// It uses git merge --no-commit to allow conflict detection before finalizing.
//
// The function operates on the main repository (wt.RepoPath). The worktree's
// current branch is determined automatically via git.
func MergeBackFn(ctx context.Context, wt *Worktree, targetBranch string, opts ...MergeOption) (*MergeBackResult, error) {
	cfg := &mergeConfig{
		abortOnConflict: true, // default: abort on conflict
	}
	for _, o := range opts {
		o(cfg)
	}

	// Determine the worktree's current branch.
	wtBranch, err := currentBranch(ctx, wt.Path)
	if err != nil {
		return nil, fmt.Errorf("mergeback: resolve worktree branch: %w", err)
	}

	if cfg.dryRun {
		conflicts, err := DetectConflicts(ctx, wt, targetBranch)
		if err != nil {
			return nil, err
		}
		return &MergeBackResult{
			Success:       len(conflicts) == 0,
			ConflictFiles: conflicts,
		}, nil
	}

	// Ensure the main repo is on the target branch.
	if out, err := gitCmd(ctx, wt.RepoPath, "checkout", targetBranch); err != nil {
		return nil, fmt.Errorf("mergeback: checkout %q: %w: %s", targetBranch, err, out)
	}

	// Build merge arguments.
	args := []string{"merge", "--no-commit", "--no-ff"}
	if cfg.squash {
		args = []string{"merge", "--squash"}
	}
	args = append(args, wtBranch)

	mergeOut, mergeErr := gitCmd(ctx, wt.RepoPath, args...)

	if mergeErr != nil {
		// Check for conflicts.
		hasConflict, conflictFiles, checkErr := HasConflicts(ctx, wt.RepoPath)
		if checkErr != nil {
			abortMerge(ctx, wt.RepoPath)
			return nil, fmt.Errorf("mergeback: merge failed and status check failed: %w", mergeErr)
		}
		if hasConflict {
			abortMerge(ctx, wt.RepoPath)
			return &MergeBackResult{
				Success:       false,
				ConflictFiles: conflictFiles,
			}, nil
		}
		// Non-conflict failure.
		abortMerge(ctx, wt.RepoPath)
		return nil, fmt.Errorf("mergeback: merge %q into %q: %w: %s", wtBranch, targetBranch, mergeErr, mergeOut)
	}

	// Merge applied cleanly. Collect the list of merged files.
	merged, _ := mergedFileList(ctx, wt.RepoPath)

	// Commit the merge.
	msg := cfg.message
	if msg == "" {
		msg = fmt.Sprintf("Merge branch '%s' into %s", wtBranch, targetBranch)
	}
	commitArgs := []string{"commit", "-m", msg}
	if _, commitErr := gitCmd(ctx, wt.RepoPath, commitArgs...); commitErr != nil {
		// If nothing to commit (e.g. identical trees), that's still a success.
		_ = mergeOut // already captured
	}

	return &MergeBackResult{
		Success:     true,
		MergedFiles: merged,
	}, nil
}

// DetectConflicts performs a dry-run merge to identify conflicting files
// between the worktree's branch and targetBranch. The repository is left
// in its original state regardless of the outcome.
func DetectConflicts(ctx context.Context, wt *Worktree, targetBranch string) ([]string, error) {
	wtBranch, err := currentBranch(ctx, wt.Path)
	if err != nil {
		return nil, fmt.Errorf("detectconflicts: resolve worktree branch: %w", err)
	}

	// Save the current branch/HEAD so we can restore.
	origBranch, err := currentBranch(ctx, wt.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("detectconflicts: resolve repo branch: %w", err)
	}

	// Checkout target branch.
	if out, err := gitCmd(ctx, wt.RepoPath, "checkout", targetBranch); err != nil {
		return nil, fmt.Errorf("detectconflicts: checkout %q: %w: %s", targetBranch, err, out)
	}

	// Ensure we restore the original branch on exit.
	defer func() {
		abortMerge(ctx, wt.RepoPath)
		_, _ = gitCmd(ctx, wt.RepoPath, "checkout", origBranch)
	}()

	// Attempt a no-commit merge.
	_, mergeErr := gitCmd(ctx, wt.RepoPath, "merge", "--no-commit", "--no-ff", wtBranch)
	if mergeErr == nil {
		// No conflicts.
		return nil, nil
	}

	// Check for conflicts.
	_, conflictFiles, checkErr := HasConflicts(ctx, wt.RepoPath)
	if checkErr != nil {
		return nil, fmt.Errorf("detectconflicts: %w", checkErr)
	}

	return conflictFiles, nil
}

// CopyBack copies specific files from the worktree directory to targetDir.
// If no files are specified, it returns an error. Files are specified as
// paths relative to the worktree root.
func CopyBack(wt *Worktree, targetDir string, files ...string) error {
	if len(files) == 0 {
		return fmt.Errorf("copyback: no files specified")
	}

	for _, f := range files {
		src := filepath.Join(wt.Path, f)
		dst := filepath.Join(targetDir, f)

		// Ensure the destination directory exists.
		if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
			return fmt.Errorf("copyback: mkdir for %q: %w", f, err)
		}

		if err := copyFile(src, dst); err != nil {
			return fmt.Errorf("copyback: %q: %w", f, err)
		}
	}
	return nil
}

// DiffStat holds line-level change counts for a single file.
type DiffStat struct {
	Path    string
	Added   int
	Deleted int
}

// DiffSummaryResult summarizes all changes in a worktree relative to its
// upstream merge base.
type DiffSummaryResult struct {
	AddedFiles    []string   // newly created files
	ModifiedFiles []string   // files with modifications
	DeletedFiles  []string   // files removed
	Stats         []DiffStat // per-file line counts
}

// DiffSummaryFn returns a summary of changes in the worktree compared to
// the merge base with the repository's HEAD.
func DiffSummaryFn(ctx context.Context, wt *Worktree) (*DiffSummaryResult, error) {
	// Get the merge base between the worktree branch and the repo HEAD.
	wtBranch, err := currentBranch(ctx, wt.Path)
	if err != nil {
		return nil, fmt.Errorf("diffsummary: resolve worktree branch: %w", err)
	}

	repoBranch, err := currentBranch(ctx, wt.RepoPath)
	if err != nil {
		return nil, fmt.Errorf("diffsummary: resolve repo branch: %w", err)
	}

	mergeBase, err := gitCmd(ctx, wt.RepoPath, "merge-base", repoBranch, wtBranch)
	if err != nil {
		return nil, fmt.Errorf("diffsummary: merge-base: %w", err)
	}
	mergeBase = strings.TrimSpace(mergeBase)

	// Get numstat diff.
	numstatOut, err := gitCmd(ctx, wt.RepoPath, "diff", "--numstat", mergeBase, wtBranch)
	if err != nil {
		return nil, fmt.Errorf("diffsummary: diff numstat: %w", err)
	}

	// Get name-status diff for added/modified/deleted classification.
	nameStatusOut, err := gitCmd(ctx, wt.RepoPath, "diff", "--name-status", mergeBase, wtBranch)
	if err != nil {
		return nil, fmt.Errorf("diffsummary: diff name-status: %w", err)
	}

	result := &DiffSummaryResult{}

	// Parse name-status output.
	scanner := bufio.NewScanner(strings.NewReader(nameStatusOut))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 2 {
			continue
		}
		status := line[0]
		name := strings.TrimSpace(line[1:])
		// Handle tab-separated format.
		if _, after, ok := strings.Cut(line, "\t"); ok {
			status = line[0]
			name = after
		}

		switch status {
		case 'A':
			result.AddedFiles = append(result.AddedFiles, name)
		case 'M':
			result.ModifiedFiles = append(result.ModifiedFiles, name)
		case 'D':
			result.DeletedFiles = append(result.DeletedFiles, name)
		}
	}

	// Parse numstat output.
	numScanner := bufio.NewScanner(strings.NewReader(numstatOut))
	for numScanner.Scan() {
		line := numScanner.Text()
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 {
			continue
		}
		added, _ := strconv.Atoi(parts[0])
		deleted, _ := strconv.Atoi(parts[1])
		result.Stats = append(result.Stats, DiffStat{
			Path:    parts[2],
			Added:   added,
			Deleted: deleted,
		})
	}

	return result, nil
}

// currentBranch returns the current branch name for the repo at dir.
func currentBranch(ctx context.Context, dir string) (string, error) {
	out, err := gitCmd(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

// gitCmd runs a git command in the given directory and returns combined output.
func gitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", dir}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// mergedFileList returns the list of files staged after a successful merge.
func mergedFileList(ctx context.Context, repoPath string) ([]string, error) {
	out, err := gitCmd(ctx, repoPath, "diff", "--cached", "--name-only")
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	var files []string
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		if f := strings.TrimSpace(scanner.Text()); f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}

// copyFile copies a single file from src to dst, preserving permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
