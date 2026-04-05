// Package timecontext provides time-of-day aware context for suggestions.
package timecontext

import "time"

// Block represents a time-of-day block.
type Block string

const (
	Morning  Block = "morning"  // 6-9 AM
	Work     Block = "work"     // 9 AM - 6 PM
	Evening  Block = "evening"  // 6-10 PM
	Night    Block = "night"    // 10 PM - 6 AM
)

// CurrentBlock returns the current time block.
func CurrentBlock() Block {
	return BlockAt(time.Now())
}

// BlockAt returns the time block for a given time.
func BlockAt(t time.Time) Block {
	hour := t.Hour()
	switch {
	case hour >= 6 && hour < 9:
		return Morning
	case hour >= 9 && hour < 18:
		return Work
	case hour >= 18 && hour < 22:
		return Evening
	default:
		return Night
	}
}

// Label returns a human-readable label for the block.
func (b Block) Label() string {
	switch b {
	case Morning:
		return "Morning (6-9 AM)"
	case Work:
		return "Work Hours (9 AM-6 PM)"
	case Evening:
		return "Evening (6-10 PM)"
	case Night:
		return "Night (10 PM-6 AM)"
	default:
		return string(b)
	}
}

// IsWeekend returns true if the given time is Saturday or Sunday.
func IsWeekend(t time.Time) bool {
	day := t.Weekday()
	return day == time.Saturday || day == time.Sunday
}

// Priorities returns suggested activity priorities for each block.
func (b Block) Priorities() []string {
	switch b {
	case Morning:
		return []string{"briefing", "srs_review", "reply_radar", "habits"}
	case Work:
		return []string{"tasks", "learning", "professional_replies", "coding_lab"}
	case Evening:
		return []string{"partner_time", "social", "studio", "dates"}
	case Night:
		return []string{"wind_down", "gratitude", "tomorrow_prep", "reading"}
	default:
		return nil
	}
}
