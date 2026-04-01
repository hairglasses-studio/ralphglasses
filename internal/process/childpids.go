package process

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// CollectChildPIDs enumerates child PIDs of the given process.
// It first tries process group membership via Getpgid, then falls back to
// /proc on Linux. Returns an empty (non-nil) slice on any failure.
func CollectChildPIDs(pid int) []int {
	pids := CollectChildPIDsByPgid(pid)
	if len(pids) > 0 {
		return pids
	}
	return CollectChildPIDsFromProc(pid)
}

// CollectChildPIDsByPgid finds processes sharing the same process group as pid.
// On systems without /proc this is the only mechanism available.
func CollectChildPIDsByPgid(pid int) []int {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return []int{}
	}

	// Scan /proc/*/stat for processes in the same pgid (Linux only).
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return []int{}
	}

	var children []int
	for _, entry := range entries {
		childPid, err := strconv.Atoi(entry.Name())
		if err != nil || childPid == pid {
			continue
		}
		statPath := filepath.Join("/proc", entry.Name(), "stat")
		data, err := os.ReadFile(statPath)
		if err != nil {
			continue
		}
		// /proc/<pid>/stat format: pid (comm) state ppid pgrp ...
		// Find closing paren to skip comm field (may contain spaces).
		s := string(data)
		idx := strings.LastIndex(s, ")")
		if idx < 0 || idx+2 >= len(s) {
			continue
		}
		fields := strings.Fields(s[idx+2:])
		if len(fields) < 3 {
			continue
		}
		pgrp, err := strconv.Atoi(fields[2]) // pgrp is 3rd field after ")"
		if err != nil {
			continue
		}
		if pgrp == pgid {
			children = append(children, childPid)
		}
	}
	if children == nil {
		return []int{}
	}
	return children
}
