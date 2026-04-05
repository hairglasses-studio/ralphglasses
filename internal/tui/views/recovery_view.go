// recovery_view.go — TUI view for crash recovery status, session table, and history.
package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RecoveryRefreshMsg signals the recovery view to refresh data.
type RecoveryRefreshMsg struct{}

// RecoveryPanel identifies the focused section.
type RecoveryPanel int

const (
	PanelPlan RecoveryPanel = iota
	PanelSessions
	PanelHistory
)

const recoveryPanelCount = 3

// RecoveryData holds all data for rendering the recovery dashboard.
type RecoveryData struct {
	CurrentPlan   *session.CrashRecoveryPlan
	Sessions      []RecoverySessionRow
	History       []*session.RecoveryOp
	BudgetTotal   float64
	BudgetSpent   float64
	PolicyEnabled bool
}

// RecoverySessionRow is a flattened row for the session table.
type RecoverySessionRow struct {
	SessionID      string
	RepoName       string
	SessionName    string
	Priority       int
	OpenTasks      int
	HasUncommitted bool
	UnpushedCount  int
	Status         string // pending, executing, succeeded, failed
	CostUSD        float64
	LastActivity   time.Time
}

// RecoveryView displays crash recovery status and history.
type RecoveryView struct {
	Viewport *ViewportView
	data     RecoveryData
	panel    RecoveryPanel
	cursor   int
	width    int
	height   int
}

// NewRecoveryView creates a new recovery view.
func NewRecoveryView() *RecoveryView {
	return &RecoveryView{
		Viewport: NewViewportView(),
	}
}

// SetData updates the recovery data and regenerates.
func (v *RecoveryView) SetData(data RecoveryData) {
	v.data = data
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *RecoveryView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content (implements View interface).
func (v *RecoveryView) Render() string {
	return v.Viewport.Render()
}

// RenderAt returns the rendered content for given dimensions (implements ViewHandler).
func (v *RecoveryView) RenderAt(width, height int) string {
	v.SetDimensions(width, height)
	return v.Render()
}

// HandleKey processes recovery-specific key events.
func (v *RecoveryView) HandleKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "tab":
		v.panel = RecoveryPanel((int(v.panel) + 1) % recoveryPanelCount)
		v.cursor = 0
		v.regenerate()
		return true, nil
	case "shift+tab":
		p := int(v.panel) - 1
		if p < 0 {
			p = recoveryPanelCount - 1
		}
		v.panel = RecoveryPanel(p)
		v.cursor = 0
		v.regenerate()
		return true, nil
	case "j", "down":
		maxCursor := v.maxCursorForPanel()
		if v.cursor < maxCursor-1 {
			v.cursor++
			v.regenerate()
		}
		return true, nil
	case "k", "up":
		if v.cursor > 0 {
			v.cursor--
			v.regenerate()
		}
		return true, nil
	case "r":
		return true, func() tea.Msg { return RecoveryRefreshMsg{} }
	}
	return false, nil
}

func (v *RecoveryView) maxCursorForPanel() int {
	switch v.panel {
	case PanelSessions:
		return len(v.data.Sessions)
	case PanelHistory:
		return len(v.data.History)
	default:
		return 0
	}
}

func (v *RecoveryView) regenerate() {
	content := v.renderContent()
	v.Viewport.SetContent(content)
}

func (v *RecoveryView) renderContent() string {
	var b strings.Builder

	// Title.
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Recovery Dashboard", styles.IconAlert)))
	b.WriteString("\n\n")

	// Tab bar.
	tabs := []string{"Plan", "Sessions", "History"}
	for i, t := range tabs {
		if RecoveryPanel(i) == v.panel {
			b.WriteString(styles.TabActive.Render(fmt.Sprintf(" %s ", t)))
		} else {
			b.WriteString(styles.TabInactive.Render(fmt.Sprintf(" %s ", t)))
		}
		b.WriteString(" ")
	}
	b.WriteString("\n")

	// Panel content.
	switch v.panel {
	case PanelPlan:
		v.renderPlanPanel(&b)
	case PanelSessions:
		v.renderSessionsPanel(&b)
	case PanelHistory:
		v.renderHistoryPanel(&b)
	}

	// Help.
	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("tab: switch panel  j/k: navigate  r: refresh  esc: back"))

	return b.String()
}

func (v *RecoveryView) renderPlanPanel(b *strings.Builder) {
	plan := v.data.CurrentPlan

	if plan == nil {
		b.WriteString(styles.InfoStyle.Render("No active recovery plan. Fleet is healthy."))
		b.WriteString("\n")
		v.renderPolicySummary(b)
		return
	}

	// Severity badge.
	severityStr := fmt.Sprintf(" %s ", strings.ToUpper(plan.Severity))
	switch plan.Severity {
	case "catastrophic":
		b.WriteString(styles.AlertCritical.Render(fmt.Sprintf("%s CATASTROPHIC", styles.IconCritical)))
	case "major":
		b.WriteString(styles.AlertCritical.Render(fmt.Sprintf("%s MAJOR", styles.IconWarning)))
	case "minor":
		b.WriteString(styles.AlertWarning.Render(fmt.Sprintf("%s MINOR", styles.IconWarning)))
	default:
		b.WriteString(styles.InfoStyle.Render(severityStr))
	}
	b.WriteString("\n")

	// Stat boxes.
	statBoxes := []string{
		styles.StatBox.Render(fmt.Sprintf("%s DEAD\n  %d sessions", styles.IconCritical, plan.DeadCount)),
		styles.StatBox.Render(fmt.Sprintf("%s ALIVE\n  %d sessions", styles.IconRunning, plan.AliveCount)),
		styles.StatBox.Render(fmt.Sprintf("%s TOTAL\n  %d sessions", styles.IconSession, plan.TotalSessions)),
		styles.StatBox.Render(fmt.Sprintf("%s TO RESUME\n  %d sessions", styles.IconAlert, len(plan.SessionsToResume))),
	}
	b.WriteString(wrapStatBoxes(statBoxes, v.width))
	b.WriteString("\n")

	// Budget gauge.
	if v.data.BudgetTotal > 0 {
		b.WriteString(styles.HeaderStyle.Render("Recovery Budget"))
		b.WriteString("\n")
		gauge := components.InlineGauge(v.data.BudgetSpent, v.data.BudgetTotal, 30)
		b.WriteString(fmt.Sprintf("  %s $%.2f / $%.2f\n", gauge, v.data.BudgetSpent, v.data.BudgetTotal))
	}

	// Detected time.
	b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("Detected: %s", plan.DetectedAt.Format("2006-01-02 15:04:05"))))
	b.WriteString("\n")

	// Priority queue preview (top 5).
	b.WriteString(styles.HeaderStyle.Render("Priority Queue (top 5)"))
	b.WriteString("\n")
	limit := 5
	if len(plan.SessionsToResume) < limit {
		limit = len(plan.SessionsToResume)
	}
	for i := 0; i < limit; i++ {
		rs := plan.SessionsToResume[i]
		name := rs.SessionName
		if name == "" {
			name = rs.SessionID[:8]
		}
		b.WriteString(fmt.Sprintf("  %d. %s (%s) — %d open tasks\n",
			rs.Priority, name, rs.RepoName, rs.OpenTasks))
	}
	b.WriteString("\n")

	v.renderPolicySummary(b)
}

func (v *RecoveryView) renderPolicySummary(b *strings.Builder) {
	b.WriteString(styles.HeaderStyle.Render("Policy"))
	b.WriteString("\n")
	if v.data.PolicyEnabled {
		b.WriteString(styles.StatusRunning.Render("  Auto-execute: ENABLED"))
	} else {
		b.WriteString(styles.StatusIdle.Render("  Auto-execute: DISABLED"))
	}
	b.WriteString("\n")
}

func (v *RecoveryView) renderSessionsPanel(b *strings.Builder) {
	if len(v.data.Sessions) == 0 {
		b.WriteString(styles.InfoStyle.Render("No sessions to display."))
		return
	}

	// Header row.
	b.WriteString(styles.HeaderStyle.Render(
		fmt.Sprintf("  %-3s %-20s %-18s %-8s %-6s %-8s %-8s",
			"#", "Name", "Repo", "Status", "Tasks", "Git", "Cost")))
	b.WriteString("\n")

	for i, s := range v.data.Sessions {
		name := s.SessionName
		if name == "" {
			name = s.SessionID[:8]
		}
		if len(name) > 20 {
			name = name[:17] + "..."
		}
		repo := s.RepoName
		if len(repo) > 18 {
			repo = repo[:15] + "..."
		}

		// Status styling.
		statusStr := s.Status
		switch s.Status {
		case "succeeded":
			statusStr = styles.StatusCompleted.Render(statusStr)
		case "failed":
			statusStr = styles.StatusFailed.Render(statusStr)
		case "executing":
			statusStr = styles.StatusRunning.Render(statusStr)
		default:
			statusStr = styles.StatusIdle.Render(statusStr)
		}

		// Git indicator.
		gitStr := "clean"
		if s.HasUncommitted {
			gitStr = styles.AlertWarning.Render("dirty")
		} else if s.UnpushedCount > 0 {
			gitStr = styles.AlertWarning.Render(fmt.Sprintf("+%d", s.UnpushedCount))
		}

		line := fmt.Sprintf("  %-3d %-20s %-18s %-8s %-6d %-8s $%.2f",
			s.Priority, name, repo, s.Status, s.OpenTasks, gitStr, s.CostUSD)

		if i == v.cursor {
			b.WriteString(styles.SelectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
}

func (v *RecoveryView) renderHistoryPanel(b *strings.Builder) {
	if len(v.data.History) == 0 {
		b.WriteString(styles.InfoStyle.Render("No recovery history."))
		return
	}

	b.WriteString(styles.HeaderStyle.Render(
		fmt.Sprintf("  %-20s %-12s %-10s %-6s %-6s %-8s",
			"Detected", "Severity", "Status", "Dead", "Resumed", "Cost")))
	b.WriteString("\n")

	for i, op := range v.data.History {
		detected := op.DetectedAt.Format("2006-01-02 15:04")

		// Severity styling.
		sevStr := op.Severity
		switch op.Severity {
		case "catastrophic":
			sevStr = styles.AlertCritical.Render(sevStr)
		case "major":
			sevStr = styles.AlertCritical.Render(sevStr)
		case "minor":
			sevStr = styles.AlertWarning.Render(sevStr)
		}

		// Status styling.
		statStr := string(op.Status)
		switch op.Status {
		case session.RecoveryOpCompleted:
			statStr = styles.StatusCompleted.Render(statStr)
		case session.RecoveryOpFailed:
			statStr = styles.StatusFailed.Render(statStr)
		case session.RecoveryOpExecuting:
			statStr = styles.StatusRunning.Render(statStr)
		}

		line := fmt.Sprintf("  %-20s %-12s %-10s %-6d %-6d $%.2f",
			detected, op.Severity, string(op.Status), op.DeadCount, op.ResumedCount, op.TotalCostUSD)

		if i == v.cursor {
			b.WriteString(styles.SelectedStyle.Render(line))
		} else {
			b.WriteString(line)
		}
		b.WriteString("\n")
	}
}
