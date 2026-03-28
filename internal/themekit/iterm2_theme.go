package themekit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// ExportITerm2 generates an iTerm2 color preset from a palette and writes it
// to the DynamicProfiles directory (merged into the claudekit profile).
func ExportITerm2(p Palette) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	profileDir := filepath.Join(home, "Library", "Application Support", "iTerm2", "DynamicProfiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return "", fmt.Errorf("create DynamicProfiles dir: %w", err)
	}

	// iTerm2 uses separate R/G/B components as floats 0-1
	colorComponent := func(name string) map[string]any {
		col := p.Get(name)
		return map[string]any{
			"Red Component":   float64(col.R) / 255.0,
			"Green Component": float64(col.G) / 255.0,
			"Blue Component":  float64(col.B) / 255.0,
			"Alpha Component": 1.0,
			"Color Space":     "sRGB",
		}
	}

	profile := map[string]any{
		"Profiles": []map[string]any{
			{
				"Name":                        "Claudekit " + p.Name,
				"Guid":                        "claudekit-theme-001",
				"Dynamic Profile Parent Name": "Claudekit Monaspace",

				"Background Color":       colorComponent("base"),
				"Foreground Color":       colorComponent("text"),
				"Bold Color":             colorComponent("text"),
				"Cursor Color":           colorComponent("rosewater"),
				"Cursor Text Color":      colorComponent("base"),
				"Selection Color":        colorComponent("surface2"),
				"Selected Text Color":    colorComponent("text"),
				"Badge Color":            colorComponent("mauve"),
				"Tab Color":              colorComponent("mantle"),
				"Cursor Guide Color":     colorComponent("surface0"),
				"Link Color":             colorComponent("blue"),

				// ANSI colors: black, red, green, yellow, blue, magenta, cyan, white
				"Ansi 0 Color":  colorComponent("surface1"),
				"Ansi 1 Color":  colorComponent("red"),
				"Ansi 2 Color":  colorComponent("green"),
				"Ansi 3 Color":  colorComponent("yellow"),
				"Ansi 4 Color":  colorComponent("blue"),
				"Ansi 5 Color":  colorComponent("pink"),
				"Ansi 6 Color":  colorComponent("teal"),
				"Ansi 7 Color":  colorComponent("subtext1"),
				"Ansi 8 Color":  colorComponent("surface2"),
				"Ansi 9 Color":  colorComponent("red"),
				"Ansi 10 Color": colorComponent("green"),
				"Ansi 11 Color": colorComponent("yellow"),
				"Ansi 12 Color": colorComponent("blue"),
				"Ansi 13 Color": colorComponent("pink"),
				"Ansi 14 Color": colorComponent("teal"),
				"Ansi 15 Color": colorComponent("subtext0"),
			},
		},
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}

	outPath := filepath.Join(profileDir, "claudekit-theme.json")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write theme profile: %w", err)
	}

	return outPath, nil
}
