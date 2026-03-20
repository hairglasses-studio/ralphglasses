package tui

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// View modes (navigation stack).
type ViewMode int

const (
	ViewOverview ViewMode = iota
	ViewRepoDetail
	ViewLogs
	ViewConfigEditor
	ViewHelp
	ViewSessions
	ViewSessionDetail
	ViewTeams
	ViewTeamDetail
	ViewFleet
)

// InputMode tracks the current input capture mode.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFilter
)

type tickMsg time.Time

// RefreshErrorMsg is sent when RefreshRepo encounters parse errors.
type RefreshErrorMsg struct {
	RepoPath string
	Errors   []error
}

// watcherBackoffMsg triggers a delayed re-watch after watcher failure.
type watcherBackoffMsg struct{}

// Model is the root Bubble Tea model.
type Model struct {
	// Config
	ScanPath string

	// Data
	Repos []*model.Repo

	// Navigation
	CurrentView ViewMode
	ViewStack   []ViewMode
	Breadcrumb  components.Breadcrumb
	ActiveTab   int // 0=repos, 1=sessions, 2=teams, 3=fleet

	// Components
	Table        *components.Table
	SessionTable *components.Table
	TeamTable    *components.Table
	StatusBar    components.StatusBar
	Notify       components.NotificationManager
	LogView      *views.LogView
	ConfigEdit   *views.ConfigEditor

	// Bubbles components
	Keys    KeyMap
	Spinner spinner.Model

	// Session management
	SessMgr         *session.Manager
	SelectedSession string // session ID for detail view
	SelectedTeam    string // team name for detail view

	// State
	Width       int
	Height      int
	InputMode   InputMode
	CommandBuf  string
	Filter      FilterState
	SelectedIdx int // index into Repos for detail/log views
	LogOffset   int64
	LastRefresh time.Time

	// Process management
	ProcMgr *process.Manager

	// Watcher state
	WatcherFails    int  // consecutive watcher failure count for backoff
	WatcherDisabled bool // true when fallen back to polling-only mode
}

// NewModel creates the root model.
func NewModel(scanPath string, sessMgr *session.Manager) Model {
	table := views.NewOverviewTable()
	sessionTable := views.NewSessionsTable()
	teamTable := views.NewTeamsTable()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.StatusRunning

	return Model{
		ScanPath:     scanPath,
		CurrentView:  ViewOverview,
		Table:        table,
		SessionTable: sessionTable,
		TeamTable:    teamTable,
		LogView:      views.NewLogView(),
		ProcMgr:      process.NewManager(),
		SessMgr:      sessMgr,
		Breadcrumb:   components.Breadcrumb{Parts: []string{"Repos"}},
		Keys:         DefaultKeyMap(),
		Spinner:      s,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.scanRepos(),
		m.tickCmd(),
		m.Spinner.Tick,
	)
}

func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m Model) scanRepos() tea.Cmd {
	path := m.ScanPath
	return func() tea.Msg {
		repos, err := discovery.Scan(path)
		if err != nil {
			return scanResultMsg{err: err}
		}
		return scanResultMsg{repos: repos}
	}
}

type scanResultMsg struct {
	repos []*model.Repo
	err   error
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Table.Width = msg.Width
		m.Table.Height = msg.Height
		m.SessionTable.Width = msg.Width
		m.SessionTable.Height = msg.Height
		m.TeamTable.Width = msg.Width
		m.TeamTable.Height = msg.Height
		m.LogView.SetDimensions(msg.Width, msg.Height)
		m.StatusBar.Width = msg.Width
		return m, nil

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.Spinner, cmd = m.Spinner.Update(msg)
		return m, cmd

	case tickMsg:
		m.refreshAllRepos()
		m.updateTable()
		m.updateSessionTable()
		m.updateTeamTable()
		m.LastRefresh = time.Now()
		var cmds []tea.Cmd
		cmds = append(cmds, m.tickCmd())
		// If viewing logs, tail the log
		if m.CurrentView == ViewLogs && m.SelectedIdx < len(m.Repos) {
			cmds = append(cmds, process.TailLog(m.Repos[m.SelectedIdx].Path, &m.LogOffset))
		}
		return m, tea.Batch(cmds...)

	case RefreshErrorMsg:
		m.Notify.Show(fmt.Sprintf("⚠ %s: parse errors", filepath.Base(msg.RepoPath)), 5*time.Second)
		return m, nil

	case scanResultMsg:
		if msg.err != nil {
			m.Notify.Show(fmt.Sprintf("Scan error: %v", msg.err), 5*time.Second)
			return m, nil
		}
		m.Repos = msg.repos
		m.updateTable()
		m.LastRefresh = time.Now()
		// Start watching status files
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		return m, process.WatchStatusFiles(paths)

	case process.WatcherErrorMsg:
		m.WatcherFails++
		if m.WatcherFails >= 5 && !m.WatcherDisabled {
			// Too many consecutive failures — fall back to polling only
			m.WatcherDisabled = true
			m.Notify.Show("⚠ Watcher failed repeatedly, using polling mode", 5*time.Second)
			return m, nil
		}
		m.Notify.Show(fmt.Sprintf("⚠ Watcher error: %v", msg.Err), 4*time.Second)
		// Exponential backoff: 1s, 2s, 4s, 8s, 16s, capped at 30s
		delay := time.Duration(1<<uint(m.WatcherFails-1)) * time.Second
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}
		return m, tea.Tick(delay, func(time.Time) tea.Msg {
			return watcherBackoffMsg{}
		})

	case watcherBackoffMsg:
		if m.WatcherDisabled {
			return m, nil
		}
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		return m, process.WatchStatusFiles(paths)

	case process.FileChangedMsg:
		// Successful watch — reset failure counter
		m.WatcherFails = 0
		// Reactive update for a single repo
		var cmds []tea.Cmd
		for _, r := range m.Repos {
			if r.Path == msg.RepoPath {
				if errs := model.RefreshRepo(r); len(errs) > 0 {
					repoPath := r.Path
					errs := errs
					cmds = append(cmds, func() tea.Msg {
						return RefreshErrorMsg{RepoPath: repoPath, Errors: errs}
					})
				}
				break
			}
		}
		m.updateTable()
		// Re-watch
		paths := make([]string, len(m.Repos))
		for i, r := range m.Repos {
			paths[i] = r.Path
		}
		cmds = append(cmds, process.WatchStatusFiles(paths))
		return m, tea.Batch(cmds...)

	case process.LogLinesMsg:
		if len(msg.Lines) > 0 {
			m.LogView.AppendLines(msg.Lines)
		}
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Command mode input
	if m.InputMode == ModeCommand {
		return m.handleCommandInput(msg)
	}

	// Filter mode input
	if m.InputMode == ModeFilter {
		return m.handleFilterInput(msg)
	}

	// Config editor in edit mode
	if m.CurrentView == ViewConfigEditor && m.ConfigEdit != nil && m.ConfigEdit.Editing {
		return m.handleConfigEditInput(msg)
	}

	// Global keys
	switch {
	case key.Matches(msg, m.Keys.Quit):
		m.ProcMgr.StopAll()
		if m.SessMgr != nil {
			m.SessMgr.StopAll()
		}
		return m, tea.Quit
	case key.Matches(msg, m.Keys.CmdMode):
		m.InputMode = ModeCommand
		m.CommandBuf = ""
		return m, nil
	case key.Matches(msg, m.Keys.FilterMode):
		m.InputMode = ModeFilter
		m.Filter.Active = true
		m.Filter.Text = ""
		return m, nil
	case key.Matches(msg, m.Keys.Help):
		if m.CurrentView == ViewHelp {
			return m.popView()
		}
		m.pushView(ViewHelp, "Help")
		return m, nil
	case key.Matches(msg, m.Keys.Escape):
		return m.popView()
	case key.Matches(msg, m.Keys.Refresh):
		return m, m.scanRepos()
	// Tab switching
	case key.Matches(msg, m.Keys.Tab1):
		m.switchTab(0, ViewOverview, "Repos")
		return m, nil
	case key.Matches(msg, m.Keys.Tab2):
		m.switchTab(1, ViewSessions, "Sessions")
		return m, nil
	case key.Matches(msg, m.Keys.Tab3):
		m.switchTab(2, ViewTeams, "Teams")
		return m, nil
	case key.Matches(msg, m.Keys.Tab4):
		m.switchTab(3, ViewFleet, "Fleet")
		return m, nil
	}

	// View-specific keys
	switch m.CurrentView {
	case ViewOverview:
		return m.handleOverviewKey(msg)
	case ViewRepoDetail:
		return m.handleDetailKey(msg)
	case ViewLogs:
		return m.handleLogKey(msg)
	case ViewConfigEditor:
		return m.handleConfigKey(msg)
	case ViewSessions:
		return m.handleSessionsKey(msg)
	case ViewSessionDetail:
		return m.handleSessionDetailKey(msg)
	case ViewTeams:
		return m.handleTeamsKey(msg)
	case ViewTeamDetail:
		return m.handleTeamDetailKey(msg)
	}

	return m, nil
}

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

// Process management helpers

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

// Navigation helpers

func (m *Model) pushView(v ViewMode, name string) {
	m.ViewStack = append(m.ViewStack, m.CurrentView)
	m.CurrentView = v
	m.Breadcrumb.Push(name)
}

func (m Model) popView() (tea.Model, tea.Cmd) {
	if len(m.ViewStack) == 0 {
		// At root — no-op on Esc
		return m, nil
	}
	m.CurrentView = m.ViewStack[len(m.ViewStack)-1]
	m.ViewStack = m.ViewStack[:len(m.ViewStack)-1]
	m.Breadcrumb.Pop()
	return m, nil
}

func (m *Model) refreshAllRepos() {
	for _, r := range m.Repos {
		model.RefreshRepo(r)
	}
}

func (m *Model) updateTable() {
	rows := views.ReposToRows(m.Repos)
	m.Table.SetRows(rows)
	m.StatusBar.RepoCount = len(m.Repos)
	m.StatusBar.RunningCount = len(m.ProcMgr.RunningPaths())
	m.StatusBar.LastRefresh = m.LastRefresh

	// Update extended status bar fields
	if m.SessMgr != nil {
		sessions := m.SessMgr.List("")
		m.StatusBar.SessionCount = len(sessions)
		var totalSpend float64
		for _, s := range sessions {
			s.Lock()
			totalSpend += s.SpentUSD
			s.Unlock()
		}
		m.StatusBar.TotalSpendUSD = totalSpend
		m.StatusBar.AlertCount = m.countAlerts()
	}
}

func (m *Model) updateSessionTable() {
	if m.SessMgr == nil {
		return
	}
	sessions := m.SessMgr.List("")
	rows := views.SessionsToRows(sessions)
	m.SessionTable.SetRows(rows)
}

func (m *Model) updateTeamTable() {
	if m.SessMgr == nil {
		return
	}
	teams := m.SessMgr.ListTeams()
	rows := views.TeamsToRows(teams)
	m.TeamTable.SetRows(rows)
}

func (m *Model) countAlerts() int {
	count := 0
	for _, r := range m.Repos {
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			count++
		}
	}
	if m.SessMgr != nil {
		for _, s := range m.SessMgr.List("") {
			s.Lock()
			if s.Status == session.StatusErrored {
				count++
			}
			s.Unlock()
		}
	}
	return count
}

// activeTable returns the table for the current view.
func (m *Model) activeTable() *components.Table {
	switch m.CurrentView {
	case ViewOverview:
		return m.Table
	case ViewSessions:
		return m.SessionTable
	case ViewTeams:
		return m.TeamTable
	default:
		return m.Table
	}
}

// switchTab changes the active tab, clearing the view stack.
func (m *Model) switchTab(tab int, view ViewMode, name string) {
	m.ActiveTab = tab
	m.CurrentView = view
	m.ViewStack = nil
	m.Breadcrumb = components.Breadcrumb{Parts: []string{name}}
	m.Filter.Clear()
}

// handleSessionsKey handles keys in the sessions table view.
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
	}
	return m, nil
}

// handleSessionDetailKey handles keys in the session detail view.
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
	}
	return m, nil
}

// handleTeamsKey handles keys in the teams table view.
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

// handleTeamDetailKey handles keys in the team detail view.
func (m Model) handleTeamDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// No actions besides Esc (handled globally)
	return m, nil
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

// buildFleetData aggregates data for the fleet dashboard.
func (m *Model) buildFleetData() views.FleetData {
	data := views.FleetData{
		TotalRepos: len(m.Repos),
		Providers:  make(map[string]views.ProviderStat),
	}

	// Repo stats
	for _, r := range m.Repos {
		status := r.StatusDisplay()
		switch status {
		case "running":
			data.RunningLoops++
		case "paused":
			data.PausedLoops++
		}
		if r.Circuit != nil && r.Circuit.State == "OPEN" {
			data.OpenCircuits++
			data.Alerts = append(data.Alerts, views.FleetAlert{
				Severity: "critical",
				Message:  fmt.Sprintf("Circuit breaker OPEN: %s — %s", r.Name, r.Circuit.Reason),
			})
		}
		if r.Status != nil {
			// Stale check (>1h since update)
			if !r.Status.Timestamp.IsZero() && time.Since(r.Status.Timestamp) > time.Hour && r.StatusDisplay() == "running" {
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Stale status (>1h): %s", r.Name),
				})
			}
			// Budget check (>=90%)
			if r.Status.BudgetStatus == "exceeded" || (r.Status.SessionSpendUSD > 0 && r.Status.BudgetStatus != "" && r.Status.BudgetStatus != "ok") {
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Budget concern: %s — %s", r.Name, r.Status.BudgetStatus),
				})
			}
		}
		if r.Circuit != nil && r.Circuit.ConsecutiveNoProgress >= 3 {
			data.Alerts = append(data.Alerts, views.FleetAlert{
				Severity: "warning",
				Message:  fmt.Sprintf("No progress (%dx): %s", r.Circuit.ConsecutiveNoProgress, r.Name),
			})
		}
	}
	data.Repos = m.Repos

	// Session stats
	if m.SessMgr != nil {
		sessions := m.SessMgr.List("")
		data.Sessions = sessions
		data.TotalSessions = len(sessions)
		for _, s := range sessions {
			s.Lock()
			provider := string(s.Provider)
			status := s.Status
			spent := s.SpentUSD
			budget := s.BudgetUSD
			repoName := s.RepoName
			id := s.ID
			s.Unlock()

			if status == session.StatusRunning || status == session.StatusLaunching {
				data.RunningSessions++
			}
			data.TotalSpendUSD += spent

			ps := data.Providers[provider]
			ps.Sessions++
			if status == session.StatusRunning || status == session.StatusLaunching {
				ps.Running++
			}
			ps.SpendUSD += spent
			data.Providers[provider] = ps

			// Session alerts
			if status == session.StatusErrored {
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "info",
					Message:  fmt.Sprintf("Session errored: %s (%s)", shortID, repoName),
				})
			}
			if budget > 0 && spent/budget >= 0.90 {
				shortID := id
				if len(shortID) > 8 {
					shortID = shortID[:8]
				}
				data.Alerts = append(data.Alerts, views.FleetAlert{
					Severity: "warning",
					Message:  fmt.Sprintf("Session budget ≥90%%: %s ($%.2f/$%.2f)", shortID, spent, budget),
				})
			}
		}
	}

	return data
}

func (m Model) findRepoByName(name string) int {
	for i, r := range m.Repos {
		if r.Name == name || filepath.Base(r.Path) == name {
			return i
		}
	}
	return -1
}

// tabNames for the tab bar.
var tabNames = []string{"1:Repos", "2:Sessions", "3:Teams", "4:Fleet"}

// View renders the TUI.
func (m Model) View() string {
	var b strings.Builder

	// Title bar
	b.WriteString(styles.TitleStyle.Render(" 👓 ralphglasses "))
	b.WriteString("  ")
	b.WriteString(m.Breadcrumb.View())
	b.WriteString("\n")

	// Tab bar
	var tabs []string
	for i, name := range tabNames {
		if i == m.ActiveTab {
			tabs = append(tabs, styles.TabActive.Render(name))
		} else {
			tabs = append(tabs, styles.TabInactive.Render(name))
		}
	}
	b.WriteString(strings.Join(tabs, " "))
	b.WriteString("\n\n")

	// Main content
	switch m.CurrentView {
	case ViewOverview:
		b.WriteString(m.Table.View())
	case ViewRepoDetail:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			b.WriteString(views.RenderRepoDetail(m.Repos[m.SelectedIdx], m.Width))
		}
	case ViewLogs:
		b.WriteString(m.LogView.View())
	case ViewConfigEditor:
		if m.ConfigEdit != nil {
			b.WriteString(m.ConfigEdit.View())
		}
	case ViewHelp:
		b.WriteString(views.RenderHelp(m.Keys.HelpGroups(), m.Width, m.Height))
	case ViewSessions:
		b.WriteString(m.SessionTable.View())
	case ViewSessionDetail:
		if m.SessMgr != nil {
			if s, ok := m.SessMgr.Get(m.SelectedSession); ok {
				b.WriteString(views.RenderSessionDetail(s, m.Width, m.Height))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Session not found"))
			}
		}
	case ViewTeams:
		b.WriteString(m.TeamTable.View())
	case ViewTeamDetail:
		if m.SessMgr != nil {
			if team, ok := m.SessMgr.GetTeam(m.SelectedTeam); ok {
				leadSession, _ := m.SessMgr.Get(team.LeadID)
				b.WriteString(views.RenderTeamDetail(team, leadSession, m.Width))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Team not found"))
			}
		}
	case ViewFleet:
		data := m.buildFleetData()
		b.WriteString(views.RenderFleetDashboard(data, m.Width, m.Height))
	}

	// Notification overlay
	if notif := m.Notify.View(); notif != "" {
		b.WriteString("\n")
		b.WriteString(notif)
	}

	b.WriteString("\n")

	// Input line
	switch m.InputMode {
	case ModeCommand:
		b.WriteString(styles.CommandStyle.Render(":"))
		b.WriteString(m.CommandBuf)
		b.WriteString(styles.CommandStyle.Render("█"))
	case ModeFilter:
		b.WriteString(styles.CommandStyle.Render("/"))
		b.WriteString(m.Filter.Text)
		b.WriteString(styles.CommandStyle.Render("█"))
	default:
		// Mode indicator in status bar
		m.StatusBar.Mode = "NORMAL"
		m.StatusBar.Filter = m.Filter.Text
		m.StatusBar.SpinnerFrame = m.Spinner.View()
		b.WriteString(m.StatusBar.View())
	}

	return b.String()
}
