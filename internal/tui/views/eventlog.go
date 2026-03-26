package views

import (
	"fmt"
	"strings"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// EventLogView is a scrollable view of recent system events from the event bus.
type EventLogView struct {
	mu      sync.Mutex
	entries []events.Event
	offset  int // scroll offset (0 = bottom)
	height  int
	width   int
	maxEntries int
}

// NewEventLogView creates a new event log view.
func NewEventLogView() *EventLogView {
	return &EventLogView{
		maxEntries: 500,
		height:     20,
	}
}

// SetDimensions sets the viewport size.
func (v *EventLogView) SetDimensions(width, height int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.width = width
	v.height = height
}

// AddEntry appends an event to the log.
func (v *EventLogView) AddEntry(ev events.Event) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = append(v.entries, ev)
	if len(v.entries) > v.maxEntries {
		v.entries = v.entries[len(v.entries)-v.maxEntries:]
	}
}

// LoadHistory bulk-loads events (e.g. from bus history on view open).
func (v *EventLogView) LoadHistory(evts []events.Event) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.entries = make([]events.Event, len(evts))
	copy(v.entries, evts)
	if len(v.entries) > v.maxEntries {
		v.entries = v.entries[len(v.entries)-v.maxEntries:]
	}
	v.offset = 0
}

// ScrollUp moves the viewport up.
func (v *EventLogView) ScrollUp() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.offset < len(v.entries)-v.height {
		v.offset++
	}
}

// ScrollDown moves the viewport down.
func (v *EventLogView) ScrollDown() {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.offset > 0 {
		v.offset--
	}
}

// View renders the event log.
func (v *EventLogView) View() string {
	v.mu.Lock()
	entries := make([]events.Event, len(v.entries))
	copy(entries, v.entries)
	offset := v.offset
	height := v.height
	width := v.width
	v.mu.Unlock()

	var b strings.Builder

	b.WriteString(styles.TitleStyle.Render(fmt.Sprintf("%s Event Log", styles.IconSession)))
	b.WriteString(fmt.Sprintf("  %s\n\n", styles.InfoStyle.Render(fmt.Sprintf("%d events", len(entries)))))

	if len(entries) == 0 {
		b.WriteString(styles.InfoStyle.Render("  No events recorded yet."))
		b.WriteString("\n")
		return b.String()
	}

	// Show visible window (newest at bottom)
	visibleLines := height - 4 // reserve space for header/footer
	if visibleLines < 5 {
		visibleLines = 5
	}

	endIdx := len(entries) - offset
	startIdx := endIdx - visibleLines
	if startIdx < 0 {
		startIdx = 0
	}
	if endIdx > len(entries) {
		endIdx = len(entries)
	}

	for i := startIdx; i < endIdx; i++ {
		ev := entries[i]
		b.WriteString(formatEventLine(ev, width))
		b.WriteString("\n")
	}

	b.WriteString("\n")
	b.WriteString(styles.HelpStyle.Render("  j/k scroll  Esc back"))

	return b.String()
}

// formatEventLine renders a single event as a colored log line.
func formatEventLine(ev events.Event, width int) string {
	ts := ev.Timestamp.Format("15:04:05")
	typeStr := string(ev.Type)

	// Color by event category
	var typeStyled string
	switch {
	case strings.HasPrefix(typeStr, "session."):
		typeStyled = styles.StatusRunning.Render(typeStr)
	case strings.HasPrefix(typeStr, "loop."):
		typeStyled = styles.CircuitHalfOpen.Render(typeStr)
	case strings.HasPrefix(typeStr, "cost.") || strings.HasPrefix(typeStr, "budget."):
		typeStyled = styles.StatusFailed.Render(typeStr)
	case strings.HasPrefix(typeStr, "team."):
		typeStyled = styles.HeaderStyle.Render(typeStr)
	default:
		typeStyled = styles.InfoStyle.Render(typeStr)
	}

	detail := ""
	if ev.RepoName != "" {
		detail = ev.RepoName
	}
	if ev.SessionID != "" {
		sid := ev.SessionID
		if len(sid) > 8 {
			sid = sid[:8]
		}
		if detail != "" {
			detail += " "
		}
		detail += sid
	}
	if ev.Provider != "" {
		if detail != "" {
			detail += " "
		}
		detail += ev.Provider
	}

	// Add select data fields
	for _, key := range []string{"metric", "verdict", "reason", "error"} {
		if val, ok := ev.Data[key]; ok {
			if detail != "" {
				detail += " "
			}
			detail += fmt.Sprintf("%s=%v", key, val)
		}
	}

	maxDetail := width - 40
	if maxDetail < 10 {
		maxDetail = 10
	}
	if len(detail) > maxDetail {
		detail = detail[:maxDetail-3] + "..."
	}

	return fmt.Sprintf("  %s  %s  %s",
		styles.InfoStyle.Render(ts),
		typeStyled,
		detail)
}

