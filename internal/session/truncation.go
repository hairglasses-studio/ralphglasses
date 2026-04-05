package session

import (
	"fmt"
	"sync"
)

const (
	// DefaultMaxOutputSize is the default maximum output size in bytes (128KB).
	DefaultMaxOutputSize = 128 * 1024

	// truncationMarkerFmt is the format string appended when output is truncated.
	truncationMarkerFmt = "...[TRUNCATED: exceeded %d bytes]"
)

// Truncator handles output truncation with configurable limits. It preserves
// valid JSON structure by closing open brackets/braces when truncating.
// Thread-safe via sync.RWMutex.
type Truncator struct {
	mu      sync.RWMutex
	maxSize int
}

// NewTruncator creates a Truncator with the default max output size.
func NewTruncator() *Truncator {
	return &Truncator{maxSize: DefaultMaxOutputSize}
}

// NewTruncatorWithSize creates a Truncator with the specified max output size.
// If maxSize <= 0, DefaultMaxOutputSize is used.
func NewTruncatorWithSize(maxSize int) *Truncator {
	if maxSize <= 0 {
		maxSize = DefaultMaxOutputSize
	}
	return &Truncator{maxSize: maxSize}
}

// SetMaxOutputSize updates the maximum output size. Thread-safe.
// If maxSize <= 0, DefaultMaxOutputSize is used.
func (t *Truncator) SetMaxOutputSize(maxSize int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if maxSize <= 0 {
		maxSize = DefaultMaxOutputSize
	}
	t.maxSize = maxSize
}

// MaxOutputSize returns the current maximum output size. Thread-safe.
func (t *Truncator) MaxOutputSize() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.maxSize
}

// TruncateResult holds the result of a truncation operation.
type TruncateResult struct {
	// Output is the (possibly truncated) output.
	Output string
	// Truncated indicates whether the output was truncated.
	Truncated bool
	// OriginalSize is the original size in bytes.
	OriginalSize int
}

// Truncate truncates the output if it exceeds the configured maximum size.
// If the output looks like JSON, it attempts to close any open brackets/braces
// to preserve valid JSON structure. A truncation marker is appended.
func (t *Truncator) Truncate(output string) TruncateResult {
	t.mu.RLock()
	maxSize := t.maxSize
	t.mu.RUnlock()

	originalSize := len(output)
	if originalSize <= maxSize {
		return TruncateResult{
			Output:       output,
			Truncated:    false,
			OriginalSize: originalSize,
		}
	}

	marker := fmt.Sprintf(truncationMarkerFmt, maxSize)
	// Reserve space for the marker and potential JSON closers (worst case ~64 chars).
	cutAt := maxSize - len(marker) - 64
	if cutAt < 0 {
		cutAt = 0
	}
	if cutAt > len(output) {
		cutAt = len(output)
	}

	truncated := output[:cutAt]

	// If the output looks like JSON, close open brackets/braces.
	if looksLikeJSON(output) {
		truncated = closeJSONStructure(truncated)
	}

	truncated += "\n" + marker

	return TruncateResult{
		Output:       truncated,
		Truncated:    true,
		OriginalSize: originalSize,
	}
}

// closeJSONStructure scans the truncated string for unmatched open brackets
// and braces, then appends the corresponding closers in reverse order.
// This is a best-effort heuristic — it does not handle strings containing
// unescaped brackets, but covers the common case of truncated JSON arrays/objects.
func closeJSONStructure(s string) string {
	var stack []byte
	inString := false
	escaped := false

	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && inString {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		switch c {
		case '{':
			stack = append(stack, '}')
		case '[':
			stack = append(stack, ']')
		case '}':
			if len(stack) > 0 && stack[len(stack)-1] == '}' {
				stack = stack[:len(stack)-1]
			}
		case ']':
			if len(stack) > 0 && stack[len(stack)-1] == ']' {
				stack = stack[:len(stack)-1]
			}
		}
	}

	// Close in reverse order.
	for i := len(stack) - 1; i >= 0; i-- {
		s += string(stack[i])
	}
	return s
}
