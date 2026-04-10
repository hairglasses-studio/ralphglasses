package observability

import (
	"context"
	"encoding/base64"
	"testing"
)

func TestNewProvider_Noop(t *testing.T) {
	// No endpoint set → noop provider with zero overhead.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	p, cleanup, err := NewProvider("test-service", "")
	if err != nil {
		t.Fatalf("NewProvider() unexpected error: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil Provider")
	}
	if !p.IsNoop() {
		t.Error("expected noop=true when no endpoint is configured")
	}
	if p.ServiceName() != "test-service" {
		t.Errorf("ServiceName() = %q, want %q", p.ServiceName(), "test-service")
	}

	// cleanup must not panic on a noop provider.
	cleanup(context.Background())
}

func TestNewProvider_ServiceNameFallback(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "")

	p, cleanup, err := NewProvider("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup(context.Background())

	if p.ServiceName() != "ralphglasses" {
		t.Errorf("ServiceName() = %q, want %q", p.ServiceName(), "ralphglasses")
	}
}

func TestNewProvider_ServiceNameFromEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "my-service")

	p, cleanup, err := NewProvider("", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup(context.Background())

	if p.ServiceName() != "my-service" {
		t.Errorf("ServiceName() = %q, want %q", p.ServiceName(), "my-service")
	}
}

func TestNewProvider_ExplicitServiceNameOverridesEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_SERVICE_NAME", "env-service")

	p, cleanup, err := NewProvider("explicit-service", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup(context.Background())

	if p.ServiceName() != "explicit-service" {
		t.Errorf("ServiceName() = %q, want %q", p.ServiceName(), "explicit-service")
	}
}

func TestNewProvider_EndpointFromEnv(t *testing.T) {
	// Point at a non-existent endpoint; the gRPC dial is lazy so NewProvider
	// should succeed regardless. We just verify that noop is false.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:14317")

	p, cleanup, err := NewProvider("test", "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup(context.Background())

	if p.IsNoop() {
		t.Error("expected noop=false when endpoint is configured")
	}
}

func TestNewProvider_ExplicitEndpoint(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	p, cleanup, err := NewProvider("test", "localhost:14317")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup(context.Background())

	if p.IsNoop() {
		t.Error("expected noop=false when explicit endpoint is given")
	}
}

func TestResolveTraceExporterConfig_PrefersTraceSpecificEnv(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "https://collector.example/v1/traces")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "authorization=Bearer trace-token")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "authorization=Bearer shared-token")

	cfg := resolveTraceExporterConfig("")
	if cfg.Endpoint != "https://collector.example/v1/traces" {
		t.Fatalf("Endpoint = %q, want trace-specific endpoint", cfg.Endpoint)
	}
	if cfg.Protocol != "http" {
		t.Fatalf("Protocol = %q, want http", cfg.Protocol)
	}
	if got := cfg.Headers["authorization"]; got != "Bearer trace-token" {
		t.Fatalf("trace-specific header = %q, want %q", got, "Bearer trace-token")
	}
}

func TestResolveTraceExporterConfig_LangfuseFallback(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", "")
	t.Setenv("OTEL_EXPORTER_OTLP_HEADERS", "")
	t.Setenv("LANGFUSE_HOST", "https://langfuse.example")
	t.Setenv("LANGFUSE_PUBLIC_KEY", "pk-test")
	t.Setenv("LANGFUSE_SECRET_KEY", "sk-test")

	cfg := resolveTraceExporterConfig("")
	if cfg.Endpoint != "https://langfuse.example/api/public/otel" {
		t.Fatalf("Endpoint = %q, want Langfuse OTLP endpoint", cfg.Endpoint)
	}
	if cfg.Protocol != "http" {
		t.Fatalf("Protocol = %q, want http", cfg.Protocol)
	}

	wantAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("pk-test:sk-test"))
	if got := cfg.Headers["Authorization"]; got != wantAuth {
		t.Fatalf("Authorization = %q, want %q", got, wantAuth)
	}
}
