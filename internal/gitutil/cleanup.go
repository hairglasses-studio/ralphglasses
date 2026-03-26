package gitutil

import (
	"os"
	"path/filepath"
	"time"
)

// CleanupStaleWorktrees removes agent worktree directories in baseDir that
// are older than maxAge and do not contain a .lock file. It returns the count
// of removed directories and any error encountered during the scan.
func CleanupStaleWorktrees(baseDir string, maxAge time.Duration) (removed int, err error) {
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Only consider directories matching the agent-* pattern.
		if len(name) < 7 || name[:6] != "agent-" {
			continue
		}

		dirPath := filepath.Join(baseDir, name)

		// Skip directories with a .lock file (in use).
		lockPath := filepath.Join(dirPath, ".lock")
		if _, lockErr := os.Stat(lockPath); lockErr == nil {
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

		// Remove the stale directory.
		if rmErr := os.RemoveAll(dirPath); rmErr != nil {
			continue
		}
		removed++
	}

	return removed, nil
}
