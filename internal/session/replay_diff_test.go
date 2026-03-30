package session

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func makeEvents(sessionID string, types []ReplayEventType, data []string, baseTime time.Time, intervals []time.Duration) []ReplayEvent {
	events := make([]ReplayEvent, len(types))
	ts := baseTime
	for i := range types {
		if i > 0 && i-1 < len(intervals) {
			ts = ts.Add(intervals[i-1])
		}
		d := ""
		if i < len(data) {
			d = data[i]
		}
		events[i] = ReplayEvent{
			Timestamp: ts,
			Type:      types[i],
			Data:      d,
			SessionID: sessionID,
		}
	}
	return events
}

func TestDiffSessions_Identical(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	events := makeEvents("sess-a", []ReplayEventType{ReplayInput, ReplayOutput, ReplayTool},
		[]string{"hello", "world", "tool-call"},
		base, []time.Duration{time.Second, time.Second})

	a := NewPlayerFromEvents(events)
	// Clone events with different session ID.
	eventsB := make([]ReplayEvent, len(events))
	copy(eventsB, events)
	for i := range eventsB {
		eventsB[i].SessionID = "sess-b"
	}
	b := NewPlayerFromEvents(eventsB)

	result, err := DiffSessions(a, b)
	if err != nil {
		t.Fatal(err)
	}

	if result.Similarity != 1.0 {
		t.Errorf("similarity = %f, want 1.0", result.Similarity)
	}
	if result.Matched != 3 {
		t.Errorf("matched = %d, want 3", result.Matched)
	}
	if result.Modified != 0 {
		t.Errorf("modified = %d, want 0", result.Modified)
	}
	if result.OnlyA != 0 {
		t.Errorf("only_a = %d, want 0", result.OnlyA)
	}
	if result.OnlyB != 0 {
		t.Errorf("only_b = %d, want 0", result.OnlyB)
	}
	if result.SessionIDA != "sess-a" {
		t.Errorf("session_id_a = %q, want %q", result.SessionIDA, "sess-a")
	}
	if result.SessionIDB != "sess-b" {
		t.Errorf("session_id_b = %q, want %q", result.SessionIDB, "sess-b")
	}
}

func TestDiffSessions_CompletelyDifferent(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	evA := makeEvents("a", []ReplayEventType{ReplayInput, ReplayOutput},
		[]string{"alpha", "beta"}, base, []time.Duration{time.Second})
	evB := makeEvents("b", []ReplayEventType{ReplayTool, ReplayStatus},
		[]string{"gamma", "delta"}, base, []time.Duration{time.Second})

	result, err := DiffSessions(NewPlayerFromEvents(evA), NewPlayerFromEvents(evB))
	if err != nil {
		t.Fatal(err)
	}

	if result.Similarity != 0.0 {
		t.Errorf("similarity = %f, want 0.0", result.Similarity)
	}
	if result.Matched != 0 {
		t.Errorf("matched = %d, want 0", result.Matched)
	}
	if result.OnlyA != 2 {
		t.Errorf("only_a = %d, want 2", result.OnlyA)
	}
	if result.OnlyB != 2 {
		t.Errorf("only_b = %d, want 2", result.OnlyB)
	}
}

func TestDiffSessions_PartialOverlap(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	evA := makeEvents("a",
		[]ReplayEventType{ReplayInput, ReplayOutput, ReplayTool},
		[]string{"shared-input", "response-a", "tool-a"},
		base, []time.Duration{time.Second, time.Second})
	evB := makeEvents("b",
		[]ReplayEventType{ReplayInput, ReplayOutput, ReplayStatus},
		[]string{"shared-input", "response-b", "status-b"},
		base, []time.Duration{time.Second, time.Second})

	result, err := DiffSessions(NewPlayerFromEvents(evA), NewPlayerFromEvents(evB))
	if err != nil {
		t.Fatal(err)
	}

	// Input events match (same type, same offset, same data).
	// Output events match by type+offset but differ in data → modified.
	// Tool vs Status → different types → only_a and only_b.
	if result.Matched != 1 {
		t.Errorf("matched = %d, want 1", result.Matched)
	}
	if result.Modified != 1 {
		t.Errorf("modified = %d, want 1", result.Modified)
	}
	if result.OnlyA != 1 {
		t.Errorf("only_a = %d, want 1", result.OnlyA)
	}
	if result.OnlyB != 1 {
		t.Errorf("only_b = %d, want 1", result.OnlyB)
	}
	// Similarity = matched / max(3,3) = 1/3 ≈ 0.333
	if result.Similarity < 0.33 || result.Similarity > 0.34 {
		t.Errorf("similarity = %f, want ~0.333", result.Similarity)
	}
}

func TestDiffSessions_BothEmpty(t *testing.T) {
	a := NewPlayerFromEvents(nil)
	b := NewPlayerFromEvents(nil)

	result, err := DiffSessions(a, b)
	if err != nil {
		t.Fatal(err)
	}

	if result.Similarity != 1.0 {
		t.Errorf("similarity = %f, want 1.0 for two empty sessions", result.Similarity)
	}
	if result.TotalA != 0 || result.TotalB != 0 {
		t.Errorf("totals = (%d, %d), want (0, 0)", result.TotalA, result.TotalB)
	}
	if len(result.Events) != 0 {
		t.Errorf("events = %d, want 0", len(result.Events))
	}
}

func TestDiffSessions_OneEmpty(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	evA := makeEvents("a", []ReplayEventType{ReplayInput}, []string{"hello"}, base, nil)

	t.Run("A empty", func(t *testing.T) {
		result, err := DiffSessions(NewPlayerFromEvents(nil), NewPlayerFromEvents(evA))
		if err != nil {
			t.Fatal(err)
		}
		if result.Similarity != 0.0 {
			t.Errorf("similarity = %f, want 0.0", result.Similarity)
		}
		if result.OnlyB != 1 {
			t.Errorf("only_b = %d, want 1", result.OnlyB)
		}
	})

	t.Run("B empty", func(t *testing.T) {
		result, err := DiffSessions(NewPlayerFromEvents(evA), NewPlayerFromEvents(nil))
		if err != nil {
			t.Fatal(err)
		}
		if result.Similarity != 0.0 {
			t.Errorf("similarity = %f, want 0.0", result.Similarity)
		}
		if result.OnlyA != 1 {
			t.Errorf("only_a = %d, want 1", result.OnlyA)
		}
	})
}

func TestDiffSessions_DifferentLengths(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	evA := makeEvents("a",
		[]ReplayEventType{ReplayInput, ReplayOutput, ReplayTool, ReplayOutput, ReplayStatus},
		[]string{"q1", "r1", "tool1", "r2", "done"},
		base, []time.Duration{time.Second, time.Second, time.Second, time.Second})

	evB := makeEvents("b",
		[]ReplayEventType{ReplayInput, ReplayOutput},
		[]string{"q1", "r1"},
		base, []time.Duration{time.Second})

	result, err := DiffSessions(NewPlayerFromEvents(evA), NewPlayerFromEvents(evB))
	if err != nil {
		t.Fatal(err)
	}

	if result.TotalA != 5 {
		t.Errorf("total_a = %d, want 5", result.TotalA)
	}
	if result.TotalB != 2 {
		t.Errorf("total_b = %d, want 2", result.TotalB)
	}
	if result.Matched != 2 {
		t.Errorf("matched = %d, want 2", result.Matched)
	}
	if result.OnlyA != 3 {
		t.Errorf("only_a = %d, want 3", result.OnlyA)
	}
	if result.OnlyB != 0 {
		t.Errorf("only_b = %d, want 0", result.OnlyB)
	}
	// Similarity = 2 / max(5,2) = 2/5 = 0.4
	if result.Similarity < 0.39 || result.Similarity > 0.41 {
		t.Errorf("similarity = %f, want 0.4", result.Similarity)
	}
}

func TestDiffSessions_TimeTolerance(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	evA := makeEvents("a", []ReplayEventType{ReplayInput}, []string{"hello"}, base, nil)
	// B has the same event but shifted 3 seconds later.
	evB := makeEvents("b", []ReplayEventType{ReplayInput}, []string{"hello"}, base.Add(3*time.Second), nil)

	t.Run("within tolerance", func(t *testing.T) {
		// Both start at offset 0 within their own session, so they always match
		// regardless of absolute time difference (offset-based alignment).
		result, err := DiffSessionsWithTolerance(
			NewPlayerFromEvents(evA), NewPlayerFromEvents(evB), 5*time.Second)
		if err != nil {
			t.Fatal(err)
		}
		if result.Matched != 1 {
			t.Errorf("matched = %d, want 1", result.Matched)
		}
	})
}

func TestDiffSessions_NilPlayer(t *testing.T) {
	a := NewPlayerFromEvents(nil)
	_, err := DiffSessions(nil, a)
	if err == nil {
		t.Error("expected error for nil player A")
	}
	_, err = DiffSessions(a, nil)
	if err == nil {
		t.Error("expected error for nil player B")
	}
}

func TestFormatDiffMarkdown(t *testing.T) {
	diff := &DiffResult{
		SessionIDA: "a",
		SessionIDB: "b",
		TotalA:     2,
		TotalB:     2,
		Matched:    1,
		Modified:   1,
		Similarity: 0.5,
		Events: []DiffEvent{
			{
				Status:  DiffMatched,
				EventA:  &ReplayEvent{Type: ReplayInput, Data: "hello"},
				EventB:  &ReplayEvent{Type: ReplayInput, Data: "hello"},
				OffsetA: 0,
				OffsetB: 0,
			},
			{
				Status:  DiffModified,
				EventA:  &ReplayEvent{Type: ReplayOutput, Data: "response-a"},
				EventB:  &ReplayEvent{Type: ReplayOutput, Data: "response-b"},
				OffsetA: time.Second,
				OffsetB: time.Second,
			},
		},
	}

	var buf bytes.Buffer
	if err := FormatDiffMarkdown(diff, &buf); err != nil {
		t.Fatal(err)
	}

	md := buf.String()

	checks := []string{
		"# Session Replay Diff",
		"Session A",
		"Session B",
		"Matched | 1",
		"Modified | 1",
		"50.0%",
		"MATCHED",
		"MODIFIED",
		"response-a",
		"response-b",
	}
	for _, check := range checks {
		if !strings.Contains(md, check) {
			t.Errorf("markdown missing %q", check)
		}
	}
}

func TestFormatDiffMarkdown_Empty(t *testing.T) {
	diff := &DiffResult{
		Similarity: 1.0,
	}
	var buf bytes.Buffer
	if err := FormatDiffMarkdown(diff, &buf); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "No events to compare") {
		t.Error("expected 'No events to compare' for empty diff")
	}
}

func TestFormatDiffMarkdown_OnlyAB(t *testing.T) {
	diff := &DiffResult{
		SessionIDA: "a",
		SessionIDB: "b",
		TotalA:     1,
		TotalB:     1,
		OnlyA:      1,
		OnlyB:      1,
		Events: []DiffEvent{
			{
				Status: DiffOnlyA,
				EventA: &ReplayEvent{Type: ReplayInput, Data: "only-in-a"},
			},
			{
				Status: DiffOnlyB,
				EventB: &ReplayEvent{Type: ReplayOutput, Data: "only-in-b"},
			},
		},
	}

	var buf bytes.Buffer
	if err := FormatDiffMarkdown(diff, &buf); err != nil {
		t.Fatal(err)
	}

	md := buf.String()
	if !strings.Contains(md, "ONLY IN A") {
		t.Error("missing ONLY IN A")
	}
	if !strings.Contains(md, "ONLY IN B") {
		t.Error("missing ONLY IN B")
	}
	if !strings.Contains(md, "only-in-a") {
		t.Error("missing only-in-a data")
	}
	if !strings.Contains(md, "only-in-b") {
		t.Error("missing only-in-b data")
	}
}

func TestNewPlayerFromEvents(t *testing.T) {
	events := []ReplayEvent{
		{Type: ReplayInput, Data: "test", Timestamp: time.Now()},
	}
	p := NewPlayerFromEvents(events)
	if len(p.Events()) != 1 {
		t.Fatalf("expected 1 event, got %d", len(p.Events()))
	}
	// Verify it's a copy — mutating original should not affect player.
	events[0].Data = "mutated"
	if p.Events()[0].Data != "test" {
		t.Error("NewPlayerFromEvents should copy events")
	}
}
