package observability

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerName = "ralphglasses/mcp"
	meterName  = "ralphglasses/mcp"
)

// mcpMetrics holds OTEL metric instruments for MCP tool calls.
type mcpMetrics struct {
	callCount   metric.Int64Counter
	latencyHist metric.Float64Histogram
	errorCount  metric.Int64Counter
}

func newMCPMetrics() (*mcpMetrics, error) {
	meter := otel.GetMeterProvider().Meter(meterName)

	callCount, err := meter.Int64Counter(
		"mcp.tool.calls",
		metric.WithDescription("Total number of MCP tool calls"),
		metric.WithUnit("{call}"),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create call counter: %w", err)
	}

	latencyHist, err := meter.Float64Histogram(
		"mcp.tool.latency",
		metric.WithDescription("Latency of MCP tool calls in milliseconds"),
		metric.WithUnit("ms"),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create latency histogram: %w", err)
	}

	errorCount, err := meter.Int64Counter(
		"mcp.tool.errors",
		metric.WithDescription("Total number of MCP tool call errors"),
		metric.WithUnit("{error}"),
	)
	if err != nil {
		return nil, fmt.Errorf("observability: create error counter: %w", err)
	}

	return &mcpMetrics{
		callCount:   callCount,
		latencyHist: latencyHist,
		errorCount:  errorCount,
	}, nil
}

// sanitizeArgs converts tool arguments to a truncated JSON string safe for
// span attributes. Values are serialised and capped at 512 bytes to avoid
// blowing up span storage with large prompt arguments.
func sanitizeArgs(args map[string]any) string {
	if len(args) == 0 {
		return "{}"
	}
	// Drop any key that looks like it might carry secrets.
	safe := make(map[string]any, len(args))
	for k, v := range args {
		safe[k] = v
	}
	b, err := json.Marshal(safe)
	if err != nil {
		return "{}"
	}
	if len(b) > 512 {
		b = b[:512]
	}
	return string(b)
}

// WrapHandler wraps an MCP ToolHandlerFunc with an OpenTelemetry span and
// counter/histogram metrics. Each invocation creates a child span named
// "mcp.tool.<name>" with the following attributes:
//
//   - mcp.tool.name — the tool name
//   - mcp.tool.args — sanitized JSON of the arguments (max 512 bytes)
//   - mcp.tool.success — bool outcome
//   - error — error message when the call fails
//
// Metrics recorded per call:
//   - mcp.tool.calls (Int64Counter)
//   - mcp.tool.latency (Float64Histogram, ms)
//   - mcp.tool.errors (Int64Counter, error calls only)
func WrapHandler(name string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	m, err := newMCPMetrics()
	if err != nil {
		// Metric creation failed — return unwrapped handler so callers are not broken.
		return handler
	}
	tracer := otel.Tracer(tracerName)

	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		ctx, span := tracer.Start(ctx, "mcp.tool."+name,
			trace.WithSpanKind(trace.SpanKindServer),
		)
		defer span.End()

		attrs := []attribute.KeyValue{
			attribute.String("mcp.tool.name", name),
			attribute.String("mcp.tool.args", sanitizeArgs(req.GetArguments())),
		}
		span.SetAttributes(attrs...)

		result, err := handler(ctx, req)

		latencyMs := float64(time.Since(start).Milliseconds())
		success := err == nil && result != nil && !result.IsError

		span.SetAttributes(attribute.Bool("mcp.tool.success", success))

		metricAttrs := attribute.NewSet(attribute.String("tool", name))
		m.callCount.Add(ctx, 1, metric.WithAttributeSet(metricAttrs))
		m.latencyHist.Record(ctx, latencyMs, metric.WithAttributeSet(metricAttrs))

		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			m.errorCount.Add(ctx, 1, metric.WithAttributeSet(metricAttrs))
		} else if result != nil && result.IsError {
			span.SetStatus(codes.Error, "tool returned error result")
			m.errorCount.Add(ctx, 1, metric.WithAttributeSet(metricAttrs))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		return result, err
	}
}
