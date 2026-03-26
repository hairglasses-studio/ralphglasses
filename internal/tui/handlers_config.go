package tui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
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
}

// --- Config edit mode dispatch table ---

var configEditKeys = []ViewKeyEntry{
	{Binding: func(km *KeyMap) key.Binding { return km.Enter }, Handler: func(m *Model, _ tea.KeyMsg) (tea.Model, tea.Cmd) {
		m.ConfigEdit.ConfirmEdit()
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

// --- View-specific key handler methods ---

func (m Model) handleConfigKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.ConfigEdit == nil {
		return m, nil
	}
	return dispatchViewKeys(configKeys, &m, msg)
}

func (m Model) handleConfigEditInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	return dispatchViewKeys(configEditKeys, &m, msg)
}
