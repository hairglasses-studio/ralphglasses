package views

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// ObservationViewData holds the data needed to render the observation sparkline view.
type ObservationViewData struct {
	RepoName     string
	Observations []session.LoopObservation
}

// RenderObservationView renders a sparkline-focused observation dashboard.
// It shows cost, latency, and file-change sparklines for recent loop iterations.
func RenderObservationView(data ObservationViewData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Observations: %s", styles.IconRunning, data.RepoName)))
	b.WriteString("\n\n")

	if len(data.Observations) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No observations recorded yet."))
		b.WriteString("\n")
		b.WriteString(styles.InfoStyle.Render("  Run loop iterations to populate this view."))
		return b.String()
	}

	sparkWidth := 30
	if width > 100 {
		sparkWidth = 50
	} else if width > 60 {
		sparkWidth = 40
	}

	// Cost sparkline
	costData := make([]float64, len(data.Observations))
	var totalCost float64
	for i, o := range data.Observations {
		costData[i] = o.TotalCostUSD
		totalCost += o.TotalCostUSD
	}
	avgCost := totalCost / float64(len(data.Observations))
	costThreshold := avgCost * 1.5

	b.WriteString(styles.HeaderStyle.Render("  Cost per Iteration"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  avg: $%.4f  total: $%.4f  n=%d\n",
		components.HealthSparkline(costData, costThreshold, sparkWidth),
		avgCost, totalCost, len(data.Observations)))
	b.WriteString("\n")

	// Latency sparkline
	latencyData := make([]float64, len(data.Observations))
	var totalLatency float64
	for i, o := range data.Observations {
		latencyData[i] = float64(o.TotalLatencyMs)
		totalLatency += float64(o.TotalLatencyMs)
	}
	avgLatency := totalLatency / float64(len(data.Observations))
	latencyThreshold := avgLatency * 1.5

	b.WriteString(styles.HeaderStyle.Render("  Latency per Iteration"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  avg: %.1fs  n=%d\n",
		components.HealthSparkline(latencyData, latencyThreshold, sparkWidth),
		avgLatency/1000, len(data.Observations)))
	b.WriteString("\n")

	// Files changed sparkline
	filesData := make([]float64, len(data.Observations))
	var totalFiles float64
	for i, o := range data.Observations {
		filesData[i] = float64(o.FilesChanged)
		totalFiles += float64(o.FilesChanged)
	}
	avgFiles := totalFiles / float64(len(data.Observations))
	filesThreshold := avgFiles * 2.0

	b.WriteString(styles.HeaderStyle.Render("  Files Changed per Iteration"))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  %s  avg: %.1f  total: %.0f  n=%d\n",
		components.HealthSparkline(filesData, filesThreshold, sparkWidth),
		avgFiles, totalFiles, len(data.Observations)))
	b.WriteString("\n")

	// Verify pass rate
	var passed int
	for _, o := range data.Observations {
		if o.VerifyPassed {
			passed++
		}
	}
	passRate := float64(passed) / float64(len(data.Observations)) * 100

	b.WriteString(styles.HeaderStyle.Render("  Verify Pass Rate"))
	b.WriteString("\n")
	passStyle := styles.StatusRunning
	if passRate < 50 {
		passStyle = styles.StatusFailed
	} else if passRate < 80 {
		passStyle = styles.CircuitHalfOpen
	}
	b.WriteString(fmt.Sprintf("  %s  %d/%d iterations passed\n",
		passStyle.Render(fmt.Sprintf("%.0f%%", passRate)),
		passed, len(data.Observations)))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Esc back"))

	return b.String()
}
