package session

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// DefaultEventLogMaxSize is the default ring-buffer capacity for a SessionEventLog.
const DefaultEventLogMaxSize = 10_000

// SessionEventLog is an append-only, bounded event log for a session.
// When the number of events exceeds maxSize, the oldest events are dropped.
// All methods are safe for concurrent use.
type SessionEventLog struct {
	events  []SessionEvent
	maxSize int
	mu      sync.RWMutex
}

// NewSessionEventLog creates a new event log with the given maximum size.
// If maxSize <= 0, DefaultEventLogMaxSize is used.
func NewSessionEventLog(maxSize int) *SessionEventLog {
	if maxSize <= 0 {
		maxSize = DefaultEventLogMaxSize
	}
	return &SessionEventLog{
		events:  make([]SessionEvent, 0, min(maxSize, 256)),
		maxSize: maxSize,
	}
}

// Append adds an event to the log. If the event has no ID, one is generated.
// If the event has a zero Timestamp, time.Now() is used.
// When the log exceeds maxSize, the oldest events are dropped.
func (l *SessionEventLog) Append(event SessionEvent) {
	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.events = append(l.events, event)
	if len(l.events) > l.maxSize {
		// Drop oldest events to stay within bounds.
		excess := len(l.events) - l.maxSize
		// Copy to release references held by the old backing array.
		trimmed := make([]SessionEvent, l.maxSize)
		copy(trimmed, l.events[excess:])
		l.events = trimmed
	}
}

// Events returns a copy of all events in the log.
func (l *SessionEventLog) Events() []SessionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	out := make([]SessionEvent, len(l.events))
	copy(out, l.events)
	return out
}

// EventsSince returns events with a timestamp at or after t.
func (l *SessionEventLog) EventsSince(t time.Time) []SessionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var out []SessionEvent
	for _, e := range l.events {
		if !e.Timestamp.Before(t) {
			out = append(out, e)
		}
	}
	return out
}

// EventsByType returns events matching the given type.
func (l *SessionEventLog) EventsByType(typ SessionEventType) []SessionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	var out []SessionEvent
	for _, e := range l.events {
		if e.Type == typ {
			out = append(out, e)
		}
	}
	return out
}

// Last returns the last n events. If n exceeds the log length, all events
// are returned. The returned slice is a copy.
func (l *SessionEventLog) Last(n int) []SessionEvent {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if n <= 0 {
		return nil
	}
	total := len(l.events)
	if n > total {
		n = total
	}
	out := make([]SessionEvent, n)
	copy(out, l.events[total-n:])
	return out
}

// Len returns the number of events in the log.
func (l *SessionEventLog) Len() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.events)
}

// FormatForContext formats the last n events as XML suitable for injection
// into an LLM context window. Each event is rendered as a <event> element
// with type, timestamp, iteration, and data attributes.
func (l *SessionEventLog) FormatForContext(lastN int) string {
	events := l.Last(lastN)
	if len(events) == 0 {
		return "<session-events />"
	}

	var b strings.Builder
	b.WriteString("<session-events>\n")
	for _, e := range events {
		b.WriteString(fmt.Sprintf("  <event type=%q time=%q iteration=%d",
			string(e.Type), e.Timestamp.UTC().Format(time.RFC3339), e.Iteration))
		if len(e.Data) > 0 {
			b.WriteString(">\n")
			for k, v := range e.Data {
				b.WriteString(fmt.Sprintf("    <%s>%v</%s>\n", k, v, k))
			}
			b.WriteString("  </event>\n")
		} else {
			b.WriteString(" />\n")
		}
	}
	b.WriteString("</session-events>")
	return b.String()
}

// Fork creates a new SessionEventLog containing events from index 0 up to
// (but not including) fromIndex. The new log is independent: mutations to
// either log do not affect the other. The new log inherits the same maxSize.
func (l *SessionEventLog) Fork(fromIndex int) *SessionEventLog {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if fromIndex < 0 {
		fromIndex = 0
	}
	if fromIndex > len(l.events) {
		fromIndex = len(l.events)
	}

	forked := &SessionEventLog{
		events:  make([]SessionEvent, fromIndex),
		maxSize: l.maxSize,
	}
	copy(forked.events, l.events[:fromIndex])
	return forked
}
