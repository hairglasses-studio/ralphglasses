package model

import (
	"bufio"
	"context"
	"encoding/json"
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
	MinInt      int     // for ConfigTypeInt
	MaxInt      int     // for ConfigTypeInt (0 means no upper bound)
	MinFloat    float64 // for ConfigTypeFloat
	MaxFloat    float64 // for ConfigTypeFloat (0 means no upper bound)
}

// KnownKeys is the canonical registry of valid .ralphrc config keys.
var KnownKeys = map[string]ConfigKeySpec{
	"PROJECT_NAME":                 {Type: ConfigTypeString, Description: "project display name"},
	"MODEL":                        {Type: ConfigTypeString, Default: "gpt-5.4", Description: "LLM model to use"},
	"MAX_CALLS_PER_HOUR":           {Type: ConfigTypeInt, Default: "120", MinInt: 1, MaxInt: 1000, Description: "rate limit per hour"},
	"CLAUDE_TIMEOUT_MINUTES":       {Type: ConfigTypeInt, Default: "30", MinInt: 1, MaxInt: 480, Description: "session timeout in minutes"},
	"BUDGET":                       {Type: ConfigTypeFloat, Default: "5.00", MinFloat: 0.01, MaxFloat: 1000, Description: "session budget in USD"},
	"RALPH_SESSION_BUDGET":         {Type: ConfigTypeFloat, Default: "100.00", MinFloat: 0.01, MaxFloat: 10000, Description: "marathon budget in USD"},
	"MARATHON_DURATION_HOURS":      {Type: ConfigTypeInt, Default: "12", MinInt: 1, MaxInt: 168, Description: "marathon duration limit"},
	"CHECKPOINT_HOURS":             {Type: ConfigTypeInt, Default: "3", MinInt: 1, MaxInt: 24, Description: "checkpoint interval in hours"},
	"CB_FAIL_THRESHOLD":            {Type: ConfigTypeInt, Default: "5", MinInt: 1, MaxInt: 100, Description: "circuit breaker failure threshold"},
	"CB_RESET_TIMEOUT":             {Type: ConfigTypeInt, Default: "300", MinInt: 10, MaxInt: 3600, Description: "circuit breaker reset timeout in seconds"},
	"CB_HALF_OPEN_MAX":             {Type: ConfigTypeInt, Default: "2", MinInt: 1, MaxInt: 20, Description: "circuit breaker half-open max attempts"},
	"SPEC_FILE":                    {Type: ConfigTypeString, Default: "spec.md", Description: "spec file path"},
	"PLAN_FILE":                    {Type: ConfigTypeString, Default: "plan.md", Description: "plan file path"},
	"FIX_PLAN_FILE":                {Type: ConfigTypeString, Default: "fix_plan.md", Description: "fix plan file path"},
	"PROVIDER":                     {Type: ConfigTypeString, Default: "codex", Description: "default session provider (claude/gemini/codex)"},
	"AUTO_ENHANCE":                 {Type: ConfigTypeBool, Default: "false", Description: "auto-enhance prompts before session launch"},
	"AUTONOMY_LEVEL":               {Type: ConfigTypeInt, Default: "0", MinInt: 0, MaxInt: 3, Description: "autonomy level (0=observe, 3=full)"},
	"AUTONOMY_AUTO_RECOVER":        {Type: ConfigTypeBool, Default: "false", Description: "enable auto-recovery for failed loops"},
	"AUTONOMY_AUTO_RECOVER_MAX":    {Type: ConfigTypeInt, Default: "3", MinInt: 1, MaxInt: 20, Description: "max auto-recoveries per loop"},
	"CASCADE_ENABLED":              {Type: ConfigTypeBool, Default: "true", Description: "enable cheap-then-expensive cascade routing"},
	"CASCADE_CHEAP_PROVIDER":       {Type: ConfigTypeString, Default: "gemini", Description: "cheap provider for cascade routing"},
	"CASCADE_CHEAP_MODEL":          {Type: ConfigTypeString, Default: "gemini-2.5-flash", Description: "cheap model for cascade routing"},
	"CASCADE_EXPENSIVE_PROVIDER":   {Type: ConfigTypeString, Default: "codex", Description: "expensive provider for cascade routing"},
	"CASCADE_EXPENSIVE_MODEL":      {Type: ConfigTypeString, Default: "gpt-5.4", Description: "expensive model for cascade routing"},
	"CASCADE_CONFIDENCE_THRESHOLD": {Type: ConfigTypeFloat, Default: "0.7", MinFloat: 0, MaxFloat: 1, Description: "confidence threshold for cascade escalation"},
	"CASCADE_MAX_CHEAP_BUDGET":     {Type: ConfigTypeFloat, Default: "2.00", MinFloat: 0.01, MaxFloat: 100, Description: "max budget for cheap cascade tier in USD"},
	"CASCADE_DIFFICULTY_THRESHOLD": {Type: ConfigTypeFloat, Default: "0.4", MinFloat: 0, MaxFloat: 1, Description: "difficulty threshold for bypassing cheap tier"},
	"KILL_ESCALATION_TIMEOUT":      {Type: ConfigTypeInt, Default: "5", MinInt: 1, MaxInt: 60, Description: "kill escalation timeout in seconds"},
}

// DeprecatedKeys maps deprecated config key names to migration hints.
var DeprecatedKeys = map[string]string{
	"CLAUDE_MODEL":    "Use PLANNER_MODEL instead",
	"GEMINI_MODEL":    "Use WORKER_MODEL instead",
	"MAX_RETRIES":     "Use RETRY_LIMIT instead",
	"TIMEOUT_SECONDS": "Use LOOP_TIMEOUT instead",
}

// deprecatedKeyMapping maps deprecated keys to their canonical replacements.
// Extracted from the hint strings in DeprecatedKeys.
var deprecatedKeyMapping = map[string]string{
	"CLAUDE_MODEL":    "PLANNER_MODEL",
	"GEMINI_MODEL":    "WORKER_MODEL",
	"MAX_RETRIES":     "RETRY_LIMIT",
	"TIMEOUT_SECONDS": "LOOP_TIMEOUT",
}

// ConfigWarning describes a non-fatal issue with a config value.
type ConfigWarning struct {
	Key     string
	Message string
}

// ConfigChange describes a single difference between two configs.
type ConfigChange struct {
	Key    string `json:"key"`
	Type   string `json:"type"` // "added", "removed", "changed"
	OldVal string `json:"old_value,omitempty"`
	NewVal string `json:"new_value,omitempty"`
}

// ValidateConfig checks config values against the known key registry.
// Returns warnings for unknown/deprecated keys and validation errors for type/range mismatches.
func ValidateConfig(cfg *RalphConfig) (warnings []ConfigWarning, errors []error) {
	if cfg == nil {
		return nil, nil
	}
	for key, val := range cfg.Values {
		// Check deprecated keys first — they produce warnings, not errors.
		if hint, deprecated := DeprecatedKeys[key]; deprecated {
			warnings = append(warnings, ConfigWarning{
				Key:     key,
				Message: fmt.Sprintf("deprecated config key: %s", hint),
			})
			// Still validate the value if it also appears in KnownKeys.
		}
		spec, known := KnownKeys[key]
		if !known {
			// If it was already flagged as deprecated, skip the "unknown" warning.
			if _, deprecated := DeprecatedKeys[key]; !deprecated {
				warnings = append(warnings, ConfigWarning{Key: key, Message: "unknown config key"})
			}
			continue
		}
		if err := validateValue(key, val, spec); err != nil {
			errors = append(errors, err)
		}
	}
	return warnings, errors
}

// ConfigDiff computes the differences between two configs.
// Either old or new may be nil (treated as empty).
func ConfigDiff(old, new *RalphConfig) []ConfigChange {
	oldVals := map[string]string{}
	newVals := map[string]string{}
	if old != nil {
		oldVals = old.Values
	}
	if new != nil {
		newVals = new.Values
	}

	var changes []ConfigChange

	// Check for removed and changed keys.
	for k, ov := range oldVals {
		nv, exists := newVals[k]
		if !exists {
			changes = append(changes, ConfigChange{Key: k, Type: "removed", OldVal: ov})
		} else if ov != nv {
			changes = append(changes, ConfigChange{Key: k, Type: "changed", OldVal: ov, NewVal: nv})
		}
	}

	// Check for added keys.
	for k, nv := range newVals {
		if _, exists := oldVals[k]; !exists {
			changes = append(changes, ConfigChange{Key: k, Type: "added", NewVal: nv})
		}
	}

	return changes
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
func LoadConfig(ctx context.Context, repoPath string) (*RalphConfig, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
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

// MigrateConfig scans cfg for deprecated keys and copies their values to the
// canonical replacement key (if the new key is not already set). It removes
// the deprecated key after migration. Returns the count of migrated keys and
// any warnings produced during migration.
func MigrateConfig(cfg *RalphConfig) (migrated int, warnings []string) {
	if cfg == nil {
		return 0, nil
	}
	for oldKey, val := range cfg.Values {
		newKey, ok := deprecatedKeyMapping[oldKey]
		if !ok {
			continue
		}
		if _, exists := cfg.Values[newKey]; exists {
			warnings = append(warnings, fmt.Sprintf(
				"deprecated key %s not migrated: canonical key %s already set", oldKey, newKey))
			continue
		}
		cfg.Values[newKey] = val
		delete(cfg.Values, oldKey)
		migrated++
		warnings = append(warnings, fmt.Sprintf("migrated %s -> %s", oldKey, newKey))
	}
	return migrated, warnings
}

// configExport is the JSON-serializable representation of a RalphConfig.
type configExport struct {
	Path   string            `json:"path,omitempty"`
	Values map[string]string `json:"values"`
}

// ExportConfig serializes a RalphConfig to JSON.
func ExportConfig(cfg *RalphConfig) ([]byte, error) {
	if cfg == nil {
		return nil, fmt.Errorf("cannot export nil config")
	}
	vals := cfg.Values
	if vals == nil {
		vals = map[string]string{}
	}
	return json.MarshalIndent(configExport{
		Path:   cfg.Path,
		Values: vals,
	}, "", "  ")
}

// ImportConfig deserializes a RalphConfig from JSON and validates it.
func ImportConfig(data []byte) (*RalphConfig, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty config data")
	}
	var exp configExport
	if err := json.Unmarshal(data, &exp); err != nil {
		return nil, fmt.Errorf("invalid config JSON: %w", err)
	}
	if exp.Values == nil {
		return nil, fmt.Errorf("config missing values field")
	}
	cfg := &RalphConfig{
		Path:   exp.Path,
		Values: exp.Values,
	}
	// Validate imported config.
	_, errs := ValidateConfig(cfg)
	if len(errs) > 0 {
		return nil, fmt.Errorf("config validation failed: %w", errs[0])
	}
	return cfg, nil
}
