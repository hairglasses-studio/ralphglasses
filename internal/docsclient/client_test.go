package docsclient

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNew_Defaults(t *testing.T) {
	c := New("", "")

	if c.BinaryPath() != "docs-mcp" {
		t.Errorf("BinaryPath() = %q, want %q", c.BinaryPath(), "docs-mcp")
	}

	home, _ := os.UserHomeDir()
	wantRoot := filepath.Join(home, "hairglasses-studio", "docs")
	if c.DocsRoot() != wantRoot {
		t.Errorf("DocsRoot() = %q, want %q", c.DocsRoot(), wantRoot)
	}

	if c.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want %v", c.timeout, 30*time.Second)
	}
}

func TestNew_CustomValues(t *testing.T) {
	c := New("/usr/local/bin/docs-mcp", "/tmp/docs", WithTimeout(10*time.Second))

	if c.BinaryPath() != "/usr/local/bin/docs-mcp" {
		t.Errorf("BinaryPath() = %q, want %q", c.BinaryPath(), "/usr/local/bin/docs-mcp")
	}
	if c.DocsRoot() != "/tmp/docs" {
		t.Errorf("DocsRoot() = %q, want %q", c.DocsRoot(), "/tmp/docs")
	}
	if c.timeout != 10*time.Second {
		t.Errorf("timeout = %v, want %v", c.timeout, 10*time.Second)
	}
}

func TestKnowledgeResult_JSONRoundTrip(t *testing.T) {
	original := KnowledgeResult{
		Topic:          "MCP protocol versioning",
		Confidence:     0.85,
		Recommendation: "Existing research covers this well. Build upon docs/research/mcp/protocol-versions.md.",
		ExistingDocs:   3,
		Sources:        []string{"research/mcp/protocol-versions.md", "research/mcp/transport.md"},
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded KnowledgeResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if decoded.Topic != original.Topic {
		t.Errorf("Topic = %q, want %q", decoded.Topic, original.Topic)
	}
	if decoded.Confidence != original.Confidence {
		t.Errorf("Confidence = %v, want %v", decoded.Confidence, original.Confidence)
	}
	if decoded.Recommendation != original.Recommendation {
		t.Errorf("Recommendation mismatch")
	}
	if decoded.ExistingDocs != original.ExistingDocs {
		t.Errorf("ExistingDocs = %d, want %d", decoded.ExistingDocs, original.ExistingDocs)
	}
	if len(decoded.Sources) != len(original.Sources) {
		t.Errorf("Sources len = %d, want %d", len(decoded.Sources), len(original.Sources))
	}
}

func TestKnowledgeResult_JSONOmitsEmptySources(t *testing.T) {
	kr := KnowledgeResult{
		Topic:      "test",
		Confidence: 0.0,
	}
	data, err := json.Marshal(kr)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// sources should be omitted when nil
	var m map[string]any
	json.Unmarshal(data, &m)
	if _, ok := m["sources"]; ok {
		t.Error("sources should be omitted when nil")
	}
}

func TestPersistRequest_Validate(t *testing.T) {
	tests := []struct {
		name    string
		req     PersistRequest
		wantErr string
	}{
		{
			name:    "empty title",
			req:     PersistRequest{Domain: "mcp", Content: "body"},
			wantErr: "title is required",
		},
		{
			name:    "empty domain",
			req:     PersistRequest{Title: "Test", Content: "body"},
			wantErr: "domain is required",
		},
		{
			name:    "empty content",
			req:     PersistRequest{Title: "Test", Domain: "mcp"},
			wantErr: "content is required",
		},
		{
			name:    "invalid domain",
			req:     PersistRequest{Title: "Test", Domain: "invalid", Content: "body"},
			wantErr: "invalid domain",
		},
		{
			name: "valid request",
			req: PersistRequest{
				Title:   "MCP Research",
				Domain:  "mcp",
				Content: "# MCP Research\n\nFindings here.",
				Sources: []string{"https://example.com"},
			},
			wantErr: "",
		},
		{
			name: "all valid domains",
			req: PersistRequest{
				Title:   "Test",
				Domain:  "agents",
				Content: "content",
			},
			wantErr: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Errorf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Errorf("Validate() expected error containing %q, got nil", tt.wantErr)
				return
			}
			if !contains(err.Error(), tt.wantErr) {
				t.Errorf("Validate() error = %q, want containing %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestPersistRequest_AllDomains(t *testing.T) {
	for _, domain := range validDomainList() {
		req := PersistRequest{
			Title:   "Test",
			Domain:  domain,
			Content: "content",
		}
		if err := req.Validate(); err != nil {
			t.Errorf("domain %q should be valid, got: %v", domain, err)
		}
	}
}

func TestParseToolResponse_ValidResponse(t *testing.T) {
	// Simulate MCP JSON-RPC response with tool result.
	resp := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"confidence\":0.75,\"recommendation\":\"Build upon existing.\",\"existing_docs\":2}"}]}}` + "\n"

	raw, err := parseToolResponse([]byte(resp))
	if err != nil {
		t.Fatalf("parseToolResponse: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if m["confidence"].(float64) != 0.75 {
		t.Errorf("confidence = %v, want 0.75", m["confidence"])
	}
}

func TestParseToolResponse_ErrorResponse(t *testing.T) {
	resp := `{"jsonrpc":"2.0","id":1,"error":{"code":-32600,"message":"invalid request"}}` + "\n"

	_, err := parseToolResponse([]byte(resp))
	if err == nil {
		t.Fatal("expected error for error response")
	}
	if !contains(err.Error(), "MCP error") {
		t.Errorf("error = %q, want containing 'MCP error'", err.Error())
	}
}

func TestParseToolResponse_ToolError(t *testing.T) {
	resp := `{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"something went wrong"}],"isError":true}}` + "\n"

	_, err := parseToolResponse([]byte(resp))
	if err == nil {
		t.Fatal("expected error for isError=true")
	}
	if !contains(err.Error(), "tool error") {
		t.Errorf("error = %q, want containing 'tool error'", err.Error())
	}
}

func TestParseToolResponse_MultipleLines(t *testing.T) {
	// Simulate init response + tool response.
	lines := `{"jsonrpc":"2.0","id":0,"result":{"protocolVersion":"2024-11-05","capabilities":{},"serverInfo":{"name":"docs-mcp","version":"0.2.0"}}}
{"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"confidence\":0.5}"}]}}
`

	raw, err := parseToolResponse([]byte(lines))
	if err != nil {
		t.Fatalf("parseToolResponse: %v", err)
	}

	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m["confidence"].(float64) != 0.5 {
		t.Errorf("confidence = %v, want 0.5", m["confidence"])
	}
}

func TestParseToolResponse_NoOutput(t *testing.T) {
	_, err := parseToolResponse([]byte(""))
	if err == nil {
		t.Fatal("expected error for empty output")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstr(s, substr)
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
