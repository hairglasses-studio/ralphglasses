package events

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/runmylife/internal/testutil"
)

func TestBus_PublishSync(t *testing.T) {
	bus := NewBus(nil)
	var received Event
	bus.Subscribe(TaskCompleted, func(e Event) {
		received = e
	})

	e := New(TaskCompleted, "test", map[string]any{"task_id": "123"})
	bus.Publish(context.Background(), e)

	if received.Type != TaskCompleted {
		t.Errorf("received type = %v, want %v", received.Type, TaskCompleted)
	}
	if received.Payload["task_id"] != "123" {
		t.Errorf("payload task_id = %v, want 123", received.Payload["task_id"])
	}
}

func TestBus_PublishAsync(t *testing.T) {
	bus := NewBus(nil)
	var wg sync.WaitGroup
	var count int32
	wg.Add(2)

	handler := func(e Event) {
		atomic.AddInt32(&count, 1)
		wg.Done()
	}
	bus.Subscribe(MoodLogged, handler)
	bus.Subscribe(MoodLogged, handler)

	e := New(MoodLogged, "test", nil)
	bus.PublishAsync(context.Background(), e)

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for async handlers")
	}

	if atomic.LoadInt32(&count) != 2 {
		t.Errorf("count = %d, want 2", count)
	}
}

func TestBus_MultipleSubscribers(t *testing.T) {
	bus := NewBus(nil)
	var count int32
	for i := 0; i < 5; i++ {
		bus.Subscribe(FocusStarted, func(e Event) {
			atomic.AddInt32(&count, 1)
		})
	}

	bus.Publish(context.Background(), New(FocusStarted, "test", nil))
	if atomic.LoadInt32(&count) != 5 {
		t.Errorf("count = %d, want 5", count)
	}
}

func TestBus_SubscribeAll(t *testing.T) {
	bus := NewBus(nil)
	var received []EventType
	var mu sync.Mutex
	bus.SubscribeAll(func(e Event) {
		mu.Lock()
		received = append(received, e.Type)
		mu.Unlock()
	})

	bus.Publish(context.Background(), New(TaskCompleted, "test", nil))
	bus.Publish(context.Background(), New(MoodLogged, "test", nil))
	bus.Publish(context.Background(), New(FocusStarted, "test", nil))

	if len(received) != 3 {
		t.Errorf("wildcard received %d events, want 3", len(received))
	}
}

func TestBus_HandlerPanic(t *testing.T) {
	bus := NewBus(nil)
	var secondCalled bool
	bus.Subscribe(TaskCompleted, func(e Event) {
		panic("boom")
	})
	bus.Subscribe(TaskCompleted, func(e Event) {
		secondCalled = true
	})

	// Should not panic; second handler should still run
	bus.Publish(context.Background(), New(TaskCompleted, "test", nil))
	if !secondCalled {
		t.Error("second handler should run even after first panics")
	}
}

func TestBus_Persistence(t *testing.T) {
	db := testutil.TestDB(t)
	bus := NewBus(db)

	e := New(TaskCompleted, "test-source", map[string]any{"task_id": "abc"})
	bus.Publish(context.Background(), e)

	// Verify event was persisted
	events, err := QueryEvents(context.Background(), db, EventFilter{Type: TaskCompleted, Limit: 10})
	if err != nil {
		t.Fatalf("QueryEvents: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("persisted %d events, want 1", len(events))
	}
	if events[0].Source != "test-source" {
		t.Errorf("source = %q, want %q", events[0].Source, "test-source")
	}

	// Count
	n, err := CountEvents(context.Background(), db, EventFilter{Type: TaskCompleted})
	if err != nil {
		t.Fatalf("CountEvents: %v", err)
	}
	if n != 1 {
		t.Errorf("count = %d, want 1", n)
	}
}

func TestBus_NoPersistence(t *testing.T) {
	bus := NewBus(nil)
	var called bool
	bus.Subscribe(MoodLogged, func(e Event) { called = true })

	// Should not panic with nil db
	bus.Publish(context.Background(), New(MoodLogged, "test", nil))
	if !called {
		t.Error("handler should be called even without persistence")
	}
}
