package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

const fleetTracerName = "ralphglasses/fleet"

// StartFleetSpan begins a new OpenTelemetry span representing a fleet
// coordination operation. The span is named "fleet.<operationType>" and
// carries the operation type as an attribute.
//
// Fleet spans act as parents for the session spans created when workers are
// assigned. Pass the returned context into StartSessionSpan to establish that
// parent→child relationship automatically via W3C trace context propagation.
//
// Callers must call span.End() when the operation completes.
//
// Example operation types: "task.submit", "worker.assign", "task.complete".
func StartFleetSpan(ctx context.Context, operationType string) (context.Context, trace.Span) {
	tracer := otel.Tracer(fleetTracerName)
	ctx, span := tracer.Start(ctx, "fleet."+operationType,
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("fleet.operation", operationType),
		),
	)
	return ctx, span
}

// LinkSessionSpan attaches the trace context of a session span as a linked
// span on the fleet span. Use this when a task completion event needs to
// reference the session that executed it without embedding it in the parent
// chain (i.e. fan-out scenarios where one fleet task spawns many sessions).
func LinkSessionSpan(fleetSpan trace.Span, sessionCtx context.Context) {
	if fleetSpan == nil {
		return
	}
	sc := trace.SpanContextFromContext(sessionCtx)
	if !sc.IsValid() {
		return
	}
	fleetSpan.AddLink(trace.Link{SpanContext: sc})
}
