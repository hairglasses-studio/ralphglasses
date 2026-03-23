package components

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// GateVerdictBadge renders a colored badge for a gate verdict: [PASS] [WARN] [FAIL] [SKIP].
func GateVerdictBadge(verdict string) string {
	label := strings.ToUpper(verdict)
	switch verdict {
	case "pass":
		return styles.StatusRunning.Render(fmt.Sprintf("[%s]", label))
	case "warn":
		return styles.WarningStyle.Render(fmt.Sprintf("[%s]", label))
	case "fail":
		return styles.StatusFailed.Render(fmt.Sprintf("[%s]", label))
	default:
		return styles.InfoStyle.Render(fmt.Sprintf("[%s]", label))
	}
}

// GateVerdictRow renders a metric name + badge + delta percentage.
func GateVerdictRow(metric, verdict string, delta float64) string {
	badge := GateVerdictBadge(verdict)
	deltaStr := ""
	if delta != 0 {
		sign := "+"
		if delta < 0 {
			sign = ""
		}
		deltaStr = fmt.Sprintf(" %s%.1f%%", sign, delta)
	}
	return fmt.Sprintf("  %s %-14s%s", badge, metric, deltaStr)
}

// GateReportSummary renders a compact 1-line summary: "4 pass · 1 warn · 0 fail".
func GateReportSummary(results []e2e.GateResult) string {
	counts := map[string]int{"pass": 0, "warn": 0, "fail": 0, "skip": 0}
	for _, r := range results {
		counts[string(r.Verdict)]++
	}

	parts := make([]string, 0, 3)
	if counts["pass"] > 0 {
		parts = append(parts, styles.StatusRunning.Render(fmt.Sprintf("%d pass", counts["pass"])))
	}
	if counts["warn"] > 0 {
		parts = append(parts, styles.WarningStyle.Render(fmt.Sprintf("%d warn", counts["warn"])))
	}
	if counts["fail"] > 0 {
		parts = append(parts, styles.StatusFailed.Render(fmt.Sprintf("%d fail", counts["fail"])))
	}
	if counts["skip"] > 0 {
		parts = append(parts, styles.InfoStyle.Render(fmt.Sprintf("%d skip", counts["skip"])))
	}
	if len(parts) == 0 {
		return styles.InfoStyle.Render("no data")
	}
	return strings.Join(parts, " · ")
}
