package timeop

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Common time format layouts for parsing
var timeLayouts = []string{
	time.RFC3339,
	time.RFC3339Nano,
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02T15:04:05",
	"2006-01-02 15:04:05",
	"2006-01-02",
	"01/02/2006",
	"Jan 2, 2006",
}

// Regex patterns for various time formats
var (
	// Matches Unix timestamps (10-13 digits)
	unixPattern = regexp.MustCompile(`^(\d{10,13})$`)

	// Matches relative times like "-1h", "-30m", "-7d", "-2w"
	relativePattern = regexp.MustCompile(`^-(\d+)(s|m|h|d|w|mo|y)$`)

	// Matches Slack timestamps like "1234567890.123456"
	slackPattern = regexp.MustCompile(`^(\d{10})\.(\d{6})$`)

	// Matches preset strings like "1h", "6h", "24h", "7d", "30d"
	presetPattern = regexp.MustCompile(`^(\d+)(h|d|w|mo)$`)
)

// Parse handles all input formats and returns a TimeSpec.
// Supported formats:
//   - RFC3339: "2024-01-01T10:00:00Z"
//   - RFC3339Nano: "2024-01-01T10:00:00.123456789Z"
//   - Date only: "2024-01-01" (assumes 00:00:00 in configured location)
//   - Unix seconds: "1704110400" (10 digits)
//   - Unix milliseconds: "1704110400000" (13 digits)
//   - Relative: "-1h", "-30m", "-7d", "-2w"
//   - Special: "now", "" (both resolve to reference time)
//   - Slack timestamp: "1234567890.123456"
func Parse(input string, opts ...ParseOptions) (TimeSpec, error) {
	opt := DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	// Ensure we have a valid location
	if opt.Location == nil {
		opt.Location = time.UTC
	}

	// Ensure we have a valid reference time
	if opt.ReferenceTime.IsZero() {
		opt.ReferenceTime = time.Now().In(opt.Location)
	}

	input = strings.TrimSpace(input)

	// Handle empty input and "now"
	if input == "" || strings.ToLower(input) == "now" {
		return TimeSpec{
			Time:       opt.ReferenceTime,
			IsRelative: true,
			Raw:        input,
			Location:   opt.Location,
		}, nil
	}

	// Try Slack timestamp first (very specific format)
	if ts, err := ParseSlackTimestamp(input); err == nil {
		ts.Location = opt.Location
		return ts, nil
	}

	// Try relative time
	if ts, err := ParseRelative(input, opt.ReferenceTime); err == nil {
		ts.Location = opt.Location
		return ts, nil
	}

	// Try preset (e.g., "7d" means "7 days ago from now")
	if d, err := ParsePreset(input); err == nil {
		t := opt.ReferenceTime.Add(-d)
		return TimeSpec{
			Time:       t,
			IsRelative: true,
			Raw:        input,
			Location:   opt.Location,
		}, nil
	}

	// Try Unix timestamp
	if ts, err := parseUnix(input, opt.Location); err == nil {
		return ts, nil
	}

	// Try standard time layouts
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, input); err == nil {
			// If the parsed time has no timezone info, assume configured location
			if t.Location() == time.UTC && !strings.Contains(input, "Z") && !strings.Contains(input, "+") && !strings.Contains(input, "-") {
				t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), opt.Location)
			}
			return TimeSpec{
				Time:       t,
				IsRelative: false,
				Raw:        input,
				Location:   t.Location(),
			}, nil
		}
	}

	return TimeSpec{}, ValidationError{
		Field:      "time",
		Input:      input,
		Message:    "unable to parse time value",
		Suggestion: "Use RFC3339 (2024-01-01T10:00:00Z), relative (-1h), preset (7d), or Unix timestamp",
	}
}

// ParseRange parses start and end times into a validated TimeRange.
// If end is empty, defaults to "now".
// If start is empty, returns an error (start is required for ranges).
func ParseRange(start, end string, opts ...ParseOptions) (TimeRange, error) {
	opt := DefaultOptions()
	if len(opts) > 0 {
		opt = opts[0]
	}

	if start == "" {
		return TimeRange{}, ValidationError{
			Field:      "start",
			Input:      start,
			Message:    "start time is required for time ranges",
			Suggestion: "Provide a start time like -1h, -7d, or 2024-01-01T00:00:00Z",
		}
	}

	// Default end to "now"
	if end == "" {
		end = "now"
	}

	startTs, err := Parse(start, opt)
	if err != nil {
		return TimeRange{}, err
	}

	endTs, err := Parse(end, opt)
	if err != nil {
		return TimeRange{}, err
	}

	tr := TimeRange{
		Start:    startTs,
		End:      endTs,
		Duration: endTs.Time.Sub(startTs.Time),
	}

	// Validate the range
	if err := ValidateRange(tr); err != nil {
		return TimeRange{}, err
	}

	return tr, nil
}

// ParsePreset converts preset strings to duration.
// Supported: "1h", "6h", "24h", "7d", "30d", "90d", "6mo"
func ParsePreset(preset string) (time.Duration, error) {
	preset = strings.ToLower(strings.TrimSpace(preset))

	// Check if it's in the preset map
	if d, ok := PresetDuration[preset]; ok {
		return d, nil
	}

	// Try to parse as a pattern like "15h" or "45d"
	matches := presetPattern.FindStringSubmatch(preset)
	if len(matches) == 3 {
		value, err := strconv.Atoi(matches[1])
		if err != nil {
			return 0, fmt.Errorf("invalid preset value: %s", preset)
		}

		var multiplier time.Duration
		switch matches[2] {
		case "h":
			multiplier = time.Hour
		case "d":
			multiplier = 24 * time.Hour
		case "w":
			multiplier = 7 * 24 * time.Hour
		case "mo":
			multiplier = 30 * 24 * time.Hour // Approximate
		default:
			return 0, fmt.Errorf("invalid preset unit: %s", matches[2])
		}

		return time.Duration(value) * multiplier, nil
	}

	// Also try standard Go duration parsing
	if d, err := time.ParseDuration(preset); err == nil {
		// Reject zero or negative durations
		if d <= 0 {
			return 0, fmt.Errorf("preset duration must be positive: %s", preset)
		}
		return d, nil
	}

	return 0, fmt.Errorf("invalid preset: %s", preset)
}

// ParseRelative handles relative time expressions like "-1h", "-30m", "-7d".
func ParseRelative(input string, reference time.Time) (TimeSpec, error) {
	input = strings.ToLower(strings.TrimSpace(input))

	matches := relativePattern.FindStringSubmatch(input)
	if len(matches) != 3 {
		return TimeSpec{}, fmt.Errorf("not a relative time: %s", input)
	}

	value, err := strconv.Atoi(matches[1])
	if err != nil {
		return TimeSpec{}, fmt.Errorf("invalid relative time value: %s", input)
	}

	if value == 0 {
		return TimeSpec{}, ValidationError{
			Field:      "relative",
			Input:      input,
			Message:    "relative time value must be greater than zero",
			Suggestion: "Use -1h, -7d, etc. instead of -0h",
		}
	}

	var duration time.Duration
	switch matches[2] {
	case "s":
		duration = time.Duration(value) * time.Second
	case "m":
		duration = time.Duration(value) * time.Minute
	case "h":
		duration = time.Duration(value) * time.Hour
	case "d":
		duration = time.Duration(value) * 24 * time.Hour
	case "w":
		duration = time.Duration(value) * 7 * 24 * time.Hour
	case "mo":
		duration = time.Duration(value) * 30 * 24 * time.Hour // Approximate
	case "y":
		duration = time.Duration(value) * 365 * 24 * time.Hour // Approximate
	default:
		return TimeSpec{}, fmt.Errorf("unknown relative time unit: %s", matches[2])
	}

	t := reference.Add(-duration)
	return TimeSpec{
		Time:       t,
		IsRelative: true,
		Raw:        input,
		Location:   reference.Location(),
	}, nil
}

// ParseSlackTimestamp handles Slack's "1234567890.123456" format.
// Preserves microsecond precision.
func ParseSlackTimestamp(ts string) (TimeSpec, error) {
	ts = strings.TrimSpace(ts)

	matches := slackPattern.FindStringSubmatch(ts)
	if len(matches) != 3 {
		return TimeSpec{}, fmt.Errorf("not a Slack timestamp: %s", ts)
	}

	seconds, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return TimeSpec{}, fmt.Errorf("invalid Slack timestamp seconds: %s", ts)
	}

	micros, err := strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return TimeSpec{}, fmt.Errorf("invalid Slack timestamp microseconds: %s", ts)
	}

	// Convert to nanoseconds for time.Unix
	nanos := micros * 1000

	t := time.Unix(seconds, nanos).UTC()
	return TimeSpec{
		Time:       t,
		IsRelative: false,
		Raw:        ts,
		Location:   time.UTC,
	}, nil
}

// parseUnix parses Unix timestamps (seconds or milliseconds).
func parseUnix(input string, loc *time.Location) (TimeSpec, error) {
	matches := unixPattern.FindStringSubmatch(input)
	if len(matches) != 2 {
		return TimeSpec{}, fmt.Errorf("not a Unix timestamp: %s", input)
	}

	value, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return TimeSpec{}, fmt.Errorf("invalid Unix timestamp: %s", input)
	}

	var t time.Time
	if len(matches[1]) >= 13 {
		// Milliseconds (13 digits)
		t = time.UnixMilli(value)
	} else {
		// Seconds (10 digits)
		t = time.Unix(value, 0)
	}

	if loc != nil {
		t = t.In(loc)
	}

	return TimeSpec{
		Time:       t,
		IsRelative: false,
		Raw:        input,
		Location:   t.Location(),
	}, nil
}

// ParseDuration parses duration strings in multiple formats:
//   - Presets: "1h", "6h", "24h", "7d", "30d", "90d", "6mo"
//   - Relative-style: "1h", "30m", "7d", "2w" (without leading dash)
//   - Standard Go: "1h30m", "90s", "2h45m"
//
// This is the recommended function for tools that need a raw time.Duration
// rather than a TimeRange. It unifies preset parsing with Go's time.ParseDuration.
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, ValidationError{
			Field:      "duration",
			Input:      s,
			Message:    "duration string is empty",
			Suggestion: "Use a duration like 1h, 24h, 7d, or 30m",
		}
	}

	// Try preset parsing first (handles "7d", "30d", "6mo", etc.)
	if d, err := ParsePreset(s); err == nil {
		return d, nil
	}

	// Fall back to standard Go duration parsing
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, ValidationError{
			Field:      "duration",
			Input:      s,
			Message:    fmt.Sprintf("invalid duration: %v", err),
			Suggestion: "Use formats like 1h, 24h, 7d, 30m, or 1h30m",
		}
	}

	if d <= 0 {
		return 0, ValidationError{
			Field:      "duration",
			Input:      s,
			Message:    "duration must be positive",
			Suggestion: "Use a positive duration like 1h or 30m",
		}
	}

	return d, nil
}

// MustParseDuration is like ParseDuration but panics on error. Use only in tests.
func MustParseDuration(s string) time.Duration {
	d, err := ParseDuration(s)
	if err != nil {
		panic(fmt.Sprintf("timeop.MustParseDuration(%q): %v", s, err))
	}
	return d
}

// MustParse is like Parse but panics on error. Use only in tests or for static input.
func MustParse(input string, opts ...ParseOptions) TimeSpec {
	ts, err := Parse(input, opts...)
	if err != nil {
		panic(fmt.Sprintf("timeop.MustParse(%q): %v", input, err))
	}
	return ts
}

// MustParseRange is like ParseRange but panics on error. Use only in tests or for static input.
func MustParseRange(start, end string, opts ...ParseOptions) TimeRange {
	tr, err := ParseRange(start, end, opts...)
	if err != nil {
		panic(fmt.Sprintf("timeop.MustParseRange(%q, %q): %v", start, end, err))
	}
	return tr
}
