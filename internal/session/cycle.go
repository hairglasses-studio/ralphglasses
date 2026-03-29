package session

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CyclePhase represents the current phase of an R&D cycle.
type CyclePhase string

const (
	CycleProposed     CyclePhase = "proposed"
	CycleBaselining   CyclePhase = "baselining"
	CycleExecuting    CyclePhase = "executing"
	CycleObserving    CyclePhase = "observing"
	CycleSynthesizing CyclePhase = "synthesizing"
	CycleComplete     CyclePhase = "complete"
	CycleFailed       CyclePhase = "failed"
)

// validTransitions defines the allowed phase transitions for the cycle state machine.
// Any phase can transition to CycleFailed (handled separately in Advance).
var validTransitions = map[CyclePhase]CyclePhase{
	CycleProposed:     CycleBaselining,
	CycleBaselining:   CycleExecuting,
	CycleExecuting:    CycleObserving,
	CycleObserving:    CycleSynthesizing,
	CycleSynthesizing: CycleComplete,
}

// CycleRun is persisted state for an R&D cycle, modeled after LoopRun.
type CycleRun struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	RepoPath        string     `json:"repo_path"`
	Phase           CyclePhase `json:"phase"`
	Objective       string     `json:"objective"`
	SuccessCriteria []string   `json:"success_criteria"`

	// Phase outputs
	BaselineID string          `json:"baseline_id,omitempty"`
	Tasks      []CycleTask     `json:"tasks"`
	LoopIDs    []string        `json:"loop_ids,omitempty"`
	Findings   []CycleFinding  `json:"findings,omitempty"`
	Synthesis  *CycleSynthesis `json:"synthesis,omitempty"`

	// Tracking
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Error     string    `json:"error,omitempty"`
}

// CycleTask is a single task within a cycle, produced from findings, roadmap, or manual input.
type CycleTask struct {
	Title     string  `json:"title"`
	Prompt    string  `json:"prompt"`
	Source    string  `json:"source"`               // "finding", "roadmap", "manual"
	FindingID string  `json:"finding_id,omitempty"`
	Priority  float64 `json:"priority"`
	Status    string  `json:"status"` // "pending", "executing", "done", "failed"
	LoopID    string  `json:"loop_id,omitempty"`
}

// CycleFinding captures an observation or issue discovered during a cycle.
type CycleFinding struct {
	ID          string `json:"id"`
	Description string `json:"description"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Source      string `json:"source"` // "observation", "gate", "manual"
}

// CycleSynthesis summarizes the outcome of a completed cycle.
type CycleSynthesis struct {
	Summary       string   `json:"summary"`
	Accomplished  []string `json:"accomplished"`
	Remaining     []string `json:"remaining"`
	NextObjective string   `json:"next_objective"`
	Patterns      []string `json:"patterns"`
}

// NewCycleRun creates a new cycle in the proposed phase.
func NewCycleRun(name, repoPath, objective string, criteria []string) *CycleRun {
	now := time.Now()
	return &CycleRun{
		ID:              uuid.New().String(),
		Name:            name,
		RepoPath:        repoPath,
		Phase:           CycleProposed,
		Objective:       objective,
		SuccessCriteria: criteria,
		Tasks:           []CycleTask{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

// Advance transitions the cycle to the next phase. Returns an error if the
// transition is invalid. Any phase may transition to CycleFailed via Fail().
func (c *CycleRun) Advance(phase CyclePhase) error {
	if phase == CycleFailed {
		return fmt.Errorf("use Fail() to transition to failed state")
	}
	if c.Phase == CycleComplete || c.Phase == CycleFailed {
		return fmt.Errorf("cannot advance from terminal phase %q", c.Phase)
	}
	next, ok := validTransitions[c.Phase]
	if !ok || next != phase {
		return fmt.Errorf("invalid transition: %s → %s", c.Phase, phase)
	}
	c.Phase = phase
	c.UpdatedAt = time.Now()
	return nil
}

// Fail transitions the cycle to the failed phase from any non-terminal phase.
func (c *CycleRun) Fail(errMsg string) {
	c.Phase = CycleFailed
	c.Error = errMsg
	c.UpdatedAt = time.Now()
}

// AddTask appends a task to the cycle.
func (c *CycleRun) AddTask(task CycleTask) {
	c.Tasks = append(c.Tasks, task)
	c.UpdatedAt = time.Now()
}

// AddFinding appends a finding to the cycle.
func (c *CycleRun) AddFinding(finding CycleFinding) {
	c.Findings = append(c.Findings, finding)
	c.UpdatedAt = time.Now()
}

// SetSynthesis sets the cycle synthesis.
func (c *CycleRun) SetSynthesis(s CycleSynthesis) {
	c.Synthesis = &s
	c.UpdatedAt = time.Now()
}
