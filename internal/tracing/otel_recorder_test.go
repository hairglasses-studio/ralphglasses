package tracing

import (
	"context"
	"testing"
)

func TestOTelRecorder_StartEndSessionSpan(t *testing.T) {
	rec := NewOTelRecorder()

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-1", "claude", "opus-4-6", "myrepo")
	if span == nil {
		t.Fatal("expected non-nil span")
	}
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
	if span.sessionID != "sess-1" {
		t.Errorf("sessionID = %q, want %q", span.sessionID, "sess-1")
	}
	if span.provider != "claude" {
		t.Errorf("provider = %q, want %q", span.provider, "claude")
	}

	// EndSessionSpan should not panic
	rec.EndSessionSpan(span, 1.23, 5, "completed")

	// Ending again should be safe (span removed from map)
	rec.EndSessionSpan(span, 0, 0, "")
}

func TestOTelRecorder_RecordError(t *testing.T) {
	rec := NewOTelRecorder()

	_, span := rec.StartSessionSpan(context.Background(), "sess-err", "gemini", "flash", "repo2")

	// RecordError should not panic
	rec.RecordError(span, "test error message")

	// RecordError on nil span should not panic
	rec.RecordError(nil, "should not panic")

	rec.EndSessionSpan(span, 0.5, 2, "errored")
}

func TestOTelRecorder_RecordTurnMetric(t *testing.T) {
	rec := NewOTelRecorder()

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-turn", "codex", "o4-mini", "repo3")

	// RecordTurnMetric should not panic
	rec.RecordTurnMetric(ctx, "codex", "o4-mini", "sess-turn", 100, 200, 0.01, 500)

	// RecordTurnMetric with background context (no span) should not panic
	rec.RecordTurnMetric(context.Background(), "codex", "o4-mini", "sess-turn", 50, 100, 0.005, 250)

	rec.EndSessionSpan(span, 0.015, 2, "completed")
}

func TestOTelRecorder_RecordCostMetric(t *testing.T) {
	rec := NewOTelRecorder()

	ctx, span := rec.StartSessionSpan(context.Background(), "sess-cost", "claude", "sonnet", "repo4")

	// RecordCostMetric should not panic
	rec.RecordCostMetric(ctx, "claude", "repo4", 2.50)

	// RecordCostMetric with no span context should not panic
	rec.RecordCostMetric(context.Background(), "claude", "repo4", 1.0)

	rec.EndSessionSpan(span, 2.50, 10, "completed")
}

func TestOTelRecorder_NilSpanSafety(t *testing.T) {
	rec := NewOTelRecorder()

	// All operations on nil span should be safe
	rec.EndSessionSpan(nil, 0, 0, "")
	rec.RecordError(nil, "error")
}
