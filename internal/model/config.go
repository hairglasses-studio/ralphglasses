package model

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var validKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// RalphConfig represents parsed .ralphrc key-value pairs.
type RalphConfig struct {
	Path   string
	Values map[string]string
}

// LoadConfig reads and parses a .ralphrc file from the given repo path.
func LoadConfig(repoPath string) (*RalphConfig, error) {
	rcPath := filepath.Join(repoPath, ".ralphrc")
	f, err := os.Open(rcPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &RalphConfig{
		Path:   rcPath,
		Values: make(map[string]string),
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		cfg.Values[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"")
	}
	return cfg, scanner.Err()
}

// Get returns a config value or a default.
func (c *RalphConfig) Get(key, defaultVal string) string {
	if c == nil {
		return defaultVal
	}
	if v, ok := c.Values[key]; ok {
		return v
	}
	return defaultVal
}

// Save writes the config back to disk.
func (c *RalphConfig) Save() error {
	for key := range c.Values {
		if !validKey.MatchString(key) {
			return fmt.Errorf("invalid config key %q: must match [A-Z_][A-Z0-9_]*", key)
		}
	}

	f, err := os.Create(c.Path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for k, v := range c.Values {
		if _, err := fmt.Fprintf(w, "%s=\"%s\"\n", k, v); err != nil {
			return err
		}
	}
	return w.Flush()
}
