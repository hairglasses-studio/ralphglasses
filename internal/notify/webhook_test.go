package notify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// --- helpers ---

func testEvent(evType events.EventType) events.Event {
	return events.Event{
		Type:      evType,
		Version:   1,
		Timestamp: time.Date(2026, 3, 30, 12, 0, 0, 0, time.UTC),
		SessionID: "sess-001",
		RepoName:  "ralphglasses",
		RepoPath:  "/tmp/ralphglasses",
		Provider:  "claude",
		Data:      map[string]any{"cost": 1.25},
	}
}

// collectServer records request bodies and lets the caller control the status code.
type collectServer struct {
	mu       sync.Mutex
	requests []capturedReq
	status   int
}

type capturedReq struct {
	Headers http.Header
	Body    []byte
}

func (cs *collectServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	cs.mu.Lock()
	cs.requests = append(cs.requests, capturedReq{Headers: r.Header.Clone(), Body: body})
	status := cs.status
	cs.mu.Unlock()
	if status == 0 {
		status = 200
	}
	w.WriteHeader(status)
}

func (cs *collectServer) Requests() []capturedReq {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	out := make([]capturedReq, len(cs.requests))
	copy(out, cs.requests)
	return out
}

func (cs *collectServer) SetStatus(code int) {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.status = code
}

// --- Dispatcher tests ---

func TestDispatcher_BasicDelivery(t *testing.T) {
	t.Parallel()
	handler := &collectServer{}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts.URL, Secret: "test-secret"},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	// Give the dispatcher time to process.
	time.Sleep(200 * time.Millisecond)

	reqs := handler.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 request")
	}

	// Verify headers.
	req := reqs[0]
	if got := req.Headers.Get("X-Ralph-Event"); got != string(events.SessionStarted) {
		t.Errorf("X-Ralph-Event = %q; want %q", got, events.SessionStarted)
	}
	if got := req.Headers.Get("X-Ralph-Timestamp"); got == "" {
		t.Error("X-Ralph-Timestamp header missing")
	}
	if got := req.Headers.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q; want application/json", got)
	}

	// Verify payload.
	var payload WebhookPayload
	if err := json.Unmarshal(req.Body, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Event != events.SessionStarted {
		t.Errorf("payload.Event = %q; want %q", payload.Event, events.SessionStarted)
	}
	if payload.SessionID != "sess-001" {
		t.Errorf("payload.SessionID = %q; want sess-001", payload.SessionID)
	}
}

func TestDispatcher_EventFilter(t *testing.T) {
	t.Parallel()
	handler := &collectServer{}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{
			URL:    ts.URL,
			Events: []string{string(events.BudgetAlert), string(events.BudgetExceeded)},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	// Publish an event that should NOT match the filter.
	bus.Publish(testEvent(events.SessionStarted))
	time.Sleep(200 * time.Millisecond)

	if len(handler.Requests()) != 0 {
		t.Errorf("expected 0 requests for filtered-out event, got %d", len(handler.Requests()))
	}

	// Publish an event that SHOULD match.
	bus.Publish(testEvent(events.BudgetAlert))
	time.Sleep(200 * time.Millisecond)

	if len(handler.Requests()) != 1 {
		t.Errorf("expected 1 request for matching event, got %d", len(handler.Requests()))
	}
}

func TestDispatcher_HMACSignature(t *testing.T) {
	t.Parallel()
	handler := &collectServer{}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	secret := "my-webhook-secret"
	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts.URL, Secret: secret},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	time.Sleep(200 * time.Millisecond)

	reqs := handler.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 request")
	}

	sig := reqs[0].Headers.Get("X-Ralph-Signature")
	if sig == "" {
		t.Fatal("X-Ralph-Signature header missing")
	}

	if !VerifySignature(reqs[0].Body, secret, sig) {
		t.Error("HMAC signature verification failed")
	}

	// Wrong secret should fail.
	if VerifySignature(reqs[0].Body, "wrong-secret", sig) {
		t.Error("HMAC verification should fail with wrong secret")
	}
}

func TestDispatcher_NoSignatureWithoutSecret(t *testing.T) {
	t.Parallel()
	handler := &collectServer{}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts.URL}, // no secret
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	time.Sleep(200 * time.Millisecond)

	reqs := handler.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 request")
	}

	if got := reqs[0].Headers.Get("X-Ralph-Signature"); got != "" {
		t.Errorf("expected no signature header without secret, got %q", got)
	}
}

func TestDispatcher_RetryOn500(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		n := callCount.Add(1)
		if n <= 2 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts.URL, MaxRetries: 3, Timeout: 2 * time.Second},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	// Wait for initial attempt + 2 retries (1s + 2s backoff).
	time.Sleep(5 * time.Second)

	got := int(callCount.Load())
	if got < 3 {
		t.Errorf("expected at least 3 attempts (initial + 2 retries), got %d", got)
	}
}

func TestDispatcher_CircuitBreaker(t *testing.T) {
	t.Parallel()
	var callCount atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body) //nolint:errcheck
		callCount.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts.URL, MaxRetries: -1, Timeout: 1 * time.Second}, // no retries, fail fast
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	// Send 5 events one at a time to trip the circuit breaker (threshold = 5).
	// Wait between each to ensure sequential processing.
	for i := 0; i < 5; i++ {
		bus.Publish(testEvent(events.SessionStarted))
		time.Sleep(200 * time.Millisecond)
	}

	// Wait for all 5 to be processed.
	time.Sleep(500 * time.Millisecond)
	countBefore := int(callCount.Load())
	if countBefore < 5 {
		t.Fatalf("expected at least 5 calls before circuit trips, got %d", countBefore)
	}

	// The circuit should be open now. Send more events — they should be skipped.
	for i := 0; i < 3; i++ {
		bus.Publish(testEvent(events.SessionStarted))
		time.Sleep(200 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)

	countAfter := int(callCount.Load())
	if countAfter > countBefore {
		t.Errorf("circuit breaker should prevent deliveries; calls before=%d, after=%d", countBefore, countAfter)
	}

	// Verify the circuit state.
	cs := d.circuits[ts.URL]
	if cs.ConsecutiveFailures() < 5 {
		t.Errorf("expected at least 5 consecutive failures, got %d", cs.ConsecutiveFailures())
	}
}

func TestDispatcher_CustomHeaders(t *testing.T) {
	t.Parallel()
	handler := &collectServer{}
	ts := httptest.NewServer(handler)
	defer ts.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{
			URL: ts.URL,
			Headers: map[string]string{
				"Authorization": "Bearer tok123",
				"X-Custom":      "hello",
			},
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	time.Sleep(200 * time.Millisecond)

	reqs := handler.Requests()
	if len(reqs) == 0 {
		t.Fatal("expected at least 1 request")
	}
	if got := reqs[0].Headers.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization = %q; want Bearer tok123", got)
	}
	if got := reqs[0].Headers.Get("X-Custom"); got != "hello" {
		t.Errorf("X-Custom = %q; want hello", got)
	}
}

func TestDispatcher_MultipleConfigs(t *testing.T) {
	t.Parallel()
	handler1 := &collectServer{}
	handler2 := &collectServer{}
	ts1 := httptest.NewServer(handler1)
	ts2 := httptest.NewServer(handler2)
	defer ts1.Close()
	defer ts2.Close()

	bus := events.NewBus(100)
	d := NewDispatcher(bus, []WebhookConfig{
		{URL: ts1.URL, Events: []string{string(events.SessionStarted)}},
		{URL: ts2.URL, Events: []string{string(events.BudgetAlert)}},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	d.Start(ctx)
	defer d.Stop()

	bus.Publish(testEvent(events.SessionStarted))
	bus.Publish(testEvent(events.BudgetAlert))
	time.Sleep(300 * time.Millisecond)

	if len(handler1.Requests()) != 1 {
		t.Errorf("server1 expected 1 request, got %d", len(handler1.Requests()))
	}
	if len(handler2.Requests()) != 1 {
		t.Errorf("server2 expected 1 request, got %d", len(handler2.Requests()))
	}
}

// --- Slack formatting tests ---

func TestFormatSlack_InfoEvent(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.SessionStarted)
	payload := FormatSlack(ev)

	if len(payload.Attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}

	att := payload.Attachments[0]
	if att.Color != "#2EB67D" {
		t.Errorf("color = %q; want green (#2EB67D) for info event", att.Color)
	}

	// Verify header block exists.
	if len(att.Blocks) < 2 {
		t.Fatalf("expected at least 2 blocks, got %d", len(att.Blocks))
	}
	header := att.Blocks[0]
	if header.Type != "header" {
		t.Errorf("first block type = %q; want header", header.Type)
	}
	if header.Text == nil || !strings.Contains(header.Text.Text, "INFO") {
		t.Errorf("header should contain INFO; got %v", header.Text)
	}
}

func TestFormatSlack_ErrorEvent(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.BudgetExceeded)
	payload := FormatSlack(ev)

	if len(payload.Attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}
	if payload.Attachments[0].Color != "#E01E5A" {
		t.Errorf("color = %q; want red (#E01E5A) for error event", payload.Attachments[0].Color)
	}

	header := payload.Attachments[0].Blocks[0]
	if header.Text == nil || !strings.Contains(header.Text.Text, "ERROR") {
		t.Errorf("header should contain ERROR; got %v", header.Text)
	}
}

func TestFormatSlack_WarningEvent(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.BudgetAlert)
	payload := FormatSlack(ev)

	if len(payload.Attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}
	if payload.Attachments[0].Color != "#ECB22E" {
		t.Errorf("color = %q; want yellow (#ECB22E) for warning event", payload.Attachments[0].Color)
	}
}

func TestFormatSlack_FieldsPopulated(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.SessionStarted)
	payload := FormatSlack(ev)

	att := payload.Attachments[0]
	fieldBlock := att.Blocks[1]
	if fieldBlock.Type != "section" {
		t.Fatalf("second block type = %q; want section", fieldBlock.Type)
	}

	// Should have at least event type, timestamp, session, repo, provider fields.
	if len(fieldBlock.Fields) < 5 {
		t.Errorf("expected at least 5 fields, got %d", len(fieldBlock.Fields))
	}

	// Verify event type field is present.
	found := false
	for _, f := range fieldBlock.Fields {
		if strings.Contains(f.Text, string(events.SessionStarted)) {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected event type to appear in fields")
	}
}

func TestFormatSlack_DataSection(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.CostUpdate)
	ev.Data = map[string]any{"cost": 3.50, "currency": "USD"}
	payload := FormatSlack(ev)

	att := payload.Attachments[0]
	// With data, we should have 3 blocks: header, fields, data.
	if len(att.Blocks) < 3 {
		t.Fatalf("expected 3 blocks with data, got %d", len(att.Blocks))
	}

	dataBlock := att.Blocks[2]
	if dataBlock.Type != "section" {
		t.Errorf("data block type = %q; want section", dataBlock.Type)
	}
	if dataBlock.Text == nil {
		t.Fatal("data block text is nil")
	}
	if !strings.Contains(dataBlock.Text.Text, "cost") {
		t.Errorf("data block should contain 'cost'; got %q", dataBlock.Text.Text)
	}
}

func TestFormatSlack_MinimalEvent(t *testing.T) {
	t.Parallel()
	ev := events.Event{
		Type:      events.ConfigChanged,
		Timestamp: time.Now(),
	}
	payload := FormatSlack(ev)

	if len(payload.Attachments) == 0 {
		t.Fatal("expected at least 1 attachment")
	}

	// With no session/repo/provider, field count should be smaller.
	fieldBlock := payload.Attachments[0].Blocks[1]
	// At minimum: event type + timestamp = 2
	if len(fieldBlock.Fields) < 2 {
		t.Errorf("expected at least 2 fields for minimal event, got %d", len(fieldBlock.Fields))
	}
}

func TestFormatSlack_JSONRoundTrip(t *testing.T) {
	t.Parallel()
	ev := testEvent(events.SessionStarted)
	payload := FormatSlack(ev)

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded SlackPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(decoded.Attachments) != len(payload.Attachments) {
		t.Errorf("attachment count mismatch after round trip: %d vs %d",
			len(decoded.Attachments), len(payload.Attachments))
	}
}

// --- VerifySignature tests ---

func TestVerifySignature_Valid(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"test"}`)
	secret := "secret123"
	sig := computeHMAC(body, []byte(secret))

	if !VerifySignature(body, secret, sig) {
		t.Error("expected valid signature to verify")
	}
}

func TestVerifySignature_Invalid(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"test"}`)
	if VerifySignature(body, "secret1", "deadbeef") {
		t.Error("expected invalid signature to fail")
	}
}

func TestVerifySignature_TamperedBody(t *testing.T) {
	t.Parallel()
	body := []byte(`{"event":"test"}`)
	secret := "secret123"
	sig := computeHMAC(body, []byte(secret))

	tampered := []byte(`{"event":"tampered"}`)
	if VerifySignature(tampered, secret, sig) {
		t.Error("expected tampered body to fail verification")
	}
}
