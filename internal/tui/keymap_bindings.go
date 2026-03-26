package tui

import (
	"github.com/charmbracelet/bubbles/key"
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
	LoopListPause key.Binding

	// Loop detail view actions
	LoopDetailStep   key.Binding
	LoopDetailToggle key.Binding
	LoopDetailPause  key.Binding

	// Loop control panel
	LoopControlPanel key.Binding
	LoopCtrlStep     key.Binding
	LoopCtrlToggle   key.Binding
	LoopCtrlPause    key.Binding

	// Observation and event log views
	ObservationView key.Binding
	EventLogView    key.Binding
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
		LoopListPause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "Pause / resume loop"),
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
		LoopControlPanel: key.NewBinding(
			key.WithKeys("C"),
			key.WithHelp("C", "Loop control panel"),
		),
		LoopCtrlStep: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "Force step iteration"),
		),
		LoopCtrlToggle: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "Toggle run/stop"),
		),
		LoopCtrlPause: key.NewBinding(
			key.WithKeys("p"),
			key.WithHelp("p", "Pause / resume"),
		),
		ObservationView: key.NewBinding(
			key.WithKeys("o"),
			key.WithHelp("o", "Observation sparklines"),
		),
		EventLogView: key.NewBinding(
			key.WithKeys("e"),
			key.WithHelp("e", "Event log"),
		),
	}
}
