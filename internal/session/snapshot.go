package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// SessionSnapshot captures the full serializable state of a session at a
// point in time, enabling pause-at-any-point and resume. This follows
// 12-Factor Agent principle #6: launch/pause/resume with simple APIs.
type SessionSnapshot struct {
	Version    int           `json:"version"`
	SessionID  string        `json:"session_id"`
	CapturedAt time.Time     `json:"captured_at"`

	// Derived state from reducer.
	State SessionState `json:"state"`

	// Event log that produced the state.
	Events []SessionEvent `json:"events"`

	// Iteration context for mid-iteration resume.
	Iteration     int    `json:"iteration"`
	IterationPhase string `json:"iteration_phase,omitempty"` // "plan", "execute", "evaluate"

	// Context window snapshot.
	ContextBudget *ContextBudget `json:"context_budget,omitempty"`

	// Accumulated errors for compaction.
	RecentErrors []LoopError `json:"recent_errors,omitempty"`

	// Pipeline state for micro-agent resume.
	PipelineStep int           `json:"pipeline_step"`
	PipelineResults []TaskResult `json:"pipeline_results,omitempty"`
}

const snapshotVersion = 1

// NewSnapshot creates a snapshot from the current session state and event log.
func NewSnapshot(sessionID string, state SessionState, events []SessionEvent) *SessionSnapshot {
	return &SessionSnapshot{
		Version:    snapshotVersion,
		SessionID:  sessionID,
		CapturedAt: time.Now(),
		State:      state,
		Events:     events,
	}
}

// SaveSnapshot writes a session snapshot to the .ralph directory as JSON.
func SaveSnapshot(repoPath string, snap *SessionSnapshot) error {
	dir := filepath.Join(repoPath, ".ralph", "snapshots")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create snapshot dir: %w", err)
	}

	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal snapshot: %w", err)
	}

	path := filepath.Join(dir, snap.SessionID+".json")

	// Atomic write: write to temp, then rename.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return fmt.Errorf("write snapshot: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("rename snapshot: %w", err)
	}

	return nil
}

// LoadSnapshot reads a session snapshot from the .ralph directory.
func LoadSnapshot(repoPath, sessionID string) (*SessionSnapshot, error) {
	path := filepath.Join(repoPath, ".ralph", "snapshots", sessionID+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("snapshot not found for session %s", sessionID)
		}
		return nil, fmt.Errorf("read snapshot: %w", err)
	}

	var snap SessionSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot: %w", err)
	}

	if snap.Version != snapshotVersion {
		return nil, fmt.Errorf("unsupported snapshot version %d (expected %d)", snap.Version, snapshotVersion)
	}

	return &snap, nil
}

// DeleteSnapshot removes a session snapshot file.
func DeleteSnapshot(repoPath, sessionID string) error {
	path := filepath.Join(repoPath, ".ralph", "snapshots", sessionID+".json")
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove snapshot: %w", err)
	}
	return nil
}

// ResumeFromSnapshot reconstructs the session state by folding the
// stored events through the reducer. Returns the derived state and
// the snapshot's iteration context for mid-iteration resume.
func ResumeFromSnapshot(snap *SessionSnapshot) (SessionState, []SideEffect) {
	return FoldEvents(SessionState{}, snap.Events)
}
