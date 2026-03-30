package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
)

// handleMouse routes mouse events to the appropriate component.
func (m Model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	mouse := msg.Mouse()

	// button: 1=left click, 0=none; action: click=press, release=release
	// In v2 we use type assertions for click vs release.
	isClick := false
	isRelease := false
	switch msg.(type) {
	case tea.MouseClickMsg:
		isClick = true
	case tea.MouseReleaseMsg:
		isRelease = true
	}
	_ = isRelease

	button := int(mouse.Button)
	var action int
	if isClick {
		action = 0 // press
	} else {
		action = 1 // release / other
	}

	// 1. Modal overlays take priority (confirm dialog, action menu).
	if m.Modals.ConfirmDialog != nil && m.Modals.ConfirmDialog.Active {
		result, handled := m.Modals.ConfirmDialog.HandleMouse(mouse.X, mouse.Y, button, action)
		if handled {
			return m.handleConfirmResult(result)
		}
		return m, nil
	}
	if m.Modals.ActionMenu != nil && m.Modals.ActionMenu.Active {
		return m.handleActionMenuMouse(mouse, button, action)
	}

	// 2. Tab bar — row 1 of the layout (after title bar at row 0).
	// The title bar takes row 0, tab bar is at row 1.
	const tabBarY = 1
	if mouse.Y == tabBarY {
		tabIdx, clicked := m.TabBar.HandleMouse(mouse.X, mouse.Y-tabBarY, button, action)
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
	if mouse.Y >= contentStartY {
		relY := mouse.Y - contentStartY
		tbl := m.activeTable()
		if tbl != nil && tbl.HandleMouse(mouse.X, relY, button, action) {
			return m, nil
		}
	}

	return m, nil
}

// handleActionMenuMouse processes mouse clicks on the action menu.
func (m Model) handleActionMenuMouse(mouse tea.Mouse, button, action int) (tea.Model, tea.Cmd) {
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
	itemIdx := mouse.Y - 1 // title is row 0
	if itemIdx >= 0 && itemIdx < len(menu.Items) {
		menu.Cursor = itemIdx
		menu.Active = false
		result := components.ActionResultMsg{Action: menu.Items[itemIdx].Action}
		return m.handleActionResult(result)
	}

	return m, nil
}
