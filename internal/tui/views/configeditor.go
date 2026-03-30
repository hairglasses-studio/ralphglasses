package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// undoEntry stores a single-level undo snapshot.
type undoEntry struct {
	Key      string
	OldValue string
}

// ConfigEditor provides a simple form-based .ralphrc editor.
type ConfigEditor struct {
	Config  *model.RalphConfig
	Keys    []string
	Cursor  int
	Editing bool
	EditBuf string
	Dirty   bool
	Height  int
	undo    *undoEntry // single-level undo buffer
	filter  string     // search/filter substring (empty = show all)
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
	if len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
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

// ConfirmEdit saves the edit and stores the previous value for undo.
func (ce *ConfigEditor) ConfirmEdit() {
	if !ce.Editing || len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
		return
	}
	key := ce.Keys[ce.Cursor]
	oldValue := ce.Config.Values[key]
	ce.undo = &undoEntry{Key: key, OldValue: oldValue}
	ce.Config.Values[key] = ce.EditBuf
	ce.Editing = false
	ce.Dirty = true
}

// Undo reverts the last edit. Returns true if an undo was performed.
func (ce *ConfigEditor) Undo() bool {
	if ce.undo == nil || ce.Editing {
		return false
	}
	ce.Config.Values[ce.undo.Key] = ce.undo.OldValue
	ce.undo = nil
	ce.Dirty = true
	return true
}

// CancelEdit discards the edit.
func (ce *ConfigEditor) CancelEdit() {
	ce.Editing = false
	ce.EditBuf = ""
}

// AddKey inserts a new key-value pair into the config. It returns an error if
// the key already exists or is empty. The key must match [A-Z_][A-Z0-9_]*.
func (ce *ConfigEditor) AddKey(key, value string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("key must not be empty")
	}
	if _, exists := ce.Config.Values[key]; exists {
		return fmt.Errorf("key %q already exists", key)
	}
	ce.Config.Values[key] = value
	ce.rebuildKeys()
	ce.Dirty = true
	// Position cursor on the newly added key.
	for i, k := range ce.Keys {
		if k == key {
			ce.Cursor = i
			break
		}
	}
	return nil
}

// EditKey modifies the value of an existing key. It returns an error if the
// key does not exist.
func (ce *ConfigEditor) EditKey(key, newValue string) error {
	if _, exists := ce.Config.Values[key]; !exists {
		return fmt.Errorf("key %q not found", key)
	}
	oldValue := ce.Config.Values[key]
	ce.undo = &undoEntry{Key: key, OldValue: oldValue}
	ce.Config.Values[key] = newValue
	ce.Dirty = true
	return nil
}

// DeleteKey removes a key from the config. It returns an error if the key does
// not exist.
func (ce *ConfigEditor) DeleteKey(key string) error {
	oldValue, exists := ce.Config.Values[key]
	if !exists {
		return fmt.Errorf("key %q not found", key)
	}
	ce.undo = &undoEntry{Key: key, OldValue: oldValue}
	delete(ce.Config.Values, key)
	ce.rebuildKeys()
	ce.Dirty = true
	// Adjust cursor if it points past the end.
	if ce.Cursor >= len(ce.Keys) && ce.Cursor > 0 {
		ce.Cursor = len(ce.Keys) - 1
	}
	return nil
}

// SetFilter sets the search/filter substring. Only keys whose name or value
// contains the substring (case-insensitive) are shown. An empty string clears
// the filter.
func (ce *ConfigEditor) SetFilter(substr string) {
	ce.filter = substr
	ce.rebuildKeys()
	if ce.Cursor >= len(ce.Keys) {
		if len(ce.Keys) > 0 {
			ce.Cursor = len(ce.Keys) - 1
		} else {
			ce.Cursor = 0
		}
	}
}

// Filter returns the current filter string.
func (ce *ConfigEditor) Filter() string {
	return ce.filter
}

// rebuildKeys regenerates the sorted, filtered key list from Config.Values.
func (ce *ConfigEditor) rebuildKeys() {
	ce.Keys = ce.Keys[:0]
	lower := strings.ToLower(ce.filter)
	for k, v := range ce.Config.Values {
		if ce.filter == "" ||
			strings.Contains(strings.ToLower(k), lower) ||
			strings.Contains(strings.ToLower(v), lower) {
			ce.Keys = append(ce.Keys, k)
		}
	}
	sort.Strings(ce.Keys)
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
		b.WriteString(styles.HelpStyle.Render("  Enter: edit  u: undo  w: save to disk  Esc: back"))
	}

	return b.String()
}
