package views

import (
	"fmt"
	"path/filepath"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// TeamColumns defines the teams table structure.
var TeamColumns = []components.Column{
	{Title: "Name", Width: 16, Sortable: true, MinWidth: 10, Flex: 2.0},
	{Title: "Repo", Width: 16, Sortable: true, MinWidth: 10, Flex: 1.5},
	{Title: "Status", Width: 14, Sortable: true, MinWidth: 10},
	{Title: "Lead", Width: 10, Sortable: false, MinWidth: 8},
	{Title: "Progress", Width: 16, Sortable: true, MinWidth: 10, Flex: 1.0},
	{Title: "Tasks", Width: 8, Sortable: true, MinWidth: 6},
}

// NewTeamsTable creates a table pre-configured for teams.
func NewTeamsTable() *components.Table {
	t := components.NewTable(TeamColumns)
	t.EmptyMessage = "No teams — create via MCP"
	return t
}

// TeamsToRows converts team statuses to table rows with styled cells.
func TeamsToRows(teams []*session.TeamStatus) []components.Row {
	rows := make([]components.Row, 0, len(teams))
	for _, t := range teams {
		leadID := t.LeadID
		if len(leadID) > 8 {
			leadID = leadID[:8]
		}

		status := string(t.Status)
		repo := filepath.Base(t.RepoPath)

		completed := 0
		for _, task := range t.Tasks {
			if task.Status == "completed" {
				completed++
			}
		}
		total := len(t.Tasks)

		// Status with icon
		statusCell := fmt.Sprintf("%s %s",
			styles.StatusIcon(status),
			styles.StatusStyle(status).Render(status))

		// Progress gauge
		progressCell := "-"
		if total > 0 {
			label := fmt.Sprintf("%d/%d", completed, total)
			progressCell = components.GaugeWithLabel(float64(completed), float64(total), 8, label)
		}

		taskStr := fmt.Sprintf("%d", total)

		rows = append(rows, components.Row{
			t.Name,
			repo,
			statusCell,
			leadID,
			progressCell,
			taskStr,
		})
	}
	return rows
}
