package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewPrometheusRecorder(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	if pr == nil {
		t.Fatal("NewPrometheusRecorder returned nil")
	}
	if pr.inner == nil {
		t.Error("inner recorder should default to NoopRecorder when nil passed")
	}
}

func TestNewPrometheusRecorder_WithInner(t *testing.T) {
	inner := NewInMemoryRecorder()
	pr := NewPrometheusRecorder(inner)
	if pr.inner != inner {
		t.Error("inner recorder should be the one passed in")
	}
}

func TestRecordError_NilSpan(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	// Should not panic with nil span
	pr.RecordError(nil, "some error")
}

func TestRecordError_WithSpan(t *testing.T) {
	inner := NewInMemoryRecorder()
	pr := NewPrometheusRecorder(inner)

	_, span := pr.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "myrepo")
	pr.RecordError(span, "test error")

	// Check that the counter was incremented
	pr.mu.Lock()
	counter, ok := pr.counters["gen_ai_errors_total"]
	pr.mu.Unlock()

	if !ok {
		t.Fatal("gen_ai_errors_total counter should exist")
	}
	total := 0.0
	for _, v := range counter.values {
		total += v
	}
	if total != 1 {
		t.Errorf("error counter should be 1, got %g", total)
	}
}

func TestStartSessionSpan_IncrementsCounters(t *testing.T) {
	pr := NewPrometheusRecorder(nil)

	ctx, span := pr.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "myrepo")
	if span == nil {
		t.Fatal("span should not be nil")
	}
	if ctx == nil {
		t.Fatal("context should not be nil")
	}

	pr.mu.Lock()
	_, hasTotal := pr.counters["gen_ai_sessions_total"]
	_, hasActive := pr.counters["gen_ai_sessions_active"]
	pr.mu.Unlock()

	if !hasTotal {
		t.Error("gen_ai_sessions_total should be recorded")
	}
	if !hasActive {
		t.Error("gen_ai_sessions_active should be recorded")
	}
}

func TestEndSessionSpan_RecordsCostAndTurns(t *testing.T) {
	pr := NewPrometheusRecorder(nil)

	_, span := pr.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "myrepo")
	pr.EndSessionSpan(span, 0.50, 10, "completed")

	pr.mu.Lock()
	_, hasCost := pr.counters["gen_ai_cost_usd_total"]
	_, hasTurns := pr.counters["gen_ai_turns_total"]
	pr.mu.Unlock()

	if !hasCost {
		t.Error("gen_ai_cost_usd_total should be recorded")
	}
	if !hasTurns {
		t.Error("gen_ai_turns_total should be recorded")
	}
}

func TestEndSessionSpan_NilSpan(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	// Should not panic
	pr.EndSessionSpan(nil, 0.0, 0, "")
}

func TestRecordTurnMetric(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	pr.RecordTurnMetric(context.Background(), "claude", "sonnet", "sess-1", 100, 50, 0.01, 500)

	pr.mu.Lock()
	_, hasInput := pr.counters["gen_ai_input_tokens_total"]
	_, hasOutput := pr.counters["gen_ai_output_tokens_total"]
	_, hasCost := pr.counters["gen_ai_turn_cost_usd_total"]
	_, hasLatency := pr.counters["gen_ai_turn_latency_ms_total"]
	pr.mu.Unlock()

	if !hasInput {
		t.Error("gen_ai_input_tokens_total should be recorded")
	}
	if !hasOutput {
		t.Error("gen_ai_output_tokens_total should be recorded")
	}
	if !hasCost {
		t.Error("gen_ai_turn_cost_usd_total should be recorded")
	}
	if !hasLatency {
		t.Error("gen_ai_turn_latency_ms_total should be recorded")
	}
}

func TestRecordCostMetric(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	pr.RecordCostMetric(context.Background(), "claude", "myrepo", 1.23)

	pr.mu.Lock()
	_, hasGauge := pr.gauges["gen_ai_session_cost_usd"]
	pr.mu.Unlock()

	if !hasGauge {
		t.Error("gen_ai_session_cost_usd gauge should be recorded")
	}
}

func TestRecordToolCall(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	pr.RecordToolCall("session_launch", 150, "ok")

	pr.mu.Lock()
	_, hasCalls := pr.counters["mcp_tool_calls_total"]
	_, hasLatency := pr.counters["mcp_tool_latency_ms_sum"]
	_, hasCount := pr.counters["mcp_tool_latency_ms_count"]
	pr.mu.Unlock()

	if !hasCalls {
		t.Error("mcp_tool_calls_total should be recorded")
	}
	if !hasLatency {
		t.Error("mcp_tool_latency_ms_sum should be recorded")
	}
	if !hasCount {
		t.Error("mcp_tool_latency_ms_count should be recorded")
	}
}

func TestHandler_ServesMetrics(t *testing.T) {
	pr := NewPrometheusRecorder(nil)

	// Record some data first
	_, span := pr.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "repo")
	pr.RecordError(span, "test error")
	pr.EndSessionSpan(span, 0.5, 5, "completed")
	pr.RecordCostMetric(context.Background(), "claude", "repo", 0.5)
	pr.RecordToolCall("test_tool", 100, "ok")

	handler := pr.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("Content-Type should be text/plain, got %q", ct)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("metrics body should not be empty after recording data")
	}
	// Should contain HELP and TYPE lines
	if !strings.Contains(body, "# HELP") {
		t.Error("body should contain # HELP lines")
	}
	if !strings.Contains(body, "# TYPE") {
		t.Error("body should contain # TYPE lines")
	}
}

func TestHandler_EmptyMetrics(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	handler := pr.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Errorf("expected 200 even with no metrics, got %d", w.Result().StatusCode)
	}
}

func TestLabelHash(t *testing.T) {
	labels := map[string]string{"b": "2", "a": "1"}
	hash := labelHash(labels)
	// Should be sorted by key
	if hash != "a=1,b=2" {
		t.Errorf("labelHash = %q, want %q", hash, "a=1,b=2")
	}
}

func TestLabelStr(t *testing.T) {
	labels := map[string]string{"provider": "claude", "model": "sonnet"}
	str := labelStr(labels)
	// Should be sorted and quoted
	if !strings.Contains(str, `model="sonnet"`) {
		t.Errorf("labelStr should contain model=\"sonnet\", got %q", str)
	}
	if !strings.Contains(str, `provider="claude"`) {
		t.Errorf("labelStr should contain provider=\"claude\", got %q", str)
	}
}

func TestMultipleRecordErrors_Accumulate(t *testing.T) {
	pr := NewPrometheusRecorder(nil)
	_, span := pr.StartSessionSpan(context.Background(), "sess-1", "claude", "sonnet", "repo")

	pr.RecordError(span, "err1")
	pr.RecordError(span, "err2")
	pr.RecordError(span, "err3")

	pr.mu.Lock()
	counter := pr.counters["gen_ai_errors_total"]
	total := 0.0
	for _, v := range counter.values {
		total += v
	}
	pr.mu.Unlock()

	if total != 3 {
		t.Errorf("error counter should be 3 after 3 errors, got %g", total)
	}
}
