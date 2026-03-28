package fontkit

import (
	"fmt"
	"os"
	"path/filepath"
)

// WezTermOpts configures WezTerm font settings.
type WezTermOpts struct {
	FontSize int // Font size in points (default 15)
}

func (o *WezTermOpts) fontSize() int {
	if o.FontSize > 0 {
		return o.FontSize
	}
	return 15
}

// ConfigureWezTerm writes a Lua module with font configuration for WezTerm.
// User includes: require("claudekit").apply(config, wezterm) in their wezterm.lua
func ConfigureWezTerm(opts WezTermOpts) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	configDir := filepath.Join(home, ".config", "wezterm")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return "", fmt.Errorf("create wezterm config dir: %w", err)
	}

	configPath := filepath.Join(configDir, "claudekit.lua")
	fontSize := opts.fontSize()

	content := fmt.Sprintf(`-- claudekit: Monaspice Neon Nerd Font configuration
local M = {}

function M.apply(config, wezterm)
  config.font = wezterm.font("MonaspiceNeNFM")
  config.font_size = %.1f
end

return M
`, float64(fontSize))

	if err := os.WriteFile(configPath, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("write wezterm font config: %w", err)
	}

	return configPath, nil
}
