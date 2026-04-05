// Package timeop provides unified time handling for webb MCP tools.
// It offers consistent parsing, formatting, and validation of timestamps
// across multiple service backends (Slack, Prometheus, Grafana, Pylon, etc.).
package timeop

import (
	"fmt"
	"time"
)

// TimeSpec represents a parsed time value with metadata about how it was specified.
type TimeSpec struct {
	// Time is the resolved absolute time in the specified location
	Time time.Time

	// IsRelative is true if the input was a relative expression (e.g., "-1h", "now")
	IsRelative bool

	// Raw is the original input string for debugging and error messages
	Raw string

	// Location is the timezone for this time (default: UTC)
	Location *time.Location
}

// TimeRange represents a validated time window with start and end times.
type TimeRange struct {
	// Start is the beginning of the time range
	Start TimeSpec

	// End is the end of the time range
	End TimeSpec

	// Duration is the computed length of the range (End.Time - Start.Time)
	Duration time.Duration
}

// ParseOptions configures parsing behavior.
type ParseOptions struct {
	// Location is the timezone for times without explicit timezone (default: UTC)
	Location *time.Location

	// ReferenceTime is the base time for relative calculations (default: time.Now())
	// Setting this is useful for testing and deterministic behavior.
	ReferenceTime time.Time

	// AllowFuture permits end times in the future (default: false)
	AllowFuture bool
}

// DefaultOptions returns sensible defaults for parsing (UTC, now, no future).
func DefaultOptions() ParseOptions {
	return ParseOptions{
		Location:      time.UTC,
		ReferenceTime: time.Now().UTC(),
		AllowFuture:   false,
	}
}

// WithLocation returns options with the specified location.
func (o ParseOptions) WithLocation(loc *time.Location) ParseOptions {
	o.Location = loc
	return o
}

// WithReference returns options with the specified reference time.
func (o ParseOptions) WithReference(t time.Time) ParseOptions {
	o.ReferenceTime = t
	return o
}

// WithAllowFuture returns options that allow future times.
func (o ParseOptions) WithAllowFuture() ParseOptions {
	o.AllowFuture = true
	return o
}

// ValidationError provides detailed error information for time-related failures.
type ValidationError struct {
	// Field is which field failed ("start", "end", "range", "format")
	Field string

	// Input is the original input value that caused the error
	Input string

	// Message is a human-readable error description
	Message string

	// Suggestion provides a helpful fix suggestion
	Suggestion string
}

// Error implements the error interface.
func (e ValidationError) Error() string {
	if e.Suggestion != "" {
		return fmt.Sprintf("time validation error in %s: %s (suggestion: %s)", e.Field, e.Message, e.Suggestion)
	}
	return fmt.Sprintf("time validation error in %s: %s", e.Field, e.Message)
}

// ValidationConfig allows custom validation rules for time ranges.
type ValidationConfig struct {
	// MaxDuration is the maximum allowed range (default: 1 year)
	MaxDuration time.Duration

	// MinDuration is the minimum allowed range (default: 1 second)
	MinDuration time.Duration

	// AllowZero permits zero-length ranges where start == end (default: false)
	AllowZero bool

	// AllowFuture permits future end times (default: false)
	AllowFuture bool

	// AllowNegative permits end before start (default: false)
	AllowNegative bool
}

// DefaultValidationConfig returns sensible defaults for validation.
func DefaultValidationConfig() ValidationConfig {
	return ValidationConfig{
		MaxDuration:   365 * 24 * time.Hour, // 1 year
		MinDuration:   time.Second,
		AllowZero:     false,
		AllowFuture:   false,
		AllowNegative: false,
	}
}

// Now returns a TimeSpec representing the current time.
func Now() TimeSpec {
	return TimeSpec{
		Time:       time.Now().UTC(),
		IsRelative: true,
		Raw:        "now",
		Location:   time.UTC,
	}
}

// FromTime creates a TimeSpec from a time.Time value.
func FromTime(t time.Time) TimeSpec {
	return TimeSpec{
		Time:       t,
		IsRelative: false,
		Raw:        t.Format(time.RFC3339),
		Location:   t.Location(),
	}
}

// Unix returns the Unix timestamp (seconds since epoch) for this TimeSpec.
func (ts TimeSpec) Unix() int64 {
	return ts.Time.Unix()
}

// UnixMilli returns the Unix timestamp in milliseconds for this TimeSpec.
func (ts TimeSpec) UnixMilli() int64 {
	return ts.Time.UnixMilli()
}

// UnixMicro returns the Unix timestamp in microseconds for this TimeSpec.
func (ts TimeSpec) UnixMicro() int64 {
	return ts.Time.UnixMicro()
}

// In returns a new TimeSpec in the specified location.
func (ts TimeSpec) In(loc *time.Location) TimeSpec {
	return TimeSpec{
		Time:       ts.Time.In(loc),
		IsRelative: ts.IsRelative,
		Raw:        ts.Raw,
		Location:   loc,
	}
}

// String returns a human-readable representation of the TimeSpec.
func (ts TimeSpec) String() string {
	if ts.IsRelative {
		return fmt.Sprintf("%s (relative, resolved to %s)", ts.Raw, ts.Time.Format(time.RFC3339))
	}
	return ts.Time.Format(time.RFC3339)
}

// IsZero reports whether the TimeSpec represents the zero time.
func (ts TimeSpec) IsZero() bool {
	return ts.Time.IsZero()
}

// Before reports whether ts is before other.
func (ts TimeSpec) Before(other TimeSpec) bool {
	return ts.Time.Before(other.Time)
}

// After reports whether ts is after other.
func (ts TimeSpec) After(other TimeSpec) bool {
	return ts.Time.After(other.Time)
}

// Add returns a new TimeSpec offset by the given duration.
func (ts TimeSpec) Add(d time.Duration) TimeSpec {
	return TimeSpec{
		Time:       ts.Time.Add(d),
		IsRelative: ts.IsRelative,
		Raw:        ts.Raw,
		Location:   ts.Location,
	}
}

// IsValid reports whether the TimeRange passes basic validation.
func (tr TimeRange) IsValid() bool {
	return !tr.Start.IsZero() && !tr.End.IsZero() && !tr.End.Before(tr.Start)
}

// Contains reports whether the given time falls within this range.
func (tr TimeRange) Contains(t time.Time) bool {
	return (t.Equal(tr.Start.Time) || t.After(tr.Start.Time)) &&
		(t.Equal(tr.End.Time) || t.Before(tr.End.Time))
}

// String returns a human-readable representation of the TimeRange.
func (tr TimeRange) String() string {
	return fmt.Sprintf("%s to %s (%s)", tr.Start.Time.Format(time.RFC3339), tr.End.Time.Format(time.RFC3339), FormatDuration(tr.Duration))
}

// TimeLocation is an alias for time.Location for cleaner external imports.
type TimeLocation = time.Location

// HoursToDuration converts hours (int) to time.Duration.
func HoursToDuration(hours int) time.Duration {
	return time.Duration(hours) * time.Hour
}

// DaysToDuration converts days (int) to time.Duration.
func DaysToDuration(days int) time.Duration {
	return time.Duration(days) * 24 * time.Hour
}

// WeeksToDuration converts weeks (int) to time.Duration.
func WeeksToDuration(weeks int) time.Duration {
	return time.Duration(weeks) * 7 * 24 * time.Hour
}
