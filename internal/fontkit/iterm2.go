package fontkit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

const (
	iterm2ProfileName = "Claudekit Monaspace"
	iterm2ProfileGUID = "claudekit-monaspace-001"
)

// ITerm2Opts configures iTerm2 profile generation.
type ITerm2Opts struct {
	FontSize    int    // Font size in points (default 15)
	ProfileName string // Override profile name
}

func (o *ITerm2Opts) fontSize() int {
	if o.FontSize > 0 {
		return o.FontSize
	}
	return 15
}

func (o *ITerm2Opts) profileName() string {
	if o.ProfileName != "" {
		return o.ProfileName
	}
	return iterm2ProfileName
}

// ConfigureITerm2 creates an iTerm2 Dynamic Profile with Monaspice font
// and Menlo fallback for non-ASCII characters.
func ConfigureITerm2(opts ITerm2Opts) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	profileDir := filepath.Join(home, "Library", "Application Support", "iTerm2", "DynamicProfiles")
	if err := os.MkdirAll(profileDir, 0o755); err != nil {
		return "", fmt.Errorf("create DynamicProfiles dir: %w", err)
	}

	fontSize := opts.fontSize()
	name := opts.profileName()

	// iTerm2 Dynamic Profile format
	profile := map[string]any{
		"Profiles": []map[string]any{
			{
				"Name":                          name,
				"Guid":                          iterm2ProfileGUID,
				"Dynamic Profile Parent Name":   "Default",
				"Normal Font":                   fmt.Sprintf("MonaspiceNeNFM-Regular %d", fontSize),
				"Non Ascii Font":                fmt.Sprintf("MenloRegular %d", fontSize),
				"Use Non-ASCII Font":            true,
				"ASCII Ligatures":               true,
				"Use Ligatures":                 true,
				"Horizontal Spacing":            1.0,
				"Vertical Spacing":              1.0,
				"Custom Directory":              "No",
				"Use Custom Command":            "No",
				"Terminal Type":                 "xterm-256color",
				"Unlimited Scrollback":          true,
				"Mouse Reporting":               true,
				"Unicode Normalization":          0,
				"Ambiguous Double Width":         false,
			},
		},
	}

	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return "", err
	}

	outPath := filepath.Join(profileDir, "claudekit-monaspace.json")
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write profile: %w", err)
	}

	return outPath, nil
}
