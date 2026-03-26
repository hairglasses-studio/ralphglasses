package views

import (
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func TestTruncateLabel(t *testing.T) {
	tests := []struct {
		label string
		width int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"longname", 5, "long\u2026"},
		{"abc", 0, "abc"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncateLabel(tt.label, tt.width)
		if got != tt.want {
			t.Errorf("truncateLabel(%q, %d) = %q, want %q", tt.label, tt.width, got, tt.want)
		}
	}
}

func TestFleetMarker(t *testing.T) {
	sel := fleetMarker(true)
	if !strings.Contains(sel, ">") {
		t.Error("selected marker should contain '>'")
	}
	unsel := fleetMarker(false)
	if strings.Contains(unsel, ">") {
		t.Error("unselected marker should not contain '>'")
	}
}

func TestEventTypeIcon(t *testing.T) {
	types := []events.EventType{
		events.SessionStarted,
		events.SessionEnded,
		events.SessionStopped,
		events.CostUpdate,
		events.BudgetExceeded,
		events.LoopStarted,
		events.LoopStopped,
		events.TeamCreated,
	}
	for _, et := range types {
		icon := eventTypeIcon(et)
		if icon == "" {
			t.Errorf("eventTypeIcon(%q) returned empty", et)
		}
	}
	// Unknown type should still return something
	icon := eventTypeIcon("unknown.event")
	if icon == "" {
		t.Error("eventTypeIcon for unknown type should not be empty")
	}
}

func TestEventTypeLabel(t *testing.T) {
	label := eventTypeLabel("session.started")
	if !strings.Contains(label, "session") {
		t.Error("label should contain 'session'")
	}
	if !strings.Contains(label, "started") {
		t.Error("label should contain 'started'")
	}

	// Single-part type (no dot)
	singleLabel := eventTypeLabel("unknown")
	if !strings.Contains(singleLabel, "unknown") {
		t.Error("single-part label should contain the type")
	}
}

func TestRenderFleetDashboardEmpty(t *testing.T) {
	out := RenderFleetDashboard(FleetData{
		Providers: nil,
	}, 120, 40)
	if !strings.Contains(out, "Fleet Dashboard") {
		t.Error("should contain title")
	}
	if !strings.Contains(out, "No alerts") {
		t.Error("empty fleet should show 'No alerts'")
	}
}

func TestRenderFleetDashboardWithAlerts(t *testing.T) {
	out := RenderFleetDashboard(FleetData{
		Alerts: []FleetAlert{
			{Severity: "critical", Message: "Budget exceeded on repo-x"},
			{Severity: "warning", Message: "High latency detected"},
		},
	}, 120, 40)
	if !strings.Contains(out, "Budget exceeded on repo-x") {
		t.Error("should contain alert message")
	}
	if !strings.Contains(out, "High latency detected") {
		t.Error("should contain warning message")
	}
}

func TestRenderFleetDashboardWithOpenCircuits(t *testing.T) {
	out := RenderFleetDashboard(FleetData{
		OpenCircuits: 3,
	}, 120, 40)
	if !strings.Contains(out, "3 open") {
		t.Error("should show open circuit count")
	}
}
