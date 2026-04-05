package batch

import "testing"

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"MCP Caching Patterns", "mcp-caching-patterns"},
		{"hello world 123", "hello-world-123"},
		{"special!@#chars", "special---chars"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeID(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestModelForComplexity(t *testing.T) {
	tests := []struct {
		complexity int
		wantModel  string
	}{
		{1, "gemini-2.0-flash-lite"},
		{2, "gemini-2.5-flash"},
		{3, "claude-sonnet-4-20250514"},
		{4, "claude-opus-4-20250514"},
		{0, "gemini-2.5-flash"},
	}
	for _, tt := range tests {
		got := modelForComplexity(tt.complexity)
		if got != tt.wantModel {
			t.Errorf("modelForComplexity(%d) = %q, want %q", tt.complexity, got, tt.wantModel)
		}
	}
}

func TestMaxTokensForComplexity(t *testing.T) {
	if got := maxTokensForComplexity(1); got != 2048 {
		t.Errorf("complexity 1: got %d, want 2048", got)
	}
	if got := maxTokensForComplexity(4); got != 16384 {
		t.Errorf("complexity 4: got %d, want 16384", got)
	}
}

func TestResearchSystemPrompt(t *testing.T) {
	low := researchSystemPrompt(1)
	if len(low) == 0 {
		t.Error("empty system prompt for complexity 1")
	}

	high := researchSystemPrompt(3)
	if len(high) <= len(low) {
		t.Error("complexity 3 prompt should be longer than complexity 1")
	}
}

func TestResearchUserPrompt(t *testing.T) {
	prompt := researchUserPrompt("test-topic", "mcp", "new", 2)
	if prompt == "" {
		t.Error("empty user prompt")
	}

	expand := researchUserPrompt("test-topic", "mcp", "expand", 2)
	if expand == prompt {
		t.Error("expand prompt should differ from new prompt")
	}
}

func TestNewResearchBatchAdapter(t *testing.T) {
	// Verify constructor doesn't panic with nil manager.
	adapter := NewResearchBatchAdapter(nil)
	if adapter == nil {
		t.Fatal("expected non-nil adapter")
	}
	if adapter.PendingCount() != 0 {
		t.Errorf("expected 0 pending, got %d", adapter.PendingCount())
	}
}

func TestParseResultsMatchesPending(t *testing.T) {
	adapter := NewResearchBatchAdapter(nil)

	// Manually add pending items.
	adapter.mu.Lock()
	adapter.pending["research-mcp-caching"] = researchItem{
		Topic: "caching", Domain: "mcp", Mode: "new", Complexity: 2,
	}
	adapter.mu.Unlock()

	results := adapter.ParseResults([]Result{
		{RequestID: "research-mcp-caching", Content: "research content"},
		{RequestID: "unknown-id", Content: "should be ignored"},
	})

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Topic != "caching" {
		t.Errorf("unexpected topic: %s", results[0].Topic)
	}
	if results[0].Content != "research content" {
		t.Errorf("unexpected content: %s", results[0].Content)
	}

	// Pending should be cleared.
	if adapter.PendingCount() != 0 {
		t.Errorf("expected 0 pending after parse, got %d", adapter.PendingCount())
	}
}
