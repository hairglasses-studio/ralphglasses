package wsclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// echoServer returns an httptest.Server that accepts WebSocket connections,
// reads one request, and writes back a canned response. connectCount is
// incremented on each new connection.
func echoServer(t *testing.T, connectCount *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectCount.Add(1)
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		var req Request
		if err := wsjson.Read(r.Context(), conn, &req); err != nil {
			return
		}
		resp := Response{
			ID:     "echo-resp",
			Type:   "response",
			Status: "completed",
			Output: []OutputItem{{
				Type:    "message",
				Content: []ContentBlock{{Type: "output_text", Text: "echo: " + req.Input}},
			}},
		}
		_ = wsjson.Write(r.Context(), conn, resp)
	}))
}

// flakyServer returns a server that rejects the first `failCount` connections
// then accepts subsequent ones normally.
func flakyServer(t *testing.T, failCount int, connectCount *atomic.Int32) *httptest.Server {
	t.Helper()
	var rejected atomic.Int32
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		connectCount.Add(1)
		n := int(rejected.Add(1))
		if n <= failCount {
			// Reject the WebSocket upgrade by writing a plain HTTP error.
			http.Error(w, "service unavailable", http.StatusServiceUnavailable)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		var req Request
		if err := wsjson.Read(r.Context(), conn, &req); err != nil {
			return
		}
		resp := Response{
			ID:     "flaky-ok",
			Type:   "response",
			Status: "completed",
		}
		_ = wsjson.Write(r.Context(), conn, resp)
	}))
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

// collectEvents returns a callback and a function to retrieve collected events.
func collectEvents() (ReconnectCallback, func() []ReconnectEvent) {
	var mu sync.Mutex
	var events []ReconnectEvent
	cb := func(evt ReconnectEvent) {
		mu.Lock()
		events = append(events, evt)
		mu.Unlock()
	}
	get := func() []ReconnectEvent {
		mu.Lock()
		defer mu.Unlock()
		out := make([]ReconnectEvent, len(events))
		copy(out, events)
		return out
	}
	return cb, get
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestReconnectingClient_BasicSend(t *testing.T) {
	var cc atomic.Int32
	srv := echoServer(t, &cc)
	defer srv.Close()

	base := NewClient("sk-test", WithEndpoint(wsURL(srv)))
	rc := NewReconnectingClient(base)
	defer rc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if rc.State() != StateConnected {
		t.Errorf("state = %q, want %q", rc.State(), StateConnected)
	}

	resp, err := rc.Send(ctx, &Request{Type: "response.create", Input: "hello", Model: "o3"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.ID != "echo-resp" {
		t.Errorf("resp.ID = %q, want echo-resp", resp.ID)
	}
	if resp.Output[0].Content[0].Text != "echo: hello" {
		t.Errorf("text = %q, want echo: hello", resp.Output[0].Content[0].Text)
	}
}

func TestReconnectingClient_AutoReconnectAfterDisconnect(t *testing.T) {
	var cc atomic.Int32
	// The server closes after each request, forcing a reconnect on the second send.
	srv := echoServer(t, &cc)
	defer srv.Close()

	cb, getEvents := collectEvents()
	base := NewClient("sk-test",
		WithEndpoint(wsURL(srv)),
		WithMaxConnAge(1*time.Millisecond), // Force expiry between sends.
	)
	rc := NewReconnectingClient(base,
		WithInitialBackoff(10*time.Millisecond),
		WithMaxBackoff(50*time.Millisecond),
		WithOnReconnect(cb),
	)
	defer rc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First send: connect + send.
	resp, err := rc.Send(ctx, &Request{Type: "response.create", Input: "first", Model: "o3"})
	if err != nil {
		t.Fatalf("first Send: %v", err)
	}
	if resp.ID != "echo-resp" {
		t.Errorf("first resp.ID = %q, want echo-resp", resp.ID)
	}

	// Wait for connection to expire.
	time.Sleep(20 * time.Millisecond)

	// Second send: should auto-reconnect via the base client's expiry logic.
	resp2, err := rc.Send(ctx, &Request{Type: "response.create", Input: "second", Model: "o3"})
	if err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if resp2.ID != "echo-resp" {
		t.Errorf("second resp.ID = %q, want echo-resp", resp2.ID)
	}

	// Verify multiple connections were made.
	if cc.Load() < 2 {
		t.Errorf("connect count = %d, want >= 2", cc.Load())
	}

	// Events should have been fired.
	events := getEvents()
	if len(events) == 0 {
		t.Error("expected at least one reconnect event")
	}
}

func TestReconnectingClient_ExponentialBackoff(t *testing.T) {
	var cc atomic.Int32
	// Fail the first 3 attempts, succeed on the 4th.
	srv := flakyServer(t, 3, &cc)
	defer srv.Close()

	cb, getEvents := collectEvents()
	base := NewClient("sk-test", WithEndpoint(wsURL(srv)))
	rc := NewReconnectingClient(base,
		WithInitialBackoff(10*time.Millisecond),
		WithMaxBackoff(100*time.Millisecond),
		WithBackoffFactor(2.0),
		WithOnReconnect(cb),
	)
	defer rc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := rc.Send(ctx, &Request{Type: "response.create", Input: "retry", Model: "o3"})
	if err != nil {
		t.Fatalf("Send after retries: %v", err)
	}
	if resp.ID != "flaky-ok" {
		t.Errorf("resp.ID = %q, want flaky-ok", resp.ID)
	}

	// Verify backoff delays increase across reconnecting events.
	events := getEvents()
	var reconnecting []ReconnectEvent
	for _, e := range events {
		if e.State == StateReconnecting {
			reconnecting = append(reconnecting, e)
		}
	}
	if len(reconnecting) < 2 {
		t.Fatalf("expected >= 2 reconnecting events, got %d", len(reconnecting))
	}
	// Each successive delay should be >= the previous.
	for i := 1; i < len(reconnecting); i++ {
		if reconnecting[i].Delay < reconnecting[i-1].Delay {
			t.Errorf("delay[%d] = %v < delay[%d] = %v (should increase)",
				i, reconnecting[i].Delay, i-1, reconnecting[i-1].Delay)
		}
	}
}

func TestReconnectingClient_MaxRetriesExhausted(t *testing.T) {
	var cc atomic.Int32
	// Server always rejects.
	srv := flakyServer(t, 1000, &cc)
	defer srv.Close()

	base := NewClient("sk-test",
		WithEndpoint(wsURL(srv)),
		WithWebSocket(true),
	)
	// Disable HTTP fallback by pointing httpURL at the same flaky server
	// which returns 503.
	base.httpURL = srv.URL

	rc := NewReconnectingClient(base,
		WithInitialBackoff(5*time.Millisecond),
		WithMaxBackoff(10*time.Millisecond),
		WithMaxRetries(3),
	)
	defer rc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := rc.Send(ctx, &Request{Type: "response.create", Input: "fail", Model: "o3"})
	if err == nil {
		t.Fatal("expected error after max retries exhausted")
	}

	// State should be disconnected.
	if rc.State() != StateDisconnected {
		t.Errorf("state = %q, want %q", rc.State(), StateDisconnected)
	}
}

func TestReconnectingClient_ContextCancellation(t *testing.T) {
	var cc atomic.Int32
	// Server always rejects so reconnection loops.
	srv := flakyServer(t, 1000, &cc)
	defer srv.Close()

	base := NewClient("sk-test",
		WithEndpoint(wsURL(srv)),
		WithWebSocket(true),
	)
	base.httpURL = srv.URL

	rc := NewReconnectingClient(base,
		WithInitialBackoff(50*time.Millisecond),
		WithMaxBackoff(100*time.Millisecond),
	)
	defer rc.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	_, err := rc.Send(ctx, &Request{Type: "response.create", Input: "timeout", Model: "o3"})
	if err == nil {
		t.Fatal("expected error on context timeout")
	}
}

func TestReconnectingClient_CloseStopsReconnection(t *testing.T) {
	var cc atomic.Int32
	srv := flakyServer(t, 1000, &cc)
	defer srv.Close()

	base := NewClient("sk-test",
		WithEndpoint(wsURL(srv)),
		WithWebSocket(true),
	)
	base.httpURL = srv.URL

	rc := NewReconnectingClient(base,
		WithInitialBackoff(50*time.Millisecond),
	)

	// Close immediately, then send should fail.
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if rc.State() != StateClosed {
		t.Errorf("state = %q, want %q", rc.State(), StateClosed)
	}

	ctx := context.Background()
	_, err := rc.Send(ctx, &Request{Type: "response.create", Input: "closed", Model: "o3"})
	if err == nil {
		t.Fatal("expected error sending on closed client")
	}
	if !strings.Contains(err.Error(), "closed") {
		t.Errorf("error = %q, want to contain 'closed'", err.Error())
	}

	// Connect should also fail.
	err = rc.Connect(ctx)
	if err == nil {
		t.Fatal("expected error connecting on closed client")
	}
}

func TestReconnectingClient_StateTransitions(t *testing.T) {
	var cc atomic.Int32
	srv := echoServer(t, &cc)
	defer srv.Close()

	cb, getEvents := collectEvents()
	base := NewClient("sk-test", WithEndpoint(wsURL(srv)))
	rc := NewReconnectingClient(base, WithOnReconnect(cb))

	// Initially disconnected.
	if rc.State() != StateDisconnected {
		t.Errorf("initial state = %q, want %q", rc.State(), StateDisconnected)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect transitions: connecting -> connected.
	if err := rc.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	events := getEvents()
	if len(events) < 2 {
		t.Fatalf("expected >= 2 events for connect, got %d", len(events))
	}
	if events[0].State != StateConnecting {
		t.Errorf("events[0].State = %q, want %q", events[0].State, StateConnecting)
	}
	if events[1].State != StateConnected {
		t.Errorf("events[1].State = %q, want %q", events[1].State, StateConnected)
	}

	// Close transitions to closed.
	if err := rc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	events = getEvents()
	last := events[len(events)-1]
	if last.State != StateClosed {
		t.Errorf("last event state = %q, want %q", last.State, StateClosed)
	}
}

func TestReconnectingClient_BackoffDelay(t *testing.T) {
	base := NewClient("sk-test")
	rc := NewReconnectingClient(base,
		WithInitialBackoff(100*time.Millisecond),
		WithMaxBackoff(1*time.Second),
		WithBackoffFactor(2.0),
	)

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 100 * time.Millisecond},  // 100ms * 2^0 = 100ms
		{2, 200 * time.Millisecond},  // 100ms * 2^1 = 200ms
		{3, 400 * time.Millisecond},  // 100ms * 2^2 = 400ms
		{4, 800 * time.Millisecond},  // 100ms * 2^3 = 800ms
		{5, 1 * time.Second},         // 100ms * 2^4 = 1600ms, capped at 1s
		{6, 1 * time.Second},         // Still capped.
		{10, 1 * time.Second},        // Still capped.
	}

	for _, tt := range tests {
		got := rc.backoffDelay(tt.attempt)
		if got != tt.expected {
			t.Errorf("backoffDelay(%d) = %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestReconnectingClient_DefaultOptions(t *testing.T) {
	base := NewClient("sk-test")
	rc := NewReconnectingClient(base)

	if rc.initialBackoff != DefaultInitialBackoff {
		t.Errorf("initialBackoff = %v, want %v", rc.initialBackoff, DefaultInitialBackoff)
	}
	if rc.maxBackoff != DefaultMaxBackoff {
		t.Errorf("maxBackoff = %v, want %v", rc.maxBackoff, DefaultMaxBackoff)
	}
	if rc.backoffFactor != DefaultBackoffFactor {
		t.Errorf("backoffFactor = %v, want %v", rc.backoffFactor, DefaultBackoffFactor)
	}
	if rc.maxRetries != DefaultMaxRetries {
		t.Errorf("maxRetries = %v, want %v", rc.maxRetries, DefaultMaxRetries)
	}
	if rc.state != StateDisconnected {
		t.Errorf("state = %q, want %q", rc.state, StateDisconnected)
	}
	if rc.client != base {
		t.Error("client should be the same base client")
	}
}

func TestReconnectingClient_ClientAccessor(t *testing.T) {
	base := NewClient("sk-test")
	rc := NewReconnectingClient(base)
	if rc.Client() != base {
		t.Error("Client() should return the underlying client")
	}
}

func TestReconnectingClient_InvalidOptionsIgnored(t *testing.T) {
	base := NewClient("sk-test")
	rc := NewReconnectingClient(base,
		WithInitialBackoff(-1*time.Second), // Invalid, should be ignored.
		WithMaxBackoff(0),                   // Invalid, should be ignored.
		WithBackoffFactor(0.5),              // Invalid (<= 1.0), should be ignored.
	)

	if rc.initialBackoff != DefaultInitialBackoff {
		t.Errorf("initialBackoff = %v, want default %v", rc.initialBackoff, DefaultInitialBackoff)
	}
	if rc.maxBackoff != DefaultMaxBackoff {
		t.Errorf("maxBackoff = %v, want default %v", rc.maxBackoff, DefaultMaxBackoff)
	}
	if rc.backoffFactor != DefaultBackoffFactor {
		t.Errorf("backoffFactor = %v, want default %v", rc.backoffFactor, DefaultBackoffFactor)
	}
}

func TestReconnectingClient_MaxBackoffCap(t *testing.T) {
	base := NewClient("sk-test")
	rc := NewReconnectingClient(base,
		WithInitialBackoff(10*time.Second),
		WithMaxBackoff(15*time.Second),
		WithBackoffFactor(3.0),
	)

	// Attempt 1: 10s * 3^0 = 10s.
	if d := rc.backoffDelay(1); d != 10*time.Second {
		t.Errorf("attempt 1 delay = %v, want 10s", d)
	}
	// Attempt 2: 10s * 3^1 = 30s, capped to 15s.
	if d := rc.backoffDelay(2); d != 15*time.Second {
		t.Errorf("attempt 2 delay = %v, want 15s (capped)", d)
	}
}
