package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LoopControlData holds the snapshot data for a single loop in the control panel.
type LoopControlData struct {
	ID              string
	RepoName        string
	Status          string
	Paused          bool
	IterCount       int
	LastIterStatus  string
	LastIterTask    string
	LastIterError   string
	CreatedAt       time.Time
	LastIterEndedAt *time.Time
	AvgIterDuration time.Duration
	NextEstimate    string
}

// SnapshotLoopControl extracts display data from active loops.
func SnapshotLoopControl(loops []*session.LoopRun) []LoopControlData {
	out := make([]LoopControlData, 0, len(loops))
	for _, l := range loops {
		l.Lock()
		d := LoopControlData{
			ID:        l.ID,
			RepoName:  l.RepoName,
			Status:    l.Status,
			Paused:    l.Paused,
			IterCount: len(l.Iterations),
			CreatedAt: l.CreatedAt,
		}
		if d.IterCount > 0 {
			last := l.Iterations[d.IterCount-1]
			d.LastIterStatus = last.Status
			d.LastIterTask = last.Task.Title
			d.LastIterError = last.Error
			d.LastIterEndedAt = last.EndedAt

			// Compute average iteration duration from completed iterations.
			var total time.Duration
			var count int
			for _, it := range l.Iterations {
				if it.EndedAt != nil {
					dur := it.EndedAt.Sub(it.StartedAt)
					if dur > 0 {
						total += dur
						count++
					}
				}
			}
			if count > 0 {
				d.AvgIterDuration = total / time.Duration(count)
			}
		}
		l.Unlock()

		// Compute next iteration estimate.
		if d.Paused {
			d.NextEstimate = "paused"
		} else if d.Status == "running" {
			if d.LastIterEndedAt != nil && d.AvgIterDuration > 0 {
				est := d.LastIterEndedAt.Add(d.AvgIterDuration)
				if est.After(time.Now()) {
					d.NextEstimate = fmt.Sprintf("~%s", FormatDuration(time.Until(est)))
				} else {
					d.NextEstimate = "imminent"
				}
			} else {
				d.NextEstimate = "imminent"
			}
		} else {
			d.NextEstimate = "—"
		}

		out = append(out, d)
	}
	return out
}

// RenderLoopControlPanel renders the loop control panel view.
func RenderLoopControlPanel(data []LoopControlData, selectedIdx, width, height int) string {
	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf(" %s Loop Control Panel ", styles.IconRunning)))
	b.WriteString("\n\n")

	if len(data) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No active loops — start a loop from the Repos tab (S)"))
		b.WriteString("\n\n")
		b.WriteString(styles.HelpStyle.Render("  Esc back"))
		return b.String()
	}

	// Summary line
	running, paused, stopped := 0, 0, 0
	for _, d := range data {
		switch {
		case d.Paused:
			paused++
		case d.Status == "running":
			running++
		default:
			stopped++
		}
	}
	b.WriteString(fmt.Sprintf("  %s %d running  %s %d paused  %s %d other  (%d total)\n\n",
		styles.StatusIcon("running"), running,
		styles.StatusIcon("paused"), paused,
		styles.StatusIcon("stopped"), stopped,
		len(data)))

	// Loop rows
	for i, d := range data {
		prefix := "  "
		if i == selectedIdx {
			prefix = styles.SelectedStyle.Render("▸ ")
		}

		id := d.ID
		if len(id) > 8 {
			id = id[:8]
		}

		statusLabel := d.Status
		if d.Paused {
			statusLabel = "paused"
		}

		b.WriteString(fmt.Sprintf("%s%s  %-16s  %s %-10s  iters:%-4d  next: %s\n",
			prefix,
			id,
			d.RepoName,
			styles.StatusIcon(statusLabel),
			styles.StatusStyle(statusLabel).Render(statusLabel),
			d.IterCount,
			d.NextEstimate,
		))

		// Show last iteration detail for selected loop
		if i == selectedIdx {
			if d.LastIterTask != "" {
				b.WriteString(fmt.Sprintf("    Task:   %s\n", d.LastIterTask))
			}
			if d.LastIterStatus != "" {
				b.WriteString(fmt.Sprintf("    Result: %s %s\n",
					styles.StatusIcon(d.LastIterStatus),
					styles.StatusStyle(d.LastIterStatus).Render(d.LastIterStatus)))
			}
			if d.LastIterError != "" {
				b.WriteString(fmt.Sprintf("    Error:  %s\n",
					styles.StatusFailed.Render(d.LastIterError)))
			}
			elapsed := time.Since(d.CreatedAt)
			b.WriteString(fmt.Sprintf("    Uptime: %s\n", FormatDuration(elapsed)))
			if d.AvgIterDuration > 0 {
				b.WriteString(fmt.Sprintf("    Avg iteration: %s\n", FormatDuration(d.AvgIterDuration)))
			}
		}
		b.WriteString("\n")
	}

	b.WriteString(styles.HelpStyle.Render("  j/k navigate  s force-step  r run/stop  p pause/resume  Esc back"))

	return b.String()
}

// LoopControlView wraps RenderLoopControlPanel in a scrollable viewport.
type LoopControlView struct {
	Viewport    *ViewportView
	data        []LoopControlData
	selectedIdx int
	width       int
	height      int
}

// NewLoopControlView creates a new LoopControlView.
func NewLoopControlView() *LoopControlView {
	return &LoopControlView{
		Viewport: NewViewportView(),
	}
}

// SetData updates the loop control data and regenerates content.
func (v *LoopControlView) SetData(data []LoopControlData, selectedIdx int) {
	v.data = data
	v.selectedIdx = selectedIdx
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *LoopControlView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *LoopControlView) Render() string {
	return v.Viewport.Render()
}

func (v *LoopControlView) regenerate() {
	content := RenderLoopControlPanel(v.data, v.selectedIdx, v.width, v.height)
	v.Viewport.SetContent(content)
}
