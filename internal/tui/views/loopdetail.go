package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RenderLoopDetail renders a detailed view of a single loop run.
func RenderLoopDetail(l *session.LoopRun, width, height int) string {
	if l == nil {
		return styles.InfoStyle.Render("  No loop selected")
	}

	l.Lock()
	id := l.ID
	repoName := l.RepoName
	status := l.Status
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
	l.Unlock()

	elapsed := time.Since(createdAt)

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Loop %s", styles.IconRunning, id)))
	b.WriteString("\n\n")

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Loop Info", styles.IconRunning)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf("  ID:         %s\n", id))
	b.WriteString(fmt.Sprintf("  Repo:       %s\n", repoName))
	b.WriteString(fmt.Sprintf("  Status:     %s %s\n",
		styles.StatusIcon(status),
		styles.StatusStyle(status).Render(status)))
	b.WriteString(fmt.Sprintf("  Iterations: %d\n", iterCount))
	b.WriteString(fmt.Sprintf("  Started:    %s %s\n", styles.IconClock, createdAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Elapsed:    %s\n", FormatDuration(elapsed)))
	b.WriteString("\n")

	if iterCount > 0 {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Last Iteration", styles.IconTurns)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf("  Number:     %d\n", lastIterNum))
		b.WriteString(fmt.Sprintf("  Status:     %s %s\n",
			styles.StatusIcon(lastIterStatus),
			styles.StatusStyle(lastIterStatus).Render(lastIterStatus)))
		if lastIterTask != "" {
			b.WriteString(fmt.Sprintf("  Task:       %s\n", lastIterTask))
		}
		if lastIterError != "" {
			b.WriteString(fmt.Sprintf("  Error:      %s\n",
				styles.StatusFailed.Render(lastIterError)))
		}
		b.WriteString("\n")
	}

	if lastError != "" {
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("%s Loop Error", styles.IconErrored)))
		b.WriteString("\n")
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  %s", lastError)))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HelpStyle.Render("  Esc: back to loop list"))

	return b.String()
}
