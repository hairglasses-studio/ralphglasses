package session

import (
	"encoding/json"
	"fmt"
	"os"
)

const activeStateFile = "/tmp/ralphglasses-active.json"

// ActiveState is the on-disk representation of the current session state,
// designed for consumption by starship prompt and external tooling.
type ActiveState struct {
	Name     string `json:"name"`
	Provider string `json:"provider"`
	Status   string `json:"status"`
	Cost     string `json:"cost"`
}

// WriteActiveState writes the session's current state to the active state file.
// Called on every status transition so external tools see real-time updates.
func WriteActiveState(s *Session) error {
	state := ActiveState{
		Name:     s.RepoName,
		Provider: string(s.Provider),
		Status:   string(s.Status),
		Cost:     fmt.Sprintf("$%.4f", s.SpentUSD),
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal active state: %w", err)
	}

	// Atomic write: temp file + rename.
	tmp, err := os.CreateTemp(os.TempDir(), "ralphglasses-active-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp active state: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write active state: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close active state: %w", err)
	}
	if err := os.Rename(tmpName, activeStateFile); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename active state: %w", err)
	}
	return nil
}

// RemoveActiveState deletes the active state file when no sessions are running.
func RemoveActiveState() {
	os.Remove(activeStateFile)
}
