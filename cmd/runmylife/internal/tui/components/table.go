package components

import (
	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"
)

// Theme colors for tables.
var (
	tableHeaderColor = lipgloss.Color("#7C3AED") // purple
	tableBorderColor = lipgloss.Color("#374151") // dark gray
	tableStripeColor = lipgloss.Color("#1F2937") // subtle dark
)

// DataTable creates a pre-themed bubble-table ready for static rendering.
func DataTable(columns []table.Column, rows []table.Row, width int) table.Model {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(tableHeaderColor)

	baseStyle := lipgloss.NewStyle().
		BorderForeground(tableBorderColor)

	t := table.New(columns).
		WithRows(rows).
		WithTargetWidth(width).
		WithBaseStyle(baseStyle).
		HeaderStyle(headerStyle).
		WithNoPagination()

	return t
}

// SimpleTable creates a minimal table (no borders, compact).
func SimpleTable(columns []table.Column, rows []table.Row, width int) table.Model {
	baseStyle := lipgloss.NewStyle().
		Padding(0, 1)

	t := table.New(columns).
		WithRows(rows).
		WithTargetWidth(width).
		WithBaseStyle(baseStyle).
		WithNoPagination()

	return t
}
