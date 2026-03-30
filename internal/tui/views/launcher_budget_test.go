package views

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

var testModels = []string{"claude-sonnet-4-20250514", "gemini-2.5-pro", "o3"}

func TestLauncherBudgetModel_Init(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)
	cmd := m.Init()
	if cmd != nil {
		t.Error("Init should return nil cmd")
	}
	if m.TotalBudget() != 10.0 {
		t.Errorf("total budget = %f, want 10.0", m.TotalBudget())
	}
	if m.SessionLimit() != 5.0 {
		t.Errorf("session limit = %f, want 5.0", m.SessionLimit())
	}
	if m.SelectedModel() != "claude-sonnet-4-20250514" {
		t.Errorf("selected model = %q, want claude-sonnet-4-20250514", m.SelectedModel())
	}
	if m.Focused() != 0 {
		t.Errorf("focused = %d, want 0", m.Focused())
	}
}

func TestLauncherBudgetModel_View(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)
	view := m.View().Content

	checks := []string{
		"Budget",
		"Session Limit",
		"Model",
		"$10.00",
		"$5.00",
		"Tab",
		"Enter",
		"Esc",
	}
	for _, want := range checks {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q", want)
		}
	}
}

func TestLauncherBudgetModel_TabCycles(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)

	if m.Focused() != fieldTotalBudget {
		t.Fatalf("initial focus = %d, want %d", m.Focused(), fieldTotalBudget)
	}

	// Tab through all fields and back to start.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focused() != fieldSessionLimit {
		t.Errorf("after 1 tab: focused = %d, want %d", m.Focused(), fieldSessionLimit)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focused() != fieldModelSelect {
		t.Errorf("after 2 tabs: focused = %d, want %d", m.Focused(), fieldModelSelect)
	}

	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focused() != fieldTotalBudget {
		t.Errorf("after 3 tabs: focused = %d, want %d (wrap)", m.Focused(), fieldTotalBudget)
	}

	// Shift+tab goes backwards.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	if m.Focused() != fieldModelSelect {
		t.Errorf("shift+tab from 0: focused = %d, want %d", m.Focused(), fieldModelSelect)
	}
}

func TestLauncherBudgetModel_Confirm(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)

	// Increase budget once.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.TotalBudget() != 10.50 {
		t.Errorf("budget after up = %f, want 10.50", m.TotalBudget())
	}

	// Confirm.
	var cmd tea.Cmd
	m, cmd = m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("confirm should produce a cmd")
	}

	msg := cmd()
	confirm, ok := msg.(LauncherBudgetConfirmMsg)
	if !ok {
		t.Fatalf("expected LauncherBudgetConfirmMsg, got %T", msg)
	}
	if confirm.Budget != 10.50 {
		t.Errorf("confirm budget = %f, want 10.50", confirm.Budget)
	}
	if confirm.Limit != 5.0 {
		t.Errorf("confirm limit = %f, want 5.0", confirm.Limit)
	}
	if confirm.Model != "claude-sonnet-4-20250514" {
		t.Errorf("confirm model = %q, want claude-sonnet-4-20250514", confirm.Model)
	}
}

func TestLauncherBudgetModel_Cancel(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd == nil {
		t.Fatal("cancel should produce a cmd")
	}

	msg := cmd()
	if _, ok := msg.(LauncherBudgetCancelMsg); !ok {
		t.Fatalf("expected LauncherBudgetCancelMsg, got %T", msg)
	}
}

func TestLauncherBudgetModel_AdjustBudgetFloor(t *testing.T) {
	m := NewLauncherBudget(testModels, 0.25)

	// Budget should floor at 0.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.TotalBudget() != 0.0 {
		t.Errorf("budget floor = %f, want 0.0", m.TotalBudget())
	}

	// Should not go negative.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.TotalBudget() != 0.0 {
		t.Errorf("budget below floor = %f, want 0.0", m.TotalBudget())
	}
}

func TestLauncherBudgetModel_ModelSelection(t *testing.T) {
	m := NewLauncherBudget(testModels, 10.0)

	// Focus model field.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	if m.Focused() != fieldModelSelect {
		t.Fatalf("expected model field focus")
	}

	// Move cursor down to select gemini.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.SelectedModel() != "gemini-2.5-pro" {
		t.Errorf("model = %q, want gemini-2.5-pro", m.SelectedModel())
	}

	// Move down again to o3.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.SelectedModel() != "o3" {
		t.Errorf("model = %q, want o3", m.SelectedModel())
	}

	// Move down past end — should stay at o3.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.SelectedModel() != "o3" {
		t.Errorf("model past end = %q, want o3", m.SelectedModel())
	}

	// Move back up.
	m, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyUp})
	if m.SelectedModel() != "gemini-2.5-pro" {
		t.Errorf("model after up = %q, want gemini-2.5-pro", m.SelectedModel())
	}
}

func TestLauncherBudgetModel_EmptyModels(t *testing.T) {
	m := NewLauncherBudget(nil, 5.0)
	if m.SelectedModel() != "" {
		t.Errorf("selected model with nil models = %q, want empty", m.SelectedModel())
	}
	view := m.View().Content
	if !strings.Contains(view, "(none)") {
		t.Error("view should show (none) when no models")
	}
}
