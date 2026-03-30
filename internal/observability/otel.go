// Package observability provides OpenTelemetry integration for ralphglasses.
//
// When OTEL_EXPORTER_OTLP_ENDPOINT is not set, noop providers are used and all
// instrumentation is zero-cost. Set the env var to route spans and metrics to
// any OpenTelemetry-compatible collector (Jaeger, Tempo, OTLP, etc.).
//
// Usage:
//
//	p, shutdown, err := observability.NewProvider("ralphglasses", "")
//	if err != nil { ... }
//	defer shutdown(context.Background())
package observability

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Provider wraps OpenTelemetry tracer and meter providers for ralphglasses.
// When created without an OTLP endpoint it uses noop implementations that add
// no runtime overhead.
type Provider struct {
	tracerProvider *sdktrace.TracerProvider
	meterProvider  *sdkmetric.MeterProvider
	serviceName    string
	noop           bool
}

// NewProvider creates an OpenTelemetry Provider.
//
// serviceName is the resource attribute "service.name"; it falls back to the
// OTEL_SERVICE_NAME env var and then "ralphglasses".
//
// endpoint is the OTLP gRPC endpoint (e.g. "localhost:4317"); it falls back to
// the OTEL_EXPORTER_OTLP_ENDPOINT env var. When both are empty, noop providers
// are returned.
//
// The returned cleanup function must be called on shutdown to flush pending
// spans and free resources.
func NewProvider(serviceName, endpoint string) (*Provider, func(context.Context), error) {
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = "ralphglasses"
	}
	if endpoint == "" {
		endpoint = os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT")
	}

	// No endpoint — use noop providers so there is zero overhead.
	if endpoint == "" {
		p := &Provider{serviceName: serviceName, noop: true}
		return p, func(context.Context) {}, nil
	}

	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			semconv.ServiceNameKey.String(serviceName),
		),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("observability: build resource: %w", err)
	}

	// Build gRPC connection (insecure for local collectors; production should
	// use TLS via the standard OTEL_EXPORTER_OTLP_CERTIFICATE env var).
	conn, err := grpc.NewClient(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("observability: grpc dial %s: %w", endpoint, err)
	}

	// Trace exporter.
	traceExp, err := otlptracegrpc.New(context.Background(),
		otlptracegrpc.WithGRPCConn(conn),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("observability: build trace exporter: %w", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExp),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	// Metric provider (no exporter — metrics are recorded via OTEL API; swap in
	// an exporter here when a push-based metrics endpoint is needed).
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	p := &Provider{
		tracerProvider: tp,
		meterProvider:  mp,
		serviceName:    serviceName,
	}

	cleanup := func(ctx context.Context) {
		shutCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		_ = tp.Shutdown(shutCtx)
		_ = mp.Shutdown(shutCtx)
	}

	return p, cleanup, nil
}

// ServiceName returns the service name used for resource attributes.
func (p *Provider) ServiceName() string {
	return p.serviceName
}

// IsNoop returns true when no OTLP endpoint is configured.
func (p *Provider) IsNoop() bool {
	return p.noop
}
