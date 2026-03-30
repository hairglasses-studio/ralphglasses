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

func TestConfigEditorAddKey(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.AddKey("D_KEY", "val4"); err != nil {
		t.Fatalf("AddKey: %v", err)
	}
	if ce.Config.Values["D_KEY"] != "val4" {
		t.Errorf("value = %q, want val4", ce.Config.Values["D_KEY"])
	}
	if len(ce.Keys) != 4 {
		t.Errorf("keys len = %d, want 4", len(ce.Keys))
	}
	if !ce.Dirty {
		t.Error("should be dirty after add")
	}
	// Cursor should land on the new key.
	if ce.Keys[ce.Cursor] != "D_KEY" {
		t.Errorf("cursor on %q, want D_KEY", ce.Keys[ce.Cursor])
	}
}

func TestConfigEditorAddKeyDuplicate(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.AddKey("A_KEY", "dup"); err == nil {
		t.Error("expected error adding duplicate key")
	}
}

func TestConfigEditorAddKeyEmpty(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.AddKey("", "val"); err == nil {
		t.Error("expected error adding empty key")
	}
}

func TestConfigEditorEditKey(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.EditKey("B_KEY", "updated"); err != nil {
		t.Fatalf("EditKey: %v", err)
	}
	if ce.Config.Values["B_KEY"] != "updated" {
		t.Errorf("value = %q, want updated", ce.Config.Values["B_KEY"])
	}
	if !ce.Dirty {
		t.Error("should be dirty after edit")
	}
	// Undo should revert.
	if !ce.Undo() {
		t.Error("undo should succeed")
	}
	if ce.Config.Values["B_KEY"] != "val2" {
		t.Errorf("after undo value = %q, want val2", ce.Config.Values["B_KEY"])
	}
}

func TestConfigEditorEditKeyNotFound(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.EditKey("MISSING", "val"); err == nil {
		t.Error("expected error editing missing key")
	}
}

func TestConfigEditorDeleteKey(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.MoveDown() // cursor on B_KEY
	if err := ce.DeleteKey("B_KEY"); err != nil {
		t.Fatalf("DeleteKey: %v", err)
	}
	if _, exists := ce.Config.Values["B_KEY"]; exists {
		t.Error("B_KEY should be removed")
	}
	if len(ce.Keys) != 2 {
		t.Errorf("keys len = %d, want 2", len(ce.Keys))
	}
	if !ce.Dirty {
		t.Error("should be dirty after delete")
	}
}

func TestConfigEditorDeleteKeyNotFound(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.DeleteKey("MISSING"); err == nil {
		t.Error("expected error deleting missing key")
	}
}

func TestConfigEditorDeleteKeyCursorAdjust(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	// Move cursor to last key.
	ce.MoveDown()
	ce.MoveDown()
	if ce.Cursor != 2 {
		t.Fatalf("cursor = %d, want 2", ce.Cursor)
	}
	// Delete last key so cursor should adjust.
	_ = ce.DeleteKey("C_KEY")
	if ce.Cursor >= len(ce.Keys) {
		t.Errorf("cursor = %d, out of bounds (len=%d)", ce.Cursor, len(ce.Keys))
	}
}

func TestConfigEditorSetFilter(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.SetFilter("a_")
	if len(ce.Keys) != 1 {
		t.Errorf("filtered keys = %d, want 1", len(ce.Keys))
	}
	if ce.Keys[0] != "A_KEY" {
		t.Errorf("filtered key = %q, want A_KEY", ce.Keys[0])
	}
	// Clear filter.
	ce.SetFilter("")
	if len(ce.Keys) != 3 {
		t.Errorf("after clear keys = %d, want 3", len(ce.Keys))
	}
}

func TestConfigEditorSetFilterByValue(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.SetFilter("val3")
	if len(ce.Keys) != 1 {
		t.Errorf("filtered keys = %d, want 1", len(ce.Keys))
	}
	if ce.Keys[0] != "C_KEY" {
		t.Errorf("filtered key = %q, want C_KEY", ce.Keys[0])
	}
}

func TestConfigEditorSetFilterNoMatch(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.SetFilter("zzz_no_match")
	if len(ce.Keys) != 0 {
		t.Errorf("filtered keys = %d, want 0", len(ce.Keys))
	}
	if ce.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", ce.Cursor)
	}
}

func TestConfigEditorSetFilterCursorAdjust(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.MoveDown()
	ce.MoveDown() // cursor at index 2
	ce.SetFilter("a_")
	// Only 1 key matches, cursor should clamp.
	if ce.Cursor >= len(ce.Keys) {
		t.Errorf("cursor = %d out of range (len=%d)", ce.Cursor, len(ce.Keys))
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
