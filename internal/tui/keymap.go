package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

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
		{Name: "Global", Bindings: []key.Binding{k.Quit, k.CmdMode, k.FilterMode, k.Help, k.Escape, k.Refresh, k.EventLogView}},
		{Name: "Loop List", Bindings: []key.Binding{k.LoopPanel, k.LoopListStart, k.LoopListStop, k.LoopListPause}},
		{Name: "Loop Detail", Bindings: []key.Binding{k.LoopDetailStep, k.LoopDetailToggle, k.LoopDetailPause}},
		{Name: "Loop Control", Bindings: []key.Binding{k.LoopControlPanel, k.LoopCtrlStep, k.LoopCtrlToggle, k.LoopCtrlPause}},
		{Name: "Repos Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StartLoop, k.StopAction, k.PauseLoop}},
		{Name: "Sessions Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort, k.StopAction}},
		{Name: "Teams Table", Bindings: []key.Binding{k.Down, k.Enter, k.Sort}},
		{Name: "Repo Detail", Bindings: []key.Binding{k.Enter, k.EditConfig, k.StartLoop, k.StopAction, k.PauseLoop, k.DiffView, k.LoopHealth, k.ObservationView}},
		{Name: "Session Detail", Bindings: []key.Binding{k.Enter, k.OutputView, k.DiffView, k.TimelineView, k.StopAction}},
		{Name: "Team Detail", Bindings: []key.Binding{k.Enter, k.DiffView, k.TimelineView}},
		{Name: "Fleet", Bindings: []key.Binding{k.Down, k.Enter, k.StopAction, k.DiffView, k.TimelineView}},
		{Name: "Log Viewer", Bindings: []key.Binding{k.Down, k.GotoEnd, k.GotoStart, k.FollowToggle, k.PageUp, k.PageDown}},
		{Name: "Config Editor", Bindings: []key.Binding{k.Down, k.Enter, k.WriteConfig}},
		{Name: "R&D Cycle", Bindings: []key.Binding{k.RDCycle, k.Down, k.GotoEnd, k.GotoStart, k.PageUp, k.PageDown}},
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
		{func(km *KeyMap) key.Binding { return km.LoopControlPanel }, handleLoopControlPanel},
		{func(km *KeyMap) key.Binding { return km.Escape }, handleEscape},
		{func(km *KeyMap) key.Binding { return km.Refresh }, handleRefresh},
		{func(km *KeyMap) key.Binding { return km.Tab1 }, handleTab1},
		{func(km *KeyMap) key.Binding { return km.Tab2 }, handleTab2},
		{func(km *KeyMap) key.Binding { return km.Tab3 }, handleTab3},
		{func(km *KeyMap) key.Binding { return km.Tab4 }, handleTab4},
		{func(km *KeyMap) key.Binding { return km.ObservationView }, handleObservationView},
		{func(km *KeyMap) key.Binding { return km.EventLogView }, handleEventLogView},
		{func(km *KeyMap) key.Binding { return km.LoopListStart }, handleLoopListStart},
		{func(km *KeyMap) key.Binding { return km.LoopListStop }, handleLoopListStop},
		{func(km *KeyMap) key.Binding { return km.LoopListPause }, handleLoopListPause},
		{func(km *KeyMap) key.Binding { return km.RDCycle }, handleRDCycleView},
	}
}

func handleQuit(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.ProcMgr.StopAll(m.Ctx)
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
	if m.Nav.CurrentView == ViewHelp {
		return m.popView()
	}
	m.pushView(ViewHelp, "Help")
	return *m, nil
}

func handleLoopPanel(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.pushView(ViewLoopList, "Loops")
	return *m, m.loopListCmd()
}

func handleLoopControlPanel(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.refreshLoopControlData()
	m.pushView(ViewLoopControl, "Loop Control")
	return *m, nil
}

func handleObservationView(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.Sel.RepoIdx < 0 || m.Sel.RepoIdx >= len(m.Repos) {
		return *m, nil
	}
	m.pushView(ViewObservation, "Observations")
	return *m, nil
}

func handleEventLogView(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.EventLog == nil {
		elv := views.NewEventLogView()
		m.EventLog = &elv
		m.EventLog.SetDimensions(m.Width, m.Height)
	}
	// Load recent history from event bus
	if m.EventBus != nil {
		history := m.EventBus.History("", 200)
		entries := make([]views.EventLogEntry, len(history))
		for i, e := range history {
			msg := string(e.Type)
			if v, ok := e.Data["message"]; ok {
				msg = fmt.Sprintf("%v", v)
			}
			entries[i] = views.EventLogEntry{
				Timestamp: e.Timestamp,
				Type:      string(e.Type),
				Session:   e.SessionID,
				Message:   msg,
			}
		}
		m.EventLog.LoadHistory(entries)
	}
	m.pushView(ViewEventLog, "Event Log")
	return *m, nil
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

func handleRDCycleView(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.pushView(ViewRDCycle, "R&D Cycle")
	return *m, nil
}
