package events

import (
	"encoding/json"
	"fmt"
)

// MigrateEvent deserializes a raw JSON event, applying migrations for old versions.
func MigrateEvent(raw json.RawMessage) (Event, error) {
	// First, try to extract version.
	var probe struct {
		Version int `json:"v"`
	}
	if err := json.Unmarshal(raw, &probe); err != nil {
		return Event{}, fmt.Errorf("unmarshal version probe: %w", err)
	}

	switch probe.Version {
	case 0:
		// v0: no version field. Unmarshal directly — all fields compatible with v1.
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return Event{}, fmt.Errorf("unmarshal v0 event: %w", err)
		}
		e.Version = 1
		return e, nil
	case 1:
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return Event{}, fmt.Errorf("unmarshal v1 event: %w", err)
		}
		return e, nil
	default:
		// Forward-compatible: attempt to deserialize future versions as-is.
		var e Event
		if err := json.Unmarshal(raw, &e); err != nil {
			return Event{}, fmt.Errorf("unmarshal v%d event: %w", probe.Version, err)
		}
		return e, nil
	}
}
