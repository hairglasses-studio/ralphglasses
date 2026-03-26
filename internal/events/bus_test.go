package events

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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

func TestSubscribeFiltered_OnlyMatchingEvents(t *testing.T) {
	bus := NewBus(100)
	ch := bus.SubscribeFiltered("filtered", SessionStarted)

	bus.Publish(Event{Type: SessionStarted, RepoName: "match"})
	bus.Publish(Event{Type: CostUpdate, RepoName: "skip"})
	bus.Publish(Event{Type: SessionStarted, RepoName: "match2"})

	// Should receive exactly the two SessionStarted events
	for _, want := range []string{"match", "match2"} {
		select {
		case e := <-ch:
			if e.Type != SessionStarted {
				t.Errorf("type = %q, want %q", e.Type, SessionStarted)
			}
			if e.RepoName != want {
				t.Errorf("repo = %q, want %q", e.RepoName, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %q", want)
		}
	}

	// Channel should be empty — CostUpdate was filtered out
	select {
	case e := <-ch:
		t.Errorf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestSubscribeFiltered_MultipleTypes(t *testing.T) {
	bus := NewBus(100)
	ch := bus.SubscribeFiltered("multi", SessionStarted, CostUpdate)

	bus.Publish(Event{Type: SessionStarted, RepoName: "a"})
	bus.Publish(Event{Type: LoopStarted, RepoName: "b"})
	bus.Publish(Event{Type: CostUpdate, RepoName: "c"})

	received := 0
	for i := 0; i < 2; i++ {
		select {
		case e := <-ch:
			if e.Type != SessionStarted && e.Type != CostUpdate {
				t.Errorf("unexpected type %q", e.Type)
			}
			received++
		case <-time.After(time.Second):
			t.Fatal("timeout")
		}
	}
	if received != 2 {
		t.Errorf("received %d events, want 2", received)
	}

	// LoopStarted should not appear
	select {
	case e := <-ch:
		t.Errorf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestSubscribeFiltered_Unsubscribe(t *testing.T) {
	bus := NewBus(100)
	ch := bus.SubscribeFiltered("f1", SessionStarted)
	bus.Unsubscribe("f1")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestStartAsyncIdempotent(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "idem_events.jsonl")

	bus := NewBus(100)
	bus.AsyncWrites = true
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}

	// Call StartAsync twice — should not panic or leak goroutines
	bus.StartAsync()
	bus.StartAsync()

	bus.Publish(Event{Type: SessionStarted, RepoName: "test"})

	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := LoadEvents(persistPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Errorf("expected 1 event, got %d", len(events))
	}
}

func TestEventTTL(t *testing.T) {
	bus := NewBus(100)
	bus.SetRetentionTTL(100 * time.Millisecond)

	// Publish events with explicit old timestamps
	oldTime := time.Now().Add(-200 * time.Millisecond)
	bus.Publish(Event{Type: SessionStarted, Timestamp: oldTime, RepoName: "old1"})
	bus.Publish(Event{Type: SessionStarted, Timestamp: oldTime, RepoName: "old2"})

	// Wait to ensure TTL window passes
	time.Sleep(150 * time.Millisecond)

	// Publish a fresh event — this triggers TTL trimming
	bus.Publish(Event{Type: SessionEnded, RepoName: "fresh"})

	history := bus.History("", 100)
	if len(history) != 1 {
		t.Fatalf("expected 1 event after TTL trim, got %d", len(history))
	}
	if history[0].RepoName != "fresh" {
		t.Errorf("expected fresh event, got %q", history[0].RepoName)
	}
}

func TestSubscribeFiltered(t *testing.T) {
	bus := NewBus(100)
	ch := bus.SubscribeFiltered("test-filter", SessionStarted)

	bus.Publish(Event{Type: SessionStarted, RepoName: "yes1"})
	bus.Publish(Event{Type: CostUpdate, RepoName: "no"})
	bus.Publish(Event{Type: LoopStarted, RepoName: "no2"})
	bus.Publish(Event{Type: SessionStarted, RepoName: "yes2"})

	// Should receive only the two SessionStarted events
	for _, want := range []string{"yes1", "yes2"} {
		select {
		case e := <-ch:
			if e.Type != SessionStarted {
				t.Errorf("type = %q, want %q", e.Type, SessionStarted)
			}
			if e.RepoName != want {
				t.Errorf("repo = %q, want %q", e.RepoName, want)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for event %q", want)
		}
	}

	// Channel should be empty
	select {
	case e := <-ch:
		t.Errorf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestWorkerEventTypes(t *testing.T) {
	for _, et := range []EventType{WorkerDeregistered, WorkerPaused, WorkerResumed} {
		if !ValidEventType(et) {
			t.Errorf("ValidEventType(%q) = false, want true", et)
		}
	}
}

func TestValidEventType_Known(t *testing.T) {
	knownTypes := []EventType{
		SessionStarted, SessionEnded, SessionStopped,
		CostUpdate, BudgetExceeded,
		LoopStarted, LoopStopped, LoopIterated, LoopRegression,
		TeamCreated, JournalWritten, ConfigChanged, ScanComplete,
		PromptEnhanced, ToolCalled, SessionError,
		AutoOptimized, ProviderSelected, SessionRecovered, ContextConflict,
		ProviderHealthChanged, SelfImproveMerged, SelfImprovePR,
		WorkerDeregistered, WorkerPaused, WorkerResumed,
	}
	for _, et := range knownTypes {
		if !ValidEventType(et) {
			t.Errorf("ValidEventType(%q) = false, want true", et)
		}
	}
}

func TestValidEventType_Unknown(t *testing.T) {
	unknownTypes := []EventType{
		"bogus.event",
		"session.unknown",
		"",
	}
	for _, et := range unknownTypes {
		if ValidEventType(et) {
			t.Errorf("ValidEventType(%q) = true, want false", et)
		}
	}
}

func TestPublishUnknownEventType_StillDelivers(t *testing.T) {
	bus := NewBus(100)
	ch := bus.Subscribe("test")

	// Publishing an unknown type should still deliver the event
	bus.Publish(Event{Type: "bogus.unknown", RepoName: "delivered"})

	select {
	case e := <-ch:
		if e.RepoName != "delivered" {
			t.Errorf("repo = %q, want delivered", e.RepoName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout — unknown event type was not delivered")
	}

	// It should also appear in history
	history := bus.History("", 10)
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Type != "bogus.unknown" {
		t.Errorf("history type = %q, want bogus.unknown", history[0].Type)
	}
}

func TestVersionFieldDefaulted(t *testing.T) {
	bus := NewBus(100)
	ch := bus.Subscribe("test")

	// Version not set — should default to 1
	bus.Publish(Event{Type: SessionStarted})

	select {
	case e := <-ch:
		if e.Version != 1 {
			t.Errorf("version = %d, want 1", e.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	history := bus.History("", 10)
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Version != 1 {
		t.Errorf("history version = %d, want 1", history[0].Version)
	}
}

func TestVersionFieldPreserved(t *testing.T) {
	bus := NewBus(100)
	ch := bus.Subscribe("test")

	// Explicitly set Version to 2
	bus.Publish(Event{Type: SessionStarted, Version: 2})

	select {
	case e := <-ch:
		if e.Version != 2 {
			t.Errorf("version = %d, want 2", e.Version)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}
}

func TestVersionFieldPersisted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(path); err != nil {
		t.Fatalf("PersistTo: %v", err)
	}
	defer bus.Close()

	bus.Publish(Event{Type: SessionStarted})
	bus.Publish(Event{Type: CostUpdate, Version: 3})

	events, err := LoadEvents(path, 0)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Version != 1 {
		t.Errorf("event[0].Version = %d, want 1 (defaulted)", events[0].Version)
	}
	if events[1].Version != 3 {
		t.Errorf("event[1].Version = %d, want 3 (preserved)", events[1].Version)
	}
}

func TestPublishPersistWriteError(t *testing.T) {
	dir := t.TempDir()
	roDir := filepath.Join(dir, "readonly")
	if err := os.MkdirAll(roDir, 0755); err != nil {
		t.Fatal(err)
	}

	bus := NewBus(10)
	persistPath := filepath.Join(roDir, "events.jsonl")
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}

	// Replace the persist file with a read-only file descriptor to trigger write errors
	bus.mu.Lock()
	bus.persistFile.Close()
	f, err := os.Open(persistPath)
	if err != nil {
		bus.mu.Unlock()
		t.Fatal(err)
	}
	bus.persistFile = f
	bus.mu.Unlock()

	// Capture slog output
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	bus.Publish(Event{Type: SessionStarted})

	_ = bus.Close()

	logged := buf.String()
	if !strings.Contains(logged, "event persist failed") {
		t.Errorf("expected 'event persist failed' in log output, got: %s", logged)
	}
}

func TestAsyncWriteMode(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "async_events.jsonl")

	bus := NewBus(100)
	bus.AsyncWrites = true
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}
	bus.StartAsync()

	numEvents := 20
	for i := 0; i < numEvents; i++ {
		bus.Publish(Event{Type: LoopIterated, Data: map[string]any{"i": i}})
	}

	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatal(err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != numEvents {
		t.Errorf("expected %d persisted events, got %d", numEvents, len(lines))
	}

	for i, line := range lines {
		var ev Event
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Errorf("line %d: invalid JSON: %v", i, err)
		}
		if ev.Type != LoopIterated {
			t.Errorf("line %d: expected type %s, got %s", i, LoopIterated, ev.Type)
		}
	}
}

func TestAsyncWriteClose(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "drain_events.jsonl")

	bus := NewBus(100)
	bus.AsyncWrites = true
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}
	bus.StartAsync()

	bus.Publish(Event{Type: SessionStarted})

	done := make(chan struct{})
	go func() {
		bus.Close()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Close() did not return within 5 seconds - possible deadlock")
	}
}

func TestAsyncDrainOnClose(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "drain100_events.jsonl")

	bus := NewBus(200)
	bus.AsyncWrites = true
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}
	bus.StartAsync()

	const total = 100
	for i := 0; i < total; i++ {
		bus.Publish(Event{Type: LoopIterated, Data: map[string]any{"i": i}})
	}

	// Close must drain all pending writes before returning
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := LoadEvents(persistPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != total {
		t.Fatalf("expected %d persisted events after drain, got %d", total, len(events))
	}
}

func TestPublishSetsDefaultVersion(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "version_events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}

	// Publish with Version=0 (zero value)
	bus.Publish(Event{Type: SessionStarted})

	// Verify in-memory history has Version=1
	history := bus.History("", 10)
	if len(history) != 1 {
		t.Fatalf("history len = %d, want 1", len(history))
	}
	if history[0].Version != 1 {
		t.Errorf("in-memory version = %d, want 1", history[0].Version)
	}

	bus.Close()

	// Verify persisted event also has Version=1
	events, err := LoadEvents(persistPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected 1 persisted event, got %d", len(events))
	}
	if events[0].Version != 1 {
		t.Errorf("persisted version = %d, want 1", events[0].Version)
	}
}

func TestPublishWarnsUnknownType(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	oldLogger := slog.Default()
	slog.SetDefault(slog.New(handler))
	defer slog.SetDefault(oldLogger)

	bus := NewBus(100)
	bus.Publish(Event{Type: "totally.bogus"})

	logged := buf.String()
	if !strings.Contains(logged, "unknown event type published") {
		t.Errorf("expected 'unknown event type published' in log output, got: %s", logged)
	}
	if !strings.Contains(logged, "totally.bogus") {
		t.Errorf("expected event type in log output, got: %s", logged)
	}
}

func TestCloseFlushes(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "flush_events.jsonl")

	bus := NewBus(200)
	bus.AsyncWrites = true
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}
	bus.StartAsync()

	const total = 50
	for i := 0; i < total; i++ {
		bus.Publish(Event{Type: LoopIterated, Data: map[string]any{"i": i}})
	}

	// Close must drain all pending async writes
	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}

	events, err := LoadEvents(persistPath, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != total {
		t.Fatalf("expected %d persisted events after close, got %d", total, len(events))
	}
}

func TestSyncWriteStillWorks(t *testing.T) {
	dir := t.TempDir()
	persistPath := filepath.Join(dir, "sync_events.jsonl")

	bus := NewBus(100)
	if err := bus.PersistTo(persistPath); err != nil {
		t.Fatal(err)
	}

	bus.Publish(Event{Type: SessionStarted})
	bus.Publish(Event{Type: SessionEnded})

	if err := bus.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(persistPath)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Errorf("expected 2 persisted events, got %d", len(lines))
	}
}
