package session

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

// CleanupLoopWorktrees removes worktree directories for a specific loop.
func CleanupLoopWorktrees(repoPath, loopID string) error {
	dir := filepath.Join(repoPath, ".ralph", "worktrees", "loops", loopID)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("remove loop worktrees %s: %w", loopID, err)
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
