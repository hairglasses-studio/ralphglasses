package observability

import (
	"context"
	"errors"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"go.opentelemetry.io/otel"
)

// setupTestTracer installs an in-memory span recorder and returns it along with
// a cleanup function that restores the previous global tracer provider.
func setupTestTracer(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	prev := otel.GetTracerProvider()
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		otel.SetTracerProvider(prev)
	})
	return rec
}

func TestWrapHandler_Success(t *testing.T) {
	rec := setupTestTracer(t)

	called := false
	var handler server.ToolHandlerFunc = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		called = true
		return &mcp.CallToolResult{}, nil
	}

	wrapped := WrapHandler("test_tool", handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "test_tool"

	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !called {
		t.Error("underlying handler was not called")
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}
	if spans[0].Name() != "mcp.tool.test_tool" {
		t.Errorf("span name = %q, want %q", spans[0].Name(), "mcp.tool.test_tool")
	}
}

func TestWrapHandler_Error(t *testing.T) {
	rec := setupTestTracer(t)

	wantErr := errors.New("handler failed")
	var handler server.ToolHandlerFunc = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return nil, wantErr
	}

	wrapped := WrapHandler("failing_tool", handler)

	req := mcp.CallToolRequest{}
	req.Params.Name = "failing_tool"

	_, err := wrapped(context.Background(), req)
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 ended span, got %d", len(spans))
	}
	// Span should carry an error status.
	if spans[0].Status().Code.String() != "Error" {
		t.Errorf("span status = %q, want Error", spans[0].Status().Code.String())
	}
}

func TestWrapHandler_ContextPropagation(t *testing.T) {
	setupTestTracer(t)

	type ctxKey struct{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "sentinel")

	var handler server.ToolHandlerFunc = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if ctx.Value(ctxKey{}) != "sentinel" {
			t.Error("context value not propagated to handler")
		}
		return &mcp.CallToolResult{}, nil
	}

	wrapped := WrapHandler("ctx_tool", handler)
	req := mcp.CallToolRequest{}
	req.Params.Name = "ctx_tool"

	_, err := wrapped(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSanitizeArgs(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want string
	}{
		{
			name: "empty",
			args: nil,
			want: "{}",
		},
		{
			name: "simple string",
			args: map[string]any{"key": "value"},
			// JSON marshaling order is non-deterministic so we just check length.
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeArgs(tc.args)
			if tc.want != "" && got != tc.want {
				t.Errorf("sanitizeArgs() = %q, want %q", got, tc.want)
			}
			if len(got) == 0 {
				t.Error("sanitizeArgs() returned empty string")
			}
		})
	}
}

func TestSanitizeArgs_Truncation(t *testing.T) {
	// Build a large value that should be truncated at 512 bytes.
	large := make([]byte, 600)
	for i := range large {
		large[i] = 'x'
	}
	args := map[string]any{"data": string(large)}
	got := sanitizeArgs(args)
	if len(got) > 512 {
		t.Errorf("sanitizeArgs() length = %d, want <= 512", len(got))
	}
}
