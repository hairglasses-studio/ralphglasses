package components

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestNewSearchInput(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	if s == nil {
		t.Fatal("NewSearchInput returned nil")
	}
	if s.Active {
		t.Error("new search input should not be active")
	}
	if s.Query != "" {
		t.Errorf("Query = %q, want empty", s.Query)
	}
	if s.maxShow != 12 {
		t.Errorf("maxShow = %d, want 12", s.maxShow)
	}
}

func TestSearchInput_Activate(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Query = "leftover"
	s.Results = []SearchResult{{Name: "old"}}
	s.Selected = 5

	s.Activate()

	if !s.Active {
		t.Error("expected Active after Activate()")
	}
	if s.Query != "" {
		t.Errorf("Query after Activate = %q, want empty", s.Query)
	}
	if s.Results != nil {
		t.Error("Results should be nil after Activate")
	}
	if s.Selected != 0 {
		t.Errorf("Selected = %d, want 0", s.Selected)
	}
}

func TestSearchInput_Deactivate(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Deactivate()
	if s.Active {
		t.Error("expected not Active after Deactivate()")
	}
}

func TestSearchInput_Reset(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "test"
	s.Results = []SearchResult{{Name: "r1"}}
	s.Selected = 1

	s.Reset()

	if !s.Active {
		t.Error("Reset should not change Active state")
	}
	if s.Query != "" {
		t.Errorf("Query after Reset = %q, want empty", s.Query)
	}
	if s.Results != nil {
		t.Error("Results should be nil after Reset")
	}
	if s.Selected != 0 {
		t.Errorf("Selected after Reset = %d, want 0", s.Selected)
	}
}

func TestSearchInput_HandleKey_Escape(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()

	result, ok := s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if ok {
		t.Error("Escape should return ok=false")
	}
	if result.Name != "" {
		t.Error("Escape should return zero SearchResult")
	}
	if s.Active {
		t.Error("Escape should deactivate")
	}
}

func TestSearchInput_HandleKey_Enter_WithResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Results = []SearchResult{
		{Type: SearchTypeRepo, Name: "repo1", Score: 95},
		{Type: SearchTypeSession, Name: "session1", Score: 80},
	}
	s.Selected = 1

	result, ok := s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !ok {
		t.Fatal("Enter with results should return ok=true")
	}
	if result.Name != "session1" {
		t.Errorf("selected result Name = %q, want session1", result.Name)
	}
	if result.Type != SearchTypeSession {
		t.Errorf("selected result Type = %q, want session", result.Type)
	}
}

func TestSearchInput_HandleKey_Enter_NoResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()

	_, ok := s.HandleKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if ok {
		t.Error("Enter with no results should return ok=false")
	}
}

func TestSearchInput_HandleKey_ArrowUp(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Results = []SearchResult{{Name: "a"}, {Name: "b"}, {Name: "c"}}
	s.Selected = 2

	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Selected != 1 {
		t.Errorf("Selected after Up = %d, want 1", s.Selected)
	}

	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Selected != 0 {
		t.Errorf("Selected after second Up = %d, want 0", s.Selected)
	}

	// Should not go below 0.
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if s.Selected != 0 {
		t.Errorf("Selected after Up at 0 = %d, want 0", s.Selected)
	}
}

func TestSearchInput_HandleKey_ArrowDown(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Results = []SearchResult{{Name: "a"}, {Name: "b"}}
	s.Selected = 0

	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Selected != 1 {
		t.Errorf("Selected after Down = %d, want 1", s.Selected)
	}

	// Should not exceed len-1.
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if s.Selected != 1 {
		t.Errorf("Selected after Down at max = %d, want 1", s.Selected)
	}
}

func TestSearchInput_HandleKey_Backspace(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "abc"
	s.Selected = 2

	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if s.Query != "ab" {
		t.Errorf("Query after Backspace = %q, want ab", s.Query)
	}
	if s.Selected != 0 {
		t.Errorf("Selected after Backspace = %d, want 0", s.Selected)
	}

	// Backspace on empty should be no-op.
	s.Query = ""
	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyBackspace})
	if s.Query != "" {
		t.Errorf("Query after Backspace on empty = %q, want empty", s.Query)
	}
}

func TestSearchInput_HandleKey_Delete(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "xy"

	s.HandleKey(tea.KeyPressMsg{Code: tea.KeyDelete})
	if s.Query != "x" {
		t.Errorf("Query after Delete = %q, want x", s.Query)
	}
}

func TestSearchInput_HandleKey_Runes(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()

	s.HandleKey(tea.KeyPressMsg{Code: 'h', Text: "h"})
	s.HandleKey(tea.KeyPressMsg{Code: 'i', Text: "i"})

	if s.Query != "hi" {
		t.Errorf("Query = %q, want hi", s.Query)
	}
	if s.Selected != 0 {
		t.Errorf("Selected after rune input = %d, want 0", s.Selected)
	}
}

func TestSearchInput_HandleKey_UnhandledKey(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "test"

	// Tab or other unhandled key type.
	_, ok := s.HandleKey(tea.KeyPressMsg{Code: tea.KeyTab})
	if ok {
		t.Error("unhandled key should return ok=false")
	}
	if s.Query != "test" {
		t.Errorf("Query should be unchanged, got %q", s.Query)
	}
}

func TestSearchInput_SetResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Selected = 5

	results := []SearchResult{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	s.SetResults(results)

	if len(s.Results) != 3 {
		t.Errorf("Results len = %d, want 3", len(s.Results))
	}
	// Selected was 5 which exceeds new length, should clamp.
	if s.Selected != 2 {
		t.Errorf("Selected after SetResults = %d, want 2", s.Selected)
	}
}

func TestSearchInput_SetResults_Empty(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Selected = 3

	s.SetResults(nil)
	if s.Selected != 0 {
		t.Errorf("Selected after empty SetResults = %d, want 0", s.Selected)
	}
}

func TestSearchInput_SetResults_SelectedWithinRange(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Selected = 1

	s.SetResults([]SearchResult{{Name: "a"}, {Name: "b"}, {Name: "c"}})
	// Selected=1 is within [0, 2], should stay unchanged.
	if s.Selected != 1 {
		t.Errorf("Selected should remain 1, got %d", s.Selected)
	}
}

func TestSearchInput_View_Inactive(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	out := s.View(80)
	if out != "" {
		t.Errorf("inactive View should be empty, got %q", out)
	}
}

func TestSearchInput_View_ActiveNoQuery(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	out := s.View(80)
	if out == "" {
		t.Error("active View should not be empty")
	}
	if !strings.Contains(out, "search:") {
		t.Error("View should contain search prompt")
	}
}

func TestSearchInput_View_WithQueryNoResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "foobar"
	out := s.View(80)
	if !strings.Contains(out, "foobar") {
		t.Error("View should contain the query text")
	}
	if !strings.Contains(out, "No results") {
		t.Error("View should show 'No results' when query present but results empty")
	}
}

func TestSearchInput_View_WithResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "test"
	s.Results = []SearchResult{
		{Type: SearchTypeRepo, Name: "my-repo", Score: 95},
		{Type: SearchTypeSession, Name: "sess-1", Score: 80},
	}
	out := s.View(80)
	if !strings.Contains(out, "my-repo") {
		t.Error("View should contain result name 'my-repo'")
	}
	if !strings.Contains(out, "sess-1") {
		t.Error("View should contain result name 'sess-1'")
	}
}

func TestSearchInput_View_TruncatesResults(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	s.Query = "test"

	// Create more results than maxShow (12).
	results := make([]SearchResult, 15)
	for i := range results {
		results[i] = SearchResult{Type: SearchTypeRepo, Name: "repo"}
	}
	s.Results = results

	out := s.View(80)
	if !strings.Contains(out, "and 3 more") {
		t.Error("View should indicate truncated results")
	}
}

func TestSearchInput_View_NarrowWidth(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	// Minimum width handling: barWidth < 20 gets clamped to 20.
	out := s.View(10)
	if out == "" {
		t.Error("View with narrow width should still render")
	}
}

func TestSearchInput_View_HelpText(t *testing.T) {
	t.Parallel()
	s := NewSearchInput()
	s.Activate()
	out := s.View(80)
	if !strings.Contains(out, "Esc") {
		t.Error("View should contain help text mentioning Esc")
	}
}
