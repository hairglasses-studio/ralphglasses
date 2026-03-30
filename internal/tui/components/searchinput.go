package components

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// SearchResultType identifies the kind of entity matched by a search.
type SearchResultType string

const (
	SearchTypeRepo    SearchResultType = "repo"
	SearchTypeSession SearchResultType = "session"
	SearchTypeTeam    SearchResultType = "team"
	SearchTypeCycle   SearchResultType = "cycle"
)

// SearchResult represents a single match from a global search.
type SearchResult struct {
	Type       SearchResultType
	Name       string
	Path       string  // repo path, session ID, team name, or cycle ID
	Score      float64 // 0-100 relevance score
	ViewTarget int     // ViewMode constant to navigate to
}

// SearchInput is an interactive search bar with inline results dropdown.
type SearchInput struct {
	Query    string
	Active   bool
	Results  []SearchResult
	Selected int
	maxShow  int // max results to display in dropdown
}

// NewSearchInput creates a SearchInput with sensible defaults.
func NewSearchInput() *SearchInput {
	return &SearchInput{
		maxShow: 12,
	}
}

// Activate enables the search overlay and resets state.
func (s *SearchInput) Activate() {
	s.Active = true
	s.Query = ""
	s.Results = nil
	s.Selected = 0
}

// Deactivate disables the search overlay.
func (s *SearchInput) Deactivate() {
	s.Active = false
}

// Reset clears query and results but keeps active state.
func (s *SearchInput) Reset() {
	s.Query = ""
	s.Results = nil
	s.Selected = 0
}

// HandleKey processes a keypress while the search input is active.
// Returns (selected result, true) when the user confirms a selection with Enter.
// Returns (zero, false) for all other keys. Caller should check Active after
// this call — Escape sets Active = false.
func (s *SearchInput) HandleKey(msg tea.KeyMsg) (SearchResult, bool) {
	switch msg.Type {
	case tea.KeyEscape:
		s.Deactivate()
		return SearchResult{}, false

	case tea.KeyEnter:
		if len(s.Results) > 0 && s.Selected < len(s.Results) {
			return s.Results[s.Selected], true
		}
		return SearchResult{}, false

	case tea.KeyUp:
		if s.Selected > 0 {
			s.Selected--
		}
		return SearchResult{}, false

	case tea.KeyDown:
		if s.Selected < len(s.Results)-1 {
			s.Selected++
		}
		return SearchResult{}, false

	case tea.KeyBackspace, tea.KeyDelete:
		if len(s.Query) > 0 {
			s.Query = s.Query[:len(s.Query)-1]
			s.Selected = 0
		}
		return SearchResult{}, false

	case tea.KeyRunes:
		s.Query += string(msg.Runes)
		s.Selected = 0
		return SearchResult{}, false
	}

	return SearchResult{}, false
}

// SetResults replaces the current result set (called after re-scoring).
func (s *SearchInput) SetResults(results []SearchResult) {
	s.Results = results
	if s.Selected >= len(results) {
		s.Selected = max(0, len(results)-1)
	}
}

// searchBarStyle is the styled border for the search bar.
var searchBarStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(styles.ColorAccent).
	Padding(0, 1)

// resultSelectedStyle highlights the selected result.
var resultSelectedStyle = lipgloss.NewStyle().
	Bold(true).
	Foreground(styles.ColorBrightWhite).
	Background(styles.ColorDarkBg)

// resultNormalStyle for non-selected results.
var resultNormalStyle = lipgloss.NewStyle().
	Foreground(styles.ColorGray)

// resultTypeStyle renders the type badge.
var resultTypeStyle = lipgloss.NewStyle().
	Foreground(styles.ColorPrimary).
	Bold(true)

// scoreStyle renders the score.
var scoreStyle = lipgloss.NewStyle().
	Foreground(styles.ColorDarkGray)

// View renders the search bar and results dropdown.
func (s *SearchInput) View(width int) string {
	if !s.Active {
		return ""
	}

	var b strings.Builder

	// Search bar
	prompt := styles.CommandStyle.Render("search: ")
	cursor := styles.CommandStyle.Render("|")
	barContent := fmt.Sprintf("%s%s%s", prompt, s.Query, cursor)

	barWidth := width - 4 // account for border padding
	if barWidth < 20 {
		barWidth = 20
	}
	bar := searchBarStyle.Width(barWidth).Render(barContent)
	b.WriteString(bar)
	b.WriteString("\n")

	// Results dropdown
	if len(s.Results) == 0 && s.Query != "" {
		b.WriteString(styles.InfoStyle.Render("  No results"))
		b.WriteString("\n")
		return b.String()
	}

	shown := s.Results
	if len(shown) > s.maxShow {
		shown = shown[:s.maxShow]
	}

	for i, r := range shown {
		typeLabel := fmt.Sprintf("[%-7s]", r.Type)
		scoreFmt := fmt.Sprintf("%3.0f", r.Score)

		line := fmt.Sprintf("  %s %s  %s",
			resultTypeStyle.Render(typeLabel),
			r.Name,
			scoreStyle.Render(scoreFmt))

		if i == s.Selected {
			line = resultSelectedStyle.Render(line)
		} else {
			line = resultNormalStyle.Render(line)
		}
		b.WriteString(line)
		b.WriteString("\n")
	}

	if len(s.Results) > s.maxShow {
		b.WriteString(styles.InfoStyle.Render(
			fmt.Sprintf("  ... and %d more", len(s.Results)-s.maxShow)))
		b.WriteString("\n")
	}

	b.WriteString(styles.HelpStyle.Render("  j/k or arrows: navigate  Enter: go  Esc: cancel"))

	return b.String()
}
