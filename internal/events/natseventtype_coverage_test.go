package events

import (
	"testing"
)

// TestNATSBus_ParseEventType tests the parseEventType internal method.
// We use a minimal no-op NATSConnection to construct the bus.
func TestNATSBus_ParseEventType(t *testing.T) {
	tests := []struct {
		name     string
		cfg      NATSBusConfig
		subject  string
		wantType EventType
	}{
		{
			name:     "simple event type no fleet",
			cfg:      NATSBusConfig{SubjectPrefix: "ralph.events"},
			subject:  "ralph.events.session.started",
			wantType: "session.started",
		},
		{
			name:     "event type with fleet id",
			cfg:      NATSBusConfig{SubjectPrefix: "ralph.events", FleetID: "fleet-1"},
			subject:  "ralph.events.fleet-1.session.started",
			wantType: "session.started",
		},
		{
			name:     "unrecognized subject returned as-is",
			cfg:      NATSBusConfig{SubjectPrefix: "ralph.events"},
			subject:  "other.topic.something",
			wantType: "other.topic.something",
		},
		{
			name:     "prefix mismatch returns full subject",
			cfg:      NATSBusConfig{SubjectPrefix: "ralph.events", FleetID: "f1"},
			subject:  "ralph.events.other-fleet.session.started",
			wantType: "ralph.events.other-fleet.session.started",
		},
		{
			name:     "empty fleet uses simple prefix",
			cfg:      NATSBusConfig{SubjectPrefix: "my.prefix"},
			subject:  "my.prefix.session.completed",
			wantType: "session.completed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewNATSBus(nil, tt.cfg)
			got := b.parseEventType(tt.subject)
			if got != tt.wantType {
				t.Errorf("parseEventType(%q) = %q, want %q", tt.subject, got, tt.wantType)
			}
		})
	}
}
