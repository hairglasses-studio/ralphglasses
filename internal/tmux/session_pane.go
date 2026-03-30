package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// PaneInfo describes a tmux pane associated with an agent session.
type PaneInfo struct {
	SessionName string `json:"session_name"`
	WindowIndex int    `json:"window_index"`
	PaneID      string `json:"pane_id"`
}

// CreateSessionPane creates a new tmux window/pane for an agent session.
// If the tmux session doesn't exist, it is created first.
// The window is named after the session ID for easy identification.
func CreateSessionPane(tmuxSession, agentSessionID string) (*PaneInfo, error) {
	if !Available() {
		return nil, fmt.Errorf("tmux not available")
	}

	// Ensure the tmux session exists
	if _, err := EnsureSession(tmuxSession); err != nil {
		return nil, fmt.Errorf("ensure tmux session: %w", err)
	}

	// Create a new window named after the agent session
	shortID := agentSessionID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	windowName := "ralph-" + shortID

	out, err := exec.Command("tmux", "new-window", "-t", tmuxSession, "-n", windowName, "-P", "-F", "#{pane_id}").Output()
	if err != nil {
		return nil, fmt.Errorf("create tmux window: %w", err)
	}

	paneID := strings.TrimSpace(string(out))
	return &PaneInfo{
		SessionName: tmuxSession,
		PaneID:      paneID,
	}, nil
}

// CloseSessionPane closes a tmux pane by ID.
func CloseSessionPane(paneID string) error {
	return exec.Command("tmux", "kill-pane", "-t", paneID).Run()
}

// SendToPane sends text to a specific tmux pane.
func SendToPane(paneID, text string) error {
	return exec.Command("tmux", "send-keys", "-t", paneID, text, "Enter").Run()
}

// ListPanes lists all panes in a tmux session with their window names.
func ListPanes(tmuxSession string) ([]PaneInfo, error) {
	out, err := exec.Command("tmux", "list-panes", "-s", "-t", tmuxSession, "-F", "#{window_index}\t#{pane_id}").Output()
	if err != nil {
		return nil, err
	}

	var panes []PaneInfo
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) < 2 {
			continue
		}
		var idx int
		fmt.Sscanf(parts[0], "%d", &idx)
		panes = append(panes, PaneInfo{
			SessionName: tmuxSession,
			WindowIndex: idx,
			PaneID:      parts[1],
		})
	}
	return panes, nil
}
