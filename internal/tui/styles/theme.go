package styles

import (
	"os"

	"github.com/charmbracelet/lipgloss"
	"gopkg.in/yaml.v3"
)

// Theme defines a color scheme for the TUI.
type Theme struct {
	Name     string `yaml:"name"`
	Primary  string `yaml:"primary"`
	Accent   string `yaml:"accent"`
	Green    string `yaml:"green"`
	Yellow   string `yaml:"yellow"`
	Red      string `yaml:"red"`
	Gray     string `yaml:"gray"`
	DarkBg   string `yaml:"dark_bg"`
	BrightFg string `yaml:"bright_fg"`
}

// DefaultThemes returns the built-in theme collection.
func DefaultThemes() map[string]*Theme {
	return map[string]*Theme{
		"k9s": {
			Name:     "k9s",
			Primary:  "39",
			Accent:   "205",
			Green:    "42",
			Yellow:   "214",
			Red:      "196",
			Gray:     "244",
			DarkBg:   "236",
			BrightFg: "255",
		},
		"dracula": {
			Name:     "dracula",
			Primary:  "141",
			Accent:   "212",
			Green:    "84",
			Yellow:   "228",
			Red:      "210",
			Gray:     "103",
			DarkBg:   "236",
			BrightFg: "253",
		},
		"gruvbox": {
			Name:     "gruvbox",
			Primary:  "109",
			Accent:   "175",
			Green:    "142",
			Yellow:   "214",
			Red:      "167",
			Gray:     "245",
			DarkBg:   "235",
			BrightFg: "223",
		},
		"nord": {
			Name:     "nord",
			Primary:  "110",
			Accent:   "139",
			Green:    "108",
			Yellow:   "222",
			Red:      "174",
			Gray:     "103",
			DarkBg:   "236",
			BrightFg: "189",
		},
	}
}

// LoadTheme reads a theme from a YAML file.
func LoadTheme(path string) (*Theme, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var t Theme
	if err := yaml.Unmarshal(data, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// ApplyTheme updates all package-level style variables to use the given theme.
func ApplyTheme(t *Theme) {
	if t == nil {
		return
	}

	ColorPrimary = lipgloss.Color(t.Primary)
	ColorAccent = lipgloss.Color(t.Accent)
	ColorGreen = lipgloss.Color(t.Green)
	ColorYellow = lipgloss.Color(t.Yellow)
	ColorRed = lipgloss.Color(t.Red)
	ColorGray = lipgloss.Color(t.Gray)
	ColorDarkBg = lipgloss.Color(t.DarkBg)
	ColorBrightWhite = lipgloss.Color(t.BrightFg)

	// Rebuild styles with new colors
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorAccent)
	HeaderStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorPrimary)
	SelectedStyle = lipgloss.NewStyle().Bold(true).Foreground(ColorBrightWhite).Background(ColorDarkBg)

	StatusRunning = lipgloss.NewStyle().Foreground(ColorGreen)
	StatusCompleted = lipgloss.NewStyle().Foreground(ColorPrimary)
	StatusFailed = lipgloss.NewStyle().Foreground(ColorRed)
	StatusIdle = lipgloss.NewStyle().Foreground(ColorGray)

	CircuitClosed = lipgloss.NewStyle().Foreground(ColorGreen)
	CircuitHalfOpen = lipgloss.NewStyle().Foreground(ColorYellow)
	CircuitOpen = lipgloss.NewStyle().Foreground(ColorRed)

	HelpStyle = lipgloss.NewStyle().Foreground(ColorDarkGray)
	InfoStyle = lipgloss.NewStyle().Foreground(ColorGray)
	WarningStyle = lipgloss.NewStyle().Foreground(ColorYellow)

	TabActive = lipgloss.NewStyle().Bold(true).Foreground(ColorBrightWhite).Background(ColorDarkBg).Padding(0, 1)
	TabInactive = lipgloss.NewStyle().Foreground(ColorGray).Padding(0, 1)

	AlertCritical = lipgloss.NewStyle().Foreground(ColorRed).Bold(true)
	AlertWarning = lipgloss.NewStyle().Foreground(ColorYellow)
	AlertInfo = lipgloss.NewStyle().Foreground(ColorGray)

	ProviderClaudeStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
	ProviderGeminiStyle = lipgloss.NewStyle().Foreground(ColorAccent)
	ProviderCodexStyle = lipgloss.NewStyle().Foreground(ColorYellow)

	StatBox = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(ColorDarkGray).Padding(0, 1)
}
