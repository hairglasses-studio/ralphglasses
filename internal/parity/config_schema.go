package parity

import (
	"strconv"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
)

type ConfigKeySchema struct {
	Name          string   `json:"name"`
	Type          string   `json:"type"`
	AllowedValues []string `json:"allowed_values,omitempty"`
	Min           int      `json:"min,omitempty"`
	Max           int      `json:"max,omitempty"`
	Constraints   string   `json:"constraints,omitempty"`
	HasDefault    bool     `json:"has_default"`
	DefaultValue  any      `json:"default_value,omitempty"`
}

type ConfigSchemaOptions struct {
	Key                string
	IncludeDefaults    bool
	IncludeConstraints bool
}

func ConfigSchema(opts ConfigSchemaOptions) []ConfigKeySchema {
	keys := config.KnownKeys()
	result := make([]ConfigKeySchema, 0, len(keys))
	for _, key := range keys {
		if opts.Key != "" && !strings.EqualFold(key.Name, opts.Key) {
			continue
		}
		item := ConfigKeySchema{
			Name:          key.Name,
			Type:          key.Type,
			AllowedValues: key.AllowedStr,
			Min:           key.MinInt,
			Max:           key.MaxInt,
		}
		if opts.IncludeConstraints {
			switch {
			case len(key.AllowedStr) > 0:
				item.Constraints = "enum: " + strings.Join(key.AllowedStr, ", ")
			case key.MinInt > 0 || key.MaxInt > 0:
				item.Constraints = joinNonEmpty(
					ifNonZero(key.MinInt, "min"),
					ifNonZero(key.MaxInt, "max"),
				)
			}
		}
		if opts.IncludeDefaults {
			item.HasDefault = false
		}
		result = append(result, item)
	}
	return result
}

func ifNonZero(v int, label string) string {
	if v == 0 {
		return ""
	}
	return label + "=" + strconv.Itoa(v)
}

func joinNonEmpty(parts ...string) string {
	filtered := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) != "" {
			filtered = append(filtered, part)
		}
	}
	return strings.Join(filtered, ", ")
}
