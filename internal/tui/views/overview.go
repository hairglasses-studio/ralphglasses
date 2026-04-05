package views

import (
	"fmt"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// OverviewColumns defines the overview table structure.
var OverviewColumns = []components.Column{
	{Title: "Name", Width: 18, Sortable: true, MinWidth: 12, Flex: 2.0, Priority: 0},
	{Title: "Status", Width: 10, Sortable: true, MinWidth: 8, Priority: 0},
	{Title: "Loop", Width: 6, Sortable: true, MinWidth: 5, Priority: 1},
	{Title: "Calls", Width: 12, Sortable: true, MinWidth: 5, Flex: 0.5, Priority: 2},
	{Title: "Budget", Width: 12, Sortable: true, MinWidth: 8, Flex: 1.0, Priority: 1},
	{Title: "Progress", Width: 10, Sortable: true, MinWidth: 8, Flex: 0.5, Priority: 2},
	{Title: "CB", Width: 10, Sortable: true, MinWidth: 8, Priority: 2},
	{Title: "Health", Width: 12, Sortable: true, MinWidth: 6, Priority: 1},
	{Title: "Updated", Width: 8, Sortable: true, MinWidth: 8, Flex: 0.5, Priority: 1},
}

// NewOverviewTable creates a table pre-configured for the overview.
func NewOverviewTable() *components.Table {
	t := components.NewTable(OverviewColumns)
	t.EmptyMessage = "No repos found — press r to scan"
	t.StatusColumn = 1
	return t
}

// RepoHealthData holds per-repo health info for the overview table.
type RepoHealthData struct {
	Verdict      string  // "pass", "warn", "fail", or ""
	CostHistory  []float64
	CostThreshold float64
}

// ReposToRows converts repo models to table rows with styled cells.
// healthData is keyed by repo path; nil entries or missing keys show "—".
func ReposToRows(repos []*model.Repo, tickFrame int, healthData map[string]RepoHealthData, termWidth int) []components.Row {
	rows := make([]components.Row, 0, len(repos))
	for _, r := range repos {
		status := r.StatusDisplay()
		circuit := r.CircuitDisplay()

		loopCount := "-"
		updated := r.UpdatedDisplay()

		// Status cell with activity dot + icon
		isActive := status == "running"
		statusCell := fmt.Sprintf("%s%s %s",
			components.ActivityDot(isActive, tickFrame),
			styles.StatusIcon(status),
			status)

		// Calls gauge
		callsCell := "-"
		if r.Status != nil {
			made := float64(r.Status.CallsMadeThisHr)
			max := float64(r.Status.MaxCallsPerHour)
			label := fmt.Sprintf("%d/%d", r.Status.CallsMadeThisHr, r.Status.MaxCallsPerHour)
			if max > 0 {
				callsCell = components.GaugeWithLabel(made, max, 5, label)
			} else {
				callsCell = label
			}
			loopCount = fmt.Sprintf("%d", r.Status.LoopCount)
		}

		// Budget gauge
		budgetCell := "-"
		if r.Status != nil && r.Status.SessionSpendUSD > 0 {
			spend := r.Status.SessionSpendUSD
			label := fmt.Sprintf("$%.0f", spend)
			budgetMax := 0.0
			if r.Config != nil {
				if v, ok := r.Config.Values["RALPH_SESSION_BUDGET"]; ok {
					_, _ = fmt.Sscanf(v, "%f", &budgetMax)
				}
			}
			if budgetMax > 0 {
				budgetCell = components.GaugeWithLabel(spend, budgetMax, 5, label)
			} else {
				budgetCell = label
			}
		}

		// Progress gauge
		progressCell := "-"
		if r.Progress != nil && len(r.Progress.CompletedIDs) > 0 {
			completed := float64(len(r.Progress.CompletedIDs))
			total := completed + 1
			if r.Progress.Iteration > int(completed) {
				total = float64(r.Progress.Iteration)
			}
			label := fmt.Sprintf("%d", len(r.Progress.CompletedIDs))
			progressCell = components.GaugeWithLabel(completed, total, 5, label)
		}

		// Circuit with icon
		circuitCell := "-"
		if circuit != "-" {
			circuitCell = fmt.Sprintf("%s %s", styles.CBIcon(circuit), circuit)
		}

		// Health cell: badge + optional sparkline when wide
		healthCell := styles.InfoStyle.Render("—")
		if healthData != nil {
			if hd, ok := healthData[r.Path]; ok && hd.Verdict != "" {
				badge := components.GateVerdictBadge(hd.Verdict)
				if termWidth > 100 && len(hd.CostHistory) > 0 {
					spark := components.HealthSparkline(hd.CostHistory, hd.CostThreshold, 8)
					healthCell = badge + " " + spark
				} else {
					healthCell = badge
				}
			}
		}

		rows = append(rows, components.Row{
			r.Name,
			statusCell,
			loopCount,
			callsCell,
			budgetCell,
			progressCell,
			circuitCell,
			healthCell,
			updated,
		})
	}
	return rows
}
