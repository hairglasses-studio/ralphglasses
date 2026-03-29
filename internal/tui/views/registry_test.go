package views

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// stubHandler is a minimal ViewHandler for testing the registry.
type stubHandler struct {
	rendered string
	w, h     int
}

func (s *stubHandler) Render(width, height int) string {
	return s.rendered
}

func (s *stubHandler) HandleKey(_ tea.KeyMsg) (bool, tea.Cmd) {
	return true, nil
}

func (s *stubHandler) SetDimensions(width, height int) {
	s.w = width
	s.h = height
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	r := NewRegistry()
	h := &stubHandler{rendered: "hello"}
	r.Register(42, h)

	got, ok := r.Get(42)
	if !ok {
		t.Fatal("expected handler to be registered")
	}
	if got != h {
		t.Fatal("expected same handler back")
	}

	_, ok = r.Get(99)
	if ok {
		t.Fatal("expected no handler for unregistered mode")
	}
}

func TestRegistry_Render(t *testing.T) {
	r := NewRegistry()
	r.Register(1, &stubHandler{rendered: "view-1"})

	if got := r.Render(1, 80, 24); got != "view-1" {
		t.Fatalf("expected 'view-1', got %q", got)
	}
	if got := r.Render(99, 80, 24); got != "" {
		t.Fatalf("expected empty for unregistered, got %q", got)
	}
}

func TestRegistry_HandleKey(t *testing.T) {
	r := NewRegistry()
	r.Register(1, &stubHandler{})

	handled, _ := r.HandleKey(1, tea.KeyMsg{})
	if !handled {
		t.Fatal("expected handled=true")
	}

	handled, _ = r.HandleKey(99, tea.KeyMsg{})
	if handled {
		t.Fatal("expected handled=false for unregistered mode")
	}
}
