package session

import (
	"fmt"
	"syscall"
)

// OneGB is a convenience constant for 1 gibibyte.
const OneGB = 1024 * 1024 * 1024

// DiskSpaceWarning checks if the given path has less than minFreeBytes available.
// Returns a warning message if low, empty string if OK.
func DiskSpaceWarning(path string, minFreeBytes uint64) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "" // can't check, don't warn
	}
	free := stat.Bavail * uint64(stat.Bsize)
	if free < minFreeBytes {
		return fmt.Sprintf("low disk space on %s: %d MB free (minimum: %d MB)",
			path, free/1024/1024, minFreeBytes/1024/1024)
	}
	return ""
}
