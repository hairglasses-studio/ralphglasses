package views

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/components"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// RDCycleView wraps the R&D Cycle dashboard in a scrollable viewport.
type RDCycleView struct {
	Viewport *ViewportView
	cycles   []*session.CycleRun
	width    int
	height   int
}

// NewRDCycleView creates a new RDCycleView.
func NewRDCycleView() *RDCycleView {
	return &RDCycleView{
		Viewport: NewViewportView(),
	}
}

// SetCycles updates the cycle data and regenerates content.
func (v *RDCycleView) SetCycles(cycles []*session.CycleRun) {
	v.cycles = cycles
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *RDCycleView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *RDCycleView) Render() string {
	return v.Viewport.Render()
}

func (v *RDCycleView) regenerate() {
	content := RenderRDCycleDashboard(v.cycles, v.width, v.height)
	v.Viewport.SetContent(content)
}

// RenderRDCycleDashboard renders the full R&D cycle dashboard.
func RenderRDCycleDashboard(cycles []*session.CycleRun, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s R&D Cycle Dashboard", styles.IconTurns)))
	b.WriteString("\n\n")

	if len(cycles) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No R&D cycles found."))
		b.WriteString("\n")
		b.WriteString(styles.InfoStyle.Render("  Use cycle_plan to create a new cycle."))
		b.WriteString("\n")
		return b.String()
	}

	// Find active cycle (first non-complete, non-failed)
	var active *session.CycleRun
	for _, c := range cycles {
		if c.Phase != session.CycleComplete && c.Phase != session.CycleFailed {
			active = c
			break
		}
	}

	// Panel 1: Active Cycle header
	if active != nil {
		b.WriteString(renderActiveCycle(active, width))
		b.WriteString("\n")

		// Panel 2: Task Table
		b.WriteString(renderCycleTaskTable(active, width))
		b.WriteString("\n")

		// Panel 3: Findings
		if len(active.Findings) > 0 {
			b.WriteString(renderCycleFindings(active.Findings))
			b.WriteString("\n")
		}

		// Panel 4: Synthesis
		if active.Synthesis != nil {
			b.WriteString(renderCycleSynthesis(active.Synthesis))
			b.WriteString("\n")
		}
	}

	// Panel 5: Cycle History (last 5 cycles)
	b.WriteString(renderCycleHistory(cycles))

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  j/k: scroll  G/g: bottom/top  Ctrl+D/U: page  Esc: back"))

	return b.String()
}

// renderActiveCycle renders the active cycle header with progress bar.
func renderActiveCycle(c *session.CycleRun, width int) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Active Cycle", styles.IconRunning)))
	b.WriteString("\n")

	// Name and phase
	phaseStyle := phaseDisplayStyle(c.Phase)
	b.WriteString(fmt.Sprintf("  Name:      %s\n", styles.TitleStyle.Render(c.Name)))
	b.WriteString(fmt.Sprintf("  Phase:     %s\n", phaseStyle.Render(string(c.Phase))))
	b.WriteString(fmt.Sprintf("  Objective: %s\n", c.Objective))

	// Progress bar (tasks done/total)
	if len(c.Tasks) > 0 {
		done := 0
		for _, t := range c.Tasks {
			if t.Status == "done" {
				done++
			}
		}
		total := len(c.Tasks)
		gauge := components.InlineGauge(float64(done), float64(total), 20)
		b.WriteString(fmt.Sprintf("  Progress:  %s %d/%d tasks\n", gauge, done, total))
	} else {
		b.WriteString("  Progress:  No tasks defined\n")
	}

	// Success criteria
	if len(c.SuccessCriteria) > 0 {
		b.WriteString("  Criteria:\n")
		for _, sc := range c.SuccessCriteria {
			b.WriteString(fmt.Sprintf("    %s %s\n", styles.InfoStyle.Render("-"), sc))
		}
	}

	if c.Error != "" {
		b.WriteString(fmt.Sprintf("  Error:     %s\n", styles.StatusFailed.Render(c.Error)))
	}

	return b.String()
}

// renderCycleTaskTable renders the task table for a cycle.
func renderCycleTaskTable(c *session.CycleRun, width int) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Tasks", styles.IconSession)))
	b.WriteString("\n")

	if len(c.Tasks) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No tasks defined"))
		b.WriteString("\n")
		return b.String()
	}

	// Header
	b.WriteString(styles.InfoStyle.Render("  Status  Title                          Source    Loop ID"))
	b.WriteString("\n")

	for _, t := range c.Tasks {
		icon := taskStatusIcon(t.Status)
		title := t.Title
		maxTitle := 30
		if width > 100 {
			maxTitle = 50
		}
		if len(title) > maxTitle {
			title = title[:maxTitle-1] + "…"
		}
		loopID := t.LoopID
		if len(loopID) > 8 {
			loopID = loopID[:8]
		}
		if loopID == "" {
			loopID = "-"
		}
		source := t.Source
		if source == "" {
			source = "-"
		}
		if len(source) > 8 {
			source = source[:8]
		}

		b.WriteString(fmt.Sprintf("  %s     %-*s  %-8s  %s\n",
			icon, maxTitle, title, source, loopID))
	}

	return b.String()
}

// renderCycleFindings renders findings grouped by category.
func renderCycleFindings(findings []session.CycleFinding) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Findings", styles.IconAlert)))
	b.WriteString("\n")

	// Group by category
	groups := make(map[string][]session.CycleFinding)
	var order []string
	for _, f := range findings {
		cat := f.Category
		if cat == "" {
			cat = "general"
		}
		if _, exists := groups[cat]; !exists {
			order = append(order, cat)
		}
		groups[cat] = append(groups[cat], f)
	}

	for _, cat := range order {
		b.WriteString(fmt.Sprintf("  %s\n", styles.TitleStyle.Render(cat)))
		for _, f := range groups[cat] {
			icon := severityIcon(f.Severity)
			b.WriteString(fmt.Sprintf("    %s %s\n", icon, f.Description))
		}
	}

	return b.String()
}

// renderCycleSynthesis renders the synthesis summary.
func renderCycleSynthesis(s *session.CycleSynthesis) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Synthesis", styles.IconInfo)))
	b.WriteString("\n")

	if s.Summary != "" {
		b.WriteString(fmt.Sprintf("  %s\n\n", s.Summary))
	}

	if len(s.Accomplished) > 0 {
		b.WriteString(styles.StatusRunning.Render("  Accomplished:"))
		b.WriteString("\n")
		for _, item := range s.Accomplished {
			b.WriteString(fmt.Sprintf("    %s %s\n", styles.StatusRunning.Render("✓"), item))
		}
	}

	if len(s.Remaining) > 0 {
		b.WriteString(styles.WarningStyle.Render("  Remaining:"))
		b.WriteString("\n")
		for _, item := range s.Remaining {
			b.WriteString(fmt.Sprintf("    %s %s\n", styles.WarningStyle.Render("○"), item))
		}
	}

	if s.NextObjective != "" {
		b.WriteString(fmt.Sprintf("\n  %s %s\n",
			styles.HeaderStyle.Render("Next Objective:"),
			s.NextObjective))
	}

	if len(s.Patterns) > 0 {
		b.WriteString(styles.InfoStyle.Render("  Patterns:"))
		b.WriteString("\n")
		for _, p := range s.Patterns {
			b.WriteString(fmt.Sprintf("    %s %s\n", styles.InfoStyle.Render("~"), p))
		}
	}

	return b.String()
}

// renderCycleHistory renders the last 5 cycles.
func renderCycleHistory(cycles []*session.CycleRun) string {
	var b strings.Builder

	b.WriteString(styles.HeaderStyle.Render(fmt.Sprintf("%s Cycle History", styles.IconClock)))
	b.WriteString("\n")

	// Header
	b.WriteString(styles.InfoStyle.Render("  Name                    Phase          Tasks  Findings"))
	b.WriteString("\n")

	shown := cycles
	if len(shown) > 5 {
		shown = shown[:5]
	}

	for _, c := range shown {
		name := c.Name
		if len(name) > 22 {
			name = name[:21] + "…"
		}
		phaseStyle := phaseDisplayStyle(c.Phase)
		b.WriteString(fmt.Sprintf("  %-22s  %s  %-5d  %d\n",
			name,
			phaseStyle.Render(fmt.Sprintf("%-13s", string(c.Phase))),
			len(c.Tasks),
			len(c.Findings)))
	}

	return b.String()
}

// taskStatusIcon returns a status icon for a cycle task status.
func taskStatusIcon(status string) string {
	switch status {
	case "pending":
		return styles.InfoStyle.Render("○")
	case "executing":
		return styles.StatusRunning.Render("⣾")
	case "done":
		return styles.StatusRunning.Render("✓")
	case "failed":
		return styles.StatusFailed.Render("✗")
	default:
		return styles.InfoStyle.Render("○")
	}
}

// severityIcon returns a colored icon for a finding severity.
func severityIcon(severity string) string {
	switch severity {
	case "critical":
		return styles.AlertCritical.Render(styles.IconCritical)
	case "warning":
		return styles.AlertWarning.Render(styles.IconWarning)
	case "info":
		return styles.AlertInfo.Render(styles.IconInfo)
	default:
		return styles.InfoStyle.Render(styles.IconInfo)
	}
}

// phaseDisplayStyle returns the appropriate style for a cycle phase.
func phaseDisplayStyle(phase session.CyclePhase) lipgloss.Style {
	switch phase {
	case session.CycleComplete:
		return styles.StatusCompleted
	case session.CycleFailed:
		return styles.StatusFailed
	case session.CycleExecuting:
		return styles.StatusRunning
	case session.CycleProposed:
		return styles.InfoStyle
	default:
		return styles.WarningStyle
	}
}
