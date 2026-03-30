package components

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// StatusBar renders the bottom status bar as a full-width, multi-segment
// activity dashboard. Segments collapse from lowest priority when the
// terminal is too narrow.
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

	// Cost
	CostHistory  []float64 // rolling cost samples for sparkline
	CostVelocity float64   // $/min spend rate

	// Loops
	ActiveLoopCount int
	LoopIterTotal   int
	LoopSuccessRate float64   // 0-1
	LoopIterHistory []float64 // iteration duration series

	// Fleet
	FleetCompletions int
	FleetFailures    int
	FleetFailureRate float64
	FleetLatencyP50  float64 // ms
	FleetUtilization float64 // 0-1

	// Health
	ProviderHealthy map[string]bool // "claude"->true, etc.

	// System
	AutonomyLevel string // "L0", "L1", "L2", etc.
	Uptime        time.Duration
}

// segment pairs a priority (0 = highest) with rendered content.
type segment struct {
	priority int
	content  string
}

// View renders the status bar with priority-based segment collapse.
func (s *StatusBar) View() string {
	sep := styles.InfoStyle.Render(" │ ")
	sepWidth := 3

	segments := []segment{
		{0, s.renderMode()},
		{1, s.renderSessions()},
		{2, s.renderCost()},
		{3, s.renderLoops()},
		{4, s.renderHealth()},
		{5, s.renderFleet()},
		{6, s.renderSystem()},
	}

	// Filter empty segments.
	var active []segment
	for _, seg := range segments {
		if seg.content != "" {
			active = append(active, seg)
		}
	}

	// Collapse from lowest priority until content fits width.
	for len(active) > 1 {
		total := 0
		for i, seg := range active {
			total += VisualWidth(seg.content)
			if i > 0 {
				total += sepWidth
			}
		}
		if total <= s.Width {
			break
		}
		// Drop the segment with the highest priority number (least important).
		maxPri, maxIdx := -1, -1
		for i, seg := range active {
			if seg.priority > maxPri {
				maxPri = seg.priority
				maxIdx = i
			}
		}
		active = append(active[:maxIdx], active[maxIdx+1:]...)
	}

	// Join with separators.
	var parts []string
	for _, seg := range active {
		parts = append(parts, seg.content)
	}
	content := strings.Join(parts, sep)

	// Pad to fill width.
	padding := s.Width - VisualWidth(content)
	if padding < 0 {
		padding = 0
	}

	return styles.StatusBarStyle.Width(s.Width).Render(
		content + strings.Repeat(" ", padding),
	)
}

// renderMode renders the mode indicator + optional filter text.
func (s *StatusBar) renderMode() string {
	out := " " + styles.CommandStyle.Render(s.Mode)
	if s.Filter != "" {
		out += fmt.Sprintf(" /%s", s.Filter)
	}
	return out
}

// renderSessions renders running/total counts, per-provider breakdown, spinner.
func (s *StatusBar) renderSessions() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s %d/%d", styles.IconRunning, s.RunningCount, s.SessionCount))

	for _, p := range []string{"claude", "gemini", "codex"} {
		if count, ok := s.ProviderCounts[p]; ok && count > 0 {
			parts = append(parts, fmt.Sprintf("%s%d", styles.ProviderIcon(p), count))
		}
	}

	if s.RunningCount > 0 && s.SpinnerFrame != "" {
		parts = append(parts, s.SpinnerFrame)
	}

	return strings.Join(parts, " ")
}

// renderCost renders total spend, sparkline trend, budget gauge, velocity.
func (s *StatusBar) renderCost() string {
	var parts []string
	parts = append(parts, fmt.Sprintf("%s $%.2f", styles.IconBudget, s.TotalSpendUSD))

	if len(s.CostHistory) > 1 {
		parts = append(parts, InlineSparkline(s.CostHistory, 5))
	}

	if s.FleetBudgetPct > 0 {
		gauge := InlineGauge(s.FleetBudgetPct, 1.0, 5)
		parts = append(parts, fmt.Sprintf("%s %.0f%%", gauge, s.FleetBudgetPct*100))
	}

	if s.CostVelocity > 0.001 {
		parts = append(parts, fmt.Sprintf("$%.2f/m", s.CostVelocity))
	}

	return strings.Join(parts, " ")
}

// renderLoops renders active loop count, convergence gauge, iteration sparkline.
// Returns empty string when no loops are active.
func (s *StatusBar) renderLoops() string {
	if s.ActiveLoopCount == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%s %d", styles.IconTurns, s.ActiveLoopCount))

	if s.LoopIterTotal > 0 {
		gauge := InlineGauge(s.LoopSuccessRate, 1.0, 4)
		parts = append(parts, fmt.Sprintf("%s %.0f%%", gauge, s.LoopSuccessRate*100))
	}

	if len(s.LoopIterHistory) > 1 {
		parts = append(parts, InlineSparkline(s.LoopIterHistory, 5))
	}

	return strings.Join(parts, " ")
}

// renderHealth renders colored dots for each provider's health status.
// Returns empty string when no health data is available.
func (s *StatusBar) renderHealth() string {
	if len(s.ProviderHealthy) == 0 {
		return ""
	}

	var dots []string
	for _, p := range []string{"claude", "gemini", "codex"} {
		healthy, ok := s.ProviderHealthy[p]
		if !ok {
			continue
		}
		if healthy {
			dots = append(dots, styles.StatusRunning.Render("●"))
		} else {
			dots = append(dots, styles.StatusFailed.Render("●"))
		}
	}

	if len(dots) == 0 {
		return ""
	}
	return strings.Join(dots, "")
}

// renderFleet renders fleet completions, failures, failure rate, P50 latency.
// Returns empty string when no fleet data exists.
func (s *StatusBar) renderFleet() string {
	if s.FleetCompletions == 0 && s.FleetFailures == 0 {
		return ""
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%s ✓%d", styles.IconFleet, s.FleetCompletions))

	if s.FleetFailures > 0 {
		parts = append(parts, styles.StatusFailed.Render(fmt.Sprintf("✗%d", s.FleetFailures)))
		parts = append(parts, fmt.Sprintf("%.1f%%", s.FleetFailureRate*100))
	}

	if s.FleetLatencyP50 > 0 {
		parts = append(parts, fmt.Sprintf("p50:%.0fms", s.FleetLatencyP50))
	}

	if s.FleetUtilization > 0 {
		parts = append(parts, fmt.Sprintf("util:%.0f%%", s.FleetUtilization*100))
	}

	return strings.Join(parts, " ")
}

// renderSystem renders autonomy level, alerts, uptime, last refresh.
func (s *StatusBar) renderSystem() string {
	var parts []string

	if s.AutonomyLevel != "" {
		parts = append(parts, styles.InfoStyle.Render(s.AutonomyLevel))
	}

	if s.AlertCount > 0 {
		icon := styles.AlertIcon(s.HighestAlertSeverity)
		parts = append(parts, fmt.Sprintf("%s%d", icon, s.AlertCount))
	}

	if s.Uptime > 0 {
		parts = append(parts, formatDuration(s.Uptime))
	}

	parts = append(parts, fmt.Sprintf("%s %s", styles.IconClock, formatAgo(s.LastRefresh)))

	return strings.Join(parts, " ")
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

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}
