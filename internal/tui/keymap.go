package tui

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// KeyMap holds all key bindings for the application.
type KeyMap struct {
	// Global
	Quit       key.Binding
	CmdMode    key.Binding
	FilterMode key.Binding
	Help       key.Binding
	Escape     key.Binding
	Refresh    key.Binding

	// Tabs
	Tab1 key.Binding
	Tab2 key.Binding
	Tab3 key.Binding
	Tab4 key.Binding

	// Table navigation
	Down  key.Binding
	Up    key.Binding
	Enter key.Binding
	Sort  key.Binding

	// Repo/Session actions
	StartLoop  key.Binding
	StopAction key.Binding
	PauseLoop  key.Binding

	// Log view
	GotoEnd      key.Binding
	GotoStart    key.Binding
	FollowToggle key.Binding
	PageUp       key.Binding
	PageDown     key.Binding

	// Config editor
	EditConfig  key.Binding
	WriteConfig key.Binding

	// Diff view
	DiffView key.Binding

	// New capabilities
	Space         key.Binding
	ActionsMenu   key.Binding
	LaunchSession key.Binding
	OutputView    key.Binding
	TimelineView  key.Binding
	LoopHealth    key.Binding
	LoopPanel     key.Binding

	// Loop list view actions
	LoopListStart key.Binding
	LoopListStop  key.Binding

	// Loop detail view actions
	LoopDetailStep   key.Binding
	LoopDetailToggle key.Binding
	LoopDetailPause  key.Binding
}

// DefaultKeyMap returns the default key bindings.
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q / Ctrl+C", "Quit"),
		),
		CmdMode: key.NewBinding(
			key.WithKeys(":"),
			key.WithHelp(":", "Command mode"),
		),
		FilterMode: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "Filter mode"),
		),
		Help: key.NewBinding(
			key.WithKeys("?"),
			key.WithHelp("?", "Toggle help"),
		),
		Escape: key.NewBinding(
			key.WithKeys("esc"),
			key.WithHelp("Esc", "Back / cancel"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "Refresh"),
		),
		Tab1: key.NewBinding(
			key.WithKeys("1"),
			key.WithHelp("1", "Repos tab"),
		),
		Tab2: key.NewBinding(
			key.WithKeys("2"),
			key.WithHelp("2", "Sessions tab"),
		),
		Tab3: key.NewBinding(
			key.WithKeys("3"),
			key.WithHelp("3", "Teams tab"),
		),
		Tab4: key.NewBinding(
			key.WithKeys("4"),
			key.WithHelp("4", "Fleet dashboard"),
		),
		Down: key.NewBinding(
			key.WithKeys("j", "down"),
			key.WithHelp("j / k", "Navigate down / up"),
		),
		Up: key.NewBinding(
			key.WithKeys("k", "up"),
			key.WithHelp("k", "up"),
		),
		Enter: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("Enter", "Drill into item"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "Cycle sort column"),
		),
		StartLoop: key.NewBinding(
			key.WithKeys("S"),
			key.WithHelp("S", "Start loop"),
		),
		StopAction: key.NewBinding(
			key.WithKeys("X"),
			key.WithHelp("X", "Stop loop / session"),
		),
		PauseLoop: key.NewBinding(
			key.WithKeys("P"),
			key.WithHelp("P", "Pause / resume loop"),
		),
		GotoEnd: key.NewBinding(
			key.WithKeys("G"),
			key.WithHelp("G", "Jump to end"),
		),
		GotoStart: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "Jump to start"),
		),
		FollowToggle: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "Toggle follow mode"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("ctrl+u"),
			key.WithHelp("Ctrl+U", "Page up"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("ctrl+d"),
			key.WithHelp("Ctrl+D", "Page down"),
		),
		EditConfig: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "Edit config"),
		),
		WriteConfig: key.NewBinding(
			key.WithKeys("w"),
			key.WithHelp("w", "Save config"),
		),
		DiffView: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "View git diff"),
		),
		Space: key.NewBinding(
			key.WithKeys(" "),
			key.WithHelp("Space", "Toggle selection"),
		),
		ActionsMenu: key.NewBinding(
			key.WithKeys("a"),
			key.WithHelp("a", "Actions menu"),
		),
		LaunchSession: key.NewBinding(
			key.WithKeys("L"),
			key.WithHelp("L", "Launch session"),
		),
		OutputView: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "Session output"),
		),
		TimelineView: key.NewBinding(
			key.WithKeys("t"),
			key.WithHelp("t", "Session timeline"),
		),
		LoopHealth: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "Loop health"),
		),
		LoopPanel: key.NewBinding(
			key.WithKeys("l"),
			key.WithHelp("l", "Loop list"),
		),
		LoopListStart: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "Start loop for repo"),
		),
		LoopListStop: key.NewBinding(
			key.WithKeys("x", "d"),
			key.WithHelp("x / d", "Stop selected loop"),
		),
		LoopDetailStep: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "Step loop once"),
		),
		LoopDetailToggle: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "Toggle run/stop"),
		),
		LoopDetailPause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "Pause / resume"),
		),
	}
}

// SetViewContext enables/disables bindings based on the current view.
func (k *KeyMap) SetViewContext(view ViewMode) {
	// Reset all view-specific bindings to enabled
	k.StartLoop.SetEnabled(true)
	k.StopAction.SetEnabled(true)
	k.PauseLoop.SetEnabled(true)
	k.EditConfig.SetEnabled(true)
	k.WriteConfig.SetEnabled(true)
	k.DiffView.SetEnabled(true)
	k.GotoEnd.SetEnabled(true)
	k.GotoStart.SetEnabled(true)
	k.FollowToggle.SetEnabled(true)
	k.PageUp.SetEnabled(true)
	k.PageDown.SetEnabled(true)
	k.Space.SetEnabled(true)
	k.ActionsMenu.SetEnabled(true)
	k.LaunchSession.SetEnabled(true)
	k.OutputView.SetEnabled(true)
	k.TimelineView.SetEnabled(true)
	k.LoopHealth.SetEnabled(true)
	k.LoopPanel.SetEnabled(true)
	k.LoopListStart.SetEnabled(true)
	k.LoopListStop.SetEnabled(true)
	k.LoopDetailStep.SetEnabled(true)
	k.LoopDetailToggle.SetEnabled(true)
	k.LoopDetailPause.SetEnabled(true)

	switch view {
	case ViewOverview:
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.GotoEnd.SetEnabled(false)
		k.GotoStart.SetEnabled(false)
		k.FollowToggle.SetEnabled(false)
		k.PageUp.SetEnabled(false)
		k.PageDown.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewRepoDetail:
		k.Space.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewSessions:
		k.StartLoop.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewSessionDetail:
		k.StartLoop.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewTeams:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewTeamDetail:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewFleet:
		k.StartLoop.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewLogs:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewConfigEditor:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewTimeline:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewDiff:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
	case ViewHelp:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
	case ViewLoopList:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopPanel.SetEnabled(false)
		k.LoopDetailStep.SetEnabled(false)
		k.LoopDetailToggle.SetEnabled(false)
		k.LoopDetailPause.SetEnabled(false)
		// LoopListStart and LoopListStop remain enabled
	case ViewLoopDetail:
		k.StartLoop.SetEnabled(false)
		k.StopAction.SetEnabled(false)
		k.PauseLoop.SetEnabled(false)
		k.EditConfig.SetEnabled(false)
		k.WriteConfig.SetEnabled(false)
		k.DiffView.SetEnabled(false)
		k.Space.SetEnabled(false)
		k.LaunchSession.SetEnabled(false)
		k.OutputView.SetEnabled(false)
		k.TimelineView.SetEnabled(false)
		k.LoopHealth.SetEnabled(false)
		k.LoopPanel.SetEnabled(false)
		k.LoopListStart.SetEnabled(false)
		k.LoopListStop.SetEnabled(false)
		k.Refresh.SetEnabled(false)
		// LoopDetailStep, LoopDetailToggle, and LoopDetailPause remain enabled
	}
}

// ShortHelp returns bindings for the short help view.
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Down, k.Up, k.Enter, k.Help, k.Quit}
}

// FullHelp returns bindings grouped for the full help view.
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Tab1, k.Tab2, k.Tab3, k.Tab4},
		{k.Quit, k.CmdMode, k.FilterMode, k.Help, k.Escape, k.Refresh},
		{k.Down, k.Up, k.Enter, k.Sort},
		{k.StartLoop, k.StopAction, k.PauseLoop},
		{k.GotoEnd, k.GotoStart, k.FollowToggle, k.PageUp, k.PageDown},
		{k.EditConfig, k.WriteConfig, k.DiffView},
	}
}

// HelpGroups returns named groups for the full help overlay.
func (k KeyMap) HelpGroups() []views.HelpGroup {
	return []views.HelpGroup{
		{Name: "Navigation", Bindings: []key.Binding{k.Tab1, k.Tab2, k.Tab3, k.Tab4}},
		{Name: "Global", Bindings: []key.Binding{k.Quit, k.CmdMode, k.FilterMode, k.Help, k.Escape, k.Refresh}},
		{Name: "Loop List", Bindings: []key.Binding{k.LoopPanel, k.LoopListStart, k.LoopListStop}},
		{Name: "Loop Detail", Bindings: []key.Binding{k.LoopDetailStep, k.LoopDetailToggle, k.LoopDetailPause}},
		{Name: "Repos Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StartLoop, k.StopAction, k.PauseLoop}},
		{Name: "Sessions Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StopAction}},
		{Name: "Teams Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort}},
		{Name: "Repo Detail", Bindings: []key.Binding{k.Enter, k.EditConfig, k.StartLoop, k.StopAction, k.PauseLoop, k.DiffView, k.LoopHealth}},
		{Name: "Session Detail", Bindings: []key.Binding{k.Enter, k.OutputView, k.DiffView, k.TimelineView, k.StopAction}},
		{Name: "Team Detail", Bindings: []key.Binding{k.Enter, k.DiffView, k.TimelineView}},
		{Name: "Fleet", Bindings: []key.Binding{k.Down, k.Enter, k.StopAction, k.DiffView, k.TimelineView}},
		{Name: "Log Viewer", Bindings: []key.Binding{k.Down, k.GotoEnd, k.GotoStart, k.FollowToggle, k.PageUp, k.PageDown}},
		{Name: "Config Editor", Bindings: []key.Binding{k.Down, k.Enter, k.WriteConfig}},
	}
}

// KeyHandler handles a key press for the given model.
type KeyHandler func(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd)

// KeyDispatchEntry pairs a binding accessor with its handler.
type KeyDispatchEntry struct {
	Binding func(km *KeyMap) key.Binding
	Handler KeyHandler
}

// KeyDispatch is the ordered global key dispatch table for handleKey.
// A slice is used instead of a map to guarantee deterministic first-match
// semantics, identical to the original switch/case block (Go map iteration
// is non-deterministic and would silently break priority ordering).
var KeyDispatch []KeyDispatchEntry

func init() {
	KeyDispatch = []KeyDispatchEntry{
		{func(km *KeyMap) key.Binding { return km.Quit }, handleQuit},
		{func(km *KeyMap) key.Binding { return km.CmdMode }, handleCmdMode},
		{func(km *KeyMap) key.Binding { return km.FilterMode }, handleFilterMode},
		{func(km *KeyMap) key.Binding { return km.Help }, handleHelp},
		{func(km *KeyMap) key.Binding { return km.LoopPanel }, handleLoopPanel},
		{func(km *KeyMap) key.Binding { return km.Escape }, handleEscape},
		{func(km *KeyMap) key.Binding { return km.Refresh }, handleRefresh},
		{func(km *KeyMap) key.Binding { return km.Tab1 }, handleTab1},
		{func(km *KeyMap) key.Binding { return km.Tab2 }, handleTab2},
		{func(km *KeyMap) key.Binding { return km.Tab3 }, handleTab3},
		{func(km *KeyMap) key.Binding { return km.Tab4 }, handleTab4},
		{func(km *KeyMap) key.Binding { return km.LoopListStart }, handleLoopListStart},
		{func(km *KeyMap) key.Binding { return km.LoopListStop }, handleLoopListStop},
	}
}

func handleQuit(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ProcMgr.StopAll()
	if m.SessMgr != nil {
		m.SessMgr.StopAll()
	}
	return *m, tea.Quit
}

func handleCmdMode(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.InputMode = ModeCommand
	m.CommandBuf = ""
	return *m, nil
}

func handleFilterMode(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.InputMode = ModeFilter
	m.Filter.Active = true
	m.Filter.Text = ""
	return *m, nil
}

func handleHelp(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.CurrentView == ViewHelp {
		return m.popView()
	}
	m.pushView(ViewHelp, "Help")
	return *m, nil
}

func handleLoopPanel(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.pushView(ViewLoopList, "Loops")
	return *m, m.loopListCmd()
}

func handleEscape(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ShowLoopPanel {
		m.ShowLoopPanel = false
		return *m, nil
	}
	tbl := m.activeTable()
	if tbl != nil && tbl.HasSelection() {
		tbl.ClearSelection()
		return *m, nil
	}
	return m.popView()
}

func handleRefresh(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	return *m, m.scanRepos()
}

func handleTab1(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.switchTab(0, ViewOverview, "Repos")
	return *m, nil
}

func handleTab2(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.switchTab(1, ViewSessions, "Sessions")
	return *m, nil
}

func handleTab3(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.switchTab(2, ViewTeams, "Teams")
	return *m, nil
}

func handleTab4(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.switchTab(3, ViewFleet, "Fleet")
	return *m, nil
}
