package session

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// KillReason classifies why a process received a kill signal.
type KillReason int

const (
	// KillReasonUnknown means the cause could not be determined.
	KillReasonUnknown KillReason = iota
	// KillReasonOOM means the process was likely killed by the OS OOM killer.
	KillReasonOOM
	// KillReasonTimeout means the process was killed due to a timeout/budget limit.
	KillReasonTimeout
	// KillReasonUser means the process was stopped by user action.
	KillReasonUser
	// KillReasonBudget means the process was killed because budget was exceeded.
	KillReasonBudget
)

// String returns a human-readable label for the kill reason.
func (r KillReason) String() string {
	switch r {
	case KillReasonOOM:
		return "oom_killed"
	case KillReasonTimeout:
		return "timeout_killed"
	case KillReasonUser:
		return "user_stopped"
	case KillReasonBudget:
		return "budget_killed"
	default:
		return "unknown_killed"
	}
}

// ClassifyExitSignal inspects the error returned by cmd.Wait() when the
// process was killed by a signal (exit code -1) and attempts to determine
// the root cause. This enables better error messages and smarter retry
// decisions for self-improvement loops.
func ClassifyExitSignal(err error, pid int, wasStopped bool) KillReason {
	if err == nil {
		return KillReasonUnknown
	}

	errStr := err.Error()

	// If Stop() was called, it's a user/system stop.
	if wasStopped {
		return KillReasonUser
	}

	// Check for SIGKILL specifically — "signal: killed" in Go's error string.
	if !strings.Contains(errStr, "signal: killed") {
		return KillReasonUnknown
	}

	// On Linux, check the OOM killer log via /proc.
	if runtime.GOOS == "linux" && isOOMKilled(pid) {
		return KillReasonOOM
	}

	// Heuristic: check if system memory is under pressure at the time of kill.
	// If the process was killed and system memory is high, it's likely OOM.
	if isSystemMemoryPressure() {
		return KillReasonOOM
	}

	// Default: if we got "signal: killed" but can't determine OOM,
	// it was most likely a timeout escalation (SIGTERM->SIGKILL).
	return KillReasonTimeout
}

// isOOMKilled checks /proc/<pid>/status or dmesg for OOM kill evidence (Linux only).
func isOOMKilled(pid int) bool {
	// Check /proc/<pid>/oom_score_adj — if the file is gone, the process is dead
	// but we can check dmesg for OOM kill messages.
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err == nil {
		// Process still exists in /proc — not OOM killed.
		_ = data
		return false
	}

	// Try reading kernel log for OOM evidence.
	// This is best-effort — dmesg requires permissions.
	cmd := exec.Command("dmesg", "--ctime", "--level=err,crit")
	output, err := cmd.Output()
	if err != nil {
		return false
	}
	lines := strings.Split(string(output), "\n")
	pidStr := fmt.Sprintf("pid=%d", pid)
	for _, line := range lines {
		if strings.Contains(line, "Out of memory") || strings.Contains(line, "oom_reaper") {
			if strings.Contains(line, pidStr) {
				return true
			}
		}
	}
	return false
}

// isSystemMemoryPressure checks if the Go runtime heap suggests high memory usage.
// This is a heuristic — if HeapAlloc is over 80% of HeapSys, we consider it pressure.
func isSystemMemoryPressure() bool {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	if m.HeapSys == 0 {
		return false
	}
	ratio := float64(m.HeapAlloc) / float64(m.HeapSys)
	return ratio > 0.85 || m.HeapAlloc > 2*OneGB
}

// FormatKillError creates a descriptive error message for a killed process,
// incorporating the classified reason for better observability.
func FormatKillError(originalErr error, reason KillReason) string {
	if originalErr == nil {
		return ""
	}
	switch reason {
	case KillReasonOOM:
		return fmt.Sprintf("process killed by OS (out of memory): %s — consider reducing concurrent workers or session budget", originalErr)
	case KillReasonTimeout:
		return fmt.Sprintf("process killed after timeout escalation (SIGTERM→SIGKILL): %s — session did not exit gracefully within the kill timeout", originalErr)
	case KillReasonUser:
		return "stopped by user"
	case KillReasonBudget:
		return fmt.Sprintf("process killed due to budget limit: %s", originalErr)
	default:
		return originalErr.Error()
	}
}

// IsKillSignalError returns true if the error string indicates the process
// was killed by a signal (SIGKILL). This is the raw "signal: killed" from Go's
// exec package.
func IsKillSignalError(errStr string) bool {
	return strings.Contains(errStr, "signal: killed")
}

// WrapExitReasonWithContext enhances the raw "signal: killed" exit reason
// with classified context for observability and pattern tracking.
func WrapExitReasonWithContext(exitReason string, s *Session) string {
	if !IsKillSignalError(exitReason) {
		return exitReason
	}

	reason := ClassifyExitSignal(fmt.Errorf("%s", exitReason), s.Pid, s.Status == StatusStopped)
	return fmt.Sprintf("%s (%s)", exitReason, reason)
}
