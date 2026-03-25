package events

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPublishSubscribe(t *testing.T) {
	bus := NewBus(100)
	ch := bus.Subscribe("test")

	bus.Publish(Event{Type: SessionStarted, RepoName: "repo1"})

	select {
	case e := <-ch:
		if e.Type != SessionStarted {
			t.Errorf("type = %q, want %q", e.Type, SessionStarted)
		}
		if e.RepoName != "repo1" {
			t.Errorf("repo = %q, want repo1", e.RepoName)
		}
		if e.Timestamp.IsZero() {
			t.Error("timestamp should be set automatically")
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestUnsubscribe(t *testing.T) {
	bus := NewBus(100)
	ch := bus.Subscribe("test")
	bus.Unsubscribe("test")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestHistory(t *testing.T) {
	bus := NewBus(100)
	bus.Publish(Event{Type: SessionStarted, RepoName: "a"})
	bus.Publish(Event{Type: SessionEnded, RepoName: "b"})
	bus.Publish(Event{Type: SessionStarted, RepoName: "c"})

	// All events
	all := bus.History("", 10)
	if len(all) != 3 {
		t.Errorf("history len = %d, want 3", len(all))
	}

	// Filtered
	started := bus.History(SessionStarted, 10)
	if len(started) != 2 {
		t.Errorf("filtered len = %d, want 2", len(started))
	}

	// Limited
	limited := bus.History("", 2)
	if len(limited) != 2 {
		t.Errorf("limited len = %d, want 2", len(limited))
	}
	// Should be the 2 most recent in chronological order
	if limited[0].RepoName != "b" || limited[1].RepoName != "c" {
		t.Errorf("limited = %v, want [b, c]", limited)
	}
}

func TestHistorySince(t *testing.T) {
	bus := NewBus(100)

	t1 := time.Now().Add(-2 * time.Second)
	bus.Publish(Event{Type: SessionStarted, Timestamp: t1, RepoName: "old"})
	t2 := time.Now()
	bus.Publish(Event{Type: SessionEnded, Timestamp: t2.Add(time.Millisecond), RepoName: "new"})

	result := bus.HistorySince(t2)
	if len(result) != 1 {
		t.Errorf("since len = %d, want 1", len(result))
	}
	if len(result) > 0 && result[0].RepoName != "new" {
		t.Errorf("since event = %q, want new", result[0].RepoName)
	}
}

func TestHistoryRingBuffer(t *testing.T) {
	bus := NewBus(5)
	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: SessionStarted, RepoName: "r"})
	}
	all := bus.History("", 100)
	if len(all) != 5 {
		t.Errorf("ring buffer len = %d, want 5", len(all))
	}
}

func TestOverflow(t *testing.T) {
	bus := NewBus(100)
	_ = bus.Subscribe("slow")

	// Fill the subscriber channel beyond buffer size
	for i := 0; i < 200; i++ {
		bus.Publish(Event{Type: SessionStarted})
	}
	// Should not panic — overflow events are dropped
}

func TestHistoryAfterCursor(t *testing.T) {
	bus := NewBus(100)
	for i := 0; i < 5; i++ {
		bus.Publish(Event{Type: SessionStarted, RepoName: fmt.Sprintf("r%d", i)})
	}

	// Cursor 0 = get everything
	evts, cursor := bus.HistoryAfterCursor(0, 100)
	if len(evts) != 5 {
		t.Fatalf("len = %d, want 5", len(evts))
	}
	if cursor != 5 {
		t.Fatalf("cursor = %d, want 5", cursor)
	}

	// Publish 3 more
	for i := 5; i < 8; i++ {
		bus.Publish(Event{Type: SessionEnded, RepoName: fmt.Sprintf("r%d", i)})
	}

	// Cursor 5 = get only new events
	evts, cursor = bus.HistoryAfterCursor(5, 100)
	if len(evts) != 3 {
		t.Fatalf("new events = %d, want 3", len(evts))
	}
	if cursor != 8 {
		t.Fatalf("cursor = %d, want 8", cursor)
	}
	if evts[0].RepoName != "r5" {
		t.Errorf("first new event = %q, want r5", evts[0].RepoName)
	}

	// Cursor at current = no new events
	evts, cursor = bus.HistoryAfterCursor(8, 100)
	if len(evts) != 0 {
		t.Fatalf("no new events expected, got %d", len(evts))
	}
	if cursor != 8 {
		t.Fatalf("cursor = %d, want 8", cursor)
	}
}

func TestHistoryAfterCursor_RingOverflow(t *testing.T) {
	bus := NewBus(5)
	// Publish 10 events, ring buffer keeps last 5
	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: SessionStarted, RepoName: fmt.Sprintf("r%d", i)})
	}

	// Cursor 0 = some events dropped, get what's available
	evts, cursor := bus.HistoryAfterCursor(0, 100)
	if len(evts) != 5 {
		t.Fatalf("len = %d, want 5 (ring buffer size)", len(evts))
	}
	if cursor != 10 {
		t.Fatalf("cursor = %d, want 10", cursor)
	}
	// Should get events r5-r9 (oldest 5 dropped)
	if evts[0].RepoName != "r5" {
		t.Errorf("first event = %q, want r5", evts[0].RepoName)
	}

	// Cursor 7 = get events after position 7
	evts, _ = bus.HistoryAfterCursor(7, 100)
	if len(evts) != 3 {
		t.Fatalf("len = %d, want 3", len(evts))
	}
	if evts[0].RepoName != "r7" {
		t.Errorf("first event = %q, want r7", evts[0].RepoName)
	}
}

func TestHistoryAfterCursor_WithLimit(t *testing.T) {
	bus := NewBus(100)
	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: SessionStarted, RepoName: fmt.Sprintf("r%d", i)})
	}

	evts, _ := bus.HistoryAfterCursor(0, 3)
	if len(evts) != 3 {
		t.Fatalf("len = %d, want 3 (limited)", len(evts))
	}
	// Should get first 3 events (r0, r1, r2)
	if evts[0].RepoName != "r0" {
		t.Errorf("first = %q, want r0", evts[0].RepoName)
	}
	if evts[2].RepoName != "r2" {
		t.Errorf("third = %q, want r2", evts[2].RepoName)
	}
}

func TestMultipleSubscribers(t *testing.T) {
	bus := NewBus(100)
	ch1 := bus.Subscribe("s1")
	ch2 := bus.Subscribe("s2")

	bus.Publish(Event{Type: LoopStarted, RepoName: "test"})

	for _, ch := range []<-chan Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Type != LoopStarted {
				t.Errorf("type = %q, want %q", e.Type, LoopStarted)
			}
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
}

func TestBusPersistTo(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}
	defer bus.Close()

	bus.Publish(Event{Type: SessionStarted, SessionID: "s1", Timestamp: time.Now()})
	bus.Publish(Event{Type: CostUpdate, SessionID: "s2", Timestamp: time.Now()})

	events, err := LoadEvents(path, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != SessionStarted {
		t.Errorf("event[0].Type = %q, want %q", events[0].Type, SessionStarted)
	}
	if events[1].SessionID != "s2" {
		t.Errorf("event[1].SessionID = %q, want %q", events[1].SessionID, "s2")
	}
}

func TestBusLoadEvents(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}

	for i := 0; i < 10; i++ {
		bus.Publish(Event{Type: LoopIterated, Timestamp: time.Now()})
	}
	bus.Close()

	events, err := LoadEvents(path, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 10 {
		t.Fatalf("expected 10 events, got %d", len(events))
	}
}

func TestBusLoadEventsLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}

	for i := 0; i < 20; i++ {
		bus.Publish(Event{Type: LoopIterated, Timestamp: time.Now()})
	}
	bus.Close()

	events, err := LoadEvents(path, 5)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d", len(events))
	}
}

func TestBusPersistConcurrent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(1000)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}
	defer bus.Close()

	const goroutines = 10
	const eventsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < eventsPerGoroutine; i++ {
				bus.Publish(Event{Type: LoopIterated, Timestamp: time.Now()})
			}
		}()
	}
	wg.Wait()

	events, err := LoadEvents(path, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	expected := goroutines * eventsPerGoroutine
	if len(events) != expected {
		t.Fatalf("expected %d events, got %d", expected, len(events))
	}
}

func TestBusRotation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(10000)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}
	defer bus.Close()

	// Write a large payload to exceed 10MB quickly
	bigData := make(map[string]any)
	bigData["payload"] = string(make([]byte, 100*1024)) // 100KB per event

	// Need >100 writes (rotation check interval) and >10MB total
	for i := 0; i < 150; i++ {
		bus.Publish(Event{
			Type:      LoopIterated,
			Timestamp: time.Now(),
			Data:      bigData,
		})
	}

	// After rotation, the .1 file should exist
	rotatedPath := path + ".1"
	if _, err := os.Stat(rotatedPath); os.IsNotExist(err) {
		t.Fatal("expected rotated file to exist")
	}

	// The main file should still be writable (new events go here)
	bus.Publish(Event{Type: SessionStarted, Timestamp: time.Now()})
	events, err := LoadEvents(path, 0)
	if err != nil {
		t.Fatalf("LoadEvents on new file: %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected events in new file after rotation")
	}
}

func TestBusLoadEventsFileNotFound(t *testing.T) {
	_, err := LoadEvents("/nonexistent/path/events.jsonl", 0)
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestBusCloseWithoutPersist(t *testing.T) {
	bus := NewBus(100)
	if err := bus.Close(); err != nil {
		t.Fatalf("Close without persist: %v", err)
	}
}

func TestBusExistingBehaviorUnchanged(t *testing.T) {
	bus := NewBus(10)
	bus.Publish(Event{Type: SessionStarted, SessionID: "s1"})
	bus.Publish(Event{Type: CostUpdate, SessionID: "s2"})

	history := bus.History("", 10)
	if len(history) != 2 {
		t.Fatalf("expected 2 history events, got %d", len(history))
	}
}
