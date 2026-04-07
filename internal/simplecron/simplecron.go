package simplecron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Field is a parsed simple-cron token.
// Supported syntax is limited to "*", exact numbers, and "*/N".
type Field struct {
	Any   bool
	Step  int
	Value int
}

// Parse parses a 5-field cron expression using the repo's simple-cron syntax.
func Parse(expr string) ([]Field, error) {
	parts := strings.Fields(strings.TrimSpace(expr))
	if len(parts) != 5 {
		return nil, fmt.Errorf("cron must have exactly 5 fields")
	}
	fields := make([]Field, len(parts))
	for i, raw := range parts {
		field, err := parseField(raw)
		if err != nil {
			return nil, err
		}
		fields[i] = field
	}
	return fields, nil
}

// FieldMatchesToken reports whether a single raw simple-cron token matches a value.
func FieldMatchesToken(token string, value int) bool {
	field, err := parseField(token)
	if err != nil {
		return false
	}
	return fieldMatches(field, value)
}

// Match reports whether a parsed expression matches the given time.
func Match(fields []Field, t time.Time) bool {
	values := []int{t.Minute(), t.Hour(), t.Day(), int(t.Month()), int(t.Weekday())}
	for i := range fields {
		if !fieldMatches(fields[i], values[i]) {
			return false
		}
	}
	return true
}

// NextMatch returns the next time after from that matches the parsed expression.
func NextMatch(fields []Field, from time.Time) time.Time {
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525600; i++ {
		if Match(fields, t) {
			return t
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}
}

// PrevMatch returns the most recent time at or before from that matches the parsed expression.
func PrevMatch(fields []Field, from time.Time) time.Time {
	t := from.Truncate(time.Minute)
	for i := 0; i < 525600; i++ {
		if Match(fields, t) && !t.After(from) {
			return t
		}
		t = t.Add(-time.Minute)
	}
	return time.Time{}
}

// NextRuns returns the next count run times after from.
func NextRuns(fields []Field, from time.Time, count int) []time.Time {
	var runs []time.Time
	t := from.Truncate(time.Minute).Add(time.Minute)
	for i := 0; i < 525600 && len(runs) < count; i++ {
		if Match(fields, t) {
			runs = append(runs, t)
		}
		t = t.Add(time.Minute)
	}
	return runs
}

func parseField(raw string) (Field, error) {
	switch {
	case raw == "*":
		return Field{Any: true}, nil
	case strings.HasPrefix(raw, "*/"):
		n, err := strconv.Atoi(strings.TrimPrefix(raw, "*/"))
		if err != nil || n <= 0 {
			return Field{}, fmt.Errorf("invalid cron field %q", raw)
		}
		return Field{Step: n}, nil
	default:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return Field{}, fmt.Errorf("invalid cron field %q", raw)
		}
		return Field{Value: n}, nil
	}
}

func fieldMatches(field Field, value int) bool {
	switch {
	case field.Any:
		return true
	case field.Step > 0:
		return value%field.Step == 0
	default:
		return value == field.Value
	}
}
