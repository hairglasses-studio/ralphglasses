package views

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// AnalyticsRefreshMsg signals the analytics view to refresh its data.
type AnalyticsRefreshMsg struct{}

// TimeRange represents a selectable time window for analytics data.
type TimeRange int

const (
	TimeRange1h  TimeRange = iota // 1 hour
	TimeRange24h                  // 24 hours
	TimeRange7d                   // 7 days
	TimeRange30d                  // 30 days
)

// String returns a human-readable label for the time range.
func (tr TimeRange) String() string {
	switch tr {
	case TimeRange1h:
		return "1h"
	case TimeRange24h:
		return "24h"
	case TimeRange7d:
		return "7d"
	case TimeRange30d:
		return "30d"
	default:
		return "?"
	}
}

// Duration returns the time.Duration for the range.
func (tr TimeRange) Duration() time.Duration {
	switch tr {
	case TimeRange1h:
		return time.Hour
	case TimeRange24h:
		return 24 * time.Hour
	case TimeRange7d:
		return 7 * 24 * time.Hour
	case TimeRange30d:
		return 30 * 24 * time.Hour
	default:
		return time.Hour
	}
}

var timeRanges = []TimeRange{TimeRange1h, TimeRange24h, TimeRange7d, TimeRange30d}

// AnalyticsPanel identifies which panel is currently focused.
type AnalyticsPanel int

const (
	PanelSessionCount AnalyticsPanel = iota
	PanelCostPerSession
	PanelProviderDist
	PanelSuccessRate
)

const analyticsPanelCount = 4

// String returns a display label for the panel.
func (p AnalyticsPanel) String() string {
	switch p {
	case PanelSessionCount:
		return "Session Count"
	case PanelCostPerSession:
		return "Cost / Session"
	case PanelProviderDist:
		return "Provider Distribution"
	case PanelSuccessRate:
		return "Success / Failure"
	default:
		return "?"
	}
}

// SessionTimeBucket holds session count for a time bucket.
type SessionTimeBucket struct {
	Time  time.Time
	Count int
}

// ProviderShare holds a provider's session count for the distribution chart.
type ProviderShare struct {
	Provider string
	Count    int
}

// AnalyticsData holds all data needed to render the analytics dashboard.
type AnalyticsData struct {
	// Session count over time (bucketed)
	SessionCounts []SessionTimeBucket

	// Cost per session over time
	CostPerSession []float64

	// Provider distribution
	Providers []ProviderShare

	// Success / failure counts
	Succeeded int
	Failed    int
	Running   int
	Total     int

	// Total cost across all sessions
	TotalCost float64
}

// AnalyticsView is the analytics dashboard view.
// It implements both the View interface (Render/SetDimensions) and ViewHandler
// (Render(w,h)/HandleKey/SetDimensions) so it can be used either way.
type AnalyticsView struct {
	Viewport *ViewportView
	data     AnalyticsData
	panel    AnalyticsPanel
	timeIdx  int // index into timeRanges
	width    int
	height   int
}

// NewAnalyticsView creates a new AnalyticsView.
func NewAnalyticsView() *AnalyticsView {
	return &AnalyticsView{
		Viewport: NewViewportView(),
		timeIdx:  1, // default 24h
	}
}

// SetData updates the analytics data and regenerates.
func (v *AnalyticsView) SetData(data AnalyticsData) {
	v.data = data
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *AnalyticsView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content (implements View interface).
func (v *AnalyticsView) Render() string {
	return v.Viewport.Render()
}

// RenderAt returns the rendered content for given dimensions (implements ViewHandler).
func (v *AnalyticsView) RenderAt(width, height int) string {
	v.SetDimensions(width, height)
	return v.Render()
}

// HandleKey processes analytics-specific key events.
// Returns true if the key was handled, plus an optional tea.Cmd.
func (v *AnalyticsView) HandleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "tab":
		v.panel = AnalyticsPanel((int(v.panel) + 1) % analyticsPanelCount)
		v.regenerate()
		return true, nil
	case "shift+tab":
		p := int(v.panel) - 1
		if p < 0 {
			p = analyticsPanelCount - 1
		}
		v.panel = AnalyticsPanel(p)
		v.regenerate()
		return true, nil
	case "r":
		return true, func() tea.Msg { return AnalyticsRefreshMsg{} }
	case "[":
		if v.timeIdx > 0 {
			v.timeIdx--
			v.regenerate()
		}
		return true, nil
	case "]":
		if v.timeIdx < len(timeRanges)-1 {
			v.timeIdx++
			v.regenerate()
		}
		return true, nil
	}
	return false, nil
}

// Panel returns the currently focused panel.
func (v *AnalyticsView) Panel() AnalyticsPanel {
	return v.panel
}

// TimeRange returns the currently selected time range.
func (v *AnalyticsView) TimeRange() TimeRange {
	if v.timeIdx < 0 || v.timeIdx >= len(timeRanges) {
		return TimeRange24h
	}
	return timeRanges[v.timeIdx]
}

// regenerate rebuilds the rendered content from current data.
func (v *AnalyticsView) regenerate() {
	content := RenderAnalytics(v.data, v.panel, v.TimeRange(), v.width, v.height)
	v.Viewport.SetContent(content)
}

// RenderAnalytics renders the full analytics dashboard.
func RenderAnalytics(data AnalyticsData, activePanel AnalyticsPanel, tr TimeRange, width, height int) string {
	var b strings.Builder

	// Title
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Analytics Dashboard", styles.IconCost)))
	b.WriteString("\n\n")

	// Time range selector
	b.WriteString(renderTimeRangeSelector(tr))
	b.WriteString("\n\n")

	// Summary stat boxes
	avgCost := 0.0
	if data.Total > 0 {
		avgCost = data.TotalCost / float64(data.Total)
	}
	successRate := 0.0
	completed := data.Succeeded + data.Failed
	if completed > 0 {
		successRate = float64(data.Succeeded) / float64(completed) * 100
	}

	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s SESSIONS\n  %d total", styles.IconSession, data.Total)),
		styles.StatBox.Render(fmt.Sprintf("%s COST\n  $%.2f total", styles.IconBudget, data.TotalCost)),
		styles.StatBox.Render(fmt.Sprintf("%s AVG COST\n  $%.4f/sess", styles.IconCost, avgCost)),
		styles.StatBox.Render(fmt.Sprintf("%s SUCCESS\n  %.1f%%", styles.IconCompleted, successRate)),
	}
	b.WriteString(wrapStatBoxes(statBoxes, width))
	b.WriteString("\n")

	// Four panels — highlight the active one
	sparkWidth := max(width-20, 8)
	if sparkWidth > 120 {
		sparkWidth = 120
	}

	// Panel 1: Session count over time
	b.WriteString(renderPanelHeader("Session Count Over Time", PanelSessionCount, activePanel))
	b.WriteString("\n")
	if len(data.SessionCounts) > 0 {
		vals := make([]float64, len(data.SessionCounts))
		for i, bucket := range data.SessionCounts {
			vals[i] = float64(bucket.Count)
		}
		b.WriteString("  ")
		b.WriteString(components.Sparkline(vals, sparkWidth))
		b.WriteString(fmt.Sprintf("  (latest: %d)", data.SessionCounts[len(data.SessionCounts)-1].Count))
	} else {
		b.WriteString(styles.InfoStyle.Render("  (no data)"))
	}
	b.WriteString("\n\n")

	// Panel 2: Cost per session
	b.WriteString(renderPanelHeader("Cost Per Session", PanelCostPerSession, activePanel))
	b.WriteString("\n")
	if len(data.CostPerSession) > 0 {
		b.WriteString("  ")
		b.WriteString(components.Sparkline(data.CostPerSession, sparkWidth))
		latest := data.CostPerSession[len(data.CostPerSession)-1]
		b.WriteString(fmt.Sprintf("  ($%.4f latest)", latest))
	} else {
		b.WriteString(styles.InfoStyle.Render("  (no data)"))
	}
	b.WriteString("\n\n")

	// Panel 3: Provider distribution (horizontal bar chart)
	b.WriteString(renderPanelHeader("Provider Distribution", PanelProviderDist, activePanel))
	b.WriteString("\n")
	if len(data.Providers) > 0 {
		b.WriteString(renderProviderBars(data.Providers, sparkWidth))
	} else {
		b.WriteString(styles.InfoStyle.Render("  (no data)"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Panel 4: Success / failure rates
	b.WriteString(renderPanelHeader("Success / Failure Rates", PanelSuccessRate, activePanel))
	b.WriteString("\n")
	b.WriteString(renderSuccessFailure(data, sparkWidth))
	b.WriteString("\n")

	// Help footer
	b.WriteString(styles.HelpStyle.Render("  Tab:panel  [/]:time range  r:refresh"))

	return b.String()
}

// renderTimeRangeSelector renders the time range pills.
func renderTimeRangeSelector(active TimeRange) string {
	var parts []string
	for _, tr := range timeRanges {
		label := " " + tr.String() + " "
		if tr == active {
			parts = append(parts, styles.TabActive.Render(label))
		} else {
			parts = append(parts, styles.TabInactive.Render(label))
		}
	}
	return "  " + strings.Join(parts, " ")
}

// renderPanelHeader renders a section header with active-panel highlighting.
func renderPanelHeader(title string, panel, activePanel AnalyticsPanel) string {
	marker := "  "
	if panel == activePanel {
		marker = styles.SelectedStyle.Render("> ")
	}
	return marker + styles.HeaderStyle.Render(title)
}

// renderProviderBars renders horizontal bar charts for provider distribution.
func renderProviderBars(providers []ProviderShare, barWidth int) string {
	if barWidth <= 0 {
		barWidth = 20
	}

	maxCount := 0
	for _, p := range providers {
		if p.Count > maxCount {
			maxCount = p.Count
		}
	}
	if maxCount == 0 {
		maxCount = 1
	}

	var b strings.Builder
	for _, p := range providers {
		filled := max(int(math.Round(float64(p.Count)/float64(maxCount)*float64(barWidth))), 0)
		if filled > barWidth {
			filled = barWidth
		}
		bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)
		icon := styles.ProviderIcon(p.Provider)
		styledName := styles.ProviderStyle(p.Provider).Render(fmt.Sprintf("%-8s", p.Provider))
		b.WriteString(fmt.Sprintf("  %s %s %s %d\n", icon, styledName, bar, p.Count))
	}
	return b.String()
}

// renderSuccessFailure renders success/failure/running breakdown with gauges.
func renderSuccessFailure(data AnalyticsData, barWidth int) string {
	var b strings.Builder

	total := float64(data.Total)
	if total == 0 {
		b.WriteString(styles.InfoStyle.Render("  (no data)"))
		b.WriteString("\n")
		return b.String()
	}

	rows := []struct {
		label string
		count int
		style lipgloss.Style
	}{
		{"succeeded", data.Succeeded, styles.StatusRunning},
		{"failed", data.Failed, styles.StatusFailed},
		{"running", data.Running, styles.WarningStyle},
	}

	for _, row := range rows {
		pct := float64(row.count) / total * 100
		gauge := components.InlineGauge(float64(row.count), total, barWidth)
		b.WriteString(fmt.Sprintf("  %-12s %s %4d (%5.1f%%)\n",
			row.style.Render(row.label), gauge, row.count, pct))
	}
	return b.String()
}
