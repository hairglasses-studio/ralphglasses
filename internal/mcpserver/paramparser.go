package mcpserver

import (
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// ParamParser provides typed parameter extraction from MCP request arguments.
// All simple getters return zero values when the key is missing or the value
// has the wrong type. Use [Required] for upfront validation of mandatory keys.
type ParamParser struct {
	args map[string]any
}

// NewParamParser wraps args for type-safe extraction. A nil map is safe;
// all getters will return zero/default values.
func NewParamParser(args map[string]any) *ParamParser {
	return &ParamParser{args: args}
}

// Has reports whether key exists in the argument map.
func (p *ParamParser) Has(key string) bool {
	if p.args == nil {
		return false
	}
	_, ok := p.args[key]
	return ok
}

// Required validates that all specified keys are present in the argument map.
// It returns an error listing every missing key, or nil if all are present.
func (p *ParamParser) Required(keys ...string) error {
	var missing []string
	for _, k := range keys {
		if !p.Has(k) {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required parameter(s): %s", strings.Join(missing, ", "))
	}
	return nil
}

// String returns the string value for key, or "" if the key is missing or
// the value is not a string.
func (p *ParamParser) String(key string) string {
	if p.args == nil {
		return ""
	}
	v, ok := p.args[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

// StringOr returns the string value for key, or defaultVal if the key is
// absent or the value is not a string.
func (p *ParamParser) StringOr(key, defaultVal string) string {
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

// StringOpt is an alias for [StringOr] for backward compatibility.
func (p *ParamParser) StringOpt(key, defaultVal string) string {
	return p.StringOr(key, defaultVal)
}

// Bool returns the boolean value for key, or false if the key is missing or
// the value is not a boolean.
func (p *ParamParser) Bool(key string) bool {
	if p.args == nil {
		return false
	}
	v, ok := p.args[key]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	if !ok {
		return false
	}
	return b
}

// Int returns the integer value for key, or 0 if the key is missing or
// the value is not a number. JSON numbers unmarshal as float64, so this
// method accepts float64 values.
func (p *ParamParser) Int(key string) int {
	if p.args == nil {
		return 0
	}
	v, ok := p.args[key]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return int(f)
}

// IntOr returns the integer value for key, or defaultVal if the key is
// absent or the value is not a number.
func (p *ParamParser) IntOr(key string, defaultVal int) int {
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

// IntOpt is an alias for [IntOr] for backward compatibility.
func (p *ParamParser) IntOpt(key string, defaultVal int) int {
	return p.IntOr(key, defaultVal)
}

// Float returns the float64 value for key, or 0 if the key is missing or
// the value is not a number.
func (p *ParamParser) Float(key string) float64 {
	if p.args == nil {
		return 0
	}
	v, ok := p.args[key]
	if !ok {
		return 0
	}
	f, ok := v.(float64)
	if !ok {
		return 0
	}
	return f
}

// StringSlice returns a []string for key, or nil if the key is missing or
// the value is not a []any containing strings. Non-string elements in the
// slice are silently skipped.
func (p *ParamParser) StringSlice(key string) []string {
	if p.args == nil {
		return nil
	}
	v, ok := p.args[key]
	if !ok {
		return nil
	}
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, elem := range raw {
		if s, ok := elem.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// --- Error-returning variants for MCP handler use ---

// StringErr returns the string value for key, or a coded INVALID_PARAMS
// CallToolResult if the key is missing or the value is not a string.
func (p *ParamParser) StringErr(key string) (string, *mcp.CallToolResult) {
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

// IntErr returns the integer value for key, or a coded INVALID_PARAMS
// CallToolResult if the key is missing or not a whole number.
func (p *ParamParser) IntErr(key string) (int, *mcp.CallToolResult) {
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

// FloatErr returns the float64 value for key, or a coded INVALID_PARAMS
// CallToolResult if the key is missing or not a number.
func (p *ParamParser) FloatErr(key string) (float64, *mcp.CallToolResult) {
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

// BoolErr returns the boolean value for key, or a coded INVALID_PARAMS
// CallToolResult if the key is missing or not a boolean.
func (p *ParamParser) BoolErr(key string) (bool, *mcp.CallToolResult) {
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
