package session

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Coordinator provides cross-session task coordination so that multiple
// concurrent sessions do not work on the same task. Claims are held in an
// in-memory map protected by a mutex; optional file-backed persistence
// allows claims to survive process restarts.
type Coordinator struct {
	mu       sync.Mutex
	claims   map[string]string // taskID -> sessionID
	filePath string            // empty means no persistence
}

// NewCoordinator creates a Coordinator with no file-backed persistence.
func NewCoordinator() *Coordinator {
	return &Coordinator{
		claims: make(map[string]string),
	}
}

// NewCoordinatorWithPersistence creates a Coordinator that persists claims
// to the given file path. If the file already exists, existing claims are
// loaded on construction.
func NewCoordinatorWithPersistence(path string) (*Coordinator, error) {
	c := &Coordinator{
		claims:   make(map[string]string),
		filePath: path,
	}
	if err := c.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return c, nil
}

// ClaimTask attempts to assign taskID to sessionID. It returns true if the
// claim was successful or if the same session already owns the task. It
// returns false (without error) if another session already claimed it.
func (c *Coordinator) ClaimTask(sessionID, taskID string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if owner, ok := c.claims[taskID]; ok {
		if owner == sessionID {
			return true, nil // idempotent
		}
		return false, nil
	}

	c.claims[taskID] = sessionID
	if err := c.persist(); err != nil {
		delete(c.claims, taskID)
		return false, err
	}
	return true, nil
}

// ReleaseTask removes the claim on taskID held by sessionID. If the task
// is not claimed by that session (or not claimed at all), it is a no-op.
func (c *Coordinator) ReleaseTask(sessionID, taskID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if owner, ok := c.claims[taskID]; ok && owner == sessionID {
		delete(c.claims, taskID)
		_ = c.persist() // best-effort
	}
}

// ActiveClaims returns all task IDs currently claimed by sessionID.
func (c *Coordinator) ActiveClaims(sessionID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var tasks []string
	for taskID, owner := range c.claims {
		if owner == sessionID {
			tasks = append(tasks, taskID)
		}
	}
	return tasks
}

// AllClaims returns a snapshot of all current claims (taskID -> sessionID).
func (c *Coordinator) AllClaims() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make(map[string]string, len(c.claims))
	maps.Copy(snapshot, c.claims)
	return snapshot
}

// ReleaseAll removes every claim held by sessionID. This is useful when a
// session terminates and its tasks should become available for others.
func (c *Coordinator) ReleaseAll(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for taskID, owner := range c.claims {
		if owner == sessionID {
			delete(c.claims, taskID)
		}
	}
	_ = c.persist()
}

// coordinatorSnapshot is the JSON representation written to disk.
type coordinatorSnapshot struct {
	Claims    map[string]string `json:"claims"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// persist writes claims to disk if a file path is configured. Must be
// called while c.mu is held.
func (c *Coordinator) persist() error {
	if c.filePath == "" {
		return nil
	}
	snap := coordinatorSnapshot{
		Claims:    c.claims,
		UpdatedAt: time.Now(),
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.filePath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.filePath, data, 0o644)
}

// load reads claims from disk. Must be called while c.mu is held (or
// during construction before the Coordinator is shared).
func (c *Coordinator) load() error {
	if c.filePath == "" {
		return nil
	}
	data, err := os.ReadFile(c.filePath)
	if err != nil {
		return err
	}
	var snap coordinatorSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return err
	}
	if snap.Claims != nil {
		c.claims = snap.Claims
	}
	return nil
}
