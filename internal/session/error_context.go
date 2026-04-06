package session

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// DefaultMaxErrors is the default ring buffer size for ErrorContext.
const DefaultMaxErrors = 10

// DefaultMaxConsecutive is the default consecutive error threshold
// before escalation is recommended.
const DefaultMaxConsecutive = 3

// ErrorCategory classifies errors for structured context injection.
type ErrorCategory string

const (
	ErrCatBuild      ErrorCategory = "build"
	ErrCatTest       ErrorCategory = "test"
	ErrCatLint       ErrorCategory = "lint"
	ErrCatRuntime    ErrorCategory = "runtime"
	ErrCatTimeout    ErrorCategory = "timeout"
	ErrCatPermission ErrorCategory = "permission"
	ErrCatNetwork    ErrorCategory = "network"
	ErrCatUnknown    ErrorCategory = "unknown"
)

// ErrorEntry records a single error occurrence with metadata for LLM context.
type ErrorEntry struct {
	Message   string        `json:"message"`
	Category  ErrorCategory `json:"category"`
	Timestamp time.Time     `json:"timestamp"`
	Iteration int           `json:"iteration"`
}

// ErrorContext implements a ring-buffer error tracker with consecutive-error
// escalation. It is safe for concurrent access via an internal RWMutex.
// Designed for 12-Factor Agents Factor 9: compact errors into context so
// agents can self-correct.
type ErrorContext struct {
	mu               sync.RWMutex
	errors           []ErrorEntry
	maxErrors        int
	consecutiveCount int
	maxConsecutive   int
}

// NewErrorContext creates an ErrorContext with the given consecutive-error
// escalation threshold. If maxConsecutive is <= 0, DefaultMaxConsecutive is used.
func NewErrorContext(maxConsecutive int) *ErrorContext {
	if maxConsecutive <= 0 {
		maxConsecutive = DefaultMaxConsecutive
	}
	return &ErrorContext{
		errors:         make([]ErrorEntry, 0, DefaultMaxErrors),
		maxErrors:      DefaultMaxErrors,
		maxConsecutive: maxConsecutive,
	}
}

// RecordError appends an error entry to the ring buffer and increments the
// consecutive error count. When the buffer is full, the oldest entry is evicted.
func (ec *ErrorContext) RecordError(msg string, category ErrorCategory, iteration int) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	entry := ErrorEntry{
		Message:   msg,
		Category:  category,
		Timestamp: time.Now(),
		Iteration: iteration,
	}

	if len(ec.errors) >= ec.maxErrors {
		// Shift left to evict the oldest entry.
		copy(ec.errors, ec.errors[1:])
		ec.errors[len(ec.errors)-1] = entry
	} else {
		ec.errors = append(ec.errors, entry)
	}

	ec.consecutiveCount++
}

// RecordSuccess resets the consecutive error count to zero, indicating
// that the most recent operation succeeded.
func (ec *ErrorContext) RecordSuccess() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.consecutiveCount = 0
}

// ShouldEscalate returns true when the consecutive error count has reached
// or exceeded the configured maximum, indicating the agent should escalate
// (e.g., switch strategy, request human review, or abort).
func (ec *ErrorContext) ShouldEscalate() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.consecutiveCount >= ec.maxConsecutive
}

// ConsecutiveErrors returns the current count of consecutive errors
// since the last recorded success.
func (ec *ErrorContext) ConsecutiveErrors() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return ec.consecutiveCount
}

// TotalErrors returns the number of errors currently in the ring buffer.
func (ec *ErrorContext) TotalErrors() int {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	return len(ec.errors)
}

// Errors returns a copy of the current error entries.
func (ec *ErrorContext) Errors() []ErrorEntry {
	ec.mu.RLock()
	defer ec.mu.RUnlock()
	out := make([]ErrorEntry, len(ec.errors))
	copy(out, ec.errors)
	return out
}

// FormatForContext formats the recent errors as XML-like structured text
// suitable for injection into an LLM context window. The output uses
// <recent_errors> tags with count and consecutive attributes.
func (ec *ErrorContext) FormatForContext() string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	if len(ec.errors) == 0 {
		return ""
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("<recent_errors count=\"%d\" consecutive=\"%d\">\n", len(ec.errors), ec.consecutiveCount))
	for _, e := range ec.errors {
		b.WriteString(fmt.Sprintf("  <error category=\"%s\" iteration=\"%d\">\n", e.Category, e.Iteration))
		b.WriteString(fmt.Sprintf("    %s\n", e.Message))
		b.WriteString("  </error>\n")
	}
	b.WriteString("</recent_errors>")
	return b.String()
}

// Reset clears all recorded errors and resets the consecutive error count.
func (ec *ErrorContext) Reset() {
	ec.mu.Lock()
	defer ec.mu.Unlock()
	ec.errors = ec.errors[:0]
	ec.consecutiveCount = 0
}

// GetErrorContext returns the ErrorContext for the given session ID,
// creating one if it does not yet exist. This is safe for concurrent use.
func (m *Manager) GetErrorContext(sessionID string) *ErrorContext {
	m.sessionsMu.Lock()
	defer m.sessionsMu.Unlock()

	if m.errorContexts == nil {
		m.errorContexts = make(map[string]*ErrorContext)
	}
	ec, ok := m.errorContexts[sessionID]
	if !ok {
		ec = NewErrorContext(DefaultMaxConsecutive)
		m.errorContexts[sessionID] = ec
	}
	return ec
}
