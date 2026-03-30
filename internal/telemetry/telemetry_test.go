package telemetry

import "testing"

func TestAllEventTypes(t *testing.T) {
	types := AllEventTypes()
	if len(types) != 5 {
		t.Errorf("AllEventTypes() returned %d types, want 5", len(types))
	}
	seen := make(map[EventType]bool)
	for _, et := range types {
		if seen[et] {
			t.Errorf("duplicate event type: %s", et)
		}
		seen[et] = true
	}
}
