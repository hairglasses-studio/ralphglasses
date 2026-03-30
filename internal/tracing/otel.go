package tracing

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

// ExporterType selects the OTel span exporter backend.
type ExporterType string

const (
	ExporterStdout ExporterType = "stdout"
	ExporterOTLP   ExporterType = "otlp"
	ExporterJaeger ExporterType = "jaeger"
)

// OTelConfig configures the OpenTelemetry tracing provider.
type OTelConfig struct {
	// ServiceName is the OTel service.name resource attribute.
	ServiceName string

	// ExporterType selects the exporter backend (stdout, otlp, jaeger).
	ExporterType ExporterType

	// Endpoint is the collector address for OTLP/Jaeger exporters.
	// Ignored for stdout.
	Endpoint string

	// SampleRate is the probability sampling rate [0.0, 1.0].
	// 1.0 means sample everything, 0.0 means drop everything.
	SampleRate float64

	// Writer overrides the stdout exporter destination (default: os.Stdout).
	// Only used when ExporterType is "stdout". Useful for testing.
	Writer io.Writer
}

// otelSpanKey is the context key for mapping session IDs to OTel spans.
type otelSpanKey struct{}

// OTelRecorder implements Recorder using the OpenTelemetry SDK.
// It creates real distributed trace spans for session lifecycle events.
type OTelRecorder struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer

	mu    sync.Mutex
	spans map[string]trace.Span // sessionID -> OTel span
}

// NewOTelRecorder initializes a TracerProvider with the configured exporter
// and returns an OTelRecorder ready to create spans.
func NewOTelRecorder(cfg OTelConfig) (*OTelRecorder, error) {
	if cfg.ServiceName == "" {
		cfg.ServiceName = "ralphglasses"
	}
	if cfg.SampleRate <= 0 {
		cfg.SampleRate = 1.0
	}
	if cfg.SampleRate > 1.0 {
		cfg.SampleRate = 1.0
	}

	exporter, err := createExporter(cfg)
	if err != nil {
		return nil, fmt.Errorf("otel: create exporter: %w", err)
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("otel: create resource: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.ParentBased(
			sdktrace.TraceIDRatioBased(cfg.SampleRate),
		)),
	)

	otel.SetTracerProvider(tp)

	return &OTelRecorder{
		provider: tp,
		tracer:   tp.Tracer("ralphglasses/session"),
		spans:    make(map[string]trace.Span),
	}, nil
}

// newOTelRecorderFromProvider creates an OTelRecorder from an existing
// TracerProvider. Used internally for testing with in-memory exporters.
func newOTelRecorderFromProvider(tp *sdktrace.TracerProvider) *OTelRecorder {
	return &OTelRecorder{
		provider: tp,
		tracer:   tp.Tracer("ralphglasses/session"),
		spans:    make(map[string]trace.Span),
	}
}

func createExporter(cfg OTelConfig) (sdktrace.SpanExporter, error) {
	switch cfg.ExporterType {
	case ExporterStdout, "":
		opts := []stdouttrace.Option{}
		if cfg.Writer != nil {
			opts = append(opts, stdouttrace.WithWriter(cfg.Writer))
		}
		return stdouttrace.New(opts...)
	case ExporterOTLP, ExporterJaeger:
		// OTLP and Jaeger exporters require additional dependencies
		// (go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc
		//  or go.opentelemetry.io/otel/exporters/jaeger).
		// Return a clear error so users know which package to add.
		return nil, fmt.Errorf("exporter %q requires additional dependencies; use stdout for local development", cfg.ExporterType)
	default:
		return nil, fmt.Errorf("unknown exporter type: %q", cfg.ExporterType)
	}
}

// StartSessionSpan creates a root span for a session lifecycle.
// The span is stored internally and ended by EndSessionSpan.
func (r *OTelRecorder) StartSessionSpan(ctx context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan) {
	ctx, otelSpan := r.tracer.Start(ctx, "session",
		trace.WithAttributes(
			attribute.String(AttrGenAISessionID, sessionID),
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIModel, model),
			attribute.String(AttrGenAIRepoName, repoName),
			attribute.String(AttrGenAISystem, provider),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)

	// Store the OTel span so EndSessionSpan can finish it.
	r.mu.Lock()
	r.spans[sessionID] = otelSpan
	r.mu.Unlock()

	// Also store in context for child span creation.
	ctx = context.WithValue(ctx, otelSpanKey{}, otelSpan)

	return ctx, &SessionSpan{
		sessionID:  sessionID,
		provider:   provider,
		model:      model,
		repoName:   repoName,
		startTime:  time.Now(),
		attributes: make(map[string]any),
	}
}

// EndSessionSpan completes the OTel span with final session attributes.
func (r *OTelRecorder) EndSessionSpan(span *SessionSpan, costUSD float64, turnCount int, exitReason string) {
	if span == nil {
		return
	}

	r.mu.Lock()
	otelSpan, ok := r.spans[span.sessionID]
	if ok {
		delete(r.spans, span.sessionID)
	}
	r.mu.Unlock()

	if ok && otelSpan != nil {
		otelSpan.SetAttributes(
			attribute.Float64(AttrGenAICostUSD, costUSD),
			attribute.Int(AttrGenAITurnCount, turnCount),
			attribute.String(AttrGenAIExitReason, exitReason),
			attribute.Int64(AttrGenAILatencyMs, time.Since(span.startTime).Milliseconds()),
		)
		otelSpan.End()
	}

	// Update the internal SessionSpan bookkeeping.
	span.mu.Lock()
	defer span.mu.Unlock()
	span.ended = true
	span.attributes[AttrGenAICostUSD] = costUSD
	span.attributes[AttrGenAITurnCount] = turnCount
	span.attributes[AttrGenAIExitReason] = exitReason
	span.attributes[AttrGenAILatencyMs] = time.Since(span.startTime).Milliseconds()
}

// RecordTurnMetric records per-turn token usage as a span event on the
// current session span.
func (r *OTelRecorder) RecordTurnMetric(ctx context.Context, provider, model, sessionID string, inputTokens, outputTokens int, costUSD float64, latencyMs int64) {
	r.mu.Lock()
	otelSpan, ok := r.spans[sessionID]
	r.mu.Unlock()

	if ok && otelSpan != nil {
		otelSpan.AddEvent("turn", trace.WithAttributes(
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIModel, model),
			attribute.Int(AttrGenAIInputTokens, inputTokens),
			attribute.Int(AttrGenAIOutputTokens, outputTokens),
			attribute.Int(AttrGenAITotalTokens, inputTokens+outputTokens),
			attribute.Float64(AttrGenAICostUSD, costUSD),
			attribute.Int64(AttrGenAILatencyMs, latencyMs),
		))
	}
}

// RecordError records an error event on the session span and sets span status.
func (r *OTelRecorder) RecordError(span *SessionSpan, errMsg string) {
	if span == nil {
		return
	}

	r.mu.Lock()
	otelSpan, ok := r.spans[span.sessionID]
	r.mu.Unlock()

	if ok && otelSpan != nil {
		otelSpan.RecordError(fmt.Errorf("%s", errMsg))
		otelSpan.SetStatus(codes.Error, errMsg)
	}

	// Also record on the internal SessionSpan.
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

// RecordCostMetric records a cumulative cost data point as a span event.
func (r *OTelRecorder) RecordCostMetric(ctx context.Context, provider, repoName string, costUSD float64) {
	// Cost metrics are recorded as events on whichever span is active in ctx.
	otelSpan := trace.SpanFromContext(ctx)
	if otelSpan.IsRecording() {
		otelSpan.AddEvent("cost", trace.WithAttributes(
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIRepoName, repoName),
			attribute.Float64(AttrGenAICostUSD, costUSD),
		))
	}
}

// StartIterationSpan creates a child span for a single loop iteration
// within a session. The returned context carries the child span.
func (r *OTelRecorder) StartIterationSpan(ctx context.Context, sessionID string, iteration int) (context.Context, trace.Span) {
	ctx, iterSpan := r.tracer.Start(ctx, "iteration",
		trace.WithAttributes(
			attribute.String(AttrGenAISessionID, sessionID),
			attribute.Int("gen_ai.iteration", iteration),
		),
		trace.WithSpanKind(trace.SpanKindInternal),
	)
	return ctx, iterSpan
}

// RecordEvent adds a named span event with arbitrary attributes to
// the current span in context.
func (r *OTelRecorder) RecordEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	otelSpan := trace.SpanFromContext(ctx)
	if otelSpan.IsRecording() {
		otelSpan.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// Shutdown flushes pending spans and shuts down the TracerProvider.
// Call during application shutdown.
func (r *OTelRecorder) Shutdown(ctx context.Context) error {
	return r.provider.Shutdown(ctx)
}

// TracerProvider returns the underlying OTel TracerProvider for
// advanced use (e.g., propagation, additional instrumentation).
func (r *OTelRecorder) TracerProvider() *sdktrace.TracerProvider {
	return r.provider
}
