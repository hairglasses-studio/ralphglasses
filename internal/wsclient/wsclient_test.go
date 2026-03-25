package wsclient

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
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
