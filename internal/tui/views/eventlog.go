package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

const maxEventLogEntries = 200

// EventLogEntry represents a single event in the log.
type EventLogEntry struct {
	Timestamp time.Time
	Type      string
	Session   string
	Message   string
}

// EventLogView displays a real-time scrollable event log.
type EventLogView struct {
	entries  []EventLogEntry
	filtered []EventLogEntry
	filter   string // empty = show all, or event type prefix to filter
	paused   bool
	scrollPos int
	width    int
	height   int
}

// NewEventLogView creates a new event log view.
func NewEventLogView() EventLogView {
	return EventLogView{
		entries: make([]EventLogEntry, 0, maxEventLogEntries),
	}
}

// AddEntry adds a new event to the log, capping at maxEventLogEntries.
func (v *EventLogView) AddEntry(entry EventLogEntry) {
	v.entries = append(v.entries, entry)
	if len(v.entries) > maxEventLogEntries {
		v.entries = v.entries[len(v.entries)-maxEventLogEntries:]
	}
	v.applyFilter()
	if !v.paused {
		v.scrollToBottom()
	}
}

// Init implements tea.Model.
func (v EventLogView) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (v EventLogView) Update(msg tea.Msg) (EventLogView, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "f":
			v.cycleFilter()
		case "p":
			v.paused = !v.paused
		case "up", "k":
			if v.scrollPos > 0 {
				v.scrollPos--
			}
		case "down", "j":
			v.scrollDown()
		case "G":
			v.scrollToBottom()
		case "g":
			v.scrollPos = 0
		}
	case tea.WindowSizeMsg:
		v.width = msg.Width
		v.height = msg.Height
		v.clampScrollPos()
	}
	return v, nil
}

// View implements tea.Model.
func (v EventLogView) View() string {
	var b strings.Builder

	// Header
	header := styles.TitleStyle.Render("  " + styles.IconLog + " Events Log")
	if v.filter != "" {
		header += styles.InfoStyle.Render(fmt.Sprintf(" [filter: %s]", v.filter))
	}
	if v.paused {
		header += " " + styles.WarningStyle.Render("(PAUSED)")
	}
	b.WriteString(header)
	b.WriteRune('\n')
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("\u2500", v.width)))
	b.WriteRune('\n')

	// Visible entries
	visible := v.filtered
	viewHeight := v.viewHeight()

	start := v.scrollPos
	end := start + viewHeight
	if start > len(visible) {
		start = len(visible)
	}
	if end > len(visible) {
		end = len(visible)
	}

	for i := start; i < end; i++ {
		e := visible[i]
		ts := styles.InfoStyle.Render(e.Timestamp.Format("15:04:05"))
		typeBadge := colorForType(e.Type)
		line := fmt.Sprintf("  %s %s %s", ts, typeBadge, e.Message)
		b.WriteString(line)
		b.WriteRune('\n')
	}

	// Footer
	b.WriteRune('\n')
	b.WriteString(styles.HelpStyle.Render(
		fmt.Sprintf("  %d events | f:filter p:pause j/k:scroll G:bottom g:top", len(v.filtered))))

	return b.String()
}

// Entries returns the current entries (for testing).
func (v *EventLogView) Entries() []EventLogEntry {
	return v.entries
}

// Filtered returns the current filtered entries (for testing).
func (v *EventLogView) Filtered() []EventLogEntry {
	return v.filtered
}

// Filter returns the current filter string (for testing).
func (v *EventLogView) Filter() string {
	return v.filter
}

// Paused returns whether the view is paused (for testing).
func (v *EventLogView) Paused() bool {
	return v.paused
}

// ScrollPos returns the current scroll position (for testing).
func (v *EventLogView) ScrollPos() int {
	return v.scrollPos
}

// SetDimensions updates the width and height.
func (v *EventLogView) SetDimensions(width, height int) {
	v.width = width
	v.height = height
}

// colorForType returns a styled badge for the event type using the project styles.
func colorForType(eventType string) string {
	switch {
	case strings.Contains(eventType, "error"):
		return styles.StatusFailed.Render("[" + eventType + "]")
	case strings.Contains(eventType, "session"):
		return styles.StatusCompleted.Render("[" + eventType + "]")
	case strings.Contains(eventType, "loop"):
		return styles.StatusRunning.Render("[" + eventType + "]")
	case strings.Contains(eventType, "fleet"):
		return styles.WarningStyle.Render("[" + eventType + "]")
	default:
		return styles.InfoStyle.Render("[" + eventType + "]")
	}
}

// cycleFilter cycles through: all -> session -> loop -> fleet -> error -> all.
func (v *EventLogView) cycleFilter() {
	filters := []string{"", "session", "loop", "fleet", "error"}
	for i, f := range filters {
		if f == v.filter {
			v.filter = filters[(i+1)%len(filters)]
			v.applyFilter()
			return
		}
	}
	v.filter = ""
	v.applyFilter()
}

// applyFilter rebuilds the filtered slice from entries and clamps scroll position.
func (v *EventLogView) applyFilter() {
	if v.filter == "" {
		v.filtered = v.entries
	} else {
		v.filtered = nil
		for _, e := range v.entries {
			if strings.Contains(e.Type, v.filter) {
				v.filtered = append(v.filtered, e)
			}
		}
	}
	v.clampScrollPos()
}

// clampScrollPos ensures scrollPos is within valid bounds after filter or resize.
func (v *EventLogView) clampScrollPos() {
	maxPos := len(v.filtered) - v.viewHeight()
	if maxPos < 0 {
		maxPos = 0
	}
	if v.scrollPos > maxPos {
		v.scrollPos = maxPos
	}
	if v.scrollPos < 0 {
		v.scrollPos = 0
	}
}

// viewHeight returns the number of lines available for entries.
func (v *EventLogView) viewHeight() int {
	h := v.height - 4 // header + separator + footer + blank line
	if h < 1 {
		h = 10
	}
	return h
}

// scrollToBottom moves the scroll position to show the latest entries.
func (v *EventLogView) scrollToBottom() {
	vh := v.viewHeight()
	v.scrollPos = len(v.filtered) - vh
	if v.scrollPos < 0 {
		v.scrollPos = 0
	}
}

// scrollDown moves the scroll position down by one line, bounded.
func (v *EventLogView) scrollDown() {
	vh := v.viewHeight()
	if v.scrollPos < len(v.filtered)-vh {
		v.scrollPos++
	}
}

// ScrollDown is the exported version of scrollDown.
func (v *EventLogView) ScrollDown() { v.scrollDown() }

// ScrollUp moves the scroll position up by one line.
func (v *EventLogView) ScrollUp() {
	if v.scrollPos > 0 {
		v.scrollPos--
	}
}

// LoadHistory populates the event log from a slice of entries.
func (v *EventLogView) LoadHistory(entries []EventLogEntry) {
	for _, e := range entries {
		v.AddEntry(e)
	}
}
