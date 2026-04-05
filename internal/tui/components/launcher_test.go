package components

import "testing"

func TestNewSessionLauncher(t *testing.T) {
	l := NewSessionLauncher("/path/to/repo", "myrepo")
	if !l.Active {
		t.Error("should be active")
	}
	if l.Fields[FieldProvider] != "codex" {
		t.Errorf("default provider = %q, want codex", l.Fields[FieldProvider])
	}
	if l.RepoName != "myrepo" {
		t.Errorf("repo name = %q", l.RepoName)
	}
}

func TestCycleProvider(t *testing.T) {
	l := NewSessionLauncher("", "")
	l.CycleProvider()
	if l.Fields[FieldProvider] != "gemini" {
		t.Errorf("after 1: %q", l.Fields[FieldProvider])
	}
	l.CycleProvider()
	if l.Fields[FieldProvider] != "claude" {
		t.Errorf("after 2: %q", l.Fields[FieldProvider])
	}
	l.CycleProvider()
	if l.Fields[FieldProvider] != "codex" {
		t.Errorf("after 3: %q", l.Fields[FieldProvider])
	}
}

func TestLauncher_Navigate(t *testing.T) {
	l := NewSessionLauncher("", "")
	l.HandleKey("down", 0)
	if l.Cursor != FieldPrompt {
		t.Errorf("cursor = %d, want FieldPrompt", l.Cursor)
	}
	l.HandleKey("up", 0)
	if l.Cursor != FieldProvider {
		t.Errorf("cursor = %d, want FieldProvider", l.Cursor)
	}
}

func TestLauncher_Edit(t *testing.T) {
	l := NewSessionLauncher("", "")
	l.Cursor = FieldPrompt
	l.HandleKey("rune", 'h')
	if !l.Editing {
		t.Error("should be editing")
	}
	l.HandleKey("rune", 'i')
	l.HandleKey("enter", 0)
	if l.Editing {
		t.Error("should stop editing on enter")
	}
	if l.Fields[FieldPrompt] != "hi" {
		t.Errorf("prompt = %q, want hi", l.Fields[FieldPrompt])
	}
}

func TestLauncher_Submit(t *testing.T) {
	l := NewSessionLauncher("/repo", "test")
	l.Fields[FieldPrompt] = "fix bug"

	result := l.Submit()
	if result.Provider != "codex" {
		t.Errorf("provider = %q", result.Provider)
	}
	if result.Prompt != "fix bug" {
		t.Errorf("prompt = %q", result.Prompt)
	}
	if result.RepoPath != "/repo" {
		t.Errorf("path = %q", result.RepoPath)
	}
}

func TestLauncher_Escape(t *testing.T) {
	l := NewSessionLauncher("", "")
	l.HandleKey("esc", 0)
	if l.Active {
		t.Error("should be inactive after escape")
	}
}

func TestLauncher_View(t *testing.T) {
	l := NewSessionLauncher("/repo", "test")
	l.Width = 60
	view := l.View()
	if view == "" {
		t.Error("expected non-empty view")
	}

	l.Active = false
	if l.View() != "" {
		t.Error("inactive launcher should render empty")
	}
}
