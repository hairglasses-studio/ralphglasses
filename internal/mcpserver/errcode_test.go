package mcpserver

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewMCPError(t *testing.T) {
	t.Parallel()

	t.Run("without data", func(t *testing.T) {
		e := NewMCPError(ErrRPCInvalidParams, "bad field")
		if e.Code != ErrRPCInvalidParams {
			t.Errorf("Code = %d, want %d", e.Code, ErrRPCInvalidParams)
		}
		if e.Message != "bad field" {
			t.Errorf("Message = %q, want %q", e.Message, "bad field")
		}
		if e.Data != nil {
			t.Errorf("Data = %v, want nil", e.Data)
		}
	})

	t.Run("with data", func(t *testing.T) {
		extra := map[string]string{"param": "id"}
		e := NewMCPError(ErrRPCInvalidParams, "missing id", extra)
		if e.Data == nil {
			t.Fatal("Data should not be nil")
		}
		m, ok := e.Data.(map[string]string)
		if !ok {
			t.Fatalf("Data type = %T, want map[string]string", e.Data)
		}
		if m["param"] != "id" {
			t.Errorf("Data[param] = %q, want %q", m["param"], "id")
		}
	})

	t.Run("extra data args ignored", func(t *testing.T) {
		e := NewMCPError(ErrRPCInternal, "boom", "first", "second")
		if e.Data != "first" {
			t.Errorf("Data = %v, want %q", e.Data, "first")
		}
	})
}

func TestWrapError(t *testing.T) {
	t.Parallel()

	orig := fmt.Errorf("connection refused")
	e := WrapError(ErrRPCInternal, orig)

	if e.Code != ErrRPCInternal {
		t.Errorf("Code = %d, want %d", e.Code, ErrRPCInternal)
	}
	if e.Message != "connection refused" {
		t.Errorf("Message = %q, want %q", e.Message, "connection refused")
	}

	// Unwrap should return the original error.
	unwrapped := e.Unwrap()
	if unwrapped == nil {
		t.Fatal("Unwrap returned nil")
	}
	if unwrapped.Error() != orig.Error() {
		t.Errorf("Unwrap().Error() = %q, want %q", unwrapped.Error(), orig.Error())
	}
}

func TestMCPError_Error_Format(t *testing.T) {
	t.Parallel()

	tests := []struct {
		code RPCCode
		msg  string
		want string
	}{
		{ErrRPCInvalidParams, "missing name", "mcp -32602: missing name"},
		{ErrRPCToolNotFound, "no such tool", "mcp -32000: no such tool"},
		{ErrRPCInternal, "unexpected", "mcp -32603: unexpected"},
	}

	for _, tt := range tests {
		e := NewMCPError(tt.code, tt.msg)
		if got := e.Error(); got != tt.want {
			t.Errorf("Error() = %q, want %q", got, tt.want)
		}
	}
}

func TestMCPError_Is(t *testing.T) {
	t.Parallel()

	a := NewMCPError(ErrRPCInvalidParams, "first")
	b := NewMCPError(ErrRPCInvalidParams, "second")
	c := NewMCPError(ErrRPCInternal, "other")

	// Same code should match via errors.Is.
	if !errors.Is(a, b) {
		t.Error("errors.Is(a, b) should be true (same code)")
	}

	// Different codes should not match.
	if errors.Is(a, c) {
		t.Error("errors.Is(a, c) should be false (different code)")
	}

	// Should not match a plain error.
	plain := fmt.Errorf("plain")
	if errors.Is(a, plain) {
		t.Error("errors.Is(MCPError, plainError) should be false")
	}

	// Wrapped MCPError should still be findable.
	wrapped := fmt.Errorf("outer: %w", a)
	if !errors.Is(wrapped, b) {
		t.Error("errors.Is(wrapped, b) should be true through chain")
	}
}

func TestIsMCPError(t *testing.T) {
	t.Parallel()

	t.Run("direct MCPError", func(t *testing.T) {
		e := NewMCPError(ErrRPCToolExecFailed, "exec failed")
		got, ok := IsMCPError(e)
		if !ok {
			t.Fatal("IsMCPError should return true")
		}
		if got.Code != ErrRPCToolExecFailed {
			t.Errorf("Code = %d, want %d", got.Code, ErrRPCToolExecFailed)
		}
	})

	t.Run("wrapped MCPError", func(t *testing.T) {
		inner := NewMCPError(ErrRPCResourceNotFound, "not found")
		wrapped := fmt.Errorf("context: %w", inner)
		got, ok := IsMCPError(wrapped)
		if !ok {
			t.Fatal("IsMCPError should find wrapped MCPError")
		}
		if got.Code != ErrRPCResourceNotFound {
			t.Errorf("Code = %d, want %d", got.Code, ErrRPCResourceNotFound)
		}
	})

	t.Run("plain error returns false", func(t *testing.T) {
		plain := fmt.Errorf("not an MCP error")
		got, ok := IsMCPError(plain)
		if ok {
			t.Error("IsMCPError should return false for plain error")
		}
		if got != nil {
			t.Error("IsMCPError should return nil for plain error")
		}
	})

	t.Run("nil error returns false", func(t *testing.T) {
		got, ok := IsMCPError(nil)
		if ok {
			t.Error("IsMCPError(nil) should return false")
		}
		if got != nil {
			t.Error("IsMCPError(nil) should return nil")
		}
	})
}

func TestErrorCode_Constants(t *testing.T) {
	t.Parallel()

	// Verify JSON-RPC 2.0 standard codes.
	stdCodes := map[string]RPCCode{
		"InvalidParams":  ErrRPCInvalidParams,
		"MethodNotFound": ErrRPCMethodNotFound,
		"Internal":       ErrRPCInternal,
	}
	for name, code := range stdCodes {
		if code > -32600 || code < -32700 {
			t.Errorf("%s = %d, want value in [-32700, -32600]", name, code)
		}
	}

	// Verify application codes are in the server-defined range.
	appCodes := map[string]RPCCode{
		"ToolNotFound":     ErrRPCToolNotFound,
		"ToolExecFailed":   ErrRPCToolExecFailed,
		"ResourceNotFound": ErrRPCResourceNotFound,
		"PermissionDenied": ErrRPCPermissionDenied,
		"RateLimited":      ErrRPCRateLimited,
		"BudgetExceeded":   ErrRPCBudgetExceeded,
	}
	for name, code := range appCodes {
		if code > -32000 || code < -32099 {
			t.Errorf("%s = %d, want value in [-32099, -32000]", name, code)
		}
	}

	// All codes must be unique.
	seen := make(map[RPCCode]string)
	all := map[string]RPCCode{}
	for k, v := range stdCodes {
		all[k] = v
	}
	for k, v := range appCodes {
		all[k] = v
	}
	for name, code := range all {
		if prev, dup := seen[code]; dup {
			t.Errorf("duplicate code %d: %s and %s", code, prev, name)
		}
		seen[code] = name
	}
}
