package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// handleMouse routes mouse events to the appropriate component.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	button := int(msg.Button)
	action := int(msg.Action)

	// 1. Modal overlays take priority (confirm dialog, action menu).
	if m.Modals.ConfirmDialog != nil && m.Modals.ConfirmDialog.Active {
		result, handled := m.Modals.ConfirmDialog.HandleMouse(msg.X, msg.Y, button, action)
		if handled {
			return m.handleConfirmResult(result)
		}
		return m, nil
	}
	if m.Modals.ActionMenu != nil && m.Modals.ActionMenu.Active {
		return m.handleActionMenuMouse(msg, button, action)
	}

	// 2. Tab bar — row 1 of the layout (after title bar at row 0).
	// The title bar takes row 0, tab bar is at row 1.
	const tabBarY = 1
	if msg.Y == tabBarY {
		tabIdx, clicked := m.TabBar.HandleMouse(msg.X, msg.Y-tabBarY, button, action)
		if clicked && tabIdx >= 0 && tabIdx < len(m.TabBar.Tabs) {
			tabViews := []struct {
				view ViewMode
				name string
			}{
				{ViewOverview, "Repos"},
				{ViewSessions, "Sessions"},
				{ViewTeams, "Teams"},
				{ViewFleet, "Fleet"},
			}
			if tabIdx < len(tabViews) {
				m.switchTab(tabIdx, tabViews[tabIdx].view, tabViews[tabIdx].name)
			}
			return m, nil
		}
	}

	// 3. Table views — content area starts at row 3 (title=0, tabbar=1, blank=2).
	const contentStartY = 3
	if msg.Y >= contentStartY {
		relY := msg.Y - contentStartY
		tbl := m.activeTable()
		if tbl != nil && tbl.HandleMouse(msg.X, relY, button, action) {
			return m, nil
		}
	}

	return m, nil
}

// handleActionMenuMouse processes mouse clicks on the action menu.
func (m Model) handleActionMenuMouse(msg tea.MouseMsg, button, action int) (tea.Model, tea.Cmd) {
	menu := m.Modals.ActionMenu
	if menu == nil || !menu.Active {
		return m, nil
	}

	// Only handle left-click press.
	if button != 1 || action != 0 {
		return m, nil
	}

	// Action menu items are rendered vertically.
	// Title is row 0, then each item is on subsequent rows.
	// We use a simple heuristic: if Y maps to an item index, select it.
	itemIdx := msg.Y - 1 // title is row 0
	if itemIdx >= 0 && itemIdx < len(menu.Items) {
		menu.Cursor = itemIdx
		menu.Active = false
		result := components.ActionResultMsg{Action: menu.Items[itemIdx].Action}
		return m.handleActionResult(result)
	}

	return m, nil
}
