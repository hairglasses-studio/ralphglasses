package session

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Coordinator provides file-backed resource coordination so concurrent
// sessions or team tasks do not claim the same resource simultaneously.
// Claims are held in-memory behind a mutex and optionally persisted.
type Coordinator struct {
	mu       sync.Mutex
	claims   map[string]string // resource -> owner
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

// ClaimResource attempts to assign resource to owner. It returns true if the
// claim was successful or if the same owner already holds it.
func (c *Coordinator) ClaimResource(owner, resource string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current, ok := c.claims[resource]; ok {
		if current == owner {
			return true, nil // idempotent
		}
		return false, nil
	}

	c.claims[resource] = owner
	if err := c.persist(); err != nil {
		delete(c.claims, resource)
		return false, err
	}
	return true, nil
}

// ClaimResources atomically claims a set of resources for owner. The optional
// conflicts callback can reject claims based on overlapping existing/requested
// resources even when the resource keys differ.
func (c *Coordinator) ClaimResources(owner string, resources []string, conflicts func(existing, requested string) bool) (bool, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, requested := range resources {
		for existing, current := range c.claims {
			if current == owner {
				continue
			}
			if existing == requested || (conflicts != nil && conflicts(existing, requested)) {
				return false, existing, nil
			}
		}
	}

	for _, resource := range resources {
		c.claims[resource] = owner
	}
	if err := c.persist(); err != nil {
		for _, resource := range resources {
			if current, ok := c.claims[resource]; ok && current == owner {
				delete(c.claims, resource)
			}
		}
		return false, "", err
	}
	return true, "", nil
}

// ReleaseResource removes the claim on resource held by owner. If the resource
// is not claimed by that owner, it is a no-op.
func (c *Coordinator) ReleaseResource(owner, resource string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if current, ok := c.claims[resource]; ok && current == owner {
		delete(c.claims, resource)
		_ = c.persist()
	}
}

// ActiveResources returns all resources currently claimed by owner.
func (c *Coordinator) ActiveResources(owner string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var resources []string
	for resource, current := range c.claims {
		if current == owner {
			resources = append(resources, resource)
		}
	}
	return resources
}

// AllResources returns a snapshot of all current claims (resource -> owner).
func (c *Coordinator) AllResources() map[string]string {
	c.mu.Lock()
	defer c.mu.Unlock()

	snapshot := make(map[string]string, len(c.claims))
	maps.Copy(snapshot, c.claims)
	return snapshot
}

// ReleaseAll removes every claim held by owner.
func (c *Coordinator) ReleaseAll(owner string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for resource, current := range c.claims {
		if current == owner {
			delete(c.claims, resource)
		}
	}
	_ = c.persist()
}

// ClaimTask is a backwards-compatible alias for ClaimResource.
func (c *Coordinator) ClaimTask(sessionID, taskID string) (bool, error) {
	return c.ClaimResource(sessionID, taskID)
}

// ReleaseTask is a backwards-compatible alias for ReleaseResource.
func (c *Coordinator) ReleaseTask(sessionID, taskID string) {
	c.ReleaseResource(sessionID, taskID)
}

// ActiveClaims is a backwards-compatible alias for ActiveResources.
func (c *Coordinator) ActiveClaims(sessionID string) []string {
	c.mu.Lock()
	defer c.mu.Unlock()

	var tasks []string
	for taskID, current := range c.claims {
		if current == sessionID {
			tasks = append(tasks, taskID)
		}
	}
	return tasks
}

// AllClaims is a backwards-compatible alias for AllResources.
func (c *Coordinator) AllClaims() map[string]string {
	return c.AllResources()
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
