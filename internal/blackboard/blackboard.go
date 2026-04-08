package blackboard

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ErrVersionConflict is returned by Put when the caller supplies a non-zero
// Version that does not match the current version of the entry (CAS failure).
var ErrVersionConflict = errors.New("blackboard: version conflict")

// Entry is a single key-value record stored on the blackboard.
type Entry struct {
	Key       string         `json:"key"`
	Namespace string         `json:"namespace"`
	Value     map[string]any `json:"value"`
	WriterID  string         `json:"writer_id"`
	Version   int64          `json:"version"`
	TTL       time.Duration  `json:"ttl"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// WatchFunc is called whenever an entry is written via Put.
type WatchFunc func(entry Entry)

// Option configures a Blackboard at construction time.
type Option func(*Blackboard)

// WithDefaultTTL sets a default TTL applied to every entry that does not
// already carry a non-zero TTL when passed to Put.
func WithDefaultTTL(d time.Duration) Option {
	return func(bb *Blackboard) {
		bb.defaultTTL = d
	}
}

// Blackboard is a shared coordination structure for fleet workers.
// It stores entries keyed by namespace+key, supports optimistic concurrency
// via monotonic versioning, watcher notifications, TTL-based GC, and JSONL
// persistence.
type Blackboard struct {
	mu         sync.RWMutex
	entries    map[string]*Entry // keyed by compositeKey(namespace, key)
	watchers   []WatchFunc
	stateDir   string
	defaultTTL time.Duration

	// evictor lifecycle
	evictStop chan struct{}
	evictDone chan struct{}
}

// compositeKey builds the map key from namespace and key.
func compositeKey(namespace, key string) string {
	return namespace + "\x00" + key
}

// NewBlackboard creates a Blackboard and loads any persisted state from
// stateDir. If stateDir is empty, persistence is disabled.
func NewBlackboard(stateDir string, opts ...Option) *Blackboard {
	bb := &Blackboard{
		entries:  make(map[string]*Entry),
		stateDir: stateDir,
	}
	for _, o := range opts {
		o(bb)
	}
	bb.load()
	return bb
}

// Put writes an entry to the blackboard. If entry.Version > 0, optimistic
// concurrency is enforced: the put succeeds only when the supplied version
// matches the stored version (CAS). On success the version is incremented,
// timestamps are set, watchers are notified, and the entry is persisted.
func (bb *Blackboard) Put(entry Entry) error {
	bb.mu.Lock()

	ck := compositeKey(entry.Namespace, entry.Key)
	existing, ok := bb.entries[ck]

	// CAS check: non-zero Version must match current.
	if entry.Version > 0 {
		if !ok {
			bb.mu.Unlock()
			return ErrVersionConflict
		}
		if existing.Version != entry.Version {
			bb.mu.Unlock()
			return ErrVersionConflict
		}
	}

	// Apply default TTL when the entry does not carry one.
	if entry.TTL == 0 && bb.defaultTTL > 0 {
		entry.TTL = bb.defaultTTL
	}

	now := time.Now()
	if ok {
		entry.Version = existing.Version + 1
		entry.CreatedAt = existing.CreatedAt
	} else {
		entry.Version = 1
		entry.CreatedAt = now
	}
	entry.UpdatedAt = now

	stored := entry // copy
	bb.entries[ck] = &stored

	// Snapshot watchers under the lock so we can notify outside.
	watchers := make([]WatchFunc, len(bb.watchers))
	copy(watchers, bb.watchers)

	bb.mu.Unlock()

	// Persist before notifying so watchers can rely on durability.
	bb.appendToFile(&stored)

	// Notify watchers outside the lock.
	for _, fn := range watchers {
		fn(stored)
	}

	return nil
}

// Get retrieves an entry by namespace and key.
func (bb *Blackboard) Get(namespace, key string) (*Entry, bool) {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	e, ok := bb.entries[compositeKey(namespace, key)]
	if !ok {
		return nil, false
	}
	cp := *e
	return &cp, true
}

// Query returns all entries in the given namespace. The returned slice is a
// snapshot; mutating it does not affect the blackboard.
func (bb *Blackboard) Query(namespace string) []Entry {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	var results []Entry
	for _, e := range bb.entries {
		if e.Namespace == namespace {
			results = append(results, *e)
		}
	}
	return results
}

// Watch registers a function to be called on every successful Put.
func (bb *Blackboard) Watch(fn WatchFunc) {
	bb.mu.Lock()
	defer bb.mu.Unlock()
	bb.watchers = append(bb.watchers, fn)
}

// GC removes entries whose TTL has expired (TTL > 0 and
// time.Since(UpdatedAt) > TTL).
func (bb *Blackboard) GC() {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	now := time.Now()
	for ck, e := range bb.entries {
		if e.TTL > 0 && now.Sub(e.UpdatedAt) > e.TTL {
			delete(bb.entries, ck)
		}
	}
}

// Len returns the total number of entries.
func (bb *Blackboard) Len() int {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return len(bb.entries)
}

// Snapshot compacts the JSONL persistence file by writing only the current
// in-memory state. This replaces the append-only log with a minimal
// representation.
func (bb *Blackboard) Snapshot() error {
	if bb.stateDir == "" {
		return nil
	}

	bb.mu.RLock()
	entries := make([]*Entry, 0, len(bb.entries))
	for _, e := range bb.entries {
		entries = append(entries, e)
	}
	bb.mu.RUnlock()

	if err := os.MkdirAll(bb.stateDir, 0755); err != nil {
		slog.Error("blackboard: create state dir for snapshot", "path", bb.stateDir, "err", err)
		return err
	}

	path := filepath.Join(bb.stateDir, "blackboard.jsonl")
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
		data = append(data, '\n')
		if _, err := f.Write(data); err != nil {
			f.Close()
			os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}

	return os.Rename(tmp, path)
}

// --- persistence helpers ---

func (bb *Blackboard) appendToFile(e *Entry) {
	if bb.stateDir == "" {
		return
	}
	if err := os.MkdirAll(bb.stateDir, 0755); err != nil {
		slog.Error("blackboard: create state dir", "path", bb.stateDir, "err", err)
		return
	}

	data, err := json.Marshal(e)
	if err != nil {
		slog.Error("blackboard: marshal entry", "key", e.Key, "err", err)
		return
	}
	data = append(data, '\n')

	path := filepath.Join(bb.stateDir, "blackboard.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Error("blackboard: open file", "path", path, "err", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		slog.Error("blackboard: write entry", "path", path, "err", err)
	}
}

func (bb *Blackboard) load() {
	if bb.stateDir == "" {
		return
	}
	path := filepath.Join(bb.stateDir, "blackboard.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			slog.Warn("blackboard: load failed", "path", path, "err", err)
		}
		return
	}

	for line := range bytes.SplitSeq(data, []byte("\n")) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var e Entry
		if json.Unmarshal(line, &e) == nil {
			ck := compositeKey(e.Namespace, e.Key)
			stored := e
			bb.entries[ck] = &stored
		}
	}
}
