package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// playbackSpeeds lists the supported playback multipliers.
var playbackSpeeds = []float64{1, 2, 5, 10}

// replayFilter enumerates event type filters.
type replayFilter int

const (
	filterAll replayFilter = iota
	filterInput
	filterOutput
	filterTool
	filterStatus
)

func (f replayFilter) String() string {
	switch f {
	case filterInput:
		return "input"
	case filterOutput:
		return "output"
	case filterTool:
		return "tool"
	case filterStatus:
		return "status"
	default:
		return "all"
	}
}

func (f replayFilter) eventType() session.ReplayEventType {
	switch f {
	case filterInput:
		return session.ReplayInput
	case filterOutput:
		return session.ReplayOutput
	case filterTool:
		return session.ReplayTool
	case filterStatus:
		return session.ReplayStatus
	default:
		return ""
	}
}

// ReplayTickMsg advances auto-play by one event.
type ReplayTickMsg struct{}

// ReplayViewerModel is a BubbleTea model for stepping through recorded session events.
type ReplayViewerModel struct {
	events   []session.ReplayEvent
	filtered []session.ReplayEvent
	cursor   int // index into filtered
	filter   replayFilter

	// Auto-play
	playing    bool
	speedIndex int // index into playbackSpeeds

	// Search
	searching   bool
	searchQuery string
	searchHits  []int // indices into filtered that match

	// Dimensions
	width  int
	height int
}

// NewReplayViewerModel creates a new replay viewer from a slice of events.
func NewReplayViewerModel(events []session.ReplayEvent) ReplayViewerModel {
	m := ReplayViewerModel{
		events: events,
	}
	m.applyFilter()
	return m
}

// Init implements tea.Model.
func (m ReplayViewerModel) Init() tea.Cmd { return nil }

// Update implements tea.Model.
func (m ReplayViewerModel) Update(msg tea.Msg) (ReplayViewerModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if m.searching {
			return m.updateSearch(msg)
		}
		return m.updateNormal(msg)
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case ReplayTickMsg:
		if m.playing && len(m.filtered) > 0 {
			if m.cursor < len(m.filtered)-1 {
				m.cursor++
				return m, m.tickCmd()
			}
			m.playing = false
		}
	}
	return m, nil
}

func (m ReplayViewerModel) updateNormal(msg tea.KeyPressMsg) (ReplayViewerModel, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.cursor < len(m.filtered)-1 {
			m.cursor++
		}
	case "k", "up":
		if m.cursor > 0 {
			m.cursor--
		}
	case "g", "home":
		m.cursor = 0
	case "G", "end":
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		}
	case "p":
		m.playing = !m.playing
		if m.playing {
			return m, m.tickCmd()
		}
	case "s":
		m.speedIndex = (m.speedIndex + 1) % len(playbackSpeeds)
	case "f":
		m.cycleFilter()
	case "/":
		m.searching = true
		m.searchQuery = ""
		m.searchHits = nil
	case "n":
		m.nextSearchHit()
	case "N":
		m.prevSearchHit()
	}
	return m, nil
}

func (m *ReplayViewerModel) updateSearch(msg tea.KeyPressMsg) (ReplayViewerModel, tea.Cmd) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyEnter:
		m.searching = false
		m.executeSearch()
	case tea.KeyEscape:
		m.searching = false
		m.searchQuery = ""
		m.searchHits = nil
	case tea.KeyBackspace:
		if len(m.searchQuery) > 0 {
			m.searchQuery = m.searchQuery[:len(m.searchQuery)-1]
		}
	default:
		if k.Text != "" {
			m.searchQuery += k.Text
		}
	}
	return *m, nil
}

// View implements tea.Model.
func (m ReplayViewerModel) View() tea.View {
	var b strings.Builder

	// Header
	header := styles.TitleStyle.Render("  " + styles.IconLog + " Replay Viewer")
	if m.filter != filterAll {
		header += styles.InfoStyle.Render(fmt.Sprintf(" [filter: %s]", m.filter))
	}
	if m.playing {
		header += " " + styles.StatusRunning.Render("PLAYING")
	}
	b.WriteString(header)
	b.WriteRune('\n')
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("\u2500", m.width)))
	b.WriteRune('\n')

	// Event list
	viewHeight := m.viewHeight()
	start, end := m.visibleRange(viewHeight)

	baseTime := time.Time{}
	if len(m.events) > 0 {
		baseTime = m.events[0].Timestamp
	}

	hitSet := make(map[int]bool, len(m.searchHits))
	for _, idx := range m.searchHits {
		hitSet[idx] = true
	}

	for i := start; i < end; i++ {
		ev := m.filtered[i]
		offset := ev.Timestamp.Sub(baseTime)
		ts := styles.InfoStyle.Render(formatOffset(offset))
		typeBadge := replayTypeBadge(ev.Type)

		// Truncate data for display
		data := ev.Data
		maxDataLen := m.width - 30
		if maxDataLen < 20 {
			maxDataLen = 20
		}
		if len(data) > maxDataLen {
			data = data[:maxDataLen-3] + "..."
		}

		prefix := "  "
		if i == m.cursor {
			prefix = styles.SelectedStyle.Render("> ")
		}

		line := fmt.Sprintf("%s%s %s %s", prefix, ts, typeBadge, data)
		if hitSet[i] {
			line = styles.WarningStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteRune('\n')
	}

	// Pad remaining lines
	rendered := end - start
	for i := rendered; i < viewHeight; i++ {
		b.WriteRune('\n')
	}

	// Search bar
	if m.searching {
		b.WriteString(styles.CommandStyle.Render(fmt.Sprintf("  /%s_", m.searchQuery)))
		b.WriteRune('\n')
	} else {
		b.WriteRune('\n')
	}

	// Status bar
	pos := 0
	total := len(m.filtered)
	if total > 0 {
		pos = m.cursor + 1
	}
	speed := playbackSpeeds[m.speedIndex]
	status := fmt.Sprintf("  %d/%d | speed: %.0fx | filter: %s",
		pos, total, speed, m.filter)
	if len(m.searchHits) > 0 {
		status += fmt.Sprintf(" | matches: %d", len(m.searchHits))
	}
	b.WriteString(styles.HelpStyle.Render(status))
	b.WriteRune('\n')
	b.WriteString(styles.HelpStyle.Render(
		"  j/k:step p:play s:speed f:filter /:search n/N:next/prev match"))

	return tea.NewView(b.String())
}

// --- Accessors for testing ---

// Cursor returns the current cursor position.
func (m *ReplayViewerModel) Cursor() int { return m.cursor }

// FilterMode returns the current filter mode.
func (m *ReplayViewerModel) FilterMode() replayFilter { return m.filter }

// Playing returns whether auto-play is active.
func (m *ReplayViewerModel) Playing() bool { return m.playing }

// SpeedIndex returns the current speed index.
func (m *ReplayViewerModel) SpeedIndex() int { return m.speedIndex }

// Speed returns the current playback speed multiplier.
func (m *ReplayViewerModel) Speed() float64 { return playbackSpeeds[m.speedIndex] }

// Filtered returns the currently visible (filtered) events.
func (m *ReplayViewerModel) Filtered() []session.ReplayEvent { return m.filtered }

// SearchQuery returns the current search query.
func (m *ReplayViewerModel) SearchQuery() string { return m.searchQuery }

// SearchHits returns indices of matching events in the filtered list.
func (m *ReplayViewerModel) SearchHits() []int { return m.searchHits }

// Searching returns whether search input mode is active.
func (m *ReplayViewerModel) Searching() bool { return m.searching }

// SetDimensions updates the display width and height.
func (m *ReplayViewerModel) SetDimensions(width, height int) {
	m.width = width
	m.height = height
}

// --- Internal helpers ---

func (m *ReplayViewerModel) cycleFilter() {
	m.filter = (m.filter + 1) % (filterStatus + 1)
	m.applyFilter()
}

func (m *ReplayViewerModel) applyFilter() {
	if m.filter == filterAll {
		m.filtered = m.events
	} else {
		m.filtered = nil
		ft := m.filter.eventType()
		for _, ev := range m.events {
			if ev.Type == ft {
				m.filtered = append(m.filtered, ev)
			}
		}
	}
	if m.cursor >= len(m.filtered) {
		if len(m.filtered) > 0 {
			m.cursor = len(m.filtered) - 1
		} else {
			m.cursor = 0
		}
	}
}

func (m *ReplayViewerModel) executeSearch() {
	if m.searchQuery == "" {
		m.searchHits = nil
		return
	}
	q := strings.ToLower(m.searchQuery)
	m.searchHits = nil
	for i, ev := range m.filtered {
		if strings.Contains(strings.ToLower(ev.Data), q) {
			m.searchHits = append(m.searchHits, i)
		}
	}
	// Jump to first hit
	if len(m.searchHits) > 0 {
		m.cursor = m.searchHits[0]
	}
}

func (m *ReplayViewerModel) nextSearchHit() {
	if len(m.searchHits) == 0 {
		return
	}
	for _, idx := range m.searchHits {
		if idx > m.cursor {
			m.cursor = idx
			return
		}
	}
	// Wrap around
	m.cursor = m.searchHits[0]
}

func (m *ReplayViewerModel) prevSearchHit() {
	if len(m.searchHits) == 0 {
		return
	}
	for i := len(m.searchHits) - 1; i >= 0; i-- {
		if m.searchHits[i] < m.cursor {
			m.cursor = m.searchHits[i]
			return
		}
	}
	// Wrap around
	m.cursor = m.searchHits[len(m.searchHits)-1]
}

func (m ReplayViewerModel) viewHeight() int {
	h := m.height - 6 // header + separator + search line + status + help + padding
	if h < 1 {
		h = 10
	}
	return h
}

func (m ReplayViewerModel) visibleRange(viewHeight int) (start, end int) {
	total := len(m.filtered)
	if total == 0 {
		return 0, 0
	}

	// Keep cursor centered when possible
	half := viewHeight / 2
	start = m.cursor - half
	if start < 0 {
		start = 0
	}
	end = start + viewHeight
	if end > total {
		end = total
		start = end - viewHeight
		if start < 0 {
			start = 0
		}
	}
	return start, end
}

func (m ReplayViewerModel) tickCmd() tea.Cmd {
	if len(m.filtered) < 2 || m.cursor >= len(m.filtered)-1 {
		return nil
	}
	speed := playbackSpeeds[m.speedIndex]
	cur := m.filtered[m.cursor]
	next := m.filtered[m.cursor+1]
	gap := next.Timestamp.Sub(cur.Timestamp)
	if gap <= 0 || speed <= 0 {
		return func() tea.Msg { return ReplayTickMsg{} }
	}
	delay := time.Duration(float64(gap) / speed)
	// Cap delay at 2 seconds to keep UI responsive
	if delay > 2*time.Second {
		delay = 2 * time.Second
	}
	return tea.Tick(delay, func(time.Time) tea.Msg { return ReplayTickMsg{} })
}

// replayTypeBadge returns a styled badge for a replay event type.
func replayTypeBadge(t session.ReplayEventType) string {
	switch t {
	case session.ReplayInput:
		return styles.StatusCompleted.Render("[input ]")
	case session.ReplayOutput:
		return styles.StatusRunning.Render("[output]")
	case session.ReplayTool:
		return styles.WarningStyle.Render("[tool  ]")
	case session.ReplayStatus:
		return styles.InfoStyle.Render("[status]")
	default:
		return styles.InfoStyle.Render("[" + string(t) + "]")
	}
}

// formatOffset formats a duration as mm:ss.mmm
func formatOffset(d time.Duration) string {
	total := d.Milliseconds()
	mins := total / 60000
	secs := (total % 60000) / 1000
	ms := total % 1000
	return fmt.Sprintf("%02d:%02d.%03d", mins, secs, ms)
}
