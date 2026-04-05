package tui

import (
	"fmt"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// --- Fleet session table inline keybinds ---
//
// These handlers delegate to the session manager for start/stop/pause/resume
// of the session selected in the fleet session table. They follow the
// same patterns used by handlers_loops.go and handlers_detail.go.

// handleFleetSessionStart launches a new session for the repo associated
// with the selected fleet session table row.
func handleFleetSessionStart(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.FleetView.SessionTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No session selected", 3*time.Second)
		return *m, nil
	}
	// Row[1] = Repo name; find the repo and launch
	repoName := row[1]
	idx := m.findRepoByName(repoName)
	if idx < 0 {
		m.Notify.Show(fmt.Sprintf("Repo not found: %s", repoName), 3*time.Second)
		return *m, nil
	}
	repo := m.Repos[idx]
	sessMgr := m.SessMgr
	ctx := m.Ctx
	repoPath := repo.Path
	return *m, func() tea.Msg {
		s, err := sessMgr.Launch(ctx, session.LaunchOptions{
			RepoPath: repoPath,
			Prompt:   "Continue working on improvements",
		})
		if err != nil {
			return FleetActionResultMsg{Action: "start", Err: err}
		}
		return FleetActionResultMsg{Action: "start", SessionID: s.ID, RepoName: repoName}
	}
}

// handleFleetSessionStop stops the selected session with a confirmation dialog.
func handleFleetSessionStop(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.FleetView.SessionTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No session selected", 3*time.Second)
		return *m, nil
	}
	// Row[0] = Name/ID prefix
	idPrefix := row[0]
	fullID := m.findFullSessionID(idPrefix)
	if fullID == "" {
		m.Notify.Show("Session not found", 3*time.Second)
		return *m, nil
	}
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:   "Confirm Stop Session",
		Message: fmt.Sprintf("Stop session %s?", idPrefix),
		Action:  "stopSession",
		Data:    fullID,
		Active:  true,
		Width:   50,
	}
	return *m, nil
}

// handleFleetSessionPause pauses (SIGSTOP) the selected session's process.
func handleFleetSessionPause(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.FleetView.SessionTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No session selected", 3*time.Second)
		return *m, nil
	}
	// Row[1] = Repo name; delegate to ProcMgr TogglePause
	repoName := row[1]
	idx := m.findRepoByName(repoName)
	if idx < 0 {
		m.Notify.Show(fmt.Sprintf("Repo not found: %s", repoName), 3*time.Second)
		return *m, nil
	}
	return m.togglePause(idx)
}

// handleFleetSessionResume resumes (SIGCONT) the selected session's process.
func handleFleetSessionResume(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.FleetView.SessionTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No session selected", 3*time.Second)
		return *m, nil
	}
	// Row[1] = Repo name; delegate to ProcMgr TogglePause (it toggles)
	repoName := row[1]
	idx := m.findRepoByName(repoName)
	if idx < 0 {
		m.Notify.Show(fmt.Sprintf("Repo not found: %s", repoName), 3*time.Second)
		return *m, nil
	}
	return m.togglePause(idx)
}

// FleetActionResultMsg carries the result of an async fleet session action.
type FleetActionResultMsg struct {
	Action    string // "start", "stop", "pause", "resume"
	SessionID string
	RepoName  string
	Err       error
}
