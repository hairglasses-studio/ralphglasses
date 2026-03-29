package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// StatusBar renders the bottom status bar.
type StatusBar struct {
	Width                int
	Mode                 string // "NORMAL", "COMMAND", "FILTER"
	Filter               string
	RepoCount            int
	RunningCount         int
	SessionCount         int
	TotalSpendUSD        float64
	AlertCount           int
	LastRefresh          time.Time
	SpinnerFrame         string
	ProviderCounts       map[string]int // running sessions per provider
	FleetBudgetPct       float64        // aggregate budget utilization (0-1)
	TickFrame            int
	HighestAlertSeverity string // "critical", "warning", "info", ""
}

// View renders the status bar.
func (s *StatusBar) View() string {
	// Mode indicator with icon
	modeStr := styles.CommandStyle.Render(s.Mode)

	// Build left sections
	var parts []string
	parts = append(parts, fmt.Sprintf(" %s", modeStr))
	parts = append(parts, fmt.Sprintf("%s %d", styles.IconRepo, s.RepoCount))
	parts = append(parts, fmt.Sprintf("%s %d", styles.IconRunning, s.RunningCount))
	parts = append(parts, fmt.Sprintf("%s %d", styles.IconSession, s.SessionCount))
	parts = append(parts, fmt.Sprintf("%s $%.2f", styles.IconBudget, s.TotalSpendUSD))

	// Per-provider running counts
	for _, p := range []string{"claude", "gemini", "codex"} {
		if count, ok := s.ProviderCounts[p]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", styles.ProviderIcon(p), count))
		}
	}

	// Fleet budget gauge (compact)
	if s.FleetBudgetPct > 0 {
		pctStr := fmt.Sprintf("%.0f%%", s.FleetBudgetPct*100)
		gauge := InlineGauge(s.FleetBudgetPct, 1.0, 5)
		parts = append(parts, fmt.Sprintf("%s %s", gauge, pctStr))
	}

	// Alerts
	if s.AlertCount > 0 {
		icon := styles.AlertIcon(s.HighestAlertSeverity)
		parts = append(parts, fmt.Sprintf("%s %d", icon, s.AlertCount))
	}

	// Spinner for running
	if s.RunningCount > 0 && s.SpinnerFrame != "" {
		parts = append(parts, s.SpinnerFrame)
	}

	left := strings.Join(parts, "  ")

	if s.Filter != "" {
		left += fmt.Sprintf("  /%s", s.Filter)
	}

	right := fmt.Sprintf("%s %s ", styles.IconClock, formatAgo(s.LastRefresh))

	padding := s.Width - VisualWidth(left) - VisualWidth(right)
	if padding < 1 {
		padding = 1
	}

	return styles.StatusBarStyle.Width(s.Width).Render(
		left + strings.Repeat(" ", padding) + right,
	)
}

// NowFunc is the clock used by formatAgo. Override in tests for determinism.
var NowFunc = time.Now

func formatAgo(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	d := NowFunc().Sub(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	return fmt.Sprintf("%dm", int(d.Minutes()))
}
