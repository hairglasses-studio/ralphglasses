package healthz

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHealthz(t *testing.T) {
	s := New(":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/healthz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("healthz status = %d, want 200", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("healthz status = %v, want ok", body["status"])
	}
}

func TestReadyz_NotReady(t *testing.T) {
	s := New(":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", rr.Code)
	}
}

func TestReadyz_Ready(t *testing.T) {
	s := New(":0")
	s.SetReady()
	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/readyz", nil)
	s.srv.Handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("readyz status = %d, want 200", rr.Code)
	}
}
