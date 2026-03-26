package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

// OrphanInfo describes a process that is still running but has no active
// session managing it.
type OrphanInfo struct {
	PID         int    `json:"pid"`
	SessionFile string `json:"session_file"`
}

// SweepOrphans checks for orphaned process groups from previous sessions.
// It reads persisted session JSON files from the sessions directory under
// ralphDir, extracts PIDs, and checks whether those processes are still
// running (via kill -0) without an active session managing them.
//
// activePIDs should contain PIDs of all currently managed sessions. Any
// running process whose PID is not in activePIDs is reported as an orphan.
func SweepOrphans(ralphDir string, activePIDs map[int]bool) []OrphanInfo {
	sessDir := filepath.Join(ralphDir, "sessions")
	entries, err := os.ReadDir(sessDir)
	if err != nil {
		slog.Debug("reaper: no sessions dir", "path", sessDir, "err", err)
		return nil
	}

	var orphans []OrphanInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessDir, entry.Name()))
		if err != nil {
			slog.Debug("reaper: cannot read session file", "file", entry.Name(), "err", err)
			continue
		}

		pid := extractPID(data)
		if pid <= 0 {
			continue
		}

		if activePIDs != nil && activePIDs[pid] {
			continue
		}

		// kill(pid, 0) checks if the process exists without sending a signal.
		if err := syscall.Kill(pid, 0); err == nil {
			orphans = append(orphans, OrphanInfo{
				PID:         pid,
				SessionFile: entry.Name(),
			})
			slog.Warn("reaper: found orphaned process", "pid", pid, "session", entry.Name())
		}
	}
	return orphans
}

// extractPID pulls the "pid" field from a persisted session JSON blob.
func extractPID(data []byte) int {
	var partial struct {
		PID int `json:"pid"`
	}
	if err := json.Unmarshal(data, &partial); err != nil {
		return 0
	}
	return partial.PID
}
