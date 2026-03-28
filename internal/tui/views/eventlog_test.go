package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewEventLogView(t *testing.T) {
	v := NewEventLogView()
	if len(v.Entries()) != 0 {
		t.Errorf("new view should have 0 entries, got %d", len(v.Entries()))
	}
	if v.Filter() != "" {
		t.Errorf("new view should have empty filter, got %q", v.Filter())
	}
	if v.Paused() {
		t.Error("new view should not be paused")
	}
	if v.ScrollPos() != 0 {
		t.Errorf("new view should have scrollPos=0, got %d", v.ScrollPos())
	}
}

func makeEntry(typ, msg string) EventLogEntry {
	return EventLogEntry{
		Timestamp: time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Type:      typ,
		Session:   "sess-1",
		Message:   msg,
	}
}

func TestAddEntry(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	v.AddEntry(makeEntry("session.started", "started"))
	if len(v.Entries()) != 1 {
		t.Errorf("expected 1 entry, got %d", len(v.Entries()))
	}
	v.AddEntry(makeEntry("loop.started", "loop"))
	if len(v.Entries()) != 2 {
		t.Errorf("expected 2 entries, got %d", len(v.Entries()))
	}
}

func TestAddEntryCapsAtMax(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	for i := 0; i < maxEventLogEntries+50; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iteration"))
	}
	if len(v.Entries()) != maxEventLogEntries {
		t.Errorf("expected %d entries after overflow, got %d", maxEventLogEntries, len(v.Entries()))
	}
}

func TestCycleFilter(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)

	expected := []string{"session", "loop", "fleet", "error", ""}
	for _, want := range expected {
		v.cycleFilter()
		if v.Filter() != want {
			t.Errorf("expected filter %q, got %q", want, v.Filter())
		}
	}
	// Cycling again should restart
	v.cycleFilter()
	if v.Filter() != "session" {
		t.Errorf("expected filter to cycle back to 'session', got %q", v.Filter())
	}
}

func TestFilterApplies(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	v.AddEntry(makeEntry("session.started", "s1"))
	v.AddEntry(makeEntry("loop.started", "l1"))
	v.AddEntry(makeEntry("session.ended", "s2"))
	v.AddEntry(makeEntry("session.error", "err"))
	v.AddEntry(makeEntry("fleet.status", "f1"))

	// No filter — all entries visible
	if len(v.Filtered()) != 5 {
		t.Errorf("no filter: expected 5, got %d", len(v.Filtered()))
	}

	// Filter to session
	v.cycleFilter() // -> "session"
	if len(v.Filtered()) != 3 {
		t.Errorf("session filter: expected 3, got %d", len(v.Filtered()))
	}

	// Filter to loop
	v.cycleFilter() // -> "loop"
	if len(v.Filtered()) != 1 {
		t.Errorf("loop filter: expected 1, got %d", len(v.Filtered()))
	}

	// Filter to fleet
	v.cycleFilter() // -> "fleet"
	if len(v.Filtered()) != 1 {
		t.Errorf("fleet filter: expected 1, got %d", len(v.Filtered()))
	}

	// Filter to error
	v.cycleFilter() // -> "error"
	if len(v.Filtered()) != 1 {
		t.Errorf("error filter: expected 1 (session.error), got %d", len(v.Filtered()))
	}

	// Back to all
	v.cycleFilter() // -> ""
	if len(v.Filtered()) != 5 {
		t.Errorf("all filter: expected 5, got %d", len(v.Filtered()))
	}
}

func TestScrollBoundsUp(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	// scrollPos starts at 0, scrolling up should stay at 0
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	if v.ScrollPos() != 0 {
		t.Errorf("scroll up from 0 should stay at 0, got %d", v.ScrollPos())
	}
}

func TestScrollBoundsDown(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	// Add fewer entries than view height — scrollDown should not move
	for i := 0; i < 3; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iter"))
	}

	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	if v.ScrollPos() != 0 {
		t.Errorf("scroll down with few entries should stay at 0, got %d", v.ScrollPos())
	}
}

func TestScrollToBottom(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	for i := 0; i < 30; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iter"))
	}

	// Go to top
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if v.ScrollPos() != 0 {
		t.Errorf("g should scroll to top, got %d", v.ScrollPos())
	}

	// Go to bottom
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	vh := v.viewHeight()
	expected := len(v.Filtered()) - vh
	if expected < 0 {
		expected = 0
	}
	if v.ScrollPos() != expected {
		t.Errorf("G should scroll to bottom, expected %d, got %d", expected, v.ScrollPos())
	}
}

func TestPauseToggle(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)

	if v.Paused() {
		t.Error("should start unpaused")
	}
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if !v.Paused() {
		t.Error("p should pause")
	}
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	if v.Paused() {
		t.Error("second p should unpause")
	}
}

func TestViewRenders(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	v.AddEntry(makeEntry("session.started", "hello world"))

	output := v.View()
	if !strings.Contains(output, "Events Log") {
		t.Error("view should contain title")
	}
	if !strings.Contains(output, "hello world") {
		t.Error("view should contain entry message")
	}
	if !strings.Contains(output, "10:30:00") {
		t.Error("view should contain formatted timestamp")
	}
	if !strings.Contains(output, "1 events") {
		t.Error("view should show event count")
	}
}

func TestViewShowsFilterLabel(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	v.cycleFilter() // -> "session"

	output := v.View()
	if !strings.Contains(output, "filter: session") {
		t.Error("view should show filter label when active")
	}
}

func TestViewShowsPausedLabel(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})

	output := v.View()
	if !strings.Contains(output, "PAUSED") {
		t.Error("view should show PAUSED when paused")
	}
}

func TestWindowSizeMsg(t *testing.T) {
	v := NewEventLogView()
	v, _ = v.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if v.width != 120 || v.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", v.width, v.height)
	}
}

func TestInit_ReturnsNil(t *testing.T) {
	v := NewEventLogView()
	cmd := v.Init()
	if cmd != nil {
		t.Error("Init() should return nil cmd")
	}
}

func TestScrollDown_Exported(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	// Add enough entries to enable scrolling (more than viewHeight)
	for i := 0; i < 30; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iter"))
	}

	// Go to top first
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	if v.ScrollPos() != 0 {
		t.Fatalf("should be at top, got %d", v.ScrollPos())
	}

	// Use exported ScrollDown
	v.ScrollDown()
	if v.ScrollPos() != 1 {
		t.Errorf("ScrollDown from 0 should move to 1, got %d", v.ScrollPos())
	}

	v.ScrollDown()
	if v.ScrollPos() != 2 {
		t.Errorf("second ScrollDown should move to 2, got %d", v.ScrollPos())
	}
}

func TestScrollDown_Bounded(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	// Add only a few entries (less than viewHeight)
	for i := 0; i < 3; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iter"))
	}

	v.ScrollDown()
	if v.ScrollPos() != 0 {
		t.Errorf("ScrollDown with few entries should stay at 0, got %d", v.ScrollPos())
	}
}

func TestScrollUp_Exported(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 10)

	for i := 0; i < 30; i++ {
		v.AddEntry(makeEntry("loop.iterated", "iter"))
	}

	// Scroll down first
	v, _ = v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	v.ScrollDown()
	v.ScrollDown()
	if v.ScrollPos() != 2 {
		t.Fatalf("should be at 2, got %d", v.ScrollPos())
	}

	v.ScrollUp()
	if v.ScrollPos() != 1 {
		t.Errorf("ScrollUp from 2 should move to 1, got %d", v.ScrollPos())
	}
}

func TestScrollUp_AtZero(t *testing.T) {
	v := NewEventLogView()
	v.ScrollUp()
	if v.ScrollPos() != 0 {
		t.Errorf("ScrollUp from 0 should stay at 0, got %d", v.ScrollPos())
	}
}

func TestLoadHistory(t *testing.T) {
	v := NewEventLogView()
	v.SetDimensions(80, 20)

	entries := []EventLogEntry{
		makeEntry("session.started", "s1"),
		makeEntry("loop.started", "l1"),
		makeEntry("fleet.status", "f1"),
	}
	v.LoadHistory(entries)

	if len(v.Entries()) != 3 {
		t.Errorf("LoadHistory should add 3 entries, got %d", len(v.Entries()))
	}
}

func TestColorForType(t *testing.T) {
	tests := []struct {
		eventType string
		contains  string
	}{
		{"session.started", "session.started"},
		{"loop.started", "loop.started"},
		{"fleet.status", "fleet.status"},
		{"session.error", "error"},
		{"unknown.type", "unknown.type"},
	}
	for _, tt := range tests {
		result := colorForType(tt.eventType)
		if !strings.Contains(result, tt.contains) {
			t.Errorf("colorForType(%q) should contain %q, got %q", tt.eventType, tt.contains, result)
		}
	}
}
