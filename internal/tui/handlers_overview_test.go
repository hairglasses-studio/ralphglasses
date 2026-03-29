package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// helper: create a model with repos and table rows populated
func newOverviewModel(repos ...*model.Repo) Model {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Nav.CurrentView = ViewOverview
	m.Keys.SetViewContext(ViewOverview)
	m.Repos = repos
	// Populate table rows matching repo names (column 0 = name)
	rows := make([]components.Row, len(repos))
	for i, r := range repos {
		rows[i] = components.Row{r.Name, "idle", r.Path}
	}
	m.Table.SetRows(rows)
	return m
}

// --- Table navigation ---

func TestOverview_MoveDown(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
		&model.Repo{Name: "beta", Path: "/tmp/beta"},
	)

	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := m2.(Model)
	row := got.Table.SelectedRow()
	if row == nil {
		t.Fatal("expected a selected row after MoveDown")
	}
	if row[0] != "beta" {
		t.Errorf("selected row = %q, want %q", row[0], "beta")
	}
}

func TestOverview_MoveUp(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
		&model.Repo{Name: "beta", Path: "/tmp/beta"},
	)
	// Move down first, then up
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = m2.(Model)
	m2, _ = m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	got := m2.(Model)
	row := got.Table.SelectedRow()
	if row == nil {
		t.Fatal("expected a selected row after MoveUp")
	}
	if row[0] != "alpha" {
		t.Errorf("selected row = %q, want %q", row[0], "alpha")
	}
}

func TestOverview_MoveDown_Arrow(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
		&model.Repo{Name: "beta", Path: "/tmp/beta"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyDown})
	got := m2.(Model)
	row := got.Table.SelectedRow()
	if row == nil {
		t.Fatal("expected a selected row after arrow down")
	}
	if row[0] != "beta" {
		t.Errorf("selected row = %q, want %q", row[0], "beta")
	}
}

// --- Enter key: push detail view ---

func TestOverview_EnterPushesDetailView(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "myrepo", Path: "/tmp/myrepo"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.Nav.CurrentView != ViewRepoDetail {
		t.Errorf("CurrentView = %v, want ViewRepoDetail", got.Nav.CurrentView)
	}
	if got.Sel.RepoIdx != 0 {
		t.Errorf("SelectedIdx = %d, want 0", got.Sel.RepoIdx)
	}
	if len(got.Nav.ViewStack) != 1 {
		t.Errorf("ViewStack len = %d, want 1", len(got.Nav.ViewStack))
	}
}

func TestOverview_EnterEmptyTable(t *testing.T) {
	m := newOverviewModel() // no repos
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	// Should stay in overview since no row selected
	if got.Nav.CurrentView != ViewOverview {
		t.Errorf("CurrentView = %v, want ViewOverview (no rows)", got.Nav.CurrentView)
	}
}

// --- Sort key ---

func TestOverview_SortKey(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
		&model.Repo{Name: "beta", Path: "/tmp/beta"},
	)
	// Just verify it doesn't panic
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	_ = m2.(Model)
}

// --- Space key: toggle selection ---

func TestOverview_SpaceToggleSelect(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
		&model.Repo{Name: "beta", Path: "/tmp/beta"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	_ = m2.(Model)
	// No crash = success
}

// --- Actions menu ---

func TestOverview_ActionsMenu(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	got := m2.(Model)
	if got.Modals.ActionMenu == nil {
		t.Error("expected ActionMenu to be set after 'a'")
	}
	if got.Modals.ActionMenu != nil && !got.Modals.ActionMenu.Active {
		t.Error("ActionMenu should be active")
	}
}

// --- Start loop key ---

func TestOverview_StartLoop(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	// 'S' starts loop for selected row
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	got := m2.(Model)
	// Should show a notification (start error since no process exists, or started)
	if !got.Notify.Active() {
		t.Error("expected notification after start loop")
	}
}

func TestOverview_StartLoop_EmptyTable(t *testing.T) {
	m := newOverviewModel() // no repos
	m2, cmd := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("S")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("start loop on empty table should return nil cmd")
	}
}

// --- Stop loop key ---

func TestOverview_StopLoop(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	got := m2.(Model)
	// Should show confirm dialog since there's a selected row
	if got.Modals.ConfirmDialog == nil {
		t.Error("expected ConfirmDialog after stop key")
	}
	if got.Modals.ConfirmDialog != nil && got.Modals.ConfirmDialog.Action != "stopLoop" {
		t.Errorf("ConfirmDialog.Action = %q, want %q", got.Modals.ConfirmDialog.Action, "stopLoop")
	}
}

func TestOverview_StopLoop_EmptyTable(t *testing.T) {
	m := newOverviewModel() // no repos
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("X")})
	got := m2.(Model)
	// No row selected, so no dialog
	if got.Modals.ConfirmDialog != nil {
		t.Error("should not show confirm dialog with no row selected")
	}
}

// --- Pause key ---

func TestOverview_PauseLoop(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	m2, _ := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	got := m2.(Model)
	// Should produce a notification (pause error since no process)
	if !got.Notify.Active() {
		t.Error("expected notification after pause")
	}
}

func TestOverview_PauseLoop_EmptyTable(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("P")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("pause on empty table should return nil cmd")
	}
}

// --- No-match key ---

func TestOverview_UnmatchedKey(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	m2, cmd := m.handleOverviewKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("unmatched key should return nil cmd")
	}
}

// --- startLoop / stopLoop / togglePause edge cases ---

func TestStartLoop_InvalidIndex(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.startLoop(-1)
	_ = m2.(Model)
	if cmd != nil {
		t.Error("startLoop(-1) should return nil cmd")
	}

	m2, cmd = m.startLoop(999)
	_ = m2.(Model)
	if cmd != nil {
		t.Error("startLoop(999) should return nil cmd")
	}
}

func TestStopLoop_InvalidIndex(t *testing.T) {
	m := newOverviewModel()
	m.Ctx = context.Background()
	m2, cmd := m.stopLoop(-1)
	_ = m2.(Model)
	if cmd != nil {
		t.Error("stopLoop(-1) should return nil cmd")
	}
}

func TestTogglePause_InvalidIndex(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.togglePause(-1)
	_ = m2.(Model)
	if cmd != nil {
		t.Error("togglePause(-1) should return nil cmd")
	}

	m2, cmd = m.togglePause(999)
	_ = m2.(Model)
	if cmd != nil {
		t.Error("togglePause(999) should return nil cmd")
	}
}

// --- startSelectedLoop / stopSelectedLoop / confirmStopSelectedLoop ---

func TestStartSelectedLoop_NoRow(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.startSelectedLoop()
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd when no row selected")
	}
}

func TestStopSelectedLoop_NoRow(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.stopSelectedLoop()
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd when no row selected")
	}
}

func TestConfirmStopSelectedLoop_NoRow(t *testing.T) {
	m := newOverviewModel()
	m2, _ := m.confirmStopSelectedLoop()
	got := m2.(Model)
	if got.Modals.ConfirmDialog != nil {
		t.Error("should not show confirm dialog when no row selected")
	}
}

func TestTogglePauseSelected_NoRow(t *testing.T) {
	m := newOverviewModel()
	m2, cmd := m.togglePauseSelected()
	_ = m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd when no row selected")
	}
}

func TestConfirmStopSelectedLoop_WithRow(t *testing.T) {
	m := newOverviewModel(
		&model.Repo{Name: "alpha", Path: "/tmp/alpha"},
	)
	m2, _ := m.confirmStopSelectedLoop()
	got := m2.(Model)
	if got.Modals.ConfirmDialog == nil {
		t.Error("expected confirm dialog with selected row")
	}
}

// --- Log view key handlers ---

func TestLogView_ScrollDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	_ = m2.(Model) // should not panic
}

func TestLogView_ScrollUp(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	_ = m2.(Model)
}

func TestLogView_GotoEnd(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	_ = m2.(Model)
}

func TestLogView_GotoStart(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	_ = m2.(Model)
}

func TestLogView_FollowToggle(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	_ = m2.(Model)
}

func TestLogView_PageUpDown(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Nav.CurrentView = ViewLogs
	m.Keys.SetViewContext(ViewLogs)
	m.LogView.SetLines([]string{"line1", "line2", "line3"})
	m.LogView.SetDimensions(80, 24)

	m2, _ := m.handleLogKey(tea.KeyMsg{Type: tea.KeyCtrlD})
	_ = m2.(Model)
	m2, _ = m.handleLogKey(tea.KeyMsg{Type: tea.KeyCtrlU})
	_ = m2.(Model)
}
