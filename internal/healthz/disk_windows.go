//go:build windows

package healthz

import "fmt"

// diskAvailable is a stub on Windows. A real implementation would call
// GetDiskFreeSpaceEx via syscall, but this project targets Linux/macOS.
func diskAvailable(_ string) (uint64, error) {
	return 0, fmt.Errorf("disk check not implemented on Windows")
}
