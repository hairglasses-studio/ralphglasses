package views

import (
	"fmt"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// SessionColumns defines the sessions table structure.
var SessionColumns = []components.Column{
	{Title: "ID", Width: 10, Sortable: true},
	{Title: "Provider", Width: 8, Sortable: true},
	{Title: "Repo", Width: 16, Sortable: true, Grow: true},
	{Title: "Status", Width: 10, Sortable: true},
	{Title: "Model", Width: 14, Sortable: true},
	{Title: "Spent", Width: 8, Sortable: true},
	{Title: "Turns", Width: 6, Sortable: true},
	{Title: "Agent", Width: 12, Sortable: false},
	{Title: "Team", Width: 12, Sortable: false},
	{Title: "Duration", Width: 10, Sortable: true},
}

// NewSessionsTable creates a table pre-configured for sessions.
func NewSessionsTable() *components.Table {
	t := components.NewTable(SessionColumns)
	t.EmptyMessage = "No sessions — launch via MCP"
	t.StatusColumn = 3
	return t
}

// SessionsToRows converts sessions to table rows with styled cells.
func SessionsToRows(sessions []*session.Session) []components.Row {
	rows := make([]components.Row, 0, len(sessions))
	for _, s := range sessions {
		s.Lock()
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}
		provider := string(s.Provider)
		repo := s.RepoName
		status := string(s.Status)
		model := s.Model
		spent := fmt.Sprintf("$%.2f", s.SpentUSD)
		turns := fmt.Sprintf("%d", s.TurnCount)
		agent := s.AgentName
		team := s.TeamName
		dur := formatDuration(s.LaunchedAt)
		s.Unlock()

		rows = append(rows, components.Row{
			id,
			components.StyledCell(styles.ProviderStyle(provider), provider),
			repo,
			components.StyledCell(styles.StatusStyle(status), status),
			model,
			spent,
			turns,
			agent,
			team,
			dur,
		})
	}
	return rows
}

func formatDuration(since time.Time) string {
	if since.IsZero() {
		return "-"
	}
	d := time.Since(since)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
