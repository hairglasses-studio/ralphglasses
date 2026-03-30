package tui

import (
	"context"
	"reflect"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

func TestDefaultKeyMapReturnsNonNil(t *testing.T) {
	km := DefaultKeyMap()
	// Spot-check that the keymap is not zero-valued.
	if len(km.Quit.Keys()) == 0 {
		t.Fatal("Quit binding should have keys")
	}
}

func TestDefaultKeyMapBindingsHaveKeys(t *testing.T) {
	km := DefaultKeyMap()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := v.Type().Field(i).Name
		b, ok := field.Interface().(key.Binding)
		if !ok {
			t.Fatalf("field %s is not a key.Binding", name)
		}
		if len(b.Keys()) == 0 {
			t.Errorf("binding %s has no keys", name)
		}
	}
}

func TestDefaultKeyMapBindingsHaveHelp(t *testing.T) {
	km := DefaultKeyMap()
	v := reflect.ValueOf(km)
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		name := v.Type().Field(i).Name
		b := field.Interface().(key.Binding)
		h := b.Help()
		if h.Key == "" || h.Desc == "" {
			t.Errorf("binding %s has incomplete help: key=%q desc=%q", name, h.Key, h.Desc)
		}
	}
}

func TestDefaultKeyMapSpecificBindings(t *testing.T) {
	km := DefaultKeyMap()
	tests := []struct {
		name string
		b    key.Binding
		keys []string
	}{
		{"Quit", km.Quit, []string{"q", "ctrl+c"}},
		{"CmdMode", km.CmdMode, []string{":"}},
		{"FilterMode", km.FilterMode, []string{"/"}},
		{"Help", km.Help, []string{"?"}},
		{"Escape", km.Escape, []string{"esc"}},
		{"Refresh", km.Refresh, []string{"r"}},
		{"Tab1", km.Tab1, []string{"1"}},
		{"Tab2", km.Tab2, []string{"2"}},
		{"Tab3", km.Tab3, []string{"3"}},
		{"Tab4", km.Tab4, []string{"4"}},
		{"Down", km.Down, []string{"j", "down"}},
		{"Up", km.Up, []string{"k", "up"}},
		{"Enter", km.Enter, []string{"enter"}},
		{"StartLoop", km.StartLoop, []string{"S"}},
		{"StopAction", km.StopAction, []string{"X"}},
		{"PauseLoop", km.PauseLoop, []string{"P"}},
		{"DiffView", km.DiffView, []string{"d"}},
		{"LaunchSession", km.LaunchSession, []string{"L"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.b.Keys()
			if !reflect.DeepEqual(got, tt.keys) {
				t.Errorf("keys = %v, want %v", got, tt.keys)
			}
		})
	}
}

func TestSetViewContextOverview(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewOverview)

	// Should be enabled in overview
	enabled := []key.Binding{km.Quit, km.CmdMode, km.FilterMode, km.Help, km.Escape, km.Refresh, km.Down, km.Up, km.Enter, km.StartLoop, km.StopAction, km.PauseLoop, km.ActionsMenu, km.LoopPanel, km.EventLogView}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewOverview", b.Help().Desc)
		}
	}

	// Should be disabled in overview
	disabled := []key.Binding{km.EditConfig, km.WriteConfig, km.DiffView, km.GotoEnd, km.GotoStart, km.FollowToggle, km.PageUp, km.PageDown, km.LaunchSession, km.OutputView, km.TimelineView, km.LoopHealth, km.LoopListStart, km.LoopListStop, km.LoopListPause, km.LoopDetailStep, km.LoopDetailToggle, km.LoopDetailPause, km.LoopCtrlStep, km.LoopCtrlToggle, km.LoopCtrlPause, km.ObservationView}
	for _, b := range disabled {
		if b.Enabled() {
			t.Errorf("binding %q should be disabled in ViewOverview", b.Help().Desc)
		}
	}
}

func TestSetViewContextRepoDetail(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewRepoDetail)

	// Enabled in repo detail
	enabled := []key.Binding{km.StartLoop, km.StopAction, km.PauseLoop, km.EditConfig, km.WriteConfig, km.DiffView, km.LaunchSession, km.TimelineView, km.LoopHealth, km.ActionsMenu, km.LoopPanel}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewRepoDetail", b.Help().Desc)
		}
	}

	// Disabled in repo detail
	disabled := []key.Binding{km.Space, km.OutputView, km.LoopListStart, km.LoopListStop, km.LoopDetailStep, km.EventLogView}
	for _, b := range disabled {
		if b.Enabled() {
			t.Errorf("binding %q should be disabled in ViewRepoDetail", b.Help().Desc)
		}
	}
}

func TestSetViewContextLogs(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewLogs)

	// Log navigation should be enabled
	enabled := []key.Binding{km.GotoEnd, km.GotoStart, km.FollowToggle, km.PageUp, km.PageDown}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewLogs", b.Help().Desc)
		}
	}

	// Action keys disabled
	disabled := []key.Binding{km.StartLoop, km.StopAction, km.PauseLoop, km.EditConfig, km.WriteConfig, km.DiffView, km.Space, km.LaunchSession, km.OutputView, km.TimelineView, km.LoopHealth}
	for _, b := range disabled {
		if b.Enabled() {
			t.Errorf("binding %q should be disabled in ViewLogs", b.Help().Desc)
		}
	}
}

func TestSetViewContextAllViews(t *testing.T) {
	views := []struct {
		name string
		view ViewMode
	}{
		{"ViewOverview", ViewOverview},
		{"ViewRepoDetail", ViewRepoDetail},
		{"ViewSessions", ViewSessions},
		{"ViewSessionDetail", ViewSessionDetail},
		{"ViewTeams", ViewTeams},
		{"ViewTeamDetail", ViewTeamDetail},
		{"ViewFleet", ViewFleet},
		{"ViewLogs", ViewLogs},
		{"ViewConfigEditor", ViewConfigEditor},
		{"ViewTimeline", ViewTimeline},
		{"ViewDiff", ViewDiff},
		{"ViewHelp", ViewHelp},
		{"ViewLoopList", ViewLoopList},
		{"ViewLoopDetail", ViewLoopDetail},
		{"ViewLoopControl", ViewLoopControl},
		{"ViewObservation", ViewObservation},
		{"ViewEventLog", ViewEventLog},
		{"ViewRDCycle", ViewRDCycle},
	}
	for _, tt := range views {
		t.Run(tt.name, func(t *testing.T) {
			km := DefaultKeyMap()
			// Should not panic for any view
			km.SetViewContext(tt.view)

			// Global bindings should always be enabled (except Refresh in LoopDetail)
			if tt.view != ViewLoopDetail {
				if !km.Quit.Enabled() {
					t.Error("Quit should always be enabled")
				}
				if !km.CmdMode.Enabled() {
					t.Error("CmdMode should always be enabled")
				}
				if !km.Help.Enabled() {
					t.Error("Help should always be enabled")
				}
			}
		})
	}
}

func TestSetViewContextFleet(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewFleet)

	// Fleet keeps StopAction, DiffView, TimelineView, EventLogView
	enabled := []key.Binding{km.StopAction, km.DiffView, km.TimelineView, km.EventLogView}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewFleet", b.Help().Desc)
		}
	}

	disabled := []key.Binding{km.StartLoop, km.PauseLoop, km.EditConfig, km.WriteConfig, km.Space, km.LaunchSession, km.OutputView, km.LoopHealth}
	for _, b := range disabled {
		if b.Enabled() {
			t.Errorf("binding %q should be disabled in ViewFleet", b.Help().Desc)
		}
	}
}

func TestSetViewContextLoopList(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopList)

	// Loop list actions remain enabled
	enabled := []key.Binding{km.LoopListStart, km.LoopListStop, km.LoopListPause}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewLoopList", b.Help().Desc)
		}
	}

	// Loop detail/control actions disabled
	disabled := []key.Binding{km.LoopDetailStep, km.LoopDetailToggle, km.LoopDetailPause, km.LoopCtrlStep, km.LoopCtrlToggle, km.LoopCtrlPause, km.LoopPanel}
	for _, b := range disabled {
		if b.Enabled() {
			t.Errorf("binding %q should be disabled in ViewLoopList", b.Help().Desc)
		}
	}
}

func TestSetViewContextLoopDetail(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopDetail)

	// Loop detail actions remain enabled
	enabled := []key.Binding{km.LoopDetailStep, km.LoopDetailToggle, km.LoopDetailPause}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewLoopDetail", b.Help().Desc)
		}
	}

	// Refresh is disabled in loop detail
	if km.Refresh.Enabled() {
		t.Error("Refresh should be disabled in ViewLoopDetail")
	}
}

func TestSetViewContextLoopControl(t *testing.T) {
	km := DefaultKeyMap()
	km.SetViewContext(ViewLoopControl)

	// Loop control actions remain enabled
	enabled := []key.Binding{km.LoopCtrlStep, km.LoopCtrlToggle, km.LoopCtrlPause}
	for _, b := range enabled {
		if !b.Enabled() {
			t.Errorf("binding %q should be enabled in ViewLoopControl", b.Help().Desc)
		}
	}

	// LoopControlPanel itself is disabled (already in it)
	if km.LoopControlPanel.Enabled() {
		t.Error("LoopControlPanel should be disabled in ViewLoopControl")
	}
}

func TestShortHelp(t *testing.T) {
	km := DefaultKeyMap()
	bindings := km.ShortHelp()
	if len(bindings) != 5 {
		t.Errorf("ShortHelp returned %d bindings, want 5", len(bindings))
	}
}

func TestFullHelp(t *testing.T) {
	km := DefaultKeyMap()
	groups := km.FullHelp()
	if len(groups) != 6 {
		t.Errorf("FullHelp returned %d groups, want 6", len(groups))
	}
}

func TestHelpGroups(t *testing.T) {
	km := DefaultKeyMap()
	groups := km.HelpGroups()
	if len(groups) == 0 {
		t.Fatal("HelpGroups should return at least one group")
	}
	// Check that group names are set
	for _, g := range groups {
		if g.Name == "" {
			t.Error("help group has empty name")
		}
		if len(g.Bindings) == 0 {
			t.Errorf("help group %q has no bindings", g.Name)
		}
	}
	// Spot check known groups
	names := make(map[string]bool)
	for _, g := range groups {
		names[g.Name] = true
	}
	expected := []string{"Navigation", "Global", "Loop List", "Loop Detail", "Loop Control", "Repos Table", "Sessions Table", "Teams Table", "Repo Detail", "Session Detail", "Fleet", "Log Viewer", "Config Editor"}
	for _, name := range expected {
		if !names[name] {
			t.Errorf("missing help group %q", name)
		}
	}
}

func TestKeyDispatchNotEmpty(t *testing.T) {
	if len(KeyDispatch) == 0 {
		t.Fatal("KeyDispatch should not be empty")
	}
	km := DefaultKeyMap()
	for i, entry := range KeyDispatch {
		b := entry.Binding(&km)
		if len(b.Keys()) == 0 {
			t.Errorf("KeyDispatch[%d] binding has no keys", i)
		}
		if entry.Handler == nil {
			t.Errorf("KeyDispatch[%d] handler is nil", i)
		}
	}
}

// --- handleTab handlers tests ---

func TestHandleTab1(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.switchTab(2, ViewTeams, "Teams") // start on different tab

	m2, cmd := handleTab1(&m, tea.KeyPressMsg{Code: '1', Text: "1"})
	got := m2.(Model)
	if got.Nav.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", got.Nav.ActiveTab)
	}
	if got.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("handleTab1 should return nil cmd")
	}
}

func TestHandleTab2(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleTab2(&m, tea.KeyPressMsg{Code: '2', Text: "2"})
	got := m2.(Model)
	if got.Nav.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d, want 1", got.Nav.ActiveTab)
	}
	if got.Nav.CurrentView != ViewSessions {
		t.Errorf("CurrentView = %v, want ViewSessions", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("handleTab2 should return nil cmd")
	}
}

func TestHandleTab3(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleTab3(&m, tea.KeyPressMsg{Code: '3', Text: "3"})
	got := m2.(Model)
	if got.Nav.ActiveTab != 2 {
		t.Errorf("ActiveTab = %d, want 2", got.Nav.ActiveTab)
	}
	if got.Nav.CurrentView != ViewTeams {
		t.Errorf("CurrentView = %v, want ViewTeams", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("handleTab3 should return nil cmd")
	}
}

func TestHandleTab4(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleTab4(&m, tea.KeyPressMsg{Code: '4', Text: "4"})
	got := m2.(Model)
	if got.Nav.ActiveTab != 3 {
		t.Errorf("ActiveTab = %d, want 3", got.Nav.ActiveTab)
	}
	if got.Nav.CurrentView != ViewFleet {
		t.Errorf("CurrentView = %v, want ViewFleet", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("handleTab4 should return nil cmd")
	}
}

// --- handleLoopControlPanel test ---

func TestHandleLoopControlPanel(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleLoopControlPanel(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewLoopControl {
		t.Errorf("CurrentView = %v, want ViewLoopControl", got.Nav.CurrentView)
	}
	if cmd != nil {
		t.Error("handleLoopControlPanel should return nil cmd")
	}
}

// --- handleObservationView test ---

func TestHandleObservationView_NoRepos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Sel.RepoIdx = 0
	// No repos — should return unchanged model
	m2, cmd := handleObservationView(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView == ViewObservation {
		t.Error("should not push observation view when no repos exist")
	}
	if cmd != nil {
		t.Error("should return nil cmd when no repos")
	}
}

func TestHandleObservationView_NegativeIdx(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Sel.RepoIdx = -1
	m2, cmd := handleObservationView(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView == ViewObservation {
		t.Error("should not push observation view with negative index")
	}
	if cmd != nil {
		t.Error("should return nil cmd")
	}
}

// --- handleEventLogView test ---

func TestHandleEventLogView_NilEventLog(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.EventLog = nil
	m.Width = 120
	m.Height = 40

	m2, cmd := handleEventLogView(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewEventLog {
		t.Errorf("CurrentView = %v, want ViewEventLog", got.Nav.CurrentView)
	}
	if got.EventLog == nil {
		t.Error("EventLog should be initialized")
	}
	if cmd != nil {
		t.Error("handleEventLogView should return nil cmd")
	}
}

// --- handleEscape tests ---

func TestHandleEscape_LoopPanelOpen(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.ShowLoopPanel = true

	m2, cmd := handleEscape(&m, tea.KeyPressMsg{Code: tea.KeyEsc})
	got := m2.(Model)
	if got.ShowLoopPanel {
		t.Error("escape should close loop panel")
	}
	if cmd != nil {
		t.Error("escape should return nil cmd")
	}
}

func TestHandleEscape_PopView(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.pushView(ViewRepoDetail, "Detail")

	m2, cmd := handleEscape(&m, tea.KeyPressMsg{Code: tea.KeyEsc})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview after pop", got.Nav.CurrentView)
	}
	_ = cmd
}

// --- handleQuit test ---

func TestHandleQuit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()

	_, cmd := handleQuit(&m, tea.KeyPressMsg{})
	if cmd == nil {
		t.Error("handleQuit should return tea.Quit")
	}
}

// --- handleCmdMode test ---

func TestHandleCmdMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleCmdMode(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.InputMode != ModeCommand {
		t.Errorf("InputMode = %d, want ModeCommand", got.InputMode)
	}
	if got.CommandBuf != "" {
		t.Error("CommandBuf should be empty")
	}
	if cmd != nil {
		t.Error("handleCmdMode should return nil cmd")
	}
}

// --- handleFilterMode test ---

func TestHandleFilterMode(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleFilterMode(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.InputMode != ModeFilter {
		t.Errorf("InputMode = %d, want ModeFilter", got.InputMode)
	}
	if !got.Filter.Active {
		t.Error("Filter should be active")
	}
	if cmd != nil {
		t.Error("handleFilterMode should return nil cmd")
	}
}

// --- handleHelp tests ---

func TestHandleHelp_Toggle(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	// Push help
	m2, _ := handleHelp(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewHelp {
		t.Errorf("CurrentView = %v, want ViewHelp", got.Nav.CurrentView)
	}

	// Pop help
	m3, _ := handleHelp(&got, tea.KeyPressMsg{})
	got2 := m3.(Model)
	if got2.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview after toggle", got2.Nav.CurrentView)
	}
}

// --- handleRefresh test ---

func TestHandleRefresh(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	_, cmd := handleRefresh(&m, tea.KeyPressMsg{})
	if cmd == nil {
		t.Error("handleRefresh should return a scan command")
	}
}

// --- handleLoopPanel test ---

func TestHandleLoopPanel(t *testing.T) {
	m := NewModel("/tmp/test", nil)

	m2, cmd := handleLoopPanel(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewLoopList {
		t.Errorf("CurrentView = %v, want ViewLoopList", got.Nav.CurrentView)
	}
	if cmd == nil {
		t.Error("handleLoopPanel should return a loop list cmd")
	}
}
