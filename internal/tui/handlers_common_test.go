package tui

import (
	"context"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- dispatchViewKeys tests ---

func TestDispatchViewKeys_BindingMatch(t *testing.T) {
	called := false
	entries := []ViewKeyEntry{
		{
			Binding: func(km *KeyMap) key.Binding { return km.Enter },
			Handler: func(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				called = true
				return *m, nil
			},
		},
	}
	m := NewModel("/tmp/test", nil)
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	dispatchViewKeys(entries, &m, msg)
	if !called {
		t.Error("expected binding handler to be called on matching key")
	}
}

func TestDispatchViewKeys_MatchFunc(t *testing.T) {
	called := false
	entries := []ViewKeyEntry{
		{
			Match: func(msg tea.KeyPressMsg) bool { return msg.Key().Code == tea.KeyBackspace },
			Handler: func(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				called = true
				return *m, nil
			},
		},
	}
	m := NewModel("/tmp/test", nil)
	msg := tea.KeyPressMsg{Code: tea.KeyBackspace}
	dispatchViewKeys(entries, &m, msg)
	if !called {
		t.Error("expected match handler to be called on matching key")
	}
}

func TestDispatchViewKeys_NoMatch(t *testing.T) {
	entries := []ViewKeyEntry{
		{
			Binding: func(km *KeyMap) key.Binding { return km.Enter },
			Handler: func(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) {
				t.Error("handler should not be called")
				return *m, nil
			},
		},
	}
	m := NewModel("/tmp/test", nil)
	msg := tea.KeyPressMsg{Code: tea.KeyBackspace}
	result, cmd := dispatchViewKeys(entries, &m, msg)
	if cmd != nil {
		t.Error("expected nil cmd when no entry matches")
	}
	_ = result
}

func TestDispatchViewKeys_FirstMatchWins(t *testing.T) {
	order := ""
	entries := []ViewKeyEntry{
		{
			Match:   func(msg tea.KeyPressMsg) bool { return true },
			Handler: func(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) { order += "A"; return *m, nil },
		},
		{
			Match:   func(msg tea.KeyPressMsg) bool { return true },
			Handler: func(m *Model, _ tea.KeyPressMsg) (tea.Model, tea.Cmd) { order += "B"; return *m, nil },
		},
	}
	m := NewModel("/tmp/test", nil)
	dispatchViewKeys(entries, &m, tea.KeyPressMsg{Code: 'x', Text: "x"})
	if order != "A" {
		t.Errorf("expected first match to win, got order=%q", order)
	}
}

// --- handleCommandInput tests ---

func TestHandleCommandInput_TypeCharacters(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = m2.(Model)
	m2, _ = m.handleCommandInput(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = m2.(Model)

	if m.CommandBuf != "hi" {
		t.Errorf("CommandBuf = %q, want %q", m.CommandBuf, "hi")
	}
}

func TestHandleCommandInput_Backspace(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand
	m.CommandBuf = "abc"

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = m2.(Model)
	if m.CommandBuf != "ab" {
		t.Errorf("CommandBuf = %q, want %q", m.CommandBuf, "ab")
	}
}

func TestHandleCommandInput_BackspaceEmpty(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand
	m.CommandBuf = ""

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = m2.(Model)
	if m.CommandBuf != "" {
		t.Errorf("CommandBuf = %q, want empty", m.CommandBuf)
	}
}

func TestHandleCommandInput_Escape(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand
	m.CommandBuf = "partial"

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Errorf("InputMode = %d, want ModeNormal", m.InputMode)
	}
	if m.CommandBuf != "" {
		t.Errorf("CommandBuf = %q, want empty after escape", m.CommandBuf)
	}
}

func TestHandleCommandInput_EnterExecutes(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand
	m.CommandBuf = "sessions"

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Errorf("InputMode = %d, want ModeNormal after enter", m.InputMode)
	}
	if m.CommandBuf != "" {
		t.Errorf("CommandBuf = %q, want empty after enter", m.CommandBuf)
	}
	if m.Nav.CurrentView != ViewSessions {
		t.Errorf("CurrentView = %v, want ViewSessions after :sessions", m.Nav.CurrentView)
	}
}

// --- handleFilterInput tests ---

func TestHandleFilterInput_TypeCharacters(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeFilter
	m.Filter.Active = true

	m2, _ := m.handleFilterInput(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = m2.(Model)
	m2, _ = m.handleFilterInput(tea.KeyPressMsg{Code: 'b', Text: "b"})
	m = m2.(Model)

	if m.Filter.Text != "ab" {
		t.Errorf("Filter.Text = %q, want %q", m.Filter.Text, "ab")
	}
}

func TestHandleFilterInput_Backspace(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeFilter
	m.Filter.Active = true
	m.Filter.Text = "xyz"

	m2, _ := m.handleFilterInput(tea.KeyPressMsg{Code: tea.KeyBackspace})
	m = m2.(Model)
	if m.Filter.Text != "xy" {
		t.Errorf("Filter.Text = %q, want %q", m.Filter.Text, "xy")
	}
}

func TestHandleFilterInput_EnterConfirms(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeFilter
	m.Filter.Active = true
	m.Filter.Text = "search"

	m2, _ := m.handleFilterInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Errorf("InputMode = %d, want ModeNormal after enter", m.InputMode)
	}
	// Filter text is preserved on enter (not cleared)
	if m.Filter.Text != "search" {
		t.Errorf("Filter.Text = %q, want %q (preserved on enter)", m.Filter.Text, "search")
	}
}

func TestHandleFilterInput_EscapeClears(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeFilter
	m.Filter.Active = true
	m.Filter.Text = "query"

	m2, _ := m.handleFilterInput(tea.KeyPressMsg{Code: tea.KeyEsc})
	m = m2.(Model)
	if m.InputMode != ModeNormal {
		t.Errorf("InputMode = %d, want ModeNormal after escape", m.InputMode)
	}
	if m.Filter.Text != "" {
		t.Errorf("Filter.Text = %q, want empty after escape", m.Filter.Text)
	}
	if m.Filter.Active {
		t.Error("Filter should not be active after escape")
	}
}

// --- execCommand tests ---

func TestExecCommand_Quit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	_, cmd := m.execCommand(Command{Name: "quit"})
	if cmd == nil {
		t.Error(":quit should produce a quit command")
	}
}

func TestExecCommand_Q(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	_, cmd := m.execCommand(Command{Name: "q"})
	if cmd == nil {
		t.Error(":q should produce a quit command")
	}
}

func TestExecCommand_Scan(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	_, cmd := m.execCommand(Command{Name: "scan"})
	if cmd == nil {
		t.Error(":scan should produce a command")
	}
}

func TestExecCommand_Sessions(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "sessions"})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewSessions {
		t.Errorf("CurrentView = %v, want ViewSessions", got.Nav.CurrentView)
	}
	if got.Nav.ActiveTab != 1 {
		t.Errorf("ActiveTab = %d, want 1", got.Nav.ActiveTab)
	}
}

func TestExecCommand_Teams(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "teams"})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewTeams {
		t.Errorf("CurrentView = %v, want ViewTeams", got.Nav.CurrentView)
	}
	if got.Nav.ActiveTab != 2 {
		t.Errorf("ActiveTab = %d, want 2", got.Nav.ActiveTab)
	}
}

func TestExecCommand_Fleet(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "fleet"})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewFleet {
		t.Errorf("CurrentView = %v, want ViewFleet", got.Nav.CurrentView)
	}
	if got.Nav.ActiveTab != 3 {
		t.Errorf("ActiveTab = %d, want 3", got.Nav.ActiveTab)
	}
}

func TestExecCommand_Repos(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Switch away first
	m.switchTab(1, ViewSessions, "Sessions")
	m2, _ := m.execCommand(Command{Name: "repos"})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview", got.Nav.CurrentView)
	}
	if got.Nav.ActiveTab != 0 {
		t.Errorf("ActiveTab = %d, want 0", got.Nav.ActiveTab)
	}
}

func TestExecCommand_StopAll(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "stopall"})
	got := m2.(Model)
	if got.Modals.ConfirmDialog == nil {
		t.Error(":stopall should show confirm dialog")
	}
	if got.Modals.ConfirmDialog != nil && got.Modals.ConfirmDialog.Action != "stopAll" {
		t.Errorf("ConfirmDialog.Action = %q, want %q", got.Modals.ConfirmDialog.Action, "stopAll")
	}
}

func TestExecCommand_Unknown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m2, cmd := m.execCommand(Command{Name: "boguscommand"})
	got := m2.(Model)
	if cmd != nil {
		t.Error("unknown command should return nil cmd")
	}
	if !got.Notify.Active() {
		t.Error("unknown command should trigger notification")
	}
}

func TestExecCommand_StartNoArgs(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, cmd := m.execCommand(Command{Name: "start"})
	_ = m2
	if cmd != nil {
		t.Error(":start with no args should return nil cmd")
	}
}

func TestExecCommand_StopRepoNotFound(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m2, _ := m.execCommand(Command{Name: "stop", Args: []string{"nonexistent"}})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error(":stop with unknown repo should trigger notification")
	}
}

// --- handleConfirmKey tests ---

func TestHandleConfirmKey_Yes(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:   "Test",
		Message: "Confirm?",
		Action:  "stopAll",
		Active:  true,
		Width:   50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: 'y', Text: "y"})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("confirm dialog should be cleared after 'y'")
	}
}

func TestHandleConfirmKey_No(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopAll",
		Active: true,
		Width:  50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("confirm dialog should be cleared after 'n'")
	}
}

func TestHandleConfirmKey_Escape(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopAll",
		Active: true,
		Width:  50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("confirm dialog should be cleared after escape")
	}
}

func TestHandleConfirmKey_Navigation(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:    "Test",
		Action:   "stopAll",
		Selected: 0,
		Active:   true,
		Width:    50,
	}

	// Move right
	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: tea.KeyRight})
	got := m2.(Model)
	// Dialog should still be active (not dismissed by navigation)
	if got.Modals.ConfirmDialog == nil || !got.Modals.ConfirmDialog.Active {
		t.Error("dialog should still be active after right arrow")
	}
	if got.Modals.ConfirmDialog != nil && got.Modals.ConfirmDialog.Selected != 1 {
		t.Errorf("Selected = %d, want 1 after right arrow", got.Modals.ConfirmDialog.Selected)
	}
}

func TestHandleConfirmKey_EnterOnYes(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:    "Test",
		Action:   "stopAll",
		Selected: 0, // Yes
		Active:   true,
		Width:    50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("confirm dialog should be cleared after enter on Yes")
	}
}

func TestHandleConfirmKey_EnterOnNo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:    "Test",
		Action:   "stopAll",
		Selected: 1, // No
		Active:   true,
		Width:    50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("confirm dialog should be cleared after enter on No")
	}
}

func TestHandleConfirmKey_Tab(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:    "Test",
		Action:   "stopAll",
		Selected: 0,
		Active:   true,
		Width:    50,
	}

	m2, _ := m.handleConfirmKey(tea.KeyPressMsg{Code: tea.KeyTab})
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil && got.Modals.ConfirmDialog.Selected != 1 {
		t.Errorf("Selected = %d, want 1 after tab", got.Modals.ConfirmDialog.Selected)
	}
}

// --- handleActionMenuKey tests ---

func TestHandleActionMenuKey_Escape(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{
		Title:  "Actions",
		Active: true,
		Items: []components.ActionItem{
			{Key: "s", Label: "Scan", Action: "scan"},
		},
	}

	m2, cmd := m.handleActionMenuKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	got := m2.(Model)
	// Escape deactivates the menu but does not trigger handleActionResult
	// (selected=false), so ActionMenu pointer remains but Active=false.
	if got.Modals.ActionMenu != nil && got.Modals.ActionMenu.Active {
		t.Error("action menu should be inactive after escape")
	}
	if cmd != nil {
		t.Error("escape should return nil cmd")
	}
}

func TestHandleActionMenuKey_Navigate(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{
		Title:  "Actions",
		Active: true,
		Cursor: 0,
		Items: []components.ActionItem{
			{Key: "s", Label: "Scan", Action: "scan"},
			{Key: "x", Label: "Stop All", Action: "stopAll"},
		},
	}

	// Down
	m2, _ := m.handleActionMenuKey(tea.KeyPressMsg{Code: tea.KeyDown})
	got := m2.(Model)
	if got.Modals.ActionMenu == nil {
		t.Fatal("action menu should still be open after down")
	}
	if got.Modals.ActionMenu.Cursor != 1 {
		t.Errorf("Cursor = %d, want 1 after down", got.Modals.ActionMenu.Cursor)
	}

	// Up
	m2, _ = got.handleActionMenuKey(tea.KeyPressMsg{Code: tea.KeyUp})
	got = m2.(Model)
	if got.Modals.ActionMenu != nil && got.Modals.ActionMenu.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0 after up", got.Modals.ActionMenu.Cursor)
	}
}

func TestHandleActionMenuKey_EnterSelectsItem(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{
		Title:  "Actions",
		Active: true,
		Cursor: 0,
		Items: []components.ActionItem{
			{Key: "s", Label: "Scan", Action: "scan"},
		},
	}

	m2, cmd := m.handleActionMenuKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("action menu should be cleared after enter selection")
	}
	if cmd == nil {
		t.Error("selecting 'scan' should produce a command")
	}
}

func TestHandleActionMenuKey_ShortcutKey(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{
		Title:  "Actions",
		Active: true,
		Cursor: 0,
		Items: []components.ActionItem{
			{Key: "s", Label: "Scan", Action: "scan"},
			{Key: "x", Label: "Stop All", Action: "stopAll"},
		},
	}

	// Press 's' shortcut key
	m2, cmd := m.handleActionMenuKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("action menu should be cleared after shortcut key selection")
	}
	if cmd == nil {
		t.Error("selecting 'scan' via shortcut should produce a command")
	}
}

// --- handleEventLogKey tests ---

func TestHandleEventLogKey_ScrollDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	ev := views.NewEventLogView()
	m.EventLog = &ev

	m2, _ := m.handleEventLogKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_ = m2 // should not panic
}

func TestHandleEventLogKey_ScrollUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	ev := views.NewEventLogView()
	m.EventLog = &ev

	m2, _ := m.handleEventLogKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	_ = m2 // should not panic
}

func TestHandleEventLogKey_NilEventLog(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.EventLog = nil

	// Should not panic with nil EventLog
	m2, _ := m.handleEventLogKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_ = m2
	m2, _ = m.handleEventLogKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	_ = m2
}

// --- truncateID tests ---

func TestTruncateID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"abcdefghij", "abcdefgh"},
		{"short", "short"},
		{"12345678", "12345678"},
		{"123456789", "12345678"},
		{"", ""},
	}
	for _, tt := range tests {
		got := truncateID(tt.input)
		if got != tt.want {
			t.Errorf("truncateID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// --- findFullSessionID tests ---

func TestFindFullSessionID_NilManager(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	got := m.findFullSessionID("abc")
	if got != "" {
		t.Errorf("findFullSessionID with nil manager = %q, want empty", got)
	}
}

// --- startOutputStreaming tests ---

func TestStartOutputStreaming_NoSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Sel.SessionID = ""

	m2, cmd := m.startOutputStreaming()
	got := m2.(Model)
	if cmd != nil {
		t.Error("startOutputStreaming with no session should return nil cmd")
	}
	if got.Stream.Active {
		t.Error("StreamingOutput should be false when no session selected")
	}
}

func TestStartOutputStreaming_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Sel.SessionID = "test-session-id"
	m.SessMgr = nil

	m2, cmd := m.startOutputStreaming()
	got := m2.(Model)
	if cmd != nil {
		t.Error("startOutputStreaming with nil SessMgr should return nil cmd")
	}
	if got.Stream.Active {
		t.Error("StreamingOutput should be false with nil SessMgr")
	}
}

// --- handleConfirmResult tests ---

func TestHandleConfirmResult_Cancel(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopAll",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopAll",
		Result: components.ConfirmCancel,
	}
	m2, cmd := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after cancel")
	}
	if cmd != nil {
		t.Error("cancel should return nil cmd")
	}
}

func TestHandleConfirmResult_NoResult(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopAll",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopAll",
		Result: components.ConfirmNo,
	}
	m2, cmd := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after No")
	}
	if cmd != nil {
		t.Error("No result should return nil cmd")
	}
}

func TestHandleConfirmResult_StopAll(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopAll",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopAll",
		Result: components.ConfirmYes,
	}
	m2, _ := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after confirm stopAll")
	}
}

// --- handleActionResult tests ---

func TestHandleActionResult_Scan(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}

	result := components.ActionResultMsg{Action: "scan"}
	m2, cmd := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil after action result")
	}
	if cmd == nil {
		t.Error("scan action should produce a command")
	}
}

func TestHandleActionResult_StopAll(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}

	result := components.ActionResultMsg{Action: "stopAll"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil after action result")
	}
	if got.Modals.ConfirmDialog == nil {
		t.Error("stopAll action should show confirm dialog")
	}
}

// --- handleLaunchResult tests ---

func TestHandleLaunchResult_EmptyPrompt(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	result := components.LaunchResultMsg{Prompt: ""}
	m2, cmd := m.handleLaunchResult(result)
	got := m2.(Model)
	if got.Modals.Launcher != nil {
		t.Error("Launcher should be nil after empty prompt")
	}
	if cmd != nil {
		t.Error("empty prompt should return nil cmd")
	}
	if !got.Notify.Active() {
		t.Error("empty prompt should show notification")
	}
}

func TestHandleLaunchResult_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	m.Modals.Launcher = &components.SessionLauncher{}

	result := components.LaunchResultMsg{
		Prompt:   "fix the bug",
		Provider: "claude",
		RepoPath: "/tmp/test",
	}
	m2, cmd := m.handleLaunchResult(result)
	got := m2.(Model)
	if got.Modals.Launcher != nil {
		t.Error("Launcher should be nil after result")
	}
	if cmd != nil {
		t.Error("nil SessMgr should return nil cmd")
	}
	if !got.Notify.Active() {
		t.Error("nil SessMgr should show notification")
	}
}

// --- streamSessionOutput tests ---

func TestStreamSessionOutput_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	cmd := m.streamSessionOutput("test-id")
	msg := cmd()
	if done, ok := msg.(SessionOutputDoneMsg); !ok {
		t.Errorf("expected SessionOutputDoneMsg, got %T", msg)
	} else if done.SessionID != "test-id" {
		t.Errorf("SessionID = %q, want %q", done.SessionID, "test-id")
	}
}

// --- Integration: command input flow through handleCommandInput into execCommand ---

func TestCommandInputFlow_QuitViaEnter(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.InputMode = ModeCommand
	m.CommandBuf = "q"

	m2, cmd := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.InputMode != ModeNormal {
		t.Errorf("InputMode = %d, want ModeNormal", got.InputMode)
	}
	if cmd == nil {
		t.Error("entering 'q' command should produce quit cmd")
	}
}

func TestCommandInputFlow_TabSwitch(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.InputMode = ModeCommand
	m.CommandBuf = "fleet"

	m2, _ := m.handleCommandInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewFleet {
		t.Errorf("CurrentView = %v, want ViewFleet", got.Nav.CurrentView)
	}
}

// --- handleConfigKey tests ---

func TestHandleConfigKey_NilEditor(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.ConfigEdit = nil

	m2, cmd := m.handleConfigKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	got := m2.(Model)
	_ = got
	if cmd != nil {
		t.Error("handleConfigKey with nil editor should return nil cmd")
	}
}

func TestHandleConfigKey_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_ = m2
	if cmd != nil {
		t.Error("move down should return nil cmd")
	}
}

func TestHandleConfigKey_MoveUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	_ = m2
	if cmd != nil {
		t.Error("move up should return nil cmd")
	}
}

func TestHandleConfigEditInput_TypeChar(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{Editing: true}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigEditInput(tea.KeyPressMsg{Code: 'a', Text: "a"})
	_ = m2
	if cmd != nil {
		t.Error("type char should return nil cmd")
	}
}

func TestHandleConfigEditInput_Backspace(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{Editing: true, EditBuf: "abc"}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigEditInput(tea.KeyPressMsg{Code: tea.KeyBackspace})
	_ = m2
	if cmd != nil {
		t.Error("backspace should return nil cmd")
	}
}

func TestHandleConfigEditInput_Escape(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{Editing: true, EditBuf: "test"}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigEditInput(tea.KeyPressMsg{Code: tea.KeyEsc})
	_ = m2
	if cmd != nil {
		t.Error("escape should return nil cmd")
	}
}

func TestHandleConfigEditInput_Enter(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	cfg := &views.ConfigEditor{Editing: true, EditBuf: "value"}
	m.ConfigEdit = cfg

	m2, cmd := m.handleConfigEditInput(tea.KeyPressMsg{Code: tea.KeyEnter})
	_ = m2
	if cmd != nil {
		t.Error("enter should return nil cmd")
	}
}

// --- handleLauncherKey tests ---

func TestHandleLauncherKey_Escape(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	m2, cmd := m.handleLauncherKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	_ = m2
	if cmd != nil {
		t.Error("escape should return nil cmd")
	}
}

func TestHandleLauncherKey_TypeChar(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	m2, cmd := m.handleLauncherKey(tea.KeyPressMsg{Code: 'a', Text: "a"})
	_ = m2
	if cmd != nil {
		t.Error("type char should return nil cmd")
	}
}

func TestHandleLauncherKey_Tab(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	m2, cmd := m.handleLauncherKey(tea.KeyPressMsg{Code: tea.KeyTab})
	_ = m2
	if cmd != nil {
		t.Error("tab should return nil cmd")
	}
}

func TestHandleLauncherKey_Backspace(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	m2, cmd := m.handleLauncherKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	_ = m2
	if cmd != nil {
		t.Error("backspace should return nil cmd")
	}
}

func TestHandleLauncherKey_UpDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = &components.SessionLauncher{}

	m2, cmd := m.handleLauncherKey(tea.KeyPressMsg{Code: tea.KeyDown})
	_ = m2
	if cmd != nil {
		t.Error("down should return nil cmd")
	}

	m3, cmd2 := m.handleLauncherKey(tea.KeyPressMsg{Code: tea.KeyUp})
	_ = m3
	if cmd2 != nil {
		t.Error("up should return nil cmd")
	}
}

// --- handleConfirmResult additional tests ---

func TestHandleConfirmResult_StopLoop(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopLoop",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopLoop",
		Result: components.ConfirmYes,
		Data:   -1, // invalid index
	}
	m2, _ := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after confirm")
	}
}

func TestHandleConfirmResult_StopSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopSession",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopSession",
		Result: components.ConfirmYes,
		Data:   "nonexistent-session-id",
	}
	m2, _ := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after confirm")
	}
}

func TestHandleConfirmResult_StopManagedLoop(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:  "Test",
		Action: "stopManagedLoop",
		Active: true,
	}

	result := components.ConfirmResultMsg{
		Action: "stopManagedLoop",
		Result: components.ConfirmYes,
		Data:   "loop-id",
	}
	m2, _ := m.handleConfirmResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("ConfirmDialog should be nil after confirm")
	}
}

// --- handleActionResult additional tests ---

func TestHandleActionResult_StartAll(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}

	result := components.ActionResultMsg{Action: "startAll"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_StartLoop_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "startLoop"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_PauseLoop_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "pauseLoop"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_ViewLogs_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "viewLogs"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_EditConfig_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "editConfig"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_LaunchSession_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "launchSession"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_ViewDiff_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "viewDiff"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_StopSession_NoSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.SessionID = ""

	result := components.ActionResultMsg{Action: "stopSession"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_RetrySession_NoSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.SessionID = ""

	result := components.ActionResultMsg{Action: "retrySession"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

func TestHandleActionResult_StreamOutput(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.SessionID = ""

	result := components.ActionResultMsg{Action: "streamOutput"}
	m2, cmd := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
	// No session selected — startOutputStreaming returns nil
	if cmd != nil {
		t.Error("streamOutput with no session should return nil cmd")
	}
}

func TestHandleActionResult_StopLoop_NoSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.RepoIdx = -1

	result := components.ActionResultMsg{Action: "stopLoop"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ActionMenu != nil {
		t.Error("ActionMenu should be nil")
	}
}

// --- handleActionResult with valid selection ---

func TestHandleActionResult_StopLoop_WithSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Repos = []*model.Repo{{Name: "test-repo", Path: "/tmp/test-repo"}}
	m.Sel.RepoIdx = 0

	result := components.ActionResultMsg{Action: "stopLoop"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog == nil {
		t.Error("stopLoop with selection should set ConfirmDialog")
	}
}

func TestHandleActionResult_ViewLogs_WithSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Repos = []*model.Repo{{Name: "test-repo", Path: "/tmp/test-repo"}}
	m.Sel.RepoIdx = 0

	result := components.ActionResultMsg{Action: "viewLogs"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Nav.CurrentView != ViewLogs {
		t.Errorf("viewLogs should push ViewLogs, got %v", got.Nav.CurrentView)
	}
}

func TestHandleActionResult_ViewDiff_WithSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Repos = []*model.Repo{{Name: "test-repo", Path: "/tmp/test-repo"}}
	m.Sel.RepoIdx = 0

	result := components.ActionResultMsg{Action: "viewDiff"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Nav.CurrentView != ViewDiff {
		t.Errorf("viewDiff should push ViewDiff, got %v", got.Nav.CurrentView)
	}
}

func TestHandleActionResult_LaunchSession_WithSelection(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Repos = []*model.Repo{{Name: "test-repo", Path: "/tmp/test-repo"}}
	m.Sel.RepoIdx = 0

	result := components.ActionResultMsg{Action: "launchSession"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.Launcher == nil {
		t.Error("launchSession should set Launcher")
	}
}

func TestHandleActionResult_StopSession_WithSession(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.ActionMenu = &components.ActionMenu{Active: true}
	m.Sel.SessionID = "sess-123"
	m.SessMgr = session.NewManager()

	result := components.ActionResultMsg{Action: "stopSession"}
	m2, _ := m.handleActionResult(result)
	got := m2.(Model)
	if got.Modals.ConfirmDialog == nil {
		t.Error("stopSession should set ConfirmDialog")
	}
}

// --- handleLaunchResult tests ---

func TestHandleLaunchResult_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Modals.Launcher = components.NewSessionLauncher("/tmp", "test")
	m.SessMgr = nil

	msg := components.LaunchResultMsg{Prompt: "do something", RepoPath: "/tmp", Provider: "claude"}
	m2, _ := m.handleLaunchResult(msg)
	got := m2.(Model)
	if got.Modals.Launcher != nil {
		t.Error("LaunchResult with no SessMgr should clear Launcher")
	}
}

// --- startOutputStreaming tests ---

func TestStartOutputStreaming_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Sel.SessionID = "sess-123"
	m.SessMgr = nil
	m2, c := m.startOutputStreaming()
	got := m2.(Model)
	if c != nil {
		t.Error("startOutputStreaming with no SessMgr should return nil cmd")
	}
	if got.Stream.Active {
		t.Error("should not set StreamingOutput with no SessMgr")
	}
}
