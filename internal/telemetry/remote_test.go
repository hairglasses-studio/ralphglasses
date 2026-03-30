package telemetry

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestSendQueuesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithBatchSize(10),
		WithFlushInterval(time.Hour), // prevent auto-flush
	)
	defer c.Close()

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		if err := c.Send(ctx, Event{Type: EventSessionStart}); err != nil {
			t.Fatalf("Send: %v", err)
		}
	}

	c.mu.Lock()
	n := len(c.queue)
	c.mu.Unlock()
	if n != 5 {
		t.Fatalf("expected 5 queued events, got %d", n)
	}
}

func TestFlushSendsCorrectJSON(t *testing.T) {
	var (
		mu       sync.Mutex
		received payload
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json content-type, got %q", r.Header.Get("Content-Type"))
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("expected Bearer test-key, got %q", r.Header.Get("Authorization"))
		}
		mu.Lock()
		defer mu.Unlock()
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Errorf("decode: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithAPIKey("test-key"),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	ctx := context.Background()
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	_ = c.Send(ctx, Event{
		Type:      EventSessionStart,
		Timestamp: ts,
		SessionID: "s1",
		Provider:  "claude",
		Data:      map[string]any{"repo": "ralphglasses"},
	})

	if err := c.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(received.Events))
	}
	ev := received.Events[0]
	if ev.Type != EventSessionStart {
		t.Errorf("type = %q, want %q", ev.Type, EventSessionStart)
	}
	if ev.SessionID != "s1" {
		t.Errorf("session_id = %q, want s1", ev.SessionID)
	}
	if ev.Provider != "claude" {
		t.Errorf("provider = %q, want claude", ev.Provider)
	}
	if ev.Data["repo"] != "ralphglasses" {
		t.Errorf("data[repo] = %v, want ralphglasses", ev.Data["repo"])
	}
}

func TestBatchFlushOnFull(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithBatchSize(3),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = c.Send(ctx, Event{Type: EventCrash})
	}

	// Give the auto-flush from Send a moment.
	time.Sleep(50 * time.Millisecond)

	if n := calls.Load(); n < 1 {
		t.Fatalf("expected at least 1 flush call when batch full, got %d", n)
	}
}

func TestRetryOnServerError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := attempts.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithMaxRetries(3),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	_ = c.Send(context.Background(), Event{Type: EventCrash})
	err := c.Flush(context.Background())
	if err != nil {
		t.Fatalf("Flush should succeed after retries: %v", err)
	}
	if n := attempts.Load(); n != 3 {
		t.Errorf("expected 3 attempts (1 initial + 2 retries), got %d", n)
	}
}

func TestNoRetryOnClientError(t *testing.T) {
	var attempts atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL, WithFlushInterval(time.Hour))
	defer c.Close()

	_ = c.Send(context.Background(), Event{Type: EventCrash})
	err := c.Flush(context.Background())
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if n := attempts.Load(); n != 1 {
		t.Errorf("expected 1 attempt for 4xx, got %d", n)
	}
}

func TestContextCancellationStopsFlush(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(5 * time.Second) // block long enough to be cancelled
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithHTTPTimeout(10*time.Second),
		WithFlushInterval(time.Hour),
	)
	defer c.Close()

	_ = c.Send(context.Background(), Event{Type: EventSessionStart})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := c.Flush(ctx)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}

	// Events should be re-queued.
	c.mu.Lock()
	n := len(c.queue)
	c.mu.Unlock()
	if n == 0 {
		t.Error("expected events to be re-queued after cancellation")
	}
}

func TestCloseFlushesRemaining(t *testing.T) {
	var (
		mu       sync.Mutex
		received []Event
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p payload
		_ = json.NewDecoder(r.Body).Decode(&p)
		mu.Lock()
		received = append(received, p.Events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL,
		WithBatchSize(1000),
		WithFlushInterval(time.Hour),
	)

	ctx := context.Background()
	for i := 0; i < 5; i++ {
		_ = c.Send(ctx, Event{Type: EventSessionStop})
	}

	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 5 {
		t.Errorf("expected 5 flushed events on Close, got %d", len(received))
	}
}

func TestSendAfterCloseReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL, WithFlushInterval(time.Hour))
	_ = c.Close()

	err := c.Send(context.Background(), Event{Type: EventSessionStart})
	if err == nil {
		t.Fatal("expected error sending after Close")
	}
}

func TestFunctionalOptions(t *testing.T) {
	c := NewRemoteClient("http://localhost",
		WithAPIKey("k"),
		WithBatchSize(50),
		WithFlushInterval(5*time.Second),
		WithHTTPTimeout(3*time.Second),
		WithMaxRetries(5),
	)
	defer c.Close()

	if c.apiKey != "k" {
		t.Errorf("apiKey = %q, want k", c.apiKey)
	}
	if c.batchSize != 50 {
		t.Errorf("batchSize = %d, want 50", c.batchSize)
	}
	if c.flushInterval != 5*time.Second {
		t.Errorf("flushInterval = %v, want 5s", c.flushInterval)
	}
	if c.httpTimeout != 3*time.Second {
		t.Errorf("httpTimeout = %v, want 3s", c.httpTimeout)
	}
	if c.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", c.maxRetries)
	}
}

func TestFlushEmptyQueueIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("unexpected request for empty flush")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL, WithFlushInterval(time.Hour))
	defer c.Close()

	if err := c.Flush(context.Background()); err != nil {
		t.Fatalf("Flush empty: %v", err)
	}
}

func TestTimestampAutoSet(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewRemoteClient(srv.URL, WithFlushInterval(time.Hour))
	defer c.Close()

	before := time.Now()
	_ = c.Send(context.Background(), Event{Type: EventCrash})

	c.mu.Lock()
	ts := c.queue[0].Timestamp
	c.mu.Unlock()

	if ts.Before(before) {
		t.Errorf("auto-set timestamp %v is before send time %v", ts, before)
	}
}
