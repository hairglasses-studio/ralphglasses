package process

import (
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
)

// scanProcessesFnPtr is an atomic pointer to the scan function, allowing
// tests to stub it without data races against background goroutines.
var scanProcessesFnPtr atomic.Pointer[func() []int]

// orphanLogFnPtr is an atomic pointer to the orphan log function, allowing
// tests to stub it without data races against background goroutines.
var orphanLogFnPtr atomic.Pointer[func(int)]

func init() {
	scanFn := scanRalphLoopProcesses
	scanProcessesFnPtr.Store(&scanFn)

	logFn := func(pid int) {
		slog.Warn("orphaned ralph process detected", "pid", pid)
	}
	orphanLogFnPtr.Store(&logFn)
}

// loadScanProcessesFn loads the current scan function atomically.
func loadScanProcessesFn() func() []int {
	return *scanProcessesFnPtr.Load()
}

// setScanProcessesFn atomically replaces the scan function (for testing).
func setScanProcessesFn(fn func() []int) {
	scanProcessesFnPtr.Store(&fn)
}

// loadOrphanLogFn loads the current orphan log function atomically.
func loadOrphanLogFn() func(int) {
	return *orphanLogFnPtr.Load()
}

// setOrphanLogFn atomically replaces the orphan log function (for testing).
func setOrphanLogFn(fn func(int)) {
	orphanLogFnPtr.Store(&fn)
}

// scanRalphLoopProcesses returns the PIDs of all running processes whose
// command line contains "ralph". On Linux it reads /proc; on other platforms
// it falls back to `ps -eo pid,comm`.
func scanRalphLoopProcesses() []int {
	if runtime.GOOS == "linux" {
		return scanRalphLoopProcessesLinux()
	}
	return scanRalphLoopProcessesPS()
}

func scanRalphLoopProcessesLinux() []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	var pids []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil {
			continue
		}
		// cmdline is NUL-delimited; check for "ralph" anywhere in it.
		cmdline := strings.ReplaceAll(string(data), "\x00", " ")
		if strings.Contains(cmdline, "ralph") {
			pids = append(pids, pid)
		}
	}
	return pids
}

func scanRalphLoopProcessesPS() []int {
	out, err := exec.Command("ps", "-eo", "pid,comm").Output()
	if err != nil {
		return nil
	}
	var pids []int
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		pid, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		if strings.Contains(fields[1], "ralph") {
			pids = append(pids, pid)
		}
	}
	return pids
}

// auditOrphanedProcesses scans for ralph-related processes and logs a warning
// for any PID that is not tracked by this manager.
func (m *Manager) auditOrphanedProcesses() {
	scanned := loadScanProcessesFn()()
	if len(scanned) == 0 {
		return
	}

	m.mu.Lock()
	known := make(map[int]bool, len(m.procs))
	for _, mp := range m.procs {
		known[mp.PID] = true
	}
	m.mu.Unlock()

	for _, pid := range scanned {
		if !known[pid] {
			loadOrphanLogFn()(pid)
		}
	}
}
