package views

import (
	"fmt"
	"strings"
	"time"

	"sort"

	"github.com/NimbleMarkets/ntcharts/sparkline"
	"github.com/charmbracelet/lipgloss"
	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
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
	TotalTurns      int
	OpenCircuits    int
	Providers       map[string]ProviderStat
	Alerts          []FleetAlert
	Repos           []*model.Repo
	Sessions        []*session.Session
	Teams           []*session.TeamStatus
	Events          []events.Event
	CostHistory     []float64
	CostPerTurn     map[string]float64
	TopExpensive    []ExpensiveSession
	RepoBudgets     []RepoBudget
	SelectedSection string
	SelectedCursor  int
	CostWindowLabel string

	// Loop health data
	HITLSnapshot  *session.HITLSnapshot
	AutonomyLevel session.AutonomyLevel
	GateReports   map[string]*e2e.GateReport // keyed by repo name
}

// RenderFleetDashboard renders the fleet-wide monitoring dashboard.
func RenderFleetDashboard(data FleetData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Fleet Dashboard", styles.IconFleet)))
	b.WriteString("\n\n")

	// Stats bar — horizontal row of boxed stats with icons
	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s REPOS\n  %d", styles.IconRepo, data.TotalRepos)),
		styles.StatBox.Render(fmt.Sprintf("%s LOOPS\n  %d run / %d pause", styles.IconRunning, data.RunningLoops, data.PausedLoops)),
		styles.StatBox.Render(fmt.Sprintf("%s SESSIONS\n  %d / %d run", styles.IconSession, data.TotalSessions, data.RunningSessions)),
		styles.StatBox.Render(fmt.Sprintf("%s SPEND\n  $%.2f", styles.IconBudget, data.TotalSpendUSD)),
		styles.StatBox.Render(fmt.Sprintf("%s TURNS\n  %d", styles.IconTurns, data.TotalTurns)),
	}

	circuitBox := fmt.Sprintf("%s CIRCUITS\n  %d open", styles.IconCBClosed, data.OpenCircuits)
	if data.OpenCircuits > 0 {
		circuitBox = fmt.Sprintf("%s CIRCUITS\n  %s", styles.IconCBOpen, styles.StatusFailed.Render(fmt.Sprintf("%d open", data.OpenCircuits)))
	}
	statBoxes = append(statBoxes, styles.StatBox.Render(circuitBox))

	// HITL score stat box
	if data.HITLSnapshot != nil && data.HITLSnapshot.TotalActions > 0 {
		autoRate := 1 - (float64(data.HITLSnapshot.ManualInterventions) / float64(data.HITLSnapshot.TotalActions))
		hitlLabel := fmt.Sprintf("%.0f%% auto", autoRate*100)
		hitlStyle := styles.StatusRunning
		if autoRate < 0.6 {
			hitlStyle = styles.StatusFailed
		} else if autoRate < 0.8 {
			hitlStyle = styles.WarningStyle
		}
		statBoxes = append(statBoxes, styles.StatBox.Render(
			fmt.Sprintf("%s HITL\n  %s", styles.IconSession, hitlStyle.Render(hitlLabel))))
	}

	// Autonomy level stat box
	statBoxes = append(statBoxes, styles.StatBox.Render(
		fmt.Sprintf("%s AUTONOMY\n  L%d %s", styles.IconConfig, data.AutonomyLevel, data.AutonomyLevel.String())))

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
		title := fmt.Sprintf("%s Cost Trend", styles.IconCost)
		if data.CostWindowLabel != "" {
			title += " (" + data.CostWindowLabel + ")"
		}
		b.WriteString(styles.HeaderStyle.Render(title))
		b.WriteString("\n")
		b.WriteString(sl.View())
		b.WriteString("\n\n")
	}

	// Provider breakdown with mini gauges
	if len(data.Providers) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Provider Breakdown", styles.IconSession)))
		b.WriteString("\n")
		for _, provider := range []string{"claude", "gemini", "codex"} {
			stat, ok := data.Providers[provider]
			if !ok {
				continue
			}
			gauge := ""
			if data.TotalSessions > 0 {
				gauge = components.InlineGauge(float64(stat.Running), float64(stat.Sessions), 6)
			}
			b.WriteString(fmt.Sprintf("  %s %-8s %s %d/%d sess  $%.2f",
				styles.ProviderIcon(provider),
				styles.ProviderStyle(provider).Render(provider),
				gauge,
				stat.Running, stat.Sessions,
				stat.SpendUSD))
			if cpt, ok := data.CostPerTurn[provider]; ok && cpt > 0 {
				b.WriteString(fmt.Sprintf("  $%.4f/turn", cpt))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Gate status per repo
	if len(data.GateReports) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Gate Status", styles.IconCBClosed)))
		b.WriteString("\n")
		// Sort repo names for determinism
		names := make([]string, 0, len(data.GateReports))
		for name := range data.GateReports {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			report := data.GateReports[name]
			badge := components.GateVerdictBadge(string(report.Overall))
			summary := components.GateReportSummary(report.Results)
			b.WriteString(fmt.Sprintf("  %-16s %s  %s\n", name, badge, summary))
		}
		b.WriteString("\n")
	}

	// Budget utilization gauges
	if len(data.RepoBudgets) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Budget Utilization", styles.IconBudget)))
		b.WriteString("\n")
		for _, rb := range data.RepoBudgets {
			label := fmt.Sprintf("$%.2f/$%.2f", rb.SpendUSD, rb.BudgetUSD)
			pct := 0.0
			if rb.BudgetUSD > 0 {
				pct = rb.SpendUSD / rb.BudgetUSD * 100
			}
			gauge := components.InlineGauge(rb.SpendUSD, rb.BudgetUSD, 20)
			b.WriteString(fmt.Sprintf("  %-16s %s %.0f%% %s\n",
				rb.Name, gauge, pct, label))
		}
		b.WriteString("\n")
	}

	// Top expensive sessions
	if len(data.TopExpensive) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Top Sessions by Spend", styles.IconCost)))
		b.WriteString("\n")
		for _, es := range data.TopExpensive {
			id := es.ID
			if len(id) > 8 {
				id = id[:8]
			}
			repo := es.RepoName
			if len(repo) > 12 {
				repo = repo[:12] + "…"
			}
			b.WriteString(fmt.Sprintf("  %-10s %s %-8s %-14s $%-9.2f %s %s\n",
				id,
				styles.ProviderIcon(es.Provider),
				styles.ProviderStyle(es.Provider).Render(es.Provider),
				repo,
				es.SpendUSD,
				styles.StatusIcon(es.Status),
				styles.StatusStyle(es.Status).Render(es.Status)))
		}
		b.WriteString("\n")
	}

	// Event Feed
	if len(data.Events) > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Recent Events", styles.IconAlert)))
		b.WriteString("\n")
		shown := data.Events
		if len(shown) > 10 {
			shown = shown[len(shown)-10:]
		}
		for _, ev := range shown {
			ts := ev.Timestamp.Format("15:04:05")
			icon := eventTypeIcon(ev.Type)
			detail := ""
			if ev.RepoName != "" {
				detail = ev.RepoName
			}
			if ev.SessionID != "" {
				sid := ev.SessionID
				if len(sid) > 8 {
					sid = sid[:8]
				}
				if detail != "" {
					detail += " "
				}
				detail += sid
			}
			b.WriteString(fmt.Sprintf("  %s %s %s %s\n",
				styles.InfoStyle.Render(ts),
				icon,
				eventTypeLabel(ev.Type),
				detail))
		}
		b.WriteString("\n")
	}

	// Alerts
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Alerts", styles.IconAlert)))
	b.WriteString("\n")
	if len(data.Alerts) == 0 {
		b.WriteString(styles.StatusRunning.Render(fmt.Sprintf("  %s No alerts", styles.IconCompleted)))
		b.WriteString("\n")
	} else {
		for _, alert := range data.Alerts {
			b.WriteString(fmt.Sprintf("  %s  %s\n",
				styles.AlertIcon(alert.Severity),
				alert.Message))
		}
	}
	b.WriteString("\n")

	// Repo + session + team lists with selection markers
	var repoList, sessionList, teamList strings.Builder

	repoList.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Repos", styles.IconRepo)))
	repoList.WriteString("\n")
	for i, r := range data.Repos {
		status := r.StatusDisplay()
		budgetStr := ""
		if r.Status != nil && r.Status.SessionSpendUSD > 0 {
			budgetStr = fmt.Sprintf(" $%.2f", r.Status.SessionSpendUSD)
		}
		repoList.WriteString(fmt.Sprintf("%s %s %-14s %s%s\n",
			fleetMarker(data.SelectedSection == "repos" && data.SelectedCursor == i),
			styles.StatusIcon(status),
			r.Name,
			styles.StatusStyle(status).Render(status),
			budgetStr))
	}

	sessionList.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Sessions", styles.IconSession)))
	sessionList.WriteString("\n")
	hasSessions := false
	for i, s := range data.Sessions {
		s.Lock()
		id := s.ID
		if len(id) > 8 {
			id = id[:8]
		}
		provider := string(s.Provider)
		repo := s.RepoName
		spent := s.SpentUSD
		status := string(s.Status)
		s.Unlock()
		sessionList.WriteString(fmt.Sprintf("%s %-8s %s %-7s %-10s $%.2f %s\n",
			fleetMarker(data.SelectedSection == "sessions" && data.SelectedCursor == i),
			id,
			styles.ProviderIcon(provider),
			provider,
			truncateLabel(repo, 10),
			spent,
			styles.StatusStyle(status).Render(status)))
		hasSessions = true
	}
	if !hasSessions {
		sessionList.WriteString(styles.InfoStyle.Render("  None"))
		sessionList.WriteString("\n")
	}

	teamList.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Teams", styles.IconTeam)))
	teamList.WriteString("\n")
	if len(data.Teams) == 0 {
		teamList.WriteString(styles.InfoStyle.Render("  None"))
		teamList.WriteString("\n")
	} else {
		for i, team := range data.Teams {
			teamList.WriteString(fmt.Sprintf("%s %-12s %s %d tasks\n",
				fleetMarker(data.SelectedSection == "teams" && data.SelectedCursor == i),
				truncateLabel(team.Name, 12),
				styles.StatusStyle(string(team.Status)).Render(string(team.Status)),
				len(team.Tasks)))
		}
	}

	panelWidth := width/3 - 2
	if panelWidth < 24 {
		panelWidth = 24
	}
	leftPanel := lipgloss.NewStyle().Width(panelWidth).Render(repoList.String())
	midPanel := lipgloss.NewStyle().Width(panelWidth).Render(sessionList.String())
	rightPanel := lipgloss.NewStyle().Width(panelWidth).Render(teamList.String())
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, midPanel, rightPanel))
	b.WriteString("\n\n")
	b.WriteString(styles.HelpStyle.Render("  Tab/←/→: section  j/k: move  Enter: open  X: stop  d: diff  t: timeline  [ ]: time window"))

	return b.String()
}

func fleetMarker(selected bool) string {
	if selected {
		return styles.SelectedStyle.Render(">")
	}
	return " "
}

func truncateLabel(label string, width int) string {
	if width <= 0 || len([]rune(label)) <= width {
		return label
	}
	return string([]rune(label)[:width-1]) + "…"
}

// eventTypeIcon returns an icon for an event type.
func eventTypeIcon(t events.EventType) string {
	switch t {
	case events.SessionStarted:
		return styles.StatusRunning.Render(styles.IconRunning)
	case events.SessionEnded, events.SessionStopped:
		return styles.StatusIdle.Render(styles.IconStopped)
	case events.CostUpdate:
		return styles.WarningStyle.Render(styles.IconBudget)
	case events.BudgetExceeded:
		return styles.StatusFailed.Render(styles.IconCritical)
	case events.LoopStarted:
		return styles.StatusRunning.Render(styles.IconRunning)
	case events.LoopStopped:
		return styles.StatusIdle.Render(styles.IconStopped)
	case events.TeamCreated:
		return styles.StatusCompleted.Render(styles.IconTeam)
	default:
		return styles.InfoStyle.Render(styles.IconInfo)
	}
}

// eventTypeLabel returns a short colored label for an event type.
func eventTypeLabel(t events.EventType) string {
	parts := strings.SplitN(string(t), ".", 2)
	if len(parts) < 2 {
		return styles.InfoStyle.Render(string(t))
	}
	return styles.HeaderStyle.Render(parts[0]) + "." + parts[1]
}

// formatTimeAgo is used for event timestamps relative display.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}
