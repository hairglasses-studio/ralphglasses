package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Instance represents a running ralphglasses process that participates in
// leader election. Each instance registers itself under .ralph/instances/
// and maintains a periodic heartbeat so peers can detect staleness.
type Instance struct {
	ID          string    `json:"id"`
	Hostname    string    `json:"hostname"`
	PID         int       `json:"pid"`
	StartedAt   time.Time `json:"started_at"`
	HeartbeatAt time.Time `json:"heartbeat_at"`

	mu       sync.Mutex
	stateDir string // parent dir for .ralph/instances/
}

// NewInstance creates a new Instance with a fresh UUID, the current hostname,
// and the current PID. StartedAt and HeartbeatAt are set to now.
func NewInstance() *Instance {
	hostname, _ := os.Hostname()
	now := time.Now()
	return &Instance{
		ID:          uuid.New().String(),
		Hostname:    hostname,
		PID:         os.Getpid(),
		StartedAt:   now,
		HeartbeatAt: now,
	}
}

// Heartbeat updates HeartbeatAt to the current time and, if a stateDir is
// configured, persists the instance file.
func (inst *Instance) Heartbeat() {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	inst.HeartbeatAt = time.Now()
	if inst.stateDir != "" {
		_ = inst.persist()
	}
}

// IsStale returns true when the instance's last heartbeat is older than the
// given timeout duration.
func (inst *Instance) IsStale(timeout time.Duration) bool {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return time.Since(inst.HeartbeatAt) > timeout
}

// Marshal returns the JSON representation of the instance.
func (inst *Instance) Marshal() ([]byte, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return json.Marshal(inst)
}

// Unmarshal populates the instance from JSON data.
func (inst *Instance) Unmarshal(data []byte) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	return json.Unmarshal(data, inst)
}

// Register writes the instance file into stateDir/instances/<id>.json.
func (inst *Instance) Register(stateDir string) error {
	inst.mu.Lock()
	inst.stateDir = stateDir
	inst.mu.Unlock()
	return inst.persist()
}

// Deregister removes the instance file from disk.
func (inst *Instance) Deregister() error {
	inst.mu.Lock()
	dir := inst.stateDir
	id := inst.ID
	inst.mu.Unlock()
	if dir == "" {
		return nil
	}
	return os.Remove(filepath.Join(dir, "instances", id+".json"))
}

// persist writes the instance JSON to disk. Caller must NOT hold inst.mu
// (Marshal acquires it).
func (inst *Instance) persist() error {
	dir := filepath.Join(inst.stateDir, "instances")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(inst)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, inst.ID+".json"), data, 0o644)
}
