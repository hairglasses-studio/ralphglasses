package mcpserver

import (
	"fmt"
	"strings"

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

// Has returns true if the parameter key exists in the request arguments.
func (p *Params) Has(key string) bool {
	m := argsMap(p.req)
	if m == nil {
		return false
	}
	_, ok := m[key]
	return ok
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

// RequireInt returns the integer parameter or a coded INVALID_PARAMS error.
// JSON numbers are float64 — this truncates to int.
func (p *Params) RequireInt(key string) (int, *mcp.CallToolResult) {
	n, errResult := p.RequireNumber(key)
	if errResult != nil {
		return 0, errResult
	}
	return int(n), nil
}

// OptionalInt returns the integer parameter or defaultVal if not present.
func (p *Params) OptionalInt(key string, defaultVal int) int {
	return int(p.OptionalNumber(key, float64(defaultVal)))
}

// RequireEnum returns the string parameter if it matches one of the allowed values,
// or a coded INVALID_PARAMS error.
func (p *Params) RequireEnum(key string, allowed []string) (string, *mcp.CallToolResult) {
	val, errResult := p.RequireString(key)
	if errResult != nil {
		return "", errResult
	}
	for _, a := range allowed {
		if val == a {
			return val, nil
		}
	}
	return "", codedError(ErrInvalidParams, fmt.Sprintf("%s must be one of: %s", key, strings.Join(allowed, ", ")))
}

// OptionalEnum returns the string parameter if present and valid, or defaultVal.
// Returns a coded error if the value is present but not in the allowed set.
func (p *Params) OptionalEnum(key string, allowed []string, defaultVal string) (string, *mcp.CallToolResult) {
	val := getStringArg(p.req, key)
	if val == "" {
		return defaultVal, nil
	}
	for _, a := range allowed {
		if val == a {
			return val, nil
		}
	}
	return "", codedError(ErrInvalidParams, fmt.Sprintf("%s must be one of: %s", key, strings.Join(allowed, ", ")))
}

// OptionalLimit returns a clamped integer parameter suitable for pagination.
// Values below 1 or above max are clamped. Missing values return defaultVal.
func (p *Params) OptionalLimit(key string, defaultVal, max int) int {
	val := p.OptionalInt(key, defaultVal)
	if val < 1 {
		return 1
	}
	if val > max {
		return max
	}
	return val
}

// StringSlice returns the string parameter split by separator, or nil if empty.
func (p *Params) StringSlice(key, sep string) []string {
	val := getStringArg(p.req, key)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, sep)
	result := make([]string, 0, len(parts))
	for _, s := range parts {
		s = strings.TrimSpace(s)
		if s != "" {
			result = append(result, s)
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// Req returns the underlying CallToolRequest for cases where direct access
// is needed (e.g., passing to existing helpers).
func (p *Params) Req() mcp.CallToolRequest {
	return p.req
}
