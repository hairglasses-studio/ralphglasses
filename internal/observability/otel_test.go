package observability

import (
	"context"
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
