package session

import (
	"encoding/json"
	"regexp"
	"strings"
)

// jsonRepair attempts to fix common JSON malformations produced by LLM providers.
// It applies a series of normalization steps in order:
//  1. Strip markdown code fences (```json ... ```)
//  2. Extract JSON from prose wrapping ("Here is the JSON: {...}")
//  3. Normalize Python-style booleans (True/False -> true/false)
//  4. Normalize Python-style None (None -> null)
//  5. Strip trailing commas before } or ]
//  6. Strip single-line // comments
//
// Returns the repaired string. If the input is already valid JSON, it is
// returned unchanged. The caller should still call json.Unmarshal on the
// result -- this function does not guarantee valid JSON, only best-effort repair.
func jsonRepair(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}

	// Step 1: Strip markdown code fences.
	s = stripMarkdownFences(s)

	// Step 2: Extract JSON from prose wrapping.
	s = extractJSONFromProse(s)

	// Step 3: Normalize Python-style booleans and None.
	s = normalizePythonLiterals(s)

	// Step 4: Strip trailing commas.
	s = stripTrailingCommas(s)

	// Step 5: Strip single-line comments.
	s = stripLineComments(s)

	return strings.TrimSpace(s)
}

// tryUnmarshalWithRepair attempts json.Unmarshal on the raw input first.
// If that fails, it applies jsonRepair and retries. Returns the repaired
// string that succeeded (or the original if no repair was needed), and
// any error from the final unmarshal attempt.
func tryUnmarshalWithRepair(raw string, v any) (string, error) {
	// Fast path: try raw input first.
	if err := json.Unmarshal([]byte(raw), v); err == nil {
		return raw, nil
	}

	// Slow path: apply repairs and retry.
	repaired := jsonRepair(raw)
	if repaired == raw {
		// No changes made by repair; re-parse to get the original error.
		err := json.Unmarshal([]byte(raw), v)
		return raw, err
	}

	err := json.Unmarshal([]byte(repaired), v)
	return repaired, err
}

// markdownFenceRe matches ```json ... ``` or ``` ... ``` blocks.
// Uses (?s) so . matches newlines.
var markdownFenceRe = regexp.MustCompile("(?s)^\\s*```(?:json|JSON)?\\s*\n?(.*?)\\s*```\\s*$")

// markdownFenceInlineRe matches fenced blocks that may be embedded in other text.
var markdownFenceInlineRe = regexp.MustCompile("(?s)```(?:json|JSON)?\\s*\n?(.*?)\\s*```")

// stripMarkdownFences removes markdown code fences around JSON content.
// Handles both full-document fences and inline fences.
func stripMarkdownFences(s string) string {
	// Try full-document fence first (entire string is a fenced block).
	if m := markdownFenceRe.FindStringSubmatch(s); len(m) == 2 {
		inner := strings.TrimSpace(m[1])
		if inner != "" {
			return inner
		}
	}

	// Try inline fence: if the text contains a fenced block, extract it.
	if m := markdownFenceInlineRe.FindStringSubmatch(s); len(m) == 2 {
		inner := strings.TrimSpace(m[1])
		if inner != "" && (inner[0] == '{' || inner[0] == '[') {
			return inner
		}
	}

	return s
}

// extractJSONFromProse extracts JSON objects or arrays from text that has
// surrounding prose, e.g., "Here is the JSON:\n{...}\nHope this helps!"
// Also handles cases where JSON starts the string but has trailing text.
func extractJSONFromProse(s string) string {
	trimmed := strings.TrimSpace(s)
	if len(trimmed) == 0 {
		return s
	}

	// Find the first { or [ in the text.
	openBrace := strings.IndexByte(trimmed, '{')
	openBracket := strings.IndexByte(trimmed, '[')

	start := -1
	closeByte := byte('}')
	if openBrace >= 0 && (openBracket < 0 || openBrace < openBracket) {
		start = openBrace
		closeByte = '}'
	} else if openBracket >= 0 {
		start = openBracket
		closeByte = ']'
	}

	if start < 0 {
		return s
	}

	end := strings.LastIndexByte(trimmed, closeByte)
	if end <= start {
		return s
	}

	extracted := trimmed[start : end+1]
	// Sanity check: the extracted portion should be non-trivially long.
	if len(extracted) < 3 {
		return s
	}

	// If nothing was stripped (start == 0 and end == last char), return as-is.
	if start == 0 && end == len(trimmed)-1 {
		return trimmed
	}

	return extracted
}

// pythonBoolRe matches Python-style True/False/None that appear as JSON values.
// Uses word boundaries and looks for these outside of quoted strings by matching
// the common JSON context: after : or , or [ or at start of value position.
var (
	pythonTrueRe  = regexp.MustCompile(`(?m)([:,\[]\s*)True(\s*[,}\]])`)
	pythonFalseRe = regexp.MustCompile(`(?m)([:,\[]\s*)False(\s*[,}\]])`)
	pythonNoneRe  = regexp.MustCompile(`(?m)([:,\[]\s*)None(\s*[,}\]])`)
)

// normalizePythonLiterals replaces Python-style True, False, None with
// their JSON equivalents true, false, null. Only replaces values that
// appear in JSON value positions (after : , or [), not inside strings.
func normalizePythonLiterals(s string) string {
	// Apply each replacement up to 10 times to handle adjacent values
	// like [True, True, False] where captures overlap.
	for i := 0; i < 10; i++ {
		prev := s
		s = pythonTrueRe.ReplaceAllString(s, "${1}true${2}")
		s = pythonFalseRe.ReplaceAllString(s, "${1}false${2}")
		s = pythonNoneRe.ReplaceAllString(s, "${1}null${2}")
		if s == prev {
			break
		}
	}
	return s
}

// trailingCommaRe matches trailing commas before closing braces/brackets,
// possibly with whitespace and newlines in between.
var trailingCommaRe = regexp.MustCompile(`(?s),(\s*[}\]])`)

// stripTrailingCommas removes trailing commas before } or ] in JSON.
func stripTrailingCommas(s string) string {
	return trailingCommaRe.ReplaceAllString(s, "$1")
}

// lineCommentRe matches // comments at the end of lines, but only outside
// of string values. This is a simplified approach that handles the common
// case of comments after JSON values.
var lineCommentRe = regexp.MustCompile(`(?m)^(\s*"[^"]*"\s*:\s*(?:"[^"]*"|[^,}\]]*?))\s*//[^\n]*$`)

// stripLineComments removes single-line // comments from JSON-like text.
func stripLineComments(s string) string {
	return lineCommentRe.ReplaceAllString(s, "$1")
}

// looksLikeJSONOrFenced returns true if the string, after trimming and
// stripping markdown fences, starts with '{' or '['. This is an enhanced
// version of looksLikeJSON that also catches fenced JSON blocks.
func looksLikeJSONOrFenced(s string) bool {
	s = strings.TrimSpace(s)
	if len(s) == 0 {
		return false
	}

	// Direct JSON check.
	if s[0] == '{' || s[0] == '[' {
		return true
	}

	// Check for markdown fences containing JSON.
	if strings.HasPrefix(s, "```") {
		stripped := stripMarkdownFences(s)
		stripped = strings.TrimSpace(stripped)
		return len(stripped) > 0 && (stripped[0] == '{' || stripped[0] == '[')
	}

	return false
}
