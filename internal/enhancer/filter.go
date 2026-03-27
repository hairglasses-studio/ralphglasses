package enhancer

import (
	"regexp"
	"strings"
)

// conversationalPattern matches short conversational prompts that should not be enhanced.
var conversationalPattern = regexp.MustCompile(
	`(?i)^(y|n|yes|no|ok|k|sure|thanks|done|continue|go ahead|looks good|lgtm|ship it|do it|next|proceed|approve|reject|cancel|stop|undo|revert|nah)$`,
)

// alreadyStructuredPattern matches prompts that already have XML structure tags.
var alreadyStructuredPattern = regexp.MustCompile(`(?i)<(instructions|role|system|prompt)[\s>]`)

// filePathOnlyPattern matches prompts that are just a file path or glob.
var filePathOnlyPattern = regexp.MustCompile(`^[./~][\w/.*\- ]+$`)

// ShouldEnhance returns true if the prompt is worth running through the enhancement pipeline.
// Short conversational replies, already-structured prompts, and bare file paths are skipped.
// However, if ANY quality dimension grades D or F, enhancement is recommended regardless of structure.
func ShouldEnhance(prompt string, cfg Config) bool {
	trimmed := strings.TrimSpace(prompt)
	if trimmed == "" {
		return false
	}

	minWords := cfg.Hook.MinWordCount
	if minWords <= 0 {
		minWords = 5
	}

	// Word count gate
	words := strings.Fields(trimmed)
	if len(words) < minWords {
		return false
	}

	// Conversational allowlist
	if conversationalPattern.MatchString(trimmed) {
		return false
	}

	// File-path-only gate
	if filePathOnlyPattern.MatchString(trimmed) {
		return false
	}

	// Custom skip patterns from config
	for _, pat := range cfg.Hook.SkipPatterns {
		if re, err := regexp.Compile(pat); err == nil {
			if re.MatchString(trimmed) {
				return false
			}
		}
	}

	// Quality dimension check: if ANY dimension grades D or F, always enhance.
	// This prevents false negatives where structured prompts with weak dimensions are skipped.
	if hasWeakDimensions(trimmed) {
		return true
	}

	// Already-structured gate — only skip if all dimensions are healthy
	if alreadyStructuredPattern.MatchString(trimmed) {
		return false
	}

	return true
}

// hasWeakDimensions returns true if any scoring dimension grades D or F (score < 65).
func hasWeakDimensions(prompt string) bool {
	ar := Analyze(prompt)
	if ar.ScoreReport == nil {
		return false
	}
	for _, d := range ar.ScoreReport.Dimensions {
		if d.Score < 65 { // D or F
			return true
		}
	}
	return false
}
