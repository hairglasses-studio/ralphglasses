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
	budgetTotal := l.Profile.PlannerBudgetUSD + l.Profile.WorkerBudgetUSD + l.Profile.VerifierBudgetUSD

	var lastIterNum int
	var lastIterStatus, lastIterError, lastIterTask string
	var plannerOutput, workerResult string
	paused := l.Paused
	if iterCount > 0 {
		last := l.Iterations[iterCount-1]
		lastIterNum = last.Number
		lastIterStatus = last.Status
		lastIterError = last.Error
		lastIterTask = last.Task.Title
		plannerOutput = last.PlannerOutput
		workerResult = last.WorkerOutput
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
	statusLabel := status
	if paused {
		statusLabel = status + " (paused)"
	}
	b.WriteString(fmt.Sprintf("  Status:     %s %s\n",
		styles.StatusIcon(status),
		styles.StatusStyle(status).Render(statusLabel)))
	b.WriteString(fmt.Sprintf("  Iterations: %d\n", iterCount))
	b.WriteString(fmt.Sprintf("  Started:    %s %s\n", styles.IconClock, createdAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Elapsed:    %s\n", FormatDuration(elapsed)))
	if budgetTotal > 0 {
		b.WriteString(fmt.Sprintf("  Budget:     %s $%.2f\n", styles.IconCost, budgetTotal))
	}
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

	if plannerOutput != "" {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Planner Output", styles.IconRunning)))
		b.WriteString("\n")
		// Truncate long output for display
		po := plannerOutput
		if len(po) > 500 {
			po = po[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s\n\n", po))
	}

	if workerResult != "" {
		b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Worker Result", styles.IconRunning)))
		b.WriteString("\n")
		wr := workerResult
		if len(wr) > 500 {
			wr = wr[:500] + "..."
		}
		b.WriteString(fmt.Sprintf("  %s\n\n", wr))
	}

	if lastError != "" {
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("%s Loop Error", styles.IconErrored)))
		b.WriteString("\n")
		b.WriteString(styles.StatusFailed.Render(fmt.Sprintf("  %s", lastError)))
		b.WriteString("\n\n")
	}

	b.WriteString(styles.HelpStyle.Render("  s step  r run/stop  p pause/resume  Esc back"))

	return b.String()
}

// LoopDetailView wraps RenderLoopDetail in a scrollable viewport.
type LoopDetailView struct {
	Viewport *ViewportView
	loop     *session.LoopRun
	width    int
	height   int
}

// NewLoopDetailView creates a new LoopDetailView.
func NewLoopDetailView() *LoopDetailView {
	return &LoopDetailView{
		Viewport: NewViewportView(),
	}
}

// SetData updates the loop data and regenerates content.
func (v *LoopDetailView) SetData(l *session.LoopRun) {
	v.loop = l
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *LoopDetailView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *LoopDetailView) Render() string {
	return v.Viewport.Render()
}

func (v *LoopDetailView) regenerate() {
	content := RenderLoopDetail(v.loop, v.width, v.height)
	v.Viewport.SetContent(content)
}
