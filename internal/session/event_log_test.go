package session

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEventLogAppendAndLen(t *testing.T) {
	log := NewSessionEventLog(100)
	if log.Len() != 0 {
		t.Fatalf("new log should be empty, got %d", log.Len())
	}

	log.Append(SessionEvent{Type: EventSessionCreated, SessionID: "s1"})
	log.Append(SessionEvent{Type: EventSessionStarted, SessionID: "s1"})

	if got := log.Len(); got != 2 {
		t.Fatalf("expected 2 events, got %d", got)
	}

	// Verify auto-generated fields.
	events := log.Events()
	for i, e := range events {
		if e.ID == "" {
			t.Errorf("event %d: expected auto-generated ID", i)
		}
		if e.Timestamp.IsZero() {
			t.Errorf("event %d: expected auto-set timestamp", i)
		}
	}
}

func TestEventLogMaxSizeTrimming(t *testing.T) {
	maxSize := 5
	log := NewSessionEventLog(maxSize)

	// Append more events than maxSize.
	for i := range 10 {
		log.Append(SessionEvent{
			Type:      EventToolCall,
			SessionID: "s1",
			Iteration: i,
		})
	}

	if got := log.Len(); got != maxSize {
		t.Fatalf("expected %d events after trimming, got %d", maxSize, got)
	}

	// The oldest events (iterations 0-4) should have been dropped.
	events := log.Events()
	if events[0].Iteration != 5 {
		t.Errorf("expected first event iteration=5 after trim, got %d", events[0].Iteration)
	}
	if events[maxSize-1].Iteration != 9 {
		t.Errorf("expected last event iteration=9, got %d", events[maxSize-1].Iteration)
	}
}

func TestEventLogDefaultMaxSize(t *testing.T) {
	log := NewSessionEventLog(0)
	// Should use default rather than panic or allow zero.
	log.Append(SessionEvent{Type: EventSessionCreated})
	if log.Len() != 1 {
		t.Fatal("append to default-sized log failed")
	}
}

func TestEventLogEventsReturnsCopy(t *testing.T) {
	log := NewSessionEventLog(100)
	log.Append(SessionEvent{Type: EventSessionCreated, SessionID: "s1", Iteration: 0})

	events := log.Events()
	// Mutate the returned slice.
	events[0].Iteration = 999

	// Original should be unaffected.
	original := log.Events()
	if original[0].Iteration != 0 {
		t.Fatalf("Events() returned a reference instead of a copy: iteration changed to %d", original[0].Iteration)
	}
}

func TestEventLogEventsSince(t *testing.T) {
	log := NewSessionEventLog(100)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range 5 {
		log.Append(SessionEvent{
			Type:      EventToolCall,
			SessionID: "s1",
			Timestamp: base.Add(time.Duration(i) * time.Hour),
			Iteration: i,
		})
	}

	// Events since hour 3 (inclusive) should return iterations 3 and 4.
	since := base.Add(3 * time.Hour)
	got := log.EventsSince(since)
	if len(got) != 2 {
		t.Fatalf("expected 2 events since %v, got %d", since, len(got))
	}
	if got[0].Iteration != 3 || got[1].Iteration != 4 {
		t.Errorf("unexpected iterations: %d, %d", got[0].Iteration, got[1].Iteration)
	}
}

func TestEventLogEventsSinceEmpty(t *testing.T) {
	log := NewSessionEventLog(100)
	got := log.EventsSince(time.Now())
	if len(got) != 0 {
		t.Fatalf("expected 0 events from empty log, got %d", len(got))
	}
}

func TestEventLogEventsByType(t *testing.T) {
	log := NewSessionEventLog(100)
	log.Append(SessionEvent{Type: EventSessionCreated, SessionID: "s1"})
	log.Append(SessionEvent{Type: EventToolCall, SessionID: "s1"})
	log.Append(SessionEvent{Type: EventToolResult, SessionID: "s1"})
	log.Append(SessionEvent{Type: EventToolCall, SessionID: "s1"})
	log.Append(SessionEvent{Type: EventCostUpdate, SessionID: "s1"})

	toolCalls := log.EventsByType(EventToolCall)
	if len(toolCalls) != 2 {
		t.Fatalf("expected 2 tool call events, got %d", len(toolCalls))
	}
	for _, e := range toolCalls {
		if e.Type != EventToolCall {
			t.Errorf("unexpected event type: %s", e.Type)
		}
	}

	// No events of this type.
	layerChanges := log.EventsByType(EventLayerChange)
	if len(layerChanges) != 0 {
		t.Fatalf("expected 0 layer change events, got %d", len(layerChanges))
	}
}

func TestEventLogLast(t *testing.T) {
	log := NewSessionEventLog(100)

	for i := range 5 {
		log.Append(SessionEvent{
			Type:      EventToolCall,
			SessionID: "s1",
			Iteration: i,
		})
	}

	tests := []struct {
		name     string
		n        int
		wantLen  int
		wantFirst int // iteration of first returned event
		wantLast  int // iteration of last returned event
	}{
		{"last 3", 3, 3, 2, 4},
		{"last 1", 1, 1, 4, 4},
		{"last 5 (all)", 5, 5, 0, 4},
		{"last 10 (more than available)", 10, 5, 0, 4},
		{"last 0", 0, 0, -1, -1},
		{"last negative", -1, 0, -1, -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := log.Last(tt.n)
			if len(got) != tt.wantLen {
				t.Fatalf("Last(%d): expected %d events, got %d", tt.n, tt.wantLen, len(got))
			}
			if tt.wantLen > 0 {
				if got[0].Iteration != tt.wantFirst {
					t.Errorf("Last(%d): first iteration=%d, want %d", tt.n, got[0].Iteration, tt.wantFirst)
				}
				if got[len(got)-1].Iteration != tt.wantLast {
					t.Errorf("Last(%d): last iteration=%d, want %d", tt.n, got[len(got)-1].Iteration, tt.wantLast)
				}
			}
		})
	}
}

func TestEventLogFormatForContext(t *testing.T) {
	log := NewSessionEventLog(100)

	// Empty log.
	if got := log.FormatForContext(5); got != "<session-events />" {
		t.Errorf("empty log format: got %q", got)
	}

	ts := time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	log.Append(SessionEvent{
		Type:      EventSessionCreated,
		SessionID: "s1",
		Timestamp: ts,
		Iteration: 0,
	})
	log.Append(SessionEvent{
		Type:      EventToolCall,
		SessionID: "s1",
		Timestamp: ts.Add(time.Minute),
		Iteration: 1,
		Data:      map[string]any{"tool": "read_file"},
	})

	got := log.FormatForContext(5)

	if !strings.Contains(got, "<session-events>") {
		t.Error("missing opening tag")
	}
	if !strings.Contains(got, "</session-events>") {
		t.Error("missing closing tag")
	}
	if !strings.Contains(got, `type="session.created"`) {
		t.Error("missing session.created event")
	}
	if !strings.Contains(got, `type="tool.call"`) {
		t.Error("missing tool.call event")
	}
	if !strings.Contains(got, "<tool>read_file</tool>") {
		t.Error("missing tool data")
	}
	if !strings.Contains(got, `iteration=0`) {
		t.Error("missing iteration 0")
	}
}

func TestEventLogFormatForContextLimit(t *testing.T) {
	log := NewSessionEventLog(100)
	for i := range 10 {
		log.Append(SessionEvent{
			Type:      EventToolCall,
			SessionID: "s1",
			Iteration: i,
		})
	}

	got := log.FormatForContext(3)
	// Should contain only the last 3 events (iterations 7, 8, 9).
	count := strings.Count(got, `type="tool.call"`)
	if count != 3 {
		t.Errorf("FormatForContext(3): expected 3 events, found %d", count)
	}
}

func TestEventLogFork(t *testing.T) {
	log := NewSessionEventLog(100)
	for i := range 5 {
		log.Append(SessionEvent{
			Type:      EventToolCall,
			SessionID: "s1",
			Iteration: i,
		})
	}

	// Fork at index 3: should contain events 0, 1, 2.
	forked := log.Fork(3)
	if forked.Len() != 3 {
		t.Fatalf("forked log: expected 3 events, got %d", forked.Len())
	}

	events := forked.Events()
	for i, e := range events {
		if e.Iteration != i {
			t.Errorf("forked event %d: iteration=%d, want %d", i, e.Iteration, i)
		}
	}

	// Forked log should be independent.
	forked.Append(SessionEvent{Type: EventSessionStopped, SessionID: "s1", Iteration: 99})
	if forked.Len() != 4 {
		t.Fatal("append to forked log failed")
	}
	if log.Len() != 5 {
		t.Fatal("append to forked log affected original")
	}
}

func TestEventLogForkEdgeCases(t *testing.T) {
	log := NewSessionEventLog(100)
	log.Append(SessionEvent{Type: EventSessionCreated, Iteration: 0})

	// Fork at 0: empty log.
	f0 := log.Fork(0)
	if f0.Len() != 0 {
		t.Errorf("Fork(0): expected 0 events, got %d", f0.Len())
	}

	// Fork at negative: treated as 0.
	fn := log.Fork(-1)
	if fn.Len() != 0 {
		t.Errorf("Fork(-1): expected 0 events, got %d", fn.Len())
	}

	// Fork beyond length: clamp to len.
	fb := log.Fork(999)
	if fb.Len() != 1 {
		t.Errorf("Fork(999): expected 1 event, got %d", fb.Len())
	}
}

func TestEventLogForkInheritsMaxSize(t *testing.T) {
	log := NewSessionEventLog(50)
	log.Append(SessionEvent{Type: EventSessionCreated, Iteration: 0})
	forked := log.Fork(1)

	// The forked log should respect the same maxSize.
	// Fill it past the limit.
	for i := range 55 {
		forked.Append(SessionEvent{Type: EventToolCall, Iteration: i})
	}
	if forked.Len() != 50 {
		t.Errorf("forked log should enforce maxSize=50, got %d", forked.Len())
	}
}

func TestEventLogConcurrentAppendAndEvents(t *testing.T) {
	log := NewSessionEventLog(1000)
	var wg sync.WaitGroup

	// Concurrent writers.
	for w := range 10 {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for i := range 100 {
				log.Append(SessionEvent{
					Type:      EventToolCall,
					SessionID: "s1",
					Iteration: worker*100 + i,
				})
			}
		}(w)
	}

	// Concurrent readers.
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 50 {
				_ = log.Events()
				_ = log.Len()
				_ = log.Last(10)
				_ = log.EventsByType(EventToolCall)
			}
		}()
	}

	wg.Wait()

	if got := log.Len(); got != 1000 {
		t.Fatalf("expected 1000 events after concurrent writes, got %d", got)
	}
}

func TestEventTypes(t *testing.T) {
	// Table-driven test to verify all event type constants are non-empty
	// and unique.
	types := []SessionEventType{
		EventSessionCreated,
		EventSessionStarted,
		EventSessionPaused,
		EventSessionResumed,
		EventSessionCompleted,
		EventSessionErrored,
		EventSessionStopped,
		EventToolCall,
		EventToolResult,
		EventCostUpdate,
		EventLayerChange,
		EventProviderSwitch,
		EventHumanContact,
	}

	seen := make(map[SessionEventType]bool, len(types))
	for _, typ := range types {
		t.Run(string(typ), func(t *testing.T) {
			if typ == "" {
				t.Fatal("event type is empty")
			}
			if seen[typ] {
				t.Fatalf("duplicate event type: %s", typ)
			}
			seen[typ] = true
		})
	}

	if len(seen) != 13 {
		t.Fatalf("expected 13 event types, got %d", len(seen))
	}
}
