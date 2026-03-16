package mcpserver

import (
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func FuzzGetStringArg(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("repo", "test-repo")

	f.Fuzz(func(t *testing.T, key, value string) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{key: value},
			},
		}
		got := getStringArg(req, key)
		if got != value {
			t.Errorf("getStringArg(%q) = %q, want %q", key, got, value)
		}
	})
}

func FuzzGetNumberArg(f *testing.F) {
	f.Add("lines", 50.0, 10.0)
	f.Add("", 0.0, 0.0)

	f.Fuzz(func(t *testing.T, key string, val, defaultVal float64) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{key: val},
			},
		}
		getNumberArg(req, key, defaultVal)
	})
}
