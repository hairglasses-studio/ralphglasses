package tui

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/evertras/bubble-table/table"

	"github.com/hairglasses-studio/runmylife/internal/tui/components"
)

type fleetData struct {
	Repos      []repoStatus
	TotalCost  float64
	ActiveLoop int
	CircuitOpen int
	CostRate   float64
}

type repoStatus struct {
	Name         string
	Provider     string
	Status       string
	LoopCount    int
	CallsUsed    int
	CallsMax     int
	Cost         float64
	BudgetPct    float64
	CircuitState string
	HealthScore  float64
	LastUpdate   time.Time
}

func loadFleetData(db *sql.DB) fleetData {
	if db == nil {
		return fleetData{}
	}
	ctx := context.Background()
	d := fleetData{}

	// Aggregate stats for today
	_ = db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0),
		        COUNT(CASE WHEN status = 'active' THEN 1 END),
		        COUNT(CASE WHEN circuit_state = 'open' THEN 1 END)
		 FROM sessions WHERE date(started_at) = date('now')`).
		Scan(&d.TotalCost, &d.ActiveLoop, &d.CircuitOpen)

	// Cost rate (last hour)
	_ = db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(cost_usd), 0) FROM sessions
		 WHERE updated_at >= datetime('now', '-1 hour')`).
		Scan(&d.CostRate)

	// Recent sessions
	rows, err := db.QueryContext(ctx,
		`SELECT COALESCE(repo, ''), COALESCE(provider, ''), COALESCE(status, ''),
		        COALESCE(loop_count, 0), COALESCE(api_calls_used, 0),
		        COALESCE(api_calls_max, 0), COALESCE(cost_usd, 0),
		        COALESCE(budget_pct, 0), COALESCE(circuit_state, 'closed'),
		        COALESCE(health_score, 0), COALESCE(updated_at, '')
		 FROM sessions
		 ORDER BY updated_at DESC LIMIT 15`)
	if err != nil {
		return d
	}
	defer rows.Close()
	for rows.Next() {
		var r repoStatus
		var updatedStr string
		if rows.Scan(&r.Name, &r.Provider, &r.Status,
			&r.LoopCount, &r.CallsUsed, &r.CallsMax, &r.Cost,
			&r.BudgetPct, &r.CircuitState, &r.HealthScore, &updatedStr) == nil {
			if t, err := time.Parse("2006-01-02 15:04:05", updatedStr); err == nil {
				r.LastUpdate = t
			} else if t, err := time.Parse(time.RFC3339, updatedStr); err == nil {
				r.LastUpdate = t
			}
			d.Repos = append(d.Repos, r)
		}
	}

	return d
}

func renderFleet(d fleetData, width int) string {
	var b strings.Builder
	gaugeWidth := 20

	// Fleet summary header
	header := components.StyledIcon(components.IconBrain, colorPrimary) + subtitleStyle.Render("Fleet Overview") + "\n"
	header += fmt.Sprintf("  Active: %s  |  Cost today: %s  |  Rate: %s/hr",
		lipgloss.NewStyle().Bold(true).Foreground(colorSuccess).Render(fmt.Sprintf("%d loops", d.ActiveLoop)),
		lipgloss.NewStyle().Bold(true).Foreground(colorWarning).Render(fmt.Sprintf("$%.2f", d.TotalCost)),
		mutedStyle.Render(fmt.Sprintf("$%.2f", d.CostRate)),
	)
	if d.CircuitOpen > 0 {
		header += "\n  " + alertStyle.Render(fmt.Sprintf("%s %d circuit(s) OPEN", components.IconAlert, d.CircuitOpen))
	}
	b.WriteString(cardStyle.Width(width).Render(header))
	b.WriteString("\n")

	// Repo table
	if len(d.Repos) > 0 {
		repoSec := subtitleStyle.Render("Sessions") + "\n"
		cols := []table.Column{
			table.NewFlexColumn("repo", "Repo", 2),
			table.NewColumn("provider", "Provider", 10),
			table.NewColumn("status", "Status", 10),
			table.NewColumn("loops", "Loops", 7),
			table.NewColumn("calls", "Calls", 12),
			table.NewColumn("cost", "Cost", 8),
			table.NewColumn("circuit", "Circuit", 10),
		}
		var rows []table.Row
		for _, r := range d.Repos {
			statusIcon := mutedStyle.Render("●")
			switch r.Status {
			case "active":
				statusIcon = successStyle.Render("●")
			case "paused":
				statusIcon = warningStyle.Render("●")
			case "error":
				statusIcon = alertStyle.Render("●")
			}

			callsStr := fmt.Sprintf("%d/%d", r.CallsUsed, r.CallsMax)
			if r.CallsMax == 0 {
				callsStr = fmt.Sprintf("%d", r.CallsUsed)
			}

			circuitStr := successStyle.Render("closed")
			switch r.CircuitState {
			case "open":
				circuitStr = alertStyle.Render("OPEN")
			case "half-open":
				circuitStr = warningStyle.Render("half")
			}

			rows = append(rows, table.NewRow(table.RowData{
				"repo":     r.Name,
				"provider": providerIcon(r.Provider) + r.Provider,
				"status":   statusIcon + " " + r.Status,
				"loops":    fmt.Sprintf("%d", r.LoopCount),
				"calls":    callsStr,
				"cost":     fmt.Sprintf("$%.2f", r.Cost),
				"circuit":  circuitStr,
			}))
		}
		tbl := components.SimpleTable(cols, rows, width-4)
		repoSec += tbl.View()
		b.WriteString(cardStyle.Width(width).Render(repoSec))
		b.WriteString("\n")
	} else {
		b.WriteString(cardStyle.Width(width).Render(mutedStyle.Render("  No fleet sessions found")))
		b.WriteString("\n")
	}

	// Cost burn rate gauge
	costGauge := components.StyledIcon(components.IconDollar, colorWarning) + subtitleStyle.Render("Daily Budget Burn") + "\n"
	dailyBudget := 10.0 // default daily budget cap
	costGauge += "  " + components.Gauge(d.TotalCost, dailyBudget, gaugeWidth,
		components.WithLabel(fmt.Sprintf("$%.2f / $%.0f", d.TotalCost, dailyBudget)),
		components.WithThresholds(0.6, 0.85),
	)
	if d.CostRate > 0 {
		remaining := dailyBudget - d.TotalCost
		if remaining > 0 && d.CostRate > 0 {
			hoursLeft := remaining / d.CostRate
			costGauge += fmt.Sprintf("\n  ~%.1f hours runway at current rate", hoursLeft)
		} else if remaining <= 0 {
			costGauge += "\n  " + alertStyle.Render("Budget exceeded!")
		}
	}
	b.WriteString(cardStyle.Width(width).Render(costGauge))
	b.WriteString("\n")

	// Circuit breaker summary
	if len(d.Repos) > 0 {
		var openRepos, halfRepos []string
		for _, r := range d.Repos {
			switch r.CircuitState {
			case "open":
				openRepos = append(openRepos, r.Name)
			case "half-open":
				halfRepos = append(halfRepos, r.Name)
			}
		}
		if len(openRepos) > 0 || len(halfRepos) > 0 {
			circuit := components.StyledIcon(components.IconAlert, colorDanger) + subtitleStyle.Render("Circuit Breakers") + "\n"
			if len(openRepos) > 0 {
				circuit += "  " + alertStyle.Render("OPEN: ") + strings.Join(openRepos, ", ") + "\n"
			}
			if len(halfRepos) > 0 {
				circuit += "  " + warningStyle.Render("HALF: ") + strings.Join(halfRepos, ", ") + "\n"
			}
			b.WriteString(cardStyle.Width(width).Render(circuit))
		}
	}

	return b.String()
}

func providerIcon(provider string) string {
	switch strings.ToLower(provider) {
	case "claude", "anthropic":
		return components.StyledIcon("◆ ", colorPrimary)
	case "gemini", "google":
		return components.StyledIcon("◆ ", colorSecondary)
	case "codex", "openai":
		return components.StyledIcon("◆ ", colorSuccess)
	default:
		return ""
	}
}
