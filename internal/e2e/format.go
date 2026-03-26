package e2e

import (
	"fmt"
	"strings"
)

// FormatGateReport returns a human-readable tabular string for a GateReport.
// Uses pass/warn/fail indicators for each gate result.
func FormatGateReport(report *GateReport) string {
	if report == nil {
		return "(no report)"
	}

	var b strings.Builder

	fmt.Fprintf(&b, "Gate Report  samples=%d  overall=%s\n", report.SampleCount, verdictIndicator(report.Overall))
	fmt.Fprintf(&b, "%-25s %-12s %-12s %-10s %s\n", "METRIC", "VALUE", "BASELINE", "DELTA", "VERDICT")
	b.WriteString(strings.Repeat("-", 70))
	b.WriteByte('\n')

	if len(report.Results) == 0 {
		b.WriteString("(no results)\n")
		return b.String()
	}

	for _, r := range report.Results {
		value := formatFloat(r.CurrentVal)
		baseline := formatFloat(r.BaselineVal)
		delta := formatDelta(r.DeltaPct)
		indicator := verdictIndicator(r.Verdict)
		fmt.Fprintf(&b, "%-25s %-12s %-12s %-10s %s\n", r.Metric, value, baseline, delta, indicator)
	}

	return b.String()
}

// FormatGateReportMarkdown returns a markdown-formatted table for a GateReport.
func FormatGateReportMarkdown(report *GateReport) string {
	if report == nil {
		return "_No report available._"
	}

	var b strings.Builder

	fmt.Fprintf(&b, "**Gate Report** | samples: %d | overall: %s\n\n", report.SampleCount, verdictIndicator(report.Overall))
	b.WriteString("| Metric | Value | Baseline | Delta | Verdict |\n")
	b.WriteString("|--------|-------|----------|-------|---------|\n")

	if len(report.Results) == 0 {
		b.WriteString("| _(no results)_ | | | | |\n")
		return b.String()
	}

	for _, r := range report.Results {
		value := formatFloat(r.CurrentVal)
		baseline := formatFloat(r.BaselineVal)
		delta := formatDelta(r.DeltaPct)
		indicator := verdictIndicator(r.Verdict)
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", r.Metric, value, baseline, delta, indicator)
	}

	return b.String()
}

// GateTrend represents the direction of change for a metric.
type GateTrend struct {
	Metric    string
	Direction string // "improved", "degraded", "unchanged"
	PrevValue float64
	CurrValue float64
}

// CompareGateReports compares two reports and returns trend information.
// For each metric present in both reports, it determines whether the metric
// improved, degraded, or stayed the same.
func CompareGateReports(prev, current *GateReport) []GateTrend {
	if prev == nil || current == nil {
		return nil
	}

	// Index previous results by metric name.
	prevByMetric := make(map[string]GateResult, len(prev.Results))
	for _, r := range prev.Results {
		prevByMetric[r.Metric] = r
	}

	var trends []GateTrend
	for _, cur := range current.Results {
		pr, ok := prevByMetric[cur.Metric]
		if !ok {
			continue
		}

		direction := classifyTrend(cur.Metric, pr.CurrentVal, cur.CurrentVal)
		trends = append(trends, GateTrend{
			Metric:    cur.Metric,
			Direction: direction,
			PrevValue: pr.CurrentVal,
			CurrValue: cur.CurrentVal,
		})
	}

	return trends
}

// classifyTrend determines if a metric change is an improvement, degradation,
// or unchanged. "Higher is better" for rates, "lower is better" for costs/latency/errors.
func classifyTrend(metric string, prev, curr float64) string {
	if prev == curr {
		return "unchanged"
	}

	// For rate metrics (completion, verify pass), higher is better.
	// For cost, latency, and error metrics, lower is better.
	higherIsBetter := isHigherBetterMetric(metric)

	if higherIsBetter {
		if curr > prev {
			return "improved"
		}
		return "degraded"
	}

	// Lower is better.
	if curr < prev {
		return "improved"
	}
	return "degraded"
}

// isHigherBetterMetric returns true for metrics where a higher value is desirable.
func isHigherBetterMetric(metric string) bool {
	switch metric {
	case "completion_rate", "verify_pass_rate":
		return true
	default:
		// cost_per_iteration, total_latency, error_rate — lower is better
		return false
	}
}

// verdictIndicator returns a human-readable indicator for a gate verdict.
func verdictIndicator(v GateVerdict) string {
	switch v {
	case VerdictPass:
		return "PASS"
	case VerdictWarn:
		return "WARN"
	case VerdictFail:
		return "FAIL"
	case VerdictSkip:
		return "SKIP"
	default:
		return string(v)
	}
}

// formatFloat formats a float for display, avoiding unnecessary trailing zeros.
func formatFloat(f float64) string {
	if f == 0 {
		return "0"
	}
	return fmt.Sprintf("%.4f", f)
}

// formatDelta formats a delta percentage for display.
func formatDelta(d float64) string {
	if d == 0 {
		return "-"
	}
	sign := "+"
	if d < 0 {
		sign = ""
	}
	return fmt.Sprintf("%s%.1f%%", sign, d)
}
