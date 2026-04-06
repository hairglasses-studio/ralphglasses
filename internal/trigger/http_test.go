package trigger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandleTrigger_Success(t *testing.T) {
	launched := false
	s := NewServer(":0", func(_ context.Context, req TriggerRequest) (string, error) {
		launched = true
		if req.Source != "github" {
			t.Errorf("source = %q, want %q", req.Source, "github")
		}
		if req.Event != "push" {
			t.Errorf("event = %q, want %q", req.Event, "push")
		}
		return "run-123", nil
	}, nil)

	body := `{"source":"github","event":"push","payload":{"ref":"refs/heads/main"}}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !launched {
		t.Fatal("launcher was not called")
	}

	var resp TriggerResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID != "run-123" {
		t.Errorf("run_id = %q, want %q", resp.RunID, "run-123")
	}
	if resp.Status != "launched" {
		t.Errorf("status = %q, want %q", resp.Status, "launched")
	}
}

func TestHandleTrigger_MissingSource(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil)

	body := `{"event":"push"}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_MissingEvent(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil)

	body := `{"source":"github"}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_InvalidJSON(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil)

	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_LaunchError(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", fmt.Errorf("provider down")
	}, nil)

	body := `{"source":"cron","event":"schedule"}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleTrigger_NoLauncher(t *testing.T) {
	s := NewServer(":0", nil, nil)

	body := `{"source":"cron","event":"schedule"}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestHandleResume_Success(t *testing.T) {
	resumed := false
	s := NewServer(":0", nil, func(_ context.Context, runID string, payload map[string]any) error {
		resumed = true
		if runID != "run-456" {
			t.Errorf("run_id = %q, want %q", runID, "run-456")
		}
		return nil
	})

	req := httptest.NewRequest("POST", "/api/resume/run-456", bytes.NewBufferString(`{"payload":{"result":"ok"}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if !resumed {
		t.Fatal("resumer was not called")
	}

	var resp ResumeResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.RunID != "run-456" {
		t.Errorf("run_id = %q, want %q", resp.RunID, "run-456")
	}
	if resp.Status != "resumed" {
		t.Errorf("status = %q, want %q", resp.Status, "resumed")
	}
}

func TestHandleResume_MissingRunID(t *testing.T) {
	s := NewServer(":0", nil, func(_ context.Context, _ string, _ map[string]any) error {
		return nil
	})

	req := httptest.NewRequest("POST", "/api/resume/", nil)
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleResume_NoResumer(t *testing.T) {
	s := NewServer(":0", nil, nil)

	req := httptest.NewRequest("POST", "/api/resume/run-123", nil)
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestHandleHealth(t *testing.T) {
	s := NewServer(":0", nil, nil)

	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()

	s.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %q, want %q", resp["status"], "ok")
	}
}

func TestRuns_TracksLaunches(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "tracked-run", nil
	}, nil)

	body := `{"source":"test","event":"fire"}`
	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleTrigger(w, req)

	runs := s.Runs()
	if _, ok := runs["tracked-run"]; !ok {
		t.Error("run 'tracked-run' not tracked")
	}
}

func TestServer_StartAndShutdown(t *testing.T) {
	s := NewServer("127.0.0.1:0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "ok", nil
	}, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}
