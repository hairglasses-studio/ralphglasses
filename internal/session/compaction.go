package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// CompactionStrategy identifies which compaction approach to use.
type CompactionStrategy string

const (
	// StrategySummarize replaces old turns with compressed summaries.
	StrategySummarize CompactionStrategy = "summarize"
	// StrategyLLMSummarize uses a separate LLM pass to generate a high-fidelity summary.
	StrategyLLMSummarize CompactionStrategy = "llm_summarize"
	// StrategyDropToolOutputs removes tool result content from old turns.
	StrategyDropToolOutputs CompactionStrategy = "drop_tool_outputs"
	// StrategyKeepRecent keeps only the most recent N turns.
	StrategyKeepRecent CompactionStrategy = "keep_recent"
	// StrategySliding applies a sliding window: summarize beyond window, keep recent.
	StrategySliding CompactionStrategy = "sliding"
)

// CompactionConfig configures the ContextCompactor.
type CompactionConfig struct {
	// Strategy selects which compaction approach to use.
	Strategy CompactionStrategy `json:"strategy"`
	// MaxTokens is the target token budget for the compacted context.
	MaxTokens int `json:"max_tokens"`
	// KeepRecentTurns is how many recent user/assistant turn pairs to preserve verbatim.
	KeepRecentTurns int `json:"keep_recent_turns"`
	// SummaryMaxWords caps summary length per compacted turn.
	SummaryMaxWords int `json:"summary_max_words"`
	// PreserveSystemPrompt keeps system messages untouched.
	PreserveSystemPrompt bool `json:"preserve_system_prompt"`
}

// DefaultCompactionConfig returns sensible defaults for compaction.
func DefaultCompactionConfig() CompactionConfig {
	return CompactionConfig{
		Strategy:             StrategySliding,
		MaxTokens:            100_000,
		KeepRecentTurns:      5,
		SummaryMaxWords:      40,
		PreserveSystemPrompt: true,
	}
}

// CompactionResult holds the output of a compaction operation.
type CompactionResult struct {
	Messages       []Message          `json:"messages"`
	OriginalTokens int                `json:"original_tokens"`
	CompactedTokens int               `json:"compacted_tokens"`
	Reduction      float64            `json:"reduction"` // 0.0-1.0 fraction removed
	Strategy       CompactionStrategy `json:"strategy"`
	TurnsRemoved   int                `json:"turns_removed"`
	TurnsSummarized int               `json:"turns_summarized"`
	ToolOutputsDropped int            `json:"tool_outputs_dropped"`
	Timestamp      time.Time          `json:"timestamp"`
}

// ContextCompactor compresses conversation history to fit within token limits.
// It orchestrates multiple strategies and tracks compaction history for
// monitoring and debugging.
type ContextCompactor struct {
	mu      sync.Mutex
	config  CompactionConfig
	history []CompactionResult
}

// NewContextCompactor creates a compactor with the given config.
func NewContextCompactor(cfg CompactionConfig) *ContextCompactor {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 100_000
	}
	if cfg.KeepRecentTurns <= 0 {
		cfg.KeepRecentTurns = 5
	}
	if cfg.SummaryMaxWords <= 0 {
		cfg.SummaryMaxWords = 40
	}
	return &ContextCompactor{
		config:  cfg,
		history: make([]CompactionResult, 0, 16),
	}
}

// Compact applies the configured compaction strategy to the messages.
// System messages are always preserved when PreserveSystemPrompt is true.
func (cc *ContextCompactor) Compact(messages []Message) CompactionResult {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	originalTokens := estimateMessagesTokens(messages)
	result := CompactionResult{
		OriginalTokens: originalTokens,
		Strategy:       cc.config.Strategy,
		Timestamp:      time.Now(),
	}

	// Fast path: already within budget.
	if originalTokens <= cc.config.MaxTokens {
		result.Messages = copyMessages(messages)
		result.CompactedTokens = originalTokens
		cc.history = append(cc.history, result)
		return result
	}

	var compacted []Message
	switch cc.config.Strategy {
	case StrategySummarize:
		compacted, result.TurnsSummarized = cc.applySummarize(messages)
	case StrategyDropToolOutputs:
		compacted, result.ToolOutputsDropped = cc.applyDropToolOutputs(messages)
	case StrategyKeepRecent:
		compacted, result.TurnsRemoved = cc.applyKeepRecent(messages)
	case StrategySliding:
		compacted, result.TurnsSummarized, result.ToolOutputsDropped, result.TurnsRemoved = cc.applySliding(messages)
	default:
		// Fallback to sliding window.
		compacted, result.TurnsSummarized, result.ToolOutputsDropped, result.TurnsRemoved = cc.applySliding(messages)
	}

	result.Messages = compacted
	result.CompactedTokens = estimateMessagesTokens(compacted)
	if originalTokens > 0 {
		result.Reduction = 1.0 - float64(result.CompactedTokens)/float64(originalTokens)
	}

	cc.history = append(cc.history, result)
	return result
}

// CompactionHistory returns the history of compaction operations.
func (cc *ContextCompactor) CompactionHistory() []CompactionResult {
	cc.mu.Lock()
	defer cc.mu.Unlock()

	out := make([]CompactionResult, len(cc.history))
	copy(out, cc.history)
	return out
}

// NeedsCompaction returns true if messages exceed the token budget.
func (cc *ContextCompactor) NeedsCompaction(messages []Message) bool {
	return estimateMessagesTokens(messages) > cc.config.MaxTokens
}

// NeedsCompactionForModel returns true if the estimated token count exceeds
// the given percentage of the model's context limit. This enables token-based
// compaction triggers that fire before hitting the hard limit.
// Pattern 10: Token-Based Compaction Trigger (from whiteclaw/Claude Code analysis).
func (cc *ContextCompactor) NeedsCompactionForModel(messages []Message, modelContextLimit int, thresholdPct float64) bool {
	if modelContextLimit <= 0 || thresholdPct <= 0 {
		return false
	}
	estimated := estimateMessagesTokens(messages)
	threshold := int(float64(modelContextLimit) * thresholdPct)
	return estimated > threshold
}

// TokenEstimate returns the estimated token count for messages.
func (cc *ContextCompactor) TokenEstimate(messages []Message) int {
	return estimateMessagesTokens(messages)
}

// SetMaxTokens updates the token budget.
func (cc *ContextCompactor) SetMaxTokens(max int) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	if max > 0 {
		cc.config.MaxTokens = max
	}
}

// SetStrategy updates the compaction strategy.
func (cc *ContextCompactor) SetStrategy(s CompactionStrategy) {
	cc.mu.Lock()
	defer cc.mu.Unlock()
	cc.config.Strategy = s
}

// applySummarize compresses old turns into summaries while keeping recent turns.
func (cc *ContextCompactor) applySummarize(messages []Message) ([]Message, int) {
	recent, old := cc.splitRecentOld(messages)
	summarized := 0

	var compacted []Message
	for _, m := range old {
		if m.Role == "system" && cc.config.PreserveSystemPrompt {
			compacted = append(compacted, m)
			continue
		}
		if m.Summarized {
			compacted = append(compacted, m)
			continue
		}
		compacted = append(compacted, Message{
			Role:       m.Role,
			Content:    compactSummary(m.Content, cc.config.SummaryMaxWords),
			ToolName:   m.ToolName,
			Summarized: true,
		})
		summarized++
	}
	compacted = append(compacted, recent...)

	// If still over budget, progressively drop oldest non-system summaries.
	for estimateMessagesTokens(compacted) > cc.config.MaxTokens && len(compacted) > len(recent)+1 {
		// Find first non-system, non-recent message to drop.
		dropped := false
		for i := 0; i < len(compacted)-len(recent); i++ {
			if compacted[i].Role != "system" || !cc.config.PreserveSystemPrompt {
				compacted = append(compacted[:i], compacted[i+1:]...)
				dropped = true
				break
			}
		}
		if !dropped {
			break
		}
	}

	return compacted, summarized
}

// applyDropToolOutputs removes tool output content from old turns.
func (cc *ContextCompactor) applyDropToolOutputs(messages []Message) ([]Message, int) {
	recent, old := cc.splitRecentOld(messages)
	dropped := 0

	var compacted []Message
	for _, m := range old {
		if m.Role == "system" && cc.config.PreserveSystemPrompt {
			compacted = append(compacted, m)
			continue
		}
		if m.Role == "tool" {
			compacted = append(compacted, Message{
				Role:     m.Role,
				Content:  fmt.Sprintf("[tool output removed: %s]", m.ToolName),
				ToolName: m.ToolName,
				ToolArgs: m.ToolArgs,
			})
			dropped++
			continue
		}
		compacted = append(compacted, m)
	}
	compacted = append(compacted, recent...)
	return compacted, dropped
}

// applyKeepRecent drops all messages except system prompts and recent turns.
func (cc *ContextCompactor) applyKeepRecent(messages []Message) ([]Message, int) {
	recent, old := cc.splitRecentOld(messages)
	removed := 0

	var compacted []Message

	// Keep system messages from old section.
	for _, m := range old {
		if m.Role == "system" && cc.config.PreserveSystemPrompt {
			compacted = append(compacted, m)
		} else {
			removed++
		}
	}

	// Add a context marker so the model knows history was dropped.
	if removed > 0 {
		compacted = append(compacted, Message{
			Role:       "system",
			Content:    fmt.Sprintf("[%d earlier messages removed for context compaction]", removed),
			Summarized: true,
		})
	}

	compacted = append(compacted, recent...)
	return compacted, removed
}

// applySliding combines all three strategies: drop tool outputs first,
// then summarize, then drop oldest turns if still over budget.
func (cc *ContextCompactor) applySliding(messages []Message) ([]Message, int, int, int) {
	recent, old := cc.splitRecentOld(messages)
	summarized, toolsDropped, turnsRemoved := 0, 0, 0

	var compacted []Message

	// Phase 1: Process old messages — drop tool outputs, summarize the rest.
	for _, m := range old {
		if m.Role == "system" && cc.config.PreserveSystemPrompt {
			compacted = append(compacted, m)
			continue
		}
		if m.Role == "tool" {
			compacted = append(compacted, Message{
				Role:     m.Role,
				Content:  fmt.Sprintf("[tool output removed: %s]", m.ToolName),
				ToolName: m.ToolName,
				ToolArgs: m.ToolArgs,
			})
			toolsDropped++
			continue
		}
		if m.Summarized {
			compacted = append(compacted, m)
			continue
		}
		compacted = append(compacted, Message{
			Role:       m.Role,
			Content:    compactSummary(m.Content, cc.config.SummaryMaxWords),
			ToolName:   m.ToolName,
			Summarized: true,
		})
		summarized++
	}

	compacted = append(compacted, recent...)

	// Phase 2: If still over budget, drop oldest non-system messages.
	for estimateMessagesTokens(compacted) > cc.config.MaxTokens && len(compacted) > len(recent)+1 {
		dropped := false
		for i := 0; i < len(compacted)-len(recent); i++ {
			if compacted[i].Role == "system" && cc.config.PreserveSystemPrompt {
				continue
			}
			compacted = append(compacted[:i], compacted[i+1:]...)
			turnsRemoved++
			dropped = true
			break
		}
		if !dropped {
			break
		}
	}

	return compacted, summarized, toolsDropped, turnsRemoved
}

// splitRecentOld divides messages into recent (kept verbatim) and old (compactable).
// "Recent" means the last N turn-pairs, counted as user+assistant message groups.
func (cc *ContextCompactor) splitRecentOld(messages []Message) (recent, old []Message) {
	if len(messages) == 0 {
		return nil, nil
	}

	// Count turns from the end. A turn is a user message (optionally followed
	// by assistant/tool messages until the next user message).
	turns := 0
	splitIdx := len(messages)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" {
			turns++
			if turns >= cc.config.KeepRecentTurns {
				splitIdx = i
				break
			}
		}
	}

	// If we didn't find enough turns, keep everything.
	if turns < cc.config.KeepRecentTurns {
		return copyMessages(messages), nil
	}

	return copyMessages(messages[splitIdx:]), copyMessages(messages[:splitIdx])
}

// compactSummary creates a compact summary of content, limited to maxWords.
func compactSummary(content string, maxWords int) string {
	if maxWords <= 0 {
		maxWords = 40
	}

	content = strings.TrimSpace(content)
	if content == "" {
		return "[empty]"
	}

	// Already a summary.
	if strings.HasPrefix(content, "[summary]") {
		return content
	}

	lines := strings.Split(content, "\n")

	// Gather key lines: first non-empty + lines with decision markers.
	var keyParts []string
	markers := []string{"decided", "chose", "selected", "will", "implemented",
		"created", "fixed", "changed", "error", "failed", "success"}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(keyParts) == 0 {
			keyParts = append(keyParts, line)
			continue
		}
		lower := strings.ToLower(line)
		for _, marker := range markers {
			if strings.Contains(lower, marker) {
				keyParts = append(keyParts, line)
				break
			}
		}
		if len(keyParts) >= 3 {
			break
		}
	}

	if len(keyParts) == 0 {
		// Fall back to first N words.
		words := strings.Fields(content)
		if len(words) > maxWords {
			words = words[:maxWords]
		}
		return "[summary] " + strings.Join(words, " ") + "..."
	}

	summary := strings.Join(keyParts, " | ")
	words := strings.Fields(summary)
	if len(words) > maxWords {
		summary = strings.Join(words[:maxWords], " ") + "..."
	}
	return "[summary] " + summary
}

// estimateMessagesTokens sums heuristic token estimates across messages.
// Uses the shared EstimateTokens function (words * 1.3).
func estimateMessagesTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += EstimateTokens(m.Content)
		if m.ToolName != "" {
			total += EstimateTokens(m.ToolName)
		}
		if m.ToolArgs != "" {
			total += EstimateTokens(m.ToolArgs)
		}
	}
	return total
}

// copyMessages returns a shallow copy of a message slice.
func copyMessages(msgs []Message) []Message {
	if msgs == nil {
		return nil
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	return out
}

// EstimateTokensForText is a convenience wrapper around EstimateTokens
// for external callers that want to estimate arbitrary text.
func EstimateTokensForText(text string) int {
	return EstimateTokens(text)
}
