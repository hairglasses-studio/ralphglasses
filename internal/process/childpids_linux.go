//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// collectChildPIDsFromProc enumerates children by scanning /proc/*/status for
// matching PPid. This is more reliable than pgid on Linux since children may
// have their own process group.
func collectChildPIDsFromProc(parentPid int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{}
	}

	parentStr := strconv.Itoa(parentPid)
	var children []int
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		statusPath := filepath.Join("/proc", entry.Name(), "status")
		data, err := os.ReadFile(statusPath)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "PPid:\t") {
				ppid := strings.TrimSpace(strings.TrimPrefix(line, "PPid:\t"))
				if ppid == parentStr {
					if childPid, err := strconv.Atoi(entry.Name()); err == nil {
						children = append(children, childPid)
					}
				}
				break
			}
		}
	}
	if children == nil {
		return []int{}
	}
	return children
}
