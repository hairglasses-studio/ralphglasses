package views

import (
	"fmt"
	"strings"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// FleetAlert represents an alert in the fleet dashboard.
type FleetAlert struct {
	Severity string // "critical", "warning", "info"
	Message  string
}

// ProviderStat holds per-provider aggregate stats.
type ProviderStat struct {
	Sessions int
	Running  int
	SpendUSD float64
}

// ExpensiveSession holds info about a high-spend session.
type ExpensiveSession struct {
	ID       string
	Provider string
	RepoName string
	SpendUSD float64
	Status   string
}

// RepoBudget holds per-repo budget utilization.
type RepoBudget struct {
	Name      string
	SpendUSD  float64
	BudgetUSD float64
}

// FleetData holds aggregated fleet-level data.
type FleetData struct {
	TotalRepos      int
	RunningLoops    int
	PausedLoops     int
	TotalSessions   int
	RunningSessions int
	TotalSpendUSD   float64
	OpenCircuits    int
	Providers       map[string]ProviderStat
	Alerts          []FleetAlert
	Repos           []*model.Repo
	Sessions        []*session.Session
	CostHistory     []float64 // aggregate cost-over-time data points
	CostPerTurn     map[string]float64  // provider → avg cost/turn
	TopExpensive    []ExpensiveSession  // top 5 by spend
	RepoBudgets     []RepoBudget        // per-repo utilization
}

// RenderFleetDashboard renders the fleet-wide monitoring dashboard.
func RenderFleetDashboard(data FleetData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render("Fleet Dashboard"))
	b.WriteString("\n\n")

	// Stats bar — horizontal row of boxed stats
	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("REPOS\n  %d", data.TotalRepos)),
		styles.StatBox.Render(fmt.Sprintf("LOOPS\n  %d run / %d pause", data.RunningLoops, data.PausedLoops)),
		styles.StatBox.Render(fmt.Sprintf("SESSIONS\n  %d / %d run", data.TotalSessions, data.RunningSessions)),
		styles.StatBox.Render(fmt.Sprintf("SPEND\n  $%.2f", data.TotalSpendUSD)),
	}

	circuitBox := fmt.Sprintf("CIRCUITS\n  %d open", data.OpenCircuits)
	if data.OpenCircuits > 0 {
		circuitBox = styles.StatusFailed.Render(circuitBox)
	}
	statBoxes = append(statBoxes, styles.StatBox.Render(circuitBox))

	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, statBoxes...))
	b.WriteString("\n\n")

	// Cost sparkline
	if len(data.CostHistory) > 1 {
		sparkWidth := width / 2
		if sparkWidth < 20 {
			sparkWidth = 20
		}
		if sparkWidth > 60 {
			sparkWidth = 60
		}
		points := data.CostHistory
		if len(points) > sparkWidth {
			points = points[len(points)-sparkWidth:]
		}
		sl := sparkline.New(sparkWidth, 3)
		for _, v := range points {
			sl.Push(v)
		}
		b.WriteString(styles.HeaderStyle.Render("Cost Trend"))
		b.WriteString("\n")
		b.WriteString(sl.View())
		b.WriteString("\n\n")
	}

	// Provider breakdown
	if len(data.Providers) > 0 {
		b.WriteString(styles.HeaderStyle.Render("Provider Breakdown"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %-10s %-10s %-10s %-10s\n",
			styles.HeaderStyle.Render("Provider"),
			styles.HeaderStyle.Render("Sessions"),
			styles.HeaderStyle.Render("Running"),
			styles.HeaderStyle.Render("Spend")))
		for provider, stat := range data.Providers {
			b.WriteString(fmt.Sprintf("  %-10s %-10d %-10d $%-9.2f\n",
				styles.ProviderStyle(provider).Render(provider),
				stat.Sessions, stat.Running, stat.SpendUSD))
		}
		b.WriteString("\n")
	}

	// Provider spend bar chart
	if len(data.Providers) > 0 {
		maxSpend := 0.0
		for _, stat := range data.Providers {
			if stat.SpendUSD > maxSpend {
				maxSpend = stat.SpendUSD
			}
		}
		if maxSpend > 0 {
			barMaxWidth := width/2 - 25
			if barMaxWidth < 10 {
				barMaxWidth = 10
			}
			b.WriteString(styles.HeaderStyle.Render("Provider Spend"))
			b.WriteString("\n")
			for provider, stat := range data.Providers {
				barLen := int(stat.SpendUSD / maxSpend * float64(barMaxWidth))
				if barLen < 1 && stat.SpendUSD > 0 {
					barLen = 1
				}
				bar := strings.Repeat("█", barLen)
				b.WriteString(fmt.Sprintf("  %-8s %s $%.2f\n",
					styles.ProviderStyle(provider).Render(provider),
					styles.ProviderStyle(provider).Render(bar),
					stat.SpendUSD))
			}
			b.WriteString("\n")
		}
	}

	// Cost-per-turn metrics
	if len(data.CostPerTurn) > 0 {
		b.WriteString(styles.HeaderStyle.Render("Cost Efficiency ($/turn)"))
		b.WriteString("\n")
		for provider, cpt := range data.CostPerTurn {
			b.WriteString(fmt.Sprintf("  %-8s $%.4f/turn\n",
				styles.ProviderStyle(provider).Render(provider), cpt))
		}
		b.WriteString("\n")
	}

	// Budget utilization gauges
	if len(data.RepoBudgets) > 0 {
		b.WriteString(styles.HeaderStyle.Render("Budget Utilization"))
		b.WriteString("\n")
		gaugeWidth := 20
		for _, rb := range data.RepoBudgets {
			pct := 0.0
			if rb.BudgetUSD > 0 {
				pct = rb.SpendUSD / rb.BudgetUSD
			}
			filled := int(pct * float64(gaugeWidth))
			if filled > gaugeWidth {
				filled = gaugeWidth
			}
			bar := strings.Repeat("█", filled) + strings.Repeat("░", gaugeWidth-filled)
			pctStr := fmt.Sprintf("%.0f%%", pct*100)
			style := styles.StatusRunning
			if pct >= 0.9 {
				style = styles.StatusFailed
			} else if pct >= 0.7 {
				style = styles.WarningStyle
			}
			b.WriteString(fmt.Sprintf("  %-16s %s %s ($%.2f/$%.2f)\n",
				rb.Name, style.Render(bar), pctStr, rb.SpendUSD, rb.BudgetUSD))
		}
		b.WriteString("\n")
	}

	// Top expensive sessions
	if len(data.TopExpensive) > 0 {
		b.WriteString(styles.HeaderStyle.Render("Top Sessions by Spend"))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  %-10s %-8s %-14s %-10s %-8s\n",
			styles.HeaderStyle.Render("ID"),
			styles.HeaderStyle.Render("Provider"),
			styles.HeaderStyle.Render("Repo"),
			styles.HeaderStyle.Render("Spend"),
			styles.HeaderStyle.Render("Status")))
		for _, es := range data.TopExpensive {
			id := es.ID
			if len(id) > 8 {
				id = id[:8]
			}
			repo := es.RepoName
			if len(repo) > 12 {
				repo = repo[:12] + "…"
			}
			b.WriteString(fmt.Sprintf("  %-10s %-8s %-14s $%-9.2f %s\n",
				id,
				styles.ProviderStyle(es.Provider).Render(es.Provider),
				repo,
				es.SpendUSD,
				styles.StatusStyle(es.Status).Render(es.Status)))
		}
		b.WriteString("\n")
	}

	// Alerts
	b.WriteString(styles.HeaderStyle.Render("Alerts"))
	b.WriteString("\n")
	if len(data.Alerts) == 0 {
		b.WriteString(styles.StatusRunning.Render("  No alerts"))
		b.WriteString("\n")
	} else {
		for _, alert := range data.Alerts {
			prefix := "  "
			switch alert.Severity {
			case "critical":
				prefix += styles.AlertCritical.Render("CRIT")
			case "warning":
				prefix += styles.AlertWarning.Render("WARN")
			default:
				prefix += styles.AlertInfo.Render("INFO")
			}
			b.WriteString(fmt.Sprintf("%s  %s\n", prefix, alert.Message))
		}
	}
	b.WriteString("\n")

	// Compact lists side-by-side
	var repoList, sessionList strings.Builder

	repoList.WriteString(styles.HeaderStyle.Render("Repos"))
	repoList.WriteString("\n")
	for _, r := range data.Repos {
		status := r.StatusDisplay()
		repoList.WriteString(fmt.Sprintf("  %-16s %s\n",
			r.Name,
			styles.StatusStyle(status).Render(status)))
	}

	sessionList.WriteString(styles.HeaderStyle.Render("Running Sessions"))
	sessionList.WriteString("\n")
	hasRunning := false
	for _, s := range data.Sessions {
		s.Lock()
		st := s.Status
		if st == session.StatusRunning || st == session.StatusLaunching {
			id := s.ID
			if len(id) > 8 {
				id = id[:8]
			}
			provider := string(s.Provider)
			repo := s.RepoName
			s.Unlock()
			sessionList.WriteString(fmt.Sprintf("  %-8s  %s  %s\n",
				id,
				styles.ProviderStyle(provider).Render(fmt.Sprintf("%-7s", provider)),
				repo))
			hasRunning = true
		} else {
			s.Unlock()
		}
	}
	if !hasRunning {
		sessionList.WriteString(styles.InfoStyle.Render("  None"))
		sessionList.WriteString("\n")
	}

	halfWidth := width/2 - 2
	if halfWidth < 20 {
		halfWidth = 30
	}
	leftPanel := lipgloss.NewStyle().Width(halfWidth).Render(repoList.String())
	rightPanel := lipgloss.NewStyle().Width(halfWidth).Render(sessionList.String())
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel))

	return b.String()
}
