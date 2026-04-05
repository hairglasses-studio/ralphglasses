package timeop

import (
	"time"
)

// ValidateRange checks that a time range is sensible using default configuration.
// Returns nil if valid, ValidationError otherwise.
func ValidateRange(tr TimeRange) error {
	return ValidateRangeWithConfig(tr, DefaultValidationConfig())
}

// ValidateRangeWithConfig allows custom validation rules for time ranges.
func ValidateRangeWithConfig(tr TimeRange, cfg ValidationConfig) error {
	// Check for zero times
	if tr.Start.IsZero() {
		return ValidationError{
			Field:      "start",
			Input:      tr.Start.Raw,
			Message:    "start time is not set",
			Suggestion: "Provide a valid start time",
		}
	}

	if tr.End.IsZero() {
		return ValidationError{
			Field:      "end",
			Input:      tr.End.Raw,
			Message:    "end time is not set",
			Suggestion: "Provide a valid end time or use 'now'",
		}
	}

	// Check for negative range (end before start)
	if !cfg.AllowNegative && tr.End.Time.Before(tr.Start.Time) {
		return ValidationError{
			Field:      "range",
			Input:      tr.String(),
			Message:    "end time is before start time",
			Suggestion: "Swap start and end times, or use negative duration like -1h",
		}
	}

	// Check for zero-length range
	duration := tr.End.Time.Sub(tr.Start.Time)
	if !cfg.AllowZero && duration == 0 {
		return ValidationError{
			Field:      "range",
			Input:      tr.String(),
			Message:    "start and end times are identical (zero-length range)",
			Suggestion: "Provide different start and end times",
		}
	}

	// Check for future end time
	if !cfg.AllowFuture && tr.End.Time.After(time.Now()) {
		return ValidationError{
			Field:      "end",
			Input:      tr.End.Raw,
			Message:    "end time is in the future",
			Suggestion: "Use 'now' for end time or set AllowFuture option",
		}
	}

	// Check for minimum duration
	if cfg.MinDuration > 0 && duration < cfg.MinDuration && duration > 0 {
		return ValidationError{
			Field:      "range",
			Input:      tr.String(),
			Message:    "time range is too short (minimum: " + FormatDuration(cfg.MinDuration) + ")",
			Suggestion: "Increase the time range to at least " + FormatDuration(cfg.MinDuration),
		}
	}

	// Check for maximum duration
	if cfg.MaxDuration > 0 && duration > cfg.MaxDuration {
		return ValidationError{
			Field:      "range",
			Input:      tr.String(),
			Message:    "time range is too long (maximum: " + FormatDuration(cfg.MaxDuration) + ")",
			Suggestion: "Reduce the time range to at most " + FormatDuration(cfg.MaxDuration),
		}
	}

	return nil
}

// QuickValidate performs basic validation (end >= start, reasonable window).
// This is the fast path for most tools.
func QuickValidate(start, end TimeSpec) error {
	if start.IsZero() {
		return ValidationError{
			Field:      "start",
			Input:      start.Raw,
			Message:    "start time is not set",
			Suggestion: "Provide a valid start time",
		}
	}

	if end.IsZero() {
		return ValidationError{
			Field:      "end",
			Input:      end.Raw,
			Message:    "end time is not set",
			Suggestion: "Provide a valid end time",
		}
	}

	if end.Time.Before(start.Time) {
		return ValidationError{
			Field:      "range",
			Message:    "end time is before start time",
			Suggestion: "Swap start and end times",
		}
	}

	return nil
}

// ValidateTimezone checks if a timezone string is valid.
// Returns the Location if valid, error otherwise.
func ValidateTimezone(tz string) (*time.Location, error) {
	if tz == "" || tz == "UTC" || tz == "utc" {
		return time.UTC, nil
	}

	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, ValidationError{
			Field:      "timezone",
			Input:      tz,
			Message:    "invalid timezone",
			Suggestion: "Use IANA timezone name like 'America/Los_Angeles' or 'Europe/London'",
		}
	}

	return loc, nil
}

// ValidatePreset checks if a preset string is valid.
func ValidatePreset(preset string) error {
	_, err := ParsePreset(preset)
	if err != nil {
		return ValidationError{
			Field:      "preset",
			Input:      preset,
			Message:    "invalid time preset",
			Suggestion: "Use presets like 1h, 6h, 24h, 7d, 30d, 90d, or Go duration strings like 2h30m",
		}
	}
	return nil
}

// ValidateNotFuture checks that a time is not in the future.
func ValidateNotFuture(ts TimeSpec) error {
	if ts.Time.After(time.Now()) {
		return ValidationError{
			Field:      "time",
			Input:      ts.Raw,
			Message:    "time is in the future",
			Suggestion: "Use a past time or 'now'",
		}
	}
	return nil
}

// ValidateNotTooOld checks that a time is not older than the specified duration.
func ValidateNotTooOld(ts TimeSpec, maxAge time.Duration) error {
	age := time.Since(ts.Time)
	if age > maxAge {
		return ValidationError{
			Field:      "time",
			Input:      ts.Raw,
			Message:    "time is too old (max age: " + FormatDuration(maxAge) + ")",
			Suggestion: "Use a more recent time",
		}
	}
	return nil
}

// ValidateReasonableRange checks common-sense limits for time ranges.
// Returns a warning (not error) if the range seems unusual.
type RangeWarning struct {
	Message    string
	Suggestion string
}

func ValidateReasonableRange(tr TimeRange) *RangeWarning {
	duration := tr.Duration

	// Warn if range is very short for typical use cases
	if duration < time.Minute {
		return &RangeWarning{
			Message:    "time range is less than 1 minute",
			Suggestion: "Most queries work better with at least 5-15 minutes of data",
		}
	}

	// Warn if range is very long
	if duration > 180*24*time.Hour {
		return &RangeWarning{
			Message:    "time range is more than 6 months",
			Suggestion: "Large time ranges may be slow or return too much data; consider using 30d or 90d",
		}
	}

	return nil
}
