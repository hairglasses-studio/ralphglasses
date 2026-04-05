package tmux

import (
	"fmt"
	"os/exec"
	"strings"
)

// Session represents a tmux session.
type Session struct {
	Name     string `json:"name"`
	Windows  string `json:"windows"`
	Attached bool   `json:"attached"`
}

// Available returns true if tmux is installed and accessible.
func Available() bool {
	_, err := exec.LookPath("tmux")
	return err == nil
}

// ListSessions returns all tmux sessions.
func ListSessions() ([]Session, error) {
	out, err := exec.Command("tmux", "list-sessions", "-F", "#{session_name}\t#{session_windows}\t#{session_attached}").Output()
	if err != nil {
		if strings.Contains(err.Error(), "no server") {
			return nil, nil
		}
		return nil, err
	}

	var sessions []Session
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) < 3 {
			continue
		}
		sessions = append(sessions, Session{
			Name:     parts[0],
			Windows:  parts[1],
			Attached: parts[2] == "1",
		})
	}
	return sessions, nil
}

// CreateSession creates a new tmux session with the given name.
func CreateSession(name string) error {
	return exec.Command("tmux", "new-session", "-d", "-s", name).Run()
}

// NameWindow renames the current window in a session.
func NameWindow(session, name string) error {
	return exec.Command("tmux", "rename-window", "-t", session, name).Run()
}

// KillSession destroys a tmux session by name.
func KillSession(name string) error {
	return exec.Command("tmux", "kill-session", "-t", name).Run()
}

// SendKeys sends keys to a tmux pane.
func SendKeys(target, keys string) error {
	return exec.Command("tmux", "send-keys", "-t", target, keys, "Enter").Run()
}

// Attach attaches to a session (interactive).
func Attach(name string) *exec.Cmd {
	return exec.Command("tmux", "attach-session", "-t", name)
}

// Detach detaches all clients from a session.
func Detach(name string) error {
	return exec.Command("tmux", "detach-client", "-s", name).Run()
}

// SessionExists checks if a named session already exists.
func SessionExists(name string) bool {
	err := exec.Command("tmux", "has-session", "-t", name).Run()
	return err == nil
}

// EnsureSession creates a session if it doesn't exist, returns its name.
func EnsureSession(name string) (string, error) {
	if SessionExists(name) {
		return name, nil
	}
	if err := CreateSession(name); err != nil {
		return "", fmt.Errorf("create tmux session %q: %w", name, err)
	}
	return name, nil
}
