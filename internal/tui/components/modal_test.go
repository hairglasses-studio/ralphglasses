package components

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// mockModal is a test double for the Modal interface.
type mockModal struct {
	active      bool
	handleCalls int
	viewCalls   int
	deactivate  bool // if true, HandleKey deactivates the modal
	lastCmd     tea.Cmd
}

func (m *mockModal) IsActive() bool { return m.active }
func (m *mockModal) Deactivate()    { m.active = false }

func (m *mockModal) ModalHandleKey(msg tea.KeyMsg) (tea.Cmd, bool) {
	m.handleCalls++
	if m.deactivate {
		m.active = false
	}
	return m.lastCmd, true
}

func (m *mockModal) ModalView(width, height int) string {
	m.viewCalls++
	return "mock-view"
}

func TestModalStack_PushPop(t *testing.T) {
	var s ModalStack
	m1 := &mockModal{active: true}
	m2 := &mockModal{active: true}

	s.Push(m1)
	s.Push(m2)

	if s.Top() != m2 {
		t.Fatal("expected m2 on top after pushing two modals")
	}

	s.Pop()

	if s.Top() != m1 {
		t.Fatal("expected m1 on top after popping")
	}

	if s.Empty() {
		t.Fatal("stack should not be empty with one modal")
	}
}

func TestModalStack_HandleKey_Empty(t *testing.T) {
	var s ModalStack
	msg := tea.KeyMsg{Type: tea.KeyEnter}

	cmd, handled := s.HandleKey(msg)
	if cmd != nil {
		t.Fatal("expected nil cmd from empty stack")
	}
	if handled {
		t.Fatal("expected handled=false from empty stack")
	}
}

func TestModalStack_HandleKey_Delegates(t *testing.T) {
	var s ModalStack
	m := &mockModal{active: true}
	s.Push(m)

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, handled := s.HandleKey(msg)

	if !handled {
		t.Fatal("expected handled=true when modal is on stack")
	}
	if m.handleCalls != 1 {
		t.Fatalf("expected 1 HandleKey call, got %d", m.handleCalls)
	}
	// Modal is still active, should remain on stack.
	if s.Empty() {
		t.Fatal("stack should not be empty; modal is still active")
	}
}

func TestModalStack_AutoPop(t *testing.T) {
	var s ModalStack
	m := &mockModal{active: true, deactivate: true}
	s.Push(m)

	msg := tea.KeyMsg{Type: tea.KeyEnter}
	_, handled := s.HandleKey(msg)

	if !handled {
		t.Fatal("expected handled=true")
	}
	if !s.Empty() {
		t.Fatal("stack should be empty after modal deactivated itself")
	}
}

func TestModalStack_View_Empty(t *testing.T) {
	var s ModalStack
	view := s.View(80, 24)
	if view != "" {
		t.Fatalf("expected empty string from empty stack, got %q", view)
	}
}

func TestModalStack_Clear(t *testing.T) {
	var s ModalStack
	s.Push(&mockModal{active: true})
	s.Push(&mockModal{active: true})
	s.Push(&mockModal{active: true})

	s.Clear()

	if !s.Empty() {
		t.Fatal("stack should be empty after Clear")
	}
	if s.Top() != nil {
		t.Fatal("Top should return nil after Clear")
	}
}
