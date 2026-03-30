//go:build linux

package process

import (
	"os"
	"path/filepath"
)

// cgroupSupportedPlatform checks for cgroup v2 unified hierarchy on Linux.
// The presence of cgroup.controllers at the cgroup root indicates v2.
func cgroupSupportedPlatform() bool {
	_, err := os.Stat(filepath.Join(cgroupRoot(), "cgroup.controllers"))
	return err == nil
}
