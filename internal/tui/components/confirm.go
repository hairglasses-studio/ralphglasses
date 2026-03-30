package components

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
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

// HandleMouse processes a mouse event for the confirm dialog.
// On left-click, it detects clicks on the Yes/No/Cancel buttons.
// Returns (confirmed result, handled). If handled is false, the click was not on a button.
func (d *ConfirmDialog) HandleMouse(x, y int, button, action int) (ConfirmResultMsg, bool) {
	if !d.Active {
		return ConfirmResultMsg{}, false
	}

	// Only handle left-click press events.
	if button != 1 || action != 0 {
		return ConfirmResultMsg{}, false
	}

	// The dialog layout (relative to the dialog box):
	// Row 0: Title
	// Row 1: blank
	// Row 2: Message
	// Row 3: blank
	// Row 4: Buttons — "  Yes  No  Cancel"
	// The button row is at y == 4 relative to the dialog start.
	// Since we cannot know the absolute position of the dialog,
	// we accept clicks on any Y and check X ranges for the buttons.
	// Buttons are rendered as: "  " + " Yes " + "  " + " No " + "  " + " Cancel "
	// Starting at x=2: Yes(5 chars), gap(2), No(4 chars), gap(2), Cancel(8 chars)

	// Button layout: "  " prefix (2 chars), then buttons with 2-char gaps
	// " Yes " = 5 chars, " No " = 4 chars, " Cancel " = 8 chars
	buttons := []struct {
		label  string
		width  int
		result ConfirmResult
	}{
		{"Yes", 5, ConfirmYes},     // " Yes " = 5
		{"No", 4, ConfirmNo},       // " No " = 4
		{"Cancel", 8, ConfirmCancel}, // " Cancel " = 8
	}

	pos := 2 // initial "  " prefix
	for _, btn := range buttons {
		if x >= pos && x < pos+btn.width {
			d.Active = false
			return ConfirmResultMsg{Action: d.Action, Result: btn.result, Data: d.Data}, true
		}
		pos += btn.width + 2 // 2-char gap between buttons
	}

	return ConfirmResultMsg{}, false
}

// --- Modal interface methods ---

// IsActive implements Modal.
func (d *ConfirmDialog) IsActive() bool { return d.Active }

// Deactivate implements Modal.
func (d *ConfirmDialog) Deactivate() { d.Active = false }

// ModalHandleKey implements Modal.HandleKey by adapting the existing HandleKey logic.
func (d *ConfirmDialog) ModalHandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	k := msg.Key()
	var keyType string
	switch k.Code {
	case tea.KeyLeft:
		keyType = "left"
	case tea.KeyRight:
		keyType = "right"
	case tea.KeyTab:
		keyType = "tab"
	case tea.KeyEnter:
		keyType = "enter"
	case tea.KeyEscape:
		keyType = "esc"
	default:
		if k.Text != "" {
			keyType = k.Text
		}
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
