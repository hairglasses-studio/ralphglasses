//go:build linux

package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CollectChildPIDsFromProc enumerates children by scanning /proc/*/status for
// matching PPid. This is more reliable than pgid on Linux since children may
// have their own process group.
func CollectChildPIDsFromProc(parentPid int) []int {
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
		for line := range strings.SplitSeq(string(data), "\n") {
			if after, ok := strings.CutPrefix(line, "PPid:\t"); ok {
				ppid := strings.TrimSpace(after)
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
