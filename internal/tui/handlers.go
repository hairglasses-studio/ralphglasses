package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- View-specific key handlers ---

func (m Model) handleOverviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.Table.MoveDown()
	case key.Matches(msg, m.Keys.Up):
		m.Table.MoveUp()
	case key.Matches(msg, m.Keys.Sort):
		m.Table.CycleSort()
	case key.Matches(msg, m.Keys.Enter):
		row := m.Table.SelectedRow()
		if row != nil {
			m.SelectedIdx = m.findRepoByName(row[0])
			if m.SelectedIdx >= 0 {
				m.pushView(ViewRepoDetail, m.Repos[m.SelectedIdx].Name)
			}
		}
	case key.Matches(msg, m.Keys.StartLoop):
		return m.startSelectedLoop()
	case key.Matches(msg, m.Keys.StopAction):
		return m.stopSelectedLoop()
	case key.Matches(msg, m.Keys.PauseLoop):
		return m.togglePauseSelected()
	case key.Matches(msg, m.Keys.Space):
		m.Table.ToggleSelect()
	case key.Matches(msg, m.Keys.ActionsMenu):
		m.ActionMenu = &components.ActionMenu{
			Title:  "Actions",
			Items:  components.OverviewActions(),
			Active: true,
			Width:  35,
		}
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SelectedIdx < 0 || m.SelectedIdx >= len(m.Repos) {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.Keys.Enter):
		m.LogOffset = 0
		m.LogView = views.NewLogView()
		m.LogView.SetDimensions(m.Width, m.Height)
		lines, _ := process.ReadFullLog(m.Repos[m.SelectedIdx].Path)
		m.LogView.SetLines(lines)
		m.pushView(ViewLogs, "Logs")
		return m, nil
	case key.Matches(msg, m.Keys.EditConfig):
		repo := m.Repos[m.SelectedIdx]
		if repo.Config != nil {
			m.ConfigEdit = views.NewConfigEditor(repo.Config)
			m.ConfigEdit.Height = m.Height
			m.pushView(ViewConfigEditor, "Config")
		} else {
			m.Notify.Show("No .ralphrc found", 3*time.Second)
		}
		return m, nil
	case key.Matches(msg, m.Keys.StartLoop):
		return m.startLoop(m.SelectedIdx)
	case key.Matches(msg, m.Keys.StopAction):
		return m.stopLoop(m.SelectedIdx)
	case key.Matches(msg, m.Keys.PauseLoop):
		return m.togglePause(m.SelectedIdx)
	case key.Matches(msg, m.Keys.DiffView):
		m.pushView(ViewDiff, "Diff")
		return m, nil
	case key.Matches(msg, m.Keys.ActionsMenu):
		m.ActionMenu = &components.ActionMenu{
			Title:  "Repo Actions",
			Items:  components.RepoDetailActions(),
			Active: true,
			Width:  35,
		}
		return m, nil
	case key.Matches(msg, m.Keys.LaunchSession):
		repo := m.Repos[m.SelectedIdx]
		m.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
		m.Launcher.Width = m.Width
		return m, nil
	case key.Matches(msg, m.Keys.TimelineView):
		m.pushView(ViewTimeline, "Timeline")
		return m, nil
	case key.Matches(msg, m.Keys.LoopHealth):
		m.pushView(ViewLoopHealth, "Loop Health")
		return m, nil
	}
	return m, nil
}

func (m Model) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.LogView.ScrollDown()
	case key.Matches(msg, m.Keys.Up):
		m.LogView.ScrollUp()
	case key.Matches(msg, m.Keys.GotoEnd):
		m.LogView.ScrollToEnd()
	case key.Matches(msg, m.Keys.GotoStart):
		m.LogView.ScrollToStart()
	case key.Matches(msg, m.Keys.FollowToggle):
		m.LogView.ToggleFollow()
	case key.Matches(msg, m.Keys.PageUp):
		m.LogView.PageUp()
	case key.Matches(msg, m.Keys.PageDown):
		m.LogView.PageDown()
	}
	return m, nil
}

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.ConfigEdit.MoveDown()
	case key.Matches(msg, m.Keys.Up):
		m.ConfigEdit.MoveUp()
	case key.Matches(msg, m.Keys.Enter):
		m.ConfigEdit.StartEdit()
	case key.Matches(msg, m.Keys.WriteConfig):
		if err := m.ConfigEdit.Save(); err != nil {
			m.Notify.Show(fmt.Sprintf("Save error: %v", err), 3*time.Second)
		} else {
			m.Notify.Show("Config saved", 2*time.Second)
		}
	}
	return m, nil
}

func (m Model) handleConfigEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Enter):
		m.ConfigEdit.ConfirmEdit()
	case key.Matches(msg, m.Keys.Escape):
		m.ConfigEdit.CancelEdit()
	case msg.Type == tea.KeyBackspace:
		m.ConfigEdit.Backspace()
	default:
		if len(msg.Runes) == 1 {
			m.ConfigEdit.TypeChar(msg.Runes[0])
		}
	}
	return m, nil
}

func (m Model) handleCommandInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Enter):
		cmd := ParseCommand(m.CommandBuf)
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return m.execCommand(cmd)
	case key.Matches(msg, m.Keys.Escape):
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return m, nil
	case msg.Type == tea.KeyBackspace:
		if len(m.CommandBuf) > 0 {
			m.CommandBuf = m.CommandBuf[:len(m.CommandBuf)-1]
		}
		return m, nil
	default:
		if len(msg.Runes) == 1 {
			m.CommandBuf += string(msg.Runes[0])
		}
		return m, nil
	}
}

func (m Model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	tbl := m.activeTable()
	switch {
	case key.Matches(msg, m.Keys.Enter):
		m.InputMode = ModeNormal
		return m, nil
	case key.Matches(msg, m.Keys.Escape):
		m.InputMode = ModeNormal
		m.Filter.Clear()
		if tbl != nil {
			tbl.SetFilter("")
		}
		return m, nil
	case msg.Type == tea.KeyBackspace:
		m.Filter.Backspace()
		if tbl != nil {
			tbl.SetFilter(m.Filter.Text)
		}
		return m, nil
	default:
		if len(msg.Runes) == 1 {
			m.Filter.Type(msg.Runes[0])
			if tbl != nil {
				tbl.SetFilter(m.Filter.Text)
			}
		}
		return m, nil
	}
}

func (m Model) execCommand(cmd Command) (tea.Model, tea.Cmd) {
	switch cmd.Name {
	case "quit", "q":
		m.ProcMgr.StopAll()
		return m, tea.Quit
	case "scan":
		return m, m.scanRepos()
	case "start":
		if len(cmd.Args) > 0 {
			idx := m.findRepoByName(cmd.Args[0])
			if idx >= 0 {
				return m.startLoop(idx)
			}
			m.Notify.Show(fmt.Sprintf("Repo not found: %s", cmd.Args[0]), 3*time.Second)
		}
	case "stop":
		if len(cmd.Args) > 0 {
			idx := m.findRepoByName(cmd.Args[0])
			if idx >= 0 {
				return m.stopLoop(idx)
			}
			m.Notify.Show(fmt.Sprintf("Repo not found: %s", cmd.Args[0]), 3*time.Second)
		}
	case "stopall":
		m.ProcMgr.StopAll()
		m.Notify.Show("All loops stopped", 3*time.Second)
	case "sessions":
		m.switchTab(1, ViewSessions, "Sessions")
	case "teams":
		m.switchTab(2, ViewTeams, "Teams")
	case "fleet":
		m.switchTab(3, ViewFleet, "Fleet")
	case "repos":
		m.switchTab(0, ViewOverview, "Repos")
	default:
		m.Notify.Show(fmt.Sprintf("Unknown command: %s", cmd.Name), 3*time.Second)
	}
	return m, nil
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
	repo := m.Repos[idx]
	if err := m.ProcMgr.Start(repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Start error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Started loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) stopLoop(idx int) (tea.Model, tea.Cmd) {
	repo := m.Repos[idx]
	if err := m.ProcMgr.Stop(repo.Path); err != nil {
		m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
	} else {
		m.Notify.Show(fmt.Sprintf("Stopped loop: %s", repo.Name), 3*time.Second)
	}
	return m, nil
}

func (m Model) togglePause(idx int) (tea.Model, tea.Cmd) {
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

// --- Loop list view key handlers ---

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
			if err := m.SessMgr.StopLoop(l.ID); err != nil {
				m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
			} else {
				m.Notify.Show(fmt.Sprintf("Stopped: %s", l.RepoName), 3*time.Second)
			}
			return *m, m.loopListCmd()
		}
	}
	m.Notify.Show("Loop not found", 3*time.Second)
	return *m, nil
}

func (m Model) handleLoopListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.LoopListTable.MoveDown()
	case key.Matches(msg, m.Keys.Up):
		m.LoopListTable.MoveUp()
	case key.Matches(msg, m.Keys.Enter):
		if m.LoopListTable == nil {
			return m, nil
		}
		row := m.LoopListTable.SelectedRow()
		if row == nil {
			return m, nil
		}
		prefix := row[0]
		if m.SessMgr != nil {
			for _, l := range m.SessMgr.ListLoops() {
				l.Lock()
				id := l.ID
				l.Unlock()
				if strings.HasPrefix(id, prefix) {
					m.SelectedLoop = id
					break
				}
			}
		}
		if m.SelectedLoop != "" {
			m.pushView(ViewLoopDetail, "Loop Detail")
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleLoopDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil || m.SelectedLoop == "" {
		return m, nil
	}
	switch {
	case key.Matches(msg, m.Keys.LoopDetailStep):
		loopID := m.SelectedLoop
		sessMgr := m.SessMgr
		return m, func() tea.Msg {
			err := sessMgr.StepLoop(context.Background(), loopID)
			return LoopStepResultMsg{LoopID: loopID, Err: err}
		}
	case key.Matches(msg, m.Keys.LoopDetailToggle):
		loopID := m.SelectedLoop
		sessMgr := m.SessMgr
		l, ok := sessMgr.GetLoop(loopID)
		if !ok {
			m.Notify.Show("Loop not found", 3*time.Second)
			return m, nil
		}
		l.Lock()
		status := l.Status
		repoPath := l.RepoPath
		l.Unlock()
		if status == "running" {
			return m, func() tea.Msg {
				err := sessMgr.StopLoop(loopID)
				return LoopToggleResultMsg{LoopID: loopID, Started: false, Err: err}
			}
		}
		return m, func() tea.Msg {
			_, err := sessMgr.StartLoop(context.Background(), repoPath, session.DefaultLoopProfile())
			return LoopToggleResultMsg{LoopID: loopID, Started: true, Err: err}
		}
	case key.Matches(msg, m.Keys.LoopDetailPause):
		loopID := m.SelectedLoop
		sessMgr := m.SessMgr
		l, ok := sessMgr.GetLoop(loopID)
		if !ok {
			m.Notify.Show("Loop not found", 3*time.Second)
			return m, nil
		}
		l.Lock()
		paused := l.Paused
		l.Unlock()
		if paused {
			return m, func() tea.Msg {
				err := sessMgr.ResumeLoop(loopID)
				return LoopPauseResultMsg{LoopID: loopID, Paused: false, Err: err}
			}
		}
		return m, func() tea.Msg {
			err := sessMgr.PauseLoop(loopID)
			return LoopPauseResultMsg{LoopID: loopID, Paused: true, Err: err}
		}
	}
	return m, nil
}

// truncateID shortens an ID to 8 characters for display.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// --- Session view key handlers ---

func (m Model) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.SessionTable.MoveDown()
	case key.Matches(msg, m.Keys.Up):
		m.SessionTable.MoveUp()
	case key.Matches(msg, m.Keys.Sort):
		m.SessionTable.CycleSort()
	case key.Matches(msg, m.Keys.Enter):
		row := m.SessionTable.SelectedRow()
		if row != nil {
			m.SelectedSession = m.findFullSessionID(row[0])
			if m.SelectedSession != "" {
				m.pushView(ViewSessionDetail, row[0])
			}
		}
	case key.Matches(msg, m.Keys.StopAction):
		row := m.SessionTable.SelectedRow()
		if row != nil {
			fullID := m.findFullSessionID(row[0])
			if fullID != "" && m.SessMgr != nil {
				if err := m.SessMgr.Stop(fullID); err != nil {
					m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
				} else {
					m.Notify.Show(fmt.Sprintf("Stopped session %s", row[0]), 3*time.Second)
				}
			}
		}
	case key.Matches(msg, m.Keys.Space):
		m.SessionTable.ToggleSelect()
	case key.Matches(msg, m.Keys.TimelineView):
		m.pushView(ViewTimeline, "Timeline")
	}
	return m, nil
}

func (m Model) handleSessionDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.StopAction):
		if m.SelectedSession != "" && m.SessMgr != nil {
			if err := m.SessMgr.Stop(m.SelectedSession); err != nil {
				m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
			} else {
				id := m.SelectedSession
				if len(id) > 8 {
					id = id[:8]
				}
				m.Notify.Show(fmt.Sprintf("Stopped session %s", id), 3*time.Second)
			}
		}
	case key.Matches(msg, m.Keys.Enter):
		if m.SelectedSession != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
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
	case key.Matches(msg, m.Keys.DiffView):
		if m.SelectedSession != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
				s.Lock()
				repoPath := s.RepoPath
				s.Unlock()
				idx := m.findRepoByPath(repoPath)
				if idx >= 0 {
					m.SelectedIdx = idx
					m.pushView(ViewDiff, "Diff")
				}
			}
		}
	case key.Matches(msg, m.Keys.ActionsMenu):
		m.ActionMenu = &components.ActionMenu{
			Title:  "Session Actions",
			Items:  components.SessionDetailActions(),
			Active: true,
			Width:  35,
		}
	case key.Matches(msg, m.Keys.OutputView):
		return m.startOutputStreaming()
	case key.Matches(msg, m.Keys.TimelineView):
		m.pushView(ViewTimeline, "Timeline")
	}
	return m, nil
}

func (m Model) handleTeamsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.TeamTable.MoveDown()
	case key.Matches(msg, m.Keys.Up):
		m.TeamTable.MoveUp()
	case key.Matches(msg, m.Keys.Sort):
		m.TeamTable.CycleSort()
	case key.Matches(msg, m.Keys.Enter):
		row := m.TeamTable.SelectedRow()
		if row != nil {
			m.SelectedTeam = row[0]
			m.pushView(ViewTeamDetail, row[0])
		}
	}
	return m, nil
}

func (m Model) handleTeamDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case key.Matches(msg, m.Keys.Enter):
		if m.SelectedTeam != "" && m.SessMgr != nil {
			team, ok := m.SessMgr.GetTeam(m.SelectedTeam)
			if ok && team.LeadID != "" {
				m.SelectedSession = team.LeadID
				m.pushView(ViewSessionDetail, "Lead Session")
			}
		}
	case key.Matches(msg, m.Keys.TimelineView):
		if m.SelectedTeam != "" && m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
				if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
					m.SelectedIdx = idx
				}
				m.pushView(ViewTimeline, "Timeline")
			}
		}
	case key.Matches(msg, m.Keys.DiffView):
		if m.SelectedTeam != "" && m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
				if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
					m.SelectedIdx = idx
					m.pushView(ViewDiff, "Diff")
				}
			}
		}
	}
	return m, nil
}

func (m Model) handleFleetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	data := m.buildFleetData()
	switch {
	case key.Matches(msg, m.Keys.Down):
		m.moveFleetCursor(data, 1)
	case key.Matches(msg, m.Keys.Up):
		m.moveFleetCursor(data, -1)
	case key.Matches(msg, m.Keys.Enter):
		return m.openFleetSelection(data)
	case key.Matches(msg, m.Keys.StopAction):
		return m.stopFleetSelection(data)
	case key.Matches(msg, m.Keys.DiffView):
		return m.diffFleetSelection(data)
	case key.Matches(msg, m.Keys.TimelineView):
		return m.timelineFleetSelection(data)
	case msg.Type == tea.KeyTab || msg.Type == tea.KeyRight:
		m.cycleFleetSection(data, 1)
	case msg.Type == tea.KeyLeft:
		m.cycleFleetSection(data, -1)
	case len(msg.Runes) == 1 && msg.Runes[0] == ']':
		m.FleetWindow = (m.FleetWindow + 1) % len(fleetWindows)
	case len(msg.Runes) == 1 && msg.Runes[0] == '[':
		m.FleetWindow--
		if m.FleetWindow < 0 {
			m.FleetWindow = len(fleetWindows) - 1
		}
	}
	return m, nil
}

// --- Modal overlay key handlers ---

func (m Model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyType := "rune"
	switch {
	case key.Matches(msg, m.Keys.Enter):
		keyType = "enter"
	case key.Matches(msg, m.Keys.Escape):
		keyType = "esc"
	case msg.Type == tea.KeyLeft:
		keyType = "left"
	case msg.Type == tea.KeyRight:
		keyType = "right"
	case msg.Type == tea.KeyTab:
		keyType = "tab"
	}
	result, done := m.ConfirmDialog.HandleKey(keyType)
	if done {
		return m.handleConfirmResult(result)
	}
	return m, nil
}

func (m Model) handleActionMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyType := "rune"
	var r rune
	switch {
	case key.Matches(msg, m.Keys.Enter):
		keyType = "enter"
	case key.Matches(msg, m.Keys.Escape):
		keyType = "esc"
	case msg.Type == tea.KeyUp || key.Matches(msg, m.Keys.Up):
		keyType = "up"
	case msg.Type == tea.KeyDown || key.Matches(msg, m.Keys.Down):
		keyType = "down"
	default:
		if len(msg.Runes) == 1 {
			keyType = "rune"
			r = msg.Runes[0]
		}
	}
	result, selected := m.ActionMenu.HandleKey(keyType, r)
	if selected {
		return m.handleActionResult(result)
	}
	return m, nil
}

func (m Model) handleLauncherKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	keyType := "rune"
	var r rune
	switch {
	case key.Matches(msg, m.Keys.Enter):
		keyType = "enter"
	case key.Matches(msg, m.Keys.Escape):
		keyType = "esc"
	case msg.Type == tea.KeyUp || key.Matches(msg, m.Keys.Up):
		keyType = "up"
	case msg.Type == tea.KeyDown || key.Matches(msg, m.Keys.Down):
		keyType = "down"
	case msg.Type == tea.KeyTab:
		keyType = "tab"
	case msg.Type == tea.KeyBackspace:
		keyType = "backspace"
	default:
		if len(msg.Runes) == 1 {
			keyType = "rune"
			r = msg.Runes[0]
		}
	}
	result, submitted := m.Launcher.HandleKey(keyType, r)
	if submitted {
		return m.handleLaunchResult(result)
	}
	return m, nil
}

// --- Result handlers ---

func (m Model) handleConfirmResult(msg components.ConfirmResultMsg) (tea.Model, tea.Cmd) {
	m.ConfirmDialog = nil
	if msg.Result != components.ConfirmYes {
		return m, nil
	}
	switch msg.Action {
	case "stopAll":
		m.ProcMgr.StopAll()
		if m.SessMgr != nil {
			m.SessMgr.StopAll()
		}
		m.Notify.Show("All loops and sessions stopped", 3*time.Second)
	case "stopLoop":
		if idx, ok := msg.Data.(int); ok && idx >= 0 && idx < len(m.Repos) {
			return m.stopLoop(idx)
		}
	case "stopSession":
		if id, ok := msg.Data.(string); ok && id != "" && m.SessMgr != nil {
			if err := m.SessMgr.Stop(id); err != nil {
				m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
			} else {
				short := id
				if len(short) > 8 {
					short = short[:8]
				}
				m.Notify.Show(fmt.Sprintf("Stopped session %s", short), 3*time.Second)
			}
		}
	}
	return m, nil
}

func (m Model) handleActionResult(msg components.ActionResultMsg) (tea.Model, tea.Cmd) {
	m.ActionMenu = nil
	switch msg.Action {
	case "scan":
		return m, m.scanRepos()
	case "startAll":
		for i, r := range m.Repos {
			if r.StatusDisplay() != "running" {
				m.startLoop(i)
			}
		}
		m.Notify.Show("Starting all loops", 3*time.Second)
	case "stopAll":
		m.ConfirmDialog = &components.ConfirmDialog{
			Title:   "Confirm Stop All",
			Message: "Stop all running loops and sessions?",
			Action:  "stopAll",
			Active:  true,
			Width:   50,
		}
	case "startLoop":
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			return m.startLoop(m.SelectedIdx)
		}
	case "stopLoop":
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
	case "pauseLoop":
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			return m.togglePause(m.SelectedIdx)
		}
	case "viewLogs":
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			m.LogOffset = 0
			m.LogView = views.NewLogView()
			m.LogView.SetDimensions(m.Width, m.Height)
			lines, _ := process.ReadFullLog(m.Repos[m.SelectedIdx].Path)
			m.LogView.SetLines(lines)
			m.pushView(ViewLogs, "Logs")
		}
	case "editConfig":
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
			if repo.Config != nil {
				m.ConfigEdit = views.NewConfigEditor(repo.Config)
				m.ConfigEdit.Height = m.Height
				m.pushView(ViewConfigEditor, "Config")
			}
		}
	case "launchSession":
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
			m.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
			m.Launcher.Width = m.Width
		}
	case "viewDiff":
		if m.CurrentView == ViewSessionDetail && m.SelectedSession != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
				s.Lock()
				repoPath := s.RepoPath
				s.Unlock()
				idx := m.findRepoByPath(repoPath)
				if idx >= 0 {
					m.SelectedIdx = idx
					m.pushView(ViewDiff, "Diff")
				}
			}
		} else if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			m.pushView(ViewDiff, "Diff")
		}
	case "stopSession":
		if m.SelectedSession != "" && m.SessMgr != nil {
			m.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop Session",
				Message: "Stop this session?",
				Action:  "stopSession",
				Data:    m.SelectedSession,
				Active:  true,
				Width:   50,
			}
		}
	case "retrySession":
		if m.SelectedSession != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
				s.Lock()
				opts := session.LaunchOptions{
					RepoPath: s.RepoPath,
					Provider: s.Provider,
					Prompt:   s.Prompt,
					Model:    s.Model,
				}
				s.Unlock()
				newS, err := m.SessMgr.Launch(context.Background(), opts)
				if err != nil {
					m.Notify.Show(fmt.Sprintf("Retry error: %v", err), 3*time.Second)
				} else {
					id := newS.ID
					if len(id) > 8 {
						id = id[:8]
					}
					m.Notify.Show(fmt.Sprintf("Retried as %s", id), 3*time.Second)
				}
			}
		}
	case "streamOutput":
		return m.startOutputStreaming()
	}
	return m, nil
}

func (m Model) handleLaunchResult(msg components.LaunchResultMsg) (tea.Model, tea.Cmd) {
	m.Launcher = nil
	if msg.Prompt == "" {
		m.Notify.Show("Launch cancelled (empty prompt)", 2*time.Second)
		return m, nil
	}
	if m.SessMgr == nil {
		m.Notify.Show("Session manager not available", 3*time.Second)
		return m, nil
	}
	opts := session.LaunchOptions{
		RepoPath: msg.RepoPath,
		Provider: session.Provider(msg.Provider),
		Prompt:   msg.Prompt,
		Model:    msg.Model,
	}
	if msg.Agent != "" {
		opts.Agent = msg.Agent
	}
	s, err := m.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		m.Notify.Show(fmt.Sprintf("Launch error: %v", err), 3*time.Second)
		return m, nil
	}
	id := s.ID
	if len(id) > 8 {
		id = id[:8]
	}
	m.SelectedSession = s.ID
	m.Notify.Show(fmt.Sprintf("Launched session %s (%s)", id, msg.Provider), 3*time.Second)
	m.pushView(ViewSessionDetail, id)
	return m, nil
}

// --- Session output streaming ---

func (m Model) startOutputStreaming() (tea.Model, tea.Cmd) {
	if m.SelectedSession == "" || m.SessMgr == nil {
		return m, nil
	}
	m.StreamingSessionID = m.SelectedSession
	m.StreamingOutput = true
	m.SessionOutputView = views.NewLogView()
	m.SessionOutputView.SetDimensions(m.Width, m.Height/2)

	// Pre-fill with output history
	if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
		s.Lock()
		history := make([]string, len(s.OutputHistory))
		copy(history, s.OutputHistory)
		s.Unlock()
		if len(history) > 0 {
			m.SessionOutputView.SetLines(history)
		}
	}

	return m, m.streamSessionOutput(m.SelectedSession)
}

func (m Model) streamSessionOutput(sessionID string) tea.Cmd {
	sessMgr := m.SessMgr
	return func() tea.Msg {
		if sessMgr == nil {
			return SessionOutputDoneMsg{SessionID: sessionID}
		}
		s, ok := sessMgr.Get(sessionID)
		if !ok {
			return SessionOutputDoneMsg{SessionID: sessionID}
		}
		s.Lock()
		ch := s.OutputCh
		s.Unlock()
		if ch == nil {
			return SessionOutputDoneMsg{SessionID: sessionID}
		}
		line, ok := <-ch
		if !ok {
			return SessionOutputDoneMsg{SessionID: sessionID}
		}
		return SessionOutputMsg{SessionID: sessionID, Line: line}
	}
}

// findFullSessionID finds the full session ID from a truncated prefix.
func (m *Model) findFullSessionID(prefix string) string {
	if m.SessMgr == nil {
		return ""
	}
	for _, s := range m.SessMgr.List("") {
		s.Lock()
		id := s.ID
		s.Unlock()
		if strings.HasPrefix(id, prefix) {
			return id
		}
	}
	return ""
}
