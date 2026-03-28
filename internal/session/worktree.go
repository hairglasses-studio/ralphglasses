package session

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// CleanupLoopWorktrees removes worktree directories for a specific loop.
func CleanupLoopWorktrees(repoPath, loopID string) error {
	if strings.TrimSpace(repoPath) == "" {
		return fmt.Errorf("cleanup: repo path is empty")
	}

	// Sanitize loopID the same way createLoopWorktree does to prevent path traversal.
	sanitized := sanitizeLoopName(loopID)
	if sanitized == "" || sanitized == "loop" && loopID != "loop" {
		return fmt.Errorf("cleanup: invalid loop ID %q", loopID)
	}

	expectedBase := filepath.Join(repoPath, ".ralph", "worktrees", "loops")
	dir := filepath.Join(expectedBase, sanitized)

	// Safety: verify the resolved path is within the expected boundary.
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("cleanup: resolve path: %w", err)
	}
	absBase, err := filepath.Abs(expectedBase)
	if err != nil {
		return fmt.Errorf("cleanup: resolve base: %w", err)
	}
	if !strings.HasPrefix(absDir, absBase+string(filepath.Separator)) {
		return fmt.Errorf("cleanup: path %q escapes worktree boundary %q", absDir, absBase)
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove loop worktrees %s: %w", sanitized, err)
	}
	// Prune stale git worktree references
	cmd := exec.Command("git", "worktree", "prune")
	cmd.Dir = repoPath
	_ = cmd.Run() // best-effort
	return nil
}

// CleanupStaleWorktrees removes loop worktree directories older than the given threshold.
// Returns the number of directories cleaned up.
func CleanupStaleWorktrees(repoPath string, olderThan time.Duration) (int, error) {
	base := filepath.Join(repoPath, ".ralph", "worktrees", "loops")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("read worktree dir: %w", err)
	}

	cutoff := time.Now().Add(-olderThan)
	cleaned := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			p := filepath.Join(base, e.Name())

			// Skip worktrees with an active index.lock file to avoid
			// removing directories that are mid-operation.
			lockPath := filepath.Join(p, ".git", "index.lock")
			if _, err := os.Stat(lockPath); err == nil {
				slog.Debug("skipping worktree with index.lock", "path", p)
				continue
			}

			// Skip worktrees with a .lock file (git worktree lock).
			worktreeLock := p + ".lock"
			if _, err := os.Stat(worktreeLock); err == nil {
				slog.Debug("skipping locked worktree", "path", p)
				continue
			}

			// Skip worktrees with uncommitted changes.
			if worktreeIsDirty(p) {
				slog.Warn("skipping stale worktree with uncommitted changes", "path", p)
				continue
			}

			if err := os.RemoveAll(p); err == nil {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		cmd := exec.Command("git", "worktree", "prune")
		cmd.Dir = repoPath
		_ = cmd.Run()
	}
	return cleaned, nil
}

// worktreeIsDirty returns true if the given path has uncommitted changes
// according to `git status --porcelain`. Returns false if the command
// fails (e.g., path is not a git directory) to avoid blocking cleanup.
func worktreeIsDirty(path string) bool {
	cmd := exec.Command("git", "-C", path, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		return false // assume clean if git fails (not a valid repo)
	}
	return len(strings.TrimSpace(string(out))) > 0
}
