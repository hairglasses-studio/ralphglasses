package gitutil

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// DefaultMaxAge is the default threshold for considering a worktree stale.
const DefaultMaxAge = 24 * time.Hour

// worktreePattern defines a scan location relative to repoPath and the glob
// pattern used to match candidate directories within it.
type worktreePattern struct {
	subdir  string // e.g. ".claude/worktrees"
	pattern string // e.g. "agent-*"
}

var defaultPatterns = []worktreePattern{
	{subdir: filepath.Join(".claude", "worktrees"), pattern: "agent-*"},
	{subdir: filepath.Join(".ralph", "worktrees", "loops"), pattern: "*"},
}

// CleanupStaleWorktrees removes agent/loop worktree directories under repoPath
// that are older than maxAge. It scans both .claude/worktrees/agent-* and
// .ralph/worktrees/loops/* directories.
//
// A directory is skipped when:
//   - It contains a .lock file (currently in use).
//   - git status --porcelain reports uncommitted changes.
//
// Removal is attempted first via "git worktree remove --force"; if that fails
// we fall back to os.RemoveAll.
//
// Returns the count of removed directories. Errors during individual directory
// removal are silently skipped; only a scan-level error (e.g. repoPath does
// not exist) is returned.
func CleanupStaleWorktrees(repoPath string, maxAge time.Duration) (removed int, err error) {
	cutoff := time.Now().Add(-maxAge)

	for _, wp := range defaultPatterns {
		scanDir := filepath.Join(repoPath, wp.subdir)
		entries, readErr := os.ReadDir(scanDir)
		if readErr != nil {
			// Directory may not exist — that is fine, not an error.
			if os.IsNotExist(readErr) {
				continue
			}
			return removed, readErr
		}

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}

			name := entry.Name()

			// Apply glob pattern filter (only relevant for agent-* prefix).
			if matched, _ := filepath.Match(wp.pattern, name); !matched {
				continue
			}

			dirPath := filepath.Join(scanDir, name)

			// Skip directories with a .lock file (in use).
			if _, lockErr := os.Stat(filepath.Join(dirPath, ".lock")); lockErr == nil {
				continue
			}

			// Skip directories younger than maxAge.
			info, infoErr := entry.Info()
			if infoErr != nil {
				continue
			}
			if info.ModTime().After(cutoff) {
				continue
			}

			// Skip directories with uncommitted changes.
			if hasUncommitted(dirPath) {
				continue
			}

			// Try git worktree remove first; fall back to os.RemoveAll.
			if !removeWorktree(repoPath, dirPath) {
				if rmErr := os.RemoveAll(dirPath); rmErr != nil {
					continue
				}
			}
			removed++
		}
	}

	return removed, nil
}

// hasUncommitted returns true if the directory appears to be a git worktree
// with uncommitted changes.
func hasUncommitted(dir string) bool {
	cmd := exec.Command("git", "-C", dir, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		// If git fails (not a git dir, etc.), treat as no uncommitted changes
		// so we can still clean up the directory.
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

// removeWorktree attempts "git worktree remove --force <dir>" from the parent
// repo. Returns true if successful.
func removeWorktree(repoPath, dir string) bool {
	cmd := exec.Command("git", "worktree", "remove", "--force", dir)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// CleanupOrphanedBranches runs "git worktree prune" in the given repo to
// clean up stale worktree references from .git/worktrees. The returned count
// is informational only (git worktree prune does not report a count, so it
// is always 0).
func CleanupOrphanedBranches(repoPath string) (int, error) {
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	if err := cmd.Run(); err != nil {
		return 0, err
	}
	return 0, nil
}
