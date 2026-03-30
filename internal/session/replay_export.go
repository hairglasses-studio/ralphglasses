package session

import (
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// ExportFilter controls which events are included in an export.
type ExportFilter struct {
	// EventTypes limits output to these event types. Nil or empty means all types.
	EventTypes []ReplayEventType
	// After excludes events before this time. Zero value means no lower bound.
	After time.Time
	// Before excludes events after this time. Zero value means no upper bound.
	Before time.Time
}

// matches returns true if the event passes the filter.
func (f *ExportFilter) matches(ev ReplayEvent) bool {
	if f == nil {
		return true
	}
	if len(f.EventTypes) > 0 {
		found := false
		for _, t := range f.EventTypes {
			if ev.Type == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	if !f.After.IsZero() && ev.Timestamp.Before(f.After) {
		return false
	}
	if !f.Before.IsZero() && ev.Timestamp.After(f.Before) {
		return false
	}
	return true
}

// filteredEvents returns events from the player that pass the filter.
func filteredEvents(p *Player, f *ExportFilter) []ReplayEvent {
	all := p.Events()
	if f == nil {
		return all
	}
	var out []ReplayEvent
	for _, ev := range all {
		if f.matches(ev) {
			out = append(out, ev)
		}
	}
	return out
}

// ExportMarkdown writes a readable Markdown document from the player's events.
// The document includes a metadata header, timeline, and event details grouped
// by type with tool calls, inputs, and outputs clearly delineated.
func ExportMarkdown(p *Player, w io.Writer, filter *ExportFilter) error {
	events := filteredEvents(p, filter)

	// Header
	if _, err := fmt.Fprintf(w, "# Session Replay Export\n\n"); err != nil {
		return err
	}

	// Metadata
	if _, err := fmt.Fprintf(w, "## Metadata\n\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "- **Total events:** %d\n", len(events)); err != nil {
		return err
	}
	if len(events) > 0 {
		if _, err := fmt.Fprintf(w, "- **Session ID:** %s\n", events[0].SessionID); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "- **First event:** %s\n", events[0].Timestamp.Format(time.RFC3339)); err != nil {
			return err
		}
		last := events[len(events)-1]
		if _, err := fmt.Fprintf(w, "- **Last event:** %s\n", last.Timestamp.Format(time.RFC3339)); err != nil {
			return err
		}
		dur := last.Timestamp.Sub(events[0].Timestamp)
		if _, err := fmt.Fprintf(w, "- **Duration:** %s\n", dur.Round(time.Millisecond)); err != nil {
			return err
		}
	}

	// Event type counts
	counts := make(map[ReplayEventType]int)
	for _, ev := range events {
		counts[ev.Type]++
	}
	if len(counts) > 0 {
		if _, err := fmt.Fprintf(w, "- **Event types:** "); err != nil {
			return err
		}
		first := true
		for _, t := range []ReplayEventType{ReplayInput, ReplayOutput, ReplayTool, ReplayStatus} {
			if c, ok := counts[t]; ok {
				if !first {
					if _, err := fmt.Fprintf(w, ", "); err != nil {
						return err
					}
				}
				if _, err := fmt.Fprintf(w, "%s=%d", t, c); err != nil {
					return err
				}
				first = false
			}
		}
		if _, err := fmt.Fprintf(w, "\n"); err != nil {
			return err
		}
	}

	// Timeline
	if _, err := fmt.Fprintf(w, "\n## Timeline\n\n"); err != nil {
		return err
	}

	if len(events) == 0 {
		if _, err := fmt.Fprintf(w, "_No events recorded._\n"); err != nil {
			return err
		}
		return nil
	}

	baseTime := events[0].Timestamp
	for _, ev := range events {
		offset := ev.Timestamp.Sub(baseTime).Round(time.Millisecond)
		icon := eventIcon(ev.Type)
		if _, err := fmt.Fprintf(w, "### %s %s [+%s]\n\n", icon, ev.Type, offset); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "**Time:** %s\n\n", ev.Timestamp.Format(time.RFC3339Nano)); err != nil {
			return err
		}
		if ev.Data != "" {
			if _, err := fmt.Fprintf(w, "```\n%s\n```\n\n", ev.Data); err != nil {
				return err
			}
		}
	}

	return nil
}

// eventIcon returns a text marker for each event type.
func eventIcon(t ReplayEventType) string {
	switch t {
	case ReplayInput:
		return "[INPUT]"
	case ReplayOutput:
		return "[OUTPUT]"
	case ReplayTool:
		return "[TOOL]"
	case ReplayStatus:
		return "[STATUS]"
	default:
		return "[EVENT]"
	}
}

// ExportJSONDocument is the structured JSON export format.
type ExportJSONDocument struct {
	Version  string              `json:"version"`
	Metadata ExportJSONMetadata  `json:"metadata"`
	Events   []ExportJSONEvent   `json:"events"`
}

// ExportJSONMetadata holds summary information about the exported replay.
type ExportJSONMetadata struct {
	SessionID  string         `json:"session_id"`
	TotalEvents int           `json:"total_events"`
	FirstEvent string         `json:"first_event,omitempty"`
	LastEvent  string         `json:"last_event,omitempty"`
	DurationMS int64          `json:"duration_ms"`
	EventCounts map[string]int `json:"event_counts"`
}

// ExportJSONEvent is a single event in the JSON export.
type ExportJSONEvent struct {
	Timestamp string          `json:"timestamp"`
	Type      ReplayEventType `json:"type"`
	Data      string          `json:"data"`
	SessionID string          `json:"session_id"`
	OffsetMS  int64           `json:"offset_ms"`
}

// ExportJSON writes a structured JSON document from the player's events.
func ExportJSON(p *Player, w io.Writer, filter *ExportFilter) error {
	events := filteredEvents(p, filter)

	doc := ExportJSONDocument{
		Version: "1.0",
		Metadata: ExportJSONMetadata{
			TotalEvents: len(events),
			EventCounts: make(map[string]int),
		},
	}

	for _, ev := range events {
		doc.Metadata.EventCounts[string(ev.Type)]++
	}

	var baseTime time.Time
	if len(events) > 0 {
		baseTime = events[0].Timestamp
		doc.Metadata.SessionID = events[0].SessionID
		doc.Metadata.FirstEvent = events[0].Timestamp.Format(time.RFC3339Nano)
		last := events[len(events)-1]
		doc.Metadata.LastEvent = last.Timestamp.Format(time.RFC3339Nano)
		doc.Metadata.DurationMS = last.Timestamp.Sub(events[0].Timestamp).Milliseconds()
	}

	doc.Events = make([]ExportJSONEvent, len(events))
	for i, ev := range events {
		doc.Events[i] = ExportJSONEvent{
			Timestamp: ev.Timestamp.Format(time.RFC3339Nano),
			Type:      ev.Type,
			Data:      ev.Data,
			SessionID: ev.SessionID,
			OffsetMS:  ev.Timestamp.Sub(baseTime).Milliseconds(),
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
