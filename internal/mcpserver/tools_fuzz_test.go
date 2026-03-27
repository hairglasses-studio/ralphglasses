package mcpserver

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// Run: go test -fuzz=FuzzGetStringArg -fuzztime=30s ./internal/mcpserver/...
func FuzzGetStringArg(f *testing.F) {
	f.Add("key", "value")
	f.Add("", "")
	f.Add("repo", "test-repo")
	f.Add("path", "   \n\t  ")
	f.Add("unicode", "こんにちは世界 🎉")
	f.Add("nulls", "\x00\x00")
	f.Add("long", strings.Repeat("a", 500))
	f.Add("special", "key=value&foo=bar\r\n")

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

// Run: go test -fuzz=FuzzGetNumberArg -fuzztime=30s ./internal/mcpserver/...
func FuzzGetNumberArg(f *testing.F) {
	f.Add("lines", 50.0, 10.0)
	f.Add("", 0.0, 0.0)
	f.Add("count", -1.0, 0.0)
	f.Add("big", 1e18, 0.0)
	f.Add("tiny", 1e-300, 1e-300)
	f.Add("nan-adjacent", 0.0, -0.0)

	f.Fuzz(func(t *testing.T, key string, val, defaultVal float64) {
		req := mcp.CallToolRequest{
			Params: mcp.CallToolParams{
				Arguments: map[string]any{key: val},
			},
		}
		getNumberArg(req, key, defaultVal)
	})
}
