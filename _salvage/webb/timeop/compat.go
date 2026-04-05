// Package timeop provides unified time handling for webb MCP tools.
// This file provides compatibility functions for migrating from legacy implementations.
package timeop

import (
	"time"
)

// ParseTimeCompat is a drop-in replacement for legacy parseTime functions.
// It accepts RFC3339, Unix timestamps, and relative times.
// Returns time.Time directly for API compatibility with existing code.
func ParseTimeCompat(s string) (time.Time, error) {
	ts, err := Parse(s)
	if err != nil {
		return time.Time{}, err
	}
	return ts.Time, nil
}

// ParseRelativeTimeCompat is a drop-in replacement for legacy parseRelativeTime functions.
// Handles "now", "", and relative expressions like "-1h".
// Returns time.Time directly for API compatibility.
func ParseRelativeTimeCompat(s string) (time.Time, error) {
	ts, err := Parse(s)
	if err != nil {
		return time.Time{}, err
	}
	return ts.Time, nil
}

// TimeRangeCompat provides a simple from/to struct for legacy code migration.
type TimeRangeCompat struct {
	From time.Time
	To   time.Time
}

// ParseTimeRangeCompat is a drop-in replacement for legacy parseTimeRange functions.
// Accepts preset strings like "1h", "6h", "24h", "7d".
// Returns TimeRangeCompat for compatibility with existing struct usage.
func ParseTimeRangeCompat(tr string) TimeRangeCompat {
	// Use default if invalid
	if tr == "" {
		tr = "1h"
	}

	timeRange, err := NewTimeRangeFromPreset(tr)
	if err != nil {
		// Fallback to 1h if parsing fails
		timeRange, _ = NewTimeRangeFromPreset("1h")
	}

	return TimeRangeCompat{
		From: timeRange.Start.Time,
		To:   timeRange.End.Time,
	}
}

// ParseTimeRangeFromStrings parses explicit start/end time strings.
// This is for tools that accept separate start_time and end_time parameters.
func ParseTimeRangeFromStrings(start, end string) (TimeRangeCompat, error) {
	tr, err := ParseRange(start, end)
	if err != nil {
		return TimeRangeCompat{}, err
	}
	return TimeRangeCompat{
		From: tr.Start.Time,
		To:   tr.End.Time,
	}, nil
}

// StartUnix returns the Unix timestamp for the From time.
func (tr TimeRangeCompat) StartUnix() int64 {
	return tr.From.Unix()
}

// EndUnix returns the Unix timestamp for the To time.
func (tr TimeRangeCompat) EndUnix() int64 {
	return tr.To.Unix()
}

// StartMillis returns the Unix milliseconds for the From time.
func (tr TimeRangeCompat) StartMillis() int64 {
	return tr.From.UnixMilli()
}

// EndMillis returns the Unix milliseconds for the To time.
func (tr TimeRangeCompat) EndMillis() int64 {
	return tr.To.UnixMilli()
}

// Duration returns the duration of the time range.
func (tr TimeRangeCompat) Duration() time.Duration {
	return tr.To.Sub(tr.From)
}
