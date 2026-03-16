package views

import (
	"fmt"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderRepoDetail renders a detailed view of a single repo.
func RenderRepoDetail(r *model.Repo, width int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(r.Name))
	b.WriteString("\n")
	b.WriteString(styles.InfoStyle.Render(r.Path))
	b.WriteString("\n\n")

	// Status section
	b.WriteString(styles.HeaderStyle.Render("Status"))
	b.WriteString("\n")
	if r.Status != nil {
		s := r.Status
		b.WriteString(fmt.Sprintf("  Status:    %s\n", styles.StatusStyle(s.Status).Render(s.Status)))
		b.WriteString(fmt.Sprintf("  Loop:      %d\n", s.LoopCount))
		b.WriteString(fmt.Sprintf("  Calls:     %d/%d\n", s.CallsMadeThisHr, s.MaxCallsPerHour))
		b.WriteString(fmt.Sprintf("  Model:     %s\n", s.Model))
		b.WriteString(fmt.Sprintf("  Spend:     $%.2f\n", s.SessionSpendUSD))
		b.WriteString(fmt.Sprintf("  Budget:    %s\n", s.BudgetStatus))
		if s.LastAction != "" {
			b.WriteString(fmt.Sprintf("  Action:    %s\n", s.LastAction))
		}
		if s.ExitReason != "" {
			b.WriteString(fmt.Sprintf("  Exit:      %s\n", s.ExitReason))
		}
		b.WriteString(fmt.Sprintf("  Updated:   %s\n", s.Timestamp.Format("15:04:05")))
	} else {
		b.WriteString(styles.InfoStyle.Render("  No status data"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Circuit breaker section
	b.WriteString(styles.HeaderStyle.Render("Circuit Breaker"))
	b.WriteString("\n")
	if r.Circuit != nil {
		cb := r.Circuit
		b.WriteString(fmt.Sprintf("  State:           %s\n", styles.CBStyle(cb.State).Render(cb.State)))
		b.WriteString(fmt.Sprintf("  No Progress:     %d\n", cb.ConsecutiveNoProgress))
		b.WriteString(fmt.Sprintf("  Same Error:      %d\n", cb.ConsecutiveSameError))
		b.WriteString(fmt.Sprintf("  Perm Denials:    %d\n", cb.ConsecutivePermissionDenials))
		b.WriteString(fmt.Sprintf("  Total Opens:     %d\n", cb.TotalOpens))
		if cb.Reason != "" {
			b.WriteString(fmt.Sprintf("  Reason:          %s\n", cb.Reason))
		}
		b.WriteString(fmt.Sprintf("  Last Change:     %s\n", cb.LastChange.Format("15:04:05")))
	} else {
		b.WriteString(styles.InfoStyle.Render("  No circuit breaker data"))
		b.WriteString("\n")
	}
	b.WriteString("\n")

	// Config section
	b.WriteString(styles.HeaderStyle.Render("Configuration"))
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
	b.WriteString(styles.HeaderStyle.Render("Progress"))
	b.WriteString("\n")
	if r.Progress != nil {
		p := r.Progress
		b.WriteString(fmt.Sprintf("  Iteration:    %d\n", p.Iteration))
		b.WriteString(fmt.Sprintf("  Completed:    %d tasks\n", len(p.CompletedIDs)))
		b.WriteString(fmt.Sprintf("  Status:       %s\n", p.Status))
		if len(p.CompletedIDs) > 0 {
			b.WriteString(fmt.Sprintf("  Task IDs:     %s\n", strings.Join(p.CompletedIDs, ", ")))
		}
	} else {
		b.WriteString(styles.InfoStyle.Render("  No progress data"))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Enter: logs  S: start  X: stop  P: pause  e: edit config  Esc: back"))

	return b.String()
}
