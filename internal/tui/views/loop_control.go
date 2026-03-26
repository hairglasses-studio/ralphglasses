package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LoopControlData holds data for the loop control panel.
type LoopControlData struct {
	Loops    []*session.LoopRun
	Selected int // index of selected loop
}

// RenderLoopControl renders the loop control panel showing all active loops
// with state, last iteration result, next scheduled iteration estimate,
// and average iteration duration. The selected loop shows inline detail.
func RenderLoopControl(data LoopControlData, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Loop Control Panel", styles.IconRunning)))
	b.WriteString("\n\n")

	if len(data.Loops) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No active loops"))
		b.WriteString("\n")
	} else {
		for i, l := range data.Loops {
			b.WriteString(renderLoopControlRow(l, i == data.Selected))
		}
		b.WriteString("\n")
		if data.Selected >= 0 && data.Selected < len(data.Loops) {
			b.WriteString(renderLoopControlInlineDetail(data.Loops[data.Selected], width))
		}
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  j/k navigate  s step  r run/stop  p pause/resume  Esc back"))

	return b.String()
}

func renderLoopControlRow(l *session.LoopRun, selected bool) string {
	l.Lock()
	id := l.ID
	if len(id) > 8 {
		id = id[:8]
	}
	repoName := l.RepoName
	status := l.Status
	paused := l.Paused
	iterCount := len(l.Iterations)
	var lastIterStatus string
	var lastIterTask string
	if iterCount > 0 {
		last := l.Iterations[iterCount-1]
		lastIterStatus = last.Status
		lastIterTask = last.Task.Title
	}
	avgDur := avgLoopIterDuration(l.Iterations)
	nextEst := estimateLoopNextIteration(l)
	l.Unlock()

	if paused {
		status = "paused"
	}

	cursor := "  "
	if selected {
		cursor = styles.StatusRunning.Render("> ")
	}

	var sb strings.Builder
	sb.WriteString(cursor)
	sb.WriteString(fmt.Sprintf("%-8s  %-20s  %s %-10s",
		id,
		loopControlTruncate(repoName, 20),
		styles.StatusIcon(status),
		styles.StatusStyle(status).Render(status),
	))
	if lastIterStatus != "" {
		sb.WriteString(fmt.Sprintf("  last:%-10s", loopControlTruncate(lastIterStatus, 10)))
	}
	if lastIterTask != "" {
		sb.WriteString(fmt.Sprintf("  task:%-20s", loopControlTruncate(lastIterTask, 20)))
	}
	if avgDur > 0 {
		sb.WriteString(fmt.Sprintf("  avg:%s", FormatDuration(avgDur)))
	}
	if nextEst != "" {
		sb.WriteString(fmt.Sprintf("  next:%s", nextEst))
	}
	sb.WriteString("\n")
	return sb.String()
}

func renderLoopControlInlineDetail(l *session.LoopRun, width int) string {
	l.Lock()
	id := l.ID
	repoName := l.RepoName
	status := l.Status
	paused := l.Paused
	iterCount := len(l.Iterations)
	lastError := l.LastError
	createdAt := l.CreatedAt

	var lastIterNum int
	var lastIterStatus, lastIterError, lastIterTask string
	if iterCount > 0 {
		last := l.Iterations[iterCount-1]
		lastIterNum = last.Number
		lastIterStatus = last.Status
		lastIterError = last.Error
		lastIterTask = last.Task.Title
	}
	avgDur := avgLoopIterDuration(l.Iterations)
	l.Unlock()

	sepLen := 60
	if width > 4 && width-2 < sepLen {
		sepLen = width - 2
	}
	sep := strings.Repeat("─", sepLen)

	statusLabel := status
	if paused {
		statusLabel = status + " (paused)"
	}
	elapsed := time.Since(createdAt)

	var b strings.Builder
	b.WriteString("  " + sep + "\n")
	b.WriteString(fmt.Sprintf("  %s %s  repo:%s  status:%s  elapsed:%s\n",
		styles.StatusIcon(status),
		id,
		repoName,
		styles.StatusStyle(status).Render(statusLabel),
		FormatDuration(elapsed),
	))

	iterLine := fmt.Sprintf("  iterations:%d", iterCount)
	if avgDur > 0 {
		iterLine += fmt.Sprintf("  avg:%s", FormatDuration(avgDur))
	}
	b.WriteString(iterLine + "\n")

	if iterCount > 0 {
		detail := fmt.Sprintf("  last iter #%d  status:%s",
			lastIterNum,
			styles.StatusStyle(lastIterStatus).Render(lastIterStatus),
		)
		if lastIterTask != "" {
			detail += fmt.Sprintf("  task:%s", loopControlTruncate(lastIterTask, 40))
		}
		if lastIterError != "" {
			detail += fmt.Sprintf("  err:%s", styles.StatusFailed.Render(loopControlTruncate(lastIterError, 40)))
		}
		b.WriteString(detail + "\n")
	}

	if lastError != "" {
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  error: %s", loopControlTruncate(lastError, 60))))
		b.WriteString("\n")
	}

	return b.String()
}

// avgLoopIterDuration computes the mean completed iteration duration.
// Must be called with the loop lock held.
func avgLoopIterDuration(iters []session.LoopIteration) time.Duration {
	var total time.Duration
	var count int
	for _, it := range iters {
		if it.EndedAt != nil {
			total += it.EndedAt.Sub(it.StartedAt)
			count++
		}
	}
	if count == 0 {
		return 0
	}
	return total / time.Duration(count)
}

// estimateLoopNextIteration returns a human-readable estimate of when the
// next iteration will start. Must be called with the loop lock held.
func estimateLoopNextIteration(l *session.LoopRun) string {
	if l.Paused {
		return "paused"
	}
	if l.Status != "running" {
		return ""
	}
	iterCount := len(l.Iterations)
	// If the last iteration has no EndedAt, it's currently executing.
	if iterCount > 0 && l.Iterations[iterCount-1].EndedAt == nil {
		return "in progress"
	}
	avg := avgLoopIterDuration(l.Iterations)
	if avg == 0 || iterCount == 0 {
		return "soon"
	}
	lastEnded := l.Iterations[iterCount-1].EndedAt
	if lastEnded == nil {
		return "in progress"
	}
	nextAt := lastEnded.Add(avg)
	if time.Now().After(nextAt) {
		return "soon"
	}
	return "in ~" + FormatDuration(time.Until(nextAt))
}

// loopControlTruncate shortens s to at most maxLen bytes (output length), appending "…" if trimmed.
func loopControlTruncate(s string, maxLen int) string {
	if len(s) < maxLen {
		return s
	}
	if maxLen <= 1 {
		return "…"
	}
	return s[:maxLen-1] + "…"
}
