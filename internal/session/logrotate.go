package session

import (
	"io"
	"os"
	"path/filepath"
)

// MaxLogSize is the default maximum log file size before rotation (100 MB).
const MaxLogSize = 100 * 1024 * 1024

// RotateLogs truncates log files in the given directory that exceed maxBytes.
// For each oversized file, the last maxBytes/2 bytes are preserved (tail preservation).
// Returns the number of files truncated.
func RotateLogs(logDir string, maxBytes int64) (int, error) {
	entries, err := os.ReadDir(logDir)
	if err != nil {
		return 0, err
	}

	keep := max(maxBytes/2, 0)

	truncated := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Size() <= maxBytes {
			continue
		}

		fpath := filepath.Join(logDir, entry.Name())
		if err := truncateFile(fpath, info.Size(), keep); err != nil {
			return truncated, err
		}
		truncated++
	}
	return truncated, nil
}

// truncateFile keeps the last keepBytes of a file, discarding the rest.
func truncateFile(path string, size, keepBytes int64) error {
	if keepBytes <= 0 {
		return os.Truncate(path, 0)
	}

	f, err := os.Open(path)
	if err != nil {
		return err
	}

	// Seek to the tail portion we want to keep.
	offset := max(size-keepBytes, 0)
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		f.Close()
		return err
	}

	tail, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		return err
	}

	return os.WriteFile(path, tail, 0o644)
}
