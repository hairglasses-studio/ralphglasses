package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	tea "charm.land/bubbletea/v2"

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
	ViewObservation
	ViewEventLog
	ViewRDCycle
	ViewTeamOrchestration
)

// InputMode tracks the current input capture mode.
type InputMode int

const (
	ModeNormal InputMode = iota
	ModeCommand
	ModeFilter
	ModeLauncher
	ModeSearch
)

// NavigationState tracks view stack and tab state.
type NavigationState struct {
	CurrentView ViewMode
	ViewStack   []ViewMode
	Breadcrumb  components.Breadcrumb
	ActiveTab   int
}

// SelectionState tracks the currently selected items across views.
type SelectionState struct {
	RepoIdx   int    // index into Repos slice
	SessionID string // selected session ID
	TeamName  string // selected team name
	LoopID    string // selected loop ID
}

// ModalState tracks active modal overlays.
type ModalState struct {
	ConfirmDialog *components.ConfirmDialog
	ActionMenu    *components.ActionMenu
	Launcher      *components.SessionLauncher
}

// StreamState tracks session output streaming.
type StreamState struct {
	SessionID  string
	Active     bool
	OutputView *views.LogView
}

// CacheState holds TTL-gated cached data.
type CacheState struct {
	Obs              map[string][]session.LoopObservation
	ObsTime          time.Time
	Gate             map[string]*GateCacheEntry
	GateExp          time.Time
	PrevGateVerdicts map[string]string
	OllamaInventory  *session.OllamaInventory
	OllamaInvTime    time.Time
}

// FleetNavState tracks fleet dashboard navigation.
type FleetNavState struct {
	Window  int
	Section int
	Cursor  int
}

// Model is the root Bubble Tea model.
type Model struct {
	Nav    NavigationState
	Sel    SelectionState
	Modals ModalState
	Stream StreamState
	Cache  CacheState
	Fleet  FleetNavState

	// Config
	ScanPath string

	// Data
	Repos []*model.Repo

	// Components
	Table                 *components.Table
	SessionTable          *components.Table
	TeamTable             *components.Table
	LoopListTable         *components.Table
	TabBar                components.TabBar
	StatusBar             components.StatusBar
	Notify                components.NotificationManager
	LogView               *views.LogView
	ConfigEdit            *views.ConfigEditor
	HelpView              *views.HelpView
	RepoDetailView        *views.RepoDetailView
	LoopHealthView        *views.LoopHealthView
	SessionDetailView     *views.SessionDetailView
	TeamDetailView        *views.TeamDetailView
	FleetView             *views.FleetView
	DiffViewport          *views.DiffViewport
	TimelineViewport      *views.TimelineViewport
	LoopDetailView        *views.LoopDetailView
	LoopControlView       *views.LoopControlView
	ObservationViewport   *views.ObservationViewport
	RDCycleView           *views.RDCycleView
	TeamOrchestrationView *views.TeamOrchestrationView

	// Bubbles components
	Keys    KeyMap
	Spinner spinner.Model

	// Session management
	SessMgr *session.Manager

	// Animation
	TickFrame int

	// Event bus
	EventBus   *events.Bus
	EventBusCh <-chan events.Event

	// State
	Width       int
	Height      int
	InputMode   InputMode
	CommandBuf  string
	Filter      FilterState
	LogOffset   int64
	LastRefresh time.Time

	// Process management
	Ctx     context.Context
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

	// Event log view
	EventLog *views.EventLogView

	// Global search
	SearchInput *components.SearchInput
	SearchView  *views.SearchView

	// View registry for incremental switch-to-dispatch migration
	viewRegistry *views.Registry

	// App start time for uptime tracking
	StartedAt time.Time
}

type tickMsg time.Time

// slowTickMsg is sent by the slow 30-second heartbeat ticker that forces a
// full table refresh regardless of which view is active.
type slowTickMsg time.Time

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

// LogLoadedMsg is returned when async log loading completes.
type LogLoadedMsg struct {
	Lines []string
	Err   error
}

type scanResultMsg struct {
	repos []*model.Repo
	err   error
}

// loadLogCmd returns a tea.Cmd that reads the full log asynchronously.
func loadLogCmd(repoPath string) tea.Cmd {
	return func() tea.Msg {
		lines, err := process.ReadFullLog(repoPath)
		return LogLoadedMsg{Lines: lines, Err: err}
	}
}

// Init returns the initial set of commands: repo scan, tick timer, slow tick timer, spinner, and process exit watcher.
func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{
		m.scanRepos(),
		m.tickCmd(),
		m.slowTickCmd(),
		m.Spinner.Tick,
		process.WaitForProcessExit(m.ProcMgr.ExitChan()),
	}
	if m.EventBusCh != nil {
		cmds = append(cmds, watchEventBus(m.EventBusCh))
	}
	return tea.Batch(cmds...)
}

// watchEventBus listens for events on the given channel and returns them as tea.Cmd.
func watchEventBus(ch <-chan events.Event) tea.Cmd {
	return func() tea.Msg {
		if ch == nil {
			return nil
		}
		e, ok := <-ch
		if !ok {
			return nil
		}
		return EventBusMsg(e)
	}
}

// tickCmd returns a fast 5-second heartbeat tick for drift correction.
func (m Model) tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

// slowTickCmd returns a slow 30-second tick that forces a full table refresh
// regardless of active view, preventing stale data in background views.
func (m Model) slowTickCmd() tea.Cmd {
	return tea.Tick(30*time.Second, func(t time.Time) tea.Msg {
		return slowTickMsg(t)
	})
}

func (m Model) scanRepos() tea.Cmd {
	path := m.ScanPath
	if path == "" {
		return nil
	}
	return func() tea.Msg {
		repos, err := discovery.Scan(context.Background(), path)
		if err != nil {
			return scanResultMsg{err: err}
		}
		return scanResultMsg{repos: repos}
	}
}

// Navigation helpers

func (m *Model) pushView(v ViewMode, name string) {
	m.Nav.ViewStack = append(m.Nav.ViewStack, m.Nav.CurrentView)
	m.Nav.CurrentView = v
	m.Nav.Breadcrumb.Push(name)
	m.Keys.SetViewContext(v)
}

func (m Model) popView() (tea.Model, tea.Cmd) {
	if len(m.Nav.ViewStack) == 0 {
		// At root — no-op on Esc
		return m, nil
	}
	m.Nav.CurrentView = m.Nav.ViewStack[len(m.Nav.ViewStack)-1]
	m.Nav.ViewStack = m.Nav.ViewStack[:len(m.Nav.ViewStack)-1]
	m.Nav.Breadcrumb.Pop()
	m.Keys.SetViewContext(m.Nav.CurrentView)
	return m, nil
}

func (m *Model) refreshAllRepos() []tea.Cmd {
	var cmds []tea.Cmd
	for _, r := range m.Repos {
		if errs := model.RefreshRepo(m.Ctx, r); len(errs) > 0 {
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
	m.refreshStatusBarCounts()
}

func (m *Model) refreshStatusBarCounts() {
	m.StatusBar.RepoCount = len(m.Repos)
	m.StatusBar.RunningCount = len(m.ProcMgr.RunningPaths())
	m.StatusBar.LastRefresh = m.LastRefresh
	m.StatusBar.TickFrame = m.TickFrame

	m.refreshSessionStatusBar()
	m.refreshCostStatusBar()
}

func (m *Model) refreshSessionStatusBar() {
	if m.SessMgr == nil {
		return
	}

	sessions := m.SessMgr.List("")
	m.StatusBar.SessionCount = len(sessions)

	providerCounts := make(map[string]int)
	for _, s := range sessions {
		s.Lock()
		if s.Status == session.StatusRunning || s.Status == session.StatusLaunching {
			providerCounts[string(s.Provider)]++
		}
		s.Unlock()
	}
	m.StatusBar.ProviderCounts = providerCounts

	m.StatusBar.AlertCount = m.countAlerts()
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

	loops := m.SessMgr.ListLoops()
	var activeLoops, totalIters, totalSuccess int
	var loopIterHistory []float64
	for _, l := range loops {
		l.Lock()
		if l.Status == "running" && !l.Paused {
			activeLoops++
		}
		for _, iter := range l.Iterations {
			totalIters++
			if iter.Status == "completed" || iter.Status == "verified" {
				totalSuccess++
			}
			if iter.EndedAt != nil {
				loopIterHistory = append(loopIterHistory, iter.EndedAt.Sub(iter.StartedAt).Seconds())
			}
		}
		l.Unlock()
	}
	m.StatusBar.ActiveLoopCount = activeLoops
	m.StatusBar.LoopIterTotal = totalIters
	if totalIters > 0 {
		m.StatusBar.LoopSuccessRate = float64(totalSuccess) / float64(totalIters)
	} else {
		m.StatusBar.LoopSuccessRate = 0
	}
	if len(loopIterHistory) > 20 {
		loopIterHistory = loopIterHistory[len(loopIterHistory)-20:]
	}
	m.StatusBar.LoopIterHistory = loopIterHistory

	healthMap := make(map[string]bool)
	for _, p := range []session.Provider{session.ProviderCodex, session.ProviderGemini, session.ProviderClaude} {
		h := session.CheckProviderHealth(p)
		healthMap[string(p)] = h.Healthy()
	}
	m.StatusBar.ProviderHealthy = healthMap

	m.StatusBar.AutonomyLevel = m.SessMgr.GetAutonomyLevel().String()
	m.StatusBar.Uptime = time.Since(m.StartedAt)
}

func (m *Model) refreshCostStatusBar() {
	if m.SessMgr == nil {
		return
	}

	sessions := m.SessMgr.List("")
	var totalSpend float64
	var totalBudget float64
	for _, s := range sessions {
		s.Lock()
		totalSpend += s.SpentUSD
		totalBudget += s.BudgetUSD
		s.Unlock()
	}
	m.StatusBar.TotalSpendUSD = totalSpend
	if totalBudget > 0 {
		m.StatusBar.FleetBudgetPct = totalSpend / totalBudget
	} else {
		m.StatusBar.FleetBudgetPct = 0
	}

	m.StatusBar.CostHistory = nil
	var earliestLaunch time.Time
	for _, s := range sessions {
		s.Lock()
		if earliestLaunch.IsZero() || (!s.LaunchedAt.IsZero() && s.LaunchedAt.Before(earliestLaunch)) {
			earliestLaunch = s.LaunchedAt
		}
		m.StatusBar.CostHistory = append(m.StatusBar.CostHistory, s.CostHistory...)
		s.Unlock()
	}
	if !earliestLaunch.IsZero() {
		if mins := time.Since(earliestLaunch).Minutes(); mins > 0 {
			m.StatusBar.CostVelocity = totalSpend / mins
		}
	}
	if len(m.StatusBar.CostHistory) > 20 {
		m.StatusBar.CostHistory = m.StatusBar.CostHistory[len(m.StatusBar.CostHistory)-20:]
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

// needsRepoTable reports whether the current view renders the repo overview table.
func (m *Model) needsRepoTable() bool {
	switch m.Nav.CurrentView {
	case ViewOverview, ViewRepoDetail, ViewLoopHealth, ViewDiff, ViewTimeline, ViewObservation:
		return true
	}
	return false
}

// needsSessionTable reports whether the current view renders the session table.
func (m *Model) needsSessionTable() bool {
	switch m.Nav.CurrentView {
	case ViewSessions, ViewSessionDetail, ViewFleet:
		return true
	}
	return false
}

// needsTeamTable reports whether the current view renders the team table.
func (m *Model) needsTeamTable() bool {
	switch m.Nav.CurrentView {
	case ViewTeams, ViewTeamDetail, ViewTeamOrchestration, ViewFleet:
		return true
	}
	return false
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
	switch m.Nav.CurrentView {
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
	m.Nav.ActiveTab = tab
	m.TabBar.Active = tab
	m.Nav.CurrentView = view
	m.Nav.ViewStack = nil
	m.Nav.Breadcrumb = components.Breadcrumb{Parts: []string{name}}
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
