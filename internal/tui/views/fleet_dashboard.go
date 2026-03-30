package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// FleetSession represents a single session in the fleet dashboard table.
type FleetSession struct {
	ID       string
	Provider string
	Status   string
	Cost     float64
	Duration time.Duration
	Task     string
}

// FleetDashboardRefreshMsg signals the fleet dashboard to refresh its data.
type FleetDashboardRefreshMsg struct{}

// FleetDashboardSelectMsg signals that a session was selected.
type FleetDashboardSelectMsg struct {
	SessionID string
}

// FleetDashboardModel implements tea.Model for the fleet dashboard view.
type FleetDashboardModel struct {
	sessions  []FleetSession
	cursor    int
	width     int
	height    int
}

// NewFleetDashboard creates a new FleetDashboardModel.
func NewFleetDashboard() FleetDashboardModel {
	return FleetDashboardModel{}
}

// SetSessions updates the session list displayed in the dashboard.
func (m *FleetDashboardModel) SetSessions(sessions []FleetSession) {
	m.sessions = sessions
	if m.cursor >= len(sessions) {
		m.cursor = max(0, len(sessions)-1)
	}
}

// Init implements tea.Model.
func (m FleetDashboardModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m FleetDashboardModel) Update(msg tea.Msg) (FleetDashboardModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return m, func() tea.Msg { return FleetDashboardRefreshMsg{} }
		case "q":
			return m, tea.Quit
		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "down", "j":
			if m.cursor < len(m.sessions)-1 {
				m.cursor++
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.sessions) {
				sid := m.sessions[m.cursor].ID
				return m, func() tea.Msg { return FleetDashboardSelectMsg{SessionID: sid} }
			}
		}
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	}
	return m, nil
}

// View implements tea.Model.
func (m FleetDashboardModel) View() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Fleet Dashboard", styles.IconFleet)))
	b.WriteString("\n\n")

	// Aggregate stats
	active, idle, failed := m.countByStatus()
	totalCost := m.totalCost()
	costRate := m.costRate()

	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s SESSIONS\n  %d total", styles.IconSession, len(m.sessions))),
		styles.StatBox.Render(fmt.Sprintf("%s ACTIVE\n  %d", styles.IconRunning, active)),
		styles.StatBox.Render(fmt.Sprintf("%s IDLE\n  %d", styles.IconIdle, idle)),
		styles.StatBox.Render(fmt.Sprintf("%s FAILED\n  %d", styles.IconErrored, failed)),
		styles.StatBox.Render(fmt.Sprintf("%s COST\n  $%.2f", styles.IconBudget, totalCost)),
		styles.StatBox.Render(fmt.Sprintf("%s RATE\n  $%.4f/s", styles.IconCost, costRate)),
	}
	b.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, statBoxes...))
	b.WriteString("\n\n")

	// Session table
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Sessions", styles.IconSession)))
	b.WriteString("\n")

	if len(m.sessions) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No sessions"))
		b.WriteString("\n")
	} else {
		// Header row
		header := fmt.Sprintf("  %-10s %-10s %-10s %10s %12s  %s",
			"ID", "Provider", "Status", "Cost", "Duration", "Task")
		b.WriteString(styles.HeaderStyle.Render(header))
		b.WriteString("\n")

		for i, s := range m.sessions {
			id := s.ID
			if len(id) > 8 {
				id = id[:8]
			}

			providerStr := styles.ProviderStyle(s.Provider).Render(fmt.Sprintf("%-10s", s.Provider))
			statusStr := styles.StatusStyle(s.Status).Render(fmt.Sprintf("%-10s", s.Status))
			costStr := fmt.Sprintf("$%.4f", s.Cost)
			durStr := fmtDuration(s.Duration)
			task := s.Task
			if len(task) > 30 {
				task = task[:27] + "..."
			}

			marker := "  "
			if i == m.cursor {
				marker = styles.SelectedStyle.Render("> ")
			}

			line := fmt.Sprintf("%s%-10s %s %s %10s %12s  %s",
				marker, id, providerStr, statusStr, costStr, durStr, task)
			b.WriteString(line)
			b.WriteString("\n")
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  r:refresh  q:back  j/k:move  Enter:select"))

	return b.String()
}

// countByStatus returns active, idle, and failed session counts.
func (m FleetDashboardModel) countByStatus() (active, idle, failed int) {
	for _, s := range m.sessions {
		switch s.Status {
		case "running":
			active++
		case "idle", "stopped":
			idle++
		case "failed", "errored":
			failed++
		}
	}
	return
}

// totalCost sums Cost across all sessions.
func (m FleetDashboardModel) totalCost() float64 {
	var total float64
	for _, s := range m.sessions {
		total += s.Cost
	}
	return total
}

// costRate calculates total cost divided by total duration seconds across running sessions.
func (m FleetDashboardModel) costRate() float64 {
	var totalCost float64
	var totalSec float64
	for _, s := range m.sessions {
		if s.Status == "running" && s.Duration > 0 {
			totalCost += s.Cost
			totalSec += s.Duration.Seconds()
		}
	}
	if totalSec == 0 {
		return 0
	}
	return totalCost / totalSec
}

// fmtDuration renders a duration as a human-friendly string.
func fmtDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
