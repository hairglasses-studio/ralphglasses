package events

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	LoopStarted   EventType = "loop.started"
	LoopStopped   EventType = "loop.stopped"
	LoopRestarted EventType = "loop.restarted"
	LoopIterated  EventType = "loop.iterated"

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

	// Errors
	SessionError EventType = "session.error" // Non-fatal session-level error

	// Self-improvement
	AutoOptimized    EventType = "auto.optimized"     // Level 2+ decision executed
	ProviderSelected EventType = "provider.selected"   // Smart provider selection
	SessionRecovered EventType = "session.recovered"   // Auto-recovery restart
	ContextConflict  EventType = "context.conflict"    // Cross-session file conflict

	// Provider health
	ProviderHealthChanged EventType = "provider.health" // Provider health state transition

	// Self-improvement acceptance
	SelfImproveMerged EventType = "self_improve.merged"     // Safe changes auto-merged
	SelfImprovePR     EventType = "self_improve.pr_created" // PR created for review-required changes

	// Worker lifecycle
	WorkerDeregistered EventType = "worker.deregistered"
	WorkerPaused       EventType = "worker.paused"
	WorkerResumed      EventType = "worker.resumed"
)

// knownEventTypes is the set of all declared EventType constants.
var knownEventTypes = map[EventType]struct{}{
	SessionStarted:        {},
	SessionEnded:          {},
	SessionStopped:        {},
	CostUpdate:            {},
	BudgetExceeded:        {},
	LoopStarted:           {},
	LoopStopped:           {},
	LoopRestarted:         {},
	LoopIterated:          {},
	LoopRegression:        {},
	TeamCreated:           {},
	JournalWritten:        {},
	ConfigChanged:         {},
	ScanComplete:          {},
	PromptEnhanced:        {},
	ToolCalled:            {},
	SessionError:          {},
	AutoOptimized:         {},
	ProviderSelected:      {},
	SessionRecovered:      {},
	ContextConflict:       {},
	ProviderHealthChanged: {},
	SelfImproveMerged:     {},
	SelfImprovePR:         {},
	WorkerDeregistered:    {},
	WorkerPaused:          {},
	WorkerResumed:         {},
}

// ValidEventType returns true if the given EventType is a known constant.
func ValidEventType(t EventType) bool {
	_, ok := knownEventTypes[t]
	return ok
}

// Event represents something that happened in the system.
type Event struct {
	Type      EventType      `json:"type"`
	Version   int            `json:"v"`
	Timestamp time.Time      `json:"timestamp"`
	NodeID    string         `json:"node_id,omitempty"`
	RepoName  string         `json:"repo_name,omitempty"`
	RepoPath  string         `json:"repo_path,omitempty"`
	SessionID string         `json:"session_id,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

// filteredSub holds a subscriber channel and the set of event types it accepts.
type filteredSub struct {
	ch    chan Event
	types map[EventType]struct{}
}

// Bus is a simple in-process pub/sub event bus with history.
type Bus struct {
	mu           sync.RWMutex
	subscribers  map[string]chan Event
	filteredSubs map[string]filteredSub
	history      []Event
	maxHistory   int
	totalCount   int // monotonic event counter (survives ring buffer drops)
	retentionTTL time.Duration

	persistFile   *os.File
	persistPath   string
	persistWrites int

	// Async write support
	AsyncWrites  bool
	asyncStarted bool
	asyncBufSize int
	writeCh      chan Event
	writeDone    chan struct{}
}

// BusOption configures a Bus during construction.
type BusOption func(*Bus)

// WithAsyncWrites returns a BusOption that enables async event persistence
// with the given channel buffer size. The background writer goroutine is
// started automatically once PersistTo is called (or immediately if PersistTo
// was already called).
func WithAsyncWrites(bufSize int) BusOption {
	return func(b *Bus) {
		b.asyncBufSize = bufSize
		if b.asyncBufSize <= 0 {
			b.asyncBufSize = 256
		}
		b.AsyncWrites = true
	}
}

// NewBus creates an event bus that retains up to maxHistory events.
func NewBus(maxHistory int, opts ...BusOption) *Bus {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	b := &Bus{
		subscribers:  make(map[string]chan Event),
		filteredSubs: make(map[string]filteredSub),
		maxHistory:   maxHistory,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// SetRetentionTTL configures how long events are retained in history.
// Events older than the TTL are trimmed during Publish. Zero disables TTL.
func (b *Bus) SetRetentionTTL(ttl time.Duration) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.retentionTTL = ttl
}

// PublishCtx sends an event to all subscribers and appends it to history,
// respecting context cancellation. If the context is already cancelled,
// the event is not published and the context error is returned.
func (b *Bus) PublishCtx(ctx context.Context, event Event) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}
	if event.Version == 0 {
		event.Version = 1
	}

	if !ValidEventType(event.Type) {
		slog.Warn("unknown event type published", "type", event.Type)
	}

	b.mu.Lock()
	// Ring buffer: drop oldest when full
	if len(b.history) >= b.maxHistory {
		b.history = b.history[1:]
	}
	b.history = append(b.history, event)
	b.totalCount++

	// TTL retention: trim events older than the retention window
	if b.retentionTTL > 0 {
		cutoff := time.Now().Add(-b.retentionTTL)
		trimIdx := 0
		for trimIdx < len(b.history) && b.history[trimIdx].Timestamp.Before(cutoff) {
			trimIdx++
		}
		if trimIdx > 0 {
			b.history = b.history[trimIdx:]
		}
	}

	// Persist to JSONL file if configured
	if b.persistFile != nil {
		if b.AsyncWrites && b.writeCh != nil {
			// Unlock before sending to avoid holding the lock during channel send
			b.mu.Unlock()
			select {
			case b.writeCh <- event:
			default:
				slog.Warn("event async write channel full, dropping persist", "type", event.Type)
			}
			b.mu.Lock()
		} else {
			b.appendEvent(event)
		}
	}

	// Snapshot unfiltered subscribers under lock
	subs := make([]chan Event, 0, len(b.subscribers))
	for _, ch := range b.subscribers {
		subs = append(subs, ch)
	}

	// Snapshot filtered subscribers that match this event type
	var filteredChans []chan Event
	for _, fs := range b.filteredSubs {
		if _, ok := fs.types[event.Type]; ok {
			filteredChans = append(filteredChans, fs.ch)
		}
	}
	b.mu.Unlock()

	// Non-blocking send to each unfiltered subscriber
	for _, ch := range subs {
		select {
		case ch <- event:
		default:
			// Drop on overflow — subscriber is too slow
		}
	}

	// Non-blocking send to each matching filtered subscriber
	for _, ch := range filteredChans {
		select {
		case ch <- event:
		default:
		}
	}

	return nil
}

// Publish sends an event to all subscribers and appends it to history.
// This is a backward-compatible wrapper around PublishCtx that uses
// context.Background().
func (b *Bus) Publish(event Event) {
	_ = b.PublishCtx(context.Background(), event)
}

// Subscribe creates a buffered channel that receives events.
func (b *Bus) Subscribe(id string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 100)
	b.subscribers[id] = ch
	return ch
}

// SubscribeFiltered returns a channel that receives only events of the specified types.
func (b *Bus) SubscribeFiltered(id string, types ...EventType) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	typeSet := make(map[EventType]struct{}, len(types))
	for _, t := range types {
		typeSet[t] = struct{}{}
	}

	ch := make(chan Event, 100)
	b.filteredSubs[id] = filteredSub{ch: ch, types: typeSet}
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
	if fs, ok := b.filteredSubs[id]; ok {
		delete(b.filteredSubs, id)
		close(fs.ch)
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

// PersistTo enables JSONL event persistence to the given file path.
// Events are appended atomically. Safe for concurrent use.
func (b *Bus) PersistTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("persist events: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("persist events: open: %w", err)
	}
	b.mu.Lock()
	b.persistFile = f
	b.persistPath = path
	b.mu.Unlock()
	return nil
}

// Close flushes and closes the persist file, if any.
// If async writes are active, drains the write channel first with a 5s timeout.
func (b *Bus) Close() error {
	// Drain async writer first (outside lock to avoid deadlock)
	b.mu.Lock()
	ch := b.writeCh
	done := b.writeDone
	if ch != nil {
		b.writeCh = nil
	}
	b.mu.Unlock()

	if ch != nil {
		close(ch)
		select {
		case <-done:
			// drained successfully
		case <-time.After(5 * time.Second):
			slog.Warn("event bus async drain timed out after 5s")
		}
		b.mu.Lock()
		b.asyncStarted = false
		b.AsyncWrites = false
		b.mu.Unlock()
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	if b.persistFile != nil {
		err := b.persistFile.Close()
		b.persistFile = nil
		b.persistPath = ""
		return err
	}
	return nil
}

// appendEvent writes a single event to the persist file and handles rotation.
// Must be called with b.mu held.
func (b *Bus) appendEvent(e Event) {
	data, err := json.Marshal(e)
	if err != nil {
		slog.Warn("event marshal failed", "type", e.Type, "err", err)
		return
	}
	if _, err := b.persistFile.Write(append(data, '\n')); err != nil {
		slog.Warn("event persist failed", "type", e.Type, "err", err)
	}
	b.persistWrites++
	if b.persistWrites%100 == 0 {
		if info, err := b.persistFile.Stat(); err == nil && info.Size() > 10*1024*1024 {
			b.rotateFile()
		}
	}
}

// StartAsyncWrites launches the background writer goroutine for async persistence.
// bufSize controls the channel buffer capacity; if <= 0, defaults to 256.
// Idempotent: calling it when async writes are already running is a no-op.
func (b *Bus) StartAsyncWrites(bufSize int) {
	b.mu.Lock()
	if b.asyncStarted {
		b.mu.Unlock()
		return
	}
	if bufSize <= 0 {
		bufSize = 256
	}
	b.asyncStarted = true
	b.AsyncWrites = true
	b.asyncBufSize = bufSize
	b.writeCh = make(chan Event, bufSize)
	b.writeDone = make(chan struct{})
	b.mu.Unlock()
	b.startWriter()
}

// StartAsync launches the background writer goroutine for async persistence.
// Deprecated: Use StartAsyncWrites(bufSize) instead.
// Only effective when AsyncWrites is true and PersistTo has been called.
func (b *Bus) StartAsync() {
	if !b.AsyncWrites {
		return
	}
	bufSize := b.asyncBufSize
	if bufSize <= 0 {
		bufSize = 256
	}
	b.StartAsyncWrites(bufSize)
}

// StopAsyncWrites closes the write channel, waits for the background goroutine
// to drain all remaining events, and sets AsyncWrites to false.
// Safe to call even if async writes were never started (no-op).
func (b *Bus) StopAsyncWrites() {
	b.mu.Lock()
	if !b.asyncStarted || b.writeCh == nil {
		b.mu.Unlock()
		return
	}
	ch := b.writeCh
	done := b.writeDone
	b.writeCh = nil
	b.mu.Unlock()

	close(ch)
	<-done

	b.mu.Lock()
	b.asyncStarted = false
	b.AsyncWrites = false
	b.mu.Unlock()
}

// startWriter spawns the background goroutine that reads from writeCh and persists events.
// Must only be called once per start cycle (guarded by asyncStarted).
func (b *Bus) startWriter() {
	go func() {
		defer close(b.writeDone)
		for e := range b.writeCh {
			b.mu.Lock()
			if b.persistFile != nil {
				b.appendEvent(e)
			}
			b.mu.Unlock()
		}
	}()
}

// rotateFile renames the current persist file to .1 and opens a new one.
// Must be called with b.mu held.
func (b *Bus) rotateFile() {
	if b.persistFile == nil {
		return
	}
	b.persistFile.Close()
	if err := os.Rename(b.persistPath, b.persistPath+".1"); err != nil {
		slog.Warn("event file rotation failed", "err", err)
	}
	f, err := os.OpenFile(b.persistPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		b.persistFile = f
	} else {
		slog.Warn("event file rotation failed", "err", err)
		b.persistFile = nil
	}
}

// LoadEvents reads events from a JSONL file and returns the last limit events.
// If limit <= 0, all events are returned.
func LoadEvents(path string, limit int) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("load events: %w", err)
	}
	defer f.Close()

	var all []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		ev, err := MigrateEvent(json.RawMessage(line))
		if err != nil {
			continue // skip malformed lines
		}
		all = append(all, ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("load events: scan: %w", err)
	}

	if limit > 0 && len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}
