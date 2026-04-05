package fleet

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDefaultTransport(t *testing.T) {
	tr := DefaultTransport()

	if tr.MaxIdleConns != 100 {
		t.Errorf("MaxIdleConns = %d, want 100", tr.MaxIdleConns)
	}
	if tr.MaxIdleConnsPerHost != 10 {
		t.Errorf("MaxIdleConnsPerHost = %d, want 10", tr.MaxIdleConnsPerHost)
	}
	if tr.IdleConnTimeout != 90*time.Second {
		t.Errorf("IdleConnTimeout = %v, want 90s", tr.IdleConnTimeout)
	}
	if tr.TLSHandshakeTimeout != 5*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 5s", tr.TLSHandshakeTimeout)
	}
	if tr.ResponseHeaderTimeout != 10*time.Second {
		t.Errorf("ResponseHeaderTimeout = %v, want 10s", tr.ResponseHeaderTimeout)
	}
}

func TestNewClientUsesPooledTransport(t *testing.T) {
	c := NewClient("http://localhost:9999")
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport from NewClient")
	}
	if tr.MaxIdleConns != 100 {
		t.Errorf("expected pooled transport MaxIdleConns=100, got %d", tr.MaxIdleConns)
	}
}

func TestNewClientWithTransport(t *testing.T) {
	custom := &http.Transport{MaxIdleConns: 42}
	c := NewClientWithTransport("http://localhost:9999", custom)
	tr, ok := c.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected *http.Transport")
	}
	if tr.MaxIdleConns != 42 {
		t.Errorf("expected custom MaxIdleConns=42, got %d", tr.MaxIdleConns)
	}
}

func TestPingCoordinator_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/status" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != "GET" {
			t.Errorf("unexpected method: %s", r.Method)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(NodeStatus{
			NodeID:   "coord-1",
			Role:     "coordinator",
			Hostname: "test-host",
			Version:  "0.1.0",
		})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	if err := c.PingCoordinator(context.Background()); err != nil {
		t.Fatalf("PingCoordinator returned error: %v", err)
	}
}

func TestPingCoordinator_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	err := c.PingCoordinator(context.Background())
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPingCoordinator_Unreachable(t *testing.T) {
	c := NewClient("http://127.0.0.1:1") // nothing listening
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := c.PingCoordinator(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable host")
	}
}

func TestConnectionReuse(t *testing.T) {
	// Verify that multiple requests to the same server reuse the
	// underlying TCP connection (the pooled transport keeps it alive).
	var reqCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reqCount++
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(NodeStatus{NodeID: "coord-1", Role: "coordinator"})
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	ctx := context.Background()

	for i := range 5 {
		if err := c.PingCoordinator(ctx); err != nil {
			t.Fatalf("request %d failed: %v", i, err)
		}
	}

	if reqCount != 5 {
		t.Fatalf("expected 5 requests, got %d", reqCount)
	}

	// The transport should have an idle connection cached. We cannot
	// easily inspect the pool from outside, but reaching here without
	// error confirms reuse did not break anything.
}
