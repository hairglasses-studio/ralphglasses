package process

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ReapOrphans scans for abandoned claude, gemini, or codex child processes
// (e.g. by parsing ps -A -o pid,ppid,command output) and kills them.
// This is typically called on startup to clean up leaked provider sessions.
func ReapOrphans() error {
	cmd := exec.Command("ps", "-A", "-o", "pid,ppid,command")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list processes: %w", err)
	}

	lines := bytes.Split(out, []byte("\n"))
	if len(lines) <= 1 {
		return nil
	}

	for _, line := range lines[1:] {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}

		fields := strings.Fields(string(line))
		if len(fields) < 3 {
			continue
		}

		pidStr := fields[0]
		ppidStr := fields[1]
		command := strings.Join(fields[2:], " ")

		// Look for orphaned processes: reparented to init/systemd/launchd (PPID == 1)
		if ppidStr != "1" {
			continue
		}

		lowerCmd := strings.ToLower(command)
		isTarget := strings.Contains(lowerCmd, "claude") ||
			strings.Contains(lowerCmd, "gemini") ||
			strings.Contains(lowerCmd, "codex")

		if !isTarget {
			continue
		}

		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}

		slog.Warn("reaping orphaned process", "pid", pid, "command", command)
		
		proc, err := os.FindProcess(pid)
		if err == nil {
			_ = proc.Kill()
		}
	}

	return nil
}
