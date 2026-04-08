package firstboot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/ralphpath"
)

type Profile struct {
	Hostname string            `json:"hostname"`
	APIKeys  map[string]string `json:"api_keys,omitempty"`
	Autonomy int               `json:"autonomy_level"`
	FleetURL string            `json:"fleet_coordinator_url,omitempty"`
}

func DefaultConfigDir() string {
	return ralphpath.ConfigDir()
}

func DefaultProfile() Profile {
	return Profile{
		Hostname: "ralph-01",
		APIKeys: map[string]string{
			"anthropic": "",
			"google":    "",
			"openai":    "",
		},
		Autonomy: 0,
	}
}

func ConfigPath(configDir string) string {
	return filepath.Join(configDir, "config.json")
}

func MarkerPath(configDir string) string {
	return filepath.Join(configDir, ".firstboot-done")
}

func Load(configDir string) (Profile, bool, error) {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	profile := DefaultProfile()
	data, err := os.ReadFile(ConfigPath(configDir))
	if err != nil {
		if os.IsNotExist(err) {
			return profile, false, nil
		}
		return profile, false, err
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return profile, false, err
	}
	if profile.APIKeys == nil {
		profile.APIKeys = DefaultProfile().APIKeys
	}
	done := false
	if _, err := os.Stat(MarkerPath(configDir)); err == nil {
		done = true
	}
	return profile, done, nil
}

func Save(configDir string, profile Profile, markDone bool) error {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	if profile.APIKeys == nil {
		profile.APIKeys = DefaultProfile().APIKeys
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	data, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(ConfigPath(configDir), data, 0o644); err != nil {
		return err
	}
	if markDone {
		return MarkDone(configDir, profile.Hostname)
	}
	return nil
}

func MarkDone(configDir, hostname string) error {
	if configDir == "" {
		configDir = DefaultConfigDir()
	}
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	return os.WriteFile(MarkerPath(configDir), []byte(strings.TrimSpace(hostname)+"\n"), 0o644)
}

func Validate(profile Profile) []string {
	var issues []string
	if strings.TrimSpace(profile.Hostname) == "" {
		issues = append(issues, "hostname is required")
	}
	if profile.Autonomy < 0 || profile.Autonomy > 3 {
		issues = append(issues, "autonomy_level must be between 0 and 3")
	}
	return issues
}
