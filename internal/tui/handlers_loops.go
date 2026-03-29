package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// --- Loop list dispatch table ---

var loopListKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopListTable.MoveDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopListTable.MoveUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.LoopListTable == nil {
			return *m, nil
		}
		row := m.LoopListTable.SelectedRow()
		if row == nil {
			return *m, nil
		}
		prefix := row[0]
		if m.SessMgr != nil {
			for _, l := range m.SessMgr.ListLoops() {
				l.Lock()
				id := l.ID
				l.Unlock()
				if strings.HasPrefix(id, prefix) {
					m.Sel.LoopID = id
					break
				}
			}
		}
		if m.Sel.LoopID != "" {
			m.pushView(ViewLoopDetail, "Loop Detail")
		}
		return *m, nil
	}},
}

// --- Loop control dispatch table ---

var loopControlKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.LoopControlIdx < len(m.LoopControlData)-1 {
			m.LoopControlIdx++
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.LoopControlIdx > 0 {
			m.LoopControlIdx--
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopCtrlStep }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.SessMgr == nil || len(m.LoopControlData) == 0 {
			return *m, nil
		}
		loopID := m.LoopControlData[m.LoopControlIdx].ID
		sessMgr := m.SessMgr
		return *m, func() tea.Msg {
			err := sessMgr.StepLoop(context.Background(), loopID)
			return LoopStepResultMsg{LoopID: loopID, Err: err}
		}
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopCtrlToggle }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.SessMgr == nil || len(m.LoopControlData) == 0 {
			return *m, nil
		}
		d := m.LoopControlData[m.LoopControlIdx]
		sessMgr := m.SessMgr
		if d.Status == "running" {
			return *m, func() tea.Msg {
				err := sessMgr.StopLoop(d.ID)
				return LoopToggleResultMsg{LoopID: d.ID, Started: false, Err: err}
			}
		}
		l, ok := sessMgr.GetLoop(d.ID)
		if !ok {
			m.Notify.Show("Loop not found", 3*time.Second)
			return *m, nil
		}
		l.Lock()
		repoPath := l.RepoPath
		l.Unlock()
		return *m, func() tea.Msg {
			_, err := sessMgr.StartLoop(context.Background(), repoPath, session.DefaultLoopProfile())
			return LoopToggleResultMsg{LoopID: d.ID, Started: true, Err: err}
		}
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopCtrlPause }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.SessMgr == nil || len(m.LoopControlData) == 0 {
			return *m, nil
		}
		d := m.LoopControlData[m.LoopControlIdx]
		sessMgr := m.SessMgr
		if d.Paused {
			return *m, func() tea.Msg {
				err := sessMgr.ResumeLoop(d.ID)
				return LoopPauseResultMsg{LoopID: d.ID, Paused: false, Err: err}
			}
		}
		return *m, func() tea.Msg {
			err := sessMgr.PauseLoop(d.ID)
			return LoopPauseResultMsg{LoopID: d.ID, Paused: true, Err: err}
		}
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopControlView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopControlView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopControlView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopControlView.Viewport.PageDown()
		return *m, nil
	}},
}

// --- Loop detail dispatch table ---

var loopDetailKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopDetailView.Viewport.PageDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopDetailStep }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		loopID := m.Sel.LoopID
		sessMgr := m.SessMgr
		return *m, func() tea.Msg {
			err := sessMgr.StepLoop(context.Background(), loopID)
			return LoopStepResultMsg{LoopID: loopID, Err: err}
		}
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopDetailToggle }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		loopID := m.Sel.LoopID
		sessMgr := m.SessMgr
		l, ok := sessMgr.GetLoop(loopID)
		if !ok {
			m.Notify.Show("Loop not found", 3*time.Second)
			return *m, nil
		}
		l.Lock()
		status := l.Status
		repoPath := l.RepoPath
		l.Unlock()
		if status == "running" {
			return *m, func() tea.Msg {
				err := sessMgr.StopLoop(loopID)
				return LoopToggleResultMsg{LoopID: loopID, Started: false, Err: err}
			}
		}
		return *m, func() tea.Msg {
			_, err := sessMgr.StartLoop(context.Background(), repoPath, session.DefaultLoopProfile())
			return LoopToggleResultMsg{LoopID: loopID, Started: true, Err: err}
		}
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopDetailPause }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		loopID := m.Sel.LoopID
		sessMgr := m.SessMgr
		l, ok := sessMgr.GetLoop(loopID)
		if !ok {
			m.Notify.Show("Loop not found", 3*time.Second)
			return *m, nil
		}
		l.Lock()
		paused := l.Paused
		l.Unlock()
		if paused {
			return *m, func() tea.Msg {
				err := sessMgr.ResumeLoop(loopID)
				return LoopPauseResultMsg{LoopID: loopID, Paused: false, Err: err}
			}
		}
		return *m, func() tea.Msg {
			err := sessMgr.PauseLoop(loopID)
			return LoopPauseResultMsg{LoopID: loopID, Paused: true, Err: err}
		}
	}},
}

// --- Loop list view standalone handler functions ---

func handleLoopListStart(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.LoopListTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No loop selected", 3*time.Second)
		return *m, nil
	}
	idPrefix := row[0]
	for _, l := range m.SessMgr.ListLoops() {
		if strings.HasPrefix(l.ID, idPrefix) {
			_, err := m.SessMgr.StartLoop(context.Background(), l.RepoPath, session.DefaultLoopProfile())
			if err != nil {
				m.Notify.Show(fmt.Sprintf("Start error: %v", err), 3*time.Second)
			} else {
				m.Notify.Show(fmt.Sprintf("Started loop: %s", l.RepoName), 3*time.Second)
			}
			return *m, m.loopListCmd()
		}
	}
	m.Notify.Show("Loop not found", 3*time.Second)
	return *m, nil
}

func handleLoopListStop(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.LoopListTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No loop selected", 3*time.Second)
		return *m, nil
	}
	idPrefix := row[0]
	for _, l := range m.SessMgr.ListLoops() {
		if strings.HasPrefix(l.ID, idPrefix) {
			m.Modals.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop Loop",
				Message: fmt.Sprintf("Stop loop %s (%s)?", idPrefix, l.RepoName),
				Action:  "stopManagedLoop",
				Data:    l.ID,
				Active:  true,
				Width:   50,
			}
			return *m, nil
		}
	}
	m.Notify.Show("Loop not found", 3*time.Second)
	return *m, nil
}

func handleLoopListPause(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil {
		m.Notify.Show("No session manager", 3*time.Second)
		return *m, nil
	}
	row := m.LoopListTable.SelectedRow()
	if row == nil {
		m.Notify.Show("No loop selected", 3*time.Second)
		return *m, nil
	}
	idPrefix := row[0]
	for _, l := range m.SessMgr.ListLoops() {
		if strings.HasPrefix(l.ID, idPrefix) {
			l.Lock()
			paused := l.Paused
			l.Unlock()
			if paused {
				if err := m.SessMgr.ResumeLoop(l.ID); err != nil {
					m.Notify.Show(fmt.Sprintf("Resume error: %v", err), 3*time.Second)
				} else {
					m.Notify.Show(fmt.Sprintf("Resumed: %s", l.RepoName), 3*time.Second)
				}
			} else {
				if err := m.SessMgr.PauseLoop(l.ID); err != nil {
					m.Notify.Show(fmt.Sprintf("Pause error: %v", err), 3*time.Second)
				} else {
					m.Notify.Show(fmt.Sprintf("Paused: %s", l.RepoName), 3*time.Second)
				}
			}
			return *m, m.loopListCmd()
		}
	}
	m.Notify.Show("Loop not found", 3*time.Second)
	return *m, nil
}

// --- View-specific key handler methods ---

func (m Model) handleLoopListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(loopListKeys, &m, msg)
}

func (m Model) handleLoopControlKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(loopControlKeys, &m, msg)
}

func (m Model) handleLoopDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil || m.Sel.LoopID == "" {
		return m, nil
	}
	return dispatchViewKeys(loopDetailKeys, &m, msg)
}
