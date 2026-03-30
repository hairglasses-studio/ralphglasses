package events

import (
	"context"
	"sync"
	"testing"
	"time"
)

// Verify memTransport satisfies EventTransport at compile time.
var _ EventTransport = (*memTransport)(nil)

func TestMemTransport_PublishSubscribe(t *testing.T) {
	tr := newMemTransport()
	defer tr.Close()

	ch, err := tr.Subscribe(context.Background(), "sub1", nil)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	evt := Event{Type: SessionStarted, RepoName: "repo1", Timestamp: time.Now(), Version: 1}
	if err := tr.Publish(context.Background(), evt); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case e := <-ch:
		if e.Type != SessionStarted {
			t.Errorf("type = %q, want %q", e.Type, SessionStarted)
		}
		if e.RepoName != "repo1" {
			t.Errorf("repo = %q, want repo1", e.RepoName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestMemTransport_FilteredSubscribe(t *testing.T) {
	tr := newMemTransport()
	defer tr.Close()

	filter := func(e Event) bool { return e.Type == SessionStarted }
	ch, err := tr.Subscribe(context.Background(), "filtered", filter)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	tr.Publish(context.Background(), Event{Type: SessionStarted, RepoName: "yes", Timestamp: time.Now(), Version: 1})
	tr.Publish(context.Background(), Event{Type: CostUpdate, RepoName: "no", Timestamp: time.Now(), Version: 1})

	select {
	case e := <-ch:
		if e.RepoName != "yes" {
			t.Errorf("repo = %q, want yes", e.RepoName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	// CostUpdate should not arrive
	select {
	case e := <-ch:
		t.Errorf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected
	}
}

func TestMemTransport_Unsubscribe(t *testing.T) {
	tr := newMemTransport()
	defer tr.Close()

	ch, _ := tr.Subscribe(context.Background(), "sub1", nil)
	tr.Unsubscribe("sub1")

	// Channel should be closed
	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe")
	}
}

func TestMemTransport_Close(t *testing.T) {
	tr := newMemTransport()

	ch1, _ := tr.Subscribe(context.Background(), "a", nil)
	ch2, _ := tr.Subscribe(context.Background(), "b", nil)

	if err := tr.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Both channels should be closed
	if _, ok := <-ch1; ok {
		t.Error("ch1 should be closed")
	}
	if _, ok := <-ch2; ok {
		t.Error("ch2 should be closed")
	}
}

func TestMemTransport_ConcurrentPublish(t *testing.T) {
	tr := newMemTransport()
	defer tr.Close()

	ch, _ := tr.Subscribe(context.Background(), "concurrent", nil)

	const goroutines = 10
	const perGoroutine = 10

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				tr.Publish(context.Background(), Event{Type: LoopIterated, Timestamp: time.Now(), Version: 1})
			}
		}()
	}
	wg.Wait()

	// Drain and count
	count := 0
	for {
		select {
		case <-ch:
			count++
		default:
			goto done
		}
	}
done:
	if count != goroutines*perGoroutine {
		t.Errorf("received %d events, want %d", count, goroutines*perGoroutine)
	}
}

// TestBusWithTransport verifies that Bus delegates to a plugged-in transport.
func TestBusWithTransport(t *testing.T) {
	tr := newMemTransport()
	bus := NewBus(100, WithTransport(tr))

	ch := bus.Subscribe("transport-sub")

	bus.Publish(Event{Type: SessionStarted, RepoName: "via-transport"})

	select {
	case e := <-ch:
		if e.Type != SessionStarted {
			t.Errorf("type = %q, want %q", e.Type, SessionStarted)
		}
		if e.RepoName != "via-transport" {
			t.Errorf("repo = %q, want via-transport", e.RepoName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event via transport")
	}

	// History should still work (local to Bus)
	hist := bus.History("", 10)
	if len(hist) != 1 {
		t.Fatalf("history len = %d, want 1", len(hist))
	}

	bus.Unsubscribe("transport-sub")
	bus.Close()
}

// TestBusWithTransport_Filtered verifies filtered subscriptions through transport.
func TestBusWithTransport_Filtered(t *testing.T) {
	tr := newMemTransport()
	bus := NewBus(100, WithTransport(tr))

	ch := bus.SubscribeFiltered("filt", SessionStarted)

	bus.Publish(Event{Type: SessionStarted, RepoName: "match"})
	bus.Publish(Event{Type: CostUpdate, RepoName: "skip"})

	select {
	case e := <-ch:
		if e.RepoName != "match" {
			t.Errorf("repo = %q, want match", e.RepoName)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout")
	}

	select {
	case e := <-ch:
		t.Errorf("unexpected event: %+v", e)
	case <-time.After(50 * time.Millisecond):
		// expected
	}

	bus.Close()
}

// TestBusWithTransport_Unsubscribe verifies unsubscribe delegates to transport.
func TestBusWithTransport_Unsubscribe(t *testing.T) {
	tr := newMemTransport()
	bus := NewBus(100, WithTransport(tr))

	ch := bus.Subscribe("unsub-me")
	bus.Unsubscribe("unsub-me")

	_, ok := <-ch
	if ok {
		t.Error("channel should be closed after unsubscribe via transport")
	}

	bus.Close()
}

// TestBusTransportAccessor verifies the Transport() accessor.
func TestBusTransportAccessor(t *testing.T) {
	// No transport configured
	bus := NewBus(100)
	if bus.Transport() != nil {
		t.Error("Transport() should be nil when not configured")
	}

	// With transport configured
	tr := newMemTransport()
	bus2 := NewBus(100, WithTransport(tr))
	if bus2.Transport() == nil {
		t.Error("Transport() should be non-nil when configured")
	}
}

// TestBusWithTransport_HistoryStillLocal verifies that history, cursor, and
// time-based queries are unaffected by the transport.
func TestBusWithTransport_HistoryStillLocal(t *testing.T) {
	tr := newMemTransport()
	bus := NewBus(100, WithTransport(tr))

	now := time.Now()
	bus.Publish(Event{Type: SessionStarted, RepoName: "a", Timestamp: now.Add(-time.Second)})
	bus.Publish(Event{Type: SessionEnded, RepoName: "b"})

	// History works
	all := bus.History("", 10)
	if len(all) != 2 {
		t.Fatalf("history len = %d, want 2", len(all))
	}

	// HistoryAfterCursor works
	evts, cursor := bus.HistoryAfterCursor(0, 10)
	if len(evts) != 2 || cursor != 2 {
		t.Fatalf("HistoryAfterCursor: got %d events, cursor=%d, want 2/2", len(evts), cursor)
	}

	// HistorySince works
	since := bus.HistorySince(now.Add(-500 * time.Millisecond))
	if len(since) != 1 {
		t.Fatalf("HistorySince: got %d, want 1", len(since))
	}
	if since[0].RepoName != "b" {
		t.Errorf("HistorySince event = %q, want b", since[0].RepoName)
	}

	bus.Close()
}
