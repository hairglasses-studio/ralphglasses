package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- Sessions dispatch table ---

var sessionsKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionTable.MoveDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionTable.MoveUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Sort }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionTable.CycleSort()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		row := m.SessionTable.SelectedRow()
		if row != nil {
			m.Sel.SessionID = m.findFullSessionID(row[0])
			if m.Sel.SessionID != "" {
				m.pushView(ViewSessionDetail, row[0])
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		row := m.SessionTable.SelectedRow()
		if row != nil {
			fullID := m.findFullSessionID(row[0])
			if fullID != "" {
				m.Modals.ConfirmDialog = &components.ConfirmDialog{
					Title:   "Confirm Stop Session",
					Message: fmt.Sprintf("Stop session %s?", row[0]),
					Action:  "stopSession",
					Data:    fullID,
					Active:  true,
					Width:   50,
				}
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Space }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionTable.ToggleSelect()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.TimelineView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewTimeline, "Timeline")
		return *m, nil
	}},
}

// --- Session detail dispatch table ---

var sessionDetailKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.SessionDetailView.Viewport.PageDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.SessionID != "" && m.SessMgr != nil {
			shortID := m.Sel.SessionID
			if len(shortID) > 8 {
				shortID = shortID[:8]
			}
			m.Modals.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop Session",
				Message: fmt.Sprintf("Stop session %s?", shortID),
				Action:  "stopSession",
				Data:    m.Sel.SessionID,
				Active:  true,
				Width:   50,
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.SessionID != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
				s.Lock()
				output := s.LastOutput
				s.Unlock()
				m.LogView = views.NewLogView()
				m.LogView.SetDimensions(m.Width, m.Height)
				if output != "" {
					m.LogView.SetLines(strings.Split(output, "\n"))
				}
				m.pushView(ViewLogs, "Session Output")
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.DiffView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.SessionID != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
				s.Lock()
				repoPath := s.RepoPath
				s.Unlock()
				idx := m.findRepoByPath(repoPath)
				if idx >= 0 {
					m.Sel.RepoIdx = idx
					m.pushView(ViewDiff, "Diff")
				}
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.ActionsMenu }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Modals.ActionMenu = &components.ActionMenu{
			Title:  "Session Actions",
			Items:  components.SessionDetailActions(),
			Active: true,
			Width:  35,
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.OutputView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		return m.startOutputStreaming()
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.TimelineView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.pushView(ViewTimeline, "Timeline")
		return *m, nil
	}},
}

// --- Teams dispatch table ---

var teamsKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamTable.MoveDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamTable.MoveUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Sort }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamTable.CycleSort()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		row := m.TeamTable.SelectedRow()
		if row != nil {
			m.Sel.TeamName = row[0]
			m.pushView(ViewTeamDetail, row[0])
		}
		return *m, nil
	}},
}

// --- Team detail dispatch table ---

var teamDetailKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamDetailView.Viewport.PageDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.TeamName != "" && m.SessMgr != nil {
			team, ok := m.SessMgr.GetTeam(m.Sel.TeamName)
			if ok && team.LeadID != "" {
				m.Sel.SessionID = team.LeadID
				m.pushView(ViewSessionDetail, "Lead Session")
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.TimelineView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.TeamName != "" && m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.Sel.TeamName); ok {
				if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
					m.Sel.RepoIdx = idx
				}
				m.pushView(ViewTimeline, "Timeline")
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.DiffView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.TeamName != "" && m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.Sel.TeamName); ok {
				if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
					m.Sel.RepoIdx = idx
					m.pushView(ViewDiff, "Diff")
				}
			}
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.OrchestrationView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.Sel.TeamName != "" {
			m.pushView(ViewTeamOrchestration, "Orchestration")
		}
		return *m, nil
	}},
}

// --- Team orchestration dispatch table ---

var teamOrchestrationKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TeamOrchestrationView.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleTeamOrchestrationKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(teamOrchestrationKeys, &m, msg)
}

// --- Fleet dispatch table ---

var fleetKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		m.moveFleetCursor(data, 1)
		m.FleetView.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		m.moveFleetCursor(data, -1)
		m.FleetView.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.FleetView.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.FleetView.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.FleetView.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.FleetView.Viewport.PageDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		return m.openFleetSelection(data)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.StopAction }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		return m.stopFleetSelection(data)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.DiffView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		return m.diffFleetSelection(data)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.TimelineView }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		return m.timelineFleetSelection(data)
	}},
	{Match: func(msg tea.KeyMsg) bool { return msg.Type == tea.KeyTab || msg.Type == tea.KeyRight }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		m.cycleFleetSection(data, 1)
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return msg.Type == tea.KeyLeft }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		data := m.buildFleetData()
		m.cycleFleetSection(data, -1)
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return len(msg.Runes) == 1 && msg.Runes[0] == ']' }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Fleet.Window = (m.Fleet.Window + 1) % len(fleetWindows)
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return len(msg.Runes) == 1 && msg.Runes[0] == '[' }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Fleet.Window--
		if m.Fleet.Window < 0 {
			m.Fleet.Window = len(fleetWindows) - 1
		}
		return *m, nil
	}},
}

// --- View-specific key handler methods ---

func (m Model) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(sessionsKeys, &m, msg)
}

func (m Model) handleSessionDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(sessionDetailKeys, &m, msg)
}

func (m Model) handleTeamsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(teamsKeys, &m, msg)
}

func (m Model) handleTeamDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(teamDetailKeys, &m, msg)
}

func (m Model) handleFleetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(fleetKeys, &m, msg)
}

// --- Diff view dispatch table ---

var diffKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.DiffViewport.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleDiffKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(diffKeys, &m, msg)
}

// --- Timeline view dispatch table ---

var timelineKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.ScrollDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.ScrollUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoEnd }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.GotoBottom()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.GotoStart }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.GotoTop()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageUp }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.PageUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.PageDown }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.TimelineViewport.Viewport.PageDown()
		return *m, nil
	}},
}

func (m Model) handleTimelineKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(timelineKeys, &m, msg)
}
