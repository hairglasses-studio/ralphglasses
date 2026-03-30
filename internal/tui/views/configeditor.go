package views

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ConfigInputMode tracks the current CRUD input mode.
type ConfigInputMode int

const (
	// ConfigModeNormal is the default navigation mode.
	ConfigModeNormal ConfigInputMode = iota
	// ConfigModeEditValue is editing an existing value (Enter key).
	ConfigModeEditValue
	// ConfigModeInsertKey is prompting for a new key name.
	ConfigModeInsertKey
	// ConfigModeInsertValue is prompting for a new key's value.
	ConfigModeInsertValue
	// ConfigModeRenameKey is prompting for a new name for the selected key.
	ConfigModeRenameKey
	// ConfigModeConfirmDelete is awaiting y/n confirmation to delete.
	ConfigModeConfirmDelete
)

// undoOp identifies the type of undoable operation.
type undoOp int

const (
	undoOpEdit   undoOp = iota // value was changed
	undoOpDelete               // key was deleted
	undoOpRename               // key was renamed
	undoOpInsert               // key was inserted
)

// undoEntry stores a single-level undo snapshot.
type undoEntry struct {
	Op       undoOp
	Key      string
	OldKey   string // previous key name (for rename undo)
	OldValue string
}

// ConfigEditor provides a simple form-based .ralphrc editor.
type ConfigEditor struct {
	Config    *model.RalphConfig
	Keys      []string
	Cursor    int
	Editing   bool // kept for backward compat; true when InputMode == ConfigModeEditValue
	EditBuf   string
	Dirty     bool
	Height    int
	InputMode ConfigInputMode
	InsertKey string // key name being inserted (used during ConfigModeInsertValue)
	undo      *undoEntry
	filter    string // search/filter substring (empty = show all)
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

// InInputMode returns true if the editor is in any text input mode.
func (ce *ConfigEditor) InInputMode() bool {
	return ce.InputMode != ConfigModeNormal && ce.InputMode != ConfigModeConfirmDelete
}

// MoveUp moves cursor up.
func (ce *ConfigEditor) MoveUp() {
	if !ce.Editing && ce.InputMode == ConfigModeNormal && ce.Cursor > 0 {
		ce.Cursor--
	}
}

// MoveDown moves cursor down.
func (ce *ConfigEditor) MoveDown() {
	if !ce.Editing && ce.InputMode == ConfigModeNormal && ce.Cursor < len(ce.Keys)-1 {
		ce.Cursor++
	}
}

// StartEdit enters edit mode for the current key.
func (ce *ConfigEditor) StartEdit() {
	if len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
		return
	}
	ce.Editing = true
	ce.InputMode = ConfigModeEditValue
	ce.EditBuf = ce.Config.Values[ce.Keys[ce.Cursor]]
}

// TypeChar appends a character in any text input mode.
func (ce *ConfigEditor) TypeChar(ch rune) {
	if ce.InInputMode() {
		ce.EditBuf += string(ch)
	}
}

// Backspace deletes the last char in any text input mode.
func (ce *ConfigEditor) Backspace() {
	if ce.InInputMode() && len(ce.EditBuf) > 0 {
		ce.EditBuf = ce.EditBuf[:len(ce.EditBuf)-1]
	}
}

// ConfirmEdit saves the current input and stores undo state.
// It handles value editing, insert key/value, and rename key modes.
// Returns an error string if the operation fails (empty on success).
func (ce *ConfigEditor) ConfirmEdit() string {
	switch ce.InputMode {
	case ConfigModeEditValue:
		if len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
			ce.resetInput()
			return ""
		}
		key := ce.Keys[ce.Cursor]
		oldValue := ce.Config.Values[key]
		ce.undo = &undoEntry{Op: undoOpEdit, Key: key, OldValue: oldValue}
		ce.Config.Values[key] = ce.EditBuf
		ce.Dirty = true
		ce.resetInput()

	case ConfigModeInsertKey:
		name := strings.TrimSpace(ce.EditBuf)
		if name == "" {
			ce.resetInput()
			return "key name must not be empty"
		}
		if _, exists := ce.Config.Values[name]; exists {
			ce.resetInput()
			return fmt.Sprintf("key %q already exists", name)
		}
		// Advance to value prompt.
		ce.InsertKey = name
		ce.InputMode = ConfigModeInsertValue
		ce.EditBuf = ""
		return ""

	case ConfigModeInsertValue:
		value := ce.EditBuf
		ce.Config.Values[ce.InsertKey] = value
		ce.undo = &undoEntry{Op: undoOpInsert, Key: ce.InsertKey}
		ce.rebuildKeys()
		ce.Dirty = true
		// Position cursor on the newly added key.
		for i, k := range ce.Keys {
			if k == ce.InsertKey {
				ce.Cursor = i
				break
			}
		}
		ce.resetInput()

	case ConfigModeRenameKey:
		if len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
			ce.resetInput()
			return ""
		}
		newName := strings.TrimSpace(ce.EditBuf)
		if newName == "" {
			ce.resetInput()
			return "key name must not be empty"
		}
		oldName := ce.Keys[ce.Cursor]
		if newName == oldName {
			ce.resetInput()
			return ""
		}
		if _, exists := ce.Config.Values[newName]; exists {
			ce.resetInput()
			return fmt.Sprintf("key %q already exists", newName)
		}
		value := ce.Config.Values[oldName]
		delete(ce.Config.Values, oldName)
		ce.Config.Values[newName] = value
		ce.undo = &undoEntry{Op: undoOpRename, Key: newName, OldKey: oldName, OldValue: value}
		ce.rebuildKeys()
		ce.Dirty = true
		for i, k := range ce.Keys {
			if k == newName {
				ce.Cursor = i
				break
			}
		}
		ce.resetInput()

	default:
		ce.resetInput()
	}
	return ""
}

// Undo reverts the last operation. Returns true if an undo was performed.
func (ce *ConfigEditor) Undo() bool {
	if ce.undo == nil || ce.Editing || ce.InputMode != ConfigModeNormal {
		return false
	}
	switch ce.undo.Op {
	case undoOpEdit:
		ce.Config.Values[ce.undo.Key] = ce.undo.OldValue
	case undoOpDelete:
		// Re-insert deleted key.
		ce.Config.Values[ce.undo.Key] = ce.undo.OldValue
		ce.rebuildKeys()
		for i, k := range ce.Keys {
			if k == ce.undo.Key {
				ce.Cursor = i
				break
			}
		}
	case undoOpRename:
		// Reverse rename: delete new name, restore old name.
		delete(ce.Config.Values, ce.undo.Key)
		ce.Config.Values[ce.undo.OldKey] = ce.undo.OldValue
		ce.rebuildKeys()
		for i, k := range ce.Keys {
			if k == ce.undo.OldKey {
				ce.Cursor = i
				break
			}
		}
	case undoOpInsert:
		// Remove inserted key.
		delete(ce.Config.Values, ce.undo.Key)
		ce.rebuildKeys()
		if ce.Cursor >= len(ce.Keys) && ce.Cursor > 0 {
			ce.Cursor = len(ce.Keys) - 1
		}
	}
	ce.undo = nil
	ce.Dirty = true
	return true
}

// CancelEdit discards the current input and returns to normal mode.
func (ce *ConfigEditor) CancelEdit() {
	ce.resetInput()
}

// resetInput clears all input state back to normal mode.
func (ce *ConfigEditor) resetInput() {
	ce.Editing = false
	ce.InputMode = ConfigModeNormal
	ce.EditBuf = ""
	ce.InsertKey = ""
}

// StartInsert enters insert mode, prompting for a new key name.
func (ce *ConfigEditor) StartInsert() {
	if ce.InputMode != ConfigModeNormal {
		return
	}
	ce.InputMode = ConfigModeInsertKey
	ce.EditBuf = ""
	ce.InsertKey = ""
}

// StartRename enters rename mode for the selected key.
func (ce *ConfigEditor) StartRename() {
	if ce.InputMode != ConfigModeNormal || len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
		return
	}
	ce.InputMode = ConfigModeRenameKey
	ce.EditBuf = ce.Keys[ce.Cursor]
}

// StartDelete enters delete confirmation mode for the selected key.
func (ce *ConfigEditor) StartDelete() {
	if ce.InputMode != ConfigModeNormal || len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
		return
	}
	ce.InputMode = ConfigModeConfirmDelete
}

// ConfirmDelete executes the pending delete. Returns the deleted key name,
// or empty string if cancelled/failed.
func (ce *ConfigEditor) ConfirmDelete() string {
	if ce.InputMode != ConfigModeConfirmDelete || len(ce.Keys) == 0 || ce.Cursor >= len(ce.Keys) {
		ce.resetInput()
		return ""
	}
	key := ce.Keys[ce.Cursor]
	oldValue := ce.Config.Values[key]
	ce.undo = &undoEntry{Op: undoOpDelete, Key: key, OldValue: oldValue}
	delete(ce.Config.Values, key)
	ce.rebuildKeys()
	ce.Dirty = true
	if ce.Cursor >= len(ce.Keys) && ce.Cursor > 0 {
		ce.Cursor = len(ce.Keys) - 1
	}
	ce.resetInput()
	return key
}

// CancelDelete returns to normal mode without deleting.
func (ce *ConfigEditor) CancelDelete() {
	ce.resetInput()
}

// RenameKey renames oldKey to newKey, preserving its value. Returns an error if
// oldKey does not exist, newKey already exists, or newKey is empty.
func (ce *ConfigEditor) RenameKey(oldKey, newKey string) error {
	newKey = strings.TrimSpace(newKey)
	if newKey == "" {
		return fmt.Errorf("key must not be empty")
	}
	if oldKey == newKey {
		return nil
	}
	if _, exists := ce.Config.Values[oldKey]; !exists {
		return fmt.Errorf("key %q not found", oldKey)
	}
	if _, exists := ce.Config.Values[newKey]; exists {
		return fmt.Errorf("key %q already exists", newKey)
	}
	value := ce.Config.Values[oldKey]
	delete(ce.Config.Values, oldKey)
	ce.Config.Values[newKey] = value
	ce.undo = &undoEntry{Op: undoOpRename, Key: newKey, OldKey: oldKey, OldValue: value}
	ce.rebuildKeys()
	ce.Dirty = true
	for i, k := range ce.Keys {
		if k == newKey {
			ce.Cursor = i
			break
		}
	}
	return nil
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
	ce.undo = &undoEntry{Op: undoOpDelete, Key: key, OldValue: oldValue}
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

	// Render key-value rows.
	for i, k := range ce.Keys {
		prefix := "  "
		if i == ce.Cursor {
			prefix = "▸ "
		}

		switch {
		case ce.InputMode == ConfigModeEditValue && i == ce.Cursor:
			b.WriteString(fmt.Sprintf("%s%-28s %s%s\n",
				styles.SelectedStyle.Render(prefix),
				styles.SelectedStyle.Render(k),
				styles.CommandStyle.Render(ce.EditBuf),
				styles.CommandStyle.Render("█"),
			))
		case ce.InputMode == ConfigModeRenameKey && i == ce.Cursor:
			b.WriteString(fmt.Sprintf("%s%s%s %-24s %s\n",
				styles.SelectedStyle.Render(prefix),
				styles.CommandStyle.Render(ce.EditBuf),
				styles.CommandStyle.Render("█"),
				"",
				styles.InfoStyle.Render(ce.Config.Values[k]),
			))
		case ce.InputMode == ConfigModeConfirmDelete && i == ce.Cursor:
			b.WriteString(fmt.Sprintf("%s%-28s %s  %s\n",
				styles.StatusFailed.Render(prefix),
				styles.StatusFailed.Render(k),
				styles.InfoStyle.Render(ce.Config.Values[k]),
				styles.StatusFailed.Render("Delete? (y/n)"),
			))
		default:
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

	if len(ce.Keys) == 0 && ce.InputMode != ConfigModeInsertKey && ce.InputMode != ConfigModeInsertValue {
		b.WriteString(styles.InfoStyle.Render("  No configuration keys found"))
	}

	// Render input prompts for insert mode.
	if ce.InputMode == ConfigModeInsertKey {
		b.WriteString(fmt.Sprintf("\n  %s %s%s\n",
			styles.HeaderStyle.Render("New key:"),
			styles.CommandStyle.Render(ce.EditBuf),
			styles.CommandStyle.Render("█"),
		))
	} else if ce.InputMode == ConfigModeInsertValue {
		b.WriteString(fmt.Sprintf("\n  %s %s\n  %s %s%s\n",
			styles.HeaderStyle.Render("Key:"),
			styles.InfoStyle.Render(ce.InsertKey),
			styles.HeaderStyle.Render("Value:"),
			styles.CommandStyle.Render(ce.EditBuf),
			styles.CommandStyle.Render("█"),
		))
	}

	b.WriteString("\n")
	switch ce.InputMode {
	case ConfigModeEditValue:
		b.WriteString(styles.HelpStyle.Render("  Enter: save value  Esc: cancel"))
	case ConfigModeInsertKey:
		b.WriteString(styles.HelpStyle.Render("  Enter: set key name  Esc: cancel"))
	case ConfigModeInsertValue:
		b.WriteString(styles.HelpStyle.Render("  Enter: save new key  Esc: cancel"))
	case ConfigModeRenameKey:
		b.WriteString(styles.HelpStyle.Render("  Enter: confirm rename  Esc: cancel"))
	case ConfigModeConfirmDelete:
		b.WriteString(styles.HelpStyle.Render("  y: confirm delete  n/Esc: cancel"))
	default:
		b.WriteString(styles.HelpStyle.Render("  Enter: edit  i: insert  r: rename  d: delete  u: undo  w: save  Esc: back"))
	}

	return b.String()
}
