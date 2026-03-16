package tui

import "testing"

func TestFilterState_Type(t *testing.T) {
	f := FilterState{}
	f.Type('a')
	f.Type('b')
	f.Type('c')

	if f.Text != "abc" {
		t.Errorf("Text = %q, want %q", f.Text, "abc")
	}
}

func TestFilterState_Backspace(t *testing.T) {
	f := FilterState{Text: "hello"}
	f.Backspace()
	if f.Text != "hell" {
		t.Errorf("Text = %q, want %q", f.Text, "hell")
	}

	// Backspace on empty should not panic
	f = FilterState{}
	f.Backspace()
	if f.Text != "" {
		t.Errorf("Text = %q, want empty", f.Text)
	}
}

func TestFilterState_Clear(t *testing.T) {
	f := FilterState{Active: true, Text: "search"}
	f.Clear()

	if f.Active {
		t.Error("Active should be false after Clear")
	}
	if f.Text != "" {
		t.Errorf("Text = %q, want empty after Clear", f.Text)
	}
}

func TestFilterState_TypeBackspaceCombination(t *testing.T) {
	f := FilterState{}
	f.Type('x')
	f.Type('y')
	f.Backspace()
	f.Type('z')

	if f.Text != "xz" {
		t.Errorf("Text = %q, want %q", f.Text, "xz")
	}
}
