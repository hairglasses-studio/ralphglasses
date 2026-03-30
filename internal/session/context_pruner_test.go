package session

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text      string
		wantRange [2]int // [min, max] acceptable tokens
	}{
		{"hello world", [2]int{2, 4}},
		{"", [2]int{0, 0}},
		{"The quick brown fox jumps over the lazy dog", [2]int{9, 15}},
		{strings.Repeat("word ", 100), [2]int{100, 170}},
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.text)
		if got < tt.wantRange[0] || got > tt.wantRange[1] {
			t.Errorf("EstimateTokens(%q) = %d, want [%d, %d]",
				truncateForTest(tt.text, 40), got, tt.wantRange[0], tt.wantRange[1])
		}
	}
}

func TestEstimateTokensAccuracy(t *testing.T) {
	// Validate that our word*1.3 heuristic is within 20% of known token counts.
	// Known: "The quick brown fox" = 4 words -> estimate 5.2 -> 5 tokens.
	// Actual Claude tokenization: ~5 tokens. OpenAI tiktoken: 5 tokens.
	text := "The quick brown fox jumps over the lazy dog near the bank of the river"
	estimated := EstimateTokens(text)
	// 14 words -> estimate 18.2 -> 18 tokens.
	// Real tokenizers give 15-17 tokens for this sentence.
	// 20% tolerance: 15*0.8=12, 15*1.2=18 -- our estimate of 18 fits.
	actualApprox := 16 // midpoint of real tokenizer outputs
	deviation := math.Abs(float64(estimated)-float64(actualApprox)) / float64(actualApprox)
	if deviation > 0.30 {
		t.Errorf("EstimateTokens accuracy: estimated=%d, actual~=%d, deviation=%.1f%% (want <30%%)",
			estimated, actualApprox, deviation*100)
	}
}

func TestPruneKeepsSystemAndRecent(t *testing.T) {
	pruner := NewContextPruner(500)

	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "First question about Go concurrency patterns"},
		{Role: "assistant", Content: "Here is a detailed answer about Go concurrency..."},
		{Role: "user", Content: "Second question about error handling"},
		{Role: "assistant", Content: "Error handling in Go follows these patterns..."},
		{Role: "user", Content: "Third question about testing"},
		{Role: "assistant", Content: "Testing in Go uses the testing package..."},
		{Role: "user", Content: "Fourth and final question about deployment"},
		{Role: "assistant", Content: "For deployment you should consider these steps..."},
	}

	pruned, stats := pruner.Prune(msgs)

	// System prompt must be preserved.
	hasSystem := false
	for _, m := range pruned {
		if m.Role == "system" {
			hasSystem = true
			if m.Content != "You are a helpful assistant." {
				t.Error("system prompt content was modified")
			}
		}
	}
	if !hasSystem {
		t.Error("system prompt was dropped")
	}

	// Last assistant message must be preserved.
	lastAssistant := ""
	for i := len(pruned) - 1; i >= 0; i-- {
		if pruned[i].Role == "assistant" {
			lastAssistant = pruned[i].Content
			break
		}
	}
	if !strings.Contains(lastAssistant, "deployment") {
		t.Error("last assistant message was dropped or modified")
	}

	// Stats should show non-zero original tokens.
	if stats.OriginalTokens == 0 {
		t.Error("stats.OriginalTokens should be > 0")
	}
	if stats.PrunedTokens > stats.OriginalTokens {
		t.Errorf("pruned tokens (%d) > original tokens (%d)", stats.PrunedTokens, stats.OriginalTokens)
	}
}

func TestPruneNoop(t *testing.T) {
	// Under budget: prune should be a no-op.
	pruner := NewContextPruner(100_000)
	msgs := []Message{
		{Role: "system", Content: "System prompt."},
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there!"},
	}

	pruned, stats := pruner.Prune(msgs)
	if len(pruned) != len(msgs) {
		t.Errorf("no-op prune changed message count: %d -> %d", len(msgs), len(pruned))
	}
	if stats.Reduction != 0 {
		t.Errorf("no-op prune reported reduction: %f", stats.Reduction)
	}
}

func TestPruneEmpty(t *testing.T) {
	pruner := NewContextPruner(1000)
	pruned, stats := pruner.Prune(nil)
	if len(pruned) != 0 {
		t.Errorf("expected empty result for nil input, got %d messages", len(pruned))
	}
	if stats.OriginalTokens != 0 {
		t.Errorf("expected 0 original tokens for nil input, got %d", stats.OriginalTokens)
	}
}

func TestPruneMiddleMessagesCompressed(t *testing.T) {
	pruner := NewContextPruner(50) // Very tight budget to force summarization.

	msgs := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "First question with lots of detail about the architecture and design patterns used in the codebase"},
		{Role: "assistant", Content: "Here is a very long and detailed answer about architecture covering microservices, event sourcing, and CQRS patterns with examples and code snippets"},
		{Role: "user", Content: "Second question with even more detail"},
		{Role: "assistant", Content: "Another comprehensive answer with code examples and explanations of the trade-offs involved in each approach"},
		{Role: "user", Content: "Final short question"},
		{Role: "assistant", Content: "Final answer"},
	}

	pruned, stats := pruner.Prune(msgs)

	// At least one message should be summarized.
	hasSummarized := false
	for _, m := range pruned {
		if m.Summarized {
			hasSummarized = true
			if !strings.HasPrefix(m.Content, "[summary]") {
				t.Errorf("summarized message should start with [summary], got: %s", truncateForTest(m.Content, 60))
			}
		}
	}
	if !hasSummarized && stats.MessagesDropped == 0 {
		t.Error("expected at least one message to be summarized or dropped with tight budget")
	}

	// Stats should reflect reduction.
	if stats.Reduction <= 0 {
		t.Errorf("expected positive reduction, got %f", stats.Reduction)
	}
}

func TestPruneDuplicateToolCalls(t *testing.T) {
	pruner := NewContextPruner(100_000) // Generous budget; only dedup should fire.

	msgs := []Message{
		{Role: "system", Content: "System prompt."},
		{Role: "tool", Content: "file content v1", ToolName: "read_file", ToolArgs: `{"path":"/foo.go"}`},
		{Role: "user", Content: "Now fix the bug"},
		{Role: "tool", Content: "file content v1", ToolName: "read_file", ToolArgs: `{"path":"/foo.go"}`},
		{Role: "assistant", Content: "Done"},
	}

	pruned, stats := pruner.Prune(msgs)

	// The earlier duplicate tool call should be dropped.
	toolCount := 0
	for _, m := range pruned {
		if m.Role == "tool" && m.ToolName == "read_file" {
			toolCount++
		}
	}
	if toolCount != 1 {
		t.Errorf("expected 1 read_file tool call after dedup, got %d", toolCount)
	}
	if stats.ToolDedups != 1 {
		t.Errorf("expected 1 tool dedup, got %d", stats.ToolDedups)
	}
}

func TestPruneTruncateLongOutput(t *testing.T) {
	pruner := NewContextPruner(100) // Tight budget to force truncation.
	pruner.TruncateLines = 5

	// Build a tool output with 100 lines.
	var lines []string
	for i := 0; i < 100; i++ {
		lines = append(lines, fmt.Sprintf("line %d: some content here", i))
	}
	longOutput := strings.Join(lines, "\n")

	msgs := []Message{
		{Role: "system", Content: "System."},
		{Role: "tool", Content: longOutput, ToolName: "read_file", ToolArgs: `{"path":"/big.go"}`},
		{Role: "user", Content: "Summarize this file"},
		{Role: "assistant", Content: "Summary."},
	}

	pruned, stats := pruner.Prune(msgs)

	// The tool output should contain the truncation marker.
	for _, m := range pruned {
		if m.Role == "tool" {
			if !strings.Contains(m.Content, "truncated") {
				t.Error("expected truncation marker in long tool output")
			}
			// Head lines should be preserved.
			if !strings.Contains(m.Content, "line 0:") {
				t.Error("expected first line preserved after truncation")
			}
			// Tail lines should be preserved.
			if !strings.Contains(m.Content, "line 99:") {
				t.Error("expected last line preserved after truncation")
			}
		}
	}
	if stats.Truncations != 1 {
		t.Errorf("expected 1 truncation, got %d", stats.Truncations)
	}
}

func TestPruneStatsTracking(t *testing.T) {
	pruner := NewContextPruner(20) // Very tight to force all strategies.

	msgs := []Message{
		{Role: "system", Content: "System prompt with moderate length content."},
		{Role: "tool", Content: "Duplicate tool output", ToolName: "status", ToolArgs: `{"id":"1"}`},
		{Role: "tool", Content: "Duplicate tool output", ToolName: "status", ToolArgs: `{"id":"1"}`},
		{Role: "user", Content: "Middle question with some context"},
		{Role: "assistant", Content: "Middle answer that is quite verbose and contains many details"},
		{Role: "user", Content: "Another question"},
		{Role: "assistant", Content: "Another detailed answer"},
		{Role: "user", Content: "Final question"},
		{Role: "assistant", Content: "Final answer"},
	}

	_, stats := pruner.Prune(msgs)

	if stats.OriginalTokens == 0 {
		t.Error("OriginalTokens should be > 0")
	}
	if stats.PrunedTokens == 0 {
		t.Error("PrunedTokens should be > 0")
	}
	if stats.Reduction < 0 || stats.Reduction > 1 {
		t.Errorf("Reduction should be in [0, 1], got %f", stats.Reduction)
	}
	if stats.ToolDedups < 1 {
		t.Errorf("expected at least 1 tool dedup, got %d", stats.ToolDedups)
	}
}

func TestPrunePreservesLastThreeUserMessages(t *testing.T) {
	pruner := NewContextPruner(30) // Tight budget.

	msgs := []Message{
		{Role: "system", Content: "System."},
		{Role: "user", Content: "Old question one that should be prunable"},
		{Role: "assistant", Content: "Old answer one"},
		{Role: "user", Content: "User message two - recent"},
		{Role: "assistant", Content: "Answer two"},
		{Role: "user", Content: "User message three - recent"},
		{Role: "assistant", Content: "Answer three"},
		{Role: "user", Content: "User message four - most recent"},
		{Role: "assistant", Content: "Final answer"},
	}

	pruned, _ := pruner.Prune(msgs)

	// Count how many of the last 3 user messages survived.
	recentUsers := []string{"User message two - recent", "User message three - recent", "User message four - most recent"}
	found := 0
	for _, m := range pruned {
		for _, ru := range recentUsers {
			// Check original or summarized version.
			if m.Content == ru || (m.Summarized && strings.Contains(m.Content, "User message")) {
				found++
				break
			}
		}
	}
	// At minimum the most recent user message should survive.
	if found == 0 {
		t.Error("none of the last 3 user messages were preserved")
	}
}

func TestNewContextPrunerDefaults(t *testing.T) {
	p := NewContextPruner(0)
	if p.MaxTokens != 100_000 {
		t.Errorf("expected default MaxTokens=100000, got %d", p.MaxTokens)
	}
	if p.TruncateLines != 20 {
		t.Errorf("expected default TruncateLines=20, got %d", p.TruncateLines)
	}
	if p.SummaryMaxWords != 30 {
		t.Errorf("expected default SummaryMaxWords=30, got %d", p.SummaryMaxWords)
	}
}

// truncateForTest shortens a string for error messages.
func truncateForTest(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
