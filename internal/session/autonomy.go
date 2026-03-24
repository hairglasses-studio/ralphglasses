package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// AutonomyLevel defines the graduated trust level for autonomous decisions.
type AutonomyLevel int

const (
	// LevelObserve records metrics and logs "would have done" decisions.
	LevelObserve AutonomyLevel = 0
	// LevelAutoRecover enables auto-restart for transient errors and provider failover.
	LevelAutoRecover AutonomyLevel = 1
	// LevelAutoOptimize enables auto-adjusting budgets, providers, and rate limits.
	LevelAutoOptimize AutonomyLevel = 2
	// LevelFullAutonomy enables auto-applying config, launching from roadmap, and scaling teams.
	LevelFullAutonomy AutonomyLevel = 3
)

// String returns the human-readable name for an autonomy level.
func (l AutonomyLevel) String() string {
	switch l {
	case LevelObserve:
		return "observe"
	case LevelAutoRecover:
		return "auto-recover"
	case LevelAutoOptimize:
		return "auto-optimize"
	case LevelFullAutonomy:
		return "full-autonomy"
	default:
		return fmt.Sprintf("unknown(%d)", l)
	}
}

// DecisionCategory groups autonomous decisions.
type DecisionCategory string

const (
	DecisionRestart       DecisionCategory = "restart"
	DecisionFailover      DecisionCategory = "failover"
	DecisionBudgetAdjust  DecisionCategory = "budget_adjust"
	DecisionProviderSelect DecisionCategory = "provider_select"
	DecisionConfigChange  DecisionCategory = "config_change"
	DecisionLaunch        DecisionCategory = "launch"
	DecisionScale         DecisionCategory = "scale"
	DecisionSelfTest       DecisionCategory = "self_test"
	DecisionReflexion      DecisionCategory = "reflexion"
	DecisionEpisodicReplay DecisionCategory = "episodic_replay"
	DecisionCascadeRoute   DecisionCategory = "cascade_route"
	DecisionCurriculum     DecisionCategory = "curriculum"
)

// AutonomousDecision records a decision made (or proposed) by the system.
type AutonomousDecision struct {
	ID            string           `json:"id"`
	Timestamp     time.Time        `json:"ts"`
	Category      DecisionCategory `json:"category"`
	RequiredLevel AutonomyLevel    `json:"required_level"`
	ActualLevel   AutonomyLevel    `json:"actual_level"`
	Executed      bool             `json:"executed"`   // true if action was taken
	Rationale     string           `json:"rationale"`  // why the system made this decision
	Inputs        map[string]any   `json:"inputs"`     // data that informed the decision
	Action        string           `json:"action"`     // what was (or would be) done
	SessionID     string           `json:"session_id,omitempty"`
	RepoName      string           `json:"repo_name,omitempty"`
	Outcome       *DecisionOutcome `json:"outcome,omitempty"`
}

// DecisionOutcome evaluates whether a decision was successful.
type DecisionOutcome struct {
	EvaluatedAt time.Time `json:"evaluated_at"`
	Success     bool      `json:"success"`
	Overridden  bool      `json:"overridden"` // human reversed the decision
	Details     string    `json:"details"`
}

// DecisionLog tracks autonomous decisions with JSONL persistence.
type DecisionLog struct {
	mu        sync.Mutex
	decisions []AutonomousDecision
	level     AutonomyLevel
	blocklist map[DecisionCategory]bool
	stateDir  string
}

// NewDecisionLog creates a decision log with the given autonomy level.
func NewDecisionLog(stateDir string, level AutonomyLevel) *DecisionLog {
	dl := &DecisionLog{
		level:     level,
		blocklist: make(map[DecisionCategory]bool),
		stateDir:  stateDir,
	}
	dl.load()
	return dl
}

// Level returns the current autonomy level.
func (dl *DecisionLog) Level() AutonomyLevel {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	return dl.level
}

// SetLevel changes the autonomy level.
func (dl *DecisionLog) SetLevel(level AutonomyLevel) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.level = level
}

// Block prevents a decision category from being executed.
func (dl *DecisionLog) Block(category DecisionCategory) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.blocklist[category] = true
}

// Unblock removes a decision category block.
func (dl *DecisionLog) Unblock(category DecisionCategory) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	delete(dl.blocklist, category)
}

// Propose records a decision and returns whether it should be executed.
// If the current autonomy level is insufficient or the category is blocked,
// the decision is logged as "would have done" but not executed.
func (dl *DecisionLog) Propose(d AutonomousDecision) bool {
	dl.mu.Lock()
	d.ActualLevel = dl.level
	blocked := dl.blocklist[d.Category]
	allowed := dl.level >= d.RequiredLevel && !blocked
	d.Executed = allowed
	if d.ID == "" {
		d.ID = fmt.Sprintf("dec-%d", time.Now().UnixNano())
	}
	if d.Timestamp.IsZero() {
		d.Timestamp = time.Now()
	}
	dl.decisions = append(dl.decisions, d)
	dl.mu.Unlock()

	dl.appendToFile(d)
	return allowed
}

// RecordOutcome attaches an outcome to a previously logged decision.
func (dl *DecisionLog) RecordOutcome(decisionID string, outcome DecisionOutcome) {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	for i := len(dl.decisions) - 1; i >= 0; i-- {
		if dl.decisions[i].ID == decisionID {
			dl.decisions[i].Outcome = &outcome
			return
		}
	}
}

// Recent returns the last N decisions.
func (dl *DecisionLog) Recent(limit int) []AutonomousDecision {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	if len(dl.decisions) <= limit {
		result := make([]AutonomousDecision, len(dl.decisions))
		copy(result, dl.decisions)
		return result
	}
	result := make([]AutonomousDecision, limit)
	copy(result, dl.decisions[len(dl.decisions)-limit:])
	return result
}

// Stats returns summary statistics about decisions.
func (dl *DecisionLog) Stats() map[string]any {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	total := len(dl.decisions)
	executed := 0
	succeeded := 0
	overridden := 0

	for _, d := range dl.decisions {
		if d.Executed {
			executed++
		}
		if d.Outcome != nil {
			if d.Outcome.Success {
				succeeded++
			}
			if d.Outcome.Overridden {
				overridden++
			}
		}
	}

	return map[string]any{
		"total_decisions":  total,
		"executed":         executed,
		"would_have_done":  total - executed,
		"succeeded":        succeeded,
		"overridden":       overridden,
		"current_level":    dl.level,
		"level_name":       dl.level.String(),
	}
}

func (dl *DecisionLog) appendToFile(d AutonomousDecision) {
	if dl.stateDir == "" {
		return
	}
	_ = os.MkdirAll(dl.stateDir, 0755)

	data, err := json.Marshal(d)
	if err != nil {
		return
	}
	data = append(data, '\n')

	path := filepath.Join(dl.stateDir, "decisions.jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return
	}
}

func (dl *DecisionLog) load() {
	if dl.stateDir == "" {
		return
	}
	path := filepath.Join(dl.stateDir, "decisions.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}

	var decisions []AutonomousDecision
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var d AutonomousDecision
		if json.Unmarshal(line, &d) == nil {
			decisions = append(decisions, d)
		}
	}

	dl.mu.Lock()
	dl.decisions = decisions
	dl.mu.Unlock()
}
