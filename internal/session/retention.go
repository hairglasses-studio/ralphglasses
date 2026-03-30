package session

import (
	"os"
	"path/filepath"
	"time"
)

// RetentionPolicy defines age-based cleanup rules for session data files.
type RetentionPolicy struct {
	// MaxAge is the maximum age of data files before cleanup.
	MaxAge time.Duration
	// MaxFiles is the maximum number of files to retain (0 = unlimited).
	MaxFiles int
}

// DefaultRetention is the default retention policy: 30 days, 1000 files.
var DefaultRetention = RetentionPolicy{
	MaxAge:   30 * 24 * time.Hour,
	MaxFiles: 1000,
}

// ApplyRetention removes files in dir that exceed the retention policy.
// Returns the number of files removed.
func ApplyRetention(dir string, policy RetentionPolicy) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	type fileEntry struct {
		name    string
		modTime time.Time
	}

	var files []fileEntry
	now := time.Now()

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{name: e.Name(), modTime: info.ModTime()})
	}

	removed := 0

	// Remove files older than MaxAge
	if policy.MaxAge > 0 {
		cutoff := now.Add(-policy.MaxAge)
		var kept []fileEntry
		for _, f := range files {
			if f.modTime.Before(cutoff) {
				if err := os.Remove(filepath.Join(dir, f.name)); err == nil {
					removed++
				}
			} else {
				kept = append(kept, f)
			}
		}
		files = kept
	}

	// Remove oldest files if over MaxFiles limit
	if policy.MaxFiles > 0 && len(files) > policy.MaxFiles {
		// Sort by mod time descending (newest first)
		for i := 0; i < len(files); i++ {
			for j := i + 1; j < len(files); j++ {
				if files[j].modTime.After(files[i].modTime) {
					files[i], files[j] = files[j], files[i]
				}
			}
		}
		// Remove excess (oldest)
		for _, f := range files[policy.MaxFiles:] {
			if err := os.Remove(filepath.Join(dir, f.name)); err == nil {
				removed++
			}
		}
	}

	return removed, nil
}
