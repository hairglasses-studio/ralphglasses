package mcpserver

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
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
	t.Parallel()
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
	t.Parallel()
	rec := NewToolCallRecorder("", nil, 100)
	mw := InstrumentationMiddleware(rec)

	errHandler := func(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return codedError(ErrInternal, "something broke"), nil
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
	t.Parallel()
	mw := InstrumentationMiddleware(nil)
	wrapped := mw(okHandler)
	result, err := wrapped(context.Background(), makeReq("x", nil))
	if err != nil || result.IsError {
		t.Fatal("nil recorder should pass through without error")
	}
}

func TestEventBusMiddleware(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	mw := EventBusMiddleware(nil)
	wrapped := mw(okHandler)
	result, err := wrapped(context.Background(), makeReq("x", nil))
	if err != nil || result.IsError {
		t.Fatal("nil bus should pass through without error")
	}
}

func TestValidationMiddleware_ValidRepo(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestValidationMiddleware_AbsoluteRepoPath_WithinScanRoot(t *testing.T) {
	t.Parallel()
	mw := ValidationMiddleware("/tmp/scan")
	wrapped := mw(okHandler)

	req := makeReq("merge_verify", map[string]any{"repo": "/tmp/scan/my-repo"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("absolute repo path within scanRoot should pass validation")
	}
}

func TestValidationMiddleware_AbsoluteRepoPath_OutsideScanRoot(t *testing.T) {
	t.Parallel()
	mw := ValidationMiddleware("/tmp/scan")
	wrapped := mw(okHandler)

	req := makeReq("coverage_report", map[string]any{"repo": "/etc/passwd"})
	result, err := wrapped(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.IsError {
		t.Fatal("absolute repo path outside scanRoot should be rejected")
	}
}

func TestValidationMiddleware_NoArgs(t *testing.T) {
	t.Parallel()
	mw := ValidationMiddleware("")
	wrapped := mw(okHandler)

	req := makeReq("test", nil)
	result, err := wrapped(context.Background(), req)
	if err != nil || result.IsError {
		t.Fatal("no args should pass through")
	}
}

func TestTraceMiddleware_GeneratesID(t *testing.T) {
	t.Parallel()
	mw := TraceMiddleware()

	// Use a handler that captures the trace ID from context.
	var capturedID string
	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		capturedID = tracing.TraceIDFromContext(ctx)
		return jsonResult(map[string]any{"status": "ok"}), nil
	}

	wrapped := mw(handler)
	result, err := wrapped(context.Background(), makeReq("test_tool", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify trace ID was generated and passed to handler.
	if capturedID == "" {
		t.Fatal("expected trace ID in context, got empty")
	}
	if len(capturedID) != 16 {
		t.Fatalf("expected 16-char trace ID, got %d: %q", len(capturedID), capturedID)
	}

	// Verify trace ID is in the JSON response.
	if len(result.Content) == 0 {
		t.Fatal("expected content in result")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var respMap map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &respMap); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if respMap["_trace_id"] != capturedID {
		t.Fatalf("expected _trace_id=%q in response, got %v", capturedID, respMap["_trace_id"])
	}
}

func TestTraceMiddleware_PreservesExisting(t *testing.T) {
	t.Parallel()
	mw := TraceMiddleware()

	existingID := "abcdef0123456789"
	var capturedID string
	handler := func(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		capturedID = tracing.TraceIDFromContext(ctx)
		return jsonResult(map[string]any{"status": "ok"}), nil
	}

	wrapped := mw(handler)
	ctx := tracing.WithTraceID(context.Background(), existingID)
	result, err := wrapped(ctx, makeReq("test_tool", nil))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify the existing trace ID was preserved.
	if capturedID != existingID {
		t.Fatalf("expected preserved trace ID %q, got %q", existingID, capturedID)
	}

	// Verify response contains the existing trace ID.
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	var respMap map[string]any
	if err := json.Unmarshal([]byte(tc.Text), &respMap); err != nil {
		t.Fatalf("failed to parse response JSON: %v", err)
	}
	if respMap["_trace_id"] != existingID {
		t.Fatalf("expected _trace_id=%q in response, got %v", existingID, respMap["_trace_id"])
	}
}

func TestEventBusMiddleware_IncludesToolPayload(t *testing.T) {
	t.Parallel()
	bus := events.NewBus(100)
	ch := bus.Subscribe("test-payload")

	mw := EventBusMiddleware(bus)
	wrapped := mw(okHandler)

	repoPath := t.TempDir()
	req := makeReq("my_tool", map[string]any{"repo": repoPath, "message": "hello"})
	_, _ = wrapped(context.Background(), req)

	select {
	case evt := <-ch:
		if evt.RepoPath != repoPath {
			t.Fatalf("event repo path = %q, want %q", evt.RepoPath, repoPath)
		}
		raw, ok := evt.Data["tool_input_json"].(string)
		if ok == false {
			t.Fatalf("tool_input_json missing from event data: %+v", evt.Data)
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err != nil {
			t.Fatalf("unmarshal tool_input_json: %v", err)
		}
		if payload["message"] != "hello" {
			t.Fatalf("tool_input_json payload = %+v", payload)
		}
		if evt.Data["tool_output"] != "ok" {
			t.Fatalf("tool_output = %v, want ok", evt.Data["tool_output"])
		}
		if evt.Data["tool_result_is_error"] != false {
			t.Fatalf("tool_result_is_error = %v, want false", evt.Data["tool_result_is_error"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for tool.called payload event")
	}

	bus.Unsubscribe("test-payload")
}
