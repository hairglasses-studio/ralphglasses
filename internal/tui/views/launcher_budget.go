package views

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

const (
	fieldTotalBudget = iota
	fieldSessionLimit
	fieldModelSelect
	fieldCount
)

const (
	budgetStep    = 0.50
	budgetMinimum = 0.0
)

// LauncherBudgetConfirmMsg is sent when the user confirms the budget form.
type LauncherBudgetConfirmMsg struct {
	Budget float64
	Limit  float64
	Model  string
}

// LauncherBudgetCancelMsg is sent when the user cancels the budget form.
type LauncherBudgetCancelMsg struct{}

// LauncherBudgetModel is a tea.Model for configuring budget parameters before
// launching a new session. It presents a form with total budget, per-session
// limit, and model selection.
type LauncherBudgetModel struct {
	totalBudget  float64
	sessionLimit float64
	selectedModel string
	models       []string
	cursor       int // index into models list
	focused      int // which field is focused (0=budget, 1=limit, 2=model)
	width        int
	height       int
}

// NewLauncherBudget creates a new LauncherBudgetModel with the given model
// list and default budget. The session limit defaults to half the total budget.
func NewLauncherBudget(models []string, defaultBudget float64) LauncherBudgetModel {
	m := LauncherBudgetModel{
		totalBudget:  defaultBudget,
		sessionLimit: defaultBudget / 2,
		models:       models,
	}
	if len(models) > 0 {
		m.selectedModel = models[0]
	}
	return m
}

// Init implements tea.Model.
func (m LauncherBudgetModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m LauncherBudgetModel) Update(msg tea.Msg) (LauncherBudgetModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab":
			m.focused = (m.focused + 1) % fieldCount
		case "shift+tab":
			m.focused = (m.focused - 1 + fieldCount) % fieldCount
		case "up", "k":
			m.adjustUp()
		case "down", "j":
			m.adjustDown()
		case "enter":
			return m, func() tea.Msg {
				return LauncherBudgetConfirmMsg{
					Budget: m.totalBudget,
					Limit:  m.sessionLimit,
					Model:  m.selectedModel,
				}
			}
		case "esc":
			return m, func() tea.Msg {
				return LauncherBudgetCancelMsg{}
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View implements tea.Model.
func (m LauncherBudgetModel) View() tea.View {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("  "+styles.IconBudget+" Launch Budget Configuration"))
	b.WriteString("\n\n")

	// Total Budget field
	budgetLabel := "  Total Budget:"
	budgetValue := fmt.Sprintf("$%.2f", m.totalBudget)
	if m.focused == fieldTotalBudget {
		b.WriteString(styles.SelectedStyle.Render(fmt.Sprintf("▸ %s  %s", budgetLabel, budgetValue)))
	} else {
		b.WriteString(fmt.Sprintf("  %s  %s", budgetLabel, styles.InfoStyle.Render(budgetValue)))
	}
	b.WriteString("\n")

	// Session Limit field
	limitLabel := "  Session Limit:"
	limitValue := fmt.Sprintf("$%.2f", m.sessionLimit)
	if m.focused == fieldSessionLimit {
		b.WriteString(styles.SelectedStyle.Render(fmt.Sprintf("▸ %s  %s", limitLabel, limitValue)))
	} else {
		b.WriteString(fmt.Sprintf("  %s  %s", limitLabel, styles.InfoStyle.Render(limitValue)))
	}
	b.WriteString("\n")

	// Model selection field
	modelLabel := "  Model:"
	if m.focused == fieldModelSelect {
		b.WriteString(styles.SelectedStyle.Render(fmt.Sprintf("▸ %s", modelLabel)))
		b.WriteString("\n")
		for i, model := range m.models {
			if i == m.cursor {
				b.WriteString(styles.CommandStyle.Render(fmt.Sprintf("    ▸ %s", model)))
			} else {
				b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("      %s", model)))
			}
			b.WriteString("\n")
		}
	} else {
		modelDisplay := m.selectedModel
		if modelDisplay == "" {
			modelDisplay = "(none)"
		}
		b.WriteString(fmt.Sprintf("  %s  %s", modelLabel, styles.InfoStyle.Render(modelDisplay)))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Tab: next field  ↑/↓: adjust  Enter: confirm  Esc: cancel"))

	return tea.NewView(b.String())
}

// TotalBudget returns the current total budget value (for testing).
func (m LauncherBudgetModel) TotalBudget() float64 { return m.totalBudget }

// SessionLimit returns the current session limit value (for testing).
func (m LauncherBudgetModel) SessionLimit() float64 { return m.sessionLimit }

// SelectedModel returns the currently selected model (for testing).
func (m LauncherBudgetModel) SelectedModel() string { return m.selectedModel }

// Focused returns which field is currently focused (for testing).
func (m LauncherBudgetModel) Focused() int { return m.focused }

// Cursor returns the current cursor position in the model list (for testing).
func (m LauncherBudgetModel) Cursor() int { return m.cursor }

// adjustUp increases budget/limit or moves model cursor up.
func (m *LauncherBudgetModel) adjustUp() {
	switch m.focused {
	case fieldTotalBudget:
		m.totalBudget += budgetStep
	case fieldSessionLimit:
		m.sessionLimit += budgetStep
	case fieldModelSelect:
		if m.cursor > 0 {
			m.cursor--
			m.selectedModel = m.models[m.cursor]
		}
	}
}

// adjustDown decreases budget/limit or moves model cursor down.
func (m *LauncherBudgetModel) adjustDown() {
	switch m.focused {
	case fieldTotalBudget:
		if m.totalBudget-budgetStep >= budgetMinimum {
			m.totalBudget -= budgetStep
		} else {
			m.totalBudget = budgetMinimum
		}
	case fieldSessionLimit:
		if m.sessionLimit-budgetStep >= budgetMinimum {
			m.sessionLimit -= budgetStep
		} else {
			m.sessionLimit = budgetMinimum
		}
	case fieldModelSelect:
		if m.cursor < len(m.models)-1 {
			m.cursor++
			m.selectedModel = m.models[m.cursor]
		}
	}
}
