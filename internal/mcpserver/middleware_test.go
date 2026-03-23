package mcpserver

import (
	"context"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

func makeReq(name string, args map[string]any) mcp.CallToolRequest {
	return mcp.CallToolRequest{
		Request: mcp.Request{Method: "tools/call"},
		Params: mcp.CallToolParams{
			Name:      name,
			Arguments: args,
		},
	}
}

func okHandler(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return textResult("ok"), nil
}

func TestInstrumentationMiddleware(t *testing.T) {
	rec := NewToolCallRecorder("", nil, 100)
	mw := InstrumentationMiddleware(rec)
	wrapped := mw(okHandler)

	req := makeReq("test_tool", map[string]any{"repo": "foo"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatalf("unexpected tool error")
	}

	entries := rec.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.ToolName != "test_tool" {
		t.Errorf("tool name = %q, want test_tool", e.ToolName)
	}
	if !e.Success {
		t.Error("expected success=true")
	}
	if e.LatencyMs < 0 {
		t.Error("latency should be >= 0")
	}
}

func TestInstrumentationMiddleware_Error(t *testing.T) {
	rec := NewToolCallRecorder("", nil, 100)
	mw := InstrumentationMiddleware(rec)

	errHandler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return errResult("something broke"), nil
	}
	wrapped := mw(errHandler)

	req := makeReq("bad_tool", nil)
	_, _ = wrapped(context.Background(), req)

	entries := rec.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Success {
		t.Error("expected success=false for error result")
	}
	if entries[0].ErrorMsg == "" {
		t.Error("expected non-empty error message")
	}
}

func TestInstrumentationMiddleware_NilRecorder(t *testing.T) {
	mw := InstrumentationMiddleware(nil)
	wrapped := mw(okHandler)
	result, err := wrapped(context.Background(), makeReq("x", nil))
	if err != nil || result.IsError {
		t.Fatal("nil recorder should pass through without error")
	}
}

func TestEventBusMiddleware(t *testing.T) {
	bus := events.NewBus(100)
	ch := bus.Subscribe("test")

	mw := EventBusMiddleware(bus)
	wrapped := mw(okHandler)

	req := makeReq("my_tool", nil)
	_, _ = wrapped(context.Background(), req)

	select {
	case evt := <-ch:
		if evt.Type != events.ToolCalled {
			t.Errorf("event type = %q, want tool.called", evt.Type)
		}
		if evt.Data["tool"] != "my_tool" {
			t.Errorf("event tool = %v, want my_tool", evt.Data["tool"])
		}
		if evt.Data["success"] != true {
			t.Errorf("event success = %v, want true", evt.Data["success"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}

	bus.Unsubscribe("test")
}

func TestEventBusMiddleware_NilBus(t *testing.T) {
	mw := EventBusMiddleware(nil)
	wrapped := mw(okHandler)
	result, err := wrapped(context.Background(), makeReq("x", nil))
	if err != nil || result.IsError {
		t.Fatal("nil bus should pass through without error")
	}
}

func TestValidationMiddleware_ValidRepo(t *testing.T) {
	mw := ValidationMiddleware("")
	wrapped := mw(okHandler)

	req := makeReq("test", map[string]any{"repo": "my-repo"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("valid repo should not produce error")
	}
}

func TestValidationMiddleware_InvalidRepo(t *testing.T) {
	mw := ValidationMiddleware("")
	wrapped := mw(okHandler)

	req := makeReq("test", map[string]any{"repo": "bad;repo"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("invalid repo should produce error")
	}
}

func TestValidationMiddleware_InvalidPath(t *testing.T) {
	mw := ValidationMiddleware("/tmp/scan")
	wrapped := mw(okHandler)

	req := makeReq("test", map[string]any{"path": "../../etc/passwd"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("traversal path should produce error")
	}
}

func TestValidationMiddleware_NoArgs(t *testing.T) {
	mw := ValidationMiddleware("")
	wrapped := mw(okHandler)

	req := makeReq("test", nil)
	result, err := wrapped(context.Background(), req)
	if err != nil || result.IsError {
		t.Fatal("no args should pass through")
	}
}
