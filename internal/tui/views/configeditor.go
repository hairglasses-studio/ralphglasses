package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ConfigEditor provides a simple form-based .ralphrc editor.
type ConfigEditor struct {
	Config  *model.RalphConfig
	Keys    []string
	Cursor  int
	Editing bool
	EditBuf string
	Dirty   bool
	Height  int
}

// NewConfigEditor creates an editor for the given config.
func NewConfigEditor(cfg *model.RalphConfig) *ConfigEditor {
	keys := make([]string, 0, len(cfg.Values))
	for k := range cfg.Values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return &ConfigEditor{
		Config: cfg,
		Keys:   keys,
	}
}

// MoveUp moves cursor up.
func (ce *ConfigEditor) MoveUp() {
	if !ce.Editing && ce.Cursor > 0 {
		ce.Cursor--
	}
}

// MoveDown moves cursor down.
func (ce *ConfigEditor) MoveDown() {
	if !ce.Editing && ce.Cursor < len(ce.Keys)-1 {
		ce.Cursor++
	}
}

// StartEdit enters edit mode for the current key.
func (ce *ConfigEditor) StartEdit() {
	if len(ce.Keys) == 0 {
		return
	}
	ce.Editing = true
	ce.EditBuf = ce.Config.Values[ce.Keys[ce.Cursor]]
}

// TypeChar appends a character in edit mode.
func (ce *ConfigEditor) TypeChar(ch rune) {
	if ce.Editing {
		ce.EditBuf += string(ch)
	}
}

// Backspace deletes the last char in edit mode.
func (ce *ConfigEditor) Backspace() {
	if ce.Editing && len(ce.EditBuf) > 0 {
		ce.EditBuf = ce.EditBuf[:len(ce.EditBuf)-1]
	}
}

// ConfirmEdit saves the edit.
func (ce *ConfigEditor) ConfirmEdit() {
	if !ce.Editing {
		return
	}
	key := ce.Keys[ce.Cursor]
	ce.Config.Values[key] = ce.EditBuf
	ce.Editing = false
	ce.Dirty = true
}

// CancelEdit discards the edit.
func (ce *ConfigEditor) CancelEdit() {
	ce.Editing = false
	ce.EditBuf = ""
}

// Save writes config to disk.
func (ce *ConfigEditor) Save() error {
	if err := ce.Config.Save(); err != nil {
		return err
	}
	ce.Dirty = false
	return nil
}

// View renders the config editor.
func (ce *ConfigEditor) View() string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render("Configuration Editor"))
	if ce.Dirty {
		b.WriteString(styles.StatusFailed.Render(" [modified]"))
	}
	b.WriteString("\n\n")

	for i, k := range ce.Keys {
		prefix := "  "
		if i == ce.Cursor {
			prefix = "▸ "
		}

		if ce.Editing && i == ce.Cursor {
			b.WriteString(fmt.Sprintf("%s%-28s %s%s\n",
				styles.SelectedStyle.Render(prefix),
				styles.SelectedStyle.Render(k),
				styles.CommandStyle.Render(ce.EditBuf),
				styles.CommandStyle.Render("█"),
			))
		} else {
			val := ce.Config.Values[k]
			if i == ce.Cursor {
				b.WriteString(fmt.Sprintf("%s%-28s %s\n",
					styles.SelectedStyle.Render(prefix),
					styles.SelectedStyle.Render(k),
					val,
				))
			} else {
				b.WriteString(fmt.Sprintf("%s%-28s %s\n", prefix, k, styles.InfoStyle.Render(val)))
			}
		}
	}

	if len(ce.Keys) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No configuration keys found"))
	}

	b.WriteString("\n")
	if ce.Editing {
		b.WriteString(styles.HelpStyle.Render("  Enter: save value  Esc: cancel"))
	} else {
		b.WriteString(styles.HelpStyle.Render("  Enter: edit  w: save to disk  Esc: back"))
	}

	return b.String()
}
