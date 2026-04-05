package timeop

import (
	"time"
)

// Preset constants for common time ranges.
const (
	Preset1h  = "1h"
	Preset4h  = "4h"
	Preset6h  = "6h"
	Preset8h  = "8h"
	Preset24h = "24h"
	Preset48h = "48h"
	Preset1w  = "1w"
	Preset7d  = "7d"
	Preset30d = "30d"
	Preset90d = "90d"
	Preset6mo = "6mo"
)

// PresetDuration maps preset strings to durations.
var PresetDuration = map[string]time.Duration{
	Preset1h:  time.Hour,
	Preset4h:  4 * time.Hour,
	Preset6h:  6 * time.Hour,
	Preset8h:  8 * time.Hour,
	Preset24h: 24 * time.Hour,
	Preset48h: 48 * time.Hour,
	Preset1w:  7 * 24 * time.Hour,
	Preset7d:  7 * 24 * time.Hour,
	Preset30d: 30 * 24 * time.Hour,
	Preset90d: 90 * 24 * time.Hour,
	Preset6mo: 180 * 24 * time.Hour,
}

// AllPresets returns all available preset strings.
func AllPresets() []string {
	return []string{Preset1h, Preset6h, Preset24h, Preset7d, Preset30d, Preset90d, Preset6mo}
}

// ShortPresets returns presets suitable for short-term analysis.
func ShortPresets() []string {
	return []string{Preset1h, Preset6h, Preset24h}
}

// SearchPresets returns presets optimized for search and investigation tools.
// Provides fine-grained short-term options (1h, 4h, 8h, 24h, 48h) plus 1 week.
func SearchPresets() []string {
	return []string{Preset1h, Preset4h, Preset8h, Preset24h, Preset48h, Preset1w}
}

// SearchPresetsString returns a comma-separated string for tool descriptions.
func SearchPresetsString() string {
	return "1h, 4h, 8h, 24h, 48h, 1w"
}

// LongPresets returns presets suitable for long-term analysis.
func LongPresets() []string {
	return []string{Preset7d, Preset30d, Preset90d, Preset6mo}
}

// UseCase represents the type of analysis being performed.
type UseCase string

const (
	// UseCaseAlerts is for checking firing alerts (default: 1h)
	UseCaseAlerts UseCase = "alerts"

	// UseCaseLogs is for searching logs (default: 1h)
	UseCaseLogs UseCase = "logs"

	// UseCaseMetrics is for metrics/observability queries (default: 24h)
	UseCaseMetrics UseCase = "metrics"

	// UseCaseIncidents is for incident analysis (default: 7d)
	UseCaseIncidents UseCase = "incidents"

	// UseCaseTickets is for ticket/issue analysis (default: 30d)
	UseCaseTickets UseCase = "tickets"

	// UseCaseAnalytics is for analytics and trends (default: 90d)
	UseCaseAnalytics UseCase = "analytics"

	// UseCaseInvestigation is for debugging/investigation (default: 24h)
	UseCaseInvestigation UseCase = "investigation"

	// UseCaseHealth is for health checks (default: 1h)
	UseCaseHealth UseCase = "health"

	// UseCaseAudit is for audit logs (default: 7d)
	UseCaseAudit UseCase = "audit"
)

// useCaseDefaults maps use cases to their default durations.
var useCaseDefaults = map[UseCase]time.Duration{
	UseCaseAlerts:        time.Hour,
	UseCaseLogs:          time.Hour,
	UseCaseMetrics:       24 * time.Hour,
	UseCaseIncidents:     7 * 24 * time.Hour,
	UseCaseTickets:       30 * 24 * time.Hour,
	UseCaseAnalytics:     90 * 24 * time.Hour,
	UseCaseInvestigation: 24 * time.Hour,
	UseCaseHealth:        time.Hour,
	UseCaseAudit:         7 * 24 * time.Hour,
}

// useCasePresets maps use cases to their default preset strings.
var useCasePresets = map[UseCase]string{
	UseCaseAlerts:        Preset1h,
	UseCaseLogs:          Preset1h,
	UseCaseMetrics:       Preset24h,
	UseCaseIncidents:     Preset7d,
	UseCaseTickets:       Preset30d,
	UseCaseAnalytics:     Preset90d,
	UseCaseInvestigation: Preset24h,
	UseCaseHealth:        Preset1h,
	UseCaseAudit:         Preset7d,
}

// DefaultForUseCase returns the recommended default duration for a use case.
func DefaultForUseCase(uc UseCase) time.Duration {
	if d, ok := useCaseDefaults[uc]; ok {
		return d
	}
	return 24 * time.Hour // Default fallback
}

// DefaultPresetForUseCase returns the preset string for a use case.
func DefaultPresetForUseCase(uc UseCase) string {
	if p, ok := useCasePresets[uc]; ok {
		return p
	}
	return Preset24h // Default fallback
}

// ValidPresetsForUseCase returns the preset strings recommended for a use case.
func ValidPresetsForUseCase(uc UseCase) []string {
	switch uc {
	case UseCaseAlerts, UseCaseLogs, UseCaseHealth:
		return ShortPresets()
	case UseCaseAnalytics:
		return LongPresets()
	default:
		return AllPresets()
	}
}

// PresetDescription returns a human-readable description of the preset options
// for a use case, suitable for MCP tool descriptions.
func PresetDescription(uc UseCase) string {
	presets := ValidPresetsForUseCase(uc)
	defaultPreset := DefaultPresetForUseCase(uc)

	result := "Time range: "
	for i, p := range presets {
		if i > 0 {
			result += ", "
		}
		result += p
	}
	result += " (default: " + defaultPreset + ")"
	return result
}

// NewTimeRangeFromPreset creates a TimeRange from a preset string,
// ending at the current time.
func NewTimeRangeFromPreset(preset string) (TimeRange, error) {
	d, err := ParsePreset(preset)
	if err != nil {
		return TimeRange{}, err
	}

	now := time.Now().UTC()
	start := now.Add(-d)

	return TimeRange{
		Start: TimeSpec{
			Time:       start,
			IsRelative: true,
			Raw:        "-" + preset,
			Location:   time.UTC,
		},
		End: TimeSpec{
			Time:       now,
			IsRelative: true,
			Raw:        "now",
			Location:   time.UTC,
		},
		Duration: d,
	}, nil
}

// NewTimeRangeForUseCase creates a TimeRange using the default for the given use case.
func NewTimeRangeForUseCase(uc UseCase) TimeRange {
	preset := DefaultPresetForUseCase(uc)
	tr, _ := NewTimeRangeFromPreset(preset) // Presets are always valid
	return tr
}

// LastHour returns a TimeRange for the last hour.
func LastHour() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset1h)
	return tr
}

// Last6Hours returns a TimeRange for the last 6 hours.
func Last6Hours() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset6h)
	return tr
}

// Last24Hours returns a TimeRange for the last 24 hours.
func Last24Hours() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset24h)
	return tr
}

// Last7Days returns a TimeRange for the last 7 days.
func Last7Days() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset7d)
	return tr
}

// Last30Days returns a TimeRange for the last 30 days.
func Last30Days() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset30d)
	return tr
}

// Last90Days returns a TimeRange for the last 90 days.
func Last90Days() TimeRange {
	tr, _ := NewTimeRangeFromPreset(Preset90d)
	return tr
}
