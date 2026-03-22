package views

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderRepoDetail renders a detailed view of a single repo.
func RenderRepoDetail(r *model.Repo, width int) string {
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
					fmt.Sscanf(v, "%f", &budgetMax)
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
