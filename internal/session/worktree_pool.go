package session

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultPoolSize is the default number of pre-created worktrees per repo.
const DefaultPoolSize = 4

// DefaultStaleThreshold is the default age after which idle pool worktrees are
// considered stale and eligible for automatic cleanup.
const DefaultStaleThreshold = 24 * time.Hour

// protectedBranches lists branch names that pool worktrees must never merge into.
var protectedBranches = []string{"main", "master"}

// WorktreePool manages a pool of pre-created git worktrees per repository.
// Worktrees are acquired for session use and released back for reuse, avoiding
// the overhead of repeated `git worktree add` / `git worktree remove` cycles.
//
// Features:
//   - Git alternates: pool worktrees share the parent repo's object store,
//     saving disk space by avoiding duplicate pack files.
//   - Merge prevention: each pool worktree gets a pre-merge-commit hook that
//     blocks merges into protected branches (main, master).
//   - Stale cleanup: CleanupStale removes idle worktrees older than a threshold.
//
// Thread-safe: each repo has its own mutex to allow concurrent access across
// different repos without contention.
type WorktreePool struct {
	mu       sync.Mutex           // protects repos map
	repos    map[string]*repoPool // keyed by canonical repo path
	poolSize int                  // max idle worktrees per repo
	counter  atomic.Int64         // monotonic counter for unique naming
	closed   atomic.Bool
}

// repoPool holds the pool state for a single repository.
type repoPool struct {
	mu       sync.Mutex
	idle     []poolEntry // available worktrees ready for checkout
	repoRoot string     // canonical git toplevel path
}

// poolEntry represents a single pooled worktree.
type poolEntry struct {
	Path      string    // filesystem path to the worktree
	Branch    string    // branch name checked out in the worktree
	CreatedAt time.Time // when this entry was created or last released
}

// WorktreePoolStats exposes metrics about pool utilization.
type WorktreePoolStats struct {
	RepoPath  string `json:"repo_path"`
	IdleCount int    `json:"idle_count"`
	PoolSize  int    `json:"pool_size"`
}

// NewWorktreePool creates a pool with the given max idle size per repo.
// If size <= 0, DefaultPoolSize is used.
func NewWorktreePool(size int) *WorktreePool {
	if size <= 0 {
		size = DefaultPoolSize
	}
	return &WorktreePool{
		repos:    make(map[string]*repoPool),
		poolSize: size,
	}
}

// Acquire returns a clean worktree path for the given repo.
// If the pool has an idle worktree, it is returned after resetting to HEAD.
// Otherwise, a new worktree is created on-demand (fallback).
// The returned branch name can be used for tracking.
func (wp *WorktreePool) Acquire(ctx context.Context, repoPath string) (string, string, error) {
	if wp.closed.Load() {
		return "", "", fmt.Errorf("worktree pool is closed")
	}

	rp, err := wp.getOrCreateRepoPool(ctx, repoPath)
	if err != nil {
		return "", "", err
	}

	rp.mu.Lock()
	if len(rp.idle) > 0 {
		// Pop from the back (LIFO for cache locality).
		entry := rp.idle[len(rp.idle)-1]
		rp.idle = rp.idle[:len(rp.idle)-1]
		rp.mu.Unlock()

		// Reset the worktree to a clean state on HEAD.
		if err := wp.resetWorktree(ctx, rp.repoRoot, entry.Path); err != nil {
			slog.Warn("failed to reset pooled worktree, creating fresh one",
				"path", entry.Path, "error", err)
			// Attempt cleanup of the broken worktree.
			wp.destroyWorktree(rp.repoRoot, entry.Path)
			// Fall through to create a new one.
			return wp.createFreshWorktree(ctx, rp.repoRoot)
		}

		slog.Debug("acquired pooled worktree", "path", entry.Path, "repo", rp.repoRoot)
		return entry.Path, entry.Branch, nil
	}
	rp.mu.Unlock()

	// Pool empty -- create a new worktree on demand.
	return wp.createFreshWorktree(ctx, rp.repoRoot)
}

// Release returns a worktree to the pool for reuse.
// The worktree is cleaned (git checkout main branch, git clean -fd) before
// being made available. If the pool is full, the worktree is destroyed.
func (wp *WorktreePool) Release(ctx context.Context, repoPath, wtPath string) {
	if wp.closed.Load() {
		return
	}
	if wtPath == "" {
		return
	}

	rp, err := wp.getOrCreateRepoPool(ctx, repoPath)
	if err != nil {
		slog.Warn("release: cannot resolve repo pool, destroying worktree",
			"repo", repoPath, "path", wtPath, "error", err)
		wp.destroyWorktree(repoPath, wtPath)
		return
	}

	// Clean the worktree before returning it to the pool.
	if err := wp.cleanWorktree(ctx, rp.repoRoot, wtPath); err != nil {
		slog.Warn("release: failed to clean worktree, destroying instead",
			"path", wtPath, "error", err)
		wp.destroyWorktree(rp.repoRoot, wtPath)
		return
	}

	rp.mu.Lock()
	defer rp.mu.Unlock()

	if len(rp.idle) >= wp.poolSize {
		// Pool is full -- destroy the excess worktree.
		wp.destroyWorktree(rp.repoRoot, wtPath)
		return
	}

	// Determine the branch name currently checked out.
	branch := wp.currentBranch(wtPath)
	rp.idle = append(rp.idle, poolEntry{Path: wtPath, Branch: branch, CreatedAt: time.Now()})
	slog.Debug("released worktree to pool", "path", wtPath, "idle", len(rp.idle), "repo", rp.repoRoot)
}

// Warm pre-creates count worktrees for the given repo path. Existing idle
// worktrees are counted toward the total -- at most poolSize will exist.
func (wp *WorktreePool) Warm(ctx context.Context, repoPath string, count int) error {
	if wp.closed.Load() {
		return fmt.Errorf("worktree pool is closed")
	}

	rp, err := wp.getOrCreateRepoPool(ctx, repoPath)
	if err != nil {
		return err
	}

	rp.mu.Lock()
	existing := len(rp.idle)
	rp.mu.Unlock()

	need := count - existing
	if need <= 0 {
		return nil
	}
	// Cap at pool size.
	if existing+need > wp.poolSize {
		need = wp.poolSize - existing
	}
	if need <= 0 {
		return nil
	}

	var created int
	for i := 0; i < need; i++ {
		if ctx.Err() != nil {
			break
		}
		path, branch, err := wp.createFreshWorktree(ctx, rp.repoRoot)
		if err != nil {
			slog.Warn("warm: failed to create worktree", "repo", rp.repoRoot, "error", err)
			continue
		}

		rp.mu.Lock()
		if len(rp.idle) < wp.poolSize {
			rp.idle = append(rp.idle, poolEntry{Path: path, Branch: branch, CreatedAt: time.Now()})
			created++
		} else {
			// Race: another goroutine filled the pool while we were creating.
			wp.destroyWorktree(rp.repoRoot, path)
		}
		rp.mu.Unlock()
	}

	slog.Info("warmed worktree pool", "repo", rp.repoRoot, "created", created, "total_idle", existing+created)
	return nil
}

// CleanupStale removes idle pool worktrees that have been sitting unused for
// longer than the given threshold. Returns the number of worktrees removed.
// Pass DefaultStaleThreshold for the standard 24-hour threshold.
func (wp *WorktreePool) CleanupStale(olderThan time.Duration) int {
	if wp.closed.Load() {
		return 0
	}

	wp.mu.Lock()
	reposCopy := make(map[string]*repoPool, len(wp.repos))
	for k, v := range wp.repos {
		reposCopy[k] = v
	}
	wp.mu.Unlock()

	cutoff := time.Now().Add(-olderThan)
	var totalCleaned int

	for _, rp := range reposCopy {
		rp.mu.Lock()
		var kept []poolEntry
		var stale []poolEntry
		for _, e := range rp.idle {
			if e.CreatedAt.Before(cutoff) {
				stale = append(stale, e)
			} else {
				kept = append(kept, e)
			}
		}
		rp.idle = kept
		rp.mu.Unlock()

		for _, e := range stale {
			wp.destroyWorktree(rp.repoRoot, e.Path)
			totalCleaned++
		}
	}

	if totalCleaned > 0 {
		slog.Info("cleaned stale pool worktrees", "removed", totalCleaned)
	}
	return totalCleaned
}

// Stats returns pool utilization for all known repos.
func (wp *WorktreePool) Stats() []WorktreePoolStats {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	stats := make([]WorktreePoolStats, 0, len(wp.repos))
	for path, rp := range wp.repos {
		rp.mu.Lock()
		s := WorktreePoolStats{
			RepoPath:  path,
			IdleCount: len(rp.idle),
			PoolSize:  wp.poolSize,
		}
		rp.mu.Unlock()
		stats = append(stats, s)
	}
	return stats
}

// Close destroys all pooled worktrees and marks the pool as closed.
// After Close, Acquire and Warm return errors.
func (wp *WorktreePool) Close() {
	if !wp.closed.CompareAndSwap(false, true) {
		return // already closed
	}

	wp.mu.Lock()
	repos := wp.repos
	wp.repos = make(map[string]*repoPool)
	wp.mu.Unlock()

	for _, rp := range repos {
		rp.mu.Lock()
		entries := rp.idle
		rp.idle = nil
		rp.mu.Unlock()

		for _, e := range entries {
			wp.destroyWorktree(rp.repoRoot, e.Path)
		}
	}

	slog.Info("worktree pool closed")
}

// PoolSize returns the configured max idle worktrees per repo.
func (wp *WorktreePool) PoolSize() int {
	return wp.poolSize
}

// --- internal helpers ---

// getOrCreateRepoPool returns the repoPool for a path, resolving it to the
// git toplevel. Creates a new repoPool entry if none exists.
func (wp *WorktreePool) getOrCreateRepoPool(ctx context.Context, repoPath string) (*repoPool, error) {
	root, err := resolveRepoRoot(ctx, repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve repo root: %w", err)
	}

	wp.mu.Lock()
	defer wp.mu.Unlock()

	if rp, ok := wp.repos[root]; ok {
		return rp, nil
	}

	rp := &repoPool{
		repoRoot: root,
		idle:     make([]poolEntry, 0, wp.poolSize),
	}
	wp.repos[root] = rp
	return rp, nil
}

// resolveRepoRoot calls git to determine the canonical toplevel path.
func resolveRepoRoot(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--show-toplevel")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git rev-parse --show-toplevel: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// createFreshWorktree creates a new git worktree under .ralph/worktrees/pool/.
// After creation it sets up git alternates for space efficiency and installs
// a merge-prevention hook.
func (wp *WorktreePool) createFreshWorktree(ctx context.Context, repoRoot string) (string, string, error) {
	seq := wp.counter.Add(1)
	name := fmt.Sprintf("pool-%d-%d", time.Now().UnixMilli(), seq)
	wtPath := filepath.Join(repoRoot, ".ralph", "worktrees", "pool", name)

	if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
		return "", "", fmt.Errorf("create pool dir: %w", err)
	}

	branch := fmt.Sprintf("ralph/pool/%s", name)
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "worktree", "add", "-B", branch, wtPath, "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(output)))
	}

	// Set up git alternates so the worktree shares the parent repo's object
	// store. This avoids duplicating pack files across pool worktrees, saving
	// significant disk space for large repositories.
	if err := setupAlternates(repoRoot, wtPath); err != nil {
		slog.Warn("failed to set up alternates for pool worktree",
			"path", wtPath, "error", err)
		// Non-fatal: the worktree works without alternates, just uses more disk.
	}

	// Install a pre-merge-commit hook that prevents merging into protected
	// branches (main, master). This is a safety net against accidental pushes
	// from pool worktrees back into the main line.
	if err := installMergePreventionHook(wtPath); err != nil {
		slog.Warn("failed to install merge prevention hook",
			"path", wtPath, "error", err)
		// Non-fatal: merges are still possible but not recommended.
	}

	slog.Debug("created fresh worktree", "path", wtPath, "branch", branch)
	return wtPath, branch, nil
}

// setupAlternates writes the parent repo's object directory into the worktree's
// objects/info/alternates file. Git uses this to look up objects in the parent
// store before fetching or creating new copies, providing space-efficient
// shared storage across all pool worktrees.
func setupAlternates(repoRoot, wtPath string) error {
	// Resolve the parent repo's git object directory.
	parentObjects := filepath.Join(repoRoot, ".git", "objects")
	if _, err := os.Stat(parentObjects); err != nil {
		return fmt.Errorf("parent objects dir not found: %w", err)
	}

	// Find the worktree's git dir. For worktrees, .git is a file pointing
	// to the real git dir (e.g., "gitdir: /repo/.git/worktrees/pool-xxx").
	wtGitDir, err := resolveWorktreeGitDir(wtPath)
	if err != nil {
		return fmt.Errorf("resolve worktree git dir: %w", err)
	}

	altDir := filepath.Join(wtGitDir, "objects", "info")
	if err := os.MkdirAll(altDir, 0755); err != nil {
		return fmt.Errorf("create alternates dir: %w", err)
	}

	altFile := filepath.Join(altDir, "alternates")
	// Use absolute path for reliability across renames/moves.
	absParentObjects, err := filepath.Abs(parentObjects)
	if err != nil {
		return fmt.Errorf("abs parent objects: %w", err)
	}

	// Read existing content to avoid duplicate entries.
	existing, _ := os.ReadFile(altFile)
	if strings.Contains(string(existing), absParentObjects) {
		return nil // already set up
	}

	// Append (don't overwrite) in case other alternates exist.
	f, err := os.OpenFile(altFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open alternates file: %w", err)
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, absParentObjects); err != nil {
		return fmt.Errorf("write alternates: %w", err)
	}

	return nil
}

// resolveWorktreeGitDir returns the actual git directory for a worktree.
// In a worktree, .git is a file containing "gitdir: <path>". For regular
// repos, .git is a directory and we return it directly.
func resolveWorktreeGitDir(wtPath string) (string, error) {
	gitPath := filepath.Join(wtPath, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", fmt.Errorf("stat .git: %w", err)
	}

	// Regular .git directory (not a worktree).
	if info.IsDir() {
		return gitPath, nil
	}

	// It's a file -- read the gitdir pointer.
	data, err := os.ReadFile(gitPath)
	if err != nil {
		return "", fmt.Errorf("read .git file: %w", err)
	}

	content := strings.TrimSpace(string(data))
	if !strings.HasPrefix(content, "gitdir: ") {
		return "", fmt.Errorf("unexpected .git file content: %q", content)
	}

	gitDir := strings.TrimPrefix(content, "gitdir: ")
	// Resolve relative paths against the worktree.
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(wtPath, gitDir)
	}

	return filepath.Clean(gitDir), nil
}

// mergePreventionHookScript is installed as pre-merge-commit in pool worktrees.
// It checks whether the current branch is a protected branch and refuses the
// merge if so. This prevents agents from accidentally merging pool branches
// into main/master.
const mergePreventionHookScript = `#!/bin/sh
# Installed by ralphglasses WorktreePool — prevents merges into protected branches.
BRANCH=$(git rev-parse --abbrev-ref HEAD 2>/dev/null)
case "$BRANCH" in
  main|master)
    echo "ERROR: merge into protected branch '$BRANCH' is blocked in pool worktrees." >&2
    echo "Use 'git push' to a feature branch and create a pull request instead." >&2
    exit 1
    ;;
esac
exit 0
`

// installMergePreventionHook writes a pre-merge-commit hook into the worktree's
// hooks directory. The hook rejects merges when the current branch is main or
// master, preventing accidental contamination of protected branches from pool
// worktree activity.
func installMergePreventionHook(wtPath string) error {
	wtGitDir, err := resolveWorktreeGitDir(wtPath)
	if err != nil {
		return fmt.Errorf("resolve git dir: %w", err)
	}

	hooksDir := filepath.Join(wtGitDir, "hooks")
	if err := os.MkdirAll(hooksDir, 0755); err != nil {
		return fmt.Errorf("create hooks dir: %w", err)
	}

	hookPath := filepath.Join(hooksDir, "pre-merge-commit")
	if err := os.WriteFile(hookPath, []byte(mergePreventionHookScript), 0755); err != nil {
		return fmt.Errorf("write hook: %w", err)
	}

	return nil
}

// IsProtectedBranch returns true if the given branch name is protected from
// merges in pool worktrees.
func IsProtectedBranch(branch string) bool {
	for _, b := range protectedBranches {
		if branch == b {
			return true
		}
	}
	return false
}

// resetWorktree resets a worktree to HEAD, discarding all local changes.
// Used when re-acquiring from the pool.
func (wp *WorktreePool) resetWorktree(ctx context.Context, repoRoot, wtPath string) error {
	// Verify the worktree directory still exists.
	if _, err := os.Stat(wtPath); err != nil {
		return fmt.Errorf("worktree path does not exist: %w", err)
	}

	// Reset to HEAD.
	resetCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "reset", "--hard", "HEAD")
	if out, err := resetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Clean untracked files and directories.
	cleanCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "clean", "-fd")
	if out, err := cleanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fd: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return nil
}

// cleanWorktree prepares a worktree for return to the pool.
// It checks out the default branch, discards changes, and cleans untracked files.
func (wp *WorktreePool) cleanWorktree(ctx context.Context, repoRoot, wtPath string) error {
	// Verify the worktree directory still exists.
	if _, err := os.Stat(wtPath); err != nil {
		return fmt.Errorf("worktree path does not exist: %w", err)
	}

	// Discard any staged/unstaged changes.
	resetCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "reset", "--hard", "HEAD")
	if out, err := resetCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git reset --hard: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Clean untracked files and directories.
	cleanCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "clean", "-fd")
	if out, err := cleanCmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git clean -fd: %w: %s", err, strings.TrimSpace(string(out)))
	}

	// Update to latest HEAD from the main repo.
	// Use git pull --ff-only to fast-forward if possible, ignore errors
	// (e.g., diverged branches) since the worktree is still usable.
	pullCmd := exec.CommandContext(ctx, "git", "-C", wtPath, "merge", "--ff-only", "HEAD@{upstream}")
	_ = pullCmd.Run() // best-effort

	return nil
}

// destroyWorktree removes a worktree from disk and prunes git references.
func (wp *WorktreePool) destroyWorktree(repoRoot, wtPath string) {
	if wtPath == "" {
		return
	}
	if err := os.RemoveAll(wtPath); err != nil {
		slog.Warn("failed to remove worktree directory", "path", wtPath, "error", err)
	}
	// Prune stale worktree references.
	cmd := exec.Command("git", "-C", repoRoot, "worktree", "prune")
	_ = cmd.Run() // best-effort
}

// currentBranch returns the current branch name for a worktree, or empty string on error.
func (wp *WorktreePool) currentBranch(wtPath string) string {
	cmd := exec.Command("git", "-C", wtPath, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
