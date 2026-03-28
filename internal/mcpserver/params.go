package mcpserver

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// Params provides ergonomic parameter extraction from MCP tool requests.
// It wraps the existing getStringArg/getNumberArg/getBoolArg helpers
// with required/optional variants that return typed errors.
type Params struct {
	req mcp.CallToolRequest
}

// NewParams wraps a CallToolRequest for parameter extraction.
func NewParams(req mcp.CallToolRequest) *Params {
	return &Params{req: req}
}

// RequireString returns the string parameter or a coded INVALID_PARAMS error.
func (p *Params) RequireString(key string) (string, *mcp.CallToolResult) {
	val := getStringArg(p.req, key)
	if val == "" {
		return "", codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	return val, nil
}

// OptionalString returns the string parameter or defaultVal if empty.
func (p *Params) OptionalString(key, defaultVal string) string {
	val := getStringArg(p.req, key)
	if val == "" {
		return defaultVal
	}
	return val
}

// RequireNumber returns the numeric parameter or a coded INVALID_PARAMS error
// if the parameter is not present (falls back to 0 check).
func (p *Params) RequireNumber(key string) (float64, *mcp.CallToolResult) {
	// Use a sentinel default that signals "not provided".
	m := argsMap(p.req)
	if m == nil {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := m[key]
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	n, ok := v.(float64)
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s must be a number", key))
	}
	return n, nil
}

// OptionalNumber returns the numeric parameter or defaultVal if not present.
func (p *Params) OptionalNumber(key string, defaultVal float64) float64 {
	return getNumberArg(p.req, key, defaultVal)
}

// RequireBool returns the boolean parameter or a coded INVALID_PARAMS error
// if the parameter is not present.
func (p *Params) RequireBool(key string) (bool, *mcp.CallToolResult) {
	m := argsMap(p.req)
	if m == nil {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := m[key]
	if !ok {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	b, ok := v.(bool)
	if !ok {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s must be a boolean", key))
	}
	return b, nil
}

// OptionalBool returns the boolean parameter or defaultVal if not present.
func (p *Params) OptionalBool(key string, defaultVal bool) bool {
	m := argsMap(p.req)
	if m == nil {
		return defaultVal
	}
	v, ok := m[key]
	if !ok {
		return defaultVal
	}
	b, ok := v.(bool)
	if !ok {
		return defaultVal
	}
	return b
}

// Req returns the underlying CallToolRequest for cases where direct access
// is needed (e.g., passing to existing helpers).
func (p *Params) Req() mcp.CallToolRequest {
	return p.req
}
