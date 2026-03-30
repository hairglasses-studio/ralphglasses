package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const sessionTracerName = "ralphglasses/session"

// StartSessionSpan begins a new OpenTelemetry span for a session lifecycle.
// The span is named "session.<provider>" and carries the session ID and
// provider as attributes.
//
// The returned context propagates the span so that child operations (steps,
// fleet task assignments) can create child spans.
// Callers must call span.End() when the session terminates.
func StartSessionSpan(ctx context.Context, sessionID, provider string) (context.Context, trace.Span) {
	tracer := otel.Tracer(sessionTracerName)
	ctx, span := tracer.Start(ctx, "session."+provider,
		trace.WithSpanKind(trace.SpanKindInternal),
		trace.WithAttributes(
			attribute.String("gen_ai.session.id", sessionID),
			attribute.String("gen_ai.provider", provider),
		),
	)
	return ctx, span
}

// RecordSessionEvent adds a named event to a span with optional key/value
// attributes. It is safe to call on a nil or ended span — in that case it is
// a no-op.
//
// Example:
//
//	RecordSessionEvent(span, "session.step",
//	    attribute.Int("turn", 3),
//	    attribute.Float64("cost_usd", 0.002),
//	)
func RecordSessionEvent(span trace.Span, eventName string, attrs ...attribute.KeyValue) {
	if span == nil {
		return
	}
	span.AddEvent(eventName, trace.WithAttributes(attrs...))
}
