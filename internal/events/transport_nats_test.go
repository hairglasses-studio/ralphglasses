package events

import (
	"testing"
)

// Verify NATSTransport satisfies EventTransport at compile time.
var _ EventTransport = (*NATSTransport)(nil)

func TestNATSConfig_Defaults(t *testing.T) {
	cfg := NATSConfig{URL: "nats://localhost:4222"}
	cfg.defaults()

	if cfg.StreamName != "RALPH_EVENTS" {
		t.Errorf("StreamName = %q, want RALPH_EVENTS", cfg.StreamName)
	}
	if cfg.SubjectPrefix != "ralph.events" {
		t.Errorf("SubjectPrefix = %q, want ralph.events", cfg.SubjectPrefix)
	}
	if cfg.MaxReconnects != 60 {
		t.Errorf("MaxReconnects = %d, want 60", cfg.MaxReconnects)
	}
	if cfg.ReconnectWait != 2e9 {
		t.Errorf("ReconnectWait = %v, want 2s", cfg.ReconnectWait)
	}
	if cfg.ConnectTimeout != 5e9 {
		t.Errorf("ConnectTimeout = %v, want 5s", cfg.ConnectTimeout)
	}
	if cfg.AckWait != 30e9 {
		t.Errorf("AckWait = %v, want 30s", cfg.AckWait)
	}
}

func TestNATSConfig_DefaultsPreserveExplicit(t *testing.T) {
	cfg := NATSConfig{
		URL:           "nats://custom:4222",
		StreamName:    "CUSTOM_STREAM",
		SubjectPrefix: "custom.prefix",
		MaxReconnects: 10,
	}
	cfg.defaults()

	if cfg.StreamName != "CUSTOM_STREAM" {
		t.Errorf("StreamName = %q, want CUSTOM_STREAM", cfg.StreamName)
	}
	if cfg.SubjectPrefix != "custom.prefix" {
		t.Errorf("SubjectPrefix = %q, want custom.prefix", cfg.SubjectPrefix)
	}
	if cfg.MaxReconnects != 10 {
		t.Errorf("MaxReconnects = %d, want 10", cfg.MaxReconnects)
	}
}

func TestSubjectForEvent(t *testing.T) {
	tests := []struct {
		prefix string
		event  EventType
		want   string
	}{
		{"ralph.events", SessionStarted, "ralph.events.session.started"},
		{"ralph.events", CostUpdate, "ralph.events.cost.update"},
		{"ralph.events", BudgetAlert, "ralph.events.budget.alert"},
		{"ralph.events", LoopIterated, "ralph.events.loop.iterated"},
		{"custom.prefix", SessionEnded, "custom.prefix.session.ended"},
		{"a", WorkerPaused, "a.worker.paused"},
	}

	for _, tt := range tests {
		got := SubjectForEvent(tt.prefix, tt.event)
		if got != tt.want {
			t.Errorf("SubjectForEvent(%q, %q) = %q, want %q", tt.prefix, tt.event, got, tt.want)
		}
	}
}

func TestParseEventTypeFromSubject(t *testing.T) {
	tests := []struct {
		prefix  string
		subject string
		want    EventType
	}{
		{"ralph.events", "ralph.events.session.started", SessionStarted},
		{"ralph.events", "ralph.events.cost.update", CostUpdate},
		{"ralph.events", "ralph.events.budget.alert", BudgetAlert},
		{"custom", "custom.loop.iterated", LoopIterated},
		// Unprefixed subject returns as-is
		{"ralph.events", "other.subject", EventType("other.subject")},
	}

	for _, tt := range tests {
		got := parseEventTypeFromSubject(tt.prefix, tt.subject)
		if got != tt.want {
			t.Errorf("parseEventTypeFromSubject(%q, %q) = %q, want %q", tt.prefix, tt.subject, got, tt.want)
		}
	}
}

func TestSanitizeConsumerName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-dashes", "with-dashes"},
		{"with_underscores", "with_underscores"},
		{"with.dots", "with_dots"},
		{"with spaces", "with_spaces"},
		{"with/slashes", "with_slashes"},
		{"MixedCase123", "MixedCase123"},
		{"special!@#$%", "special_____"},
	}

	for _, tt := range tests {
		got := sanitizeConsumerName(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeConsumerName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSubjectRoundTrip(t *testing.T) {
	prefix := "ralph.events"
	for et := range knownEventTypes {
		subject := SubjectForEvent(prefix, et)
		parsed := parseEventTypeFromSubject(prefix, subject)
		if parsed != et {
			t.Errorf("round-trip failed for %q: subject=%q, parsed=%q", et, subject, parsed)
		}
	}
}

func TestNewNATSTransport_ConnectionFailure(t *testing.T) {
	// Attempting to connect to a non-existent NATS server should fail
	// gracefully with an error, not panic.
	cfg := NATSConfig{
		URL:            "nats://127.0.0.1:14222", // unlikely to have a server here
		ConnectTimeout: 100e6,                     // 100ms — fail fast
		MaxReconnects:  1,
	}
	tr, err := NewNATSTransport(cfg)
	if err == nil {
		tr.Close()
		t.Fatal("expected error connecting to non-existent NATS server")
	}
	// Verify it's a connection error, not a configuration panic
	if tr != nil {
		t.Error("transport should be nil on error")
	}
}
