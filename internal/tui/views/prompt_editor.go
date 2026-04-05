package views

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/styles"
)

// PromptAcceptedMsg is emitted when the user accepts the enhanced prompt.
type PromptAcceptedMsg struct {
	Text     string
	Provider string
}

// PromptEditorPane identifies which pane is focused.
type PromptEditorPane int

const (
	PaneOriginal PromptEditorPane = iota
	PaneEnhanced
)

const promptEditorPaneCount = 2

// String returns a display label for the pane.
func (p PromptEditorPane) String() string {
	switch p {
	case PaneOriginal:
		return "Original"
	case PaneEnhanced:
		return "Enhanced"
	default:
		return "?"
	}
}

// QualityScore holds the prompt quality evaluation for display.
type QualityScore struct {
	Overall    int              // 0-100
	Grade      string           // A/B/C/D/F
	Dimensions []ScoreDimension // individual dimensions
}

// ScoreDimension is a single scoring axis.
type ScoreDimension struct {
	Name  string
	Score int    // 0-100
	Grade string // A/B/C/D/F
}

// PromptEditorModel is the side-by-side prompt A/B comparison view.
// It implements tea.Model and shows original vs enhanced prompts with
// quality scores and a provider selector.
type PromptEditorModel struct {
	original    string
	enhanced    string
	activePane  PromptEditorPane
	providers   []string
	providerIdx int
	score       *QualityScore
	scrollLeft  int
	scrollRight int
	width       int
	height      int
}

// NewPromptEditor creates a PromptEditorModel with the original prompt text
// and the list of available provider names (e.g. "claude", "gemini", "openai").
func NewPromptEditor(original string, providers []string) PromptEditorModel {
	if len(providers) == 0 {
		providers = []string{"claude"}
	}
	return PromptEditorModel{
		original:  original,
		providers: providers,
	}
}

// SetEnhanced updates the enhanced prompt text.
func (m *PromptEditorModel) SetEnhanced(text string) {
	m.enhanced = text
}

// SetScore updates the quality score display.
func (m *PromptEditorModel) SetScore(s *QualityScore) {
	m.score = s
}

// ActivePane returns the currently focused pane.
func (m PromptEditorModel) ActivePane() PromptEditorPane {
	return m.activePane
}

// SelectedProvider returns the name of the currently selected provider.
func (m PromptEditorModel) SelectedProvider() string {
	if m.providerIdx < 0 || m.providerIdx >= len(m.providers) {
		return ""
	}
	return m.providers[m.providerIdx]
}

// Init satisfies tea.Model.
func (m PromptEditorModel) Init() tea.Cmd {
	return nil
}

// Update satisfies tea.Model.
func (m PromptEditorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m PromptEditorModel) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "tab":
		m.activePane = PromptEditorPane((int(m.activePane) + 1) % promptEditorPaneCount)
		return m, nil

	case "shift+tab":
		p := int(m.activePane) - 1
		if p < 0 {
			p = promptEditorPaneCount - 1
		}
		m.activePane = PromptEditorPane(p)
		return m, nil

	case "enter":
		text := m.original
		if m.activePane == PaneEnhanced && m.enhanced != "" {
			text = m.enhanced
		}
		return m, func() tea.Msg {
			return PromptAcceptedMsg{
				Text:     text,
				Provider: m.SelectedProvider(),
			}
		}

	case "p":
		if len(m.providers) > 1 {
			m.providerIdx = (m.providerIdx + 1) % len(m.providers)
		}
		return m, nil

	case "P":
		if len(m.providers) > 1 {
			m.providerIdx--
			if m.providerIdx < 0 {
				m.providerIdx = len(m.providers) - 1
			}
		}
		return m, nil

	case "up", "k":
		if m.activePane == PaneOriginal {
			if m.scrollLeft > 0 {
				m.scrollLeft--
			}
		} else {
			if m.scrollRight > 0 {
				m.scrollRight--
			}
		}
		return m, nil

	case "down", "j":
		if m.activePane == PaneOriginal {
			m.scrollLeft++
		} else {
			m.scrollRight++
		}
		return m, nil

	case "q", "esc":
		return m, tea.Quit
	}
	return m, nil
}

// View satisfies tea.Model.
func (m PromptEditorModel) View() tea.View {
	return tea.NewView(m.renderAt(m.width, m.height))
}

// renderAt builds the full view at the given dimensions.
func (m PromptEditorModel) renderAt(width, height int) string {
	if width < 20 {
		width = 80
	}
	if height < 10 {
		height = 24
	}

	var sb strings.Builder

	// Title bar
	title := styles.TitleStyle.Render("Prompt A/B Editor")
	provLabel := m.renderProviderSelector()
	titleLine := lipgloss.JoinHorizontal(lipgloss.Top, title, "  ", provLabel)
	sb.WriteString(titleLine)
	sb.WriteString("\n\n")

	// Score bar
	if m.score != nil {
		sb.WriteString(m.renderScoreBar())
		sb.WriteString("\n")
	}

	// Side-by-side panes
	paneWidth := max(
		// 3 chars for divider
		(width-3)/2, 10)
	paneHeight := max(
		// reserve for title, score, help
		height-8, 3)

	leftPane := m.renderPane("Original", m.original, PaneOriginal, paneWidth, paneHeight, m.scrollLeft)
	rightPane := m.renderPane("Enhanced", m.enhanced, PaneEnhanced, paneWidth, paneHeight, m.scrollRight)

	divider := lipgloss.NewStyle().Foreground(styles.ColorDarkGray).Render(strings.Repeat(" | \n", paneHeight+2))
	// Use lipgloss to join horizontally
	joined := lipgloss.JoinHorizontal(lipgloss.Top, leftPane, " ", rightPane)
	_ = divider // divider is conceptual; join handles alignment
	sb.WriteString(joined)
	sb.WriteString("\n")

	// Help line
	sb.WriteString(m.renderHelp())

	return sb.String()
}

// renderPane renders a single prompt pane with border and scroll.
func (m PromptEditorModel) renderPane(label, content string, pane PromptEditorPane, width, height, scroll int) string {
	isActive := m.activePane == pane

	// Header
	headerStyle := styles.HeaderStyle
	if isActive {
		headerStyle = styles.SelectedStyle
	}
	header := headerStyle.Render(fmt.Sprintf(" %s ", label))

	// Content lines with scroll
	lines := strings.Split(content, "\n")
	if content == "" {
		lines = []string{"(empty)"}
	}

	// Clamp scroll
	maxScroll := max(len(lines)-height, 0)
	if scroll > maxScroll {
		scroll = maxScroll
	}

	visible := lines
	if scroll < len(lines) {
		visible = lines[scroll:]
	}
	if len(visible) > height {
		visible = visible[:height]
	}

	// Wrap/truncate lines to pane width
	contentWidth := max(
		// border padding
		width-2, 1)
	var rendered []string
	for _, line := range visible {
		if len(line) > contentWidth {
			line = line[:contentWidth]
		}
		rendered = append(rendered, line)
	}
	// Pad to fill height
	for len(rendered) < height {
		rendered = append(rendered, "")
	}

	body := strings.Join(rendered, "\n")

	// Box style
	borderColor := styles.ColorDarkGray
	if isActive {
		borderColor = styles.ColorPrimary
	}
	boxStyle := lipgloss.NewStyle().
		Width(width).
		Height(height).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 1)

	return header + "\n" + boxStyle.Render(body)
}

// renderScoreBar renders the quality score summary.
func (m PromptEditorModel) renderScoreBar() string {
	if m.score == nil {
		return ""
	}

	gradeStyle := gradeColor(m.score.Grade)
	overall := fmt.Sprintf("Quality: %s (%d/100)", gradeStyle.Render(m.score.Grade), m.score.Overall)

	if len(m.score.Dimensions) == 0 {
		return overall
	}

	var dims []string
	for _, d := range m.score.Dimensions {
		ds := gradeColor(d.Grade)
		dims = append(dims, fmt.Sprintf("%s:%s", d.Name, ds.Render(d.Grade)))
	}
	return overall + "  " + styles.InfoStyle.Render(strings.Join(dims, " "))
}

// renderProviderSelector renders the provider selector.
func (m PromptEditorModel) renderProviderSelector() string {
	var parts []string
	for i, p := range m.providers {
		s := providerStyle(p)
		label := p
		if i == m.providerIdx {
			label = fmt.Sprintf("[%s]", p)
			s = s.Bold(true)
		}
		parts = append(parts, s.Render(label))
	}
	return strings.Join(parts, " ")
}

// renderHelp renders the help bar.
func (m PromptEditorModel) renderHelp() string {
	return styles.HelpStyle.Render("tab:switch pane  enter:accept  p/P:provider  j/k:scroll  esc:quit")
}

// gradeColor returns a style colored by letter grade.
func gradeColor(grade string) lipgloss.Style {
	switch grade {
	case "A":
		return lipgloss.NewStyle().Foreground(styles.ColorGreen)
	case "B":
		return lipgloss.NewStyle().Foreground(styles.ColorPrimary)
	case "C":
		return lipgloss.NewStyle().Foreground(styles.ColorYellow)
	case "D":
		return lipgloss.NewStyle().Foreground(lipgloss.Color("208"))
	default:
		return lipgloss.NewStyle().Foreground(styles.ColorRed)
	}
}

// providerStyle returns the appropriate style for a provider name.
func providerStyle(provider string) lipgloss.Style {
	switch provider {
	case "claude":
		return styles.ProviderClaudeStyle
	case "gemini":
		return styles.ProviderGeminiStyle
	case "codex", "openai":
		return styles.ProviderCodexStyle
	default:
		return styles.InfoStyle
	}
}
