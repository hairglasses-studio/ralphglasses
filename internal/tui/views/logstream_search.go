package views

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// LogLine represents a single parsed log entry.
type LogLine struct {
	Index     int
	Timestamp time.Time
	Level     string
	Message   string
}

// LogSearchModel is a tea.Model that provides interactive search over log lines
// with match highlighting and navigation.
type LogSearchModel struct {
	lines        []LogLine
	query        string
	matches      []int
	currentMatch int
	scrollOffset int
	width        int
	height       int
	searching    bool
}

// NewLogSearch creates a LogSearchModel pre-loaded with the given log lines.
func NewLogSearch(lines []LogLine) LogSearchModel {
	return LogSearchModel{
		lines:        lines,
		currentMatch: -1,
	}
}

// Search performs a case-insensitive substring search across all log lines
// and returns the indices of matching lines.
func (m *LogSearchModel) Search(query string) []int {
	if query == "" {
		return nil
	}
	needle := strings.ToLower(query)
	var matches []int
	for i, line := range m.lines {
		haystack := strings.ToLower(line.Message)
		if strings.Contains(haystack, needle) {
			matches = append(matches, i)
		}
	}
	return matches
}

// Init satisfies tea.Model.
func (m LogSearchModel) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model.
func (m LogSearchModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		if m.searching {
			return m.updateSearchMode(msg)
		}
		return m.updateNormalMode(msg)
	}
	return m, nil
}

func (m LogSearchModel) updateSearchMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	k := msg.Key()
	switch k.Code {
	case tea.KeyEscape:
		m.searching = false
		return m, nil
	case tea.KeyEnter:
		m.searching = false
		m.matches = m.Search(m.query)
		if len(m.matches) > 0 {
			m.currentMatch = 0
			m.scrollToMatch()
		} else {
			m.currentMatch = -1
		}
		return m, nil
	case tea.KeyBackspace:
		if len(m.query) > 0 {
			m.query = m.query[:len(m.query)-1]
		}
		return m, nil
	default:
		if k.Text != "" {
			m.query += k.Text
		}
		return m, nil
	}
}

func (m LogSearchModel) updateNormalMode(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "/":
		m.searching = true
		m.query = ""
		m.matches = nil
		m.currentMatch = -1
		return m, nil
	case "n":
		m.nextMatch()
		return m, nil
	case "N":
		m.prevMatch()
		return m, nil
	case "up", "k":
		if m.scrollOffset > 0 {
			m.scrollOffset--
		}
		return m, nil
	case "down", "j":
		maxOffset := m.maxScrollOffset()
		if m.scrollOffset < maxOffset {
			m.scrollOffset++
		}
		return m, nil
	case "q":
		return m, tea.Quit
	case "esc":
		m.query = ""
		m.matches = nil
		m.currentMatch = -1
		return m, nil
	}
	return m, nil
}

func (m *LogSearchModel) nextMatch() {
	if len(m.matches) == 0 {
		return
	}
	m.currentMatch = (m.currentMatch + 1) % len(m.matches)
	m.scrollToMatch()
}

func (m *LogSearchModel) prevMatch() {
	if len(m.matches) == 0 {
		return
	}
	m.currentMatch--
	if m.currentMatch < 0 {
		m.currentMatch = len(m.matches) - 1
	}
	m.scrollToMatch()
}

func (m *LogSearchModel) scrollToMatch() {
	if m.currentMatch < 0 || m.currentMatch >= len(m.matches) {
		return
	}
	target := m.matches[m.currentMatch]
	viewHeight := m.viewableHeight()
	if viewHeight <= 0 {
		viewHeight = 1
	}
	// Center the match in the viewport.
	m.scrollOffset = max(target-viewHeight/2, 0)
	maxOffset := m.maxScrollOffset()
	if m.scrollOffset > maxOffset {
		m.scrollOffset = maxOffset
	}
}

func (m *LogSearchModel) viewableHeight() int {
	// Reserve 3 lines: header, separator, status bar.
	h := max(m.height-3, 1)
	return h
}

func (m *LogSearchModel) maxScrollOffset() int {
	max := len(m.lines) - m.viewableHeight()
	if max < 0 {
		return 0
	}
	return max
}

// View satisfies tea.Model.
func (m LogSearchModel) View() tea.View {
	var b strings.Builder

	viewHeight := m.viewableHeight()

	// Header / status line
	if m.searching {
		b.WriteString(styles.HeaderStyle.Render("Search: "))
		b.WriteString(m.query)
		b.WriteString("_")
	} else if m.query != "" && len(m.matches) > 0 {
		b.WriteString(styles.HeaderStyle.Render(
			fmt.Sprintf("match %d of %d", m.currentMatch+1, len(m.matches)),
		))
	} else if m.query != "" {
		b.WriteString(styles.WarningStyle.Render("no matches"))
	} else {
		b.WriteString(styles.HeaderStyle.Render(
			fmt.Sprintf("Log Search  (%d lines)", len(m.lines)),
		))
	}
	b.WriteRune('\n')

	// Separator
	width := m.width
	if width <= 0 {
		width = 80
	}
	b.WriteString(styles.InfoStyle.Render(strings.Repeat("\u2500", width)))
	b.WriteRune('\n')

	// Build a set of matching line indices for O(1) lookup.
	matchSet := make(map[int]bool, len(m.matches))
	for _, idx := range m.matches {
		matchSet[idx] = true
	}
	currentHighlight := -1
	if m.currentMatch >= 0 && m.currentMatch < len(m.matches) {
		currentHighlight = m.matches[m.currentMatch]
	}

	// Render visible lines.
	end := m.scrollOffset + viewHeight
	if end > len(m.lines) {
		end = len(m.lines)
	}
	for i := m.scrollOffset; i < end; i++ {
		line := m.lines[i]
		text := fmt.Sprintf("%s [%s] %s",
			line.Timestamp.Format("15:04:05"),
			line.Level,
			line.Message,
		)
		if width > 0 && len([]rune(text)) > width {
			text = string([]rune(text)[:width])
		}

		if i == currentHighlight {
			text = styles.SelectedStyle.Render(text)
		} else if matchSet[i] {
			text = styles.StatusRunning.Render(text)
		} else {
			text = colorizeLine(text)
		}

		b.WriteString(text)
		if i < end-1 {
			b.WriteRune('\n')
		}
	}

	// Pad remaining lines if content is shorter than viewport.
	rendered := end - m.scrollOffset
	for i := rendered; i < viewHeight; i++ {
		b.WriteRune('\n')
	}

	// Help bar
	b.WriteRune('\n')
	b.WriteString(styles.HelpStyle.Render("  /: search  n/N: next/prev match  j/k: scroll  Esc: clear  q: quit"))

	return tea.NewView(b.String())
}
