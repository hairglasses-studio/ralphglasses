// Package resource provides system resource monitoring for marathon mode.
package resource

import (
	"fmt"
	"os"
	"runtime"
	"syscall"
)

// Status represents the current system resource state.
type Status struct {
	DiskFreeBytes  uint64  `json:"disk_free_bytes"`
	DiskTotalBytes uint64  `json:"disk_total_bytes"`
	DiskUsedPct    float64 `json:"disk_used_pct"`
	MemAllocMB     float64 `json:"mem_alloc_mb"`
	MemSysMB       float64 `json:"mem_sys_mb"`
	NumGoroutine   int     `json:"num_goroutine"`
	Warnings       []string `json:"warnings,omitempty"`
}

// Check performs a system resource check and returns the current status.
// path is the filesystem path to check disk space for (usually the repo or log dir).
func Check(path string) Status {
	s := Status{
		NumGoroutine: runtime.NumGoroutine(),
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	s.MemAllocMB = float64(m.Alloc) / (1024 * 1024)
	s.MemSysMB = float64(m.Sys) / (1024 * 1024)

	// Disk stats
	if path != "" {
		if info, err := diskUsage(path); err == nil {
			s.DiskFreeBytes = info.free
			s.DiskTotalBytes = info.total
			if info.total > 0 {
				s.DiskUsedPct = float64(info.total-info.free) / float64(info.total) * 100
			}
		}
	}

	// Generate warnings
	if s.DiskUsedPct > 95 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("disk usage critical: %.1f%%", s.DiskUsedPct))
	} else if s.DiskUsedPct > 90 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("disk usage high: %.1f%%", s.DiskUsedPct))
	}
	if s.MemAllocMB > 1024 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("high memory allocation: %.0fMB", s.MemAllocMB))
	}
	if s.NumGoroutine > 10000 {
		s.Warnings = append(s.Warnings, fmt.Sprintf("goroutine leak suspected: %d goroutines", s.NumGoroutine))
	}

	return s
}

// IsHealthy returns true if no warnings are present.
func (s Status) IsHealthy() bool {
	return len(s.Warnings) == 0
}

type diskInfo struct {
	total uint64
	free  uint64
}

func diskUsage(path string) (diskInfo, error) {
	// Ensure path exists
	if _, err := os.Stat(path); err != nil {
		return diskInfo{}, err
	}

	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return diskInfo{}, err
	}
	return diskInfo{
		total: stat.Blocks * uint64(stat.Bsize),
		free:  stat.Bavail * uint64(stat.Bsize),
	}, nil
}
