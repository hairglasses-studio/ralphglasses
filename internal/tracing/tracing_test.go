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
