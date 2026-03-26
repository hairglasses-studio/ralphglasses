package tui

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- Overview (repo list) dispatch table ---

var overviewKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Table.MoveDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Table.MoveUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Sort }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Table.CycleSort()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		row := m.Table.SelectedRow()
		if row != nil {
			m.SelectedIdx = m.findRepoByName(row[0])
			if m.SelectedIdx >= 0 {
				m.pushView(ViewRepoDetail, m.Repos[m.SelectedIdx].Name)
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StartLoop }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.startSelectedLoop()
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.confirmStopSelectedLoop()
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PauseLoop }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.togglePauseSelected()
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Space }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Table.ToggleSelect()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.ActionsMenu }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ActionMenu = &components.ActionMenu{
			Title:  "Actions",
			Items:  components.OverviewActions(),
			Active: true,
			Width:  35,
		}
		return *m, nil
	}},
}

// --- Repo detail dispatch table ---

var detailKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogOffset = 0
		m.LogView = views.NewLogView()
		m.LogView.SetDimensions(m.Width, m.Height)
		lines, _ := process.ReadFullLog(m.Repos[m.SelectedIdx].Path)
		m.LogView.SetLines(lines)
		m.pushView(ViewLogs, "Logs")
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.EditConfig }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		repo := m.Repos[m.SelectedIdx]
		if repo.Config != nil {
			m.ConfigEdit = views.NewConfigEditor(repo.Config)
			m.ConfigEdit.Height = m.Height
			m.pushView(ViewConfigEditor, "Config")
		} else {
			m.Notify.Show("No .ralphrc found", 3*time.Second)
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StartLoop }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.startLoop(m.SelectedIdx)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			m.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop",
				Message: fmt.Sprintf("Stop loop for %s?", m.Repos[m.SelectedIdx].Name),
				Action:  "stopLoop",
				Data:    m.SelectedIdx,
				Active:  true,
				Width:   50,
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PauseLoop }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.togglePause(m.SelectedIdx)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.DiffView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewDiff, "Diff")
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.ActionsMenu }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ActionMenu = &components.ActionMenu{
			Title:  "Repo Actions",
			Items:  components.RepoDetailActions(),
			Active: true,
			Width:  35,
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LaunchSession }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		repo := m.Repos[m.SelectedIdx]
		m.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
		m.Launcher.Width = m.Width
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.TimelineView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewTimeline, "Timeline")
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LoopHealth }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewLoopHealth, "Loop Health")
		return *m, nil
	}},
}

// --- Log view dispatch table ---

var logKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.ScrollToEnd()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.ScrollToStart()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.FollowToggle }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.ToggleFollow()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogView.PageDown()
		return *m, nil
	}},
}

// --- View-specific key handler methods ---

func (m Model) handleOverviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(overviewKeys, &m, msg)
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SelectedIdx < 0 || m.SelectedIdx >= len(m.Repos) {
		return m, nil
	}
	return dispatchViewKeys(detailKeys, &m, msg)
}

func (m Model) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(logKeys, &m, msg)
}

// --- Process management helpers ---

func (m Model) startSelectedLoop() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.startLoop(idx)
	}
	return m, nil
}

func (m Model) stopSelectedLoop() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.stopLoop(idx)
	}
	return m, nil
}

func (m Model) confirmStopSelectedLoop() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 && idx < len(m.Repos) {
		m.ConfirmDialog = &components.ConfirmDialog{
			Title:   "Confirm Stop",
			Message: fmt.Sprintf("Stop loop for %s?", m.Repos[idx].Name),
			Action:  "stopLoop",
			Data:    idx,
			Active:  true,
			Width:   50,
		}
	}
	return m, nil
}

func (m Model) togglePauseSelected() (tea.Model, tea.Cmd) {
	row := m.Table.SelectedRow()
	if row == nil {
		return m, nil
	}
	idx := m.findRepoByName(row[0])
	if idx >= 0 {
		return m.togglePause(idx)
	}
	return m, nil
}

func (m Model) startLoop(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.Repos) {
		return m, nil
	}
	repo := m.Repos[idx]
	if err := m.ProcMgr.Start(context.TODO(), repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Start error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Started loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) stopLoop(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.Repos) {
		return m, nil
	}
	repo := m.Repos[idx]
	if err := m.ProcMgr.Stop(context.TODO(), repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Stopped loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) togglePause(idx int) (tea.Model, tea.Cmd) {
	if idx < 0 || idx >= len(m.Repos) {
		return m, nil
	}
	repo := m.Repos[idx]
	paused, err := m.ProcMgr.TogglePause(repo.Path)
	if err != nil {
		m.Notify.Show(fmt.Sprintf("Pause error: %v", err), 3*time.Second)
	} else if paused {
		m.Notify.Show(fmt.Sprintf("Paused: %s", repo.Name), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Resumed: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}
