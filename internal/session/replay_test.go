package session

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRecorderAndPlayer(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	// Record events.
	rec := NewRecorder("sess-1", path)
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

	events := []ReplayEvent{
		{Timestamp: t0, Type: ReplayInput, Data: "hello"},
		{Timestamp: t0.Add(1 * time.Second), Type: ReplayOutput, Data: "world"},
		{Timestamp: t0.Add(3 * time.Second), Type: ReplayTool, Data: "run_tests"},
		{Timestamp: t0.Add(5 * time.Second), Type: ReplayStatus, Data: "completed"},
	}
	for _, ev := range events {
		if err := rec.Record(ev); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Load and verify.
	player, err := NewPlayer(path)
	if err != nil {
		t.Fatalf("NewPlayer: %v", err)
	}

	got := player.Events()
	if len(got) != len(events) {
		t.Fatalf("got %d events, want %d", len(got), len(events))
	}
	for i, ev := range got {
		if ev.SessionID != "sess-1" {
			t.Errorf("event %d: SessionID = %q, want %q", i, ev.SessionID, "sess-1")
		}
		if ev.Type != events[i].Type {
			t.Errorf("event %d: Type = %q, want %q", i, ev.Type, events[i].Type)
		}
		if ev.Data != events[i].Data {
			t.Errorf("event %d: Data = %q, want %q", i, ev.Data, events[i].Data)
		}
	}
}

func TestPlayerDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-dur", path)
	t0 := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	_ = rec.Record(ReplayEvent{Timestamp: t0, Type: ReplayInput, Data: "a"})
	_ = rec.Record(ReplayEvent{Timestamp: t0.Add(10 * time.Second), Type: ReplayOutput, Data: "b"})
	_ = rec.Close()

	p, err := NewPlayer(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := p.Duration(); d != 10*time.Second {
		t.Errorf("Duration = %v, want 10s", d)
	}
}

func TestPlayerDurationFewEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-0", path)
	_ = rec.Record(ReplayEvent{Timestamp: time.Now(), Type: ReplayInput, Data: "only"})
	_ = rec.Close()

	p, _ := NewPlayer(path)
	if d := p.Duration(); d != 0 {
		t.Errorf("Duration with 1 event = %v, want 0", d)
	}
}

func TestPlayerSearch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-s", path)
	now := time.Now()
	_ = rec.Record(ReplayEvent{Timestamp: now, Type: ReplayInput, Data: "Fix the BUG in parser"})
	_ = rec.Record(ReplayEvent{Timestamp: now, Type: ReplayOutput, Data: "done"})
	_ = rec.Record(ReplayEvent{Timestamp: now, Type: ReplayTool, Data: "lint tool"})
	_ = rec.Close()

	p, _ := NewPlayer(path)

	// Case-insensitive search.
	results := p.Search("bug")
	if len(results) != 1 {
		t.Fatalf("Search(bug) got %d results, want 1", len(results))
	}
	if results[0].Data != "Fix the BUG in parser" {
		t.Errorf("unexpected match: %q", results[0].Data)
	}

	// No match.
	if got := p.Search("nonexistent"); len(got) != 0 {
		t.Errorf("Search(nonexistent) got %d results, want 0", len(got))
	}
}

func TestPlayerPlay(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-p", path)
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = rec.Record(ReplayEvent{Timestamp: t0, Type: ReplayInput, Data: "a"})
	_ = rec.Record(ReplayEvent{Timestamp: t0.Add(100 * time.Millisecond), Type: ReplayOutput, Data: "b"})
	_ = rec.Record(ReplayEvent{Timestamp: t0.Add(200 * time.Millisecond), Type: ReplayTool, Data: "c"})
	_ = rec.Close()

	p, _ := NewPlayer(path)

	// Instant replay (speed <= 0).
	var collected []string
	err := p.Play(context.Background(), 0, func(ev ReplayEvent) {
		collected = append(collected, ev.Data)
	})
	if err != nil {
		t.Fatalf("Play: %v", err)
	}
	if len(collected) != 3 {
		t.Fatalf("got %d events, want 3", len(collected))
	}
}

func TestPlayerPlayCancellation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-cancel", path)
	t0 := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 10 {
		_ = rec.Record(ReplayEvent{
			Timestamp: t0.Add(time.Duration(i) * time.Second),
			Type:      ReplayOutput,
			Data:      "event",
		})
	}
	_ = rec.Close()

	p, _ := NewPlayer(path)

	ctx, cancel := context.WithCancel(context.Background())
	var count int
	err := p.Play(ctx, 0.001, func(_ ReplayEvent) {
		count++
		if count >= 2 {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("expected context error, got nil")
	}
}

func TestRecorderConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-race", path)
	defer rec.Close()

	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = rec.Record(ReplayEvent{
				Timestamp: time.Now(),
				Type:      ReplayOutput,
				Data:      "msg",
			})
		}(i)
	}
	wg.Wait()

	p, err := NewPlayer(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Events()) != 20 {
		t.Errorf("got %d events, want 20", len(p.Events()))
	}
}

func TestRecorderAutoTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "replay.jsonl")

	rec := NewRecorder("sess-ts", path)
	before := time.Now()
	_ = rec.Record(ReplayEvent{Type: ReplayInput, Data: "auto"})
	_ = rec.Close()

	p, _ := NewPlayer(path)
	ev := p.Events()[0]
	if ev.Timestamp.Before(before) {
		t.Error("auto-timestamp is before Record call")
	}
}

func TestNewPlayerMissingFile(t *testing.T) {
	_, err := NewPlayer("/nonexistent/path/replay.jsonl")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestNewPlayerEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.jsonl")
	_ = os.WriteFile(path, nil, 0o644)

	p, err := NewPlayer(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(p.Events()) != 0 {
		t.Errorf("got %d events, want 0", len(p.Events()))
	}
	if p.Duration() != 0 {
		t.Error("expected zero duration for empty replay")
	}
}

func TestNewPlayerBadJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.jsonl")
	_ = os.WriteFile(path, []byte("not json\n"), 0o644)

	_, err := NewPlayer(path)
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}
