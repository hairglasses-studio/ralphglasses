package session

import (
	"fmt"
	"strings"
)

// Message represents a conversation message for context pruning.
// This is a lightweight type used by ContextPruner; it maps onto the
// role+content pairs that LLM providers exchange.
type Message struct {
	Role       string `json:"role"`                  // "system", "user", "assistant", "tool"
	Content    string `json:"content"`
	ToolName   string `json:"tool_name,omitempty"`    // populated for role="tool"
	ToolArgs   string `json:"tool_args,omitempty"`    // JSON-encoded tool arguments
	Summarized bool   `json:"summarized,omitempty"`   // true if content was compressed
}

// PruneStats tracks the token reduction achieved by a prune pass.
type PruneStats struct {
	OriginalTokens int     `json:"original_tokens"`
	PrunedTokens   int     `json:"pruned_tokens"`
	Reduction      float64 `json:"reduction"` // 0.0–1.0 fraction removed
	MessagesDropped int    `json:"messages_dropped"`
	ToolDedups      int    `json:"tool_dedups"`
	Truncations     int    `json:"truncations"`
}

// ContextPruner reduces conversation context to fit within a token budget.
// It applies three strategies in order:
//  1. Dedup — collapse repeated tool calls with identical output
//  2. Truncate — shorten very long tool outputs, keeping head+tail
//  3. Summarize — compress middle messages to one-line summaries
//
// Invariant: system prompt, last 3 user messages, and last assistant message
// are always preserved unmodified.
type ContextPruner struct {
	MaxTokens       int // target token budget
	TruncateLines   int // lines to keep at head+tail of long outputs (default 20)
	SummaryMaxWords int // max words in a compressed summary (default 30)
}

// NewContextPruner creates a pruner with the given token budget.
// Sensible defaults are applied for truncation and summary limits.
func NewContextPruner(maxTokens int) *ContextPruner {
	if maxTokens <= 0 {
		maxTokens = 100_000 // ~75k words, conservative default
	}
	return &ContextPruner{
		MaxTokens:       maxTokens,
		TruncateLines:   20,
		SummaryMaxWords: 30,
	}
}

// EstimateTokens returns a fast word-based token count approximation.
// Empirically, English text averages ~1.3 tokens per whitespace-delimited word
// across GPT-4, Claude, and Gemini tokenizers.
func EstimateTokens(text string) int {
	words := len(strings.Fields(text))
	return int(float64(words) * 1.3)
}

// Prune reduces messages to fit within MaxTokens while preserving the most
// important context. Returns the pruned messages and statistics.
func (p *ContextPruner) Prune(messages []Message) ([]Message, PruneStats) {
	if len(messages) == 0 {
		return messages, PruneStats{}
	}

	originalTokens := p.totalTokens(messages)
	stats := PruneStats{OriginalTokens: originalTokens}

	// Build a working copy so we don't mutate the caller's slice.
	work := make([]Message, len(messages))
	copy(work, messages)

	// Phase 1: Dedup repeated tool calls with identical output.
	// Always run dedup — it's a free optimization even under budget.
	work, stats.ToolDedups = p.dedupToolCalls(work)

	// Fast path: already within budget after dedup.
	if p.totalTokens(work) <= p.MaxTokens {
		stats.PrunedTokens = p.totalTokens(work)
		stats.MessagesDropped = len(messages) - len(work)
		p.setReduction(&stats)
		return work, stats
	}

	// Phase 2: Truncate long tool outputs (keep first+last N lines).
	work, stats.Truncations = p.truncateLongOutputs(work)

	if p.totalTokens(work) <= p.MaxTokens {
		stats.PrunedTokens = p.totalTokens(work)
		stats.MessagesDropped = len(messages) - len(work)
		p.setReduction(&stats)
		return work, stats
	}

	// Phase 3: Summarize middle messages (preserve anchors).
	work, summarized := p.summarizeMiddle(work)
	stats.MessagesDropped += summarized

	stats.PrunedTokens = p.totalTokens(work)
	p.setReduction(&stats)
	return work, stats
}

// dedupToolCalls collapses consecutive tool messages where the same tool was
// called with the same args. Only the latest occurrence is kept.
func (p *ContextPruner) dedupToolCalls(msgs []Message) ([]Message, int) {
	if len(msgs) < 2 {
		return msgs, 0
	}

	type toolKey struct {
		name string
		args string
	}

	// Walk backward so we keep the *latest* occurrence.
	seen := make(map[toolKey]int) // key -> index of kept message
	keep := make([]bool, len(msgs))
	dedups := 0

	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if m.Role != "tool" || m.ToolName == "" {
			keep[i] = true
			continue
		}
		k := toolKey{name: m.ToolName, args: m.ToolArgs}
		if _, exists := seen[k]; exists {
			// Duplicate — drop this older one.
			dedups++
			continue
		}
		seen[k] = i
		keep[i] = true
	}

	out := make([]Message, 0, len(msgs)-dedups)
	for i, m := range msgs {
		if keep[i] {
			out = append(out, m)
		}
	}
	return out, dedups
}

// truncateLongOutputs shortens tool outputs that exceed a line threshold,
// keeping the first N and last N lines with a "[truncated]" marker.
func (p *ContextPruner) truncateLongOutputs(msgs []Message) ([]Message, int) {
	truncateLines := p.TruncateLines
	if truncateLines <= 0 {
		truncateLines = 20
	}
	threshold := truncateLines * 3 // only truncate if significantly longer

	truncations := 0
	for i := range msgs {
		if msgs[i].Role != "tool" {
			continue
		}
		lines := strings.Split(msgs[i].Content, "\n")
		if len(lines) <= threshold {
			continue
		}

		head := lines[:truncateLines]
		tail := lines[len(lines)-truncateLines:]
		omitted := len(lines) - 2*truncateLines
		msgs[i].Content = strings.Join(head, "\n") +
			fmt.Sprintf("\n\n[... %d lines truncated ...]\n\n", omitted) +
			strings.Join(tail, "\n")
		truncations++
	}
	return msgs, truncations
}

// summarizeMiddle compresses messages in the middle of the conversation.
// Protected anchors (system prompt, last 3 user messages, last assistant
// message) are never touched.
func (p *ContextPruner) summarizeMiddle(msgs []Message) ([]Message, int) {
	if len(msgs) <= 4 {
		return msgs, 0
	}

	protected := p.protectedIndices(msgs)

	summarized := 0
	for i := range msgs {
		if protected[i] {
			continue
		}
		// Already summarized from a previous pass — skip.
		if msgs[i].Summarized {
			continue
		}
		// If still over budget, compress this message.
		if p.totalTokens(msgs) <= p.MaxTokens {
			break
		}
		msgs[i].Content = p.summarizeContent(msgs[i].Content)
		msgs[i].Summarized = true
		summarized++
	}

	// If still over budget after summarization, drop non-protected messages.
	if p.totalTokens(msgs) > p.MaxTokens {
		var kept []Message
		for i, m := range msgs {
			if protected[i] || p.totalTokens(kept) < p.MaxTokens {
				kept = append(kept, m)
			} else {
				summarized++
			}
		}
		msgs = kept
	}

	return msgs, summarized
}

// protectedIndices returns a set of message indices that must not be
// summarized or dropped.
func (p *ContextPruner) protectedIndices(msgs []Message) map[int]bool {
	protected := make(map[int]bool)

	// Always protect system prompts.
	for i, m := range msgs {
		if m.Role == "system" {
			protected[i] = true
		}
	}

	// Protect last 3 user messages.
	userCount := 0
	for i := len(msgs) - 1; i >= 0 && userCount < 3; i-- {
		if msgs[i].Role == "user" {
			protected[i] = true
			userCount++
		}
	}

	// Protect last assistant message.
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "assistant" {
			protected[i] = true
			break
		}
	}

	return protected
}

// summarizeContent compresses a message body to a short one-line summary.
// It extracts the first sentence and key structural markers (decisions, code
// references) to retain the most salient information.
func (p *ContextPruner) summarizeContent(content string) string {
	maxWords := p.SummaryMaxWords
	if maxWords <= 0 {
		maxWords = 30
	}

	lines := strings.Split(strings.TrimSpace(content), "\n")
	if len(lines) == 0 {
		return content
	}

	// Collect key lines: first non-empty line + any line with decision markers.
	var keyParts []string
	decisionMarkers := []string{"decided", "chose", "selected", "will use", "implemented", "created", "fixed", "changed"}

	// Always take the first non-empty line.
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			keyParts = append(keyParts, line)
			break
		}
	}

	// Scan for decision markers in remaining lines.
	for _, line := range lines[1:] {
		lower := strings.ToLower(line)
		for _, marker := range decisionMarkers {
			if strings.Contains(lower, marker) {
				keyParts = append(keyParts, strings.TrimSpace(line))
				break
			}
		}
		if len(keyParts) >= 3 {
			break
		}
	}

	summary := strings.Join(keyParts, " | ")

	// Enforce word limit.
	words := strings.Fields(summary)
	if len(words) > maxWords {
		summary = strings.Join(words[:maxWords], " ") + "..."
	}

	return "[summary] " + summary
}

// totalTokens sums estimated tokens across all messages.
func (p *ContextPruner) totalTokens(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	return total
}

// setReduction computes the reduction fraction from stats.
func (p *ContextPruner) setReduction(stats *PruneStats) {
	if stats.OriginalTokens > 0 {
		stats.Reduction = 1.0 - float64(stats.PrunedTokens)/float64(stats.OriginalTokens)
	}
}
