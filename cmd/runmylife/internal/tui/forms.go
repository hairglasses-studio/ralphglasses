package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

func openMoodForm(db *sql.DB) (string, *huh.Form, func() tea.Msg) {
	var score int
	var energy int
	var sleepStr string
	var notes string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[int]().
				Title("Mood Score").
				Options(
					huh.NewOption("1 — Awful", 1),
					huh.NewOption("2 — Bad", 2),
					huh.NewOption("3 — Poor", 3),
					huh.NewOption("4 — Below Average", 4),
					huh.NewOption("5 — Okay", 5),
					huh.NewOption("6 — Fine", 6),
					huh.NewOption("7 — Good", 7),
					huh.NewOption("8 — Great", 8),
					huh.NewOption("9 — Excellent", 9),
					huh.NewOption("10 — Amazing", 10),
				).Value(&score),
			huh.NewSelect[int]().
				Title("Energy Level").
				Options(
					huh.NewOption("1 — Exhausted", 1),
					huh.NewOption("3 — Low", 3),
					huh.NewOption("5 — Moderate", 5),
					huh.NewOption("7 — Good", 7),
					huh.NewOption("10 — Peak", 10),
				).Value(&energy),
			huh.NewInput().
				Title("Sleep Hours (e.g. 7.5)").
				Value(&sleepStr),
			huh.NewInput().
				Title("Notes (optional)").
				Value(&notes),
		),
	).WithShowHelp(false)

	onSubmit := func() tea.Msg {
		if score == 0 {
			return nil
		}
		sleep, _ := strconv.ParseFloat(sleepStr, 64)
		if energy == 0 {
			energy = 5
		}
		ctx := context.Background()
		_, _ = db.ExecContext(ctx,
			`INSERT INTO mood_log (date, mood_score, energy_level, sleep_hours, notes) VALUES (date('now'), ?, ?, ?, ?)`,
			score, energy, sleep, notes)
		return formSubmittedMsg{toast: newToast(fmt.Sprintf("Mood logged: %d/10", score), toastSuccess)}
	}

	return "Log Mood", form, onSubmit
}

func openExpenseForm(db *sql.DB) (string, *huh.Form, func() tea.Msg) {
	var amount string
	var description string
	var category string

	categories := loadBudgetCategories(db)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Amount ($)").
				Value(&amount),
			huh.NewInput().
				Title("Description").
				Value(&description),
			huh.NewSelect[string]().
				Title("Category").
				Options(categories...).
				Value(&category),
		),
	).WithShowHelp(false)

	onSubmit := func() tea.Msg {
		amt, err := strconv.ParseFloat(amount, 64)
		if err != nil || amt <= 0 {
			return formSubmittedMsg{toast: newToast("Invalid amount", toastError)}
		}
		ctx := context.Background()
		_, _ = db.ExecContext(ctx,
			`INSERT INTO transactions (date, description, amount, category, type)
			 VALUES (date('now'), ?, ?, ?, 'expense')`,
			description, -amt, category)
		return formSubmittedMsg{toast: newToast(fmt.Sprintf("$%.2f expense added", amt), toastSuccess)}
	}

	return "Add Expense", form, onSubmit
}

func openTaskForm(db *sql.DB) (string, *huh.Form, func() tea.Msg) {
	var title string
	var priority int
	var dueDate string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Task Title").
				Value(&title),
			huh.NewSelect[int]().
				Title("Priority").
				Options(
					huh.NewOption("1 — Low", 1),
					huh.NewOption("2 — Medium", 2),
					huh.NewOption("3 — High", 3),
					huh.NewOption("4 — Urgent", 4),
				).Value(&priority),
			huh.NewInput().
				Title("Due Date (YYYY-MM-DD, optional)").
				Value(&dueDate),
		),
	).WithShowHelp(false)

	onSubmit := func() tea.Msg {
		if title == "" {
			return formSubmittedMsg{toast: newToast("Title required", toastError)}
		}
		ctx := context.Background()
		_, _ = db.ExecContext(ctx,
			`INSERT INTO tasks (title, priority, due_date, completed) VALUES (?, ?, NULLIF(?, ''), 0)`,
			title, priority, dueDate)
		return formSubmittedMsg{toast: newToast(fmt.Sprintf("Task added: %s", title), toastSuccess)}
	}

	return "Add Task", form, onSubmit
}

func openFocusForm(db *sql.DB) (string, *huh.Form, func() tea.Msg) {
	var category string
	var minutesStr string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Category").
				Options(
					huh.NewOption("Deep Work", "deep-work"),
					huh.NewOption("Coding", "coding"),
					huh.NewOption("Reading", "reading"),
					huh.NewOption("Creative", "creative"),
					huh.NewOption("General", "general"),
				).Value(&category),
			huh.NewInput().
				Title("Planned Minutes (default 25)").
				Value(&minutesStr),
		),
	).WithShowHelp(false)

	onSubmit := func() tea.Msg {
		if category == "" {
			category = "general"
		}
		minutes, _ := strconv.Atoi(minutesStr)
		if minutes <= 0 {
			minutes = 25
		}
		ctx := context.Background()
		_, _ = db.ExecContext(ctx,
			`INSERT INTO focus_sessions (category, started_at, planned_minutes) VALUES (?, datetime('now'), ?)`,
			category, minutes)
		return formSubmittedMsg{toast: newToast(fmt.Sprintf("Focus started: %s (%dm)", category, minutes), toastSuccess)}
	}

	return "Start Focus", form, onSubmit
}

func loadBudgetCategories(db *sql.DB) []huh.Option[string] {
	defaults := []huh.Option[string]{huh.NewOption("Other", "other")}
	rows, err := db.Query(`SELECT DISTINCT category FROM budgets ORDER BY category`)
	if err != nil {
		return defaults
	}
	defer rows.Close()

	var cats []huh.Option[string]
	for rows.Next() {
		var cat string
		if rows.Scan(&cat) == nil && cat != "" {
			cats = append(cats, huh.NewOption(cat, cat))
		}
	}
	if len(cats) == 0 {
		return defaults
	}
	return append(cats, huh.NewOption("Other", "other"))
}
