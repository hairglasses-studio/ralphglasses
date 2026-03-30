package mcpserver

import (
	"errors"
	"fmt"
)

// RPCCode represents a JSON-RPC 2.0 / MCP protocol-level error code.
// Standard JSON-RPC codes live in the -32xxx range; application codes
// start at -32000.
//
// These complement the string-based ErrorCode constants in errors.go,
// which are used for structured tool-result payloads. RPCCode is for
// protocol-level error signaling (e.g., invalid params, method not found).
type RPCCode int

const (
	// JSON-RPC 2.0 standard error codes.
	ErrRPCInvalidParams   RPCCode = -32602
	ErrRPCMethodNotFound  RPCCode = -32601
	ErrRPCInternal        RPCCode = -32603

	// Application-defined MCP error codes (-32000 to -32099).
	ErrRPCToolNotFound      RPCCode = -32000
	ErrRPCToolExecFailed    RPCCode = -32001
	ErrRPCResourceNotFound  RPCCode = -32002
	ErrRPCPermissionDenied  RPCCode = -32003
	ErrRPCRateLimited       RPCCode = -32004
	ErrRPCBudgetExceeded    RPCCode = -32005
)

// MCPError is a structured error carrying a JSON-RPC-style code, a
// human-readable message, and optional machine-readable data.
type MCPError struct {
	Code    RPCCode
	Message string
	Data    any
}

// NewMCPError creates an MCPError. An optional data argument (first only)
// is attached for structured context.
func NewMCPError(code RPCCode, msg string, data ...any) *MCPError {
	e := &MCPError{Code: code, Message: msg}
	if len(data) > 0 {
		e.Data = data[0]
	}
	return e
}

// WrapError creates an MCPError from an existing error, using the error's
// message and preserving the original as Data for unwrapping.
func WrapError(code RPCCode, err error) *MCPError {
	return &MCPError{
		Code:    code,
		Message: err.Error(),
		Data:    err,
	}
}

// Error implements the error interface.
func (e *MCPError) Error() string {
	return fmt.Sprintf("mcp %d: %s", e.Code, e.Message)
}

// Is supports errors.Is by matching on code equality when the target is
// also an *MCPError.
func (e *MCPError) Is(target error) bool {
	var t *MCPError
	if errors.As(target, &t) {
		return e.Code == t.Code
	}
	return false
}

// Unwrap returns the wrapped error if Data holds one, enabling errors.Unwrap.
func (e *MCPError) Unwrap() error {
	if err, ok := e.Data.(error); ok {
		return err
	}
	return nil
}

// IsMCPError checks whether err (or any error in its chain) is an *MCPError.
// If so it returns the matched error and true; otherwise nil, false.
func IsMCPError(err error) (*MCPError, bool) {
	var me *MCPError
	if errors.As(err, &me) {
		return me, true
	}
	return nil, false
}
