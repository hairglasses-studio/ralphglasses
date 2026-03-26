package components

import "testing"

func TestActionMenu_HandleKey_Navigate(t *testing.T) {
	m := &ActionMenu{
		Active: true,
		Items: []ActionItem{
			{Key: "a", Label: "First", Action: "first"},
			{Key: "b", Label: "Second", Action: "second"},
		},
	}

	m.HandleKey("down", 0)
	if m.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.Cursor)
	}
	m.HandleKey("down", 0) // past end
	if m.Cursor != 1 {
		t.Errorf("cursor = %d, want 1", m.Cursor)
	}
	m.HandleKey("up", 0)
	if m.Cursor != 0 {
		t.Errorf("cursor = %d, want 0", m.Cursor)
	}
}

func TestActionMenu_HandleKey_Enter(t *testing.T) {
	m := &ActionMenu{
		Active: true,
		Items:  []ActionItem{{Key: "a", Label: "Test", Action: "testAction"}},
	}

	result, selected := m.HandleKey("enter", 0)
	if !selected {
		t.Error("expected selection on enter")
	}
	if result.Action != "testAction" {
		t.Errorf("action = %q, want testAction", result.Action)
	}
}

func TestActionMenu_HandleKey_Shortcut(t *testing.T) {
	m := &ActionMenu{
		Active: true,
		Items: []ActionItem{
			{Key: "s", Label: "Start", Action: "start"},
			{Key: "x", Label: "Stop", Action: "stop"},
		},
	}

	result, selected := m.HandleKey("rune", 'x')
	if !selected {
		t.Error("expected selection on shortcut")
	}
	if result.Action != "stop" {
		t.Errorf("action = %q, want stop", result.Action)
	}
}

func TestActionMenu_HandleKey_Escape(t *testing.T) {
	m := &ActionMenu{Active: true, Items: []ActionItem{{Key: "a", Action: "a"}}}
	_, selected := m.HandleKey("esc", 0)
	if selected {
		t.Error("escape should not select")
	}
	if m.Active {
		t.Error("menu should be inactive after escape")
	}
}

func TestActionMenu_View(t *testing.T) {
	m := &ActionMenu{
		Active: true,
		Title:  "Actions",
		Items:  []ActionItem{{Key: "s", Label: "Start", Action: "start"}},
		Width:  30,
	}
	view := m.View()
	if view == "" {
		t.Error("expected non-empty view")
	}

	m.Active = false
	if m.View() != "" {
		t.Error("inactive menu should render empty")
	}
}

func TestOverviewActions(t *testing.T) {
	actions := OverviewActions()
	if len(actions) == 0 {
		t.Fatal("expected non-empty actions")
	}
	found := false
	for _, a := range actions {
		if a.Action == "scan" {
			found = true
		}
	}
	if !found {
		t.Error("expected scan action in overview actions")
	}
}

func TestRepoDetailActions(t *testing.T) {
	actions := RepoDetailActions()
	if len(actions) == 0 {
		t.Fatal("expected non-empty actions")
	}
	found := false
	for _, a := range actions {
		if a.Action == "startLoop" {
			found = true
		}
	}
	if !found {
		t.Error("expected startLoop action")
	}
}

func TestSessionDetailActions(t *testing.T) {
	actions := SessionDetailActions()
	if len(actions) == 0 {
		t.Fatal("expected non-empty actions")
	}
	found := false
	for _, a := range actions {
		if a.Action == "stopSession" {
			found = true
		}
	}
	if !found {
		t.Error("expected stopSession action")
	}
}
