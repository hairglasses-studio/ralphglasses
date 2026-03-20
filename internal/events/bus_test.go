package events

import (
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
