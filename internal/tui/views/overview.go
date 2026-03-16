package views

import (
	"fmt"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// OverviewColumns defines the overview table structure.
var OverviewColumns = []components.Column{
	{Title: "Name", Width: 20, Sortable: true},
	{Title: "Status", Width: 12, Sortable: true},
	{Title: "Loop #", Width: 8, Sortable: true},
	{Title: "Calls", Width: 10, Sortable: true},
	{Title: "Circuit", Width: 12, Sortable: true},
	{Title: "Last Action", Width: 16, Sortable: false},
	{Title: "Updated", Width: 12, Sortable: true},
}

// NewOverviewTable creates a table pre-configured for the overview.
func NewOverviewTable() *components.Table {
	return components.NewTable(OverviewColumns)
}

// ReposToRows converts repo models to table rows with styled cells.
func ReposToRows(repos []*model.Repo) []components.Row {
	rows := make([]components.Row, 0, len(repos))
	for _, r := range repos {
		status := r.StatusDisplay()
		circuit := r.CircuitDisplay()

		loopCount := "-"
		calls := r.CallsDisplay()
		lastAction := "-"
		updated := r.UpdatedDisplay()

		if r.Status != nil {
			loopCount = fmt.Sprintf("%d", r.Status.LoopCount)
			if r.Status.LastAction != "" {
				lastAction = r.Status.LastAction
			}
		}

		rows = append(rows, components.Row{
			r.Name,
			components.StyledCell(styles.StatusStyle(status), status),
			loopCount,
			calls,
			components.StyledCell(styles.CBStyle(circuit), circuit),
			lastAction,
			updated,
		})
	}
	return rows
}
