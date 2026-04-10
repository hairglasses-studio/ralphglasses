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
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
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
// endpoint is the OTLP traces endpoint. Plain host:port values are treated as
// OTLP/gRPC endpoints. Full http(s) URLs are treated as OTLP/HTTP endpoints.
// It falls back to OTEL_EXPORTER_OTLP_TRACES_ENDPOINT, then
// OTEL_EXPORTER_OTLP_ENDPOINT. When those are empty, Langfuse OTLP/HTTP can be
// derived from LANGFUSE_HOST + LANGFUSE_PUBLIC_KEY + LANGFUSE_SECRET_KEY.
// When no endpoint can be resolved, noop providers are returned.
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
	cfg := resolveTraceExporterConfig(endpoint)

	// No endpoint — use noop providers so there is zero overhead.
	if cfg.Endpoint == "" {
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

	traceExp, err := newTraceExporter(context.Background(), cfg)
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

type traceExporterConfig struct {
	Endpoint string
	Protocol string
	Headers  map[string]string
}

func resolveTraceExporterConfig(explicitEndpoint string) traceExporterConfig {
	endpoint := strings.TrimSpace(explicitEndpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"))
	}
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"))
	}
	if endpoint == "" {
		endpoint = deriveLangfuseOTLPEndpoint()
	}

	return traceExporterConfig{
		Endpoint: endpoint,
		Protocol: resolveTraceExporterProtocol(endpoint),
		Headers:  resolveTraceExporterHeaders(),
	}
}

func resolveTraceExporterProtocol(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		return "http"
	}
	return "grpc"
}

func newTraceExporter(ctx context.Context, cfg traceExporterConfig) (sdktrace.SpanExporter, error) {
	switch cfg.Protocol {
	case "http":
		opts := []otlptracehttp.Option{
			otlptracehttp.WithEndpointURL(cfg.Endpoint),
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracehttp.WithHeaders(cfg.Headers))
		}
		return otlptracehttp.New(ctx, opts...)
	case "grpc":
		opts := []otlptracegrpc.Option{
			otlptracegrpc.WithEndpoint(cfg.Endpoint),
			otlptracegrpc.WithInsecure(),
		}
		if strings.HasPrefix(cfg.Endpoint, "http://") || strings.HasPrefix(cfg.Endpoint, "https://") {
			opts = []otlptracegrpc.Option{
				otlptracegrpc.WithEndpointURL(cfg.Endpoint),
			}
		}
		if len(cfg.Headers) > 0 {
			opts = append(opts, otlptracegrpc.WithHeaders(cfg.Headers))
		}
		return otlptracegrpc.New(ctx, opts...)
	default:
		return nil, fmt.Errorf("unsupported OTLP protocol for endpoint %q", cfg.Endpoint)
	}
}

func resolveTraceExporterHeaders() map[string]string {
	if headers := parseHeaderEnv(os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS")); len(headers) > 0 {
		return headers
	}
	if headers := parseHeaderEnv(os.Getenv("OTEL_EXPORTER_OTLP_HEADERS")); len(headers) > 0 {
		return headers
	}
	return deriveLangfuseOTLPHeaders()
}

func parseHeaderEnv(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	headers := make(map[string]string)
	for _, pair := range strings.Split(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		key, value, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		headers[key] = value
	}
	if len(headers) == 0 {
		return nil
	}
	return headers
}

func deriveLangfuseOTLPEndpoint() string {
	host := strings.TrimSpace(os.Getenv("LANGFUSE_HOST"))
	if host == "" {
		return ""
	}
	host = strings.TrimRight(host, "/")
	return host + "/api/public/otel"
}

func deriveLangfuseOTLPHeaders() map[string]string {
	publicKey := strings.TrimSpace(os.Getenv("LANGFUSE_PUBLIC_KEY"))
	secretKey := strings.TrimSpace(os.Getenv("LANGFUSE_SECRET_KEY"))
	if publicKey == "" || secretKey == "" {
		return nil
	}
	token := base64.StdEncoding.EncodeToString([]byte(publicKey + ":" + secretKey))
	return map[string]string{
		"Authorization": "Basic " + token,
	}
}

// ServiceName returns the service name used for resource attributes.
func (p *Provider) ServiceName() string {
	return p.serviceName
}

// IsNoop returns true when no OTLP endpoint is configured.
func (p *Provider) IsNoop() bool {
	return p.noop
}
