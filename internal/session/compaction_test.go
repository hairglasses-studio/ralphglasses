package session

import (
	"strings"
	"testing"
)

func TestDefaultCompactionConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultCompactionConfig()
	if cfg.Strategy != StrategySliding {
		t.Fatalf("expected StrategySliding, got %s", cfg.Strategy)
	}
	if cfg.MaxTokens != 100_000 {
		t.Fatalf("expected MaxTokens=100000, got %d", cfg.MaxTokens)
	}
	if cfg.KeepRecentTurns != 5 {
		t.Fatalf("expected KeepRecentTurns=5, got %d", cfg.KeepRecentTurns)
	}
}

func TestNewContextCompactor_Defaults(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{})
	if cc.config.MaxTokens != 100_000 {
		t.Fatalf("expected default MaxTokens=100000, got %d", cc.config.MaxTokens)
	}
	if cc.config.KeepRecentTurns != 5 {
		t.Fatalf("expected default KeepRecentTurns=5, got %d", cc.config.KeepRecentTurns)
	}
	if cc.config.SummaryMaxWords != 40 {
		t.Fatalf("expected default SummaryMaxWords=40, got %d", cc.config.SummaryMaxWords)
	}
}

func TestCompact_WithinBudget(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{MaxTokens: 1_000_000})
	msgs := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	result := cc.Compact(msgs)
	if len(result.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(result.Messages))
	}
	if result.Reduction != 0 {
		t.Fatalf("expected no reduction, got %f", result.Reduction)
	}
}

func TestCompact_Summarize(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{
		Strategy:             StrategySummarize,
		MaxTokens:            1,
		KeepRecentTurns:      1,
		SummaryMaxWords:      5,
		PreserveSystemPrompt: true,
	})
	msgs := buildTestMessages(10)
	result := cc.Compact(msgs)
	if result.TurnsSummarized == 0 {
		t.Fatal("expected some turns to be summarized")
	}
	if result.Reduction <= 0 {
		t.Fatal("expected positive reduction")
	}
}

func TestCompact_DropToolOutputs(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{
		Strategy:             StrategyDropToolOutputs,
		MaxTokens:            1,
		KeepRecentTurns:      1,
		PreserveSystemPrompt: true,
	})
	msgs := []Message{
		{Role: "system", Content: "system prompt"},
		{Role: "user", Content: "do something"},
		{Role: "tool", Content: "long tool output here...", ToolName: "grep"},
		{Role: "assistant", Content: "done"},
		{Role: "user", Content: "recent question"},
		{Role: "assistant", Content: "recent answer"},
	}
	result := cc.Compact(msgs)
	if result.ToolOutputsDropped == 0 {
		t.Fatal("expected tool outputs to be dropped")
	}
	// System prompt should be preserved.
	if result.Messages[0].Content != "system prompt" {
		t.Fatalf("expected system prompt preserved, got %q", result.Messages[0].Content)
	}
}

func TestCompact_KeepRecent(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{
		Strategy:             StrategyKeepRecent,
		MaxTokens:            1,
		KeepRecentTurns:      1,
		PreserveSystemPrompt: true,
	})
	msgs := buildTestMessages(10)
	result := cc.Compact(msgs)
	if result.TurnsRemoved == 0 {
		t.Fatal("expected some turns removed")
	}
}

func TestCompact_Sliding(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{
		Strategy:             StrategySliding,
		MaxTokens:            1,
		KeepRecentTurns:      1,
		SummaryMaxWords:      5,
		PreserveSystemPrompt: true,
	})
	msgs := []Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "old question with lots of text"},
		{Role: "tool", Content: "tool output", ToolName: "run"},
		{Role: "assistant", Content: "old answer"},
		{Role: "user", Content: "recent"},
		{Role: "assistant", Content: "recent reply"},
	}
	result := cc.Compact(msgs)
	if result.Strategy != StrategySliding {
		t.Fatalf("expected StrategySliding, got %s", result.Strategy)
	}
}

func TestCompact_UnknownStrategy(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{
		Strategy:        "unknown",
		MaxTokens:       1,
		KeepRecentTurns: 1,
		SummaryMaxWords: 5,
	})
	msgs := buildTestMessages(6)
	result := cc.Compact(msgs)
	// Should fallback to sliding.
	if len(result.Messages) == 0 {
		t.Fatal("expected non-empty result from fallback strategy")
	}
}

func TestCompactionHistory(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{MaxTokens: 1_000_000})
	msgs := []Message{{Role: "user", Content: "hello"}}
	cc.Compact(msgs)
	cc.Compact(msgs)

	hist := cc.CompactionHistory()
	if len(hist) != 2 {
		t.Fatalf("expected 2 history entries, got %d", len(hist))
	}
}

func TestNeedsCompaction(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(CompactionConfig{MaxTokens: 1})
	msgs := []Message{{Role: "user", Content: "this is a test message that should exceed 1 token"}}
	if !cc.NeedsCompaction(msgs) {
		t.Fatal("expected NeedsCompaction=true")
	}

	cc2 := NewContextCompactor(CompactionConfig{MaxTokens: 1_000_000})
	if cc2.NeedsCompaction(msgs) {
		t.Fatal("expected NeedsCompaction=false")
	}
}

func TestTokenEstimate(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(DefaultCompactionConfig())
	msgs := []Message{{Role: "user", Content: "hello world"}}
	est := cc.TokenEstimate(msgs)
	if est <= 0 {
		t.Fatalf("expected positive token estimate, got %d", est)
	}
}

func TestSetMaxTokens(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(DefaultCompactionConfig())
	cc.SetMaxTokens(50_000)
	if cc.config.MaxTokens != 50_000 {
		t.Fatalf("expected 50000, got %d", cc.config.MaxTokens)
	}
	// Zero should not change.
	cc.SetMaxTokens(0)
	if cc.config.MaxTokens != 50_000 {
		t.Fatalf("expected 50000 unchanged, got %d", cc.config.MaxTokens)
	}
}

func TestNeedsCompactionForModel(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(DefaultCompactionConfig())
	msgs := []Message{
		{Role: "user", Content: strings.Repeat("word ", 1000)}, // ~1300 tokens
	}

	// Under 80% of 200k — no compaction needed.
	if cc.NeedsCompactionForModel(msgs, 200_000, 0.80) {
		t.Fatal("expected no compaction needed when well under model limit")
	}

	// Over 80% of a small limit — compaction needed.
	if !cc.NeedsCompactionForModel(msgs, 1500, 0.80) {
		t.Fatal("expected compaction needed when estimated tokens > 80% of 1500")
	}

	// Zero/negative model limit — always false.
	if cc.NeedsCompactionForModel(msgs, 0, 0.80) {
		t.Fatal("expected false for zero model limit")
	}
	if cc.NeedsCompactionForModel(msgs, -1, 0.80) {
		t.Fatal("expected false for negative model limit")
	}
}

func TestSetStrategy(t *testing.T) {
	t.Parallel()
	cc := NewContextCompactor(DefaultCompactionConfig())
	cc.SetStrategy(StrategySummarize)
	if cc.config.Strategy != StrategySummarize {
		t.Fatalf("expected StrategySummarize, got %s", cc.config.Strategy)
	}
}

func TestCompactSummary(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		content string
		max     int
		check   func(string) bool
	}{
		{"empty", "", 10, func(s string) bool { return s == "[empty]" }},
		{"already summary", "[summary] already done", 10, func(s string) bool { return s == "[summary] already done" }},
		{"short text", "hello", 10, func(s string) bool { return strings.HasPrefix(s, "[summary]") }},
		{"long text truncated", strings.Repeat("word ", 100), 5, func(s string) bool { return strings.HasSuffix(s, "...") }},
		{
			"decision markers",
			"First line.\nSecond line decided to do X.\nThird line chose Y.",
			40,
			func(s string) bool { return strings.Contains(s, "decided") },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compactSummary(tt.content, tt.max)
			if !tt.check(result) {
				t.Fatalf("check failed for %q, got %q", tt.name, result)
			}
		})
	}
}

func TestCompactSummary_ZeroMaxWords(t *testing.T) {
	t.Parallel()
	result := compactSummary("hello world", 0)
	if !strings.HasPrefix(result, "[summary]") {
		t.Fatalf("expected [summary] prefix, got %q", result)
	}
}

func TestEstimateMessagesTokens(t *testing.T) {
	t.Parallel()
	msgs := []Message{
		{Role: "user", Content: "hello world", ToolName: "grep", ToolArgs: `{"pattern":"foo"}`},
	}
	est := estimateMessagesTokens(msgs)
	if est <= 0 {
		t.Fatalf("expected positive estimate, got %d", est)
	}
}

func TestCopyMessages_Nil(t *testing.T) {
	t.Parallel()
	if copyMessages(nil) != nil {
		t.Fatal("expected nil for nil input")
	}
}

func TestEstimateTokensForText(t *testing.T) {
	t.Parallel()
	if est := EstimateTokensForText("hello world"); est <= 0 {
		t.Fatalf("expected positive, got %d", est)
	}
}

// buildTestMessages creates a conversation with N user/assistant turn pairs.
func buildTestMessages(n int) []Message {
	msgs := []Message{{Role: "system", Content: "you are a helper"}}
	for range n {
		msgs = append(msgs,
			Message{Role: "user", Content: strings.Repeat("question ", 20)},
			Message{Role: "assistant", Content: strings.Repeat("answer ", 20)},
		)
	}
	return msgs
}
