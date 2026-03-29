package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ConfirmResult is the outcome of a confirm dialog.
type ConfirmResult int

const (
	ConfirmYes ConfirmResult = iota
	ConfirmNo
	ConfirmCancel
)

// ConfirmResultMsg is sent when a confirm dialog is resolved.
type ConfirmResultMsg struct {
	Action string
	Result ConfirmResult
	Data   any
}

// ConfirmDialog is a modal yes/no/cancel dialog.
type ConfirmDialog struct {
	Title    string
	Message  string
	Action   string // opaque action key for routing the result
	Data     any    // arbitrary data passed through to result
	Selected int    // 0=Yes, 1=No, 2=Cancel
	Active   bool
	Width    int
}

// Ensure ConfirmDialog satisfies Modal at compile time.
var _ Modal = (*ConfirmDialog)(nil)

// HandleKey processes a key press in the confirm dialog.
// Returns a result message and true if the dialog was dismissed.
// Supports: left/right/tab navigation, enter to confirm selection,
// y/Y for immediate yes, n/N or esc for immediate cancel.
func (d *ConfirmDialog) HandleKey(keyType string) (ConfirmResultMsg, bool) {
	switch keyType {
	case "left":
		if d.Selected > 0 {
			d.Selected--
		}
	case "right", "tab":
		if d.Selected < 2 {
			d.Selected++
		}
	case "enter":
		result := ConfirmResultMsg{Action: d.Action, Data: d.Data}
		switch d.Selected {
		case 0:
			result.Result = ConfirmYes
		case 1:
			result.Result = ConfirmNo
		case 2:
			result.Result = ConfirmCancel
		}
		d.Active = false
		return result, true
	case "y":
		d.Active = false
		return ConfirmResultMsg{Action: d.Action, Result: ConfirmYes, Data: d.Data}, true
	case "n":
		d.Active = false
		return ConfirmResultMsg{Action: d.Action, Result: ConfirmNo, Data: d.Data}, true
	case "esc":
		d.Active = false
		return ConfirmResultMsg{Action: d.Action, Result: ConfirmCancel, Data: d.Data}, true
	}
	return ConfirmResultMsg{}, false
}

// View renders the confirm dialog as a centered modal.
func (d *ConfirmDialog) View() string {
	if !d.Active {
		return ""
	}

	width := d.Width
	if width <= 0 {
		width = 50
	}
	innerWidth := width - 4

	var b strings.Builder

	// Title
	title := fmt.Sprintf(" %s ", d.Title)
	b.WriteString(styles.TitleStyle.Render(title))
	b.WriteString("\n\n")

	// Message
	b.WriteString(fmt.Sprintf("  %s\n\n", d.Message))

	// Buttons
	buttons := []string{"Yes", "No", "Cancel"}
	var rendered []string
	for i, btn := range buttons {
		label := fmt.Sprintf(" %s ", btn)
		if i == d.Selected {
			rendered = append(rendered, styles.SelectedStyle.Render(label))
		} else {
			rendered = append(rendered, styles.InfoStyle.Render(label))
		}
	}
	b.WriteString("  " + strings.Join(rendered, "  "))

	content := b.String()

	// Box it
	box := styles.StatBox.Width(innerWidth).Render(content)
	return box
}

// --- Modal interface methods ---

// IsActive implements Modal.
func (d *ConfirmDialog) IsActive() bool { return d.Active }

// Deactivate implements Modal.
func (d *ConfirmDialog) Deactivate() { d.Active = false }

// ModalHandleKey implements Modal.HandleKey by adapting the existing HandleKey logic.
func (d *ConfirmDialog) ModalHandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	keyType := msg.Type.String()
	if msg.Type == tea.KeyRunes {
		keyType = string(msg.Runes)
	}

	switch keyType {
	case "left", "right", "tab":
		d.HandleKey(keyType)
		return nil, true
	case "enter", "y", "n", "esc":
		result, dismissed := d.HandleKey(keyType)
		if dismissed {
			return func() tea.Msg { return result }, true
		}
		return nil, true
	}
	return nil, false
}

// ModalView implements Modal.View.
func (d *ConfirmDialog) ModalView(width, height int) string {
	return d.View()
}
