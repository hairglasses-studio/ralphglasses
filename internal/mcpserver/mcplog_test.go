package mcpserver

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// captureLog sets up slog to write to a buffer for the duration of the test.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	slog.SetDefault(slog.New(handler))
	return &buf
}

func TestMCPLoggerLevels(t *testing.T) {
	t.Parallel()

	levels := []struct {
		name   string
		call   func(l *MCPLogger, ctx context.Context)
		expect string // slog level keyword that should appear in output
	}{
		{"debug", func(l *MCPLogger, ctx context.Context) { l.Debug(ctx, "dbg msg", nil) }, "DEBUG"},
		{"info", func(l *MCPLogger, ctx context.Context) { l.Info(ctx, "inf msg", nil) }, "INFO"},
		{"warn", func(l *MCPLogger, ctx context.Context) { l.Warn(ctx, "wrn msg", nil) }, "WARN"},
		{"error", func(l *MCPLogger, ctx context.Context) { l.Error(ctx, "err msg", nil) }, "ERROR"},
	}

	for _, tt := range levels {
		t.Run(tt.name, func(t *testing.T) {
			buf := captureLog(t)

			// No server — forces slog fallback.
			logger := NewMCPLogger(nil, "test-logger")
			tt.call(logger, context.Background())

			output := buf.String()
			if !strings.Contains(output, tt.expect) {
				t.Errorf("expected slog level %q in output, got: %s", tt.expect, output)
			}
			if !strings.Contains(output, "logger=test-logger") {
				t.Errorf("expected logger name in output, got: %s", output)
			}
		})
	}
}

func TestMCPLoggerData(t *testing.T) {
	t.Parallel()
	buf := captureLog(t)

	logger := NewMCPLogger(nil, "ralphglasses")
	logger.Info(context.Background(), "query done", map[string]any{
		"tool":       "observation_query",
		"repo":       "myrepo",
		"elapsed_ms": 150,
	})

	output := buf.String()
	for _, want := range []string{"tool=observation_query", "repo=myrepo", "elapsed_ms=150"} {
		if !strings.Contains(output, want) {
			t.Errorf("expected %q in output, got: %s", want, output)
		}
	}
}

func TestMCPLoggerNilData(t *testing.T) {
	t.Parallel()
	buf := captureLog(t)

	logger := NewMCPLogger(nil, "test")
	logger.Info(context.Background(), "bare message", nil)

	output := buf.String()
	if !strings.Contains(output, "bare message") {
		t.Errorf("expected message in output, got: %s", output)
	}
}

func TestMCPLoggingMiddleware_Success(t *testing.T) {
	t.Parallel()
	buf := captureLog(t)

	logger := NewMCPLogger(nil, "mw-test")
	mw := MCPLoggingMiddleware(logger)
	wrapped := mw(okHandler)

	req := makeReq("test_tool", map[string]any{"repo": "foo"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("unexpected tool error")
	}

	output := buf.String()
	if !strings.Contains(output, "tool=test_tool") {
		t.Errorf("expected tool name in log, got: %s", output)
	}
	if !strings.Contains(output, "is_error=false") {
		t.Errorf("expected is_error=false in log, got: %s", output)
	}
	if !strings.Contains(output, "elapsed_ms=") {
		t.Errorf("expected elapsed_ms in log, got: %s", output)
	}
}

func TestMCPLoggingMiddleware_Error(t *testing.T) {
	t.Parallel()
	buf := captureLog(t)

	logger := NewMCPLogger(nil, "mw-test")
	mw := MCPLoggingMiddleware(logger)

	errHandler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return errResult("something broke"), nil
	}
	wrapped := mw(errHandler)

	req := makeReq("bad_tool", nil)
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected tool error")
	}

	output := buf.String()
	if !strings.Contains(output, "tool=bad_tool") {
		t.Errorf("expected tool name in log, got: %s", output)
	}
	if !strings.Contains(output, "is_error=true") {
		t.Errorf("expected is_error=true in log, got: %s", output)
	}
	// Error-level tool calls should produce ERROR-level slog output.
	if !strings.Contains(output, "ERROR") {
		t.Errorf("expected ERROR level in log, got: %s", output)
	}
}

func TestMCPLoggingMiddleware_NilLogger(t *testing.T) {
	t.Parallel()
	mw := MCPLoggingMiddleware(nil)
	wrapped := mw(okHandler)

	result, err := wrapped(context.Background(), makeReq("x", nil))
	if err != nil || result.IsError {
		t.Fatal("nil logger should pass through without error")
	}
}
