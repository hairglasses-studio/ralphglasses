package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ActionItem is a single menu item.
type ActionItem struct {
	Key         string // shortcut key (e.g. "s", "x")
	Label       string
	Action      string // opaque action identifier
	Destructive bool
}

// ActionResultMsg is sent when a menu item is selected.
type ActionResultMsg struct {
	Action string
}

// ActionMenu is a floating actions menu.
type ActionMenu struct {
	Title  string
	Items  []ActionItem
	Cursor int
	Active bool
	Width  int
}

// HandleKey processes a key press. Returns an action result and true if selected.
func (m *ActionMenu) HandleKey(keyType string, r rune) (ActionResultMsg, bool) {
	switch keyType {
	case "up":
		if m.Cursor > 0 {
			m.Cursor--
		}
	case "down":
		if m.Cursor < len(m.Items)-1 {
			m.Cursor++
		}
	case "enter":
		if m.Cursor < len(m.Items) {
			m.Active = false
			return ActionResultMsg{Action: m.Items[m.Cursor].Action}, true
		}
	case "esc":
		m.Active = false
		return ActionResultMsg{}, false
	case "rune":
		// Direct shortcut key
		for _, item := range m.Items {
			if len(item.Key) == 1 && rune(item.Key[0]) == r {
				m.Active = false
				return ActionResultMsg{Action: item.Action}, true
			}
		}
	}
	return ActionResultMsg{}, false
}

// View renders the action menu as a floating panel.
func (m *ActionMenu) View() string {
	if !m.Active || len(m.Items) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" %s ", m.Title)))
	b.WriteString("\n")

	for i, item := range m.Items {
		prefix := "  "
		if i == m.Cursor {
			prefix = "> "
		}

		label := fmt.Sprintf("%s[%s] %s", prefix, item.Key, item.Label)
		if i == m.Cursor {
			if item.Destructive {
				b.WriteString(styles.StatusFailed.Render(label))
			} else {
				b.WriteString(styles.SelectedStyle.Render(label))
			}
		} else {
			if item.Destructive {
				b.WriteString(styles.StatusFailed.Render(label))
			} else {
				b.WriteString(label)
			}
		}
		b.WriteString("\n")
	}

	width := m.Width
	if width <= 0 {
		width = 30
	}
	return styles.StatBox.Width(width).Render(b.String())
}

// Ensure ActionMenu satisfies Modal at compile time.
var _ Modal = (*ActionMenu)(nil)

// --- Modal interface methods ---

// IsActive implements Modal.
func (m *ActionMenu) IsActive() bool { return m.Active }

// Deactivate implements Modal.
func (m *ActionMenu) Deactivate() { m.Active = false }

// ModalHandleKey implements Modal.ModalHandleKey by adapting the existing HandleKey logic.
func (m *ActionMenu) ModalHandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	keyType := msg.Type.String()
	var r rune
	if msg.Type == tea.KeyRunes {
		keyType = "rune"
		if len(msg.Runes) > 0 {
			r = msg.Runes[0]
		}
	}

	switch keyType {
	case "up", "down":
		m.HandleKey(keyType, 0)
		return nil, true
	case "enter":
		result, selected := m.HandleKey(keyType, 0)
		if selected {
			return func() tea.Msg { return result }, true
		}
		return nil, true
	case "esc":
		m.HandleKey(keyType, 0)
		return nil, true
	case "rune":
		result, selected := m.HandleKey(keyType, r)
		if selected {
			return func() tea.Msg { return result }, true
		}
		return nil, true
	}
	return nil, false
}

// ModalView implements Modal.ModalView.
func (m *ActionMenu) ModalView(width, height int) string {
	return m.View()
}

// OverviewActions returns actions for the overview/repos tab.
func OverviewActions() []ActionItem {
	return []ActionItem{
		{Key: "r", Label: "Scan repos", Action: "scan"},
		{Key: "S", Label: "Start all loops", Action: "startAll"},
		{Key: "X", Label: "Stop all loops", Action: "stopAll", Destructive: true},
	}
}

// RepoDetailActions returns actions for the repo detail view.
func RepoDetailActions() []ActionItem {
	return []ActionItem{
		{Key: "S", Label: "Start loop", Action: "startLoop"},
		{Key: "X", Label: "Stop loop", Action: "stopLoop", Destructive: true},
		{Key: "P", Label: "Pause/resume", Action: "pauseLoop"},
		{Key: "l", Label: "View logs", Action: "viewLogs"},
		{Key: "e", Label: "Edit config", Action: "editConfig"},
		{Key: "L", Label: "Launch session", Action: "launchSession"},
		{Key: "d", Label: "View diff", Action: "viewDiff"},
	}
}

// SessionDetailActions returns actions for the session detail view.
func SessionDetailActions() []ActionItem {
	return []ActionItem{
		{Key: "X", Label: "Stop session", Action: "stopSession", Destructive: true},
		{Key: "R", Label: "Retry session", Action: "retrySession"},
		{Key: "o", Label: "Stream output", Action: "streamOutput"},
		{Key: "d", Label: "View diff", Action: "viewDiff"},
	}
}
