package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// Config holds all runmylife configuration.
type Config struct {
	DBPath      string            `json:"db_path"`
	RalphDBPath string            `json:"ralph_db_path,omitempty"` // optional ralphglasses fleet DB
	Credentials map[string]string `json:"credentials"`             // service -> API token
	Location    *Location         `json:"location,omitempty"`
}

// Location holds user location for weather and timezone.
type Location struct {
	City      string  `json:"city"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timezone  string  `json:"timezone"`
}

var (
	cached    *Config
	cachedErr error
	once      sync.Once
	mu        sync.Mutex
)

// DefaultDir returns ~/.config/runmylife
func DefaultDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "runmylife")
	}
	return filepath.Join(home, ".config", "runmylife")
}

// DefaultConfigPath returns ~/.config/runmylife/config.json
func DefaultConfigPath() string {
	return filepath.Join(DefaultDir(), "config.json")
}

func loadFromDisk() (*Config, error) {
	path := DefaultConfigPath()
	cfg := &Config{
		DBPath:      filepath.Join(DefaultDir(), "runmylife.db"),
		Credentials: make(map[string]string),
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Load reads config from disk on first call, returns cached copy thereafter.
func Load() (*Config, error) {
	mu.Lock()
	defer mu.Unlock()
	once.Do(func() {
		cached, cachedErr = loadFromDisk()
	})
	return cached, cachedErr
}

// InvalidateCache resets the cached config so the next Load() re-reads from disk.
func InvalidateCache() {
	mu.Lock()
	defer mu.Unlock()
	once = sync.Once{}
	cached = nil
	cachedErr = nil
}

// Validate checks the config for common issues and returns warnings.
func Validate(c *Config) []string {
	var warnings []string
	if c.Location == nil {
		warnings = append(warnings, "location not configured — weather features will not work")
	}
	if c.Credentials["todoist"] == "" {
		warnings = append(warnings, "no Todoist API token — task sync will not work")
	}
	if c.Credentials["discord"] == "" {
		warnings = append(warnings, "no Discord bot token — Discord features will not work")
	}
	if c.Credentials["notion"] == "" {
		warnings = append(warnings, "no Notion integration token — Notion features will not work")
	}
	if c.Credentials["google"] == "" {
		warnings = append(warnings, "no Google OAuth credentials file found — Gmail/Calendar/Drive sync will not work")
	}
	return warnings
}

// Save writes config to disk and invalidates the cache.
func (c *Config) Save() error {
	dir := filepath.Dir(DefaultConfigPath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(DefaultConfigPath(), data, 0600); err != nil {
		return err
	}
	InvalidateCache()
	return nil
}
