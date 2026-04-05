package patterns

import (
	"errors"
	"maps"
	"sync"
)

// Errors used across the patterns package.
var (
	ErrKeyNotFound          = errors.New("patterns: key not found")
	ErrUnknownMessageType   = errors.New("patterns: unknown message type")
	ErrNoExecutors          = errors.New("patterns: no executor sessions configured")
	ErrEmptyChain           = errors.New("patterns: review chain is empty")
	ErrNoMatchingSpecialist = errors.New("patterns: no specialist matched the task")
)

// SharedMemory is a thread-safe key-value store for inter-session communication.
// It supports watch/notify so sessions can react to changes from other sessions.
type SharedMemory struct {
	mu        sync.RWMutex
	data      map[string]string
	revisions map[string]int64
	watchers  map[string][]chan MemoryUpdate
	globalRev int64
}

// NewSharedMemory creates an empty shared memory store.
func NewSharedMemory() *SharedMemory {
	return &SharedMemory{
		data:      make(map[string]string),
		revisions: make(map[string]int64),
		watchers:  make(map[string][]chan MemoryUpdate),
	}
}

// Set writes a key-value pair and notifies all watchers of that key.
func (sm *SharedMemory) Set(key, value string) int64 {
	sm.mu.Lock()
	sm.globalRev++
	rev := sm.globalRev
	sm.data[key] = value
	sm.revisions[key] = rev
	// Snapshot watchers under lock, notify outside to avoid holding lock during send.
	watchers := make([]chan MemoryUpdate, len(sm.watchers[key]))
	copy(watchers, sm.watchers[key])
	sm.mu.Unlock()

	update := MemoryUpdate{Key: key, Value: value, Revision: rev}
	for _, ch := range watchers {
		select {
		case ch <- update:
		default:
			// Non-blocking: drop if watcher is slow.
		}
	}
	return rev
}

// Get retrieves a value by key. Returns ErrKeyNotFound if missing.
func (sm *SharedMemory) Get(key string) (string, int64, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	v, ok := sm.data[key]
	if !ok {
		return "", 0, ErrKeyNotFound
	}
	return v, sm.revisions[key], nil
}

// Delete removes a key and notifies watchers with an empty value.
func (sm *SharedMemory) Delete(key string) bool {
	sm.mu.Lock()
	_, existed := sm.data[key]
	if !existed {
		sm.mu.Unlock()
		return false
	}
	sm.globalRev++
	rev := sm.globalRev
	delete(sm.data, key)
	delete(sm.revisions, key)
	watchers := make([]chan MemoryUpdate, len(sm.watchers[key]))
	copy(watchers, sm.watchers[key])
	sm.mu.Unlock()

	update := MemoryUpdate{Key: key, Value: "", Revision: rev}
	for _, ch := range watchers {
		select {
		case ch <- update:
		default:
		}
	}
	return true
}

// Keys returns all current keys (snapshot).
func (sm *SharedMemory) Keys() []string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	keys := make([]string, 0, len(sm.data))
	for k := range sm.data {
		keys = append(keys, k)
	}
	return keys
}

// Watch returns a channel that receives MemoryUpdate events for the given key.
// bufSize controls the channel buffer; use 0 for unbuffered (may drop updates).
func (sm *SharedMemory) Watch(key string, bufSize int) <-chan MemoryUpdate {
	ch := make(chan MemoryUpdate, bufSize)
	sm.mu.Lock()
	sm.watchers[key] = append(sm.watchers[key], ch)
	sm.mu.Unlock()
	return ch
}

// Unwatch removes a watcher channel for the given key and closes it.
func (sm *SharedMemory) Unwatch(key string, ch <-chan MemoryUpdate) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	watchers := sm.watchers[key]
	for i, w := range watchers {
		// Compare by channel identity via interface comparison.
		if (<-chan MemoryUpdate)(w) == ch {
			sm.watchers[key] = append(watchers[:i], watchers[i+1:]...)
			close(w)
			return
		}
	}
}

// Snapshot returns a copy of all key-value pairs.
func (sm *SharedMemory) Snapshot() map[string]string {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	snap := make(map[string]string, len(sm.data))
	maps.Copy(snap, sm.data)
	return snap
}
