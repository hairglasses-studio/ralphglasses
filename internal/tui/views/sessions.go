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
	{Title: "Provider", Width: 10, Sortable: true},
	{Title: "Repo", Width: 16, Sortable: true, Grow: true},
	{Title: "Status", Width: 14, Sortable: true},
	{Title: "Budget", Width: 16, Sortable: true},
	{Title: "Trend", Width: 10, Sortable: false},
	{Title: "Turns", Width: 12, Sortable: true},
	{Title: "Agent", Width: 10, Sortable: false},
	{Title: "Team", Width: 10, Sortable: false},
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
func SessionsToRows(sessions []*session.Session, tickFrame int) []components.Row {
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
		spent := s.SpentUSD
		budget := s.BudgetUSD
		turns := s.TurnCount
		maxTurns := s.MaxTurns
		agent := s.AgentName
		team := s.TeamName
		dur := formatDuration(s.LaunchedAt)
		costHistory := make([]float64, len(s.CostHistory))
		copy(costHistory, s.CostHistory)
		s.Unlock()

		// Provider with icon
		providerCell := fmt.Sprintf("%s %s",
			styles.ProviderIcon(provider),
			styles.ProviderStyle(provider).Render(provider))

		// Status with activity dot + icon
		isActive := status == "running" || status == "launching"
		statusCell := fmt.Sprintf("%s %s %s",
			components.ActivityDot(isActive, tickFrame),
			styles.StatusIcon(status),
			styles.StatusStyle(status).Render(status))

		// Budget gauge
		budgetCell := fmt.Sprintf("$%.2f", spent)
		if budget > 0 {
			budgetCell = components.GaugeWithLabel(spent, budget, 8, fmt.Sprintf("$%.2f", spent))
		}

		// Cost trend sparkline
		trendCell := ""
		if len(costHistory) > 1 {
			trendCell = components.InlineSparkline(costHistory, 8)
		}

		// Turns gauge
		turnsCell := fmt.Sprintf("%d", turns)
		if maxTurns > 0 {
			turnsCell = components.GaugeWithLabel(float64(turns), float64(maxTurns), 6, fmt.Sprintf("%d/%d", turns, maxTurns))
		}

		rows = append(rows, components.Row{
			id,
			providerCell,
			repo,
			statusCell,
			budgetCell,
			trendCell,
			turnsCell,
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
