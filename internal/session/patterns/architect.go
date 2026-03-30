package patterns

import (
	"fmt"
	"sync"
	"time"
)

// Phase represents the current phase of the architect pattern.
type Phase string

const (
	PhasePlan    Phase = "plan"
	PhaseExecute Phase = "execute"
	PhaseDone    Phase = "done"
)

// PlanStep is a single unit of work produced by the architect session.
type PlanStep struct {
	ID          string            `json:"id"`
	Description string            `json:"description"`
	AssignedTo  string            `json:"assigned_to,omitempty"` // executor session ID
	SkillTags   []string          `json:"skill_tags,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"` // step IDs that must complete first
	Status      string            `json:"status"`               // "pending", "assigned", "completed", "failed"
	Result      string            `json:"result,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// ArchitectPattern implements the plan-then-execute orchestration model.
// One session (the architect) produces a plan; the remaining sessions execute steps.
type ArchitectPattern struct {
	mu          sync.RWMutex
	architectID string
	executorIDs []string
	phase       Phase
	plan        []PlanStep
	memory      *SharedMemory
	startedAt   time.Time
}

// NewArchitectPattern creates a new architect pattern.
// architectID is the planning session; executorIDs are the worker sessions.
func NewArchitectPattern(architectID string, executorIDs []string, mem *SharedMemory) (*ArchitectPattern, error) {
	if len(executorIDs) == 0 {
		return nil, ErrNoExecutors
	}
	if mem == nil {
		mem = NewSharedMemory()
	}
	return &ArchitectPattern{
		architectID: architectID,
		executorIDs: executorIDs,
		phase:       PhasePlan,
		memory:      mem,
		startedAt:   time.Now(),
	}, nil
}

// Phase returns the current phase.
func (ap *ArchitectPattern) Phase() Phase {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	return ap.phase
}

// ArchitectID returns the architect session ID.
func (ap *ArchitectPattern) ArchitectID() string {
	return ap.architectID
}

// SetPlan transitions from PhasePlan to PhaseExecute with the given steps.
// Can only be called during PhasePlan.
func (ap *ArchitectPattern) SetPlan(steps []PlanStep) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	if ap.phase != PhasePlan {
		return fmt.Errorf("patterns: cannot set plan in phase %q", ap.phase)
	}
	for i := range steps {
		if steps[i].Status == "" {
			steps[i].Status = "pending"
		}
	}
	ap.plan = make([]PlanStep, len(steps))
	copy(ap.plan, steps)
	ap.phase = PhaseExecute
	return nil
}

// Plan returns a copy of the current plan steps.
func (ap *ArchitectPattern) Plan() []PlanStep {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	out := make([]PlanStep, len(ap.plan))
	copy(out, ap.plan)
	return out
}

// NextTasks returns plan steps whose dependencies are all satisfied and that
// have not yet been assigned. This drives the execution phase.
func (ap *ArchitectPattern) NextTasks() []PlanStep {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	completed := make(map[string]bool, len(ap.plan))
	for _, s := range ap.plan {
		if s.Status == "completed" {
			completed[s.ID] = true
		}
	}
	var ready []PlanStep
	for _, s := range ap.plan {
		if s.Status != "pending" {
			continue
		}
		depsOK := true
		for _, dep := range s.DependsOn {
			if !completed[dep] {
				depsOK = false
				break
			}
		}
		if depsOK {
			ready = append(ready, s)
		}
	}
	return ready
}

// AssignTask marks a step as assigned to a specific executor.
func (ap *ArchitectPattern) AssignTask(stepID, executorID string) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	for i := range ap.plan {
		if ap.plan[i].ID == stepID {
			ap.plan[i].Status = "assigned"
			ap.plan[i].AssignedTo = executorID
			return nil
		}
	}
	return fmt.Errorf("patterns: step %q not found", stepID)
}

// CompleteTask marks a step as completed with the given result.
// If all steps are complete, transitions to PhaseDone.
func (ap *ArchitectPattern) CompleteTask(stepID, result string) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	found := false
	for i := range ap.plan {
		if ap.plan[i].ID == stepID {
			ap.plan[i].Status = "completed"
			ap.plan[i].Result = result
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("patterns: step %q not found", stepID)
	}
	// Check if all steps are done.
	allDone := true
	for _, s := range ap.plan {
		if s.Status != "completed" && s.Status != "failed" {
			allDone = false
			break
		}
	}
	if allDone {
		ap.phase = PhaseDone
	}
	return nil
}

// FailTask marks a step as failed.
func (ap *ArchitectPattern) FailTask(stepID, reason string) error {
	ap.mu.Lock()
	defer ap.mu.Unlock()
	for i := range ap.plan {
		if ap.plan[i].ID == stepID {
			ap.plan[i].Status = "failed"
			ap.plan[i].Result = reason
			return nil
		}
	}
	return fmt.Errorf("patterns: step %q not found", stepID)
}

// IsComplete returns true when all plan steps are finished.
func (ap *ArchitectPattern) IsComplete() bool {
	ap.mu.RLock()
	defer ap.mu.RUnlock()
	return ap.phase == PhaseDone
}

// Memory returns the shared memory store.
func (ap *ArchitectPattern) Memory() *SharedMemory {
	return ap.memory
}
