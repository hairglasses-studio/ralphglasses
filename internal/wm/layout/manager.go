package layout

import (
	"fmt"
	"sort"
	"sync"
)

// LayoutManager maps sessions to monitors and manages workspace assignments.
// All methods are safe for concurrent use.
type LayoutManager struct {
	mu     sync.RWMutex
	preset LayoutPreset

	// sessionToMonitor maps session ID → monitor index.
	sessionToMonitor map[string]int

	// monitorSessions maps monitor index → ordered list of session IDs.
	monitorSessions map[int][]string
}

// NewLayoutManager creates a LayoutManager configured with the given preset.
func NewLayoutManager(preset LayoutPreset) *LayoutManager {
	return &LayoutManager{
		preset:           preset,
		sessionToMonitor: make(map[string]int),
		monitorSessions:  make(map[int][]string),
	}
}

// AssignSession assigns a session to the specified monitor index.
// Returns an error if the monitor index is not defined in the preset
// or if the session is already assigned.
func (lm *LayoutManager) AssignSession(sessionID string, monitorIndex int) error {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	if _, ok := lm.preset.MonitorAssignments[monitorIndex]; !ok {
		return fmt.Errorf("monitor index %d not defined in preset %q", monitorIndex, lm.preset.Name)
	}

	if existing, ok := lm.sessionToMonitor[sessionID]; ok {
		return fmt.Errorf("session %q already assigned to monitor %d", sessionID, existing)
	}

	lm.sessionToMonitor[sessionID] = monitorIndex
	lm.monitorSessions[monitorIndex] = append(lm.monitorSessions[monitorIndex], sessionID)
	return nil
}

// UnassignSession removes a session's monitor assignment. If the session
// is not assigned, this is a no-op.
func (lm *LayoutManager) UnassignSession(sessionID string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	monIdx, ok := lm.sessionToMonitor[sessionID]
	if !ok {
		return
	}

	delete(lm.sessionToMonitor, sessionID)

	sessions := lm.monitorSessions[monIdx]
	for i, id := range sessions {
		if id == sessionID {
			lm.monitorSessions[monIdx] = append(sessions[:i], sessions[i+1:]...)
			break
		}
	}
}

// GetMonitorForSession returns the monitor index for a session.
// The bool is false if the session is not assigned.
func (lm *LayoutManager) GetMonitorForSession(sessionID string) (int, bool) {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	idx, ok := lm.sessionToMonitor[sessionID]
	return idx, ok
}

// GetSessionsOnMonitor returns a copy of the session IDs assigned to the
// given monitor, sorted alphabetically for determinism.
func (lm *LayoutManager) GetSessionsOnMonitor(monitorIndex int) []string {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	orig := lm.monitorSessions[monitorIndex]
	if len(orig) == 0 {
		return nil
	}

	result := make([]string, len(orig))
	copy(result, orig)
	sort.Strings(result)
	return result
}

// ApplyLayout validates the current assignments against the preset and returns
// an error if any assigned monitor exceeds its workspace capacity. This is a
// dry-run check — it does not interact with the window manager.
func (lm *LayoutManager) ApplyLayout() error {
	lm.mu.RLock()
	defer lm.mu.RUnlock()

	for monIdx, sessions := range lm.monitorSessions {
		assignment, ok := lm.preset.MonitorAssignments[monIdx]
		if !ok {
			return fmt.Errorf("monitor %d has sessions but is not in preset %q", monIdx, lm.preset.Name)
		}
		if len(sessions) > len(assignment.Workspaces) {
			return fmt.Errorf(
				"monitor %d has %d sessions but only %d workspaces in preset %q",
				monIdx, len(sessions), len(assignment.Workspaces), lm.preset.Name,
			)
		}
	}
	return nil
}
