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

// ViewKeyEntry pairs a key matcher with a handler for table-driven dispatch.
type ViewKeyEntry struct {
	Match   func(m Model, msg tea.KeyMsg) bool
	Handler func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)
}

// dispatchViewKeys iterates entries and invokes the first matching handler.
// Returns (m, nil) when no entry matches, preserving original switch semantics.
func dispatchViewKeys(m Model, msg tea.KeyMsg, entries []ViewKeyEntry) (tea.Model, tea.Cmd) {
	for _, e := range entries {
		if e.Match(m, msg) {
			return e.Handler(m, msg)
		}
	}
	return m, nil
}

var overviewKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.Table.MoveDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.Table.MoveUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Sort) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.Table.CycleSort(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			row := m.Table.SelectedRow()
			if row != nil {
				m.SelectedIdx = m.findRepoByName(row[0])
				if m.SelectedIdx >= 0 {
					m.pushView(ViewRepoDetail, m.Repos[m.SelectedIdx].Name)
				}
			}
			return m, nil
		},
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StartLoop) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.startSelectedLoop() },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StopAction) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.stopSelectedLoop() },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.PauseLoop) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.togglePauseSelected() },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Space) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.Table.ToggleSelect(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.ActionsMenu) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.ActionMenu = &components.ActionMenu{
				Title:  "Actions",
				Items:  components.OverviewActions(),
				Active: true,
				Width:  35,
			}
			return m, nil
		},
	},
}

var detailKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.LogOffset = 0
			m.LogView = views.NewLogView()
			m.LogView.SetDimensions(m.Width, m.Height)
			lines, _ := process.ReadFullLog(m.Repos[m.SelectedIdx].Path)
			m.LogView.SetLines(lines)
			m.pushView(ViewLogs, "Logs")
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.EditConfig) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			repo := m.Repos[m.SelectedIdx]
			if repo.Config != nil {
				m.ConfigEdit = views.NewConfigEditor(repo.Config)
				m.ConfigEdit.Height = m.Height
				m.pushView(ViewConfigEditor, "Config")
			} else {
				m.Notify.Show("No .ralphrc found", 3*time.Second)
			}
			return m, nil
		},
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StartLoop) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.startLoop(m.SelectedIdx) },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StopAction) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.stopLoop(m.SelectedIdx) },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.PauseLoop) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.togglePause(m.SelectedIdx) },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.DiffView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.pushView(ViewDiff, "Diff")
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.ActionsMenu) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.ActionMenu = &components.ActionMenu{
				Title:  "Repo Actions",
				Items:  components.RepoDetailActions(),
				Active: true,
				Width:  35,
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LaunchSession) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			repo := m.Repos[m.SelectedIdx]
			m.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
			m.Launcher.Width = m.Width
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.TimelineView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.pushView(ViewTimeline, "Timeline")
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopHealth) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.pushView(ViewLoopHealth, "Loop Health")
			return m, nil
		},
	},
}

var logKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.ScrollDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.ScrollUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.GotoEnd) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.ScrollToEnd(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.GotoStart) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.ScrollToStart(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.FollowToggle) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.ToggleFollow(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.PageUp) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.PageUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.PageDown) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LogView.PageDown(); return m, nil },
	},
}

var configKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.MoveDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.MoveUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.StartEdit(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.WriteConfig) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if err := m.ConfigEdit.Save(); err != nil {
				m.Notify.Show(fmt.Sprintf("Save error: %v", err), 3*time.Second)
			} else {
				m.Notify.Show("Config saved", 2*time.Second)
			}
			return m, nil
		},
	},
}

var configEditInputKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.ConfirmEdit(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Escape) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.CancelEdit(); return m, nil },
	},
	{
		Match:   func(_ Model, msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.ConfigEdit.Backspace(); return m, nil },
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return len(msg.Runes) == 1 },
		Handler: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.ConfigEdit.TypeChar(msg.Runes[0])
			return m, nil
		},
	},
}

var commandInputKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			cmd := ParseCommand(m.CommandBuf)
			m.InputMode = ModeNormal
			m.CommandBuf = ""
			return m.execCommand(cmd)
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Escape) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.InputMode = ModeNormal
			m.CommandBuf = ""
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if len(m.CommandBuf) > 0 {
				m.CommandBuf = m.CommandBuf[:len(m.CommandBuf)-1]
			}
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return len(msg.Runes) == 1 },
		Handler: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.CommandBuf += string(msg.Runes[0])
			return m, nil
		},
	},
}

var filterInputKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.InputMode = ModeNormal
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Escape) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.InputMode = ModeNormal
			m.Filter.Clear()
			if tbl := m.activeTable(); tbl != nil {
				tbl.SetFilter("")
			}
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.Filter.Backspace()
			if tbl := m.activeTable(); tbl != nil {
				tbl.SetFilter(m.Filter.Text)
			}
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return len(msg.Runes) == 1 },
		Handler: func(m Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.Filter.Type(msg.Runes[0])
			if tbl := m.activeTable(); tbl != nil {
				tbl.SetFilter(m.Filter.Text)
			}
			return m, nil
		},
	},
}

func (m Model) handleOverviewKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, overviewKeys)
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SelectedIdx < 0 || m.SelectedIdx >= len(m.Repos) {
		return m, nil
	}
	return dispatchViewKeys(m, msg, detailKeys)
}

func (m Model) handleLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, logKeys)
}

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	return dispatchViewKeys(m, msg, configKeys)
}

func (m Model) handleConfigEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, configEditInputKeys)
}

func (m Model) handleCommandInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, commandInputKeys)
}

func (m Model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, filterInputKeys)
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

var loopListKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LoopListTable.MoveDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.LoopListTable.MoveUp(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		},
	},
}

var loopControlKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.LoopControlIdx < len(m.LoopControlData)-1 {
				m.LoopControlIdx++
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.LoopControlIdx > 0 {
				m.LoopControlIdx--
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopCtrlStep) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SessMgr == nil || len(m.LoopControlData) == 0 {
				return m, nil
			}
			loopID := m.LoopControlData[m.LoopControlIdx].ID
			sessMgr := m.SessMgr
			return m, func() tea.Msg {
				err := sessMgr.StepLoop(context.Background(), loopID)
				return LoopStepResultMsg{LoopID: loopID, Err: err}
			}
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopCtrlToggle) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SessMgr == nil || len(m.LoopControlData) == 0 {
				return m, nil
			}
			d := m.LoopControlData[m.LoopControlIdx]
			sessMgr := m.SessMgr
			if d.Status == "running" {
				return m, func() tea.Msg {
					err := sessMgr.StopLoop(d.ID)
					return LoopToggleResultMsg{LoopID: d.ID, Started: false, Err: err}
				}
			}
			l, ok := sessMgr.GetLoop(d.ID)
			if !ok {
				m.Notify.Show("Loop not found", 3*time.Second)
				return m, nil
			}
			l.Lock()
			repoPath := l.RepoPath
			l.Unlock()
			return m, func() tea.Msg {
				_, err := sessMgr.StartLoop(context.Background(), repoPath, session.DefaultLoopProfile())
				return LoopToggleResultMsg{LoopID: d.ID, Started: true, Err: err}
			}
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopCtrlPause) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SessMgr == nil || len(m.LoopControlData) == 0 {
				return m, nil
			}
			d := m.LoopControlData[m.LoopControlIdx]
			sessMgr := m.SessMgr
			if d.Paused {
				return m, func() tea.Msg {
					err := sessMgr.ResumeLoop(d.ID)
					return LoopPauseResultMsg{LoopID: d.ID, Paused: false, Err: err}
				}
			}
			return m, func() tea.Msg {
				err := sessMgr.PauseLoop(d.ID)
				return LoopPauseResultMsg{LoopID: d.ID, Paused: true, Err: err}
			}
		},
	},
}

var loopDetailKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopDetailStep) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			loopID := m.SelectedLoop
			sessMgr := m.SessMgr
			return m, func() tea.Msg {
				err := sessMgr.StepLoop(context.Background(), loopID)
				return LoopStepResultMsg{LoopID: loopID, Err: err}
			}
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopDetailToggle) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.LoopDetailPause) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
		},
	},
}

func (m Model) handleLoopListKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, loopListKeys)
}

func (m Model) handleLoopControlKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, loopControlKeys)
}

func (m Model) handleLoopDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.SessMgr == nil || m.SelectedLoop == "" {
		return m, nil
	}
	return dispatchViewKeys(m, msg, loopDetailKeys)
}

// truncateID shortens an ID to 8 characters for display.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// --- Session view key handlers ---

var sessionsKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.SessionTable.MoveDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.SessionTable.MoveUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Sort) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.SessionTable.CycleSort(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			row := m.SessionTable.SelectedRow()
			if row != nil {
				m.SelectedSession = m.findFullSessionID(row[0])
				if m.SelectedSession != "" {
					m.pushView(ViewSessionDetail, row[0])
				}
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StopAction) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, nil
		},
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Space) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.SessionTable.ToggleSelect(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.TimelineView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.pushView(ViewTimeline, "Timeline")
			return m, nil
		},
	},
}

var sessionDetailKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StopAction) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.DiffView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
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
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.ActionsMenu) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.ActionMenu = &components.ActionMenu{
				Title:  "Session Actions",
				Items:  components.SessionDetailActions(),
				Active: true,
				Width:  35,
			}
			return m, nil
		},
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.OutputView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.startOutputStreaming() },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.TimelineView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.pushView(ViewTimeline, "Timeline")
			return m, nil
		},
	},
}

var teamsKeys = []ViewKeyEntry{
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.TeamTable.MoveDown(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.TeamTable.MoveUp(); return m, nil },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Sort) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { m.TeamTable.CycleSort(); return m, nil },
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			row := m.TeamTable.SelectedRow()
			if row != nil {
				m.SelectedTeam = row[0]
				m.pushView(ViewTeamDetail, row[0])
			}
			return m, nil
		},
	},
}

var teamDetailKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SelectedTeam != "" && m.SessMgr != nil {
				team, ok := m.SessMgr.GetTeam(m.SelectedTeam)
				if ok && team.LeadID != "" {
					m.SelectedSession = team.LeadID
					m.pushView(ViewSessionDetail, "Lead Session")
				}
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.TimelineView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SelectedTeam != "" && m.SessMgr != nil {
				if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
					if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
						m.SelectedIdx = idx
					}
					m.pushView(ViewTimeline, "Timeline")
				}
			}
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.DiffView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			if m.SelectedTeam != "" && m.SessMgr != nil {
				if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
					if idx := m.findRepoByPath(team.RepoPath); idx >= 0 {
						m.SelectedIdx = idx
						m.pushView(ViewDiff, "Diff")
					}
				}
			}
			return m, nil
		},
	},
}

var fleetKeys = []ViewKeyEntry{
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Down) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.moveFleetCursor(m.buildFleetData(), 1)
			return m, nil
		},
	},
	{
		Match: func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Up) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.moveFleetCursor(m.buildFleetData(), -1)
			return m, nil
		},
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.Enter) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.openFleetSelection(m.buildFleetData()) },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.StopAction) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.stopFleetSelection(m.buildFleetData()) },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.DiffView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.diffFleetSelection(m.buildFleetData()) },
	},
	{
		Match:   func(m Model, msg tea.KeyMsg) bool { return key.Matches(msg, m.Keys.TimelineView) },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) { return m.timelineFleetSelection(m.buildFleetData()) },
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return msg.Type == tea.KeyTab || msg.Type == tea.KeyRight },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.cycleFleetSection(m.buildFleetData(), 1)
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return msg.Type == tea.KeyLeft },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.cycleFleetSection(m.buildFleetData(), -1)
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return len(msg.Runes) == 1 && msg.Runes[0] == ']' },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.FleetWindow = (m.FleetWindow + 1) % len(fleetWindows)
			return m, nil
		},
	},
	{
		Match: func(_ Model, msg tea.KeyMsg) bool { return len(msg.Runes) == 1 && msg.Runes[0] == '[' },
		Handler: func(m Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
			m.FleetWindow--
			if m.FleetWindow < 0 {
				m.FleetWindow = len(fleetWindows) - 1
			}
			return m, nil
		},
	},
}

func (m Model) handleSessionsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, sessionsKeys)
}

func (m Model) handleSessionDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, sessionDetailKeys)
}

func (m Model) handleTeamsKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, teamsKeys)
}

func (m Model) handleTeamDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, teamDetailKeys)
}

func (m Model) handleFleetKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(m, msg, fleetKeys)
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
