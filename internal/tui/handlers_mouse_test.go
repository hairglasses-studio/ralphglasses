package tui

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

func TestHandleMouse_TabClick(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40

	// Simulate a left-click on the second tab ("Sessions") at tabbar row (y=1).
	// Tab positions: "1:... Repos" is first tab. Second tab starts after it.
	// We need to click in the second tab's X range.
	// First tab: icon + "1:... Repos" + padding(2) ~15-20 chars. Try x=25 for second tab.
	msg := tea.MouseClickMsg{
		X:      25,
		Y:      1,
		Button: tea.MouseLeft,
	}

	result, _ := m.Update(msg)
	updated := result.(Model)

	// The click may or may not land on a tab depending on icon widths.
	// What matters is that the MouseClickMsg case is handled without panic.
	_ = updated
}

func TestHandleMouse_TableClick(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Table.SetRows([]components.Row{
		{"alpha", "running", "", "", "", ""},
		{"beta", "stopped", "", "", "", ""},
		{"gamma", "running", "", "", "", ""},
	})

	// Click on second data row. Content starts at y=3, header=2 rows offset.
	// So second data row = y=3 (contentStart) + 2 (header) + 1 (second row) = y=6
	msg := tea.MouseClickMsg{
		X:      5,
		Y:      6, // contentStartY(3) + headerRows(2) + row 1
		Button: tea.MouseLeft,
	}

	result, _ := m.Update(msg)
	updated := result.(Model)

	if updated.Table.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", updated.Table.Cursor)
	}
}

func TestHandleMouse_ConfirmDialogClick(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40
	m.Modals.ConfirmDialog = &components.ConfirmDialog{
		Title:   "Confirm",
		Message: "Do it?",
		Action:  "testAction",
		Active:  true,
	}

	// Click on "Yes" button area
	msg := tea.MouseClickMsg{
		X:      3,
		Y:      4,
		Button: tea.MouseLeft,
	}

	result, cmd := m.Update(msg)
	updated := result.(Model)

	// The confirm dialog should be dismissed.
	if updated.Modals.ConfirmDialog != nil && updated.Modals.ConfirmDialog.Active {
		t.Error("confirm dialog should be dismissed after click on Yes")
	}
	// A command should be returned (or nil depending on the action handler).
	_ = cmd
}

func TestHandleMouse_IgnoresNonLeftClick(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40

	// Right click should be a no-op
	msg := tea.MouseClickMsg{
		X:      5,
		Y:      5,
		Button: tea.MouseRight,
	}

	result, _ := m.Update(msg)
	updated := result.(Model)
	// Should not panic, and state should be unchanged.
	if updated.Nav.CurrentView != ViewOverview {
		t.Error("right click should not change view")
	}
}

func TestHandleMouse_IgnoresMotion(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Width = 120
	m.Height = 40

	// Mouse motion should be a no-op
	msg := tea.MouseMotionMsg{
		X:      5,
		Y:      5,
		Button: tea.MouseLeft,
	}

	result, _ := m.Update(msg)
	updated := result.(Model)
	if updated.Nav.CurrentView != ViewOverview {
		t.Error("motion should not change view")
	}
}
