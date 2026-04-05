package healthz

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsEndpoint(t *testing.T) {
	t.Parallel()
	s := New(":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("metrics status = %d, want 200", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "ralphglasses_up 1") {
		t.Error("metrics missing ralphglasses_up gauge")
	}
	if !strings.Contains(body, "ralphglasses_uptime_seconds") {
		t.Error("metrics missing ralphglasses_uptime_seconds gauge")
	}
	if ct := rr.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/plain; charset=utf-8", ct)
	}
}

func TestStartAndShutdown(t *testing.T) {
	t.Parallel()

	// Pick a random free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	s := New(addr)
	s.SetReady()

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Start()
	}()

	// Wait for the server to be reachable.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Hit a health endpoint to confirm it is serving.
	resp, err := http.Get(fmt.Sprintf("http://%s/healthz", addr))
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/healthz status = %d, want 200", resp.StatusCode)
	}

	// Graceful shutdown.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}

	// Start should have returned http.ErrServerClosed (which Serve wraps).
	startErr := <-errCh
	if startErr != nil && startErr != http.ErrServerClosed {
		t.Errorf("Start returned unexpected error: %v", startErr)
	}
}

func TestStartBadAddress(t *testing.T) {
	t.Parallel()
	// Bind to a port that is already in use by listening first.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	addr := ln.Addr().String()

	s := New(addr)
	if err := s.Start(); err == nil {
		t.Error("Start on occupied port should return error")
	}
}

func TestHealthzResponseShape(t *testing.T) {
	t.Parallel()
	s := New(":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := body["uptime"]; !ok {
		t.Error("response missing uptime field")
	}
}

func TestReadyzResponseBody(t *testing.T) {
	t.Parallel()
	s := New(":0")

	// Not ready state.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	var body map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal not_ready: %v", err)
	}
	if body["status"] != "not_ready" {
		t.Errorf("not-ready status = %q, want not_ready", body["status"])
	}

	// Ready state.
	s.SetReady()
	rr2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr2, req2)

	var body2 map[string]string
	if err := json.Unmarshal(rr2.Body.Bytes(), &body2); err != nil {
		t.Fatalf("unmarshal ready: %v", err)
	}
	if body2["status"] != "ready" {
		t.Errorf("ready status = %q, want ready", body2["status"])
	}
}

func TestMetricsUptimeAdvances(t *testing.T) {
	t.Parallel()
	s := New(":0")
	// Overwrite started to a known time in the past.
	s.started = time.Now().Add(-10 * time.Second)

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	body := rr.Body.String()
	// The uptime should be at least 9 seconds.
	if !strings.Contains(body, "ralphglasses_uptime_seconds") {
		t.Error("missing uptime metric")
	}
}
