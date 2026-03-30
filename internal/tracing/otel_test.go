package tracing

import (
	"bytes"
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// newTestRecorder creates an OTelRecorder backed by an in-memory exporter
// for assertions. The exporter is returned so tests can inspect spans.
func newTestRecorder(t *testing.T) (*OTelRecorder, *tracetest.InMemoryExporter) {
	t.Helper()
	exp := tracetest.NewInMemoryExporter()
	res, err := resource.New(context.Background(),
		resource.WithAttributes(semconv.ServiceName("ralphglasses-test")),
	)
	if err != nil {
		t.Fatalf("resource.New: %v", err)
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exp),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
	)
	rec := newOTelRecorderFromProvider(tp)
	return rec, exp
}

func TestOTelRecorder_SpanCreationAndAttributes(t *testing.T) {
	rec, exp := newTestRecorder(t)

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-42", "claude", "sonnet-4", "my-repo")
	if span == nil {
		t.Fatal("StartSessionSpan returned nil span")
	}
	if ctx == nil {
		t.Fatal("StartSessionSpan returned nil context")
	}

	rec.EndSessionSpan(span, 1.25, 8, "completed")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	if s.Name != "session" {
		t.Errorf("span name = %q, want session", s.Name)
	}

	// Verify launch-time attributes.
	assertAttr(t, s.Attributes, AttrGenAISessionID, "sess-42")
	assertAttr(t, s.Attributes, AttrGenAIProvider, "claude")
	assertAttr(t, s.Attributes, AttrGenAIModel, "sonnet-4")
	assertAttr(t, s.Attributes, AttrGenAIRepoName, "my-repo")

	// Verify end-time attributes.
	assertAttrFloat(t, s.Attributes, AttrGenAICostUSD, 1.25)
	assertAttrInt(t, s.Attributes, AttrGenAITurnCount, 8)
	assertAttr(t, s.Attributes, AttrGenAIExitReason, "completed")
}

func TestOTelRecorder_IterationSpanParentChild(t *testing.T) {
	rec, exp := newTestRecorder(t)

	// Create session span (parent).
	ctx, span := rec.StartSessionSpan(context.Background(), "sess-parent", "gemini", "pro", "repo")

	// Create iteration spans (children).
	ctx1, iter1 := rec.StartIterationSpan(ctx, "sess-parent", 1)
	_ = ctx1
	iter1.End()

	ctx2, iter2 := rec.StartIterationSpan(ctx, "sess-parent", 2)
	_ = ctx2
	iter2.End()

	rec.EndSessionSpan(span, 0.5, 2, "done")

	spans := exp.GetSpans()
	if len(spans) != 3 {
		t.Fatalf("expected 3 spans (1 session + 2 iterations), got %d", len(spans))
	}

	// Find session span and iteration spans.
	var sessionSpanID, sessionTraceID string
	iterParentIDs := map[string]bool{}

	for _, s := range spans {
		if s.Name == "session" {
			sessionSpanID = s.SpanContext.SpanID().String()
			sessionTraceID = s.SpanContext.TraceID().String()
		}
	}
	if sessionSpanID == "" {
		t.Fatal("no session span found")
	}

	for _, s := range spans {
		if s.Name == "iteration" {
			iterParentIDs[s.Parent.SpanID().String()] = true
			// Verify same trace ID.
			if s.SpanContext.TraceID().String() != sessionTraceID {
				t.Errorf("iteration span trace ID %s != session trace ID %s",
					s.SpanContext.TraceID(), sessionTraceID)
			}
		}
	}

	// Both iteration spans should have the session span as parent.
	if !iterParentIDs[sessionSpanID] {
		t.Errorf("iteration spans should be children of session span %s, parents: %v",
			sessionSpanID, iterParentIDs)
	}
}

func TestOTelRecorder_IterationSpanAttributes(t *testing.T) {
	rec, exp := newTestRecorder(t)

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-iter", "openai", "gpt-4o", "repo")
	_, iterSpan := rec.StartIterationSpan(ctx, "sess-iter", 5)
	iterSpan.End()
	rec.EndSessionSpan(span, 0.0, 1, "done")

	spans := exp.GetSpans()
	for _, s := range spans {
		if s.Name == "iteration" {
			assertAttr(t, s.Attributes, AttrGenAISessionID, "sess-iter")
			assertAttrInt(t, s.Attributes, "gen_ai.iteration", 5)
			return
		}
	}
	t.Fatal("no iteration span found")
}

func TestOTelRecorder_RecordError(t *testing.T) {
	rec, exp := newTestRecorder(t)

	_, span := rec.StartSessionSpan(context.Background(), "sess-err", "claude", "opus", "repo")
	rec.RecordError(span, "context deadline exceeded")
	rec.EndSessionSpan(span, 0.0, 0, "error")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	// Span should have error status.
	if s.Status.Code.String() != "Error" {
		t.Errorf("span status = %s, want Error", s.Status.Code)
	}
	if s.Status.Description != "context deadline exceeded" {
		t.Errorf("span status description = %q, want 'context deadline exceeded'", s.Status.Description)
	}

	// Should have an exception event.
	foundErr := false
	for _, ev := range s.Events {
		if ev.Name == "exception" {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Error("span should have an exception event from RecordError")
	}

	// Internal SessionSpan should also record.
	span.mu.Lock()
	defer span.mu.Unlock()
	if len(span.events) != 1 {
		t.Fatalf("internal span should have 1 event, got %d", len(span.events))
	}
}

func TestOTelRecorder_RecordError_NilSpan(t *testing.T) {
	rec, _ := newTestRecorder(t)
	// Should not panic.
	rec.RecordError(nil, "some error")
}

func TestOTelRecorder_RecordTurnMetric(t *testing.T) {
	rec, exp := newTestRecorder(t)

	_, span := rec.StartSessionSpan(context.Background(), "sess-turn", "claude", "haiku", "repo")
	rec.RecordTurnMetric(context.Background(), "claude", "haiku", "sess-turn", 500, 1000, 0.03, 250)
	rec.RecordTurnMetric(context.Background(), "claude", "haiku", "sess-turn", 600, 1200, 0.04, 300)
	rec.EndSessionSpan(span, 0.07, 2, "done")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	s := spans[0]
	turnEvents := 0
	for _, ev := range s.Events {
		if ev.Name == "turn" {
			turnEvents++
		}
	}
	if turnEvents != 2 {
		t.Errorf("expected 2 turn events, got %d", turnEvents)
	}
}

func TestOTelRecorder_RecordEvent(t *testing.T) {
	rec, exp := newTestRecorder(t)

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-ev", "claude", "sonnet", "repo")
	rec.RecordEvent(ctx, "prompt_enhanced",
		attribute.String("original_grade", "C"),
		attribute.String("enhanced_grade", "A"),
	)
	rec.EndSessionSpan(span, 0.0, 1, "done")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, ev := range spans[0].Events {
		if ev.Name == "prompt_enhanced" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected prompt_enhanced event on span")
	}
}

func TestOTelRecorder_EndSessionSpan_NilSpan(t *testing.T) {
	rec, _ := newTestRecorder(t)
	// Should not panic.
	rec.EndSessionSpan(nil, 0.0, 0, "")
}

func TestOTelRecorder_Shutdown(t *testing.T) {
	rec, exp := newTestRecorder(t)

	// Create and end a span.
	_, span := rec.StartSessionSpan(context.Background(), "sess-shut", "claude", "sonnet", "repo")
	rec.EndSessionSpan(span, 0.0, 0, "done")

	// With a synchronous exporter the span is already available.
	if len(exp.GetSpans()) != 1 {
		t.Fatalf("expected 1 span before shutdown, got %d", len(exp.GetSpans()))
	}

	// Shutdown should complete without error.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rec.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestOTelRecorder_ImplementsRecorder(t *testing.T) {
	rec, _ := newTestRecorder(t)
	// Compile-time check: OTelRecorder satisfies Recorder.
	var _ Recorder = rec
}

func TestNewOTelRecorder_StdoutExporter(t *testing.T) {
	// Use a buffer so stdout exporter doesn't pollute test output.
	var buf bytes.Buffer
	rec, err := NewOTelRecorder(OTelConfig{
		ServiceName:  "test-svc",
		ExporterType: ExporterStdout,
		SampleRate:   1.0,
		Writer:       &buf,
	})
	if err != nil {
		t.Fatalf("NewOTelRecorder: %v", err)
	}
	defer rec.Shutdown(context.Background())

	if rec.provider == nil {
		t.Error("TracerProvider should not be nil")
	}
	if rec.tracer == nil {
		t.Error("Tracer should not be nil")
	}
}

func TestNewOTelRecorder_DefaultServiceName(t *testing.T) {
	var buf bytes.Buffer
	rec, err := NewOTelRecorder(OTelConfig{
		ExporterType: ExporterStdout,
		Writer:       &buf,
	})
	if err != nil {
		t.Fatalf("NewOTelRecorder: %v", err)
	}
	defer rec.Shutdown(context.Background())
	// Should not error even with empty service name.
}

func TestNewOTelRecorder_InvalidExporter(t *testing.T) {
	_, err := NewOTelRecorder(OTelConfig{
		ExporterType: "unknown-backend",
	})
	if err == nil {
		t.Fatal("expected error for unknown exporter type")
	}
}

func TestNewOTelRecorder_OTLPRequiresExtraDeps(t *testing.T) {
	_, err := NewOTelRecorder(OTelConfig{
		ExporterType: ExporterOTLP,
	})
	if err == nil {
		t.Fatal("expected error for OTLP without extra deps")
	}
}

func TestNewOTelRecorder_SampleRateClamping(t *testing.T) {
	var buf bytes.Buffer
	// Negative rate should be clamped to 1.0.
	rec, err := NewOTelRecorder(OTelConfig{
		ExporterType: ExporterStdout,
		SampleRate:   -1.0,
		Writer:       &buf,
	})
	if err != nil {
		t.Fatalf("NewOTelRecorder: %v", err)
	}
	defer rec.Shutdown(context.Background())

	// Rate > 1.0 should be clamped.
	rec2, err := NewOTelRecorder(OTelConfig{
		ExporterType: ExporterStdout,
		SampleRate:   5.0,
		Writer:       &buf,
	})
	if err != nil {
		t.Fatalf("NewOTelRecorder: %v", err)
	}
	defer rec2.Shutdown(context.Background())
}

func TestOTelRecorder_RecordCostMetric(t *testing.T) {
	rec, exp := newTestRecorder(t)

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-cost", "claude", "sonnet", "repo")
	rec.RecordCostMetric(ctx, "claude", "repo", 2.50)
	rec.EndSessionSpan(span, 2.50, 5, "done")

	spans := exp.GetSpans()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	found := false
	for _, ev := range spans[0].Events {
		if ev.Name == "cost" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cost event on span")
	}
}

func TestOTelRecorder_TracerProvider(t *testing.T) {
	rec, _ := newTestRecorder(t)
	if rec.TracerProvider() == nil {
		t.Error("TracerProvider() should not return nil")
	}
}

// ---------- helpers ----------

func assertAttr(t *testing.T, attrs []attribute.KeyValue, key, want string) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if got := a.Value.AsString(); got != want {
				t.Errorf("attr %s = %q, want %q", key, got, want)
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}

func assertAttrFloat(t *testing.T, attrs []attribute.KeyValue, key string, want float64) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if got := a.Value.AsFloat64(); got != want {
				t.Errorf("attr %s = %f, want %f", key, got, want)
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}

func assertAttrInt(t *testing.T, attrs []attribute.KeyValue, key string, want int) {
	t.Helper()
	for _, a := range attrs {
		if string(a.Key) == key {
			if got := a.Value.AsInt64(); got != int64(want) {
				t.Errorf("attr %s = %d, want %d", key, got, want)
			}
			return
		}
	}
	t.Errorf("attribute %s not found", key)
}
