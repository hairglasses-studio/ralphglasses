package config

import (
	"fmt"
	"sort"
)

// ValidationWarning represents a single config validation issue.
type ValidationWarning struct {
	Key      string
	Message  string
	Severity string // "warning" or "error"
}

// String formats the warning as "[severity] key: message".
func (w ValidationWarning) String() string {
	return fmt.Sprintf("[%s] %s: %s", w.Severity, w.Key, w.Message)
}

// keySpec describes the expected type and constraints for a config key.
type keySpec struct {
	Type       string   // "int", "float", "string", "bool", "[]string", "duration"
	MinInt     int      // inclusive, for int type
	MaxInt     int      // inclusive, for int type
	AllowedStr []string // for string type, nil means any string
}

// canonicalKeys is the registry of all known .ralphrc / config keys with their
// expected types and valid ranges.
var canonicalKeys = map[string]keySpec{
	"scan_paths":            {Type: "[]string"},
	"default_provider":      {Type: "string", AllowedStr: []string{"claude", "gemini", "codex", "openai", "ollama"}},
	"worker_provider":       {Type: "string", AllowedStr: []string{"claude", "gemini", "codex", "openai", "ollama"}},
	"max_workers":           {Type: "int", MinInt: 0, MaxInt: 50},
	"default_budget_usd":    {Type: "float"},
	"cost_rate_multiplier":  {Type: "float"},
	"session_timeout":       {Type: "duration"},
	"health_check_interval": {Type: "duration"},
	"kill_timeout":          {Type: "int", MinInt: 1, MaxInt: 60},
	"max_restarts":          {Type: "int", MinInt: 0, MaxInt: 100},
	"scan_interval":         {Type: "int", MinInt: 1, MaxInt: 3600},
	"log_level":             {Type: "string", AllowedStr: []string{"debug", "info", "warn", "error"}},
	"auto_restart":          {Type: "bool"},
	"provider":              {Type: "string", AllowedStr: []string{"claude", "gemini", "codex", "openai", "ollama"}},
	"notify_desktop":        {Type: "bool"},
	"notify_sound":          {Type: "bool"},
	"telemetry_enabled":     {Type: "bool"},
}

// KeyInfo describes a config key for external consumers.
type KeyInfo struct {
	Name       string   `json:"name"`
	Type       string   `json:"type"`
	AllowedStr []string `json:"allowed_values,omitempty"`
	MinInt     int      `json:"min,omitempty"`
	MaxInt     int      `json:"max,omitempty"`
}

// KnownKeys returns a sorted list of all recognized config keys with their specs.
func KnownKeys() []KeyInfo {
	keys := make([]KeyInfo, 0, len(canonicalKeys))
	for name, spec := range canonicalKeys {
		keys = append(keys, KeyInfo{
			Name:       name,
			Type:       spec.Type,
			AllowedStr: spec.AllowedStr,
			MinInt:     spec.MinInt,
			MaxInt:     spec.MaxInt,
		})
	}
	// Sort for stable output.
	sort.Slice(keys, func(i, j int) bool { return keys[i].Name < keys[j].Name })
	return keys
}

// ValidateRawConfig validates a raw key-value config map (e.g. from a .ralphrc
// file) against the canonical key registry. It checks for unknown keys, type
// mismatches, numeric range violations, and enum membership.
func ValidateRawConfig(cfg map[string]any) []ValidationWarning {
	if cfg == nil {
		return nil
	}

	var warnings []ValidationWarning

	for key, val := range cfg {
		spec, known := canonicalKeys[key]
		if !known {
			warnings = append(warnings, ValidationWarning{
				Key:      key,
				Message:  fmt.Sprintf("unknown config key %q", key),
				Severity: "warning",
			})
			continue
		}

		warnings = append(warnings, validateValue(key, val, spec)...)
	}

	return warnings
}

// validateValue checks a single value against its key spec.
func validateValue(key string, val any, spec keySpec) []ValidationWarning {
	switch spec.Type {
	case "int":
		return validateInt(key, val, spec)
	case "float":
		return validateFloat(key, val)
	case "string":
		return validateString(key, val, spec)
	case "bool":
		return validateBool(key, val)
	case "[]string":
		return validateStringSlice(key, val)
	case "duration":
		return validateDuration(key, val)
	}
	return nil
}

func validateInt(key string, val any, spec keySpec) []ValidationWarning {
	// JSON numbers decode as float64.
	f, ok := val.(float64)
	if !ok {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected integer, got %T", val),
			Severity: "error",
		}}
	}
	n := int(f)
	if float64(n) != f {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected integer, got float %g", f),
			Severity: "error",
		}}
	}
	if n < spec.MinInt || n > spec.MaxInt {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("value %d out of range [%d, %d]", n, spec.MinInt, spec.MaxInt),
			Severity: "error",
		}}
	}
	return nil
}

func validateFloat(key string, val any) []ValidationWarning {
	if _, ok := val.(float64); !ok {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected number, got %T", val),
			Severity: "error",
		}}
	}
	return nil
}

func validateString(key string, val any, spec keySpec) []ValidationWarning {
	s, ok := val.(string)
	if !ok {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected string, got %T", val),
			Severity: "error",
		}}
	}
	if spec.AllowedStr != nil {
		for _, a := range spec.AllowedStr {
			if s == a {
				return nil
			}
		}
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("value %q not in allowed set %v", s, spec.AllowedStr),
			Severity: "error",
		}}
	}
	return nil
}

func validateBool(key string, val any) []ValidationWarning {
	if _, ok := val.(bool); !ok {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected bool, got %T", val),
			Severity: "error",
		}}
	}
	return nil
}

func validateStringSlice(key string, val any) []ValidationWarning {
	arr, ok := val.([]any)
	if !ok {
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected array, got %T", val),
			Severity: "error",
		}}
	}
	for i, elem := range arr {
		if _, ok := elem.(string); !ok {
			return []ValidationWarning{{
				Key:      key,
				Message:  fmt.Sprintf("element [%d] expected string, got %T", i, elem),
				Severity: "error",
			}}
		}
	}
	return nil
}

func validateDuration(key string, val any) []ValidationWarning {
	// Accept either a string ("10s", "5m") or a numeric value (seconds).
	switch val.(type) {
	case string:
		return nil
	case float64:
		return nil
	default:
		return []ValidationWarning{{
			Key:      key,
			Message:  fmt.Sprintf("expected duration string or number, got %T", val),
			Severity: "error",
		}}
	}
}
