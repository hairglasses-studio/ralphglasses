//go:build !windows

package healthz

import "syscall"

// diskAvailable returns the number of bytes available to the current user
// on the filesystem containing the given path.
func diskAvailable(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}
