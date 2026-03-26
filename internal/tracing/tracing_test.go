package tracing

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNoopRecorder(t *testing.T) {
	rec := &NoopRecorder{}
	ctx, span := rec.StartSessionSpan(context.Background(), "s1", "claude", "sonnet", "repo")
	if ctx == nil || span == nil {
		t.Fatal("noop should return non-nil")
	}
	rec.RecordTurnMetric(ctx, "claude", "sonnet", "s1", 100, 200, 0.01, 500)
	rec.RecordError(span, "some error")
	rec.RecordCostMetric(ctx, "claude", "repo", 0.50)
	rec.EndSessionSpan(span, 0.50, 10, "completed")
}

func TestInMemoryRecorder(t *testing.T) {
	rec := NewInMemoryRecorder()
	ctx, span := rec.StartSessionSpan(context.Background(), "s1", "claude", "sonnet", "myrepo")

	rec.RecordTurnMetric(ctx, "claude", "sonnet", "s1", 100, 200, 0.01, 500)
	rec.RecordTurnMetric(ctx, "claude", "sonnet", "s1", 150, 250, 0.02, 600)
	rec.RecordError(span, "timeout")
	rec.RecordCostMetric(ctx, "claude", "myrepo", 0.50)
	rec.EndSessionSpan(span, 0.50, 10, "completed")

	if rec.SpanCount() != 1 {
		t.Fatalf("expected 1 span, got %d", rec.SpanCount())
	}
	// 2 turns × 2 metrics/turn + 1 cost metric = 5
	if rec.MetricCount() != 5 {
		t.Fatalf("expected 5 metrics, got %d", rec.MetricCount())
	}
}

func TestGlobalRecorder(t *testing.T) {
	rec := NewInMemoryRecorder()
	SetRecorder(rec)
	defer SetRecorder(&NoopRecorder{})

	got := Get()
	if got != rec {
		t.Fatal("expected global recorder to match")
	}
}

func TestTraceIDGeneration(t *testing.T) {
	t.Parallel()
	id := NewTraceID()
	if len(id) != 16 {
		t.Fatalf("expected 16-char trace ID, got %d: %q", len(id), id)
	}
	// Verify it's valid hex.
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Fatalf("trace ID contains non-hex char %q: %s", c, id)
		}
	}
	// Two IDs should be unique.
	id2 := NewTraceID()
	if id == id2 {
		t.Fatal("two consecutive trace IDs should differ")
	}
}

func TestTraceIDContext(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// No trace ID in empty context.
	if got := TraceIDFromContext(ctx); got != "" {
		t.Fatalf("expected empty trace ID from fresh context, got %q", got)
	}

	// Round-trip.
	ctx = WithTraceID(ctx, "abc123def456abcd")
	if got := TraceIDFromContext(ctx); got != "abc123def456abcd" {
		t.Fatalf("expected abc123def456abcd, got %q", got)
	}
}

func TestPrometheusRecorder(t *testing.T) {
	inner := NewInMemoryRecorder()
	prom := NewPrometheusRecorder(inner)

	ctx, span := prom.StartSessionSpan(context.Background(), "s1", "claude", "sonnet", "repo")
	prom.RecordTurnMetric(ctx, "claude", "sonnet", "s1", 1000, 2000, 0.05, 300)
	prom.EndSessionSpan(span, 0.05, 5, "completed")

	// Test metrics endpoint
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	prom.Handler().ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "gen_ai_sessions_total") {
		t.Fatal("expected gen_ai_sessions_total in metrics output")
	}
	if !strings.Contains(body, "gen_ai_input_tokens_total") {
		t.Fatal("expected gen_ai_input_tokens_total in metrics output")
	}
	if !strings.Contains(body, `provider="claude"`) {
		t.Fatal("expected claude provider label in metrics output")
	}
}
