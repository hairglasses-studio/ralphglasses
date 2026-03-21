package views

import (
	"fmt"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// OverviewColumns defines the overview table structure.
var OverviewColumns = []components.Column{
	{Title: "Name", Width: 20, Sortable: true, Grow: true},
	{Title: "Status", Width: 14, Sortable: true},
	{Title: "Loop#", Width: 8, Sortable: true},
	{Title: "Calls", Width: 16, Sortable: true},
	{Title: "Budget", Width: 16, Sortable: true},
	{Title: "Progress", Width: 14, Sortable: true},
	{Title: "Circuit", Width: 12, Sortable: true},
	{Title: "Updated", Width: 10, Sortable: true},
}

// NewOverviewTable creates a table pre-configured for the overview.
func NewOverviewTable() *components.Table {
	t := components.NewTable(OverviewColumns)
	t.EmptyMessage = "No repos found — press r to scan"
	t.StatusColumn = 1
	return t
}

// ReposToRows converts repo models to table rows with styled cells.
func ReposToRows(repos []*model.Repo, tickFrame int) []components.Row {
	rows := make([]components.Row, 0, len(repos))
	for _, r := range repos {
		status := r.StatusDisplay()
		circuit := r.CircuitDisplay()

		loopCount := "-"
		updated := r.UpdatedDisplay()

		// Status cell with activity dot + icon
		isActive := status == "running"
		statusCell := fmt.Sprintf("%s %s %s",
			components.ActivityDot(isActive, tickFrame),
			styles.StatusIcon(status),
			styles.StatusStyle(status).Render(status))

		// Calls gauge
		callsCell := "-"
		if r.Status != nil {
			made := float64(r.Status.CallsMadeThisHr)
			max := float64(r.Status.MaxCallsPerHour)
			label := fmt.Sprintf("%d/%d", r.Status.CallsMadeThisHr, r.Status.MaxCallsPerHour)
			if max > 0 {
				callsCell = components.GaugeWithLabel(made, max, 8, label)
			} else {
				callsCell = label
			}
			loopCount = fmt.Sprintf("%d", r.Status.LoopCount)
		}

		// Budget gauge
		budgetCell := "-"
		if r.Status != nil && r.Status.SessionSpendUSD > 0 {
			spend := r.Status.SessionSpendUSD
			label := fmt.Sprintf("$%.2f", spend)
			// Try to derive budget from config
			budgetMax := 0.0
			if r.Config != nil {
				if v, ok := r.Config.Values["RALPH_SESSION_BUDGET"]; ok {
					fmt.Sscanf(v, "%f", &budgetMax)
				}
			}
			if budgetMax > 0 {
				budgetCell = components.GaugeWithLabel(spend, budgetMax, 8, label)
			} else {
				budgetCell = label
			}
		}

		// Progress gauge
		progressCell := "-"
		if r.Progress != nil && len(r.Progress.CompletedIDs) > 0 {
			completed := float64(len(r.Progress.CompletedIDs))
			// Estimate total from iteration count (or just show completed)
			total := completed + 1 // at minimum
			if r.Progress.Iteration > int(completed) {
				total = float64(r.Progress.Iteration)
			}
			label := fmt.Sprintf("%d", len(r.Progress.CompletedIDs))
			progressCell = components.GaugeWithLabel(completed, total, 8, label)
		}

		// Circuit with icon
		circuitCell := "-"
		if circuit != "-" {
			circuitCell = fmt.Sprintf("%s %s", styles.CBIcon(circuit), styles.CBStyle(circuit).Render(circuit))
		}

		rows = append(rows, components.Row{
			r.Name,
			statusCell,
			loopCount,
			callsCell,
			budgetCell,
			progressCell,
			circuitCell,
			updated,
		})
	}
	return rows
}
