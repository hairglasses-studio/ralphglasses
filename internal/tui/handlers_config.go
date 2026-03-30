package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// --- Config editor dispatch table ---

var configKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Down }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.MoveDown()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Up }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.MoveUp()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.StartEdit()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.WriteConfig }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if err := m.ConfigEdit.Save(); err != nil {
			m.Notify.Show(fmt.Sprintf("Save error: %v", err), 3*time.Second)
		} else {
			m.Notify.Show("Config saved", 2*time.Second)
		}
		return *m, nil
	}},
	// Insert new key (i or a)
	{Binding: func(km *KeyMap) key.Binding { return km.ConfigInsert }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.StartInsert()
		return *m, nil
	}},
	// Rename selected key (r)
	{Binding: func(km *KeyMap) key.Binding { return km.ConfigRename }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.StartRename()
		return *m, nil
	}},
	// Delete selected key (d or x)
	{Binding: func(km *KeyMap) key.Binding { return km.ConfigDelete }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.StartDelete()
		return *m, nil
	}},
	// Undo last operation (u)
	{Binding: func(km *KeyMap) key.Binding { return km.ConfigUndo }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if m.ConfigEdit.Undo() {
			m.Notify.Show("Undone", 2*time.Second)
		} else {
			m.Notify.Show("Nothing to undo", 2*time.Second)
		}
		return *m, nil
	}},
}

// --- Config edit mode dispatch table (value editing, insert, rename) ---

var configEditKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if errMsg := m.ConfigEdit.ConfirmEdit(); errMsg != "" {
			m.Notify.Show(errMsg, 3*time.Second)
		}
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Escape }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.CancelEdit()
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return msg.Type == tea.KeyBackspace }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.Backspace()
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool { return true }, Handler: func(m *Model, msg tea.KeyMsg) (tea.Model, tea.Cmd) {
		if len(msg.Runes) == 1 {
			m.ConfigEdit.TypeChar(msg.Runes[0])
		}
		return *m, nil
	}},
}

// --- Config delete confirmation dispatch table ---

var configDeleteKeys = []ViewKeyEntry{
	{Match: func(msg tea.KeyMsg) bool {
		return len(msg.Runes) == 1 && (msg.Runes[0] == 'y' || msg.Runes[0] == 'Y')
	}, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		if key := m.ConfigEdit.ConfirmDelete(); key != "" {
			m.Notify.Show(fmt.Sprintf("Deleted %s", key), 2*time.Second)
		}
		return *m, nil
	}},
	{Match: func(msg tea.KeyMsg) bool {
		return len(msg.Runes) == 1 && (msg.Runes[0] == 'n' || msg.Runes[0] == 'N')
	}, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.CancelDelete()
		return *m, nil
	}},
	{Binding: func(km *KeyMap) key.Binding { return km.Escape }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.CancelDelete()
		return *m, nil
	}},
}

// --- View-specific key handler methods ---

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	return dispatchViewKeys(configKeys, &m, msg)
}

func (m Model) handleConfigEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	// Route to appropriate dispatch table based on input mode.
	switch m.ConfigEdit.InputMode {
	case views.ConfigModeConfirmDelete:
		return dispatchViewKeys(configDeleteKeys, &m, msg)
	default:
		return dispatchViewKeys(configEditKeys, &m, msg)
	}
}
