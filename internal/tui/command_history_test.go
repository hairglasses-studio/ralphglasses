package tui

import "testing"

func TestCommandHistory_AddAndNavigate(t *testing.T) {
	h := &CommandHistory{maxLen: 100, cursor: -1}

	h.Add("start mesmer")
	h.Add("stop mesmer")
	h.Add("sessions")

	list := h.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}

	// Navigate backwards
	prev := h.Previous()
	if prev != "sessions" {
		t.Errorf("expected 'sessions', got %q", prev)
	}
	prev = h.Previous()
	if prev != "stop mesmer" {
		t.Errorf("expected 'stop mesmer', got %q", prev)
	}

	// Navigate forward
	next := h.Next()
	if next != "sessions" {
		t.Errorf("expected 'sessions', got %q", next)
	}
}

func TestCommandHistory_SkipConsecutiveDuplicates(t *testing.T) {
	h := &CommandHistory{maxLen: 100, cursor: -1}

	h.Add("scan")
	h.Add("scan")
	h.Add("scan")

	list := h.List()
	if len(list) != 1 {
		t.Errorf("expected 1 entry after dedup, got %d", len(list))
	}
}

func TestCommandHistory_MaxLen(t *testing.T) {
	h := &CommandHistory{maxLen: 3, cursor: -1}

	h.Add("a")
	h.Add("b")
	h.Add("c")
	h.Add("d")

	list := h.List()
	if len(list) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(list))
	}
	if list[0] != "b" {
		t.Errorf("expected 'b' as oldest, got %q", list[0])
	}
}

func TestCommandHistory_EmptyNavigation(t *testing.T) {
	h := &CommandHistory{maxLen: 100, cursor: -1}

	if prev := h.Previous(); prev != "" {
		t.Errorf("expected empty on empty history, got %q", prev)
	}
	if next := h.Next(); next != "" {
		t.Errorf("expected empty on empty history, got %q", next)
	}
}
