package common

import (
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
)

// GetStringParam extracts a string parameter with a default value.
func GetStringParam(req mcp.CallToolRequest, key string, defaultVal string) string {
	return req.GetString(key, defaultVal)
}

// RequireStringParam extracts a required string parameter.
func RequireStringParam(req mcp.CallToolRequest, key string) (string, bool) {
	val, err := req.RequireString(key)
	if err != nil {
		return "", false
	}
	return val, true
}

// GetIntParam extracts an integer parameter with a default value.
func GetIntParam(req mcp.CallToolRequest, key string, defaultVal int) int {
	return req.GetInt(key, defaultVal)
}

// GetLimitParam extracts a limit parameter clamped to [1, 100].
func GetLimitParam(req mcp.CallToolRequest, defaultVal int) int {
	v := req.GetInt("limit", defaultVal)
	if v < 1 {
		return 1
	}
	if v > 100 {
		return 100
	}
	return v
}

// GetOffsetParam extracts an offset parameter clamped to >= 0.
func GetOffsetParam(req mcp.CallToolRequest) int {
	v := req.GetInt("offset", 0)
	if v < 0 {
		return 0
	}
	return v
}

// GetBoolParam extracts a boolean parameter.
func GetBoolParam(req mcp.CallToolRequest, key string, defaultVal bool) bool {
	if args, ok := req.Params.Arguments.(map[string]interface{}); ok {
		if val, exists := args[key]; exists {
			switch v := val.(type) {
			case bool:
				return v
			case string:
				return v == "true" || v == "1" || v == "yes"
			}
		}
	}
	return defaultVal
}

// GetFloatParam extracts a float64 parameter with a default value.
func GetFloatParam(req mcp.CallToolRequest, key string, defaultVal float64) float64 {
	return req.GetFloat(key, defaultVal)
}

// GetFlexibleStringParam extracts a string parameter that may arrive as a JSON number.
func GetFlexibleStringParam(req mcp.CallToolRequest, key string, defaultVal string) string {
	if s := req.GetString(key, ""); s != "" {
		return s
	}
	if args, ok := req.Params.Arguments.(map[string]interface{}); ok {
		if val, exists := args[key]; exists {
			if f, ok := val.(float64); ok {
				return fmt.Sprintf("%.0f", f)
			}
		}
	}
	return defaultVal
}

// GetEnumParam extracts a string parameter and validates against valid values.
func GetEnumParam(req mcp.CallToolRequest, key string, valid []string, defaultVal string) string {
	val := req.GetString(key, "")
	if val == "" {
		return defaultVal
	}
	for _, v := range valid {
		if val == v {
			return val
		}
	}
	return defaultVal
}

// RequireEnumParam extracts a required string parameter and validates it.
func RequireEnumParam(req mcp.CallToolRequest, key string, valid []string) (string, bool) {
	val, err := req.RequireString(key)
	if err != nil {
		return "", false
	}
	for _, v := range valid {
		if val == v {
			return val, true
		}
	}
	return "", false
}

// RequireJSONParam extracts a parameter that may arrive as a JSON string or parsed value.
func RequireJSONParam(req mcp.CallToolRequest, key string) (string, bool) {
	if val, err := req.RequireString(key); err == nil && val != "" {
		return val, true
	}
	if args, ok := req.Params.Arguments.(map[string]interface{}); ok {
		if val, exists := args[key]; exists {
			b, err := json.Marshal(val)
			if err == nil && len(b) > 2 {
				return string(b), true
			}
		}
	}
	return "", false
}
