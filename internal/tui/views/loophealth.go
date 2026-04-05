package views

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LoopHealthData holds everything needed to render the loop health view.
type LoopHealthData struct {
	RepoName     string
	Observations []session.LoopObservation
	GateReport   *e2e.GateReport
	Summary      *e2e.Summary
	Baseline     *e2e.LoopBaseline
}

// RenderLoopHealth renders the loop health dashboard for a single repo.
func RenderLoopHealth(data LoopHealthData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Loop Health: %s", styles.IconRunning, data.RepoName)))
	b.WriteString("\n\n")

	if len(data.Observations) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No loop observations recorded yet."))
		b.WriteString("\n")
		b.WriteString(styles.InfoStyle.Render("  Run loop cycles to populate this view."))
		return b.String()
	}

	// Panel 1: Gate Summary with sparklines
	b.WriteString(renderGateSummary(data))
	b.WriteString("\n")

	// Panel 2: Recent Iterations table
	b.WriteString(renderIterationTable(data, width))
	b.WriteString("\n")

	// Panel 3: Task Type Distribution
	if data.Summary != nil && data.Summary.CrossScenario != nil {
		b.WriteString(renderTaskDistribution(data.Summary.CrossScenario.TaskTypeDist, width))
	}

	return b.String()
}

func renderGateSummary(data LoopHealthData) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Regression Gates", styles.IconCBClosed)))
	b.WriteString("\n")

	if data.GateReport == nil || len(data.GateReport.Results) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No gate data available"))
		b.WriteString("\n")
		return b.String()
	}

	// Overall verdict
	b.WriteString(fmt.Sprintf("  Overall: %s  %s\n",
		components.GateVerdictBadge(string(data.GateReport.Overall)),
		components.GateReportSummary(data.GateReport.Results)))
	b.WriteString(fmt.Sprintf("  Samples: %d observations\n", data.GateReport.SampleCount))

	// Per-metric rows with sparklines
	costData := extractMetricSeries(data.Observations, "cost")
	latencyData := extractMetricSeries(data.Observations, "latency")

	for _, r := range data.GateReport.Results {
		badge := components.GateVerdictBadge(string(r.Verdict))

		// Choose sparkline data based on metric
		var spark string
		switch {
		case strings.Contains(r.Metric, "cost"):
			spark = components.HealthSparkline(costData, r.BaselineVal*1.3, 12)
		case strings.Contains(r.Metric, "latency"):
			spark = components.HealthSparkline(latencyData, r.BaselineVal*1.5, 12)
		}

		deltaStr := ""
		if r.DeltaPct != 0 {
			sign := "+"
			if r.DeltaPct < 0 {
				sign = ""
			}
			deltaStr = fmt.Sprintf("%s%.1f%%", sign, r.DeltaPct)
		}

		baselineStr := ""
		if r.BaselineVal > 0 {
			if strings.Contains(r.Metric, "cost") {
				baselineStr = fmt.Sprintf("base: $%.3f", r.BaselineVal)
			} else if strings.Contains(r.Metric, "latency") {
				baselineStr = fmt.Sprintf("base: %.1fs", r.BaselineVal/1000)
			} else {
				baselineStr = fmt.Sprintf("base: %.0f%%", r.BaselineVal*100)
			}
		}

		line := fmt.Sprintf("  %s %-16s %s", badge, r.Metric, spark)
		if deltaStr != "" {
			line += fmt.Sprintf("  %s", deltaStr)
		}
		if baselineStr != "" {
			line += fmt.Sprintf("  %s", styles.InfoStyle.Render(baselineStr))
		}
		b.WriteString(line + "\n")
	}

	return b.String()
}

func renderIterationTable(data LoopHealthData, width int) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Recent Iterations", styles.IconTurns)))
	b.WriteString("\n")

	// Show last 15 iterations
	obs := data.Observations
	if len(obs) > 15 {
		obs = obs[len(obs)-15:]
	}

	// Header
	b.WriteString(styles.InfoStyle.Render("  #    Status   Cost      Latency   Files  Verify  Type"))
	b.WriteString("\n")

	// Rows (newest first)
	for i := len(obs) - 1; i >= 0; i-- {
		o := obs[i]
		verifyIcon := styles.StatusFailed.Render("✗")
		if o.VerifyPassed {
			verifyIcon = styles.StatusRunning.Render("✓")
		}

		statusStyle := styles.StatusRunning
		if o.Status != "idle" {
			statusStyle = styles.StatusFailed
		}

		latencyStr := fmt.Sprintf("%.1fs", float64(o.TotalLatencyMs)/1000)

		taskType := o.TaskType
		if taskType == "" {
			taskType = "-"
		}
		if len(taskType) > 8 {
			taskType = taskType[:8]
		}

		b.WriteString(fmt.Sprintf("  %-4d %s  $%-7.3f %-9s %-6d %s     %s\n",
			o.IterationNumber,
			statusStyle.Render(fmt.Sprintf("%-8s", o.Status)),
			o.TotalCostUSD,
			latencyStr,
			o.FilesChanged,
			verifyIcon,
			taskType))
	}

	return b.String()
}

func renderTaskDistribution(dist map[string]int, width int) string {
	if len(dist) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Task Type Distribution", styles.IconSession)))
	b.WriteString("\n")

	// Sort by count descending
	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	total := 0
	for k, v := range dist {
		sorted = append(sorted, kv{k, v})
		total += v
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })

	barWidth := 20
	if width > 80 {
		barWidth = 30
	}

	for _, kv := range sorted {
		pct := float64(kv.count) / float64(total)
		gauge := components.InlineGauge(float64(kv.count), float64(total), barWidth)
		b.WriteString(fmt.Sprintf("  %-12s %s %.0f%% (%d)\n",
			kv.key, gauge, pct*100, kv.count))
	}

	return b.String()
}

// LoopHealthView wraps RenderLoopHealth in a scrollable viewport.
type LoopHealthView struct {
	Viewport *ViewportView
	data     LoopHealthData
	width    int
	height   int
}

// NewLoopHealthView creates a new LoopHealthView.
func NewLoopHealthView() *LoopHealthView {
	return &LoopHealthView{
		Viewport: NewViewportView(),
	}
}

// SetData updates the loop health data and regenerates content.
func (v *LoopHealthView) SetData(data LoopHealthData) {
	v.data = data
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *LoopHealthView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *LoopHealthView) Render() string {
	return v.Viewport.Render()
}

func (v *LoopHealthView) regenerate() {
	content := RenderLoopHealth(v.data, v.width, v.height)
	v.Viewport.SetContent(content)
}

// extractMetricSeries extracts a time-ordered series of values from observations.
func extractMetricSeries(obs []session.LoopObservation, metric string) []float64 {
	result := make([]float64, len(obs))
	for i, o := range obs {
		switch metric {
		case "cost":
			result[i] = o.TotalCostUSD
		case "latency":
			result[i] = float64(o.TotalLatencyMs)
		default:
			result[i] = 0
		}
	}
	return result
}

// FormatDuration formats a duration for display.
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	return fmt.Sprintf("%.1fh", d.Hours())
}
