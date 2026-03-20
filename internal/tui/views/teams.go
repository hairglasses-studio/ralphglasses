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
	{Title: "Name", Width: 16, Sortable: true, Grow: true},
	{Title: "Repo", Width: 16, Sortable: true},
	{Title: "Status", Width: 10, Sortable: true},
	{Title: "Lead", Width: 10, Sortable: false},
	{Title: "Tasks", Width: 12, Sortable: true},
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
		taskStr := fmt.Sprintf("%d/%d", completed, len(t.Tasks))

		rows = append(rows, components.Row{
			t.Name,
			repo,
			components.StyledCell(styles.StatusStyle(status), status),
			leadID,
			taskStr,
		})
	}
	return rows
}
