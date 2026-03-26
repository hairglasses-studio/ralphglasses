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

// RenderObservationView displays loop iteration metrics as sparklines.
// It renders token usage, cost, duration, and files changed per iteration,
// plus a summary line with totals.
func RenderObservationView(data ObservationViewData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Observation Sparklines: %s", styles.IconCost, data.RepoName)))
	b.WriteString("\n\n")

	if len(data.Observations) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No loop observations recorded yet."))
		b.WriteString("\n")
		b.WriteString(styles.InfoStyle.Render("  Run loop cycles to populate this view."))
		return b.String()
	}

	// Extract per-iteration metric series
	tokens := make([]float64, len(data.Observations))
	costs := make([]float64, len(data.Observations))
	durations := make([]float64, len(data.Observations))
	filesChanged := make([]float64, len(data.Observations))

	for i, obs := range data.Observations {
		tokens[i] = float64(obs.PlannerTokensOut + obs.WorkerTokensOut)
		costs[i] = obs.TotalCostUSD
		durations[i] = float64(obs.TotalLatencyMs) / 1000.0 // seconds
		filesChanged[i] = float64(obs.FilesChanged)
	}

	// Sparkline width adapts to terminal width
	sparkWidth := width - 35
	if sparkWidth < 12 {
		sparkWidth = 12
	}
	if sparkWidth > 60 {
		sparkWidth = 60
	}

	// Panel: Sparkline rows
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Iteration Metrics (%d iterations)", styles.IconTurns, len(data.Observations))))
	b.WriteString("\n\n")

	rows := []struct {
		label  string
		values []float64
		unit   string
	}{
		{"  Tokens ", tokens, "tok"},
		{"  Cost   ", costs, "$"},
		{"  Duration", durations, "s"},
		{"  Files  ", filesChanged, ""},
	}

	for _, row := range rows {
		sparkline := components.Sparkline(row.values, sparkWidth)
		latest := row.values[len(row.values)-1]
		label := styles.InfoStyle.Render(row.label)
		spark := styles.StatusRunning.Render(sparkline)
		latestStr := fmt.Sprintf("%.1f%s", latest, row.unit)
		b.WriteString(fmt.Sprintf("%s  %s  %s\n", label, spark, latestStr))
	}

	b.WriteString("\n")

	// Summary line with totals
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Summary", styles.IconBudget)))
	b.WriteString("\n")

	var totalTokens float64
	var totalCost float64
	var totalDuration float64
	var totalFiles float64
	for i := range data.Observations {
		totalTokens += tokens[i]
		totalCost += costs[i]
		totalDuration += durations[i]
		totalFiles += filesChanged[i]
	}
	avgCost := totalCost / float64(len(data.Observations))
	avgDuration := totalDuration / float64(len(data.Observations))

	b.WriteString(fmt.Sprintf("  Total tokens:   %.0f\n", totalTokens))
	b.WriteString(fmt.Sprintf("  Total cost:     $%.3f  (avg $%.3f/iter)\n", totalCost, avgCost))
	b.WriteString(fmt.Sprintf("  Total duration: %.1fs  (avg %.1fs/iter)\n", totalDuration, avgDuration))
	b.WriteString(fmt.Sprintf("  Total files:    %.0f changed\n", totalFiles))

	// Velocity if we have enough data
	if len(data.Observations) >= 2 {
		velocity := session.LoopVelocity(data.Observations, 1.0)
		b.WriteString(fmt.Sprintf("  Velocity:       %.1f useful iters/hr\n", velocity))
	}

	return b.String()
}
