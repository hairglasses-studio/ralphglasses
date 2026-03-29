package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

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
			m.Sel.RepoIdx = m.findRepoByName(row[0])
			if m.Sel.RepoIdx >= 0 {
				m.pushView(ViewRepoDetail, m.Repos[m.Sel.RepoIdx].Name)
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
		m.Modals.ActionMenu = &components.ActionMenu{
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
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.RepoDetailView.Viewport.PageDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LogOffset = 0
		m.LogView = views.NewLogView()
		m.LogView.SetDimensions(m.Width, m.Height)
		repoPath := m.Repos[m.Sel.RepoIdx].Path
		m.pushView(ViewLogs, "Logs")
		return *m, loadLogCmd(repoPath)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.EditConfig }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		repo := m.Repos[m.Sel.RepoIdx]
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
		return m.startLoop(m.Sel.RepoIdx)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			m.Modals.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop",
				Message: fmt.Sprintf("Stop loop for %s?", m.Repos[m.Sel.RepoIdx].Name),
				Action:  "stopLoop",
				Data:    m.Sel.RepoIdx,
				Active:  true,
				Width:   50,
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PauseLoop }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.togglePause(m.Sel.RepoIdx)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.DiffView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewDiff, "Diff")
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.ActionsMenu }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Modals.ActionMenu = &components.ActionMenu{
			Title:  "Repo Actions",
			Items:  components.RepoDetailActions(),
			Active: true,
			Width:  35,
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.LaunchSession }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		repo := m.Repos[m.Sel.RepoIdx]
		m.Modals.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
		m.Modals.Launcher.Width = m.Width
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
	if m.Sel.RepoIdx < 0 || m.Sel.RepoIdx >= len(m.Repos) {
		return m, nil
	}
	return dispatchViewKeys(detailKeys, &m, msg)
}

func (m Model) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(logKeys, &m, msg)
}

// --- Help view dispatch table ---

var helpKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.HelpView.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleHelpKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(helpKeys, &m, msg)
}

// --- Loop health view dispatch table ---

var loopHealthKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.LoopHealthView.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleLoopHealthKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(loopHealthKeys, &m, msg)
}

// --- Observation view dispatch table ---

var observationKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ObservationViewport.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleObservationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(observationKeys, &m, msg)
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
		m.Modals.ConfirmDialog = &components.ConfirmDialog{
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
	if err := m.ProcMgr.Start(m.Ctx, repo.Path); err != nil {
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
	if err := m.ProcMgr.Stop(m.Ctx, repo.Path); err != nil {
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
