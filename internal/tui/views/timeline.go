package views

import (
	"fmt"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// TimelineEntry represents a session on the timeline.
type TimelineEntry struct {
	ID        string
	Provider  string
	StartTime time.Time
	EndTime   *time.Time
	Status    string
}

// RenderTimeline renders a horizontal bar chart timeline of sessions.
func RenderTimeline(entries []TimelineEntry, repoName string, width, height int) string {
	if len(entries) == 0 {
		return styles.InfoStyle.Render("  No sessions to display")
	}

	var b strings.Builder
	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("Session Timeline — %s", repoName)))
	b.WriteString("\n\n")

	// Find time range
	earliest := entries[0].StartTime
	latest := time.Now()
	for _, e := range entries {
		if e.StartTime.Before(earliest) {
			earliest = e.StartTime
		}
		if e.EndTime != nil && e.EndTime.After(latest) {
			latest = *e.EndTime
		}
	}

	totalDur := latest.Sub(earliest)
	if totalDur <= 0 {
		totalDur = time.Second
	}

	barWidth := max(
		// leave room for labels
		width-30, 20)

	// Time axis header
	b.WriteString(fmt.Sprintf("  %-20s", ""))
	b.WriteString(styles.InfoStyle.Render(fmt.Sprintf("%-*s%s",
		barWidth/2, earliest.Format("15:04"),
		latest.Format("15:04"))))
	b.WriteString("\n")

	// Separator
	b.WriteString(fmt.Sprintf("  %-20s", ""))
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("─", barWidth)))
	b.WriteString("\n")

	// Render each entry
	maxEntries := max(height-8, 5)
	shown := entries
	if len(shown) > maxEntries {
		shown = shown[len(shown)-maxEntries:]
	}

	for _, e := range shown {
		id := e.ID
		if len(id) > 8 {
			id = id[:8]
		}
		label := fmt.Sprintf("  %-8s %-7s ", id, e.Provider)
		b.WriteString(styles.ProviderStyle(e.Provider).Render(label))

		// Calculate bar position
		startOff := e.StartTime.Sub(earliest)
		endTime := latest
		if e.EndTime != nil {
			endTime = *e.EndTime
		}
		endOff := endTime.Sub(earliest)

		startCol := int(float64(startOff) / float64(totalDur) * float64(barWidth))
		endCol := int(float64(endOff) / float64(totalDur) * float64(barWidth))
		if endCol <= startCol {
			endCol = startCol + 1
		}
		if endCol > barWidth {
			endCol = barWidth
		}

		// Build bar using runes for proper Unicode block chars
		barRunes := make([]rune, barWidth)
		for i := range barRunes {
			barRunes[i] = ' '
		}
		for i := startCol; i < endCol; i++ {
			barRunes[i] = '█'
		}

		pre := string(barRunes[:startCol])
		filled := string(barRunes[startCol:endCol])
		post := string(barRunes[endCol:])

		style := statusTimelineStyle(e.Status)
		b.WriteString(pre)
		b.WriteString(style.Render(filled))
		b.WriteString(post)
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  Legend: "))
	b.WriteString(styles.StatusRunning.Render("running"))
	b.WriteString("  ")
	b.WriteString(styles.StatusCompleted.Render("completed"))
	b.WriteString("  ")
	b.WriteString(styles.StatusFailed.Render("errored"))
	b.WriteString("  ")
	b.WriteString(styles.StatusIdle.Render("stopped"))
	b.WriteString("  ")
	b.WriteString(styles.WarningStyle.Render("launching"))

	return b.String()
}

func statusTimelineStyle(status string) StyleFunc {
	switch status {
	case "running":
		return wrapStyle(styles.StatusRunning)
	case "completed":
		return wrapStyle(styles.StatusCompleted)
	case "errored":
		return wrapStyle(styles.StatusFailed)
	case "stopped":
		return wrapStyle(styles.StatusIdle)
	case "launching":
		return wrapStyle(styles.WarningStyle)
	default:
		return wrapStyle(styles.InfoStyle)
	}
}

// StyleFunc allows timeline to use lipgloss styles uniformly.
type StyleFunc = interface{ Render(strs ...string) string }

func wrapStyle(s StyleFunc) StyleFunc { return s }

// TimelineViewport wraps RenderTimeline in a scrollable viewport.
type TimelineViewport struct {
	Viewport *ViewportView
	entries  []TimelineEntry
	repoName string
	width    int
	height   int
}

// NewTimelineViewport creates a new TimelineViewport.
func NewTimelineViewport() *TimelineViewport {
	return &TimelineViewport{
		Viewport: NewViewportView(),
	}
}

// SetData updates the timeline entries and regenerates content.
func (v *TimelineViewport) SetData(entries []TimelineEntry, repoName string) {
	v.entries = entries
	v.repoName = repoName
	v.regenerate()
}

// SetDimensions updates the available width and height.
func (v *TimelineViewport) SetDimensions(width, height int) {
	v.width = width
	v.height = height
	v.Viewport.SetDimensions(width, height)
	v.regenerate()
}

// Render returns the scrollable viewport content.
func (v *TimelineViewport) Render() string {
	return v.Viewport.Render()
}

func (v *TimelineViewport) regenerate() {
	if v.entries == nil && v.repoName == "" {
		return
	}
	content := RenderTimeline(v.entries, v.repoName, v.width, v.height)
	v.Viewport.SetContent(content)
}
