package mcpserver

import (
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// ParamParser provides type-safe parameter extraction from a map[string]any
// with validation. Each typed getter returns a coded INVALID_PARAMS error
// when the key is missing or the value has the wrong type.
type ParamParser struct {
	args map[string]any
}

// NewParamParser wraps args for type-safe extraction. A nil map is safe;
// all required getters will return missing-key errors.
func NewParamParser(args map[string]any) *ParamParser {
	return &ParamParser{args: args}
}

// String returns the string value for key, or an INVALID_PARAMS error if
// the key is missing or the value is not a string.
func (p *ParamParser) String(key string) (string, *mcp.CallToolResult) {
	if p.args == nil {
		return "", codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := p.args[key]
	if !ok {
		return "", codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	s, ok := v.(string)
	if !ok {
		return "", codedError(ErrInvalidParams, fmt.Sprintf("%s must be a string", key))
	}
	return s, nil
}

// StringOpt returns the string value for key, or defaultVal if the key is
// absent or the value is not a string.
func (p *ParamParser) StringOpt(key, defaultVal string) string {
	if p.args == nil {
		return defaultVal
	}
	v, ok := p.args[key]
	if !ok {
		return defaultVal
	}
	s, ok := v.(string)
	if !ok {
		return defaultVal
	}
	return s
}

// Int returns the integer value for key. JSON numbers unmarshal as float64,
// so this method accepts float64 values that represent whole numbers.
func (p *ParamParser) Int(key string) (int, *mcp.CallToolResult) {
	if p.args == nil {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := p.args[key]
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	f, ok := v.(float64)
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s must be a number", key))
	}
	n := int(f)
	if float64(n) != f {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s must be an integer", key))
	}
	return n, nil
}

// IntOpt returns the integer value for key, or defaultVal if the key is
// absent or the value is not a number.
func (p *ParamParser) IntOpt(key string, defaultVal int) int {
	if p.args == nil {
		return defaultVal
	}
	v, ok := p.args[key]
	if !ok {
		return defaultVal
	}
	f, ok := v.(float64)
	if !ok {
		return defaultVal
	}
	return int(f)
}

// Float returns the float64 value for key, or an INVALID_PARAMS error if
// the key is missing or the value is not a number.
func (p *ParamParser) Float(key string) (float64, *mcp.CallToolResult) {
	if p.args == nil {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := p.args[key]
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	f, ok := v.(float64)
	if !ok {
		return 0, codedError(ErrInvalidParams, fmt.Sprintf("%s must be a number", key))
	}
	return f, nil
}

// Bool returns the boolean value for key, or an INVALID_PARAMS error if
// the key is missing or the value is not a boolean.
func (p *ParamParser) Bool(key string) (bool, *mcp.CallToolResult) {
	if p.args == nil {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	v, ok := p.args[key]
	if !ok {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s required", key))
	}
	b, ok := v.(bool)
	if !ok {
		return false, codedError(ErrInvalidParams, fmt.Sprintf("%s must be a boolean", key))
	}
	return b, nil
}
