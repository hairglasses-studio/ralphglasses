package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// ---------------------------------------------------------------------------
// ActionMenu — Modal interface methods (0%)
// ---------------------------------------------------------------------------

func TestActionMenu_IsActive(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{Active: false}
	if m.IsActive() {
		t.Error("should be inactive")
	}
	m.Active = true
	if !m.IsActive() {
		t.Error("should be active")
	}
}

func TestActionMenu_Deactivate(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{Active: true}
	m.Deactivate()
	if m.Active {
		t.Error("should be inactive after Deactivate")
	}
}

func TestActionMenu_ModalHandleKey_Up(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "A", Action: "act-a"}, {Key: "b", Label: "B", Action: "act-b"}},
		Cursor: 1,
	}
	cmd, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if !handled {
		t.Error("up key should be handled")
	}
	if cmd != nil {
		t.Error("up key should not produce a cmd")
	}
	if m.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor)
	}
}

func TestActionMenu_ModalHandleKey_Down(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "A", Action: "act-a"}, {Key: "b", Label: "B", Action: "act-b"}},
		Cursor: 0,
	}
	cmd, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if !handled {
		t.Error("down key should be handled")
	}
	if cmd != nil {
		t.Error("down key should not produce a cmd")
	}
	if m.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.Cursor)
	}
}

func TestActionMenu_ModalHandleKey_Enter(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "A", Action: "act-a"}},
		Cursor: 0,
	}
	cmd, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Error("enter key should be handled")
	}
	if cmd == nil {
		t.Fatal("enter key should produce a cmd")
	}
	// Execute the cmd to get the ActionResultMsg.
	msg := cmd()
	result, ok := msg.(ActionResultMsg)
	if !ok {
		t.Fatalf("expected ActionResultMsg, got %T", msg)
	}
	if result.Action != "act-a" {
		t.Errorf("action = %q, want act-a", result.Action)
	}
}

func TestActionMenu_ModalHandleKey_Escape(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "A", Action: "act-a"}},
	}
	_, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Error("escape key should be handled")
	}
	if m.Active {
		t.Error("menu should be inactive after escape")
	}
}

func TestActionMenu_ModalHandleKey_Rune(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items: []ActionItem{
			{Key: "s", Label: "Start", Action: "start"},
			{Key: "x", Label: "Stop", Action: "stop"},
		},
	}
	cmd, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if !handled {
		t.Error("rune key should be handled")
	}
	if cmd == nil {
		t.Fatal("matching rune should produce a cmd")
	}
	msg := cmd()
	result := msg.(ActionResultMsg)
	if result.Action != "stop" {
		t.Errorf("action = %q, want stop", result.Action)
	}
}

func TestActionMenu_ModalHandleKey_UnknownKey(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "A", Action: "act"}},
	}
	_, handled := m.ModalHandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if handled {
		t.Error("tab key should not be handled by action menu")
	}
}

func TestActionMenu_ModalView(t *testing.T) {
	t.Parallel()
	m := &ActionMenu{
		Active: true,
		Title:  "Test Menu",
		Items:  []ActionItem{{Key: "a", Label: "Action A", Action: "act-a"}},
		Width:  40,
	}
	view := m.ModalView(80, 24)
	if view == "" {
		t.Error("ModalView should return non-empty string when active")
	}
}

// ---------------------------------------------------------------------------
// ConfirmDialog — Modal interface methods (0%)
// ---------------------------------------------------------------------------

func TestConfirmDialog_IsActive(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: false}
	if d.IsActive() {
		t.Error("should be inactive")
	}
	d.Active = true
	if !d.IsActive() {
		t.Error("should be active")
	}
}

func TestConfirmDialog_Deactivate(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true}
	d.Deactivate()
	if d.Active {
		t.Error("should be inactive after Deactivate")
	}
}

func TestConfirmDialog_ModalHandleKey_Left(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Selected: 1}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyLeft})
	if !handled {
		t.Error("left key should be handled")
	}
	if cmd != nil {
		t.Error("navigation should not produce a cmd")
	}
	if d.Selected != 0 {
		t.Errorf("selected = %d, want 0", d.Selected)
	}
}

func TestConfirmDialog_ModalHandleKey_Right(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Selected: 0}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRight})
	if !handled {
		t.Error("right key should be handled")
	}
	if cmd != nil {
		t.Error("navigation should not produce a cmd")
	}
	if d.Selected != 1 {
		t.Errorf("selected = %d, want 1", d.Selected)
	}
}

func TestConfirmDialog_ModalHandleKey_Tab(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Selected: 0}
	_, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyTab})
	if !handled {
		t.Error("tab key should be handled")
	}
	if d.Selected != 1 {
		t.Errorf("selected = %d, want 1 after tab", d.Selected)
	}
}

func TestConfirmDialog_ModalHandleKey_Enter(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Action: "delete", Selected: 0}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !handled {
		t.Error("enter key should be handled")
	}
	if cmd == nil {
		t.Fatal("enter key should produce a cmd")
	}
	msg := cmd()
	result := msg.(ConfirmResultMsg)
	if result.Result != ConfirmYes {
		t.Errorf("result = %d, want ConfirmYes", result.Result)
	}
	if result.Action != "delete" {
		t.Errorf("action = %q, want delete", result.Action)
	}
}

func TestConfirmDialog_ModalHandleKey_Y(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Action: "confirm"}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'y'}})
	if !handled {
		t.Error("y key should be handled")
	}
	if cmd == nil {
		t.Fatal("y key should produce a cmd")
	}
	msg := cmd()
	result := msg.(ConfirmResultMsg)
	if result.Result != ConfirmYes {
		t.Errorf("result = %d, want ConfirmYes", result.Result)
	}
}

func TestConfirmDialog_ModalHandleKey_N(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Action: "confirm"}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}})
	if !handled {
		t.Error("n key should be handled")
	}
	if cmd == nil {
		t.Fatal("n key should produce a cmd")
	}
	msg := cmd()
	result := msg.(ConfirmResultMsg)
	if result.Result != ConfirmNo {
		t.Errorf("result = %d, want ConfirmNo", result.Result)
	}
}

func TestConfirmDialog_ModalHandleKey_Escape(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true, Action: "confirm"}
	cmd, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Error("escape should be handled")
	}
	if cmd == nil {
		t.Fatal("escape should produce a cmd")
	}
	msg := cmd()
	result := msg.(ConfirmResultMsg)
	if result.Result != ConfirmCancel {
		t.Errorf("result = %d, want ConfirmCancel", result.Result)
	}
}

func TestConfirmDialog_ModalHandleKey_UnknownKey(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{Active: true}
	_, handled := d.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	if handled {
		t.Error("z key should not be handled by confirm dialog")
	}
}

func TestConfirmDialog_ModalView(t *testing.T) {
	t.Parallel()
	d := &ConfirmDialog{
		Active:  true,
		Title:   "Confirm",
		Message: "Are you sure?",
		Width:   50,
	}
	view := d.ModalView(80, 24)
	if view == "" {
		t.Error("ModalView should return non-empty string when active")
	}
}

// ---------------------------------------------------------------------------
// SessionLauncher — Modal interface methods (0%)
// ---------------------------------------------------------------------------

func TestSessionLauncher_IsActive(t *testing.T) {
	t.Parallel()
	l := &SessionLauncher{Active: false}
	if l.IsActive() {
		t.Error("should be inactive")
	}
	l.Active = true
	if !l.IsActive() {
		t.Error("should be active")
	}
}

func TestSessionLauncher_Deactivate(t *testing.T) {
	t.Parallel()
	l := &SessionLauncher{Active: true}
	l.Deactivate()
	if l.Active {
		t.Error("should be inactive after Deactivate")
	}
}

func TestSessionLauncher_ModalHandleKey_Escape(t *testing.T) {
	t.Parallel()
	l := NewSessionLauncher("/path", "repo")
	cmd, handled := l.ModalHandleKey(tea.KeyMsg{Type: tea.KeyEscape})
	if !handled {
		t.Error("escape should be handled")
	}
	if cmd != nil {
		t.Error("escape should not produce a submit cmd")
	}
	if l.Active {
		t.Error("launcher should be inactive after escape")
	}
}

func TestSessionLauncher_ModalHandleKey_UpDown(t *testing.T) {
	t.Parallel()
	l := NewSessionLauncher("/path", "repo")
	l.ModalHandleKey(tea.KeyMsg{Type: tea.KeyDown})
	if l.Cursor != FieldPrompt {
		t.Errorf("cursor after down = %d, want %d", l.Cursor, FieldPrompt)
	}
	l.ModalHandleKey(tea.KeyMsg{Type: tea.KeyUp})
	if l.Cursor != FieldProvider {
		t.Errorf("cursor after up = %d, want %d", l.Cursor, FieldProvider)
	}
}

func TestSessionLauncher_ModalHandleKey_Rune(t *testing.T) {
	t.Parallel()
	l := NewSessionLauncher("/path", "repo")
	l.Cursor = FieldPrompt
	l.ModalHandleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	if !l.Editing {
		t.Error("should enter editing mode on rune input")
	}
}

func TestSessionLauncher_ModalView(t *testing.T) {
	t.Parallel()
	l := NewSessionLauncher("/path", "repo")
	view := l.ModalView(80, 24)
	if view == "" {
		t.Error("ModalView should return non-empty string when active")
	}
}
