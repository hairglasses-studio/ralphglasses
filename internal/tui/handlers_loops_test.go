package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- Loop list key handlers ---

func TestLoopList_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.LoopListTable.SetRows([]components.Row{
		{"abc12345", "repo-a", "running", "5", "idle"},
		{"def67890", "repo-b", "stopped", "2", "idle"},
	})

	m2, _ := m.handleLoopListKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	got := m2.(Model)
	row := got.LoopListTable.SelectedRow()
	if row == nil {
		t.Fatal("expected selected row after move down")
	}
	if row[0] != "def67890" {
		t.Errorf("selected row = %q, want %q", row[0], "def67890")
	}
}

func TestLoopList_MoveUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.LoopListTable.SetRows([]components.Row{
		{"abc12345", "repo-a", "running", "5", "idle"},
		{"def67890", "repo-b", "stopped", "2", "idle"},
	})

	// Down then up
	m2, _ := m.handleLoopListKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = m2.(Model)
	m2, _ = m.handleLoopListKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	got := m2.(Model)
	row := got.LoopListTable.SelectedRow()
	if row == nil {
		t.Fatal("expected selected row after move up")
	}
	if row[0] != "abc12345" {
		t.Errorf("selected row = %q, want %q", row[0], "abc12345")
	}
}

func TestLoopList_Enter_EmptyTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	// No rows set

	m2, _ := m.handleLoopListKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewLoopList {
		t.Error("should stay in loop list with empty table")
	}
}

func TestLoopList_Enter_NilTable(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.LoopListTable = nil

	// Use the raw dispatch entry to test the nil check
	entries := loopListKeys
	// Find the enter handler (index 2)
	msg := tea.KeyPressMsg{Code: tea.KeyEnter}
	m.LoopListTable = nil
	// The Enter handler checks LoopListTable == nil
	// We need to manually test it since handleLoopListKey would dereference nil
	handler := entries[2].Handler
	m3 := m
	result, _ := handler(&m3, msg)
	_ = result // should not panic
}

func TestLoopList_Enter_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil
	m.LoopListTable.SetRows([]components.Row{
		{"abc12345", "repo-a", "running", "5", "idle"},
	})

	m2, _ := m.handleLoopListKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	got := m2.(Model)
	// No SessMgr, so can't resolve loop ID -> should stay
	if got.Nav.CurrentView != ViewLoopList {
		t.Errorf("should stay in loop list without SessMgr, got %v", got.Nav.CurrentView)
	}
}

// --- handleLoopListStart ---

func TestLoopListStart_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil
	m.LoopListTable.SetRows([]components.Row{
		{"abc12345", "repo-a", "running", "5", "idle"},
	})

	m2, _ := handleLoopListStart(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification about no session manager")
	}
}

func TestLoopListStart_NoRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil // doesn't matter, row check first
	// Empty table

	m2, _ := handleLoopListStart(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification for no session manager or no row")
	}
}

// --- handleLoopListStop ---

func TestLoopListStop_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil

	m2, _ := handleLoopListStop(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification about no session manager")
	}
}

func TestLoopListStop_NoRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil

	m2, _ := handleLoopListStop(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification for no row")
	}
}

// --- handleLoopListPause ---

func TestLoopListPause_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil

	m2, _ := handleLoopListPause(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification about no session manager")
	}
}

func TestLoopListPause_NoRow(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopList
	m.Keys.SetViewContext(ViewLoopList)
	m.SessMgr = nil

	m2, _ := handleLoopListPause(&m, tea.KeyPressMsg{})
	got := m2.(Model)
	if !got.Notify.Active() {
		t.Error("expected notification")
	}
}

// --- handleLoopDetailKey ---

func TestLoopDetail_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopDetail
	m.Keys.SetViewContext(ViewLoopDetail)
	m.SessMgr = nil
	m.Sel.LoopID = "some-loop"

	m2, cmd := m.handleLoopDetailKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with nil SessMgr")
	}
}

func TestLoopDetail_EmptySelectedLoop(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopDetail
	m.Keys.SetViewContext(ViewLoopDetail)
	m.Sel.LoopID = ""

	m2, cmd := m.handleLoopDetailKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with empty SelectedLoop")
	}
}

// --- handleLoopControlKey ---

func TestLoopControl_MoveDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
		{ID: "loop-2", Status: "stopped"},
	}
	m.LoopControlIdx = 0

	m2, _ := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	got := m2.(Model)
	if got.LoopControlIdx != 1 {
		t.Errorf("LoopControlIdx = %d, want 1", got.LoopControlIdx)
	}
}

func TestLoopControl_MoveDown_AtEnd(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
	}
	m.LoopControlIdx = 0

	m2, _ := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	got := m2.(Model)
	if got.LoopControlIdx != 0 {
		t.Errorf("LoopControlIdx = %d, want 0 (should not exceed list)", got.LoopControlIdx)
	}
}

func TestLoopControl_MoveUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
		{ID: "loop-2", Status: "stopped"},
	}
	m.LoopControlIdx = 1

	m2, _ := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	got := m2.(Model)
	if got.LoopControlIdx != 0 {
		t.Errorf("LoopControlIdx = %d, want 0", got.LoopControlIdx)
	}
}

func TestLoopControl_MoveUp_AtStart(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
	}
	m.LoopControlIdx = 0

	m2, _ := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	got := m2.(Model)
	if got.LoopControlIdx != 0 {
		t.Errorf("LoopControlIdx = %d, want 0", got.LoopControlIdx)
	}
}

func TestLoopControl_Step_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.SessMgr = nil
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
	}
	m.LoopControlIdx = 0

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with nil SessMgr")
	}
}

func TestLoopControl_Step_EmptyData(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = nil

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 's', Text: "s"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with empty data")
	}
}

func TestLoopControl_Toggle_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.SessMgr = nil
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
	}

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with nil SessMgr")
	}
}

func TestLoopControl_Toggle_EmptyData(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = nil

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'r', Text: "r"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with empty data")
	}
}

func TestLoopControl_Pause_NoSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.SessMgr = nil
	m.LoopControlData = []views.LoopControlData{
		{ID: "loop-1", Status: "running"},
	}

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with nil SessMgr")
	}
}

func TestLoopControl_Pause_EmptyData(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLoopControl
	m.Keys.SetViewContext(ViewLoopControl)
	m.LoopControlData = nil

	m2, cmd := m.handleLoopControlKey(tea.KeyPressMsg{Code: 'p', Text: "p"})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd with empty data")
	}
}

// --- loopListCmd ---

func TestLoopListCmd_NilSessMgr(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.SessMgr = nil
	cmd := m.loopListCmd()
	if cmd == nil {
		t.Fatal("expected non-nil cmd")
	}
	msg := cmd()
	if loops, ok := msg.(LoopListMsg); !ok {
		t.Errorf("expected LoopListMsg, got %T", msg)
	} else if loops != nil {
		t.Error("expected nil loops from nil SessMgr")
	}
}
