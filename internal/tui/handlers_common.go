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

// ViewKeyEntry pairs a key matcher with a handler for view-specific dispatch.
// If Binding is non-nil it is checked via key.Matches; otherwise Match is used.
type ViewKeyEntry struct {
	Binding func(km *KeyMap) key.Binding
	Match   func(msg tea.KeyMsg) bool
	Handler KeyHandler
}

// dispatchViewKeys iterates entries in order; first match wins.
// Returns the matched handler's result, or (*m, nil) if nothing matched.
func dispatchViewKeys(entries []ViewKeyEntry, m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	for _, e := range entries {
		if e.Binding != nil {
			if key.Matches(msg, e.Binding(&m.Keys)) {
				return e.Handler(m, msg)
			}
		} else if e.Match != nil && e.Match(msg) {
			return e.Handler(m, msg)
		}
	}
	return *m, nil
}

// --- Command/filter input dispatch tables ---

var commandInputKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		cmd := ParseCommand(m.CommandBuf)
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return m.execCommand(cmd)
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Escape }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.InputMode = ModeNormal
		m.CommandBuf = ""
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if len(m.CommandBuf) > 0 {
			m.CommandBuf = m.CommandBuf[:len(m.CommandBuf)-1]
		}
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return true }, Handler: func(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
		if len(msg.Runes) == 1 {
			m.CommandBuf += string(msg.Runes[0])
		}
		return *m, nil
	}},
}

var filterInputKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.InputMode = ModeNormal
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Escape }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.InputMode = ModeNormal
		m.Filter.Clear()
		if tbl := m.activeTable(); tbl != nil {
			tbl.SetFilter("")
		}
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.Filter.Backspace()
		if tbl := m.activeTable(); tbl != nil {
			tbl.SetFilter(m.Filter.Text)
		}
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return true }, Handler: func(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
		if len(msg.Runes) == 1 {
			m.Filter.Type(msg.Runes[0])
			if tbl := m.activeTable(); tbl != nil {
				tbl.SetFilter(m.Filter.Text)
			}
		}
		return *m, nil
	}},
}

func (m Model) handleCommandInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(commandInputKeys, &m, msg)
}

func (m Model) handleFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(filterInputKeys, &m, msg)
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

// --- Utility helpers ---

// truncateID shortens an ID to 8 characters for display.
func truncateID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
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

// --- Event log view dispatch table ---

var eventLogKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.EventLog != nil {
			m.EventLog.ScrollDown()
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.EventLog != nil {
			m.EventLog.ScrollUp()
		}
		return *m, nil
	}},
}

func (m Model) handleEventLogKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(eventLogKeys, &m, msg)
}
