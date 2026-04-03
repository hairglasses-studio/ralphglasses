package tracing

import (
	"context"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const otelTracerName = "ralphglasses/session"

// OTelRecorder implements Recorder using the official OpenTelemetry SDK.
// When the global tracer provider is a noop (no OTLP endpoint configured),
// all operations are zero-cost. When a real provider is registered via
// observability.NewProvider(), spans are exported to the configured collector.
type OTelRecorder struct {
	// otelSpans maps SessionSpan pointers to their OTel span counterparts.
	// This bridges the custom SessionSpan type with native OTel spans.
	otelSpans map[*SessionSpan]trace.Span
}

// NewOTelRecorder creates a Recorder that emits real OpenTelemetry spans.
func NewOTelRecorder() *OTelRecorder {
	return &OTelRecorder{
		otelSpans: make(map[*SessionSpan]trace.Span),
	}
}

// StartSessionSpan begins a new OTel span for a session lifecycle.
func (r *OTelRecorder) StartSessionSpan(ctx context.Context, sessionID, provider, model, repoName string) (context.Context, *SessionSpan) {
	tracer := otel.Tracer(otelTracerName)
	ctx, otelSpan := tracer.Start(ctx, "session."+provider,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String(AttrGenAISessionID, sessionID),
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIModel, model),
			attribute.String(AttrGenAIRepoName, repoName),
			attribute.String(AttrGenAISystem, provider),
		),
	)

	ss := &SessionSpan{
		sessionID: sessionID,
		provider:  provider,
		model:     model,
		repoName:  repoName,
		startTime: time.Now(),
	}

	r.otelSpans[ss] = otelSpan
	return ctx, ss
}

// EndSessionSpan completes the OTel span with final session attributes.
func (r *OTelRecorder) EndSessionSpan(span *SessionSpan, costUSD float64, turnCount int, exitReason string) {
	if span == nil {
		return
	}
	otelSpan, ok := r.otelSpans[span]
	if !ok {
		return
	}
	defer delete(r.otelSpans, span)

	otelSpan.SetAttributes(
		attribute.Float64(AttrGenAICostUSD, costUSD),
		attribute.Int(AttrGenAITurnCount, turnCount),
		attribute.String(AttrGenAIExitReason, exitReason),
	)
	otelSpan.SetStatus(codes.Ok, "")
	otelSpan.End()
}

// RecordTurnMetric records per-turn token usage as a span event.
func (r *OTelRecorder) RecordTurnMetric(ctx context.Context, provider, model, sessionID string, inputTokens, outputTokens int, costUSD float64, latencyMs int64) {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent("gen_ai.turn",
		trace.WithAttributes(
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIModel, model),
			attribute.String(AttrGenAISessionID, sessionID),
			attribute.Int(AttrGenAIInputTokens, inputTokens),
			attribute.Int(AttrGenAIOutputTokens, outputTokens),
			attribute.Float64(AttrGenAICostUSD, costUSD),
			attribute.Int64(AttrGenAILatencyMs, latencyMs),
		),
	)
}

// RecordError records an error on the OTel span.
func (r *OTelRecorder) RecordError(span *SessionSpan, errMsg string) {
	if span == nil {
		return
	}
	otelSpan, ok := r.otelSpans[span]
	if !ok {
		return
	}
	otelSpan.SetStatus(codes.Error, errMsg)
	otelSpan.AddEvent("gen_ai.error",
		trace.WithAttributes(
			attribute.String(AttrGenAIError, errMsg),
		),
	)
}

// RecordCostMetric records a cumulative cost data point as a span event on
// the current context span, if any.
func (r *OTelRecorder) RecordCostMetric(ctx context.Context, provider, repoName string, costUSD float64) {
	span := trace.SpanFromContext(ctx)
	if span == nil || !span.IsRecording() {
		return
	}
	span.AddEvent("gen_ai.cost",
		trace.WithAttributes(
			attribute.String(AttrGenAIProvider, provider),
			attribute.String(AttrGenAIRepoName, repoName),
			attribute.Float64(AttrGenAICostUSD, costUSD),
		),
	)
}
