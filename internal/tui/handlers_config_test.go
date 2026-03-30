package tui

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

// helper: create a model with ConfigEdit initialized
func newConfigModel(values map[string]string) Model {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Nav.CurrentView = ViewConfigEditor
	m.Keys.SetViewContext(ViewConfigEditor)
	cfg := &model.RalphConfig{
		Path:   "/tmp/test/.ralphrc",
		Values: values,
	}
	m.ConfigEdit = views.NewConfigEditor(cfg)
	m.Width = 120
	m.Height = 40
	return m
}

// --- handleConfigKey ---

func TestConfigKey_NilConfigEdit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Ctx = context.Background()
	m.Nav.CurrentView = ViewConfigEditor
	m.ConfigEdit = nil

	m2, cmd := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd when ConfigEdit is nil")
	}
	if got.ConfigEdit != nil {
		t.Error("ConfigEdit should remain nil")
	}
}

func TestConfigKey_MoveDown(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "1",
		"beta":  "2",
	})

	m2, cmd := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	got := m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd for move down")
	}
	if got.ConfigEdit.Cursor != 1 {
		t.Errorf("Cursor = %d, want 1", got.ConfigEdit.Cursor)
	}
}

func TestConfigKey_MoveUp(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "1",
		"beta":  "2",
	})
	m.ConfigEdit.Cursor = 1

	m2, cmd := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	got := m2.(Model)
	if cmd != nil {
		t.Error("expected nil cmd for move up")
	}
	if got.ConfigEdit.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0", got.ConfigEdit.Cursor)
	}
}

func TestConfigKey_MoveUpAtTop(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "1",
	})
	m.ConfigEdit.Cursor = 0

	m2, _ := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	got := m2.(Model)
	if got.ConfigEdit.Cursor != 0 {
		t.Errorf("Cursor = %d, want 0 (should stay at top)", got.ConfigEdit.Cursor)
	}
}

func TestConfigKey_EnterStartsEdit(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "value1",
	})

	m2, _ := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if !got.ConfigEdit.Editing {
		t.Error("expected Editing to be true after Enter")
	}
	if got.ConfigEdit.EditBuf != "value1" {
		t.Errorf("EditBuf = %q, want %q", got.ConfigEdit.EditBuf, "value1")
	}
}

func TestConfigKey_WriteConfig_SaveError(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "1",
	})
	// Config.Path points to a non-writable location, so Save() will fail

	m2, _ := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("w")})
	got := m2.(Model)
	// Should show a notification (either save error or save success)
	if !got.Notify.Active() {
		t.Error("expected notification after write attempt")
	}
}

func TestConfigKey_UnmatchedKey(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "1",
	})

	m2, cmd := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("z")})
	_ = m2.(Model)
	if cmd != nil {
		t.Error("unmatched key should return nil cmd")
	}
}

// --- handleConfigEditInput ---

func TestConfigEditInput_EnterConfirmsEdit(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "old",
	})
	m.ConfigEdit.StartEdit()
	m.ConfigEdit.EditBuf = "new"

	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.ConfigEdit.Editing {
		t.Error("expected Editing to be false after confirm")
	}
	if got.ConfigEdit.Config.Values["alpha"] != "new" {
		t.Errorf("Value = %q, want %q", got.ConfigEdit.Config.Values["alpha"], "new")
	}
	if !got.ConfigEdit.Dirty {
		t.Error("expected Dirty to be true after edit")
	}
}

func TestConfigEditInput_EscapeCancelsEdit(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "original",
	})
	m.ConfigEdit.StartEdit()
	m.ConfigEdit.EditBuf = "changed"

	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyEscape})
	got := m2.(Model)
	if got.ConfigEdit.Editing {
		t.Error("expected Editing to be false after Escape")
	}
	// Value should remain unchanged
	if got.ConfigEdit.Config.Values["alpha"] != "original" {
		t.Errorf("Value = %q, want %q (unchanged)", got.ConfigEdit.Config.Values["alpha"], "original")
	}
}

func TestConfigEditInput_BackspaceDeletesChar(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "abc",
	})
	m.ConfigEdit.StartEdit()

	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyBackspace})
	got := m2.(Model)
	if got.ConfigEdit.EditBuf != "ab" {
		t.Errorf("EditBuf = %q, want %q", got.ConfigEdit.EditBuf, "ab")
	}
}

func TestConfigEditInput_TypeCharAppendsChar(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "val",
	})
	m.ConfigEdit.StartEdit()

	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	got := m2.(Model)
	if got.ConfigEdit.EditBuf != "valx" {
		t.Errorf("EditBuf = %q, want %q", got.ConfigEdit.EditBuf, "valx")
	}
}

func TestConfigEditInput_MultipleChars(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "",
	})
	m.ConfigEdit.StartEdit()

	// Type 'h'
	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = m2.(Model)
	// Type 'i'
	m2, _ = m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	got := m2.(Model)
	if got.ConfigEdit.EditBuf != "hi" {
		t.Errorf("EditBuf = %q, want %q", got.ConfigEdit.EditBuf, "hi")
	}
}

func TestConfigEditInput_BackspaceOnEmpty(t *testing.T) {
	m := newConfigModel(map[string]string{
		"alpha": "",
	})
	m.ConfigEdit.StartEdit()
	m.ConfigEdit.EditBuf = ""

	m2, _ := m.handleConfigEditInput(tea.KeyMsg{Type: tea.KeyBackspace})
	got := m2.(Model)
	if got.ConfigEdit.EditBuf != "" {
		t.Errorf("EditBuf = %q, want empty string", got.ConfigEdit.EditBuf)
	}
}

// --- configKeys dispatch table ---

func TestConfigKeys_EmptyConfigEnterNoOp(t *testing.T) {
	m := newConfigModel(map[string]string{})

	// Enter on empty config should not panic
	m2, cmd := m.handleConfigKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := m2.(Model)
	if got.ConfigEdit.Editing {
		t.Error("should not enter edit mode with empty config")
	}
	if cmd != nil {
		t.Error("expected nil cmd")
	}
}
