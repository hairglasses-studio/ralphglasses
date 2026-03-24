package views

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RepoDetailHealth holds optional loop performance data for the repo detail view.
type RepoDetailHealth struct {
	Observations     []session.LoopObservation
	GateReport       *e2e.GateReport
	ProviderProfiles []session.ProviderProfile
}

// RenderRepoDetail renders a detailed view of a single repo.
// health may be nil if no observation data is available.
func RenderRepoDetail(r *model.Repo, width int, health *RepoDetailHealth) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s %s", styles.IconRepo, r.Name)))
	b.WriteString("\n")
	b.WriteString(styles.InfoStyle.Render(r.Path))
	b.WriteString("\n\n")

	// Status section
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Status", styles.IconRunning)))
	b.WriteString("\n")
	if r.Status != nil {
		s := r.Status
		b.WriteString(fmt.Sprintf("  Status:    %s %s\n",
			styles.StatusIcon(s.Status),
			styles.StatusStyle(s.Status).Render(s.Status)))
		b.WriteString(fmt.Sprintf("  Loop:      %d\n", s.LoopCount))

		// Calls/hour gauge
		callsLabel := fmt.Sprintf("%d/%d", s.CallsMadeThisHr, s.MaxCallsPerHour)
		if s.MaxCallsPerHour > 0 {
			gauge := components.InlineGauge(float64(s.CallsMadeThisHr), float64(s.MaxCallsPerHour), 30)
			b.WriteString(fmt.Sprintf("  Calls:     %s %s\n", gauge, callsLabel))
		} else {
			b.WriteString(fmt.Sprintf("  Calls:     %s\n", callsLabel))
		}

		b.WriteString(fmt.Sprintf("  Model:     %s\n", s.Model))

		// Budget gauge
		if s.SessionSpendUSD > 0 {
			budgetMax := 0.0
			if r.Config != nil {
				if v, ok := r.Config.Values["RALPH_SESSION_BUDGET"]; ok {
					_, _ = fmt.Sscanf(v, "%f", &budgetMax)
				}
			}
			if budgetMax > 0 {
				gauge := components.InlineGauge(s.SessionSpendUSD, budgetMax, 30)
				pct := s.SessionSpendUSD / budgetMax * 100
				b.WriteString(fmt.Sprintf("  Budget:    %s %.0f%% ($%.2f/$%.0f)\n", gauge, pct, s.SessionSpendUSD, budgetMax))
			} else {
				b.WriteString(fmt.Sprintf("  Spend:     $%.2f\n", s.SessionSpendUSD))
			}
		} else {
			b.WriteString(fmt.Sprintf("  Spend:     $%.2f\n", s.SessionSpendUSD))
		}

		if s.BudgetStatus != "" {
			b.WriteString(fmt.Sprintf("  Bdg Stat:  %s\n", s.BudgetStatus))
		}
		if s.LastAction != "" {
			b.WriteString(fmt.Sprintf("  Action:    %s\n", s.LastAction))
		}
		if s.ExitReason != "" {
			b.WriteString(fmt.Sprintf("  Exit:      %s\n", s.ExitReason))
		}
		b.WriteString(fmt.Sprintf("  Updated:   %s %s\n", styles.IconClock, s.Timestamp.Format("15:04:05")))
	} else {
		b.WriteString(styles.InfoStyle.Render("  No status data"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Circuit breaker section
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Circuit Breaker", styles.IconCBClosed)))
	b.WriteString("\n")
	if r.Circuit != nil {
		cb := r.Circuit
		b.WriteString(fmt.Sprintf("  State:           %s %s\n",
			styles.CBIcon(cb.State),
			styles.CBStyle(cb.State).Render(cb.State)))
		b.WriteString(fmt.Sprintf("  No Progress:     %d\n", cb.ConsecutiveNoProgress))
		b.WriteString(fmt.Sprintf("  Same Error:      %d\n", cb.ConsecutiveSameError))
		b.WriteString(fmt.Sprintf("  Perm Denials:    %d\n", cb.ConsecutivePermissionDenials))
		b.WriteString(fmt.Sprintf("  Total Opens:     %d\n", cb.TotalOpens))
		if cb.Reason != "" {
			b.WriteString(fmt.Sprintf("  Reason:          %s\n", cb.Reason))
		}
		b.WriteString(fmt.Sprintf("  Last Change:     %s %s\n", styles.IconClock, cb.LastChange.Format("15:04:05")))
	} else {
		b.WriteString(styles.InfoStyle.Render("  No circuit breaker data"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Config section
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Configuration", styles.IconConfig)))
	b.WriteString("\n")
	if r.Config != nil {
		for k, v := range r.Config.Values {
			b.WriteString(fmt.Sprintf("  %-28s %s\n", k, v))
		}
	} else if r.HasRC {
		b.WriteString(styles.InfoStyle.Render("  .ralphrc exists but failed to parse"))
		b.WriteString("\n")
	} else {
		b.WriteString(styles.InfoStyle.Render("  No .ralphrc"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Progress section
	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Progress", styles.IconTurns)))
	b.WriteString("\n")
	if r.Progress != nil {
		p := r.Progress
		b.WriteString(fmt.Sprintf("  Iteration:    %d\n", p.Iteration))
		completed := len(p.CompletedIDs)
		b.WriteString(fmt.Sprintf("  Completed:    %d tasks\n", completed))
		// Task completion gauge
		if completed > 0 {
			total := completed + 1
			if p.Iteration > completed {
				total = p.Iteration
			}
			gauge := components.InlineGauge(float64(completed), float64(total), 30)
			b.WriteString(fmt.Sprintf("  Progress:     %s %d/%d\n", gauge, completed, total))
		}
		b.WriteString(fmt.Sprintf("  Status:       %s\n", p.Status))
		if len(p.CompletedIDs) > 0 {
			b.WriteString(fmt.Sprintf("  Task IDs:     %s\n", strings.Join(p.CompletedIDs, ", ")))
		}
	} else {
		b.WriteString(styles.InfoStyle.Render("  No progress data"))
		b.WriteString("\n")
	}

	// Loop performance section (from observation data)
	if health != nil && len(health.Observations) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Loop Performance (last 24h)", styles.IconTurns)))
		b.WriteString("\n")

		obs := health.Observations
		b.WriteString(fmt.Sprintf("  Iterations:  %d\n", len(obs)))

		// Avg cost + P95
		var totalCost, totalLatency float64
		var verifyPass, completions int
		costs := make([]float64, 0, len(obs))
		latencies := make([]float64, 0, len(obs))
		for _, o := range obs {
			totalCost += o.TotalCostUSD
			costs = append(costs, o.TotalCostUSD)
			ms := float64(o.TotalLatencyMs) / 1000.0
			totalLatency += ms
			latencies = append(latencies, ms)
			if o.VerifyPassed {
				verifyPass++
			}
			if o.Status == "idle" || o.Status == "completed" {
				completions++
			}
		}
		avgCost := totalCost / float64(len(obs))
		avgLatency := totalLatency / float64(len(obs))
		p95Cost := percentile(costs, 0.95)
		p95Latency := percentile(latencies, 0.95)

		b.WriteString(fmt.Sprintf("  Avg Cost:    $%.2f  (P95: $%.2f)\n", avgCost, p95Cost))
		b.WriteString(fmt.Sprintf("  Avg Latency: %.1fs   (P95: %.1fs)\n", avgLatency, p95Latency))
		if len(obs) > 0 {
			compRate := float64(completions) / float64(len(obs)) * 100
			verifyRate := float64(verifyPass) / float64(len(obs)) * 100
			b.WriteString(fmt.Sprintf("  Completion:  %.0f%%\n", compRate))
			b.WriteString(fmt.Sprintf("  Verify Rate: %.0f%%\n", verifyRate))
		}

		// Gate verdict
		if health.GateReport != nil {
			badge := components.GateVerdictBadge(string(health.GateReport.Overall))
			summary := components.GateReportSummary(health.GateReport.Results)
			b.WriteString(fmt.Sprintf("  Gate:        %s  %s\n", badge, summary))
		}

		// Cost trend sparkline
		if len(costs) > 2 {
			spark := components.InlineSparkline(costs, 20)
			b.WriteString(fmt.Sprintf("  Cost Trend:  %s\n", spark))
		}
	}

	// Provider performance section
	if health != nil && len(health.ProviderProfiles) > 0 {
		b.WriteString("\n")
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Provider Performance", styles.IconSession)))
		b.WriteString("\n")
		for _, pp := range health.ProviderProfiles {
			b.WriteString(fmt.Sprintf("  %s %-8s $%.2f/task  %d turns  %.0f%% complete\n",
				styles.ProviderIcon(pp.Provider),
				styles.ProviderStyle(pp.Provider).Render(pp.Provider),
				pp.AvgCostUSD,
				pp.AvgTurns,
				pp.CompletionRate*100))
		}
	}
	b.WriteString("\n")

	// Parse error warnings
	if len(r.RefreshErrors) > 0 {
		b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("%s Warnings", styles.IconWarning)))
		b.WriteString("\n")
		for _, e := range r.RefreshErrors {
			b.WriteString(styles.WarningStyle.Render(fmt.Sprintf("  %s %s", styles.IconWarning, e.Error())))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.HelpStyle.Render("  Enter: logs  S: start  X: stop  P: pause  e: edit config  Esc: back"))

	return b.String()
}

// percentile computes p-th percentile (0.0–1.0) of a sorted copy of vals.
func percentile(vals []float64, p float64) float64 {
	if len(vals) == 0 {
		return 0
	}
	sorted := make([]float64, len(vals))
	copy(sorted, vals)
	// Simple insertion sort — N is small (≤ hundreds)
	for i := 1; i < len(sorted); i++ {
		key := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > key {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = key
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
