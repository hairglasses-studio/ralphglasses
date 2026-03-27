package model

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var validKey = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// ConfigKeyType describes the expected value type for a known config key.
type ConfigKeyType int

const (
	ConfigTypeString ConfigKeyType = iota
	ConfigTypeInt
	ConfigTypeFloat
	ConfigTypeBool
)

// ConfigKeySpec describes a known .ralphrc config key.
type ConfigKeySpec struct {
	Type        ConfigKeyType
	Default     string
	Description string
	MinInt      int    // for ConfigTypeInt
	MaxInt      int    // for ConfigTypeInt (0 means no upper bound)
	MinFloat    float64 // for ConfigTypeFloat
	MaxFloat    float64 // for ConfigTypeFloat (0 means no upper bound)
}

// KnownKeys is the canonical registry of valid .ralphrc config keys.
var KnownKeys = map[string]ConfigKeySpec{
	"PROJECT_NAME":          {Type: ConfigTypeString, Description: "project display name"},
	"MODEL":                 {Type: ConfigTypeString, Default: "sonnet", Description: "LLM model to use"},
	"MAX_CALLS_PER_HOUR":   {Type: ConfigTypeInt, Default: "120", MinInt: 1, MaxInt: 1000, Description: "rate limit per hour"},
	"CLAUDE_TIMEOUT_MINUTES": {Type: ConfigTypeInt, Default: "30", MinInt: 1, MaxInt: 480, Description: "session timeout in minutes"},
	"BUDGET":                {Type: ConfigTypeFloat, Default: "5.00", MinFloat: 0.01, MaxFloat: 1000, Description: "session budget in USD"},
	"RALPH_SESSION_BUDGET":  {Type: ConfigTypeFloat, Default: "100.00", MinFloat: 0.01, MaxFloat: 10000, Description: "marathon budget in USD"},
	"MARATHON_DURATION_HOURS": {Type: ConfigTypeInt, Default: "12", MinInt: 1, MaxInt: 168, Description: "marathon duration limit"},
	"CHECKPOINT_HOURS":      {Type: ConfigTypeInt, Default: "3", MinInt: 1, MaxInt: 24, Description: "checkpoint interval in hours"},
	"CB_FAIL_THRESHOLD":     {Type: ConfigTypeInt, Default: "5", MinInt: 1, MaxInt: 100, Description: "circuit breaker failure threshold"},
	"CB_RESET_TIMEOUT":      {Type: ConfigTypeInt, Default: "300", MinInt: 10, MaxInt: 3600, Description: "circuit breaker reset timeout in seconds"},
	"CB_HALF_OPEN_MAX":      {Type: ConfigTypeInt, Default: "2", MinInt: 1, MaxInt: 20, Description: "circuit breaker half-open max attempts"},
	"SPEC_FILE":             {Type: ConfigTypeString, Default: "spec.md", Description: "spec file path"},
	"PLAN_FILE":             {Type: ConfigTypeString, Default: "plan.md", Description: "plan file path"},
	"FIX_PLAN_FILE":         {Type: ConfigTypeString, Default: "fix_plan.md", Description: "fix plan file path"},
	"PROVIDER":              {Type: ConfigTypeString, Default: "claude", Description: "default session provider (claude/gemini/codex)"},
	"AUTO_ENHANCE":          {Type: ConfigTypeBool, Default: "false", Description: "auto-enhance prompts before session launch"},
	"AUTONOMY_LEVEL":              {Type: ConfigTypeInt, Default: "0", MinInt: 0, MaxInt: 3, Description: "autonomy level (0=observe, 3=full)"},
	"AUTONOMY_AUTO_RECOVER":       {Type: ConfigTypeBool, Default: "false", Description: "enable auto-recovery for failed loops"},
	"AUTONOMY_AUTO_RECOVER_MAX":   {Type: ConfigTypeInt, Default: "3", MinInt: 1, MaxInt: 20, Description: "max auto-recoveries per loop"},
	"CASCADE_ENABLED":              {Type: ConfigTypeBool, Default: "false", Description: "enable cheap-then-expensive cascade routing"},
	"CASCADE_CHEAP_PROVIDER":       {Type: ConfigTypeString, Default: "gemini", Description: "cheap provider for cascade routing"},
	"CASCADE_CHEAP_MODEL":          {Type: ConfigTypeString, Default: "gemini-2.5-flash", Description: "cheap model for cascade routing"},
	"CASCADE_EXPENSIVE_PROVIDER":   {Type: ConfigTypeString, Default: "claude", Description: "expensive provider for cascade routing"},
	"CASCADE_EXPENSIVE_MODEL":      {Type: ConfigTypeString, Default: "claude-sonnet-4-6", Description: "expensive model for cascade routing"},
	"CASCADE_CONFIDENCE_THRESHOLD": {Type: ConfigTypeFloat, Default: "0.7", MinFloat: 0, MaxFloat: 1, Description: "confidence threshold for cascade escalation"},
	"CASCADE_MAX_CHEAP_BUDGET":     {Type: ConfigTypeFloat, Default: "2.00", MinFloat: 0.01, MaxFloat: 100, Description: "max budget for cheap cascade tier in USD"},
	"CASCADE_DIFFICULTY_THRESHOLD": {Type: ConfigTypeFloat, Default: "0.4", MinFloat: 0, MaxFloat: 1, Description: "difficulty threshold for bypassing cheap tier"},
	"KILL_ESCALATION_TIMEOUT":      {Type: ConfigTypeInt, Default: "5", MinInt: 1, MaxInt: 60, Description: "kill escalation timeout in seconds"},
}

// ConfigWarning describes a non-fatal issue with a config value.
type ConfigWarning struct {
	Key     string
	Message string
}

// ValidateConfig checks config values against the known key registry.
// Returns warnings for unknown keys and validation errors for type/range mismatches.
func ValidateConfig(cfg *RalphConfig) (warnings []ConfigWarning, errors []error) {
	if cfg == nil {
		return nil, nil
	}
	for key, val := range cfg.Values {
		spec, known := KnownKeys[key]
		if !known {
			warnings = append(warnings, ConfigWarning{Key: key, Message: "unknown config key"})
			continue
		}
		if err := validateValue(key, val, spec); err != nil {
			errors = append(errors, err)
		}
	}
	return warnings, errors
}

func validateValue(key, val string, spec ConfigKeySpec) error {
	switch spec.Type {
	case ConfigTypeInt:
		n, err := strconv.Atoi(val)
		if err != nil {
			return fmt.Errorf("%s: expected integer, got %q", key, val)
		}
		if spec.MinInt != 0 || spec.MaxInt != 0 {
			if n < spec.MinInt {
				return fmt.Errorf("%s: value %d below minimum %d", key, n, spec.MinInt)
			}
			if spec.MaxInt > 0 && n > spec.MaxInt {
				return fmt.Errorf("%s: value %d exceeds maximum %d", key, n, spec.MaxInt)
			}
		}
	case ConfigTypeFloat:
		f, err := strconv.ParseFloat(val, 64)
		if err != nil {
			return fmt.Errorf("%s: expected float, got %q", key, val)
		}
		if spec.MinFloat != 0 || spec.MaxFloat != 0 {
			if f < spec.MinFloat {
				return fmt.Errorf("%s: value %g below minimum %g", key, f, spec.MinFloat)
			}
			if spec.MaxFloat > 0 && f > spec.MaxFloat {
				return fmt.Errorf("%s: value %g exceeds maximum %g", key, f, spec.MaxFloat)
			}
		}
	case ConfigTypeBool:
		switch strings.ToLower(val) {
		case "true", "false", "1", "0", "yes", "no":
			// valid
		default:
			return fmt.Errorf("%s: expected boolean, got %q", key, val)
		}
	}
	return nil
}

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
