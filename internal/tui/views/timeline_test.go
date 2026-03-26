package views

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTimeline_Empty(t *testing.T) {
	out := RenderTimeline(nil, "test-repo", 120, 40)
	if !strings.Contains(out, "No sessions to display") {
		t.Error("empty entries should show 'No sessions to display'")
	}
}

func TestRenderTimeline_SingleEntry(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{
			ID:        "abcdef1234567890",
			Provider:  "claude",
			StartTime: now.Add(-10 * time.Minute),
			EndTime:   nil, // still running
			Status:    "running",
		},
	}

	out := RenderTimeline(entries, "my-repo", 120, 40)

	checks := []string{
		"Session Timeline",
		"my-repo",
		"abcdef12", // truncated ID
		"claude",
		"Legend",
		"running",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderTimeline_MultipleEntries(t *testing.T) {
	now := time.Now()
	endTime := now.Add(-2 * time.Minute)
	entries := []TimelineEntry{
		{ID: "sess0001", Provider: "claude", StartTime: now.Add(-30 * time.Minute), EndTime: &endTime, Status: "completed"},
		{ID: "sess0002", Provider: "gemini", StartTime: now.Add(-15 * time.Minute), EndTime: nil, Status: "running"},
		{ID: "sess0003", Provider: "codex", StartTime: now.Add(-5 * time.Minute), EndTime: nil, Status: "errored"},
	}

	out := RenderTimeline(entries, "repo", 120, 40)

	if !strings.Contains(out, "sess0001") {
		t.Error("should show first session ID")
	}
	if !strings.Contains(out, "sess0002") {
		t.Error("should show second session ID")
	}
	// Should contain block characters for the bars
	if !strings.Contains(out, "\u2588") {
		t.Error("should contain block characters for timeline bars")
	}
}

func TestRenderTimeline_CompletedEntry(t *testing.T) {
	now := time.Now()
	start := now.Add(-20 * time.Minute)
	end := now.Add(-5 * time.Minute)
	entries := []TimelineEntry{
		{ID: "done-sess", Provider: "codex", StartTime: start, EndTime: &end, Status: "completed"},
	}

	out := RenderTimeline(entries, "finished-repo", 100, 40)
	if !strings.Contains(out, "completed") {
		t.Error("legend should include 'completed'")
	}
}

func TestRenderTimeline_NarrowWidth(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{ID: "narrow01", Provider: "claude", StartTime: now.Add(-5 * time.Minute), Status: "running"},
	}

	// Should not panic at narrow width
	out := RenderTimeline(entries, "repo", 40, 20)
	if out == "" {
		t.Error("should produce output even at narrow width")
	}
}

func TestRenderTimeline_TruncatesLongList(t *testing.T) {
	now := time.Now()
	entries := make([]TimelineEntry, 30)
	for i := range entries {
		entries[i] = TimelineEntry{
			ID:        "sess" + string(rune('A'+i%26)),
			Provider:  "claude",
			StartTime: now.Add(-time.Duration(30-i) * time.Minute),
			Status:    "completed",
		}
	}

	// With small height, should truncate the list
	out := RenderTimeline(entries, "repo", 120, 15)
	if out == "" {
		t.Error("should produce output even with many entries")
	}
}
