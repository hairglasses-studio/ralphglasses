package wsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

func TestNewClient(t *testing.T) {
	c := NewClient("sk-test-key")

	if c.apiKey != "sk-test-key" {
		t.Errorf("apiKey = %q, want %q", c.apiKey, "sk-test-key")
	}
	if c.endpoint != DefaultEndpoint {
		t.Errorf("endpoint = %q, want %q", c.endpoint, DefaultEndpoint)
	}
	if c.httpURL != DefaultHTTPEndpoint {
		t.Errorf("httpURL = %q, want %q", c.httpURL, DefaultHTTPEndpoint)
	}
	if c.maxConnAge != DefaultMaxConnAge {
		t.Errorf("maxConnAge = %v, want %v", c.maxConnAge, DefaultMaxConnAge)
	}
	if !c.UseWebSocket {
		t.Error("UseWebSocket should default to true")
	}
	if c.connected {
		t.Error("connected should default to false")
	}
	if c.httpClient == nil {
		t.Error("httpClient should not be nil")
	}
}

func TestNewClientWithOptions(t *testing.T) {
	c := NewClient("sk-test",
		WithEndpoint("wss://custom.example.com/ws"),
		WithHTTPURL("https://custom.example.com/api"),
		WithMaxConnAge(30*time.Minute),
		WithWebSocket(false),
	)

	if c.endpoint != "wss://custom.example.com/ws" {
		t.Errorf("endpoint = %q, want custom", c.endpoint)
	}
	if c.httpURL != "https://custom.example.com/api" {
		t.Errorf("httpURL = %q, want custom", c.httpURL)
	}
	if c.maxConnAge != 30*time.Minute {
		t.Errorf("maxConnAge = %v, want 30m", c.maxConnAge)
	}
	if c.UseWebSocket {
		t.Error("UseWebSocket should be false after WithWebSocket(false)")
	}
}

func TestIsExpired(t *testing.T) {
	c := NewClient("sk-test", WithMaxConnAge(100*time.Millisecond))

	// Not connected — should not report expired.
	if c.isExpired() {
		t.Error("isExpired() should be false when not connected")
	}

	// Simulate a connected state.
	c.mu.Lock()
	c.connected = true
	c.connectedAt = time.Now()
	c.mu.Unlock()

	// Just connected — should not be expired.
	if c.isExpired() {
		t.Error("isExpired() should be false immediately after connection")
	}

	// Wait for expiration.
	time.Sleep(150 * time.Millisecond)

	if !c.isExpired() {
		t.Error("isExpired() should be true after maxConnAge elapsed")
	}
}

func TestConnectRequiresAPIKey(t *testing.T) {
	c := NewClient("")

	err := c.Connect(context.Background())
	if err == nil {
		t.Fatal("Connect() should return error without API key")
	}
	if err.Error() != "wsclient: API key is required" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSendRequiresAPIKey(t *testing.T) {
	c := NewClient("")

	_, err := c.Send(context.Background(), &Request{
		Type:  "response.create",
		Input: "hello",
		Model: "o3",
	})
	if err == nil {
		t.Fatal("Send() should return error without API key")
	}
}

func TestFallbackToHTTP(t *testing.T) {
	// Set up a test HTTP server that returns a valid response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify auth header is present.
		auth := r.Header.Get("Authorization")
		if auth != "Bearer sk-test-fallback" {
			t.Errorf("Authorization = %q, want %q", auth, "Bearer sk-test-fallback")
		}

		resp := Response{
			ID:     "resp-test-123",
			Type:   "response",
			Status: "completed",
			Output: []OutputItem{
				{
					Type: "message",
					Content: []ContentBlock{
						{Type: "output_text", Text: "fallback response"},
					},
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	// Create a client with an unreachable WebSocket endpoint so it falls back.
	c := NewClient("sk-test-fallback",
		WithEndpoint("wss://localhost:1/unreachable"),
		WithHTTPURL(srv.URL),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Send(ctx, &Request{
		Type:  "response.create",
		Input: "test input",
		Model: "o3",
	})
	if err != nil {
		t.Fatalf("Send() with HTTP fallback failed: %v", err)
	}

	if resp.ID != "resp-test-123" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "resp-test-123")
	}
	if resp.Status != "completed" {
		t.Errorf("resp.Status = %q, want %q", resp.Status, "completed")
	}
	if len(resp.Output) != 1 {
		t.Fatalf("len(resp.Output) = %d, want 1", len(resp.Output))
	}
	if len(resp.Output[0].Content) != 1 || resp.Output[0].Content[0].Text != "fallback response" {
		t.Error("unexpected output content in fallback response")
	}
}

func TestFallbackToHTTPWhenWebSocketDisabled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := Response{
			ID:     "resp-direct-http",
			Type:   "response",
			Status: "completed",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient("sk-test",
		WithHTTPURL(srv.URL),
		WithWebSocket(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Send(ctx, &Request{
		Type:  "response.create",
		Input: "test",
		Model: "o3",
	})
	if err != nil {
		t.Fatalf("Send() with WebSocket disabled failed: %v", err)
	}
	if resp.ID != "resp-direct-http" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "resp-direct-http")
	}
}

// ---------------------------------------------------------------------------
// WebSocket lifecycle integration tests
// ---------------------------------------------------------------------------

func TestWSConnectSendClose(t *testing.T) {
	var receivedAuth string

	// Server that captures auth header and echoes back a response.
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
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
			ID:     "ws-resp-1",
			Type:   "response",
			Status: "completed",
			Output: []OutputItem{{
				Type:    "message",
				Content: []ContentBlock{{Type: "output_text", Text: "WS response to: " + req.Input}},
			}},
		}
		wsjson.Write(r.Context(), conn, resp)

		// Wait for client close frame to complete the handshake.
		conn.Read(r.Context())
	}))
	defer authSrv.Close()

	wsURL := "ws" + strings.TrimPrefix(authSrv.URL, "http")
	c := NewClient("sk-ws-test",
		WithEndpoint(wsURL),
		WithWebSocket(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect.
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	if !c.isConnected() {
		t.Fatal("expected connected=true after Connect")
	}

	// Verify auth header was sent during WS handshake.
	if receivedAuth != "Bearer sk-ws-test" {
		t.Errorf("WS auth = %q, want %q", receivedAuth, "Bearer sk-ws-test")
	}

	// Send.
	resp, err := c.Send(ctx, &Request{
		Type:  "response.create",
		Input: "hello ws",
		Model: "o3",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if resp.ID != "ws-resp-1" {
		t.Errorf("resp.ID = %q, want %q", resp.ID, "ws-resp-1")
	}
	if resp.Status != "completed" {
		t.Errorf("resp.Status = %q, want %q", resp.Status, "completed")
	}
	if len(resp.Output) != 1 || len(resp.Output[0].Content) != 1 {
		t.Fatal("unexpected output shape")
	}
	if resp.Output[0].Content[0].Text != "WS response to: hello ws" {
		t.Errorf("text = %q, want %q", resp.Output[0].Content[0].Text, "WS response to: hello ws")
	}

	// Close.
	if err := c.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if c.isConnected() {
		t.Error("expected connected=false after Close")
	}
}

func TestWSAutoReconnectOnExpiry(t *testing.T) {
	var connectCount atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			ID:     "ws-reconn",
			Type:   "response",
			Status: "completed",
		}
		wsjson.Write(r.Context(), conn, resp)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := NewClient("sk-reconn",
		WithEndpoint(wsURL),
		WithWebSocket(true),
		WithMaxConnAge(50*time.Millisecond), // Very short expiry.
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// First send — should connect.
	resp, err := c.Send(ctx, &Request{Type: "response.create", Input: "first", Model: "o3"})
	if err != nil {
		t.Fatalf("first Send: %v", err)
	}
	if resp.ID != "ws-reconn" {
		t.Errorf("resp.ID = %q, want ws-reconn", resp.ID)
	}

	firstCount := connectCount.Load()
	if firstCount != 1 {
		t.Errorf("connect count = %d, want 1", firstCount)
	}

	// Wait for connection to expire.
	time.Sleep(100 * time.Millisecond)

	if !c.isExpired() {
		t.Fatal("connection should be expired")
	}

	// Second send — should auto-reconnect.
	resp2, err := c.Send(ctx, &Request{Type: "response.create", Input: "second", Model: "o3"})
	if err != nil {
		t.Fatalf("second Send: %v", err)
	}
	if resp2.ID != "ws-reconn" {
		t.Errorf("resp2.ID = %q, want ws-reconn", resp2.ID)
	}

	secondCount := connectCount.Load()
	if secondCount < 2 {
		t.Errorf("connect count after reconnect = %d, want >= 2", secondCount)
	}

	c.Close()
}

func TestWSSendResponseStructure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()

		var req Request
		if err := wsjson.Read(r.Context(), conn, &req); err != nil {
			return
		}

		// Return a multi-output response.
		resp := Response{
			ID:     "ws-multi",
			Type:   "response",
			Status: "completed",
			Output: []OutputItem{
				{
					Type: "message",
					Content: []ContentBlock{
						{Type: "output_text", Text: "Part 1"},
						{Type: "output_text", Text: "Part 2"},
					},
				},
				{
					Type: "function_call",
					Content: []ContentBlock{
						{Type: "input_json", Text: `{"tool":"test"}`},
					},
				},
			},
		}
		wsjson.Write(r.Context(), conn, resp)
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := NewClient("sk-struct",
		WithEndpoint(wsURL),
		WithWebSocket(true),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := c.Send(ctx, &Request{
		Type:         "response.create",
		Input:        "multi output test",
		Model:        "o3",
		Instructions: "be thorough",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	if resp.ID != "ws-multi" {
		t.Errorf("ID = %q, want ws-multi", resp.ID)
	}
	if len(resp.Output) != 2 {
		t.Fatalf("len(Output) = %d, want 2", len(resp.Output))
	}
	if resp.Output[0].Type != "message" {
		t.Errorf("Output[0].Type = %q, want message", resp.Output[0].Type)
	}
	if len(resp.Output[0].Content) != 2 {
		t.Fatalf("len(Output[0].Content) = %d, want 2", len(resp.Output[0].Content))
	}
	if resp.Output[0].Content[0].Text != "Part 1" {
		t.Errorf("Content[0].Text = %q, want Part 1", resp.Output[0].Content[0].Text)
	}
	if resp.Output[0].Content[1].Text != "Part 2" {
		t.Errorf("Content[1].Text = %q, want Part 2", resp.Output[0].Content[1].Text)
	}
	if resp.Output[1].Type != "function_call" {
		t.Errorf("Output[1].Type = %q, want function_call", resp.Output[1].Type)
	}

	c.Close()
}

func TestWSHTTPFallbackRequest_VerifyBody(t *testing.T) {
	var capturedBody []byte
	var capturedHeaders http.Header

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedHeaders = r.Header.Clone()
		var body httpRequest
		json.NewDecoder(r.Body).Decode(&body)
		capturedBody, _ = json.Marshal(body)
		resp := Response{ID: "http-body-test", Type: "response", Status: "completed"}
		json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	c := NewClient("sk-body-test",
		WithHTTPURL(srv.URL),
		WithWebSocket(false),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := c.Send(ctx, &Request{
		Type:         "response.create",
		Input:        "body test input",
		Model:        "o3-mini",
		Instructions: "be concise",
	})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}

	// Verify auth.
	if got := capturedHeaders.Get("Authorization"); got != "Bearer sk-body-test" {
		t.Errorf("Authorization = %q, want Bearer sk-body-test", got)
	}
	if got := capturedHeaders.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", got)
	}

	// Verify body shape.
	var body httpRequest
	if err := json.Unmarshal(capturedBody, &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Model != "o3-mini" {
		t.Errorf("body.Model = %q, want o3-mini", body.Model)
	}
	if body.Input != "body test input" {
		t.Errorf("body.Input = %q, want body test input", body.Input)
	}
	if body.Instructions != "be concise" {
		t.Errorf("body.Instructions = %q, want be concise", body.Instructions)
	}
}

func TestCloseWhenNotConnected(t *testing.T) {
	c := NewClient("sk-test")
	// Should be a no-op, not an error.
	if err := c.Close(); err != nil {
		t.Errorf("Close on unconnected client: %v", err)
	}
}

func TestDoubleClose(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{InsecureSkipVerify: true})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		// Keep connection open until client closes.
		for {
			_, _, err := conn.Read(r.Context())
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()

	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	c := NewClient("sk-dclose", WithEndpoint(wsURL))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := c.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}

	// First close should succeed.
	if err := c.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	// Second close should be a no-op.
	if err := c.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// Suppress unused import warnings.
var _ = strings.TrimPrefix
var _ = atomic.Int32{}
