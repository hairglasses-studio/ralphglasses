package fontkit

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// GhosttyOpts configures Ghostty font settings.
type GhosttyOpts struct {
	FontSize int // Font size in points (default 15)
}

func (o *GhosttyOpts) fontSize() int {
	if o.FontSize > 0 {
		return o.FontSize
	}
	return 15
}

// ConfigureGhostty writes font configuration to the Ghostty config file.
// Preserves existing non-font config lines.
func ConfigureGhostty(opts GhosttyOpts) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(home, ".config", "ghostty")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("create ghostty config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "config")
	fontSize := opts.fontSize()

	// Font directives to write
	fontLines := map[string]string{
		"font-family":        "MonaspiceNeNFM",
		"font-family-bold":   "MonaspiceNeNFM-Bold",
		"font-family-italic": "MonaspiceNeNFM-Italic",
		"font-size":          fmt.Sprintf("%d", fontSize),
	}

	// Read existing config, filter out font lines
	var preserved []string
	if f, err := os.Open(configPath); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()
			key := strings.SplitN(strings.TrimSpace(line), "=", 2)[0]
			key = strings.TrimSpace(key)
			if _, isFont := fontLines[key]; !isFont {
				preserved = append(preserved, line)
			}
		}
		f.Close()
	}

	// Append font config
	preserved = append(preserved, "")
	preserved = append(preserved, "# claudekit: Monaspice Neon Nerd Font")
	for k, v := range fontLines {
		preserved = append(preserved, fmt.Sprintf("%s = %s", k, v))
	}
	preserved = append(preserved, "")

	if err := os.WriteFile(configPath, []byte(strings.Join(preserved, "\n")), 0o644); err != nil {
		return "", fmt.Errorf("write ghostty config: %w", err)
	}

	return configPath, nil
}
