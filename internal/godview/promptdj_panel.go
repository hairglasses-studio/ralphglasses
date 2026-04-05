package godview

import (
	"fmt"
	"strings"
)

// Color helpers wrapping ANSI codes.
func djdjCyan(s string) string   { return Claude + s + Reset }
func djGreen(s string) string  { return StatusOK + s + Reset }
func djYellow(s string) string { return StatusWarn + s + Reset }
func djRed(s string) string    { return StatusErr + s + Reset }

// PromptDJStats holds routing statistics for the godview dashboard panel.
type PromptDJStats struct {
	TotalDecisions  int64              `json:"total_decisions"`
	TotalDispatches int64              `json:"total_dispatches"`
	SuccessRate     float64            `json:"success_rate"` // 0-1
	AvgConfidence   float64            `json:"avg_confidence"`
	AvgScore        float64            `json:"avg_score"`
	TotalCostUSD    float64            `json:"total_cost_usd"`
	EnhancedPct     float64            `json:"enhanced_pct"` // 0-1
	ByProvider      map[string]int64   `json:"by_provider"`
	ByTaskType      map[string]int64   `json:"by_task_type"`
	ByConfidence    map[string]int64   `json:"by_confidence"` // high/medium/low
	RecentDecisions []RecentDecision   `json:"recent_decisions"`
}

// RecentDecision is a summary of a recent routing decision for the dashboard.
type RecentDecision struct {
	DecisionID string  `json:"decision_id"`
	Provider   string  `json:"provider"`
	TaskType   string  `json:"task_type"`
	Score      int     `json:"score"`
	Confidence float64 `json:"confidence"`
	CostUSD    float64 `json:"cost_usd"`
	Status     string  `json:"status"` // routed, dispatched, succeeded, failed
}

// RenderPromptDJPanel formats the Prompt DJ stats as a dashboard panel.
func RenderPromptDJPanel(stats *PromptDJStats, width int) string {
	if stats == nil || stats.TotalDecisions == 0 {
		return boxTitle("PROMPT DJ", width) + "\n  No routing decisions yet\n"
	}

	var b strings.Builder

	b.WriteString(boxTitle("PROMPT DJ", width))
	b.WriteByte('\n')

	// Summary line
	fmt.Fprintf(&b, "  Decisions: %s  Success: %s  Avg Score: %s  Cost: %s\n",
		djCyan(fmt.Sprintf("%d", stats.TotalDecisions)),
		colorRate(stats.SuccessRate),
		colorScore(stats.AvgScore),
		Yellow(fmt.Sprintf("$%.2f", stats.TotalCostUSD)),
	)

	// Confidence + Enhancement
	fmt.Fprintf(&b, "  Confidence: %s  Enhanced: %s\n",
		colorConfidence(stats.AvgConfidence),
		colorPct(stats.EnhancedPct),
	)

	// Provider distribution
	if len(stats.ByProvider) > 0 {
		b.WriteString("  Providers: ")
		first := true
		for p, count := range stats.ByProvider {
			if !first {
				b.WriteString("  ")
			}
			fmt.Fprintf(&b, "%s:%s", p, djCyan(fmt.Sprintf("%d", count)))
			first = false
		}
		b.WriteByte('\n')
	}

	// Recent decisions (last 3)
	if len(stats.RecentDecisions) > 0 {
		b.WriteString("  Recent:\n")
		limit := 3
		if len(stats.RecentDecisions) < limit {
			limit = len(stats.RecentDecisions)
		}
		for _, d := range stats.RecentDecisions[:limit] {
			status := Green("OK")
			if d.Status == "failed" {
				status = Red("FAIL")
			} else if d.Status == "routed" {
				status = Yellow("PEND")
			}
			fmt.Fprintf(&b, "    %s %s/%s score=%d conf=%.2f %s\n",
				status, d.Provider, d.TaskType, d.Score, d.Confidence,
				d.DecisionID[:8])
		}
	}

	return b.String()
}

func boxTitle(title string, width int) string {
	pad := width - len(title) - 4
	if pad < 0 {
		pad = 0
	}
	return fmt.Sprintf("┌─ %s %s┐", title, strings.Repeat("─", pad))
}

func colorRate(rate float64) string {
	pct := fmt.Sprintf("%.0f%%", rate*100)
	if rate >= 0.8 {
		return Green(pct)
	} else if rate >= 0.5 {
		return Yellow(pct)
	}
	return Red(pct)
}

func colorScore(score float64) string {
	s := fmt.Sprintf("%.0f", score)
	if score >= 80 {
		return Green(s)
	} else if score >= 50 {
		return Yellow(s)
	}
	return Red(s)
}

func colorConfidence(conf float64) string {
	s := fmt.Sprintf("%.2f", conf)
	if conf >= 0.8 {
		return Green(s)
	} else if conf >= 0.5 {
		return Yellow(s)
	}
	return Red(s)
}

func colorPct(pct float64) string {
	s := fmt.Sprintf("%.0f%%", pct*100)
	if pct < 0.2 {
		return Green(s)
	} else if pct < 0.5 {
		return Yellow(s)
	}
	return Red(s) // high enhancement rate = quality issues
}
