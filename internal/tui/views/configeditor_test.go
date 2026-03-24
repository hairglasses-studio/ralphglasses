package views

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
)

func makeTestConfig(t *testing.T) *model.RalphConfig {
	t.Helper()
	return &model.RalphConfig{
		Path:   filepath.Join(t.TempDir(), ".ralphrc"),
		Values: map[string]string{"A_KEY": "val1", "B_KEY": "val2", "C_KEY": "val3"},
	}
}

func TestNewConfigEditor(t *testing.T) {
	cfg := makeTestConfig(t)
	ce := NewConfigEditor(cfg)
	if len(ce.Keys) != 3 {
		t.Errorf("expected 3 keys, got %d", len(ce.Keys))
	}
	if ce.Keys[0] != "A_KEY" || ce.Keys[1] != "B_KEY" || ce.Keys[2] != "C_KEY" {
		t.Errorf("keys not sorted: %v", ce.Keys)
	}
}

func TestConfigEditorMoveUpDown(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.MoveDown()
	if ce.Cursor != 1 {
		t.Errorf("cursor after down = %d", ce.Cursor)
	}
	ce.MoveDown()
	ce.MoveDown() // past end
	if ce.Cursor != 2 {
		t.Errorf("cursor past end = %d", ce.Cursor)
	}
	ce.MoveUp()
	ce.MoveUp()
	ce.MoveUp() // past start
	if ce.Cursor != 0 {
		t.Errorf("cursor at top = %d", ce.Cursor)
	}
}

func TestConfigEditorEditCycle(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartEdit()
	if !ce.Editing {
		t.Error("should be in edit mode")
	}
	if ce.EditBuf != "val1" {
		t.Errorf("EditBuf = %q, want val1", ce.EditBuf)
	}
	ce.EditBuf = ""
	ce.TypeChar('n')
	ce.TypeChar('e')
	ce.TypeChar('w')
	ce.ConfirmEdit()
	if ce.Config.Values["A_KEY"] != "new" {
		t.Errorf("value not updated: %q", ce.Config.Values["A_KEY"])
	}
	if !ce.Dirty {
		t.Error("should be dirty after edit")
	}
}

func TestConfigEditorCancelEdit(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartEdit()
	ce.TypeChar('x')
	ce.CancelEdit()
	if ce.Editing {
		t.Error("should not be editing after cancel")
	}
	if ce.Config.Values["A_KEY"] != "val1" {
		t.Error("value should not change on cancel")
	}
}

func TestConfigEditorBackspace(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartEdit()
	ce.Backspace()
	if ce.EditBuf != "val" {
		t.Errorf("after backspace = %q", ce.EditBuf)
	}
}

func TestConfigEditorSave(t *testing.T) {
	cfg := makeTestConfig(t)
	_ = os.WriteFile(cfg.Path, []byte(""), 0644)
	ce := NewConfigEditor(cfg)
	ce.StartEdit()
	ce.EditBuf = "updated"
	ce.ConfirmEdit()
	if err := ce.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if ce.Dirty {
		t.Error("should not be dirty after save")
	}
}

func TestConfigEditorView(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	view := ce.View()
	if !strings.Contains(view, "Configuration Editor") {
		t.Error("view should contain title")
	}
	if !strings.Contains(view, "A_KEY") {
		t.Error("view should contain keys")
	}
}

func TestConfigEditorEmpty(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{}}
	ce := NewConfigEditor(cfg)
	view := ce.View()
	if !strings.Contains(view, "No configuration keys") {
		t.Error("empty editor should show 'No configuration keys'")
	}
	ce.StartEdit()
	if ce.Editing {
		t.Error("should not enter edit mode with no keys")
	}
}
