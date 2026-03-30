package views

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ---------------------------------------------------------------------------
// SearchView — NewSearchView, SetDimensions, Render (all 0%)
// ---------------------------------------------------------------------------

func TestSearchView_New(t *testing.T) {
	t.Parallel()
	sv := NewSearchView()
	if sv == nil {
		t.Fatal("NewSearchView returned nil")
	}
	if sv.Viewport == nil {
		t.Error("Viewport should be initialized")
	}
}

func TestSearchView_SetDimensions(t *testing.T) {
	t.Parallel()
	sv := NewSearchView()
	sv.SetDimensions(100, 50)
	if sv.width != 100 {
		t.Errorf("width = %d, want 100", sv.width)
	}
	if sv.height != 50 {
		t.Errorf("height = %d, want 50", sv.height)
	}
}

func TestSearchView_Render(t *testing.T) {
	t.Parallel()
	sv := NewSearchView()
	sv.SetDimensions(80, 24)
	// Render should not panic even with no content.
	_ = sv.Render()
}

// ---------------------------------------------------------------------------
// DiffViewport — SetData (0%)
// ---------------------------------------------------------------------------

func TestDiffViewport_SetData(t *testing.T) {
	t.Parallel()
	dv := NewDiffViewport()
	dv.SetDimensions(80, 24)
	// SetData with a non-git path should not panic.
	dv.SetData(t.TempDir(), "HEAD~1")
	if dv.repoPath == "" {
		t.Error("repoPath should be set")
	}
	if dv.fromRef != "HEAD~1" {
		t.Errorf("fromRef = %q, want HEAD~1", dv.fromRef)
	}
}

func TestDiffViewport_SetData_EmptyPath(t *testing.T) {
	t.Parallel()
	dv := NewDiffViewport()
	dv.SetDimensions(60, 20)
	// Should not panic with empty path.
	dv.SetData("", "")
}

// ---------------------------------------------------------------------------
// LoopDetailView — SetData (0%)
// ---------------------------------------------------------------------------

func TestLoopDetailView_SetData_NilLoop(t *testing.T) {
	t.Parallel()
	lv := NewLoopDetailView()
	lv.SetDimensions(80, 24)
	// Should not panic with nil loop.
	lv.SetData(nil)
}

func TestLoopDetailView_SetData_WithLoop(t *testing.T) {
	t.Parallel()
	lv := NewLoopDetailView()
	lv.SetDimensions(80, 24)
	loop := &session.LoopRun{
		ID:       "test-loop",
		RepoPath: t.TempDir(),
		Status:   "running",
	}
	lv.SetData(loop)
	if lv.loop != loop {
		t.Error("loop should be set")
	}
}

// ---------------------------------------------------------------------------
// ConfigEditor — Undo (0%)
// ---------------------------------------------------------------------------

func TestConfigEditor_Undo(t *testing.T) {
	t.Parallel()
	ce := NewConfigEditor(makeTestConfig(t))
	// Edit a value.
	ce.StartEdit()
	ce.EditBuf = "changed"
	ce.ConfirmEdit()
	if ce.Config.Values["A_KEY"] != "changed" {
		t.Fatal("precondition: value should be changed after edit")
	}

	// Undo it.
	ok := ce.Undo()
	if !ok {
		t.Error("Undo should return true when there's something to undo")
	}
	if ce.Config.Values["A_KEY"] != "val1" {
		t.Errorf("value after undo = %q, want val1", ce.Config.Values["A_KEY"])
	}
}

func TestConfigEditor_Undo_NoHistory(t *testing.T) {
	t.Parallel()
	ce := NewConfigEditor(makeTestConfig(t))
	ok := ce.Undo()
	if ok {
		t.Error("Undo should return false when nothing to undo")
	}
}

func TestConfigEditor_Undo_WhileEditing(t *testing.T) {
	t.Parallel()
	ce := NewConfigEditor(makeTestConfig(t))
	// Edit a value then start a new edit.
	ce.StartEdit()
	ce.EditBuf = "changed"
	ce.ConfirmEdit()

	ce.StartEdit() // now in editing mode
	ok := ce.Undo()
	if ok {
		t.Error("Undo should return false while in editing mode")
	}
}

func TestConfigEditor_Undo_OnlyOnce(t *testing.T) {
	t.Parallel()
	ce := NewConfigEditor(makeTestConfig(t))
	ce.StartEdit()
	ce.EditBuf = "changed"
	ce.ConfirmEdit()

	ce.Undo()
	// Second undo should be a no-op.
	ok := ce.Undo()
	if ok {
		t.Error("second Undo should return false")
	}
}
