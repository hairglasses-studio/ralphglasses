package session

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// WeeklyReportGenerator aggregates session metrics over a time window and
// produces a markdown summary with trends, anomalies, and recommendations.
type WeeklyReportGenerator struct {
	// WindowDays controls the lookback period (default 7).
	WindowDays int
}

// NewWeeklyReportGenerator creates a report generator with the given window.
func NewWeeklyReportGenerator(windowDays int) *WeeklyReportGenerator {
	if windowDays <= 0 {
		windowDays = 7
	}
	return &WeeklyReportGenerator{WindowDays: windowDays}
}

// ReportInput bundles the data sources needed to generate a report.
type ReportInput struct {
	Sessions []SessionSummary  `json:"sessions"`
	Now      time.Time         `json:"now"` // allows deterministic testing
}

// SessionSummary is a lightweight session record for report aggregation.
type SessionSummary struct {
	ID         string    `json:"id"`
	Provider   string    `json:"provider"`
	TaskType   string    `json:"task_type"`
	Status     string    `json:"status"` // "completed", "errored", "stopped"
	CostUSD    float64   `json:"cost_usd"`
	DurSec     float64   `json:"duration_sec"`
	TurnCount  int       `json:"turn_count"`
	StartedAt  time.Time `json:"started_at"`
}

// WeeklyReport is the structured output of report generation.
type WeeklyReport struct {
	GeneratedAt time.Time         `json:"generated_at"`
	WindowStart time.Time         `json:"window_start"`
	WindowEnd   time.Time         `json:"window_end"`
	Summary     ReportSummary     `json:"summary"`
	ByProvider  []ProviderStats   `json:"by_provider"`
	ByTaskType  []TaskTypeStats   `json:"by_task_type"`
	Anomalies   []ReportAnomaly   `json:"anomalies,omitempty"`
	Recommendations []string      `json:"recommendations,omitempty"`
	Markdown    string            `json:"markdown"`
}

// ReportSummary holds top-level aggregate metrics.
type ReportSummary struct {
	TotalSessions  int     `json:"total_sessions"`
	Completed      int     `json:"completed"`
	Errored        int     `json:"errored"`
	Stopped        int     `json:"stopped"`
	SuccessRate    float64 `json:"success_rate"`
	TotalCostUSD   float64 `json:"total_cost_usd"`
	AvgCostUSD     float64 `json:"avg_cost_usd"`
	AvgDurSec      float64 `json:"avg_duration_sec"`
	TotalTurns     int     `json:"total_turns"`
	AvgTurns       float64 `json:"avg_turns"`
}

// ProviderStats holds per-provider metrics.
type ProviderStats struct {
	Provider    string  `json:"provider"`
	Sessions    int     `json:"sessions"`
	SuccessRate float64 `json:"success_rate"`
	TotalCost   float64 `json:"total_cost_usd"`
	AvgCost     float64 `json:"avg_cost_usd"`
	AvgDurSec   float64 `json:"avg_duration_sec"`
}

// TaskTypeStats holds per-task-type metrics.
type TaskTypeStats struct {
	TaskType    string  `json:"task_type"`
	Sessions    int     `json:"sessions"`
	SuccessRate float64 `json:"success_rate"`
	TotalCost   float64 `json:"total_cost_usd"`
	AvgCost     float64 `json:"avg_cost_usd"`
}

// ReportAnomaly flags unusual patterns in the data.
type ReportAnomaly struct {
	Category    string  `json:"category"` // "cost_spike", "error_rate", "duration"
	Description string  `json:"description"`
	Severity    string  `json:"severity"` // "info", "warning", "critical"
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
}

// Generate produces a WeeklyReport from the given input data.
func (g *WeeklyReportGenerator) Generate(input ReportInput) WeeklyReport {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	windowStart := now.AddDate(0, 0, -g.WindowDays)

	// Filter sessions within the window.
	var sessions []SessionSummary
	for _, s := range input.Sessions {
		if !s.StartedAt.Before(windowStart) && !s.StartedAt.After(now) {
			sessions = append(sessions, s)
		}
	}

	report := WeeklyReport{
		GeneratedAt: now,
		WindowStart: windowStart,
		WindowEnd:   now,
	}

	report.Summary = g.computeSummary(sessions)
	report.ByProvider = g.computeProviderStats(sessions)
	report.ByTaskType = g.computeTaskTypeStats(sessions)
	report.Anomalies = g.detectAnomalies(sessions, report.Summary)
	report.Recommendations = g.generateRecommendations(report)
	report.Markdown = g.renderMarkdown(report)

	return report
}

func (g *WeeklyReportGenerator) computeSummary(sessions []SessionSummary) ReportSummary {
	s := ReportSummary{TotalSessions: len(sessions)}
	if len(sessions) == 0 {
		return s
	}

	for _, sess := range sessions {
		switch sess.Status {
		case "completed":
			s.Completed++
		case "errored":
			s.Errored++
		case "stopped":
			s.Stopped++
		}
		s.TotalCostUSD += sess.CostUSD
		s.AvgDurSec += sess.DurSec
		s.TotalTurns += sess.TurnCount
	}

	n := float64(len(sessions))
	s.SuccessRate = float64(s.Completed) / n
	s.AvgCostUSD = s.TotalCostUSD / n
	s.AvgDurSec = s.AvgDurSec / n
	s.AvgTurns = float64(s.TotalTurns) / n

	return s
}

func (g *WeeklyReportGenerator) computeProviderStats(sessions []SessionSummary) []ProviderStats {
	byProvider := make(map[string]*ProviderStats)

	for _, s := range sessions {
		p := s.Provider
		if p == "" {
			p = "unknown"
		}
		ps, ok := byProvider[p]
		if !ok {
			ps = &ProviderStats{Provider: p}
			byProvider[p] = ps
		}
		ps.Sessions++
		ps.TotalCost += s.CostUSD
		ps.AvgDurSec += s.DurSec
		if s.Status == "completed" {
			ps.SuccessRate++
		}
	}

	var result []ProviderStats
	for _, ps := range byProvider {
		n := float64(ps.Sessions)
		ps.SuccessRate = ps.SuccessRate / n
		ps.AvgCost = ps.TotalCost / n
		ps.AvgDurSec = ps.AvgDurSec / n
		result = append(result, *ps)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Sessions > result[j].Sessions
	})
	return result
}

func (g *WeeklyReportGenerator) computeTaskTypeStats(sessions []SessionSummary) []TaskTypeStats {
	byTask := make(map[string]*TaskTypeStats)

	for _, s := range sessions {
		t := s.TaskType
		if t == "" {
			t = "unclassified"
		}
		ts, ok := byTask[t]
		if !ok {
			ts = &TaskTypeStats{TaskType: t}
			byTask[t] = ts
		}
		ts.Sessions++
		ts.TotalCost += s.CostUSD
		if s.Status == "completed" {
			ts.SuccessRate++
		}
	}

	var result []TaskTypeStats
	for _, ts := range byTask {
		n := float64(ts.Sessions)
		ts.SuccessRate = ts.SuccessRate / n
		ts.AvgCost = ts.TotalCost / n
		result = append(result, *ts)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Sessions > result[j].Sessions
	})
	return result
}

func (g *WeeklyReportGenerator) detectAnomalies(sessions []SessionSummary, summary ReportSummary) []ReportAnomaly {
	var anomalies []ReportAnomaly

	if len(sessions) < 3 {
		return nil
	}

	// High error rate anomaly.
	errorRate := 1.0 - summary.SuccessRate
	if errorRate > 0.3 {
		severity := "warning"
		if errorRate > 0.5 {
			severity = "critical"
		}
		anomalies = append(anomalies, ReportAnomaly{
			Category:    "error_rate",
			Description: fmt.Sprintf("Error rate is %.0f%% (threshold: 30%%)", errorRate*100),
			Severity:    severity,
			Value:       errorRate,
			Threshold:   0.3,
		})
	}

	// Cost spike: check if any single session exceeds 3x the average.
	if summary.AvgCostUSD > 0 {
		threshold := summary.AvgCostUSD * 3.0
		for _, s := range sessions {
			if s.CostUSD > threshold {
				anomalies = append(anomalies, ReportAnomaly{
					Category:    "cost_spike",
					Description: fmt.Sprintf("Session %s cost $%.2f (3x average of $%.2f)", s.ID, s.CostUSD, summary.AvgCostUSD),
					Severity:    "warning",
					Value:       s.CostUSD,
					Threshold:   threshold,
				})
			}
		}
	}

	// Duration anomaly: sessions exceeding 2 standard deviations.
	if len(sessions) >= 5 {
		var durations []float64
		for _, s := range sessions {
			durations = append(durations, s.DurSec)
		}
		mean, stddev := meanStddev(durations)
		threshold := mean + 2.0*stddev
		if stddev > 0 {
			for _, s := range sessions {
				if s.DurSec > threshold {
					anomalies = append(anomalies, ReportAnomaly{
						Category:    "duration",
						Description: fmt.Sprintf("Session %s took %.0fs (mean: %.0fs, stddev: %.0fs)", s.ID, s.DurSec, mean, stddev),
						Severity:    "info",
						Value:       s.DurSec,
						Threshold:   threshold,
					})
				}
			}
		}
	}

	return anomalies
}

func (g *WeeklyReportGenerator) generateRecommendations(report WeeklyReport) []string {
	var recs []string

	// Low success rate recommendation.
	if report.Summary.SuccessRate < 0.7 && report.Summary.TotalSessions >= 3 {
		recs = append(recs, fmt.Sprintf(
			"Success rate is %.0f%%. Consider reviewing error patterns and adjusting prompts or provider selection.",
			report.Summary.SuccessRate*100,
		))
	}

	// Provider comparison: recommend switching if one provider is significantly better.
	if len(report.ByProvider) >= 2 {
		best := report.ByProvider[0]
		for _, p := range report.ByProvider[1:] {
			if p.SuccessRate > best.SuccessRate {
				best = p
			}
		}
		for _, p := range report.ByProvider {
			if p.Provider != best.Provider && p.Sessions >= 3 && best.SuccessRate-p.SuccessRate > 0.2 {
				recs = append(recs, fmt.Sprintf(
					"Consider routing more %s tasks to %s (%.0f%% vs %.0f%% success rate).",
					p.Provider, best.Provider,
					best.SuccessRate*100, p.SuccessRate*100,
				))
			}
		}
	}

	// Cost anomalies.
	for _, a := range report.Anomalies {
		if a.Category == "cost_spike" && a.Severity != "info" {
			recs = append(recs, "Review cost spikes — consider setting tighter budget limits for high-cost sessions.")
			break
		}
	}

	// High average cost recommendation.
	if report.Summary.AvgCostUSD > 2.0 && report.Summary.TotalSessions >= 5 {
		recs = append(recs, fmt.Sprintf(
			"Average session cost is $%.2f. Consider using cheaper providers for simple tasks or reducing max turns.",
			report.Summary.AvgCostUSD,
		))
	}

	return recs
}

func (g *WeeklyReportGenerator) renderMarkdown(report WeeklyReport) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# Weekly Report: %s to %s\n\n",
		report.WindowStart.Format("2006-01-02"),
		report.WindowEnd.Format("2006-01-02"),
	))

	// Summary.
	s := report.Summary
	b.WriteString("## Summary\n\n")
	b.WriteString(fmt.Sprintf("| Metric | Value |\n|--------|-------|\n"))
	b.WriteString(fmt.Sprintf("| Total Sessions | %d |\n", s.TotalSessions))
	b.WriteString(fmt.Sprintf("| Completed | %d |\n", s.Completed))
	b.WriteString(fmt.Sprintf("| Errored | %d |\n", s.Errored))
	b.WriteString(fmt.Sprintf("| Stopped | %d |\n", s.Stopped))
	b.WriteString(fmt.Sprintf("| Success Rate | %.0f%% |\n", s.SuccessRate*100))
	b.WriteString(fmt.Sprintf("| Total Cost | $%.2f |\n", s.TotalCostUSD))
	b.WriteString(fmt.Sprintf("| Avg Cost/Session | $%.2f |\n", s.AvgCostUSD))
	b.WriteString(fmt.Sprintf("| Avg Duration | %.0fs |\n", s.AvgDurSec))
	b.WriteString(fmt.Sprintf("| Total Turns | %d |\n", s.TotalTurns))
	b.WriteString("\n")

	// By provider.
	if len(report.ByProvider) > 0 {
		b.WriteString("## By Provider\n\n")
		b.WriteString("| Provider | Sessions | Success Rate | Total Cost | Avg Cost | Avg Duration |\n")
		b.WriteString("|----------|----------|--------------|------------|----------|-------------|\n")
		for _, p := range report.ByProvider {
			b.WriteString(fmt.Sprintf("| %s | %d | %.0f%% | $%.2f | $%.2f | %.0fs |\n",
				p.Provider, p.Sessions, p.SuccessRate*100, p.TotalCost, p.AvgCost, p.AvgDurSec,
			))
		}
		b.WriteString("\n")
	}

	// By task type.
	if len(report.ByTaskType) > 0 {
		b.WriteString("## By Task Type\n\n")
		b.WriteString("| Task Type | Sessions | Success Rate | Total Cost | Avg Cost |\n")
		b.WriteString("|-----------|----------|--------------|------------|----------|\n")
		for _, t := range report.ByTaskType {
			b.WriteString(fmt.Sprintf("| %s | %d | %.0f%% | $%.2f | $%.2f |\n",
				t.TaskType, t.Sessions, t.SuccessRate*100, t.TotalCost, t.AvgCost,
			))
		}
		b.WriteString("\n")
	}

	// Anomalies.
	if len(report.Anomalies) > 0 {
		b.WriteString("## Anomalies\n\n")
		for _, a := range report.Anomalies {
			icon := "INFO"
			switch a.Severity {
			case "warning":
				icon = "WARNING"
			case "critical":
				icon = "CRITICAL"
			}
			b.WriteString(fmt.Sprintf("- **[%s]** %s\n", icon, a.Description))
		}
		b.WriteString("\n")
	}

	// Recommendations.
	if len(report.Recommendations) > 0 {
		b.WriteString("## Recommendations\n\n")
		for _, r := range report.Recommendations {
			b.WriteString(fmt.Sprintf("- %s\n", r))
		}
		b.WriteString("\n")
	}

	return b.String()
}

// meanStddev computes the mean and population standard deviation of a float slice.
func meanStddev(vals []float64) (float64, float64) {
	if len(vals) == 0 {
		return 0, 0
	}
	var sum float64
	for _, v := range vals {
		sum += v
	}
	mean := sum / float64(len(vals))

	var sqDiffSum float64
	for _, v := range vals {
		d := v - mean
		sqDiffSum += d * d
	}
	stddev := math.Sqrt(sqDiffSum / float64(len(vals)))
	return mean, stddev
}
