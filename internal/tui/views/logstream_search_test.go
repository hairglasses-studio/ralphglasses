package views

import (
	"strings"
	"testing"
	"time"
)

func sampleLines() []LogLine {
	t0 := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
	return []LogLine{
		{Index: 0, Timestamp: t0, Level: "INFO", Message: "Server started"},
		{Index: 1, Timestamp: t0.Add(time.Second), Level: "DEBUG", Message: "Loading config"},
		{Index: 2, Timestamp: t0.Add(2 * time.Second), Level: "WARN", Message: "Slow query detected"},
		{Index: 3, Timestamp: t0.Add(3 * time.Second), Level: "ERROR", Message: "Connection failed"},
		{Index: 4, Timestamp: t0.Add(4 * time.Second), Level: "INFO", Message: "Server restarted"},
		{Index: 5, Timestamp: t0.Add(5 * time.Second), Level: "INFO", Message: "Config loaded successfully"},
	}
}

func TestLogSearchModel_Search_CaseInsensitive(t *testing.T) {
	m := NewLogSearch(sampleLines())

	// "server" should match "Server started" and "Server restarted" (case-insensitive)
	matches := m.Search("server")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
	if matches[0] != 0 {
		t.Errorf("expected first match at index 0, got %d", matches[0])
	}
	if matches[1] != 4 {
		t.Errorf("expected second match at index 4, got %d", matches[1])
	}

	// "CONFIG" (uppercase) should match "Loading config" and "Config loaded successfully"
	matches = m.Search("CONFIG")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches for CONFIG, got %d", len(matches))
	}
	if matches[0] != 1 {
		t.Errorf("expected first match at index 1, got %d", matches[0])
	}
	if matches[1] != 5 {
		t.Errorf("expected second match at index 5, got %d", matches[1])
	}
}

func TestLogSearchModel_Search_NoResults(t *testing.T) {
	m := NewLogSearch(sampleLines())

	matches := m.Search("nonexistent")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches, got %d", len(matches))
	}

	matches = m.Search("")
	if matches != nil {
		t.Errorf("expected nil for empty query, got %v", matches)
	}
}

func TestLogSearchModel_NextMatch_Wraps(t *testing.T) {
	m := NewLogSearch(sampleLines())
	m.width = 80
	m.height = 20

	m.matches = m.Search("server")
	if len(m.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.matches))
	}
	m.currentMatch = 0

	// Advance to second match.
	m.nextMatch()
	if m.currentMatch != 1 {
		t.Errorf("expected currentMatch=1, got %d", m.currentMatch)
	}

	// Wrap around to first match.
	m.nextMatch()
	if m.currentMatch != 0 {
		t.Errorf("expected currentMatch=0 after wrap, got %d", m.currentMatch)
	}
}

func TestLogSearchModel_PrevMatch_Wraps(t *testing.T) {
	m := NewLogSearch(sampleLines())
	m.width = 80
	m.height = 20

	m.matches = m.Search("server")
	if len(m.matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(m.matches))
	}
	m.currentMatch = 0

	// Previous from 0 should wrap to last match.
	m.prevMatch()
	if m.currentMatch != 1 {
		t.Errorf("expected currentMatch=1 after wrap, got %d", m.currentMatch)
	}

	// Previous again should go to 0.
	m.prevMatch()
	if m.currentMatch != 0 {
		t.Errorf("expected currentMatch=0, got %d", m.currentMatch)
	}
}

func TestLogSearchModel_View(t *testing.T) {
	m := NewLogSearch(sampleLines())
	m.width = 120
	m.height = 20

	// Initial view should show line count header and help bar.
	v := m.View().Content
	if !strings.Contains(v, "6 lines") {
		t.Error("expected view to contain line count")
	}
	if !strings.Contains(v, "/: search") {
		t.Error("expected view to contain help text")
	}

	// After searching, view should show match count.
	m.query = "server"
	m.matches = m.Search("server")
	m.currentMatch = 0
	m.scrollToMatch()
	v = m.View().Content
	if !strings.Contains(v, "match 1 of 2") {
		t.Errorf("expected match status in view, got:\n%s", v)
	}

	// All sample lines should be visible (viewport large enough).
	if !strings.Contains(v, "Server started") {
		t.Error("expected 'Server started' in view")
	}
	if !strings.Contains(v, "Connection failed") {
		t.Error("expected 'Connection failed' in view")
	}

	// Test searching state.
	m.searching = true
	m.query = "test"
	v = m.View().Content
	if !strings.Contains(v, "Search:") {
		t.Error("expected 'Search:' prompt in searching mode")
	}

	// Test no-match state.
	m.searching = false
	m.query = "zzz"
	m.matches = m.Search("zzz")
	m.currentMatch = -1
	v = m.View().Content
	if !strings.Contains(v, "no matches") {
		t.Error("expected 'no matches' in view")
	}
}
