package tracing

import (
	"context"
	"testing"
)

func TestEndSessionSpan_NilSpan_Extra(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	// Should not panic on nil span.
	rec.EndSessionSpan(nil, 1.0, 5, "completed")
}

func TestEndSessionSpan_SetsAttributes(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	_, span := rec.StartSessionSpan(context.Background(), "s1", "claude", "sonnet", "repo")

	rec.EndSessionSpan(span, 2.50, 15, "budget_exceeded")

	span.mu.Lock()
	defer span.mu.Unlock()

	if !span.ended {
		t.Error("span should be marked as ended")
	}
	if cost, ok := span.attributes[AttrGenAICostUSD]; !ok || cost != 2.50 {
		t.Errorf("cost = %v, want 2.50", cost)
	}
	if turns, ok := span.attributes[AttrGenAITurnCount]; !ok || turns != 15 {
		t.Errorf("turn count = %v, want 15", turns)
	}
	if exit, ok := span.attributes[AttrGenAIExitReason]; !ok || exit != "budget_exceeded" {
		t.Errorf("exit reason = %v, want budget_exceeded", exit)
	}
	if _, ok := span.attributes[AttrGenAILatencyMs]; !ok {
		t.Error("latency_ms should be set")
	}
}

func TestRecordTurnMetric_StoresTokensAndCost(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	ctx := context.Background()

	rec.RecordTurnMetric(ctx, "gemini", "pro", "s2", 500, 1000, 0.05, 200)

	if rec.MetricCount() != 2 {
		t.Fatalf("expected 2 metrics (tokens + cost), got %d", rec.MetricCount())
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	// First metric: tokens.
	m0 := rec.Metrics[0]
	if m0.Name != "gen_ai.turn.tokens" {
		t.Errorf("metric[0] name = %q, want gen_ai.turn.tokens", m0.Name)
	}
	if m0.Value != 1500 {
		t.Errorf("metric[0] value = %f, want 1500", m0.Value)
	}
	if m0.Labels["provider"] != "gemini" {
		t.Errorf("metric[0] provider = %q, want gemini", m0.Labels["provider"])
	}

	// Second metric: cost.
	m1 := rec.Metrics[1]
	if m1.Name != "gen_ai.turn.cost_usd" {
		t.Errorf("metric[1] name = %q, want gen_ai.turn.cost_usd", m1.Name)
	}
	if m1.Value != 0.05 {
		t.Errorf("metric[1] value = %f, want 0.05", m1.Value)
	}
}

func TestRecordError_NilSpan_Extra(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	// Should not panic on nil span.
	rec.RecordError(nil, "some error")
}

func TestRecordError_AppendsEvent(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	_, span := rec.StartSessionSpan(context.Background(), "s3", "openai", "gpt4", "repo2")

	rec.RecordError(span, "timeout occurred")
	rec.RecordError(span, "retry failed")

	span.mu.Lock()
	defer span.mu.Unlock()

	if len(span.events) != 2 {
		t.Fatalf("expected 2 error events, got %d", len(span.events))
	}
	if span.events[0].Name != "error" {
		t.Errorf("event[0] name = %q, want error", span.events[0].Name)
	}
	if errMsg, ok := span.events[0].Attributes[AttrGenAIError]; !ok || errMsg != "timeout occurred" {
		t.Errorf("event[0] error = %v, want 'timeout occurred'", errMsg)
	}
	if errMsg, ok := span.events[1].Attributes[AttrGenAIError]; !ok || errMsg != "retry failed" {
		t.Errorf("event[1] error = %v, want 'retry failed'", errMsg)
	}
}

func TestRecordCostMetric_StoresProviderAndRepo(t *testing.T) {
	t.Parallel()
	rec := NewInMemoryRecorder()
	ctx := context.Background()

	rec.RecordCostMetric(ctx, "claude", "my-repo", 1.23)

	if rec.MetricCount() != 1 {
		t.Fatalf("expected 1 metric, got %d", rec.MetricCount())
	}

	rec.mu.Lock()
	defer rec.mu.Unlock()

	m := rec.Metrics[0]
	if m.Name != "gen_ai.session.cost_usd" {
		t.Errorf("name = %q, want gen_ai.session.cost_usd", m.Name)
	}
	if m.Value != 1.23 {
		t.Errorf("value = %f, want 1.23", m.Value)
	}
	if m.Labels["provider"] != "claude" {
		t.Errorf("provider = %q, want claude", m.Labels["provider"])
	}
	if m.Labels["repo_name"] != "my-repo" {
		t.Errorf("repo_name = %q, want my-repo", m.Labels["repo_name"])
	}
}
