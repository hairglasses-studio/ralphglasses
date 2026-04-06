package session

import (
	"strings"
	"testing"
	"time"
)

func TestCompactErrors_Empty(t *testing.T) {
	t.Parallel()
	result := CompactErrors(nil, 0)
	if result != "" {
		t.Errorf("expected empty string for nil errors, got %q", result)
	}
}

func TestCompactErrors_SingleError(t *testing.T) {
	t.Parallel()
	errs := []LoopError{
		{Iteration: 1, Phase: "execute", Message: "timeout waiting for response", Retryable: true},
	}
	result := CompactErrors(errs, MaxCompactedTokens)
	if !strings.Contains(result, "execute phase") {
		t.Error("expected execute phase header")
	}
	if !strings.Contains(result, "timeout waiting for response") {
		t.Error("expected error message in output")
	}
	if !strings.Contains(result, "[retryable]") {
		t.Error("expected retryable tag")
	}
}

func TestCompactErrors_Deduplication(t *testing.T) {
	t.Parallel()
	errs := []LoopError{
		{Iteration: 1, Phase: "plan", Message: "JSON parse error", Retryable: true},
		{Iteration: 2, Phase: "plan", Message: "JSON parse error", Retryable: true},
		{Iteration: 3, Phase: "plan", Message: "JSON parse error", Retryable: true},
	}
	result := CompactErrors(errs, MaxCompactedTokens)
	if !strings.Contains(result, "(x3)") {
		t.Errorf("expected deduplicated count (x3), got:\n%s", result)
	}
	// Should only appear once as a line item.
	count := strings.Count(result, "JSON parse error")
	if count != 1 {
		t.Errorf("expected 1 occurrence of error message, got %d", count)
	}
}

func TestCompactErrors_MultiPhase(t *testing.T) {
	t.Parallel()
	errs := []LoopError{
		{Iteration: 1, Phase: "plan", Message: "model refused", Retryable: false},
		{Iteration: 1, Phase: "execute", Message: "test failed", Retryable: true},
		{Iteration: 2, Phase: "evaluate", Message: "coverage dropped", Retryable: false},
	}
	result := CompactErrors(errs, MaxCompactedTokens)
	if !strings.Contains(result, "plan phase") {
		t.Error("expected plan phase")
	}
	if !strings.Contains(result, "execute phase") {
		t.Error("expected execute phase")
	}
	if !strings.Contains(result, "evaluate phase") {
		t.Error("expected evaluate phase")
	}
}

func TestCompactErrors_RetryableSummary(t *testing.T) {
	t.Parallel()
	errs := []LoopError{
		{Iteration: 1, Phase: "execute", Message: "timeout", Retryable: true},
		{Iteration: 2, Phase: "execute", Message: "crash", Retryable: false},
	}
	result := CompactErrors(errs, MaxCompactedTokens)
	if !strings.Contains(result, "1 of 2 errors are retryable") {
		t.Errorf("expected retryable summary, got:\n%s", result)
	}
}

func TestCompactErrors_TokenBudget(t *testing.T) {
	t.Parallel()
	// Generate many errors to test truncation.
	var errs []LoopError
	for i := 0; i < 200; i++ {
		errs = append(errs, LoopError{
			Iteration: i,
			Phase:     "execute",
			Message:   strings.Repeat("word ", 20), // ~20 words per error
			Timestamp: time.Now(),
			Retryable: true,
		})
	}
	result := CompactErrors(errs, 100) // very tight budget
	words := len(strings.Fields(result))
	tokens := int(float64(words) * 1.3)
	// Should be roughly bounded. Allow some overhead for headers.
	if tokens > 300 {
		t.Errorf("expected compact output under ~300 tokens, got %d tokens (%d words)", tokens, words)
	}
}

func TestCompactErrors_LessonsHeader(t *testing.T) {
	t.Parallel()
	errs := []LoopError{
		{Iteration: 1, Phase: "plan", Message: "parse error"},
	}
	result := CompactErrors(errs, MaxCompactedTokens)
	if !strings.HasPrefix(result, "## Lessons from previous attempts") {
		t.Error("expected lessons header")
	}
}
