package session

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// CycleState represents the state of a single orchestrated R&D cycle.
type CycleState struct {
	ID        string             `json:"id"`
	Phase     string             `json:"phase"` // proposed, baselining, improving, validating, complete, failed
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Metrics   map[string]float64 `json:"metrics,omitempty"`
	Attempts  int                `json:"attempts"`
	Error     string             `json:"error,omitempty"`
}

// orchestratorPhaseOrder defines the linear phase progression for the orchestrator.
var orchestratorPhaseOrder = []string{
	"proposed",
	"baselining",
	"improving",
	"validating",
	"complete",
}

// orchestratorNextPhase maps each phase to its successor.
var orchestratorNextPhase = map[string]string{
	"proposed":   "baselining",
	"baselining": "improving",
	"improving":  "validating",
	"validating": "complete",
}

// CycleOrchestrator manages multiple concurrent R&D cycles with
// concurrency limits and thread-safe access.
type CycleOrchestrator struct {
	cycles        map[string]*CycleState
	mu            sync.RWMutex
	maxConcurrent int
	stateDir      string
}

// NewCycleOrchestrator creates a new orchestrator with the given concurrency
// limit and state directory.
func NewCycleOrchestrator(maxConcurrent int, stateDir string) *CycleOrchestrator {
	return &CycleOrchestrator{
		cycles:        make(map[string]*CycleState),
		maxConcurrent: maxConcurrent,
		stateDir:      stateDir,
	}
}

// isTerminalPhase returns true if the phase is complete or failed.
func isTerminalPhase(phase string) bool {
	return phase == "complete" || phase == "failed"
}

// Create adds a new cycle in the "proposed" phase. Returns an error if the
// cycle ID already exists or if the maximum concurrent (non-terminal) cycle
// limit has been reached.
func (o *CycleOrchestrator) Create(id string) (*CycleState, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	if _, exists := o.cycles[id]; exists {
		return nil, fmt.Errorf("cycle %q already exists", id)
	}

	// Count active (non-terminal) cycles.
	active := 0
	for _, c := range o.cycles {
		if !isTerminalPhase(c.Phase) {
			active++
		}
	}
	if active >= o.maxConcurrent {
		return nil, fmt.Errorf("max concurrent cycles reached (%d)", o.maxConcurrent)
	}

	now := time.Now()
	cs := &CycleState{
		ID:        id,
		Phase:     "proposed",
		CreatedAt: now,
		UpdatedAt: now,
		Metrics:   make(map[string]float64),
		Attempts:  0,
	}
	o.cycles[id] = cs
	return cs, nil
}

// Advance moves a cycle to the next phase in the linear progression:
// proposed -> baselining -> improving -> validating -> complete.
// Returns an error if the cycle is not found, is in a terminal phase,
// or has no valid next phase.
func (o *CycleOrchestrator) Advance(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	cs, ok := o.cycles[id]
	if !ok {
		return fmt.Errorf("cycle %q not found", id)
	}

	if isTerminalPhase(cs.Phase) {
		return fmt.Errorf("cannot advance cycle %q from terminal phase %q", id, cs.Phase)
	}

	next, ok := orchestratorNextPhase[cs.Phase]
	if !ok {
		return fmt.Errorf("no valid transition from phase %q", cs.Phase)
	}

	cs.Phase = next
	cs.UpdatedAt = time.Now()
	cs.Attempts++
	return nil
}

// Fail transitions a cycle to the "failed" phase from any non-terminal phase.
func (o *CycleOrchestrator) Fail(id string, reason string) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	cs, ok := o.cycles[id]
	if !ok {
		return fmt.Errorf("cycle %q not found", id)
	}

	if isTerminalPhase(cs.Phase) {
		return fmt.Errorf("cannot fail cycle %q from terminal phase %q", id, cs.Phase)
	}

	cs.Phase = "failed"
	cs.Error = reason
	cs.UpdatedAt = time.Now()
	return nil
}

// Get returns a cycle by ID.
func (o *CycleOrchestrator) Get(id string) (*CycleState, bool) {
	o.mu.RLock()
	defer o.mu.RUnlock()

	cs, ok := o.cycles[id]
	return cs, ok
}

// List returns all cycles sorted by CreatedAt descending (newest first).
func (o *CycleOrchestrator) List() []*CycleState {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make([]*CycleState, 0, len(o.cycles))
	for _, cs := range o.cycles {
		result = append(result, cs)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})
	return result
}

// ActiveCount returns the number of non-terminal cycles.
func (o *CycleOrchestrator) ActiveCount() int {
	o.mu.RLock()
	defer o.mu.RUnlock()

	count := 0
	for _, cs := range o.cycles {
		if !isTerminalPhase(cs.Phase) {
			count++
		}
	}
	return count
}
