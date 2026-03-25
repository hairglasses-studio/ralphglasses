package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// BlackboardEntry is a single key-value record on the shared blackboard.
type BlackboardEntry struct {
	Key       string    `json:"key"`
	Value     any       `json:"value"`
	Source    string    `json:"source"`    // subsystem that wrote it
	UpdatedAt time.Time `json:"updated_at"`
}

// Blackboard provides a shared key-value store for inter-subsystem communication.
// Subsystems can publish observations and read observations from others without
// direct coupling.
type Blackboard struct {
	mu       sync.RWMutex
	entries  map[string]BlackboardEntry
	stateDir string
}

// NewBlackboard creates a blackboard, loading any persisted state.
func NewBlackboard(stateDir string) *Blackboard {
	bb := &Blackboard{
		entries:  make(map[string]BlackboardEntry),
		stateDir: stateDir,
	}
	bb.load()
	return bb
}

// Put writes a key-value pair to the blackboard.
func (bb *Blackboard) Put(key string, value any, source string) {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	bb.entries[key] = BlackboardEntry{
		Key:       key,
		Value:     value,
		Source:    source,
		UpdatedAt: time.Now(),
	}
	bb.save()
}

// Get retrieves a value by key. Returns nil and false if not found.
func (bb *Blackboard) Get(key string) (any, bool) {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	entry, ok := bb.entries[key]
	if !ok {
		return nil, false
	}
	return entry.Value, true
}

// Query returns all entries matching the given source subsystem.
func (bb *Blackboard) Query(source string) []BlackboardEntry {
	bb.mu.RLock()
	defer bb.mu.RUnlock()

	var result []BlackboardEntry
	for _, entry := range bb.entries {
		if source == "" || entry.Source == source {
			result = append(result, entry)
		}
	}
	return result
}

// Len returns the number of entries.
func (bb *Blackboard) Len() int {
	bb.mu.RLock()
	defer bb.mu.RUnlock()
	return len(bb.entries)
}

func (bb *Blackboard) load() {
	path := filepath.Join(bb.stateDir, "blackboard.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var entries map[string]BlackboardEntry
	if err := json.Unmarshal(data, &entries); err == nil {
		bb.entries = entries
	}
}

func (bb *Blackboard) save() {
	if bb.stateDir == "" {
		return
	}
	_ = os.MkdirAll(bb.stateDir, 0755)
	data, err := json.Marshal(bb.entries)
	if err != nil {
		return
	}
	path := filepath.Join(bb.stateDir, "blackboard.json")
	_ = os.WriteFile(path, data, 0644)
}
