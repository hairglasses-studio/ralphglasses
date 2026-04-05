package session

import "testing"

func TestToKebabCase(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MCP Caching Patterns", "mcp-caching-patterns"},
		{"go-ecosystem overview", "go-ecosystem-overview"},
		{"Hello, World!", "hello-world"},
		{"multiple   spaces", "multiple-spaces"},
		{"ALLCAPS", "allcaps"},
		{"with/slashes/and\\backslashes", "with-slashes-and-backslashes"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"already-kebab-case", "already-kebab-case"},
		{"123-numbers-456", "123-numbers-456"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toKebabCase(tt.input)
			if got != tt.want {
				t.Errorf("toKebabCase(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// TestDocsResearchGatewayImplementsInterface ensures the concrete type
// satisfies the ResearchGateway interface at compile time.
func TestDocsResearchGatewayImplementsInterface(t *testing.T) {
	var _ ResearchGateway = (*DocsResearchGateway)(nil)
}
