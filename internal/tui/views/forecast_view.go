package views

import (
	"fmt"
	"math"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ForecastRange controls the forecast projection window.
type ForecastRange int

const (
	ForecastRange1H  ForecastRange = iota // 1 hour
	ForecastRange4H                   // 4 hours
	ForecastRange12H                  // 12 hours
	ForecastRange24H                  // 24 hours
)

// String returns a human-readable label for the time range.
func (tr ForecastRange) String() string {
	switch tr {
	case ForecastRange1H:
		return "1h"
	case ForecastRange4H:
		return "4h"
	case ForecastRange12H:
		return "12h"
	case ForecastRange24H:
		return "24h"
	default:
		return "1h"
	}
}

// Hours returns the duration in hours.
func (tr ForecastRange) Hours() float64 {
	switch tr {
	case ForecastRange1H:
		return 1
	case ForecastRange4H:
		return 4
	case ForecastRange12H:
		return 12
	case ForecastRange24H:
		return 24
	default:
		return 1
	}
}

// ForecastRefreshMsg signals the forecast view to refresh data.
type ForecastRefreshMsg struct{}

// ProviderCost holds cost data for a single provider.
type ProviderCost struct {
	Provider     string
	CurrentSpend float64
	SessionCount int
}

// HourlySpend represents spend in a single time bucket.
type HourlySpend struct {
	Hour   time.Time
	Amount float64
}

// ForecastData holds all data needed to render the forecast view.
type ForecastData struct {
	TotalBudget   float64
	CurrentSpend  float64
	Providers     []ProviderCost
	HourlySpends  []HourlySpend
	StartTime     time.Time
	ActiveSessions int
}

// ForecastView displays a cost forecasting dashboard.
type ForecastView struct {
	data      ForecastData
	timeRange ForecastRange
	width     int
	height    int
}

// NewForecastView creates a new ForecastView with default settings.
func NewForecastView() ForecastView {
	return ForecastView{
		timeRange: ForecastRange1H,
	}
}

// SetData updates the forecast view with new cost data.
func (v *ForecastView) SetData(data ForecastData) {
	v.data = data
}

// SetDimensions updates the available width and height.
func (v *ForecastView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
}

// Data returns the current forecast data (for testing).
func (v *ForecastView) Data() ForecastData {
	return v.data
}

// CurrentForecastRange returns the active time range (for testing).
func (v *ForecastView) CurrentForecastRange() ForecastRange {
	return v.timeRange
}

// Init implements tea.Model.
func (v ForecastView) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (v ForecastView) Update(msg tea.Msg) (ForecastView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "r":
			return v, func() tea.Msg { return ForecastRefreshMsg{} }
		case "t":
			v.cycleForecastRange()
		}
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
	}
	return v, nil
}

// View implements tea.Model.
func (v ForecastView) View() string {
	return v.Render()
}

// Render returns the view content as a string (implements View interface).
func (v ForecastView) Render() string {
	var b strings.Builder

	// Title
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("  %s Cost Forecast", styles.IconCost)))
	b.WriteString("\n")
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("\u2500", v.effectiveWidth())))
	b.WriteString("\n\n")

	// Summary stat boxes
	b.WriteString(v.renderSummaryBoxes())
	b.WriteString("\n\n")

	// Budget alerts
	alerts := v.renderBudgetAlerts()
	if alerts != "" {
		b.WriteString(alerts)
		b.WriteString("\n\n")
	}

	// Sparkline chart
	b.WriteString(v.renderSparkline())
	b.WriteString("\n\n")

	// Provider breakdown
	b.WriteString(v.renderProviderBreakdown())
	b.WriteString("\n\n")

	// Recommendations
	recs := v.renderRecommendations()
	if recs != "" {
		b.WriteString(recs)
		b.WriteString("\n\n")
	}

	// Footer
	b.WriteString(styles.HelpStyle.Render(fmt.Sprintf(
		"  r:refresh  t:time range (%s)  q:back", v.timeRange)))

	return b.String()
}

// effectiveWidth returns width with a sensible minimum.
func (v ForecastView) effectiveWidth() int {
	if v.width < 40 {
		return 80
	}
	return v.width
}

// ProjectedSpend calculates the linear extrapolation of current spend over the time range.
func (v ForecastView) ProjectedSpend() float64 {
	elapsed := v.elapsedHours()
	if elapsed <= 0 {
		return v.data.CurrentSpend
	}
	hourlyRate := v.data.CurrentSpend / elapsed
	return v.data.CurrentSpend + hourlyRate*v.timeRange.Hours()
}

// BudgetRemaining returns budget minus current spend, floored at zero.
func (v ForecastView) BudgetRemaining() float64 {
	rem := v.data.TotalBudget - v.data.CurrentSpend
	if rem < 0 {
		return 0
	}
	return rem
}

// CostPerSession returns average cost across active sessions.
func (v ForecastView) CostPerSession() float64 {
	if v.data.ActiveSessions <= 0 {
		return 0
	}
	return v.data.CurrentSpend / float64(v.data.ActiveSessions)
}

// BudgetPercent returns the percentage of budget consumed (0-100).
func (v ForecastView) BudgetPercent() float64 {
	if v.data.TotalBudget <= 0 {
		return 0
	}
	pct := (v.data.CurrentSpend / v.data.TotalBudget) * 100
	if pct > 100 {
		return 100
	}
	return pct
}

// BudgetAlertLevel returns the threshold level: "", "50", "75", "90", "100".
func (v ForecastView) BudgetAlertLevel() string {
	pct := v.BudgetPercent()
	switch {
	case pct >= 100:
		return "100"
	case pct >= 90:
		return "90"
	case pct >= 75:
		return "75"
	case pct >= 50:
		return "50"
	default:
		return ""
	}
}

// elapsedHours returns how long the session has been running.
func (v ForecastView) elapsedHours() float64 {
	if v.data.StartTime.IsZero() {
		return 0
	}
	return time.Since(v.data.StartTime).Hours()
}

func (v ForecastView) renderSummaryBoxes() string {
	projected := v.ProjectedSpend()
	remaining := v.BudgetRemaining()
	perSession := v.CostPerSession()

	boxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s SPENT\n  $%.2f", styles.IconBudget, v.data.CurrentSpend)),
		styles.StatBox.Render(fmt.Sprintf("%s PROJECTED\n  $%.2f", styles.IconCost, projected)),
		styles.StatBox.Render(fmt.Sprintf("%s REMAINING\n  $%.2f", styles.IconBudget, remaining)),
		styles.StatBox.Render(fmt.Sprintf("%s PER SESSION\n  $%.4f", styles.IconSession, perSession)),
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, boxes...)
}

func (v ForecastView) renderBudgetAlerts() string {
	level := v.BudgetAlertLevel()
	if level == "" {
		return ""
	}

	pct := v.BudgetPercent()
	var b strings.Builder
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Budget Alerts", styles.IconAlert)))
	b.WriteString("\n")

	switch level {
	case "100":
		b.WriteString(styles.AlertCritical.Render(
			fmt.Sprintf("  %s BUDGET EXCEEDED — %.1f%% of $%.2f consumed",
				styles.IconCritical, pct, v.data.TotalBudget)))
	case "90":
		b.WriteString(styles.AlertCritical.Render(
			fmt.Sprintf("  %s CRITICAL — %.1f%% of budget consumed",
				styles.IconCritical, pct)))
	case "75":
		b.WriteString(styles.AlertWarning.Render(
			fmt.Sprintf("  %s WARNING — %.1f%% of budget consumed",
				styles.IconWarning, pct)))
	case "50":
		b.WriteString(styles.AlertInfo.Render(
			fmt.Sprintf("  %s INFO — %.1f%% of budget consumed",
				styles.IconInfo, pct)))
	}

	return b.String()
}

// Sparkline renders an ASCII sparkline from hourly spend data.
func (v ForecastView) renderSparkline() string {
	var b strings.Builder
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Hourly Spend", styles.IconCost)))
	b.WriteString("\n")

	if len(v.data.HourlySpends) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No spend data yet."))
		return b.String()
	}

	b.WriteString("  ")
	b.WriteString(SparklineFromValues(extractAmounts(v.data.HourlySpends)))

	// Legend: min and max
	_, maxVal := minMax(extractAmounts(v.data.HourlySpends))
	b.WriteString("\n")
	b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("  peak: $%.4f/hr  buckets: %d",
		maxVal, len(v.data.HourlySpends))))

	return b.String()
}

func (v ForecastView) renderProviderBreakdown() string {
	var b strings.Builder
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Cost by Provider", styles.IconBudget)))
	b.WriteString("\n")

	if len(v.data.Providers) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No provider data."))
		return b.String()
	}

	for _, p := range v.data.Providers {
		pctOfTotal := float64(0)
		if v.data.CurrentSpend > 0 {
			pctOfTotal = (p.CurrentSpend / v.data.CurrentSpend) * 100
		}
		provStyle := styles.ProviderStyle(p.Provider)
		b.WriteString(fmt.Sprintf("  %s %-8s  $%.4f  (%4.1f%%)  %d sessions\n",
			styles.ProviderIcon(p.Provider),
			provStyle.Render(p.Provider),
			p.CurrentSpend,
			pctOfTotal,
			p.SessionCount))
	}

	return b.String()
}

func (v ForecastView) renderRecommendations() string {
	recs := v.generateRecommendations()
	if len(recs) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Recommendations", styles.IconInfo)))
	b.WriteString("\n")

	for _, r := range recs {
		b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("  - %s", r)))
		b.WriteString("\n")
	}

	return b.String()
}

// generateRecommendations produces cost-saving suggestions based on current data.
func (v ForecastView) generateRecommendations() []string {
	var recs []string

	// Find Claude and Gemini provider data
	var claudeCost, geminiCost float64
	var claudeSessions int
	for _, p := range v.data.Providers {
		switch p.Provider {
		case "claude":
			claudeCost = p.CurrentSpend
			claudeSessions = p.SessionCount
		case "gemini":
			geminiCost = p.CurrentSpend
		}
	}

	// Recommend shifting Claude sessions to Gemini if Claude dominates cost
	if claudeSessions > 0 && claudeCost > 0 {
		// Gemini is roughly 3x cheaper per token on average
		perClaudeSession := claudeCost / float64(claudeSessions)
		switchable := claudeSessions / 3
		if switchable < 1 {
			switchable = 1
		}
		if switchable > 0 && claudeSessions > 1 {
			savings := float64(switchable) * perClaudeSession * 0.66 // ~66% savings per shifted session
			recs = append(recs, fmt.Sprintf(
				"Switch %d Claude sessions to Gemini to save ~$%.2f/hr",
				switchable, savings))
		}
	}

	// Warn if projected spend exceeds budget
	projected := v.ProjectedSpend()
	if v.data.TotalBudget > 0 && projected > v.data.TotalBudget {
		overBy := projected - v.data.TotalBudget
		recs = append(recs, fmt.Sprintf(
			"Projected spend exceeds budget by $%.2f — consider reducing sessions",
			overBy))
	}

	// Suggest cost awareness if no budget set
	if v.data.TotalBudget <= 0 && v.data.CurrentSpend > 0 {
		recs = append(recs, "No budget set — configure a budget to enable alerts and forecasting")
	}

	// If Gemini is unused but Claude cost is high
	if claudeCost > 0 && geminiCost == 0 && claudeSessions > 2 {
		recs = append(recs, "Consider adding Gemini workers for cost-optimized parallelism")
	}

	return recs
}

func (v *ForecastView) cycleForecastRange() {
	switch v.timeRange {
	case ForecastRange1H:
		v.timeRange = ForecastRange4H
	case ForecastRange4H:
		v.timeRange = ForecastRange12H
	case ForecastRange12H:
		v.timeRange = ForecastRange24H
	case ForecastRange24H:
		v.timeRange = ForecastRange1H
	}
}

// SparklineFromValues renders an ASCII sparkline string from a slice of float64 values.
// Uses Unicode block characters for 8 vertical levels.
func SparklineFromValues(values []float64) string {
	if len(values) == 0 {
		return ""
	}

	bars := []rune{'\u2581', '\u2582', '\u2583', '\u2584', '\u2585', '\u2586', '\u2587', '\u2588'}

	minVal, maxVal := minMax(values)
	spread := maxVal - minVal
	if spread == 0 {
		// All values equal — render mid-height bars
		return strings.Repeat(string(bars[3]), len(values))
	}

	var b strings.Builder
	for _, val := range values {
		normalized := (val - minVal) / spread
		idx := int(math.Round(normalized * 7))
		if idx < 0 {
			idx = 0
		}
		if idx > 7 {
			idx = 7
		}
		b.WriteRune(bars[idx])
	}
	return b.String()
}

func extractAmounts(spends []HourlySpend) []float64 {
	vals := make([]float64, len(spends))
	for i, s := range spends {
		vals[i] = s.Amount
	}
	return vals
}

func minMax(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	mn, mx := values[0], values[0]
	for _, v := range values[1:] {
		if v < mn {
			mn = v
		}
		if v > mx {
			mx = v
		}
	}
	return mn, mx
}
