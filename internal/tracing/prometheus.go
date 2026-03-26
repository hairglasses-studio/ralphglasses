package tracing

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// PrometheusRecorder records metrics in Prometheus exposition format.
// Serves /metrics endpoint without importing the Prometheus client library.
type PrometheusRecorder struct {
	inner    Recorder // delegate span operations
	mu       sync.Mutex
	gauges   map[string]*promMetric
	counters map[string]*promMetric
}

type promMetric struct {
	name   string
	help   string
	mtype  string // "gauge" or "counter"
	values map[string]float64            // label hash → value
	labels map[string]map[string]string   // label hash → label map
}

// NewPrometheusRecorder creates a recorder that exposes /metrics.
func NewPrometheusRecorder(inner Recorder) *PrometheusRecorder {
	if inner == nil {
		inner = &NoopRecorder{}
	}
	return &PrometheusRecorder{
		inner:    inner,
		gauges:   make(map[string]*promMetric),
		counters: make(map[string]*promMetric),
	}
}

// StartSessionSpan creates a session span and increments the active sessions counter.
func (p *PrometheusRecorder) StartSessionSpan(ctx context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan) {
	p.incCounter("gen_ai_sessions_total", "Total sessions launched", map[string]string{
		"provider": provider, "model": model, "repo_name": repoName,
	})
	p.addCounter("gen_ai_sessions_active", map[string]string{"provider": provider}, 1)
	return p.inner.StartSessionSpan(ctx, sessionID, provider, model, repoName)
}

// EndSessionSpan records session cost and turn totals, then decrements the active sessions gauge.
func (p *PrometheusRecorder) EndSessionSpan(span *SessionSpan, costUSD float64, turnCount int, exitReason string) {
	if span != nil {
		p.addCounter("gen_ai_cost_usd_total", map[string]string{
			"provider": span.provider, "model": span.model, "repo_name": span.repoName,
		}, costUSD)
		p.addCounter("gen_ai_turns_total", map[string]string{
			"provider": span.provider, "model": span.model,
		}, float64(turnCount))
		p.addCounter("gen_ai_sessions_active", map[string]string{"provider": span.provider}, -1)
	}
	p.inner.EndSessionSpan(span, costUSD, turnCount, exitReason)
}

// RecordTurnMetric increments token, cost, and latency counters for a single turn.
func (p *PrometheusRecorder) RecordTurnMetric(ctx context.Context, provider, model, sessionID string, inputTokens, outputTokens int, costUSD float64, latencyMs int64) {
	labels := map[string]string{"provider": provider, "model": model}
	p.addCounter("gen_ai_input_tokens_total", labels, float64(inputTokens))
	p.addCounter("gen_ai_output_tokens_total", labels, float64(outputTokens))
	p.addCounter("gen_ai_turn_cost_usd_total", labels, costUSD)
	p.addCounter("gen_ai_turn_latency_ms_total", labels, float64(latencyMs))
	p.inner.RecordTurnMetric(ctx, provider, model, sessionID, inputTokens, outputTokens, costUSD, latencyMs)
}

// RecordError increments the error counter for the span's provider.
func (p *PrometheusRecorder) RecordError(span *SessionSpan, errMsg string) {
	if span != nil {
		p.incCounter("gen_ai_errors_total", "Total session errors", map[string]string{
			"provider": span.provider,
		})
	}
	p.inner.RecordError(span, errMsg)
}

// RecordCostMetric sets the current session cost gauge for a provider and repo.
func (p *PrometheusRecorder) RecordCostMetric(ctx context.Context, provider, repoName string, costUSD float64) {
	p.setGauge("gen_ai_session_cost_usd", "Current session cost", map[string]string{
		"provider": provider, "repo_name": repoName,
	}, costUSD)
	p.inner.RecordCostMetric(ctx, provider, repoName, costUSD)
}

// Handler returns an http.HandlerFunc that serves /metrics in Prometheus text format.
func (p *PrometheusRecorder) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		p.mu.Lock()
		defer p.mu.Unlock()

		var lines []string
		for _, m := range p.counters {
			lines = append(lines, fmt.Sprintf("# HELP %s %s", m.name, m.help))
			lines = append(lines, fmt.Sprintf("# TYPE %s counter", m.name))
			for hash, val := range m.values {
				lines = append(lines, fmt.Sprintf("%s{%s} %g", m.name, labelStr(m.labels[hash]), val))
			}
		}
		for _, m := range p.gauges {
			lines = append(lines, fmt.Sprintf("# HELP %s %s", m.name, m.help))
			lines = append(lines, fmt.Sprintf("# TYPE %s gauge", m.name))
			for hash, val := range m.values {
				lines = append(lines, fmt.Sprintf("%s{%s} %g", m.name, labelStr(m.labels[hash]), val))
			}
		}

		sort.Strings(lines)
		fmt.Fprintln(w, strings.Join(lines, "\n"))
	}
}

// RecordToolCall records a single MCP tool invocation for Prometheus.
func (p *PrometheusRecorder) RecordToolCall(toolName string, latencyMs int64, status string) {
	labels := map[string]string{"tool": toolName, "status": status}
	p.incCounter("mcp_tool_calls_total", "Total MCP tool calls", labels)
	p.addCounter("mcp_tool_latency_ms_sum", map[string]string{"tool": toolName}, float64(latencyMs))
	p.addCounter("mcp_tool_latency_ms_count", map[string]string{"tool": toolName}, 1)
}

func (p *PrometheusRecorder) incCounter(name, help string, labels map[string]string) {
	p.addCounter(name, labels, 1)
	p.mu.Lock()
	if m, ok := p.counters[name]; ok {
		m.help = help
	}
	p.mu.Unlock()
}

func (p *PrometheusRecorder) addCounter(name string, labels map[string]string, delta float64) {
	hash := labelHash(labels)
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.counters[name]
	if !ok {
		m = &promMetric{name: name, help: name, mtype: "counter", values: make(map[string]float64), labels: make(map[string]map[string]string)}
		p.counters[name] = m
	}
	m.values[hash] += delta
	m.labels[hash] = labels
}

func (p *PrometheusRecorder) setGauge(name, help string, labels map[string]string, val float64) {
	hash := labelHash(labels)
	p.mu.Lock()
	defer p.mu.Unlock()
	m, ok := p.gauges[name]
	if !ok {
		m = &promMetric{name: name, help: help, mtype: "gauge", values: make(map[string]float64), labels: make(map[string]map[string]string)}
		p.gauges[name] = m
	}
	m.values[hash] = val
	m.labels[hash] = labels
}

func labelHash(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, k+"="+labels[k])
	}
	return strings.Join(parts, ",")
}

func labelStr(labels map[string]string) string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return strings.Join(parts, ",")
}
