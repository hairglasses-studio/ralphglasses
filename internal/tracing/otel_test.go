package tracing

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestNewTracer_EmptyServiceName(t *testing.T) {
	t.Parallel()
	_, err := NewTracer("")
	if err == nil {
		t.Fatal("expected error for empty service name")
	}
}

func TestNewTracer_NoopMode(t *testing.T) {
	t.Parallel()
	// No endpoint configured -> noop mode.
	tr, err := NewTracer("test-svc")
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())

	ctx, span := tr.StartSpan(context.Background(), "op1")
	if span == nil {
		t.Fatal("span should not be nil")
	}
	if ctx == nil {
		t.Fatal("ctx should not be nil")
	}
	span.SetAttribute("key", "value")
	span.SetStatus(StatusOK, "all good")
	span.AddEvent("checkpoint")
	span.End()

	// No panic, no export error.
}

func TestTracer_SpanExportToCollector(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var received []otlpExportRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}
		var req otlpExportRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("unmarshal: %v", err)
			return
		}
		mu.Lock()
		received = append(received, req)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(1), // flush on every span
		WithSampling(1.0),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	_, span := tr.StartSpan(context.Background(), "test-op")
	span.SetAttribute("user", "alice")
	span.SetAttribute("count", 42)
	span.SetAttribute("rate", 0.95)
	span.SetAttribute("active", true)
	span.SetStatus(StatusOK, "done")
	span.AddEvent("started", map[string]any{"phase": "init"})
	span.End()

	// Give a moment for the async export.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 export request, got %d", len(received))
	}

	req := received[0]
	if len(req.ResourceSpans) != 1 {
		t.Fatalf("expected 1 resourceSpan, got %d", len(req.ResourceSpans))
	}

	rs := req.ResourceSpans[0]
	// Verify service.name resource attribute.
	foundSvc := false
	for _, attr := range rs.Resource.Attributes {
		if attr.Key == "service.name" && attr.Value.StringValue != nil && *attr.Value.StringValue == "test-svc" {
			foundSvc = true
		}
	}
	if !foundSvc {
		t.Error("service.name resource attribute not found")
	}

	if len(rs.ScopeSpans) != 1 {
		t.Fatalf("expected 1 scopeSpan, got %d", len(rs.ScopeSpans))
	}

	spans := rs.ScopeSpans[0].Spans
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "test-op" {
		t.Errorf("span name = %q, want test-op", s.Name)
	}
	if s.TraceID == "" || len(s.TraceID) != 32 {
		t.Errorf("traceID should be 32 hex chars, got %q", s.TraceID)
	}
	if s.SpanID == "" || len(s.SpanID) != 16 {
		t.Errorf("spanID should be 16 hex chars, got %q", s.SpanID)
	}
	if s.Kind != 1 {
		t.Errorf("kind = %d, want 1 (INTERNAL)", s.Kind)
	}
	if s.Status == nil || s.Status.Code != int(StatusOK) {
		t.Errorf("status code = %v, want %d", s.Status, StatusOK)
	}

	// Verify attributes.
	attrMap := make(map[string]otlpValue)
	for _, a := range s.Attributes {
		attrMap[a.Key] = a.Value
	}
	if v, ok := attrMap["user"]; !ok || v.StringValue == nil || *v.StringValue != "alice" {
		t.Errorf("missing or wrong 'user' attribute")
	}
	if v, ok := attrMap["count"]; !ok || v.IntValue == nil || *v.IntValue != 42 {
		t.Errorf("missing or wrong 'count' attribute")
	}
	if v, ok := attrMap["rate"]; !ok || v.DoubleValue == nil || *v.DoubleValue != 0.95 {
		t.Errorf("missing or wrong 'rate' attribute")
	}
	if v, ok := attrMap["active"]; !ok || v.BoolValue == nil || *v.BoolValue != true {
		t.Errorf("missing or wrong 'active' attribute")
	}

	// Verify events.
	if len(s.Events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(s.Events))
	}
	if s.Events[0].Name != "started" {
		t.Errorf("event name = %q, want started", s.Events[0].Name)
	}
}

func TestTracer_ParentChildPropagation(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var received []otlpExportRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req otlpExportRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		received = append(received, req)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(2),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	ctx, parent := tr.StartSpan(context.Background(), "parent-op")
	_, child := tr.StartSpan(ctx, "child-op")

	child.End()
	parent.End()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// Both spans should have been flushed (batch of 2).
	if len(received) != 1 {
		t.Fatalf("expected 1 batch, got %d", len(received))
	}

	spans := received[0].ResourceSpans[0].ScopeSpans[0].Spans
	if len(spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(spans))
	}

	spanMap := make(map[string]otlpSpan)
	for _, s := range spans {
		spanMap[s.Name] = s
	}

	parentSpan := spanMap["parent-op"]
	childSpan := spanMap["child-op"]

	// Same trace ID.
	if parentSpan.TraceID != childSpan.TraceID {
		t.Errorf("parent trace %q != child trace %q", parentSpan.TraceID, childSpan.TraceID)
	}

	// Child's parent should be the parent span.
	if childSpan.ParentSpanID != parentSpan.SpanID {
		t.Errorf("child parentSpanID %q != parent spanID %q", childSpan.ParentSpanID, parentSpan.SpanID)
	}

	// Parent should have no parent.
	if parentSpan.ParentSpanID != "" {
		t.Errorf("parent should have no parent, got %q", parentSpan.ParentSpanID)
	}
}

func TestTracer_Shutdown_FlushesBuffer(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var received []otlpExportRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req otlpExportRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		received = append(received, req)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(100), // large batch so it won't auto-flush
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	_, span := tr.StartSpan(context.Background(), "buffered-op")
	span.End()

	// Nothing should be exported yet (batch not full).
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	preShutdown := len(received)
	mu.Unlock()
	if preShutdown != 0 {
		t.Fatalf("expected 0 exports before shutdown, got %d", preShutdown)
	}

	// Shutdown should flush.
	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(received) != 1 {
		t.Fatalf("expected 1 export after shutdown, got %d", len(received))
	}
}

func TestTracer_SamplingZero_DropsSpans(t *testing.T) {
	t.Parallel()

	exportCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		exportCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithSampling(0.0),
		WithBatchSize(1),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	for i := 0; i < 10; i++ {
		_, span := tr.StartSpan(context.Background(), "dropped")
		span.End()
	}

	time.Sleep(50 * time.Millisecond)
	tr.Shutdown(context.Background())

	if exportCount != 0 {
		t.Errorf("expected 0 exports with 0%% sampling, got %d", exportCount)
	}
}

func TestSpan_EndIdempotent(t *testing.T) {
	t.Parallel()

	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	_, span := tr.StartSpan(context.Background(), "op")
	span.End()
	span.End() // second End() should be no-op
	span.End() // third End() should be no-op

	time.Sleep(50 * time.Millisecond)
	tr.Shutdown(context.Background())

	if callCount != 1 {
		t.Errorf("expected 1 export (idempotent End), got %d", callCount)
	}
}

func TestSpan_SetAfterEnd_Ignored(t *testing.T) {
	t.Parallel()
	tr, err := NewTracer("test-svc")
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())

	_, span := tr.StartSpan(context.Background(), "op")
	span.End()

	// These should be silently ignored.
	span.SetAttribute("key", "value")
	span.SetStatus(StatusError, "fail")
	span.AddEvent("late-event")

	span.mu.Lock()
	defer span.mu.Unlock()
	if _, ok := span.attributes["key"]; ok {
		t.Error("attribute should not be set after End()")
	}
	if span.statusCode != StatusUnset {
		t.Error("status should not change after End()")
	}
	if len(span.events) != 0 {
		t.Error("events should not be added after End()")
	}
}

func TestSpanFromContext(t *testing.T) {
	t.Parallel()
	tr, err := NewTracer("test-svc")
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())

	// Empty context.
	if s := SpanFromContext(context.Background()); s != nil {
		t.Error("expected nil span from empty context")
	}

	ctx, span := tr.StartSpan(context.Background(), "op")
	got := SpanFromContext(ctx)
	if got != span {
		t.Error("SpanFromContext should return the span set in context")
	}
}

func TestSpan_IDs(t *testing.T) {
	t.Parallel()
	tr, err := NewTracer("test-svc")
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())

	_, span := tr.StartSpan(context.Background(), "op")
	defer span.End()

	if len(span.TraceID()) != 32 {
		t.Errorf("traceID length = %d, want 32", len(span.TraceID()))
	}
	if len(span.SpanID()) != 16 {
		t.Errorf("spanID length = %d, want 16", len(span.SpanID()))
	}
}

func TestTracer_CollectorError(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	_, span := tr.StartSpan(context.Background(), "op")
	span.End()

	// Should not panic even when collector returns error.
	time.Sleep(50 * time.Millisecond)
	tr.Shutdown(context.Background())
}

func TestTracer_StatusError(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	var received []otlpExportRequest

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req otlpExportRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		received = append(received, req)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(1),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	_, span := tr.StartSpan(context.Background(), "failing-op")
	span.SetStatus(StatusError, "something went wrong")
	span.End()

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	if len(received) != 1 {
		t.Fatalf("expected 1 export, got %d", len(received))
	}

	s := received[0].ResourceSpans[0].ScopeSpans[0].Spans[0]
	if s.Status == nil {
		t.Fatal("expected status to be set")
	}
	if s.Status.Code != int(StatusError) {
		t.Errorf("status code = %d, want %d", s.Status.Code, StatusError)
	}
	if s.Status.Message != "something went wrong" {
		t.Errorf("status message = %q, want 'something went wrong'", s.Status.Message)
	}
}

func TestTracer_ConcurrentSpans(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	spanCount := 0

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req otlpExportRequest
		_ = json.Unmarshal(body, &req)
		mu.Lock()
		for _, rs := range req.ResourceSpans {
			for _, ss := range rs.ScopeSpans {
				spanCount += len(ss.Spans)
			}
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tr, err := NewTracer("test-svc",
		WithEndpoint(srv.URL),
		WithBatchSize(5),
		withHTTPClient(srv.Client()),
	)
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, span := tr.StartSpan(context.Background(), "concurrent-op")
			span.SetAttribute("goroutine", true)
			span.End()
		}()
	}
	wg.Wait()

	if err := tr.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	// Allow export to complete.
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if spanCount != 20 {
		t.Errorf("expected 20 spans exported, got %d", spanCount)
	}
}

func TestNewTracer_SamplingClamping(t *testing.T) {
	t.Parallel()

	// Negative sampling rate should be clamped.
	tr, err := NewTracer("test-svc", WithSampling(-5.0))
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())
	if tr.cfg.sampling != 0 {
		t.Errorf("sampling = %f, want 0", tr.cfg.sampling)
	}

	// >1.0 should be clamped.
	tr2, err := NewTracer("test-svc", WithSampling(5.0))
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr2.Shutdown(context.Background())
	if tr2.cfg.sampling != 1.0 {
		t.Errorf("sampling = %f, want 1.0", tr2.cfg.sampling)
	}
}

func TestNewTracer_BatchSizeClamping(t *testing.T) {
	t.Parallel()
	tr, err := NewTracer("test-svc", WithBatchSize(0))
	if err != nil {
		t.Fatalf("NewTracer: %v", err)
	}
	defer tr.Shutdown(context.Background())
	if tr.cfg.batchSize != 1 {
		t.Errorf("batchSize = %d, want 1", tr.cfg.batchSize)
	}
}
