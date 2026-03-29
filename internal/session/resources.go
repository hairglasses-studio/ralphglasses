package session

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"syscall"
	"time"
)

// OneGB is a convenience constant for 1 gibibyte.
const OneGB = 1024 * 1024 * 1024

// DefaultMinFreeDiskBytes is the default minimum free disk space (5 GB).
const DefaultMinFreeDiskBytes = 5 * OneGB

// DefaultMaxHeapBytes is the threshold above which a memory pressure warning fires (2 GB).
const DefaultMaxHeapBytes = 2 * OneGB

// DefaultHeapUsageRatio is the HeapAlloc/HeapSys ratio that triggers a memory warning.
const DefaultHeapUsageRatio = 0.9

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

// MemoryPressureWarning checks the Go runtime heap statistics and returns a
// warning if HeapAlloc exceeds maxHeapBytes or if HeapAlloc/HeapSys exceeds
// maxRatio. Returns empty string when memory usage is healthy.
func MemoryPressureWarning(maxHeapBytes uint64, maxRatio float64) string {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	if m.HeapAlloc > maxHeapBytes {
		return fmt.Sprintf("high memory pressure: HeapAlloc=%d MB exceeds threshold %d MB",
			m.HeapAlloc/1024/1024, maxHeapBytes/1024/1024)
	}

	if m.HeapSys > 0 {
		ratio := float64(m.HeapAlloc) / float64(m.HeapSys)
		if ratio > maxRatio {
			return fmt.Sprintf("high memory pressure: HeapAlloc/HeapSys=%.2f exceeds threshold %.2f (HeapAlloc=%d MB, HeapSys=%d MB)",
				ratio, maxRatio, m.HeapAlloc/1024/1024, m.HeapSys/1024/1024)
		}
	}

	return ""
}

// forceGCAndPause triggers a garbage collection cycle and briefly pauses to let
// the OS reclaim memory before launching new sessions. This reduces the chance
// of the OS OOM killer targeting child processes during memory spikes.
func forceGCAndPause() {
	debug.FreeOSMemory()
	runtime.GC()
	time.Sleep(500 * time.Millisecond)
}
