package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ContextEntry tracks what a session is working on for cross-session coordination.
type ContextEntry struct {
	SessionID   string    `json:"session_id"`
	RepoPath    string    `json:"repo_path"`
	RepoName    string    `json:"repo_name"`
	Provider    Provider  `json:"provider"`
	TaskDesc    string    `json:"task_desc"`
	ActiveFiles []string  `json:"active_files,omitempty"` // files being modified
	LastCommit  string    `json:"last_commit,omitempty"`
	RegisteredAt time.Time `json:"registered_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ContextStore tracks active sessions per repo for conflict detection.
type ContextStore struct {
	mu       sync.Mutex
	entries  map[string]*ContextEntry // session ID → entry
	stateDir string
}

// NewContextStore creates a context store that persists to the given directory.
func NewContextStore(stateDir string) *ContextStore {
	cs := &ContextStore{
		entries:  make(map[string]*ContextEntry),
		stateDir: stateDir,
	}
	cs.load()
	return cs
}

// Register adds or updates a session's context entry.
func (cs *ContextStore) Register(entry ContextEntry) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	now := time.Now()
	entry.RegisteredAt = now
	entry.UpdatedAt = now
	cs.entries[entry.SessionID] = &entry
	cs.save()
}

// Deregister removes a session's context entry.
func (cs *ContextStore) Deregister(sessionID string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	delete(cs.entries, sessionID)
	cs.save()
}

// UpdateFiles updates the active files for a session.
func (cs *ContextStore) UpdateFiles(sessionID string, files []string) {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if entry, ok := cs.entries[sessionID]; ok {
		entry.ActiveFiles = files
		entry.UpdatedAt = time.Now()
		cs.save()
	}
}

// ActiveForRepo returns all active context entries for a given repo.
func (cs *ContextStore) ActiveForRepo(repoPath string) []ContextEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	var result []ContextEntry
	for _, entry := range cs.entries {
		if entry.RepoPath == repoPath {
			result = append(result, *entry)
		}
	}
	return result
}

// FileConflicts detects which files would conflict with a proposed file list.
// Returns a map of file → session ID of the conflicting session.
func (cs *ContextStore) FileConflicts(repoPath string, proposedFiles []string, excludeSessionID string) map[string]string {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	proposed := make(map[string]bool, len(proposedFiles))
	for _, f := range proposedFiles {
		proposed[f] = true
	}

	conflicts := make(map[string]string)
	for _, entry := range cs.entries {
		if entry.RepoPath != repoPath || entry.SessionID == excludeSessionID {
			continue
		}
		for _, f := range entry.ActiveFiles {
			if proposed[f] {
				conflicts[f] = entry.SessionID
			}
		}
	}
	return conflicts
}

// All returns all context entries.
func (cs *ContextStore) All() []ContextEntry {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	result := make([]ContextEntry, 0, len(cs.entries))
	for _, entry := range cs.entries {
		result = append(result, *entry)
	}
	return result
}

// Cleanup removes entries older than the given duration.
func (cs *ContextStore) Cleanup(maxAge time.Duration) int {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for id, entry := range cs.entries {
		if entry.UpdatedAt.Before(cutoff) {
			delete(cs.entries, id)
			removed++
		}
	}
	if removed > 0 {
		cs.save()
	}
	return removed
}

func (cs *ContextStore) save() {
	if cs.stateDir == "" {
		return
	}
	_ = os.MkdirAll(cs.stateDir, 0755)

	data, err := json.MarshalIndent(cs.entries, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(cs.stateDir, "context_store.json"), data, 0644)
}

func (cs *ContextStore) load() {
	if cs.stateDir == "" {
		return
	}
	data, err := os.ReadFile(filepath.Join(cs.stateDir, "context_store.json"))
	if err != nil {
		return
	}

	var entries map[string]*ContextEntry
	if json.Unmarshal(data, &entries) == nil && entries != nil {
		cs.entries = entries
	}
}
