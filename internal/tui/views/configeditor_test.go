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

// --- CRUD operation tests ---

func TestConfigEditorStartInsert(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	if ce.InputMode != ConfigModeInsertKey {
		t.Errorf("InputMode = %d, want ConfigModeInsertKey", ce.InputMode)
	}
	if ce.EditBuf != "" {
		t.Errorf("EditBuf = %q, want empty", ce.EditBuf)
	}
}

func TestConfigEditorInsertKeyValueFlow(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()

	// Type key name
	ce.TypeChar('N')
	ce.TypeChar('E')
	ce.TypeChar('W')
	if ce.EditBuf != "NEW" {
		t.Fatalf("EditBuf = %q, want NEW", ce.EditBuf)
	}

	// Confirm key name -> moves to value prompt
	errMsg := ce.ConfirmEdit()
	if errMsg != "" {
		t.Fatalf("ConfirmEdit returned error: %s", errMsg)
	}
	if ce.InputMode != ConfigModeInsertValue {
		t.Fatalf("InputMode = %d, want ConfigModeInsertValue", ce.InputMode)
	}
	if ce.InsertKey != "NEW" {
		t.Fatalf("InsertKey = %q, want NEW", ce.InsertKey)
	}

	// Type value
	ce.TypeChar('v')
	ce.TypeChar('1')
	errMsg = ce.ConfirmEdit()
	if errMsg != "" {
		t.Fatalf("ConfirmEdit returned error: %s", errMsg)
	}

	// Verify key was added
	if ce.Config.Values["NEW"] != "v1" {
		t.Errorf("value = %q, want v1", ce.Config.Values["NEW"])
	}
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("InputMode = %d, want normal", ce.InputMode)
	}
	if !ce.Dirty {
		t.Error("should be dirty after insert")
	}
	// Cursor should be on the new key.
	if ce.Keys[ce.Cursor] != "NEW" {
		t.Errorf("cursor on %q, want NEW", ce.Keys[ce.Cursor])
	}
}

func TestConfigEditorInsertEmptyKey(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	// Confirm with empty key name
	errMsg := ce.ConfirmEdit()
	if errMsg == "" {
		t.Error("expected error for empty key name")
	}
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("should return to normal mode, got %d", ce.InputMode)
	}
}

func TestConfigEditorInsertDuplicateKey(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.EditBuf = "A_KEY"
	errMsg := ce.ConfirmEdit()
	if errMsg == "" {
		t.Error("expected error for duplicate key")
	}
}

func TestConfigEditorInsertCancel(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.TypeChar('X')
	ce.CancelEdit()
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("InputMode = %d, want normal", ce.InputMode)
	}
	if _, exists := ce.Config.Values["X"]; exists {
		t.Error("cancelled insert should not add key")
	}
}

func TestConfigEditorInsertCancelDuringValue(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.EditBuf = "NEW_KEY"
	ce.ConfirmEdit() // advance to value mode
	ce.CancelEdit()
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("InputMode = %d, want normal", ce.InputMode)
	}
	if _, exists := ce.Config.Values["NEW_KEY"]; exists {
		t.Error("cancelled insert should not add key")
	}
}

func TestConfigEditorUndoInsert(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.EditBuf = "INSERTED"
	ce.ConfirmEdit() // advance to value
	ce.EditBuf = "val"
	ce.ConfirmEdit() // confirm insert

	if _, exists := ce.Config.Values["INSERTED"]; !exists {
		t.Fatal("key should exist after insert")
	}

	if !ce.Undo() {
		t.Error("undo should succeed")
	}
	if _, exists := ce.Config.Values["INSERTED"]; exists {
		t.Error("key should be removed after undo")
	}
	if len(ce.Keys) != 3 {
		t.Errorf("keys = %d, want 3", len(ce.Keys))
	}
}

func TestConfigEditorStartRename(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	if ce.InputMode != ConfigModeRenameKey {
		t.Errorf("InputMode = %d, want ConfigModeRenameKey", ce.InputMode)
	}
	// Should pre-fill with current key name
	if ce.EditBuf != "A_KEY" {
		t.Errorf("EditBuf = %q, want A_KEY", ce.EditBuf)
	}
}

func TestConfigEditorRenameKeyFlow(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.EditBuf = "RENAMED"
	errMsg := ce.ConfirmEdit()
	if errMsg != "" {
		t.Fatalf("ConfirmEdit error: %s", errMsg)
	}

	// Old key gone, new key present with same value
	if _, exists := ce.Config.Values["A_KEY"]; exists {
		t.Error("old key should be removed")
	}
	if ce.Config.Values["RENAMED"] != "val1" {
		t.Errorf("value = %q, want val1", ce.Config.Values["RENAMED"])
	}
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("InputMode = %d, want normal", ce.InputMode)
	}
	if !ce.Dirty {
		t.Error("should be dirty after rename")
	}
}

func TestConfigEditorRenameToSameName(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	// Keep the same name
	errMsg := ce.ConfirmEdit()
	if errMsg != "" {
		t.Errorf("renaming to same name should succeed silently, got: %s", errMsg)
	}
	// Key should still exist unchanged
	if ce.Config.Values["A_KEY"] != "val1" {
		t.Error("value should be unchanged")
	}
}

func TestConfigEditorRenameToExisting(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.EditBuf = "B_KEY" // already exists
	errMsg := ce.ConfirmEdit()
	if errMsg == "" {
		t.Error("expected error renaming to existing key")
	}
}

func TestConfigEditorRenameToEmpty(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.EditBuf = ""
	errMsg := ce.ConfirmEdit()
	if errMsg == "" {
		t.Error("expected error renaming to empty")
	}
}

func TestConfigEditorRenameCancel(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.EditBuf = "RENAMED"
	ce.CancelEdit()
	if ce.InputMode != ConfigModeNormal {
		t.Error("should return to normal mode")
	}
	if _, exists := ce.Config.Values["A_KEY"]; !exists {
		t.Error("original key should remain after cancel")
	}
}

func TestConfigEditorUndoRename(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.EditBuf = "RENAMED"
	ce.ConfirmEdit()

	if !ce.Undo() {
		t.Error("undo should succeed")
	}
	if _, exists := ce.Config.Values["RENAMED"]; exists {
		t.Error("renamed key should be gone after undo")
	}
	if ce.Config.Values["A_KEY"] != "val1" {
		t.Errorf("original key value = %q, want val1", ce.Config.Values["A_KEY"])
	}
}

func TestConfigEditorRenameKeyMethod(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.RenameKey("A_KEY", "Z_KEY"); err != nil {
		t.Fatalf("RenameKey: %v", err)
	}
	if _, exists := ce.Config.Values["A_KEY"]; exists {
		t.Error("old key should not exist")
	}
	if ce.Config.Values["Z_KEY"] != "val1" {
		t.Errorf("value = %q, want val1", ce.Config.Values["Z_KEY"])
	}
	if !ce.Dirty {
		t.Error("should be dirty after rename")
	}
}

func TestConfigEditorRenameKeyMethodErrors(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if err := ce.RenameKey("MISSING", "X"); err == nil {
		t.Error("expected error renaming missing key")
	}
	if err := ce.RenameKey("A_KEY", "B_KEY"); err == nil {
		t.Error("expected error renaming to existing key")
	}
	if err := ce.RenameKey("A_KEY", ""); err == nil {
		t.Error("expected error renaming to empty key")
	}
	// Same name should be a no-op
	if err := ce.RenameKey("A_KEY", "A_KEY"); err != nil {
		t.Errorf("renaming to same name should not error: %v", err)
	}
}

func TestConfigEditorStartDelete(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartDelete()
	if ce.InputMode != ConfigModeConfirmDelete {
		t.Errorf("InputMode = %d, want ConfigModeConfirmDelete", ce.InputMode)
	}
}

func TestConfigEditorConfirmDelete(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartDelete()
	key := ce.ConfirmDelete()
	if key != "A_KEY" {
		t.Errorf("deleted key = %q, want A_KEY", key)
	}
	if _, exists := ce.Config.Values["A_KEY"]; exists {
		t.Error("key should be deleted")
	}
	if len(ce.Keys) != 2 {
		t.Errorf("keys = %d, want 2", len(ce.Keys))
	}
	if ce.InputMode != ConfigModeNormal {
		t.Errorf("InputMode = %d, want normal", ce.InputMode)
	}
	if !ce.Dirty {
		t.Error("should be dirty after delete")
	}
}

func TestConfigEditorCancelDelete(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartDelete()
	ce.CancelDelete()
	if ce.InputMode != ConfigModeNormal {
		t.Error("should return to normal mode")
	}
	if _, exists := ce.Config.Values["A_KEY"]; !exists {
		t.Error("key should still exist after cancel")
	}
}

func TestConfigEditorUndoDelete(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.MoveDown() // cursor on B_KEY
	ce.StartDelete()
	ce.ConfirmDelete()

	if _, exists := ce.Config.Values["B_KEY"]; exists {
		t.Fatal("key should be deleted")
	}

	if !ce.Undo() {
		t.Error("undo should succeed")
	}
	if ce.Config.Values["B_KEY"] != "val2" {
		t.Errorf("value after undo = %q, want val2", ce.Config.Values["B_KEY"])
	}
	if len(ce.Keys) != 3 {
		t.Errorf("keys = %d, want 3", len(ce.Keys))
	}
	// Cursor should move to restored key
	if ce.Keys[ce.Cursor] != "B_KEY" {
		t.Errorf("cursor on %q, want B_KEY", ce.Keys[ce.Cursor])
	}
}

func TestConfigEditorDeleteLastKeyCursorAdjust(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.MoveDown()
	ce.MoveDown() // cursor on C_KEY (last)
	ce.StartDelete()
	ce.ConfirmDelete()
	if ce.Cursor >= len(ce.Keys) {
		t.Errorf("cursor = %d, out of bounds (len=%d)", ce.Cursor, len(ce.Keys))
	}
}

func TestConfigEditorStartDeleteOnEmpty(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{}}
	ce := NewConfigEditor(cfg)
	ce.StartDelete()
	if ce.InputMode != ConfigModeNormal {
		t.Error("StartDelete on empty editor should be a no-op")
	}
}

func TestConfigEditorStartRenameOnEmpty(t *testing.T) {
	cfg := &model.RalphConfig{Values: map[string]string{}}
	ce := NewConfigEditor(cfg)
	ce.StartRename()
	if ce.InputMode != ConfigModeNormal {
		t.Error("StartRename on empty editor should be a no-op")
	}
}

func TestConfigEditorMoveBlockedDuringInput(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.MoveDown()
	if ce.Cursor != 0 {
		t.Error("cursor should not move during input mode")
	}
	ce.MoveUp()
	if ce.Cursor != 0 {
		t.Error("cursor should not move during input mode")
	}
}

func TestConfigEditorUndoBlockedDuringInput(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	// Create an undo-able state first
	ce.StartEdit()
	ce.EditBuf = "changed"
	ce.ConfirmEdit()
	// Enter insert mode
	ce.StartInsert()
	if ce.Undo() {
		t.Error("undo should be blocked during input mode")
	}
}

func TestConfigEditorInInputMode(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	if ce.InInputMode() {
		t.Error("normal mode should not be input mode")
	}
	ce.StartInsert()
	if !ce.InInputMode() {
		t.Error("insert key mode should be input mode")
	}
	ce.CancelEdit()

	ce.StartDelete()
	if ce.InInputMode() {
		t.Error("confirm delete mode should not be text input mode")
	}
	ce.CancelDelete()

	ce.StartRename()
	if !ce.InInputMode() {
		t.Error("rename mode should be input mode")
	}
}

func TestConfigEditorViewInsertMode(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	view := ce.View()
	if !strings.Contains(view, "New key:") {
		t.Error("view should show insert key prompt")
	}
	if !strings.Contains(view, "set key name") {
		t.Error("view should show insert key help")
	}
}

func TestConfigEditorViewInsertValueMode(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartInsert()
	ce.EditBuf = "TEST"
	ce.ConfirmEdit()
	view := ce.View()
	if !strings.Contains(view, "Value:") {
		t.Error("view should show value prompt")
	}
	if !strings.Contains(view, "TEST") {
		t.Error("view should show the key name")
	}
}

func TestConfigEditorViewRenameMode(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	view := ce.View()
	if !strings.Contains(view, "confirm rename") {
		t.Error("view should show rename help")
	}
}

func TestConfigEditorViewDeleteConfirmMode(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartDelete()
	view := ce.View()
	if !strings.Contains(view, "Delete? (y/n)") {
		t.Error("view should show delete confirmation")
	}
	if !strings.Contains(view, "confirm delete") {
		t.Error("view should show delete help")
	}
}

func TestConfigEditorViewNormalHelp(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	view := ce.View()
	if !strings.Contains(view, "i: insert") {
		t.Error("normal mode help should mention insert")
	}
	if !strings.Contains(view, "r: rename") {
		t.Error("normal mode help should mention rename")
	}
	if !strings.Contains(view, "d: delete") {
		t.Error("normal mode help should mention delete")
	}
	if !strings.Contains(view, "u: undo") {
		t.Error("normal mode help should mention undo")
	}
}

func TestConfigEditorStartInsertBlockedDuringInput(t *testing.T) {
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartRename()
	ce.StartInsert() // should be a no-op since already in rename mode
	if ce.InputMode != ConfigModeRenameKey {
		t.Errorf("InputMode = %d, want ConfigModeRenameKey (insert should be blocked)", ce.InputMode)
	}
}
