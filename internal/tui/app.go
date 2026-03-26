package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/discovery"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
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
	ViewDiff
	ViewTimeline
	ViewLoopHealth
	ViewLoopList
	ViewLoopDetail
	ViewLoopControl
)

// InputMode tracks the current input capture mode.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFilter
	ModeLauncher
)

type tickMsg time.Time

// LoopListMsg carries a refreshed snapshot of active loop runs for the loop list view.
type LoopListMsg []*session.LoopRun

// RefreshErrorMsg is sent when RefreshRepo encounters parse errors.
type RefreshErrorMsg struct {
	RepoPath string
	Errors   []error
}

// watcherBackoffMsg triggers a delayed re-watch after watcher failure.
type watcherBackoffMsg struct{}

// SessionOutputMsg carries a line of streamed session output.
type SessionOutputMsg struct {
	SessionID string
	Line      string
}

// SessionOutputDoneMsg signals streaming has ended.
type SessionOutputDoneMsg struct {
	SessionID string
}

// LoopStepResultMsg carries the result of a single StepLoop call.
type LoopStepResultMsg struct {
	LoopID string
	Err    error
}

// LoopToggleResultMsg carries the result of starting or stopping a loop.
type LoopToggleResultMsg struct {
	LoopID  string
	Started bool
	Err     error
}

// LoopPauseResultMsg carries the result of pausing or resuming a loop.
type LoopPauseResultMsg struct {
	LoopID string
	Paused bool
	Err    error
}

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
	Table         *components.Table
	SessionTable  *components.Table
	TeamTable     *components.Table
	LoopListTable *components.Table
	TabBar       components.TabBar
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
	SelectedLoop    string // loop ID for detail view
	FleetWindow     int
	FleetSection    int
	FleetCursor     int

	// Modal overlays
	ConfirmDialog *components.ConfirmDialog
	ActionMenu    *components.ActionMenu
	Launcher      *components.SessionLauncher

	// Session output streaming
	StreamingSessionID string
	StreamingOutput    bool
	SessionOutputView  *views.LogView

	// Animation
	TickFrame int

	// Event bus
	EventBus *events.Bus

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

	// Desktop notifications
	NotifyEnabled bool

	// Loop panel overlay
	ShowLoopPanel bool
	LoopView      string

	// Loop control panel
	LoopControlIdx  int
	LoopControlData []views.LoopControlData

	// Loop observation cache (refreshed less often than 2s tick)
	ObsCache     map[string][]session.LoopObservation // keyed by repo path
	ObsCacheTime time.Time
	GateCache    map[string]*GateCacheEntry // keyed by repo path
	GateCacheExp time.Time
	PrevGateVerdicts map[string]string // keyed by repo path, for change detection
}

// NewModel creates the root model.
func NewModel(scanPath string, sessMgr *session.Manager) Model {
	table := views.NewOverviewTable()
	sessionTable := views.NewSessionsTable()
	teamTable := views.NewTeamsTable()
	loopListTable := views.NewLoopListTable()

	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = styles.StatusRunning

	table.MultiSelect = true
	sessionTable.MultiSelect = true

	return Model{
		ScanPath:      scanPath,
		CurrentView:   ViewOverview,
		Table:         table,
		SessionTable:  sessionTable,
		TeamTable:     teamTable,
		LoopListTable: loopListTable,
		TabBar:       components.TabBar{Tabs: tabNames},
		LogView:      views.NewLogView(),
		ProcMgr:      process.NewManager(),
		SessMgr:      sessMgr,
		Breadcrumb:   components.Breadcrumb{Parts: []string{"Repos"}},
		Keys:         DefaultKeyMap(),
		Spinner:      s,
		FleetWindow:  1,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(
		m.scanRepos(),
		m.tickCmd(),
		m.Spinner.Tick,
		process.WaitForProcessExit(m.ProcMgr.ExitChan()),
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
		repos, err := discovery.Scan(context.TODO(), path)
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

// Navigation helpers

func (m *Model) pushView(v ViewMode, name string) {
	m.ViewStack = append(m.ViewStack, m.CurrentView)
	m.CurrentView = v
	m.Breadcrumb.Push(name)
	m.Keys.SetViewContext(v)
}

func (m Model) popView() (tea.Model, tea.Cmd) {
	if len(m.ViewStack) == 0 {
		// At root — no-op on Esc
		return m, nil
	}
	m.CurrentView = m.ViewStack[len(m.ViewStack)-1]
	m.ViewStack = m.ViewStack[:len(m.ViewStack)-1]
	m.Breadcrumb.Pop()
	m.Keys.SetViewContext(m.CurrentView)
	return m, nil
}

func (m *Model) refreshAllRepos() []tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.Repos {
		if errs := model.RefreshRepo(r); len(errs) > 0 {
			repoPath := r.Path
			errs := errs
			cmds = append(cmds, func() tea.Msg {
				return RefreshErrorMsg{RepoPath: repoPath, Errors: errs}
			})
		}
	}
	return cmds
}

func (m *Model) refreshLoopView() {
	if m.SessMgr == nil {
		m.LoopView = styles.InfoStyle.Render("  No active loops")
		return
	}
	loops := m.SessMgr.ListLoops()
	if len(loops) == 0 {
		m.LoopView = styles.InfoStyle.Render("  No active loops")
		return
	}
	var b strings.Builder
	for _, l := range loops {
		l.Lock()
		repoName := l.RepoName
		status := l.Status
		iterCount := len(l.Iterations)
		var lastTask string
		if iterCount > 0 {
			lastTask = l.Iterations[iterCount-1].Task.Title
		}
		l.Unlock()
		b.WriteString(fmt.Sprintf("  %s  %s  iters:%d",
			repoName,
			styles.StatusStyle(status).Render(status),
			iterCount,
		))
		if lastTask != "" {
			b.WriteString(fmt.Sprintf("  %s", lastTask))
		}
		b.WriteString("\n")
	}
	m.LoopView = b.String()
}

func (m *Model) refreshLoopControlData() {
	if m.SessMgr == nil {
		m.LoopControlData = nil
		return
	}
	m.LoopControlData = views.SnapshotLoopControl(m.SessMgr.ListLoops())
	if m.LoopControlIdx >= len(m.LoopControlData) {
		m.LoopControlIdx = max(0, len(m.LoopControlData)-1)
	}
}

func (m *Model) updateTable() {
	healthData := m.buildHealthData()
	rows := views.ReposToRows(m.Repos, m.TickFrame, healthData, m.Width)
	m.Table.SetRows(rows)
	m.StatusBar.RepoCount = len(m.Repos)
	m.StatusBar.RunningCount = len(m.ProcMgr.RunningPaths())
	m.StatusBar.LastRefresh = m.LastRefresh
	m.StatusBar.TickFrame = m.TickFrame

	// Update extended status bar fields
	if m.SessMgr != nil {
		sessions := m.SessMgr.List("")
		m.StatusBar.SessionCount = len(sessions)
		var totalSpend float64
		var totalBudget float64
		providerCounts := make(map[string]int)
		for _, s := range sessions {
			s.Lock()
			totalSpend += s.SpentUSD
			totalBudget += s.BudgetUSD
			if s.Status == session.StatusRunning || s.Status == session.StatusLaunching {
				providerCounts[string(s.Provider)]++
			}
			s.Unlock()
		}
		m.StatusBar.TotalSpendUSD = totalSpend
		m.StatusBar.ProviderCounts = providerCounts
		if totalBudget > 0 {
			m.StatusBar.FleetBudgetPct = totalSpend / totalBudget
		} else {
			m.StatusBar.FleetBudgetPct = 0
		}
		m.StatusBar.AlertCount = m.countAlerts()
		// Determine highest alert severity
		m.StatusBar.HighestAlertSeverity = ""
		if m.StatusBar.AlertCount > 0 {
			m.StatusBar.HighestAlertSeverity = "info"
			for _, r := range m.Repos {
				if r.Circuit != nil && r.Circuit.State == "OPEN" {
					m.StatusBar.HighestAlertSeverity = "critical"
					break
				}
			}
		}
	}
}

func (m *Model) updateSessionTable() {
	if m.SessMgr == nil {
		return
	}
	sessions := m.SessMgr.List("")
	rows := views.SessionsToRows(sessions, m.TickFrame)
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

// loopListCmd fetches active loops and returns them as a LoopListMsg.
func (m Model) loopListCmd() tea.Cmd {
	if m.SessMgr == nil {
		return func() tea.Msg { return LoopListMsg(nil) }
	}
	loops := m.SessMgr.ListLoops()
	return func() tea.Msg { return LoopListMsg(loops) }
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
	case ViewLoopList:
		return m.LoopListTable
	default:
		return m.Table
	}
}

// switchTab changes the active tab, clearing the view stack.
func (m *Model) switchTab(tab int, view ViewMode, name string) {
	m.ActiveTab = tab
	m.TabBar.Active = tab
	m.CurrentView = view
	m.ViewStack = nil
	m.Breadcrumb = components.Breadcrumb{Parts: []string{name}}
	m.Filter.Clear()
	m.Keys.SetViewContext(view)
}

func (m Model) findRepoByName(name string) int {
	for i, r := range m.Repos {
		if r.Name == name || filepath.Base(r.Path) == name {
			return i
		}
	}
	return -1
}

func (m Model) findRepoByPath(path string) int {
	for i, r := range m.Repos {
		if r.Path == path {
			return i
		}
	}
	return -1
}

// tabNames for the tab bar.
var tabNames = []string{
	fmt.Sprintf("1:%s Repos", styles.IconRepo),
	fmt.Sprintf("2:%s Sessions", styles.IconSession),
	fmt.Sprintf("3:%s Teams", styles.IconTeam),
	fmt.Sprintf("4:%s Fleet", styles.IconFleet),
}

// View renders the TUI.
func (m Model) View() string {
	var b strings.Builder

	// Title bar
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" %s ralphglasses ", styles.IconGlasses)))
	b.WriteString("  ")
	b.WriteString(m.Breadcrumb.View())
	b.WriteString("\n")

	// Tab bar
	b.WriteString(m.TabBar.View())
	b.WriteString("\n\n")

	// Main content
	switch m.CurrentView {
	case ViewOverview:
		b.WriteString(m.Table.View())
	case ViewRepoDetail:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
			var detailHealth *views.RepoDetailHealth
			if entry := m.getGateEntry(repo.Path); entry != nil {
				detailHealth = &views.RepoDetailHealth{
					Observations: m.getObservations(repo.Path),
					GateReport:   entry.Report,
				}
				if m.SessMgr != nil {
					detailHealth.ProviderProfiles = m.SessMgr.ProviderProfiles()
				}
			}
			b.WriteString(views.RenderRepoDetail(repo, m.Width, detailHealth))
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
	case ViewDiff:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			b.WriteString(views.RenderDiffView(m.Repos[m.SelectedIdx].Path, "", m.Width, m.Height))
		}
	case ViewTimeline:
		entries := m.buildTimelineEntries()
		repoName := "All Sessions"
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repoName = m.Repos[m.SelectedIdx].Name
		}
		b.WriteString(views.RenderTimeline(entries, repoName, m.Width, m.Height))
	case ViewLoopHealth:
		if m.SelectedIdx >= 0 && m.SelectedIdx < len(m.Repos) {
			repo := m.Repos[m.SelectedIdx]
			healthData := views.LoopHealthData{
				RepoName:     repo.Name,
				Observations: m.getObservations(repo.Path),
			}
			if entry := m.getGateEntry(repo.Path); entry != nil {
				healthData.GateReport = entry.Report
				healthData.Summary = entry.Summary
			}
			b.WriteString(views.RenderLoopHealth(healthData, m.Width, m.Height))
		}
	case ViewLoopList:
		b.WriteString(m.LoopListTable.View())
		b.WriteString("\n")
		b.WriteString(styles.HelpStyle.Render("  s start loop  x/d stop loop  p pause/resume  Enter detail  j/k navigate  Esc back"))
	case ViewLoopDetail:
		if m.SessMgr != nil && m.SelectedLoop != "" {
			if l, ok := m.SessMgr.GetLoop(m.SelectedLoop); ok {
				b.WriteString(views.RenderLoopDetail(l, m.Width, m.Height))
			} else {
				b.WriteString(styles.InfoStyle.Render("  Loop not found"))
			}
		}
	case ViewLoopControl:
		b.WriteString(views.RenderLoopControlPanel(m.LoopControlData, m.LoopControlIdx, m.Width, m.Height))
	}

	// Loop panel overlay
	if m.ShowLoopPanel {
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Loop Status "))
		b.WriteString("\n")
		b.WriteString(m.LoopView)
	}

	// Modal overlays
	if m.ConfirmDialog != nil && m.ConfirmDialog.Active {
		b.WriteString("\n")
		b.WriteString(m.ConfirmDialog.View())
	}
	if m.ActionMenu != nil && m.ActionMenu.Active {
		b.WriteString("\n")
		b.WriteString(m.ActionMenu.View())
	}
	if m.Launcher != nil && m.Launcher.Active {
		b.WriteString("\n")
		b.WriteString(m.Launcher.View())
	}

	// Notification overlay
	if notif := m.Notify.View(); notif != "" {
		b.WriteString("\n")
		b.WriteString(notif)
	}

	// Session output streaming view (split pane)
	if m.StreamingOutput && m.SessionOutputView != nil {
		b.WriteString("\n")
		b.WriteString(styles.TitleStyle.Render(" Live Output "))
		b.WriteString("\n")
		b.WriteString(m.SessionOutputView.View())
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
