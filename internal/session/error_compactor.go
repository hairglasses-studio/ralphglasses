package session

import (
	"fmt"
	"strings"
	"time"
)

// LoopError records a structured error from a loop iteration.
type LoopError struct {
	Iteration int       `json:"iteration"`
	Phase     string    `json:"phase"` // "plan", "execute", "evaluate", "improve"
	ErrorCode string    `json:"error_code,omitempty"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
	Retryable bool      `json:"retryable"`
}

// MaxCompactedTokens is the default upper bound for compacted error context
// injected into the next iteration. Approximately 2000 tokens ~ 1500 words.
const MaxCompactedTokens = 2000

// CompactErrors produces a structured markdown summary of errors suitable
// for injection into an LLM context window. Errors are grouped by phase
// and deduplicated. The output is bounded by maxTokens (estimated).
func CompactErrors(errors []LoopError, maxTokens int) string {
	if len(errors) == 0 {
		return ""
	}
	if maxTokens <= 0 {
		maxTokens = MaxCompactedTokens
	}

	// Group by phase.
	grouped := make(map[string][]LoopError)
	for _, e := range errors {
		phase := e.Phase
		if phase == "" {
			phase = "unknown"
		}
		grouped[phase] = append(grouped[phase], e)
	}

	var b strings.Builder
	b.WriteString("## Lessons from previous attempts\n\n")

	// Deduplicate by message within each phase.
	for phase, errs := range grouped {
		deduped := deduplicateErrors(errs)
		b.WriteString(fmt.Sprintf("### %s phase (%d error(s))\n", phase, len(errs)))
		for _, de := range deduped {
			retryTag := ""
			if de.retryable {
				retryTag = " [retryable]"
			}
			if de.count > 1 {
				b.WriteString(fmt.Sprintf("- (x%d) %s%s\n", de.count, de.message, retryTag))
			} else {
				b.WriteString(fmt.Sprintf("- %s%s\n", de.message, retryTag))
			}
		}
		b.WriteString("\n")

		// Rough token estimate: 1.3 tokens per word.
		words := len(strings.Fields(b.String()))
		if int(float64(words)*1.3) > maxTokens {
			b.WriteString("_(error context truncated to fit budget)_\n")
			break
		}
	}

	// Final suggestion.
	retryableCount := 0
	for _, e := range errors {
		if e.Retryable {
			retryableCount++
		}
	}
	if retryableCount > 0 {
		b.WriteString(fmt.Sprintf("**%d of %d errors are retryable.** Adjust your approach based on the patterns above.\n",
			retryableCount, len(errors)))
	}

	return b.String()
}

type dedupedError struct {
	message   string
	count     int
	retryable bool
}

func deduplicateErrors(errs []LoopError) []dedupedError {
	seen := make(map[string]*dedupedError)
	var order []string

	for _, e := range errs {
		key := e.Message
		if de, ok := seen[key]; ok {
			de.count++
		} else {
			seen[key] = &dedupedError{message: e.Message, count: 1, retryable: e.Retryable}
			order = append(order, key)
		}
	}

	result := make([]dedupedError, 0, len(order))
	for _, key := range order {
		result = append(result, *seen[key])
	}
	return result
}
