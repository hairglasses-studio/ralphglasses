package views

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func makeReplayEvent(typ session.ReplayEventType, data string, offsetMs int) session.ReplayEvent {
	base := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	return session.ReplayEvent{
		Timestamp: base.Add(time.Duration(offsetMs) * time.Millisecond),
		Type:      typ,
		Data:      data,
		SessionID: "test-sess",
	}
}

func sampleEvents() []session.ReplayEvent {
	return []session.ReplayEvent{
		makeReplayEvent(session.ReplayInput, "hello world", 0),
		makeReplayEvent(session.ReplayOutput, "response one", 500),
		makeReplayEvent(session.ReplayTool, "tool:read file.go", 1200),
		makeReplayEvent(session.ReplayOutput, "response two", 2000),
		makeReplayEvent(session.ReplayStatus, "completed", 3000),
		makeReplayEvent(session.ReplayInput, "another input", 4000),
	}
}

func keyMsg(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestNewReplayViewerModel(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	if m.Cursor() != 0 {
		t.Errorf("initial cursor should be 0, got %d", m.Cursor())
	}
	if m.FilterMode() != filterAll {
		t.Errorf("initial filter should be all, got %s", m.FilterMode())
	}
	if m.Playing() {
		t.Error("should not be playing initially")
	}
	if len(m.Filtered()) != 6 {
		t.Errorf("expected 6 filtered events, got %d", len(m.Filtered()))
	}
}

func TestNewReplayViewerModel_Empty(t *testing.T) {
	m := NewReplayViewerModel(nil)
	if m.Cursor() != 0 {
		t.Errorf("cursor should be 0 for empty events, got %d", m.Cursor())
	}
	if len(m.Filtered()) != 0 {
		t.Errorf("expected 0 filtered, got %d", len(m.Filtered()))
	}
}

func TestInit_ReturnsNilCmd(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	if cmd := m.Init(); cmd != nil {
		t.Error("Init() should return nil")
	}
}

func TestStepForward(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("j"))
	if m.Cursor() != 1 {
		t.Errorf("j should move cursor to 1, got %d", m.Cursor())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	if m.Cursor() != 2 {
		t.Errorf("down arrow should move cursor to 2, got %d", m.Cursor())
	}
}

func TestStepBack(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Move forward first
	m, _ = m.Update(keyMsg("j"))
	m, _ = m.Update(keyMsg("j"))
	if m.Cursor() != 2 {
		t.Fatalf("cursor should be 2, got %d", m.Cursor())
	}

	m, _ = m.Update(keyMsg("k"))
	if m.Cursor() != 1 {
		t.Errorf("k should move cursor to 1, got %d", m.Cursor())
	}

	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	if m.Cursor() != 0 {
		t.Errorf("up arrow should move cursor to 0, got %d", m.Cursor())
	}
}

func TestBoundaryFirst(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Already at 0, moving back should stay at 0
	m, _ = m.Update(keyMsg("k"))
	if m.Cursor() != 0 {
		t.Errorf("k at start should stay at 0, got %d", m.Cursor())
	}
}

func TestBoundaryLast(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Jump to end
	m, _ = m.Update(keyMsg("G"))
	if m.Cursor() != 5 {
		t.Errorf("G should move to last event (5), got %d", m.Cursor())
	}

	// Pressing j at end should stay
	m, _ = m.Update(keyMsg("j"))
	if m.Cursor() != 5 {
		t.Errorf("j at end should stay at 5, got %d", m.Cursor())
	}
}

func TestJumpToStartAndEnd(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("G"))
	if m.Cursor() != 5 {
		t.Errorf("G should go to end, got %d", m.Cursor())
	}

	m, _ = m.Update(keyMsg("g"))
	if m.Cursor() != 0 {
		t.Errorf("g should go to start, got %d", m.Cursor())
	}
}

func TestFilterCycles(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// all -> input -> output -> tool -> status -> all
	expected := []struct {
		filter replayFilter
		count  int
	}{
		{filterInput, 2},
		{filterOutput, 2},
		{filterTool, 1},
		{filterStatus, 1},
		{filterAll, 6},
	}

	for _, want := range expected {
		m, _ = m.Update(keyMsg("f"))
		if m.FilterMode() != want.filter {
			t.Errorf("expected filter %s, got %s", want.filter, m.FilterMode())
		}
		if len(m.Filtered()) != want.count {
			t.Errorf("filter %s: expected %d events, got %d", want.filter, want.count, len(m.Filtered()))
		}
	}
}

func TestFilterClampsCursor(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Move to end
	m, _ = m.Update(keyMsg("G"))
	if m.Cursor() != 5 {
		t.Fatalf("should be at 5, got %d", m.Cursor())
	}

	// Filter to tool (only 1 event)
	m, _ = m.Update(keyMsg("f")) // input (2)
	m, _ = m.Update(keyMsg("f")) // output (2)
	m, _ = m.Update(keyMsg("f")) // tool (1)
	if m.Cursor() != 0 {
		t.Errorf("cursor should be clamped to 0 with 1 tool event, got %d", m.Cursor())
	}
}

func TestPlayToggle(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	if m.Playing() {
		t.Error("should not be playing initially")
	}

	m, cmd := m.Update(keyMsg("p"))
	if !m.Playing() {
		t.Error("p should start playing")
	}
	if cmd == nil {
		t.Error("playing should produce a tick command")
	}

	m, _ = m.Update(keyMsg("p"))
	if m.Playing() {
		t.Error("second p should stop playing")
	}
}

func TestSpeedCycle(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	if m.Speed() != 1 {
		t.Errorf("initial speed should be 1, got %.0f", m.Speed())
	}

	m, _ = m.Update(keyMsg("s"))
	if m.Speed() != 2 {
		t.Errorf("first s should set speed to 2, got %.0f", m.Speed())
	}

	m, _ = m.Update(keyMsg("s"))
	if m.Speed() != 5 {
		t.Errorf("second s should set speed to 5, got %.0f", m.Speed())
	}

	m, _ = m.Update(keyMsg("s"))
	if m.Speed() != 10 {
		t.Errorf("third s should set speed to 10, got %.0f", m.Speed())
	}

	m, _ = m.Update(keyMsg("s"))
	if m.Speed() != 1 {
		t.Errorf("fourth s should wrap to 1, got %.0f", m.Speed())
	}
}

func TestAutoPlayAdvances(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Start playing
	m, _ = m.Update(keyMsg("p"))
	if m.Cursor() != 0 {
		t.Fatalf("cursor should be 0 before tick, got %d", m.Cursor())
	}

	// Simulate tick
	m, _ = m.Update(ReplayTickMsg{})
	if m.Cursor() != 1 {
		t.Errorf("tick should advance cursor to 1, got %d", m.Cursor())
	}

	// Another tick
	m, _ = m.Update(ReplayTickMsg{})
	if m.Cursor() != 2 {
		t.Errorf("second tick should advance cursor to 2, got %d", m.Cursor())
	}
}

func TestAutoPlayStopsAtEnd(t *testing.T) {
	events := sampleEvents()[:2] // only 2 events
	m := NewReplayViewerModel(events)
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("p"))
	m, _ = m.Update(ReplayTickMsg{}) // advance to index 1 (last)
	if m.Cursor() != 1 {
		t.Fatalf("should be at last event, got %d", m.Cursor())
	}

	// Another tick should stop playing
	m, _ = m.Update(ReplayTickMsg{})
	if m.Playing() {
		t.Error("should stop playing at end")
	}
}

func TestSearchEnterAndJump(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Enter search mode
	m, _ = m.Update(keyMsg("/"))
	if !m.Searching() {
		t.Fatal("/ should enter search mode")
	}

	// Type query
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("response")})
	if m.SearchQuery() != "response" {
		t.Errorf("query should be 'response', got %q", m.SearchQuery())
	}

	// Confirm search
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.Searching() {
		t.Error("enter should exit search mode")
	}
	if len(m.SearchHits()) != 2 {
		t.Errorf("expected 2 search hits for 'response', got %d", len(m.SearchHits()))
	}
	// Cursor should jump to first hit
	if m.Cursor() != 1 {
		t.Errorf("cursor should jump to first hit at index 1, got %d", m.Cursor())
	}
}

func TestSearchNextPrev(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	// Search for "response"
	m, _ = m.Update(keyMsg("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("response")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// At first hit (index 1)
	if m.Cursor() != 1 {
		t.Fatalf("should be at hit 0 (index 1), got %d", m.Cursor())
	}

	// Next hit
	m, _ = m.Update(keyMsg("n"))
	if m.Cursor() != 3 {
		t.Errorf("n should move to next hit at index 3, got %d", m.Cursor())
	}

	// Next again should wrap
	m, _ = m.Update(keyMsg("n"))
	if m.Cursor() != 1 {
		t.Errorf("n should wrap to first hit at index 1, got %d", m.Cursor())
	}

	// Prev from first should wrap to last
	m, _ = m.Update(keyMsg("N"))
	if m.Cursor() != 3 {
		t.Errorf("N from first hit should wrap to last hit at index 3, got %d", m.Cursor())
	}
}

func TestSearchEscape(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})

	if m.Searching() {
		t.Error("esc should exit search mode")
	}
	if m.SearchQuery() != "" {
		t.Error("esc should clear search query")
	}
}

func TestSearchBackspace(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("abc")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})

	if m.SearchQuery() != "ab" {
		t.Errorf("backspace should remove last char, got %q", m.SearchQuery())
	}
}

func TestSearchNoHits(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("zzzznotfound")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(m.SearchHits()) != 0 {
		t.Errorf("expected 0 hits, got %d", len(m.SearchHits()))
	}
	if m.Cursor() != 0 {
		t.Errorf("cursor should not move on no hits, got %d", m.Cursor())
	}
}

func TestReplayViewRenders(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	output := m.View()
	if !strings.Contains(output, "Replay Viewer") {
		t.Error("view should contain title")
	}
	if !strings.Contains(output, "hello world") {
		t.Error("view should contain first event data")
	}
	if !strings.Contains(output, "00:00.000") {
		t.Error("view should contain time offset for first event")
	}
	if !strings.Contains(output, "1/6") {
		t.Error("view should show position/total")
	}
	if !strings.Contains(output, "speed: 1x") {
		t.Error("view should show playback speed")
	}
}

func TestReplayViewShowsFilterLabel(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("f")) // -> input
	output := m.View()
	if !strings.Contains(output, "filter: input") {
		t.Error("view should show active filter label")
	}
}

func TestViewShowsPlayingLabel(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("p"))
	output := m.View()
	if !strings.Contains(output, "PLAYING") {
		t.Error("view should show PLAYING when auto-playing")
	}
}

func TestViewShowsSearchMatches(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m.SetDimensions(80, 30)

	m, _ = m.Update(keyMsg("/"))
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("response")})
	m, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	output := m.View()
	if !strings.Contains(output, "matches: 2") {
		t.Error("view should show match count")
	}
}

func TestReplayWindowSizeMsg(t *testing.T) {
	m := NewReplayViewerModel(sampleEvents())
	m, _ = m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if m.width != 120 || m.height != 40 {
		t.Errorf("expected 120x40, got %dx%d", m.width, m.height)
	}
}

func TestFormatOffset(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{0, "00:00.000"},
		{500 * time.Millisecond, "00:00.500"},
		{65500 * time.Millisecond, "01:05.500"},
	}
	for _, tt := range tests {
		got := formatOffset(tt.d)
		if got != tt.want {
			t.Errorf("formatOffset(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestEmptyEventsView(t *testing.T) {
	m := NewReplayViewerModel(nil)
	m.SetDimensions(80, 30)

	output := m.View()
	if !strings.Contains(output, "0/0") {
		t.Error("empty model should show 0/0")
	}
}
