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
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
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
		m.ProcMgr.StopAll(m.Ctx)
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
		m.Modals.ConfirmDialog = &components.ConfirmDialog{
			Title:   "Confirm Stop All",
			Message: "Stop all running loops and sessions?",
			Action:  "stopAll",
			Active:  true,
			Width:   50,
		}
	case "sessions":
		m.switchTab(1, ViewSessions, "Sessions")
	case "teams":
		m.switchTab(2, ViewTeams, "Teams")
	case "fleet":
		m.switchTab(3, ViewFleet, "Fleet")
	case "repos":
		m.switchTab(0, ViewOverview, "Repos")
	case "launch":
		// :launch <repo> [model] — launch a new session from discovered repos
		if len(cmd.Args) == 0 {
			// Show list of available repos
			var names []string
			for _, r := range m.Repos {
				names = append(names, r.Name)
			}
			if len(names) == 0 {
				m.Notify.Show("No repos discovered. Run :scan first.", 3*time.Second)
			} else {
				m.Notify.Show(fmt.Sprintf("Usage: :launch <repo> [model] — repos: %s", strings.Join(names, ", ")), 4*time.Second)
			}
			return m, nil
		}
		idx := m.findRepoByName(cmd.Args[0])
		if idx < 0 {
			m.Notify.Show(fmt.Sprintf("Repo not found: %s", cmd.Args[0]), 3*time.Second)
			return m, nil
		}
		launchModel := ""
		if len(cmd.Args) > 1 {
			launchModel = cmd.Args[1]
		}
		repo := m.Repos[idx]
		opts := session.LaunchOptions{
			RepoPath: repo.Path,
			Prompt:   "Continue working on improvements",
			Model:    launchModel,
		}
		s, err := m.SessMgr.Launch(context.Background(), opts)
		if err != nil {
			m.Notify.Show(fmt.Sprintf("Launch failed: %s", err), 4*time.Second)
			return m, nil
		}
		m.Notify.Show(fmt.Sprintf("Launched session %s for %s", s.ID[:8], repo.Name), 3*time.Second)
		return m, nil
	case "theme":
		if len(cmd.Args) == 0 {
			names := make([]string, 0)
			for n := range styles.DefaultThemes() {
				names = append(names, n)
			}
			m.Notify.Show(fmt.Sprintf("Usage: :theme <name> — available: %s", strings.Join(names, ", ")), 4*time.Second)
			return m, nil
		}
		if t := styles.ResolveTheme(cmd.Args[0]); t != nil {
			styles.ApplyTheme(t)
			m.Notify.Show(fmt.Sprintf("Theme: %s", t.Name), 2*time.Second)
		} else {
			m.Notify.Show(fmt.Sprintf("Theme not found: %s", cmd.Args[0]), 3*time.Second)
		}
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
	case len(msg.Runes) == 1 && (msg.Runes[0] == 'y' || msg.Runes[0] == 'Y'):
		keyType = "y"
	case len(msg.Runes) == 1 && (msg.Runes[0] == 'n' || msg.Runes[0] == 'N'):
		keyType = "n"
	}
	result, done := m.Modals.ConfirmDialog.HandleKey(keyType)
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
	result, selected := m.Modals.ActionMenu.HandleKey(keyType, r)
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
	result, submitted := m.Modals.Launcher.HandleKey(keyType, r)
	if submitted {
		return m.handleLaunchResult(result)
	}
	return m, nil
}

// --- Result handlers ---

func (m Model) handleConfirmResult(msg components.ConfirmResultMsg) (tea.Model, tea.Cmd) {
	m.Modals.ConfirmDialog = nil
	if msg.Result != components.ConfirmYes {
		return m, nil
	}
	switch msg.Action {
	case "stopAll":
		m.ProcMgr.StopAll(m.Ctx)
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
	case "stopManagedLoop":
		if id, ok := msg.Data.(string); ok && id != "" && m.SessMgr != nil {
			if err := m.SessMgr.StopLoop(id); err != nil {
				m.Notify.Show(fmt.Sprintf("Stop error: %v", err), 3*time.Second)
			} else {
				m.Notify.Show(fmt.Sprintf("Stopped loop %s", truncateID(id)), 3*time.Second)
			}
			return m, m.loopListCmd()
		}
	}
	return m, nil
}

func (m Model) handleActionResult(msg components.ActionResultMsg) (tea.Model, tea.Cmd) {
	m.Modals.ActionMenu = nil
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
		m.Modals.ConfirmDialog = &components.ConfirmDialog{
			Title:   "Confirm Stop All",
			Message: "Stop all running loops and sessions?",
			Action:  "stopAll",
			Active:  true,
			Width:   50,
		}
	case "startLoop":
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			return m.startLoop(m.Sel.RepoIdx)
		}
	case "stopLoop":
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
	case "pauseLoop":
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			return m.togglePause(m.Sel.RepoIdx)
		}
	case "viewLogs":
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			m.LogOffset = 0
			m.LogView = views.NewLogView()
			m.LogView.SetDimensions(m.Width, m.Height)
			repoPath := m.Repos[m.Sel.RepoIdx].Path
			m.pushView(ViewLogs, "Logs")
			return m, loadLogCmd(repoPath)
		}
	case "editConfig":
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repo := m.Repos[m.Sel.RepoIdx]
			if repo.Config != nil {
				m.ConfigEdit = views.NewConfigEditor(repo.Config)
				m.ConfigEdit.Height = m.Height
				m.pushView(ViewConfigEditor, "Config")
			}
		}
	case "launchSession":
		if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			repo := m.Repos[m.Sel.RepoIdx]
			m.Modals.Launcher = components.NewSessionLauncher(repo.Path, repo.Name)
			m.Modals.Launcher.Width = m.Width
		}
	case "viewDiff":
		if m.Nav.CurrentView == ViewSessionDetail && m.Sel.SessionID != "" && m.SessMgr != nil {
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
		} else if m.Sel.RepoIdx >= 0 && m.Sel.RepoIdx < len(m.Repos) {
			m.pushView(ViewDiff, "Diff")
		}
	case "stopSession":
		if m.Sel.SessionID != "" && m.SessMgr != nil {
			m.Modals.ConfirmDialog = &components.ConfirmDialog{
				Title:   "Confirm Stop Session",
				Message: "Stop this session?",
				Action:  "stopSession",
				Data:    m.Sel.SessionID,
				Active:  true,
				Width:   50,
			}
		}
	case "retrySession":
		if m.Sel.SessionID != "" && m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
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
	m.Modals.Launcher = nil
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
	m.Sel.SessionID = s.ID
	m.Notify.Show(fmt.Sprintf("Launched session %s (%s)", id, msg.Provider), 3*time.Second)
	m.pushView(ViewSessionDetail, id)
	return m, nil
}

// --- Session output streaming ---

func (m Model) startOutputStreaming() (tea.Model, tea.Cmd) {
	if m.Sel.SessionID == "" || m.SessMgr == nil {
		return m, nil
	}
	m.Stream.SessionID = m.Sel.SessionID
	m.Stream.Active = true
	m.Stream.OutputView = views.NewLogView()
	m.Stream.OutputView.SetDimensions(m.Width, m.Height/2)

	// Pre-fill with output history
	if s, ok := m.SessMgr.Get(m.Sel.SessionID); ok {
		s.Lock()
		history := make([]string, len(s.OutputHistory))
		copy(history, s.OutputHistory)
		s.Unlock()
		if len(history) > 0 {
			m.Stream.OutputView.SetLines(history)
		}
	}

	return m, m.streamSessionOutput(m.Sel.SessionID)
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
