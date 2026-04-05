package godview

import (
	"fmt"
	"strings"
)

// color wraps text in ANSI escape codes.
func color(code, s string) string { return code + s + Reset }

// PromptDJStats holds routing statistics for the godview dashboard panel.
type PromptDJStats struct {
	TotalDecisions  int64              `json:"total_decisions"`
	TotalDispatches int64              `json:"total_dispatches"`
	SuccessRate     float64            `json:"success_rate"`
	AvgConfidence   float64            `json:"avg_confidence"`
	AvgScore        float64            `json:"avg_score"`
	TotalCostUSD    float64            `json:"total_cost_usd"`
	EnhancedPct     float64            `json:"enhanced_pct"`
	ByProvider      map[string]int64   `json:"by_provider"`
	ByTaskType      map[string]int64   `json:"by_task_type"`
	ByConfidence    map[string]int64   `json:"by_confidence"`
	RecentDecisions []RecentDecision   `json:"recent_decisions"`
}

// RecentDecision is a summary of a recent routing decision.
type RecentDecision struct {
	DecisionID string  `json:"decision_id"`
	Provider   string  `json:"provider"`
	TaskType   string  `json:"task_type"`
	Score      int     `json:"score"`
	Confidence float64 `json:"confidence"`
	CostUSD    float64 `json:"cost_usd"`
	Status     string  `json:"status"`
}

// RenderPromptDJPanel formats the Prompt DJ stats as a dashboard panel.
func RenderPromptDJPanel(stats *PromptDJStats, width int) string {
	if stats == nil || stats.TotalDecisions == 0 {
		return boxTitle("PROMPT DJ", width) + "\n  No routing decisions yet\n"
	}

	var b strings.Builder
	b.WriteString(boxTitle("PROMPT DJ", width))
	b.WriteByte('\n')

	// Summary
	fmt.Fprintf(&b, "  Decisions: %s  Success: %s  Avg Score: %s  Cost: %s\n",
		color(Claude, fmt.Sprintf("%d", stats.TotalDecisions)),
		rateColor(stats.SuccessRate),
		scoreColor(stats.AvgScore),
		color(StatusWarn, fmt.Sprintf("$%.2f", stats.TotalCostUSD)),
	)
	fmt.Fprintf(&b, "  Confidence: %s  Enhanced: %s\n",
		confColor(stats.AvgConfidence),
		pctColor(stats.EnhancedPct),
	)

	// Providers
	if len(stats.ByProvider) > 0 {
		b.WriteString("  Providers: ")
		first := true
		for p, count := range stats.ByProvider {
			if !first {
				b.WriteString("  ")
			}
			fmt.Fprintf(&b, "%s:%s", p, color(Claude, fmt.Sprintf("%d", count)))
			first = false
		}
		b.WriteByte('\n')
	}

	// Recent
	if len(stats.RecentDecisions) > 0 {
		b.WriteString("  Recent:\n")
		limit := 3
		if len(stats.RecentDecisions) < limit {
			limit = len(stats.RecentDecisions)
		}
		for _, d := range stats.RecentDecisions[:limit] {
			st := color(StatusOK, "OK")
			if d.Status == "failed" {
				st = color(StatusErr, "FAIL")
			} else if d.Status == "routed" {
				st = color(StatusWarn, "PEND")
			}
			hash := d.DecisionID
			if len(hash) > 8 {
				hash = hash[:8]
			}
			fmt.Fprintf(&b, "    %s %s/%s score=%d conf=%.2f %s\n",
				st, d.Provider, d.TaskType, d.Score, d.Confidence, hash)
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

func rateColor(rate float64) string {
	pct := fmt.Sprintf("%.0f%%", rate*100)
	if rate >= 0.8 {
		return color(StatusOK, pct)
	} else if rate >= 0.5 {
		return color(StatusWarn, pct)
	}
	return color(StatusErr, pct)
}

func scoreColor(score float64) string {
	s := fmt.Sprintf("%.0f", score)
	if score >= 80 {
		return color(StatusOK, s)
	} else if score >= 50 {
		return color(StatusWarn, s)
	}
	return color(StatusErr, s)
}

func confColor(conf float64) string {
	s := fmt.Sprintf("%.2f", conf)
	if conf >= 0.8 {
		return color(StatusOK, s)
	} else if conf >= 0.5 {
		return color(StatusWarn, s)
	}
	return color(StatusErr, s)
}

func pctColor(pct float64) string {
	s := fmt.Sprintf("%.0f%%", pct*100)
	if pct < 0.2 {
		return color(StatusOK, s)
	} else if pct < 0.5 {
		return color(StatusWarn, s)
	}
	return color(StatusErr, s)
}
