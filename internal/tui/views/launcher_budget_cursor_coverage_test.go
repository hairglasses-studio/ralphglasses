package views

import (
	"testing"
)

func TestLauncherBudgetModel_Cursor_Initial(t *testing.T) {
	m := NewLauncherBudget([]string{"gpt-4", "claude-3"}, 10.0)
	if m.Cursor() != 0 {
		t.Errorf("Cursor() = %d, want 0", m.Cursor())
	}
}

func TestLauncherBudgetModel_AdjustUp_ModelSelect(t *testing.T) {
	models := []string{"model-a", "model-b", "model-c"}
	m := NewLauncherBudget(models, 10.0)
	m.focused = fieldModelSelect
	// Move cursor down first.
	m.adjustDown()
	m.adjustDown()
	if m.Cursor() != 2 {
		t.Fatalf("expected cursor=2 after 2 adjustDown, got %d", m.Cursor())
	}
	// Move up.
	m.adjustUp()
	if m.Cursor() != 1 {
		t.Errorf("Cursor after adjustUp = %d, want 1", m.Cursor())
	}
	// adjustUp at 0 should stay at 0.
	m.cursor = 0
	m.adjustUp()
	if m.Cursor() != 0 {
		t.Errorf("Cursor at 0 after adjustUp = %d, want 0", m.Cursor())
	}
}

func TestLauncherBudgetModel_AdjustDown_Budget(t *testing.T) {
	m := NewLauncherBudget([]string{"gpt-4"}, 10.0)
	m.focused = fieldTotalBudget
	initial := m.totalBudget
	m.adjustDown()
	if m.totalBudget != initial-budgetStep {
		t.Errorf("totalBudget after adjustDown = %f, want %f", m.totalBudget, initial-budgetStep)
	}
}

func TestLauncherBudgetModel_AdjustDown_BudgetAtMin(t *testing.T) {
	m := NewLauncherBudget([]string{"gpt-4"}, budgetMinimum+budgetStep/2)
	m.focused = fieldTotalBudget
	m.adjustDown()
	if m.totalBudget < budgetMinimum {
		t.Errorf("totalBudget %f went below minimum %f", m.totalBudget, budgetMinimum)
	}
}
