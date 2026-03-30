// Package worktree wraps git worktree operations (add, list, remove, prune)
// with context-aware cancellation and structured error reporting.
//
// This package is intentionally independent — it has no imports from other
// internal packages and can be used standalone.
package worktree

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// WorktreeInfo holds parsed metadata for a single git worktree entry
// as returned by `git worktree list --porcelain`.
type WorktreeInfo struct {
	Path       string // filesystem path to the worktree
	Branch     string // full ref (e.g. "refs/heads/main"), empty if detached
	HEAD       string // commit SHA at HEAD
	IsBare     bool   // true for the bare worktree entry
	IsDetached bool   // true when HEAD is detached (no branch)
}

// Create adds a new git worktree at worktreePath, checked out on a new branch.
// It runs: git worktree add -b <branch> <worktreePath>
//
// The repoPath must point to an existing git repository (or worktree within one).
func Create(ctx context.Context, repoPath, worktreePath, branch string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "add", "-b", branch, worktreePath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: create %q branch %q: %w: %s", worktreePath, branch, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CreateFromRef adds a new git worktree at worktreePath based on an existing ref
// (branch, tag, or commit), without creating a new branch.
// It runs: git worktree add <worktreePath> <ref>
func CreateFromRef(ctx context.Context, repoPath, worktreePath, ref string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "add", worktreePath, ref)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: create from ref %q at %q: %w: %s", ref, worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// List returns all worktrees registered in the repository at repoPath.
// It parses the porcelain output of `git worktree list --porcelain`.
func List(ctx context.Context, repoPath string) ([]WorktreeInfo, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "list", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return nil, fmt.Errorf("worktree: list: %w: %s", err, stderr)
	}
	return parsePorcelain(out)
}

// Remove removes the worktree at worktreePath from the repository.
// If force is true, it passes --force to allow removal of dirty worktrees.
func Remove(ctx context.Context, repoPath, worktreePath string, force bool) error {
	args := []string{"-C", repoPath, "worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, worktreePath)

	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: remove %q: %w: %s", worktreePath, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Prune removes stale worktree administrative entries (e.g. when the worktree
// directory has been deleted manually).
func Prune(ctx context.Context, repoPath string) error {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "worktree", "prune")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: prune: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// parsePorcelain parses the output of `git worktree list --porcelain`.
//
// Porcelain format emits blocks separated by blank lines. Each block contains:
//
//	worktree <path>
//	HEAD <sha>
//	branch <ref>        (or "detached" on a line by itself)
//	bare                (optional, for bare repos)
func parsePorcelain(data []byte) ([]WorktreeInfo, error) {
	var result []WorktreeInfo
	var current *WorktreeInfo

	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()

		// Blank line terminates a worktree entry.
		if line == "" {
			if current != nil {
				result = append(result, *current)
				current = nil
			}
			continue
		}

		switch {
		case strings.HasPrefix(line, "worktree "):
			current = &WorktreeInfo{
				Path: strings.TrimPrefix(line, "worktree "),
			}
		case strings.HasPrefix(line, "HEAD "):
			if current != nil {
				current.HEAD = strings.TrimPrefix(line, "HEAD ")
			}
		case strings.HasPrefix(line, "branch "):
			if current != nil {
				current.Branch = strings.TrimPrefix(line, "branch ")
			}
		case line == "detached":
			if current != nil {
				current.IsDetached = true
			}
		case line == "bare":
			if current != nil {
				current.IsBare = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("worktree: parse porcelain: %w", err)
	}

	// Handle final entry if there's no trailing blank line.
	if current != nil {
		result = append(result, *current)
	}

	return result, nil
}

// ---------------------------------------------------------------------------
// Worktree struct API — higher-level wrapper around the flat functions above
// ---------------------------------------------------------------------------

// Worktree represents a git worktree with associated metadata.
type Worktree struct {
	Path       string    // filesystem path to the worktree
	Branch     string    // branch checked out in this worktree
	BaseBranch string    // the branch this worktree was created from
	CreatedAt  time.Time // when the worktree was created
	RepoPath   string    // filesystem path to the main repository
}

// WorktreeStatus describes the state of files in a worktree.
type WorktreeStatus struct {
	Clean    bool     // true when there are no uncommitted changes
	Modified []string // files with modifications (staged or unstaged)
	Added    []string // untracked files
	Deleted  []string // deleted files
}

// Option configures worktree creation.
type Option func(*createOpts)

type createOpts struct {
	baseBranch string // branch to base the new worktree on (default: current HEAD)
	path       string // override the worktree path (default: derived from repoPath)
	orphan     bool   // create an orphan branch with no history
}

// WithBaseBranch sets the base branch (start-point) for the new worktree.
func WithBaseBranch(branch string) Option {
	return func(o *createOpts) { o.baseBranch = branch }
}

// WithPath overrides the filesystem path where the worktree will be created.
func WithPath(path string) Option {
	return func(o *createOpts) { o.path = path }
}

// WithOrphan creates the worktree with an orphan branch (no parent commit).
func WithOrphan() Option {
	return func(o *createOpts) { o.orphan = true }
}

// EnsureGit validates that the git binary is available on $PATH.
func EnsureGit() error {
	_, err := exec.LookPath("git")
	if err != nil {
		return fmt.Errorf("worktree: git not found on PATH: %w", err)
	}
	return nil
}

// CreateWorktree creates a new git worktree for the given branch.
// It returns a *Worktree with metadata populated. The repoPath must point to
// an existing git repository.
func CreateWorktree(ctx context.Context, repoPath, branch string, opts ...Option) (*Worktree, error) {
	if err := EnsureGit(); err != nil {
		return nil, err
	}

	o := &createOpts{}
	for _, fn := range opts {
		fn(o)
	}

	// Determine worktree path.
	wtPath := o.path
	if wtPath == "" {
		wtPath = repoPath + "-" + branch
	}

	// Build the git worktree add command.
	var args []string
	if o.orphan {
		// git worktree add --orphan -b <branch> <path>
		// Note: --orphan requires git >= 2.38.
		args = []string{"-C", repoPath, "worktree", "add", "--orphan", "-b", branch, wtPath}
	} else if o.baseBranch != "" {
		// git worktree add -b <branch> <path> <start-point>
		args = []string{"-C", repoPath, "worktree", "add", "-b", branch, wtPath, o.baseBranch}
	} else {
		// git worktree add -b <branch> <path>
		args = []string{"-C", repoPath, "worktree", "add", "-b", branch, wtPath}
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("worktree: create %q branch %q: %w: %s", wtPath, branch, err, strings.TrimSpace(string(out)))
	}

	// Determine the base branch name for metadata.
	baseBranch := o.baseBranch
	if baseBranch == "" && !o.orphan {
		// Default: the branch that was checked out when we created.
		base := gitOutput(ctx, repoPath, "rev-parse", "--abbrev-ref", "HEAD")
		if base != "" {
			baseBranch = base
		}
	}

	return &Worktree{
		Path:       wtPath,
		Branch:     branch,
		BaseBranch: baseBranch,
		CreatedAt:  time.Now(),
	}, nil
}

// ListWorktrees returns all worktrees as []*Worktree for the repository.
func ListWorktrees(ctx context.Context, repoPath string) ([]*Worktree, error) {
	if err := EnsureGit(); err != nil {
		return nil, err
	}

	infos, err := List(ctx, repoPath)
	if err != nil {
		return nil, err
	}

	wts := make([]*Worktree, len(infos))
	for i, info := range infos {
		branch := info.Branch
		// Strip refs/heads/ prefix for readability.
		branch = strings.TrimPrefix(branch, "refs/heads/")
		wts[i] = &Worktree{
			Path:   info.Path,
			Branch: branch,
		}
	}
	return wts, nil
}

// RemoveWorktree removes the given worktree and cleans up git references.
func RemoveWorktree(ctx context.Context, repoPath string, wt *Worktree) error {
	if err := EnsureGit(); err != nil {
		return err
	}
	if wt == nil {
		return fmt.Errorf("worktree: nil worktree")
	}

	if err := Remove(ctx, repoPath, wt.Path, true); err != nil {
		return err
	}

	// Prune stale references (best-effort).
	_ = Prune(ctx, repoPath)
	return nil
}

// Checkout checks out the given ref (branch, tag, or commit SHA) in the worktree.
func Checkout(ctx context.Context, wt *Worktree, ref string) error {
	if err := EnsureGit(); err != nil {
		return err
	}
	if wt == nil {
		return fmt.Errorf("worktree: nil worktree")
	}

	cmd := exec.CommandContext(ctx, "git", "-C", wt.Path, "checkout", ref)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("worktree: checkout %q in %q: %w: %s", ref, wt.Path, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Status returns the git status of the worktree as a WorktreeStatus.
func Status(ctx context.Context, wt *Worktree) (WorktreeStatus, error) {
	if err := EnsureGit(); err != nil {
		return WorktreeStatus{}, err
	}
	if wt == nil {
		return WorktreeStatus{}, fmt.Errorf("worktree: nil worktree")
	}

	cmd := exec.CommandContext(ctx, "git", "-C", wt.Path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		stderr := ""
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = strings.TrimSpace(string(ee.Stderr))
		}
		return WorktreeStatus{}, fmt.Errorf("worktree: status %q: %w: %s", wt.Path, err, stderr)
	}

	var st WorktreeStatus
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if len(line) < 3 {
			continue
		}
		xy := line[:2]
		file := strings.TrimSpace(line[3:])

		switch {
		case xy == "??" || xy == "A " || xy == " A":
			st.Added = append(st.Added, file)
		case xy == " D" || xy == "D " || xy == "DD":
			st.Deleted = append(st.Deleted, file)
		default:
			st.Modified = append(st.Modified, file)
		}
	}
	if err := scanner.Err(); err != nil {
		return WorktreeStatus{}, fmt.Errorf("worktree: parse status: %w", err)
	}

	st.Clean = len(st.Modified) == 0 && len(st.Added) == 0 && len(st.Deleted) == 0
	return st, nil
}

// IsClean returns true if the worktree has no uncommitted changes.
func IsClean(ctx context.Context, wt *Worktree) (bool, error) {
	st, err := Status(ctx, wt)
	if err != nil {
		return false, err
	}
	return st.Clean, nil
}

// gitOutput runs a git command and returns trimmed stdout, or empty on error.
func gitOutput(ctx context.Context, repoPath string, args ...string) string {
	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(ctx, "git", fullArgs...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
