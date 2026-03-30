// Package tracing provides session-level observability using OpenTelemetry
// GenAI semantic conventions. It records spans, metrics, and attributes for
// each session lifecycle event (launch, turn, error, complete).
//
// When no OTel SDK is configured, a no-op provider is used and all recording
// is zero-cost. Enable OTel by calling InitOTel with an OTLP endpoint.
package tracing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"sync"
	"time"
)

// traceIDKey is the context key for propagating trace IDs across MCP tool calls.
type traceIDKey struct{}

// WithTraceID returns a new context with the given trace ID attached.
func WithTraceID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, id)
}

// TraceIDFromContext extracts the trace ID from context, or returns "" if none.
func TraceIDFromContext(ctx context.Context) string {
	if id, ok := ctx.Value(traceIDKey{}).(string); ok {
		return id
	}
	return ""
}

// Request-scoped context keys for MCP handler logging.
type toolNameKey struct{}
type repoKey struct{}
type requestStartKey struct{}

// WithToolName attaches the tool name to the context.
func WithToolName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, toolNameKey{}, name)
}

// ToolNameFromContext extracts the tool name, or "" if not set.
func ToolNameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(toolNameKey{}).(string); ok {
		return v
	}
	return ""
}

// WithRepo attaches the repo name to the context.
func WithRepo(ctx context.Context, repo string) context.Context {
	return context.WithValue(ctx, repoKey{}, repo)
}

// RepoFromContext extracts the repo name, or "" if not set.
func RepoFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(repoKey{}).(string); ok {
		return v
	}
	return ""
}

// WithRequestStart attaches the request start time to the context.
func WithRequestStart(ctx context.Context, t time.Time) context.Context {
	return context.WithValue(ctx, requestStartKey{}, t)
}

// RequestStartFromContext extracts the request start time, or zero if not set.
func RequestStartFromContext(ctx context.Context) time.Time {
	if v, ok := ctx.Value(requestStartKey{}).(time.Time); ok {
		return v
	}
	return time.Time{}
}

// RequestLogger returns an slog.Logger pre-scoped with request context fields
// (tool name, repo, elapsed time). Use in MCP handlers instead of bare slog calls.
func RequestLogger(ctx context.Context) *slog.Logger {
	var attrs []any
	if tool := ToolNameFromContext(ctx); tool != "" {
		attrs = append(attrs, "tool", tool)
	}
	if repo := RepoFromContext(ctx); repo != "" {
		attrs = append(attrs, "repo", repo)
	}
	if tid := TraceIDFromContext(ctx); tid != "" {
		attrs = append(attrs, "trace_id", tid)
	}
	if start := RequestStartFromContext(ctx); !start.IsZero() {
		attrs = append(attrs, "elapsed_ms", time.Since(start).Milliseconds())
	}
	return slog.With(attrs...)
}

// NewTraceID generates a 16-character hex trace ID using crypto/rand.
func NewTraceID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		// Fallback: should never happen with crypto/rand.
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

// GenAI semantic convention attribute keys (OpenTelemetry GenAI spec).
const (
	AttrGenAISystem       = "gen_ai.system"
	AttrGenAIModel        = "gen_ai.request.model"
	AttrGenAIProvider     = "gen_ai.provider"
	AttrGenAIMaxTokens    = "gen_ai.request.max_tokens"
	AttrGenAITemperature  = "gen_ai.request.temperature"
	AttrGenAIInputTokens  = "gen_ai.usage.input_tokens"
	AttrGenAIOutputTokens = "gen_ai.usage.output_tokens"
	AttrGenAITotalTokens  = "gen_ai.usage.total_tokens"
	AttrGenAICostUSD      = "gen_ai.usage.cost_usd"
	AttrGenAITurnCount    = "gen_ai.usage.turn_count"
	AttrGenAILatencyMs    = "gen_ai.latency_ms"
	AttrGenAISessionID    = "gen_ai.session.id"
	AttrGenAIRepoName     = "gen_ai.session.repo_name"
	AttrGenAITeamName     = "gen_ai.session.team_name"
	AttrGenAIExitReason   = "gen_ai.session.exit_reason"
	AttrGenAIError        = "gen_ai.error"
)

// SessionSpan records a session lifecycle as a trace span.
type SessionSpan struct {
	mu          sync.Mutex
	sessionID   string
	provider    string
	model       string
	repoName    string
	startTime   time.Time
	attributes  map[string]any
	events      []SpanEvent
	ended       bool
}

// SpanEvent is a timestamped event within a span.
type SpanEvent struct {
	Name       string         `json:"name"`
	Timestamp  time.Time      `json:"timestamp"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

// Metric holds a single metric data point.
type Metric struct {
	Name       string         `json:"name"`
	Value      float64        `json:"value"`
	Labels     map[string]string `json:"labels,omitempty"`
	Timestamp  time.Time      `json:"timestamp"`
}

// Recorder is the interface for recording session telemetry.
// Implementations include NoopRecorder (default) and OTelRecorder.
type Recorder interface {
	// StartSessionSpan begins a new span for a session launch.
	StartSessionSpan(ctx context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan)

	// EndSessionSpan completes the span with final attributes.
	EndSessionSpan(span *SessionSpan, costUSD float64, turnCount int, exitReason string)

	// RecordTurnMetric records per-turn token usage.
	RecordTurnMetric(ctx context.Context, provider, model, sessionID string, inputTokens, outputTokens int, costUSD float64, latencyMs int64)

	// RecordError records an error event on the span.
	RecordError(span *SessionSpan, errMsg string)

	// RecordCostMetric records a cumulative cost data point.
	RecordCostMetric(ctx context.Context, provider, repoName string, costUSD float64)
}

// global is the package-level recorder. Default is noop.
var (
	globalMu  sync.RWMutex
	globalRec Recorder = &NoopRecorder{}
)

// SetRecorder sets the global recorder. Call during initialization.
func SetRecorder(r Recorder) {
	globalMu.Lock()
	defer globalMu.Unlock()
	globalRec = r
}

// Get returns the global recorder.
func Get() Recorder {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalRec
}

// NoopRecorder satisfies the Recorder interface with zero-cost no-ops.
type NoopRecorder struct{}

// StartSessionSpan returns a no-op span with the given metadata.
func (n *NoopRecorder) StartSessionSpan(_ context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan) {
	return context.Background(), &SessionSpan{
		sessionID: sessionID,
		provider:  provider,
		model:     model,
		repoName:  repoName,
		startTime: time.Now(),
	}
}

// EndSessionSpan is a no-op.
func (n *NoopRecorder) EndSessionSpan(_ *SessionSpan, _ float64, _ int, _ string) {}

// RecordTurnMetric is a no-op.
func (n *NoopRecorder) RecordTurnMetric(_ context.Context, _, _, _ string, _, _ int, _ float64, _ int64) {
}

// RecordError is a no-op.
func (n *NoopRecorder) RecordError(_ *SessionSpan, _ string) {}

// RecordCostMetric is a no-op.
func (n *NoopRecorder) RecordCostMetric(_ context.Context, _, _ string, _ float64) {}

// InMemoryRecorder captures metrics in memory for testing and local dashboards.
type InMemoryRecorder struct {
	mu      sync.Mutex
	Spans   []*SessionSpan
	Metrics []Metric
}

// NewInMemoryRecorder creates an InMemoryRecorder for testing and local dashboards.
func NewInMemoryRecorder() *InMemoryRecorder {
	return &InMemoryRecorder{}
}

// StartSessionSpan creates and stores a span in memory.
func (r *InMemoryRecorder) StartSessionSpan(_ context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan) {
	span := &SessionSpan{
		sessionID:  sessionID,
		provider:   provider,
		model:      model,
		repoName:   repoName,
		startTime:  time.Now(),
		attributes: make(map[string]any),
	}
	r.mu.Lock()
	r.Spans = append(r.Spans, span)
	r.mu.Unlock()
	return context.Background(), span
}

// EndSessionSpan marks a span as ended and records cost, turn count, and exit reason.
func (r *InMemoryRecorder) EndSessionSpan(span *SessionSpan, costUSD float64, turnCount int, exitReason string) {
	if span == nil {
		return
	}
	span.mu.Lock()
	defer span.mu.Unlock()
	span.ended = true
	span.attributes[AttrGenAICostUSD] = costUSD
	span.attributes[AttrGenAITurnCount] = turnCount
	span.attributes[AttrGenAIExitReason] = exitReason
	span.attributes[AttrGenAILatencyMs] = time.Since(span.startTime).Milliseconds()
}

// RecordTurnMetric stores token and cost metrics for a single turn.
func (r *InMemoryRecorder) RecordTurnMetric(_ context.Context, provider, model, sessionID string, inputTokens, outputTokens int, costUSD float64, latencyMs int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Metrics = append(r.Metrics, Metric{
		Name:  "gen_ai.turn.tokens",
		Value: float64(inputTokens + outputTokens),
		Labels: map[string]string{
			"provider":   provider,
			"model":      model,
			"session_id": sessionID,
		},
		Timestamp: time.Now(),
	})
	r.Metrics = append(r.Metrics, Metric{
		Name:  "gen_ai.turn.cost_usd",
		Value: costUSD,
		Labels: map[string]string{
			"provider":   provider,
			"model":      model,
			"session_id": sessionID,
		},
		Timestamp: time.Now(),
	})
}

// RecordError appends an error event to the span.
func (r *InMemoryRecorder) RecordError(span *SessionSpan, errMsg string) {
	if span == nil {
		return
	}
	span.mu.Lock()
	defer span.mu.Unlock()
	span.events = append(span.events, SpanEvent{
		Name:      "error",
		Timestamp: time.Now(),
		Attributes: map[string]any{
			AttrGenAIError: errMsg,
		},
	})
}

// RecordCostMetric stores a cost metric for a provider and repo.
func (r *InMemoryRecorder) RecordCostMetric(_ context.Context, provider, repoName string, costUSD float64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Metrics = append(r.Metrics, Metric{
		Name:  "gen_ai.session.cost_usd",
		Value: costUSD,
		Labels: map[string]string{
			"provider":  provider,
			"repo_name": repoName,
		},
		Timestamp: time.Now(),
	})
}

// SpanCount returns the number of recorded spans.
func (r *InMemoryRecorder) SpanCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Spans)
}

// MetricCount returns the number of recorded metrics.
func (r *InMemoryRecorder) MetricCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.Metrics)
}
