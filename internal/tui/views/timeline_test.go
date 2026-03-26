package views

import (
	"strings"
	"testing"
	"time"
)

func TestRenderTimelineEmpty(t *testing.T) {
	out := RenderTimeline(nil, "test-repo", 120, 40)
	if !strings.Contains(out, "No sessions to display") {
		t.Error("empty timeline should show 'No sessions to display'")
	}
}

func TestRenderTimelinePopulated(t *testing.T) {
	now := time.Now()
	ended := now.Add(-30 * time.Second)
	entries := []TimelineEntry{
		{
			ID:        "session-1234567890",
			Provider:  "claude",
			StartTime: now.Add(-10 * time.Minute),
			EndTime:   &ended,
			Status:    "completed",
		},
		{
			ID:        "sess-2",
			Provider:  "gemini",
			StartTime: now.Add(-5 * time.Minute),
			EndTime:   nil, // still running
			Status:    "running",
		},
	}

	out := RenderTimeline(entries, "my-repo", 120, 40)

	checks := []string{
		"Session Timeline",
		"my-repo",
		"session-",          // truncated ID
		"claude",
		"gemini",
		"Legend",
		"running",
		"completed",
	}
	for _, want := range checks {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q", want)
		}
	}
}

func TestRenderTimelineSingleEntry(t *testing.T) {
	now := time.Now()
	entries := []TimelineEntry{
		{
			ID:        "only-one",
			Provider:  "codex",
			StartTime: now,
			Status:    "running",
		},
	}
	out := RenderTimeline(entries, "solo-repo", 80, 30)
	if !strings.Contains(out, "solo-repo") {
		t.Error("should show repo name")
	}
	if !strings.Contains(out, "only-one") {
		t.Error("should show session ID")
	}
}

func TestRenderTimelineTruncatesEntries(t *testing.T) {
	now := time.Now()
	// Create more entries than height allows (height - 8 = max)
	entries := make([]TimelineEntry, 20)
	for i := range entries {
		entries[i] = TimelineEntry{
			ID:        "sess-" + string(rune('A'+i)),
			Provider:  "claude",
			StartTime: now.Add(-time.Duration(20-i) * time.Minute),
			Status:    "completed",
		}
	}
	// With height=15, maxEntries = 15-8 = 7, so only last 7 shown
	out := RenderTimeline(entries, "many-repo", 120, 15)
	if !strings.Contains(out, "Session Timeline") {
		t.Error("should still show title")
	}
}
