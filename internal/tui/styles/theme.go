package styles

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"
)

// ThemeMu serializes access to package-level style variables.
// ApplyTheme holds this during writes; callers reading globals from
// concurrent goroutines should RLock.
var ThemeMu sync.RWMutex

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
			Primary:  "#00afff",
			Accent:   "#ff5faf",
			Green:    "#00d787",
			Yellow:   "#ffaf00",
			Red:      "#ff0000",
			Gray:     "#808080",
			DarkBg:   "#303030",
			BrightFg: "#eeeeee",
		},
		"dracula": {
			Name:     "dracula",
			Primary:  "#bd93f9",
			Accent:   "#ff79c6",
			Green:    "#50fa7b",
			Yellow:   "#f1fa8c",
			Red:      "#ff8787",
			Gray:     "#8787af",
			DarkBg:   "#303030",
			BrightFg: "#dadada",
		},
		"gruvbox": {
			Name:     "gruvbox",
			Primary:  "#83a598",
			Accent:   "#d3869b",
			Green:    "#b8bb26",
			Yellow:   "#fabd2f",
			Red:      "#fb4934",
			Gray:     "#8a8a8a",
			DarkBg:   "#262626",
			BrightFg: "#ebdbb2",
		},
		"nord": {
			Name:     "nord",
			Primary:  "#88c0d0",
			Accent:   "#b48ead",
			Green:    "#a3be8c",
			Yellow:   "#ebcb8b",
			Red:      "#bf616a",
			Gray:     "#8787af",
			DarkBg:   "#303030",
			BrightFg: "#d7d7ff",
		},
		"catppuccin-macchiato": {
			Name:     "catppuccin-macchiato",
			Primary:  "#8aadf4",
			Accent:   "#f5bde6",
			Green:    "#a6da95",
			Yellow:   "#eed49f",
			Red:      "#ed8796",
			Gray:     "#8087a2",
			DarkBg:   "#24273a",
			BrightFg: "#cad3f5",
		},
		"catppuccin-mocha": {
			Name:     "catppuccin-mocha",
			Primary:  "#89b4fa",
			Accent:   "#f5c2e7",
			Green:    "#a6e3a1",
			Yellow:   "#f9e2af",
			Red:      "#f38ba8",
			Gray:     "#7f849c",
			DarkBg:   "#1e1e2e",
			BrightFg: "#cdd6f4",
		},
		"tokyo-night": {
			Name:     "tokyo-night",
			Primary:  "#7aa2f7",
			Accent:   "#bb9af7",
			Green:    "#9ece6a",
			Yellow:   "#e0af68",
			Red:      "#f7768e",
			Gray:     "#565f89",
			DarkBg:   "#1a1b26",
			BrightFg: "#c0caf5",
		},
		"snazzy": {
			Name:     "snazzy",
			Primary:  "#57c7ff",
			Accent:   "#ff6ac1",
			Green:    "#5af78e",
			Yellow:   "#f3f99d",
			Red:      "#ff5c57",
			Gray:     "#686868",
			DarkBg:   "#1a1a1a",
			BrightFg: "#f1f1f0",
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

// ThemeDir returns the default external theme directory:
// ~/.config/ralphglasses/themes/
func ThemeDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "ralphglasses", "themes")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "ralphglasses", "themes")
}

// LoadExternalThemes scans the external theme directory for .yaml/.yml files
// and returns them keyed by name (filename without extension).
func LoadExternalThemes(dir string) map[string]*Theme {
	themes := make(map[string]*Theme)
	if dir == "" {
		return themes
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return themes
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		t, err := LoadTheme(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if t.Name == "" {
			t.Name = name
		}
		themes[name] = t
	}
	return themes
}

// ResolveTheme looks up a theme by name: built-in themes first, then external
// themes from ThemeDir(), then tries loading the name as a file path.
func ResolveTheme(name string) *Theme {
	// Built-in themes.
	if t := DefaultThemes()[name]; t != nil {
		return t
	}
	// External theme directory.
	if dir := ThemeDir(); dir != "" {
		if ext := LoadExternalThemes(dir); ext[name] != nil {
			return ext[name]
		}
	}
	// Direct file path.
	if t, err := LoadTheme(name); err == nil {
		return t
	}
	return nil
}

// ApplyTheme updates all package-level style variables to use the given theme.
func ApplyTheme(t *Theme) {
	if t == nil {
		return
	}

	ThemeMu.Lock()
	defer ThemeMu.Unlock()

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
