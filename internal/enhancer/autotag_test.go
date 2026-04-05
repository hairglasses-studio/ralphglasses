package enhancer

import "testing"

func TestAutoTag_GoPrompt(t *testing.T) {
	r := AutoTag("Write a Go function that implements a sync.Mutex-protected cache with goroutine-safe reads")
	if len(r.Tags) == 0 {
		t.Fatal("expected tags for Go prompt")
	}
	hasGo := false
	for _, tag := range r.Tags {
		if tag == "go" {
			hasGo = true
			break
		}
	}
	if !hasGo {
		t.Errorf("expected 'go' tag, got %v", r.Tags)
	}
}

func TestAutoTag_MCPPrompt(t *testing.T) {
	r := AutoTag("Create a new MCP tool handler using mcpkit that exposes a registry search API")
	hasMCP := false
	for _, tag := range r.Tags {
		if tag == "mcp" {
			hasMCP = true
		}
	}
	if !hasMCP {
		t.Errorf("expected 'mcp' tag, got %v", r.Tags)
	}
}

func TestAutoTag_EmptyPrompt(t *testing.T) {
	r := AutoTag("")
	if len(r.Tags) == 0 {
		t.Fatal("expected at least 'general' tag")
	}
	if r.Tags[0] != "general" {
		t.Errorf("expected 'general' fallback, got %v", r.Tags)
	}
}

func TestAutoTag_MultiDomain(t *testing.T) {
	r := AutoTag("Deploy the Go MCP server to Kubernetes with TLS certificates and rate limiting")
	if len(r.Tags) < 2 {
		t.Errorf("expected multiple domain tags, got %v", r.Tags)
	}
}

func TestAutoTagWithTaskType(t *testing.T) {
	tags, tt := AutoTagWithTaskType("Write a Go function that implements sorting with benchmarks")
	if len(tags) == 0 {
		t.Error("expected non-empty tags")
	}
	if tt == "" {
		t.Error("expected non-empty task type")
	}
}
