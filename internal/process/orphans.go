package process

import (
	"log"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
)

// scanProcessesFn is the package-level hook used by auditOrphanedProcesses.
// Tests may replace it to inject a controlled set of PIDs.
var scanProcessesFn = scanRalphLoopProcesses

// orphanLogFn is called for each orphaned PID. Tests may replace it to
// capture log output without writing to stderr.
var orphanLogFn = func(pid int) {
	log.Printf("WARNING: orphaned ralph process detected: PID %d is not tracked by this manager", pid)
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
	for _, line := range strings.Split(string(out), "\n") {
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
	scanned := scanProcessesFn()
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
			orphanLogFn(pid)
		}
	}
}
