package components

import tea "charm.land/bubbletea/v2"

// Modal is the interface for overlay dialogs.
// Methods are prefixed with "Modal" to avoid collisions with existing
// HandleKey/View methods that have different signatures.
type Modal interface {
	IsActive() bool
	Deactivate()
	ModalHandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) // cmd, handled
	ModalView(width, height int) string
}

// ModalStack manages a stack of modal overlays.
type ModalStack struct {
	modals []Modal
}

// Push adds a modal to the top of the stack.
func (s *ModalStack) Push(m Modal) {
	s.modals = append(s.modals, m)
}

// Pop removes the top modal from the stack.
func (s *ModalStack) Pop() {
	if len(s.modals) > 0 {
		s.modals = s.modals[:len(s.modals)-1]
	}
}

// Top returns the top modal, or nil if the stack is empty.
func (s *ModalStack) Top() Modal {
	if len(s.modals) == 0 {
		return nil
	}
	return s.modals[len(s.modals)-1]
}

// Clear removes all modals from the stack.
func (s *ModalStack) Clear() {
	s.modals = nil
}

// Empty returns true if the stack has no modals.
func (s *ModalStack) Empty() bool {
	return len(s.modals) == 0
}

// HandleKey delegates key handling to the top modal.
// Returns nil, false if the stack is empty.
// If the top modal deactivates itself, it is automatically popped.
func (s *ModalStack) HandleKey(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	top := s.Top()
	if top == nil {
		return nil, false
	}
	cmd, handled := top.ModalHandleKey(msg)
	if !top.IsActive() {
		s.Pop()
	}
	return cmd, handled
}

// View renders the top modal. Returns "" if the stack is empty.
func (s *ModalStack) View(width, height int) string {
	top := s.Top()
	if top == nil {
		return ""
	}
	return top.ModalView(width, height)
}
