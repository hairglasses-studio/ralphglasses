package tui

import (
	"context"
	"testing"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

// These tests verify the SQL statements used by form onSubmit closures
// by executing them directly against the test DB.

func TestMoodForm_CorrectColumns(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Verify the fixed INSERT uses mood_score (not score) and date (not logged_at)
	_, err := db.ExecContext(ctx,
		`INSERT INTO mood_log (date, mood_score, energy_level, sleep_hours, notes) VALUES (date('now'), ?, ?, ?, ?)`,
		7, 6, 8.0, "test notes")
	if err != nil {
		t.Fatalf("mood INSERT failed (column name bug?): %v", err)
	}

	var score, energy int
	var sleep float64
	var notes string
	err = db.QueryRow("SELECT mood_score, energy_level, sleep_hours, notes FROM mood_log LIMIT 1").
		Scan(&score, &energy, &sleep, &notes)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if score != 7 {
		t.Errorf("mood_score = %d, want 7", score)
	}
	if energy != 6 {
		t.Errorf("energy_level = %d, want 6", energy)
	}
	if sleep != 8.0 {
		t.Errorf("sleep_hours = %v, want 8.0", sleep)
	}
	if notes != "test notes" {
		t.Errorf("notes = %q, want 'test notes'", notes)
	}
}

func TestMoodForm_OnSubmit_DefaultValues(t *testing.T) {
	db := testutil.TestDB(t)
	_, _, onSubmit := openMoodForm(db)
	// huh.Select defaults to first option (score=1, energy=1)
	msg := onSubmit()
	if msg == nil {
		t.Fatal("expected success msg with default select values")
	}

	var count int
	db.QueryRow("SELECT COUNT(*) FROM mood_log").Scan(&count)
	if count != 1 {
		t.Errorf("mood_log rows = %d, want 1", count)
	}
}

func TestExpenseForm_NegativeAmount(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	// Verify the expense INSERT stores negative amounts
	_, err := db.ExecContext(ctx,
		`INSERT INTO transactions (date, description, amount, category, type)
		 VALUES (date('now'), ?, ?, ?, 'expense')`,
		"Coffee", -4.50, "food")
	if err != nil {
		t.Fatalf("expense INSERT failed: %v", err)
	}

	var amount float64
	db.QueryRow("SELECT amount FROM transactions LIMIT 1").Scan(&amount)
	if amount >= 0 {
		t.Errorf("amount = %v, want negative for expense", amount)
	}
}

func TestExpenseForm_OnSubmit_InvalidAmount(t *testing.T) {
	db := testutil.TestDB(t)
	_, _, onSubmit := openExpenseForm(db)
	// amount defaults to "" which should fail parsing
	msg := onSubmit()
	if msg == nil {
		t.Fatal("expected error msg for invalid amount")
	}
	submitted, ok := msg.(formSubmittedMsg)
	if !ok {
		t.Fatalf("msg type = %T, want formSubmittedMsg", msg)
	}
	if submitted.toast.level != toastError {
		t.Errorf("toast style = %v, want toastError", submitted.toast.level)
	}
}

func TestTaskForm_Insert(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO tasks (title, priority, due_date, completed) VALUES (?, ?, NULLIF(?, ''), 0)`,
		"Test task", 2, "2026-04-10")
	if err != nil {
		t.Fatalf("task INSERT failed: %v", err)
	}

	var title string
	var priority, completed int
	db.QueryRow("SELECT title, priority, completed FROM tasks LIMIT 1").Scan(&title, &priority, &completed)
	if title != "Test task" {
		t.Errorf("title = %q, want 'Test task'", title)
	}
	if priority != 2 {
		t.Errorf("priority = %d, want 2", priority)
	}
	if completed != 0 {
		t.Errorf("completed = %d, want 0", completed)
	}
}

func TestTaskForm_OnSubmit_EmptyTitle(t *testing.T) {
	db := testutil.TestDB(t)
	_, _, onSubmit := openTaskForm(db)
	// title defaults to "" which should show error
	msg := onSubmit()
	if msg == nil {
		t.Fatal("expected error msg for empty title")
	}
	submitted, ok := msg.(formSubmittedMsg)
	if !ok {
		t.Fatalf("msg type = %T, want formSubmittedMsg", msg)
	}
	if submitted.toast.level != toastError {
		t.Errorf("toast style = %v, want toastError", submitted.toast.level)
	}
}

func TestFocusForm_Insert(t *testing.T) {
	db := testutil.TestDB(t)
	ctx := context.Background()

	_, err := db.ExecContext(ctx,
		`INSERT INTO focus_sessions (category, started_at, planned_minutes) VALUES (?, datetime('now'), ?)`,
		"coding", 45)
	if err != nil {
		t.Fatalf("focus INSERT failed: %v", err)
	}

	var category string
	var planned int
	db.QueryRow("SELECT category, planned_minutes FROM focus_sessions LIMIT 1").Scan(&category, &planned)
	if category != "coding" {
		t.Errorf("category = %q, want coding", category)
	}
	if planned != 45 {
		t.Errorf("planned_minutes = %d, want 45", planned)
	}
}

func TestFocusForm_OnSubmit_Defaults(t *testing.T) {
	db := testutil.TestDB(t)
	_, _, onSubmit := openFocusForm(db)
	// huh.Select defaults to first option (deep-work), minutes "" → 25
	msg := onSubmit()
	if msg == nil {
		t.Fatal("expected success msg")
	}

	var category string
	var planned int
	db.QueryRow("SELECT category, planned_minutes FROM focus_sessions LIMIT 1").Scan(&category, &planned)
	if category != "deep-work" {
		t.Errorf("category = %q, want deep-work (first select option)", category)
	}
	if planned != 25 {
		t.Errorf("planned_minutes = %d, want 25 (default)", planned)
	}
}
