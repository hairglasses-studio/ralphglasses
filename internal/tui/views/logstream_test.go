package views

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestNewLogView(t *testing.T) {
	lv := NewLogView()
	if !lv.Follow {
		t.Error("new log view should have Follow=true")
	}
}

func TestAppendLines(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 20)
	lv.AppendLines([]string{"line 1", "line 2"})
	if len(lv.Lines()) != 2 {
		t.Errorf("expected 2 lines, got %d", len(lv.Lines()))
	}
	lv.AppendLines([]string{"line 3"})
	if len(lv.Lines()) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lv.Lines()))
	}
}

func TestSetLines(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 20)
	lv.SetLines([]string{"a", "b", "c"})
	if len(lv.Lines()) != 3 {
		t.Errorf("expected 3, got %d", len(lv.Lines()))
	}
	lv.SetLines([]string{"x"})
	if len(lv.Lines()) != 1 {
		t.Errorf("after replace: expected 1, got %d", len(lv.Lines()))
	}
}

func TestScrollUpDown(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 5)
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	lv.SetLines(lines)
	lv.ScrollUp()
	if lv.Follow {
		t.Error("ScrollUp should disable follow")
	}
}

func TestScrollToEnd(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 5)
	lv.Follow = false
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	lv.SetLines(lines)
	lv.ScrollToEnd()
	if !lv.Follow {
		t.Error("ScrollToEnd should enable follow")
	}
}

func TestScrollToStart(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 20)
	lv.SetLines(make([]string, 50))
	lv.Follow = true
	lv.ScrollToStart()
	if lv.Follow {
		t.Error("ScrollToStart should disable follow")
	}
}

func TestPageUpDown(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 10)
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	lv.SetLines(lines)
	// Viewport starts at bottom (follow mode), so PageUp should move up
	lv.PageUp()
	if lv.Follow {
		t.Error("PageUp should disable follow")
	}
}

func TestToggleFollow(t *testing.T) {
	lv := NewLogView()
	lv.ToggleFollow()
	if lv.Follow {
		t.Error("first toggle should disable")
	}
	lv.ToggleFollow()
	if !lv.Follow {
		t.Error("second toggle should enable")
	}
}

func TestLogView_SearchFilter(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 10)
	lv.SetLines([]string{"ERROR something broke", "INFO all good", "ERROR another issue", "DEBUG trace"})

	lv.Search = "error"
	filtered := lv.filteredLines()
	if len(filtered) != 2 {
		t.Errorf("expected 2 filtered lines, got %d", len(filtered))
	}

	lv.rebuildContent()
	view := lv.View()
	if !strings.Contains(view, "ERROR something broke") {
		t.Error("view should contain matching error line")
	}
	if strings.Contains(view, "INFO all good") {
		t.Error("view should not contain non-matching line")
	}
	if !strings.Contains(view, `Search:`) {
		t.Error("view should show search indicator")
	}
}

func TestLogView_SearchFilterEmpty(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 10)
	lv.SetLines([]string{"line1", "line2", "line3"})

	lv.Search = ""
	filtered := lv.filteredLines()
	if len(filtered) != 3 {
		t.Errorf("expected all 3 lines with empty search, got %d", len(filtered))
	}
}

func TestLogView_SearchFilterCaseInsensitive(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 10)
	lv.SetLines([]string{"Error Message", "info message", "WARNING"})

	lv.Search = "ERROR"
	filtered := lv.filteredLines()
	if len(filtered) != 1 {
		t.Errorf("expected 1 case-insensitive match, got %d", len(filtered))
	}
}

func TestLogViewView(t *testing.T) {
	lv := NewLogView()
	lv.SetDimensions(80, 10)
	lv.SetLines([]string{"hello", "world"})
	view := lv.View()
	if !strings.Contains(view, "hello") {
		t.Error("view should contain 'hello'")
	}
}

func TestLineRing_Capacity(t *testing.T) {
	r := newLineRing(3)
	r.push("a")
	r.push("b")
	r.push("c")
	if !r.isFull() {
		t.Fatal("ring should be full after 3 pushes into cap-3 ring")
	}
	r.push("d") // evicts "a"
	got := r.slice()
	want := []string{"b", "c", "d"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestLogView_BoundedByCapacity(t *testing.T) {
	lv := NewLogViewWithCapacity(5)
	lv.SetDimensions(80, 20)
	for i := 0; i < 20; i++ {
		lv.AppendLines([]string{fmt.Sprintf("line %d", i)})
	}
	if lv.Len() != 5 {
		t.Errorf("expected 5 lines (capped), got %d", lv.Len())
	}
	lines := lv.Lines()
	if lines[0] != "line 15" {
		t.Errorf("expected oldest retained line to be 'line 15', got %q", lines[0])
	}
}

func TestLogView_SetLines_Truncates(t *testing.T) {
	lv := NewLogViewWithCapacity(3)
	lv.SetDimensions(80, 10)
	lv.SetLines([]string{"a", "b", "c", "d", "e"})
	if lv.Len() != 3 {
		t.Errorf("SetLines with more than cap should truncate, got %d", lv.Len())
	}
	lines := lv.Lines()
	if lines[0] != "c" || lines[2] != "e" {
		t.Errorf("expected [c d e], got %v", lines)
	}
}

func TestLogView_EvictionCount(t *testing.T) {
	r := newLineRing(3)
	r.push("a")
	r.push("b")
	r.push("c")
	r.push("d")
	if r.evicted != 1 {
		t.Errorf("expected 1 eviction, got %d", r.evicted)
	}
}

func TestLineRing_Reset_ClearsEvicted(t *testing.T) {
	r := newLineRing(3)
	r.push("a")
	r.push("b")
	r.push("c")
	r.push("d") // evicts "a"
	r.reset()
	if r.evicted != 0 {
		t.Errorf("reset should clear evicted count, got %d", r.evicted)
	}
	if r.len() != 0 {
		t.Errorf("reset should clear size, got %d", r.len())
	}
}
