//go:build linux

package process

import (
	"os"
	"strconv"
	"strings"
)

// CollectChildPIDs returns the PIDs of all processes that share the same
// process group as pid. On Linux this is done by scanning /proc for entries
// whose pgrp field (field 5 of /proc/[n]/stat) matches the pgid of pid.
// The target pid itself is excluded from the result.
// Returns nil if the pgid cannot be determined or /proc is unavailable.
func CollectChildPIDs(pid int) []int {
	pgid, err := getpgid(pid)
	if err != nil || pgid <= 0 {
		return nil
	}

	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}

	var result []int
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		entryPID, err := strconv.Atoi(entry.Name())
		if err != nil || entryPID == pid {
			continue
		}
		statPath := "/proc/" + entry.Name() + "/stat"
		data, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}
		// /proc/[pid]/stat layout:
		//   pid (comm) state ppid pgrp session ...
		// (comm) may contain spaces; find the closing ')' from the right.
		s := string(data)
		closeParen := strings.LastIndex(s, ")")
		if closeParen < 0 || closeParen+1 >= len(s) {
			continue
		}
		// After ')': state ppid pgrp ...
		fields := strings.Fields(s[closeParen+1:])
		if len(fields) < 3 {
			continue
		}
		pgrp, err := strconv.Atoi(fields[2])
		if err != nil || pgrp != pgid {
			continue
		}
		result = append(result, entryPID)
	}
	return result
}
