package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestTimeoutMiddleware_Success(t *testing.T) {
	mw := TimeoutMiddleware(1*time.Second, nil)

	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}},
		}, nil
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "test_tool"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if result.IsError {
		t.Fatal("expected success result, got error")
	}
}

func TestTimeoutMiddleware_Timeout(t *testing.T) {
	mw := TimeoutMiddleware(50*time.Millisecond, nil)

	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case <-time.After(5 * time.Second):
			return &mcp.CallToolResult{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "slow_tool"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("expected nil error (coded error in result), got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result for timed-out handler")
	}

	// Verify error contains expected information.
	if len(result.Content) == 0 {
		t.Fatal("expected content in error result")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}

	var errData map[string]string
	if err := json.Unmarshal([]byte(tc.Text), &errData); err != nil {
		t.Fatalf("failed to parse error JSON: %v", err)
	}
	if errData["error_code"] != string(ErrInternal) {
		t.Fatalf("expected error_code %s, got %s", ErrInternal, errData["error_code"])
	}
	if !strings.Contains(errData["error"], "slow_tool") {
		t.Fatalf("expected error to mention tool name, got: %s", errData["error"])
	}
	if !strings.Contains(errData["error"], "timed out") {
		t.Fatalf("expected error to mention timeout, got: %s", errData["error"])
	}
}

func TestTimeoutMiddleware_ContextAlreadyCanceled(t *testing.T) {
	mw := TimeoutMiddleware(1*time.Second, nil)

	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		select {
		case <-time.After(5 * time.Second):
			return &mcp.CallToolResult{}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	req := mcp.CallToolRequest{}
	req.Params.Name = "any_tool"

	result, err := handler(ctx, req)
	// With an already-canceled context, the timeout middleware's derived context
	// is also done immediately, so we expect the coded error path.
	if err != nil {
		t.Fatalf("expected nil error (coded error in result), got: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if !result.IsError {
		t.Fatal("expected error result for already-canceled context")
	}
}

func TestTimeoutMiddleware_Override(t *testing.T) {
	// Default is 10ms (very short), but override gives 2s for "slow_ok_tool".
	mw := TimeoutMiddleware(10*time.Millisecond, map[string]time.Duration{
		"slow_ok_tool": 2 * time.Second,
	})

	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(50 * time.Millisecond) // Would timeout with default but not with override.
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}},
		}, nil
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "slow_ok_tool"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success — override should have extended timeout")
	}
}

func TestTimeoutMiddleware_Exempt(t *testing.T) {
	// Default is 10ms, but "exempt_tool" is mapped to 0 (exempt).
	mw := TimeoutMiddleware(10*time.Millisecond, map[string]time.Duration{
		"exempt_tool": 0,
	})

	handler := mw(func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		time.Sleep(50 * time.Millisecond) // Would timeout with default.
		return &mcp.CallToolResult{
			Content: []mcp.Content{mcp.TextContent{Type: "text", Text: "ok"}},
		}, nil
	})

	req := mcp.CallToolRequest{}
	req.Params.Name = "exempt_tool"

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.IsError {
		t.Fatal("expected success — exempt tool should skip timeout entirely")
	}
}
