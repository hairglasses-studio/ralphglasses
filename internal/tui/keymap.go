package tui

import (
	"github.com/charmbracelet/bubbles/key"

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
		{k.EditConfig, k.WriteConfig},
	}
}

// HelpGroups returns named groups for the full help overlay.
func (k KeyMap) HelpGroups() []views.HelpGroup {
	return []views.HelpGroup{
		{Name: "Navigation", Bindings: []key.Binding{k.Tab1, k.Tab2, k.Tab3, k.Tab4}},
		{Name: "Global", Bindings: []key.Binding{k.Quit, k.CmdMode, k.FilterMode, k.Help, k.Escape, k.Refresh}},
		{Name: "Repos Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StartLoop, k.StopAction, k.PauseLoop}},
		{Name: "Sessions Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StopAction}},
		{Name: "Teams Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort}},
		{Name: "Repo Detail", Bindings: []key.Binding{k.Enter, k.EditConfig, k.StartLoop, k.StopAction, k.PauseLoop}},
		{Name: "Log Viewer", Bindings: []key.Binding{k.Down, k.GotoEnd, k.GotoStart, k.FollowToggle, k.PageUp, k.PageDown}},
		{Name: "Config Editor", Bindings: []key.Binding{k.Down, k.Enter, k.WriteConfig}},
	}
}
