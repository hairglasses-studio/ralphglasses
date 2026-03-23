package events

import (
	"sync"
	"time"
)

// EventType categorizes events in the system.
type EventType string

const (
	// Session lifecycle
	SessionStarted EventType = "session.started"
	SessionEnded   EventType = "session.ended"
	SessionStopped EventType = "session.stopped"

	// Cost
	CostUpdate     EventType = "cost.update"
	BudgetExceeded EventType = "budget.exceeded"

	// Loop lifecycle
	LoopStarted  EventType = "loop.started"
	LoopStopped  EventType = "loop.stopped"
	LoopIterated EventType = "loop.iterated"

	// Loop benchmarking
	LoopRegression EventType = "loop.regression"

	// Team
	TeamCreated EventType = "team.created"

	// Journal
	JournalWritten EventType = "journal.written"

	// Config and scan
	ConfigChanged EventType = "config.changed"
	ScanComplete  EventType = "scan.complete"

	// Prompt enhancement
	PromptEnhanced EventType = "prompt.enhanced"

	// Tool instrumentation
	ToolCalled EventType = "tool.called"

	// Self-improvement
	AutoOptimized    EventType = "auto.optimized"     // Level 2+ decision executed
	ProviderSelected EventType = "provider.selected"   // Smart provider selection
	SessionRecovered EventType = "session.recovered"   // Auto-recovery restart
	ContextConflict  EventType = "context.conflict"    // Cross-session file conflict
)

// Event represents something that happened in the system.
type Event struct {
	Type      EventType      `json:"type"`
	Timestamp time.Time      `json:"timestamp"`
	NodeID    string         `json:"node_id,omitempty"`
	RepoName  string         `json:"repo_name,omitempty"`
	RepoPath  string         `json:"repo_path,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// Bus is a simple in-process pub/sub event bus with history.
type Bus struct {
	mu          sync.RWMutex
	subscribers map[string]chan Event
	history     []Event
	maxHistory  int
	totalCount  int // monotonic event counter (survives ring buffer drops)
}

// NewBus creates an event bus that retains up to maxHistory events.
func NewBus(maxHistory int) *Bus {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &Bus{
		subscribers: make(map[string]chan Event),
		maxHistory:  maxHistory,
	}
}

// Publish sends an event to all subscribers and appends it to history.
func (b *Bus) Publish(event Event) {
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	b.mu.Lock()
	// Ring buffer: drop oldest when full
	if len(b.history) >= b.maxHistory {
		b.history = b.history[1:]
	}
	b.history = append(b.history, event)
	b.totalCount++

	// Snapshot subscribers under lock
	subs := make([]chan Event, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		subs = append(subs, ch)
	}
	b.mu.Unlock()

	// Non-blocking send to each subscriber
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop on overflow — subscriber is too slow
		}
	}
}

// Subscribe creates a buffered channel that receives events.
func (b *Bus) Subscribe(id string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 100)
	b.subscribers[id] = ch
	return ch
}

// Unsubscribe removes a subscriber and closes its channel.
func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, ok := b.subscribers[id]; ok {
		delete(b.subscribers, id)
		close(ch)
	}
}

// History returns the most recent events, optionally filtered by type.
func (b *Bus) History(filter EventType, limit int) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	var result []Event
	for i := len(b.history) - 1; i >= 0 && len(result) < limit; i-- {
		e := b.history[i]
		if filter == "" || e.Type == filter {
			result = append(result, e)
		}
	}

	// Reverse to chronological order
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}

// HistoryAfterCursor returns events published after the given cursor position,
// up to limit. It also returns the current totalCount for use as the next cursor.
// This mirrors the TotalOutputCount/OutputHistory pattern from session/runner.go.
func (b *Bus) HistoryAfterCursor(cursor, limit int) ([]Event, int) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if limit <= 0 {
		limit = 50
	}

	// Events dropped from the ring buffer.
	dropped := b.totalCount - len(b.history)

	// Where in the history slice does the cursor land?
	startIdx := cursor - dropped
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(b.history) {
		return nil, b.totalCount
	}

	result := b.history[startIdx:]
	if len(result) > limit {
		result = result[:limit]
	}

	// Copy to avoid caller holding a reference to the internal slice.
	out := make([]Event, len(result))
	copy(out, result)
	return out, b.totalCount
}

// HistorySince returns all events after the given time.
func (b *Bus) HistorySince(since time.Time) []Event {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var result []Event
	for _, e := range b.history {
		if e.Timestamp.After(since) {
			result = append(result, e)
		}
	}
	return result
}
