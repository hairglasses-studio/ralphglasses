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

const (
	testTenant = "tenant-alpha"
	testToken  = "token-alpha"
)

func testAuthorizer(tokens map[string]string) TenantAuthorizer {
	return func(_ context.Context, token string) (string, error) {
		return tokens[token], nil
	}
}

func authRequest(method, path, body string) *http.Request {
	var buf *bytes.Buffer
	if body == "" {
		buf = bytes.NewBuffer(nil)
	} else {
		buf = bytes.NewBufferString(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Authorization", "Bearer "+testToken)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func TestHandleTrigger_Success(t *testing.T) {
	launched := false
	s := NewServer(":0", func(_ context.Context, req TriggerRequest) (string, error) {
		launched = true
		if req.TenantID != testTenant {
			t.Errorf("tenant_id = %q, want %q", req.TenantID, testTenant)
		}
		if req.Source != "github" {
			t.Errorf("source = %q, want %q", req.Source, "github")
		}
		if req.Event != "push" {
			t.Errorf("event = %q, want %q", req.Event, "push")
		}
		return "run-123", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"source":"github","event":"push","payload":{"ref":"refs/heads/main"}}`)
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
	if resp.TenantID != testTenant {
		t.Errorf("tenant_id = %q, want %q", resp.TenantID, testTenant)
	}
	if resp.Status != "launched" {
		t.Errorf("status = %q, want %q", resp.Status, "launched")
	}
}

func TestHandleTrigger_MissingBearer(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(`{"source":"github","event":"push"}`))
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestHandleTrigger_InvalidToken(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{}))

	req := httptest.NewRequest("POST", "/api/trigger", bytes.NewBufferString(`{"source":"github","event":"push"}`))
	req.Header.Set("Authorization", "Bearer invalid")
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleTrigger_BodyTenantCannotOverrideBearer(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"tenant_id":"other","source":"github","event":"push"}`)
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleTrigger_MissingSource(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"event":"push"}`)
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_MissingEvent(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"source":"github"}`)
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_InvalidJSON(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", nil
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", "not json")
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleTrigger_LaunchError(t *testing.T) {
	s := NewServer(":0", func(_ context.Context, _ TriggerRequest) (string, error) {
		return "", fmt.Errorf("provider down")
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"source":"cron","event":"schedule"}`)
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleTrigger_NoLauncher(t *testing.T) {
	s := NewServer(":0", nil, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"source":"cron","event":"schedule"}`)
	w := httptest.NewRecorder()

	s.handleTrigger(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestHandleResume_Success(t *testing.T) {
	resumed := false
	s := NewServer(":0", nil, func(_ context.Context, tenantID, runID string, payload map[string]any) error {
		resumed = true
		if tenantID != testTenant {
			t.Errorf("tenant_id = %q, want %q", tenantID, testTenant)
		}
		if runID != "run-456" {
			t.Errorf("run_id = %q, want %q", runID, "run-456")
		}
		if payload["result"] != "ok" {
			t.Errorf("payload result = %v, want ok", payload["result"])
		}
		return nil
	}, testAuthorizer(map[string]string{testToken: testTenant}))
	s.runs["run-456"] = trackedRun{TenantID: testTenant, CreatedAt: time.Now()}

	req := authRequest("POST", "/api/resume/run-456", `{"payload":{"result":"ok"}}`)
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
	if resp.TenantID != testTenant {
		t.Errorf("tenant_id = %q, want %q", resp.TenantID, testTenant)
	}
	if resp.Status != "resumed" {
		t.Errorf("status = %q, want %q", resp.Status, "resumed")
	}
}

func TestHandleResume_CrossTenantRejected(t *testing.T) {
	s := NewServer(":0", nil, func(_ context.Context, _, _ string, _ map[string]any) error {
		return nil
	}, testAuthorizer(map[string]string{testToken: testTenant}))
	s.runs["run-789"] = trackedRun{TenantID: "other-tenant", CreatedAt: time.Now()}

	req := authRequest("POST", "/api/resume/run-789", "")
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestHandleResume_MissingRunID(t *testing.T) {
	s := NewServer(":0", nil, func(_ context.Context, _, _ string, _ map[string]any) error {
		return nil
	}, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/resume/", "")
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleResume_NoResumer(t *testing.T) {
	s := NewServer(":0", nil, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/resume/run-123", "")
	w := httptest.NewRecorder()

	s.handleResume(w, req)

	if w.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotImplemented)
	}
}

func TestHandleHealth(t *testing.T) {
	s := NewServer(":0", nil, nil, nil)

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
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	req := authRequest("POST", "/api/trigger", `{"source":"test","event":"fire"}`)
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
	}, nil, testAuthorizer(map[string]string{testToken: testTenant}))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := s.Start(ctx)
	if err != nil {
		t.Fatalf("Start returned error: %v", err)
	}
}
