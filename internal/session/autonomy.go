package session

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// AutonomyConfig holds bootstrapped autonomy settings parsed from .ralphrc.
type AutonomyConfig struct {
	Level         AutonomyLevel // 0=observe, 1=auto-recover (max for bootstrap)
	AutoRecover   bool          // whether auto-recovery is enabled
	MaxRecoveries int           // max auto-recoveries per loop before requiring manual intervention
}

// BootstrapAutonomy reads autonomy settings from a .ralphrc config map and
// returns an AutonomyConfig. Only levels 0 (observe) and 1 (auto-recover)
// are supported during bootstrapping; higher values are clamped to 1.
func BootstrapAutonomy(cfg map[string]string) *AutonomyConfig {
	ac := &AutonomyConfig{
		Level:         LevelObserve,
		AutoRecover:   false,
		MaxRecoveries: 3,
	}

	if v, ok := cfg["AUTONOMY_LEVEL"]; ok {
		if n, err := strconv.Atoi(v); err == nil {
			if n > int(LevelAutoRecover) {
				n = int(LevelAutoRecover) // clamp to level 1 during bootstrap
			}
			if n < 0 {
				n = 0
			}
			ac.Level = AutonomyLevel(n)
		}
	}

	if v, ok := cfg["AUTONOMY_AUTO_RECOVER"]; ok {
		switch strings.ToLower(v) {
		case "true", "1", "yes":
			ac.AutoRecover = true
		}
	}

	if v, ok := cfg["AUTONOMY_AUTO_RECOVER_MAX"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ac.MaxRecoveries = n
		}
	}

	return ac
}

// ShouldRecover returns true if autonomy config allows auto-recovery for a loop
// that has failed recoveryCount times already.
func (ac *AutonomyConfig) ShouldRecover(recoveryCount int) bool {
	return ac.Level >= LevelAutoRecover && ac.AutoRecover && recoveryCount < ac.MaxRecoveries
}

// RecoveryBackoff returns the backoff duration for the given recovery attempt
// (0-indexed). The schedule is 30s, 60s, 120s, etc. (exponential with base 30s, factor 2).
func RecoveryBackoff(attempt int) time.Duration {
	base := 30 * time.Second
	for range attempt {
		base *= 2
	}
	return base
}

// DecisionCategory groups autonomous decisions.
type DecisionCategory string

const (
	DecisionRestart        DecisionCategory = "restart"
	DecisionFailover       DecisionCategory = "failover"
	DecisionBudgetAdjust   DecisionCategory = "budget_adjust"
	DecisionProviderSelect DecisionCategory = "provider_select"
	DecisionConfigChange   DecisionCategory = "config_change"
	DecisionLaunch         DecisionCategory = "launch"
	DecisionScale          DecisionCategory = "scale"
	DecisionSelfTest       DecisionCategory = "self_test"
	DecisionReflexion      DecisionCategory = "reflexion"
	DecisionEpisodicReplay DecisionCategory = "episodic_replay"
	DecisionCascadeRoute   DecisionCategory = "cascade_route"
	DecisionCurriculum     DecisionCategory = "curriculum"
)

// AutonomousDecision records a decision made (or proposed) by the system.
type AutonomousDecision struct {
	ID             string           `json:"id"`
	Timestamp      time.Time        `json:"ts"`
	Category       DecisionCategory `json:"category"`
	RequiredLevel  AutonomyLevel    `json:"required_level"`
	ActualLevel    AutonomyLevel    `json:"actual_level"`
	Executed       bool             `json:"executed"`  // true if action was taken
	Rationale      string           `json:"rationale"` // why the system made this decision
	Inputs         map[string]any   `json:"inputs"`    // data that informed the decision
	Action         string           `json:"action"`    // what was (or would be) done
	PolicySource   string           `json:"policy_source,omitempty"`
	RollbackHint   string           `json:"rollback_hint,omitempty"`
	UndoHandle     string           `json:"undo_handle,omitempty"`
	RiskTags       []string         `json:"risk_tags,omitempty"`
	Counterfactual string           `json:"counterfactual,omitempty"`
	SessionID      string           `json:"session_id,omitempty"`
	RepoName       string           `json:"repo_name,omitempty"`
	Outcome        *DecisionOutcome `json:"outcome,omitempty"`
}

// DecisionOutcome evaluates whether a decision was successful.
type DecisionOutcome struct {
	EvaluatedAt time.Time `json:"evaluated_at"`
	Success     bool      `json:"success"`
	Overridden  bool      `json:"overridden"` // human reversed the decision
	Details     string    `json:"details"`
}

// AutonomousDecisionSummary is a compact decision view for MCP responses.
type AutonomousDecisionSummary struct {
	ID             string           `json:"id"`
	Timestamp      time.Time        `json:"ts"`
	Category       DecisionCategory `json:"category"`
	Executed       bool             `json:"executed"`
	RequiredLevel  AutonomyLevel    `json:"required_level"`
	ActualLevel    AutonomyLevel    `json:"actual_level"`
	Action         string           `json:"action"`
	Rationale      string           `json:"rationale"`
	PolicySource   string           `json:"policy_source,omitempty"`
	RollbackHint   string           `json:"rollback_hint,omitempty"`
	UndoHandle     string           `json:"undo_handle,omitempty"`
	RiskTags       []string         `json:"risk_tags,omitempty"`
	Counterfactual string           `json:"counterfactual,omitempty"`
	SessionID      string           `json:"session_id,omitempty"`
	RepoName       string           `json:"repo_name,omitempty"`
	Outcome        *DecisionOutcome `json:"outcome,omitempty"`
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

// RestoreLevel sets the autonomy level without persisting to disk.
// Intended for loading persisted state on startup.
func (dl *DecisionLog) RestoreLevel(level AutonomyLevel) {
	dl.mu.Lock()
	defer dl.mu.Unlock()
	dl.level = level
}

// SetLevel changes the autonomy level and persists it to disk.
func (dl *DecisionLog) SetLevel(level AutonomyLevel) {
	dl.mu.Lock()
	dl.level = level
	stateDir := dl.stateDir
	dl.mu.Unlock()
	if stateDir != "" {
		if err := SaveAutonomyLevel(stateDir, int(level)); err != nil {
			slog.Error("failed to persist autonomy level", "level", level, "error", err)
			// Retry once after 100ms before giving up.
			time.Sleep(100 * time.Millisecond)
			if retryErr := SaveAutonomyLevel(stateDir, int(level)); retryErr != nil {
				slog.Error("failed to persist autonomy level after retry", "level", level, "error", retryErr)
			}
		}
	}
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
	dl.decorateDecision(&d, blocked)
	dl.decisions = append(dl.decisions, d)
	dl.mu.Unlock()

	dl.appendToFile(d)
	return allowed
}

func (dl *DecisionLog) decorateDecision(d *AutonomousDecision, blocked bool) {
	if d.PolicySource == "" {
		d.PolicySource = defaultDecisionPolicySource(d.Executed, blocked)
	}
	if len(d.RiskTags) == 0 {
		d.RiskTags = defaultDecisionRiskTags(d.Category)
	}
	if d.RollbackHint == "" {
		d.RollbackHint = defaultRollbackHint(d.Category)
	}
	if d.UndoHandle == "" {
		d.UndoHandle = defaultUndoHandle(*d)
	}
	if !d.Executed && d.Counterfactual == "" {
		d.Counterfactual = defaultCounterfactual(*d, blocked)
	}
}

func defaultDecisionPolicySource(executed bool, blocked bool) string {
	switch {
	case blocked:
		return "decision-log:blocklist"
	case executed:
		return "decision-log:autonomy-level"
	default:
		return "decision-log:insufficient-level"
	}
}

func defaultDecisionRiskTags(category DecisionCategory) []string {
	switch category {
	case DecisionRestart, DecisionFailover:
		return []string{"availability", "runtime"}
	case DecisionBudgetAdjust:
		return []string{"cost", "budget"}
	case DecisionProviderSelect, DecisionCascadeRoute:
		return []string{"routing", "provider"}
	case DecisionConfigChange:
		return []string{"config", "workspace"}
	case DecisionLaunch, DecisionScale:
		return []string{"execution", "capacity"}
	case DecisionSelfTest:
		return []string{"verification"}
	case DecisionReflexion, DecisionEpisodicReplay, DecisionCurriculum:
		return []string{"learning", "state"}
	default:
		return []string{"operational"}
	}
}

func defaultRollbackHint(category DecisionCategory) string {
	switch category {
	case DecisionRestart:
		return "Restart the previously healthy session or restore the prior runtime process."
	case DecisionFailover:
		return "Switch traffic back to the prior provider and replay the interrupted work."
	case DecisionBudgetAdjust:
		return "Restore the previous budget or rate-limit configuration."
	case DecisionProviderSelect, DecisionCascadeRoute:
		return "Route the next task back through the prior provider/model pair."
	case DecisionConfigChange:
		return "Revert the config change or restore the previous config file values."
	case DecisionLaunch:
		return "Terminate the launched session or revert the launched worktree changes."
	case DecisionScale:
		return "Scale worker concurrency back to the previous level."
	case DecisionSelfTest:
		return "No rollback normally required; rerun validation after restoring the prior state."
	case DecisionReflexion, DecisionEpisodicReplay, DecisionCurriculum:
		return "Clear or revert the derived learning state before re-running."
	default:
		return "Review the affected state and restore the prior known-good values."
	}
}

func defaultUndoHandle(d AutonomousDecision) string {
	switch {
	case d.SessionID != "":
		return "session:" + d.SessionID
	case d.RepoName != "":
		return "repo:" + d.RepoName
	default:
		return "decision:" + d.ID
	}
}

func defaultCounterfactual(d AutonomousDecision, blocked bool) string {
	if blocked {
		return fmt.Sprintf("Skipped because %s is blocklisted at autonomy level %s.", d.Category, d.ActualLevel.String())
	}
	return fmt.Sprintf("Would execute at %s or higher; current autonomy level is %s.", d.RequiredLevel.String(), d.ActualLevel.String())
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

// RecentSummaries returns compact summaries for the last N decisions.
func (dl *DecisionLog) RecentSummaries(limit int) []AutonomousDecisionSummary {
	decisions := dl.Recent(limit)
	summaries := make([]AutonomousDecisionSummary, len(decisions))
	for i, d := range decisions {
		summaries[i] = SummarizeDecision(d)
	}
	return summaries
}

// Snapshot returns a compact decision-log snapshot for status surfaces.
func (dl *DecisionLog) Snapshot(limit int) map[string]any {
	return map[string]any{
		"level":      int(dl.Level()),
		"level_name": dl.Level().String(),
		"stats":      dl.Stats(),
		"recent":     dl.RecentSummaries(limit),
	}
}

// SummarizeDecision returns a compact, MCP-friendly view of a decision.
func SummarizeDecision(d AutonomousDecision) AutonomousDecisionSummary {
	return AutonomousDecisionSummary{
		ID:             d.ID,
		Timestamp:      d.Timestamp,
		Category:       d.Category,
		Executed:       d.Executed,
		RequiredLevel:  d.RequiredLevel,
		ActualLevel:    d.ActualLevel,
		Action:         d.Action,
		Rationale:      d.Rationale,
		PolicySource:   d.PolicySource,
		RollbackHint:   d.RollbackHint,
		UndoHandle:     d.UndoHandle,
		RiskTags:       append([]string(nil), d.RiskTags...),
		Counterfactual: d.Counterfactual,
		SessionID:      d.SessionID,
		RepoName:       d.RepoName,
		Outcome:        d.Outcome,
	}
}

// Stats returns summary statistics about decisions.
func (dl *DecisionLog) Stats() map[string]any {
	dl.mu.Lock()
	defer dl.mu.Unlock()

	total := len(dl.decisions)
	executed := 0
	succeeded := 0
	overridden := 0
	byPolicySource := make(map[string]int)
	byRiskTag := make(map[string]int)

	for _, d := range dl.decisions {
		if d.Executed {
			executed++
		}
		if d.PolicySource != "" {
			byPolicySource[d.PolicySource]++
		}
		for _, tag := range d.RiskTags {
			byRiskTag[tag]++
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
		"by_policy_source": byPolicySource,
		"by_risk_tag":      byRiskTag,
	}
}

func (dl *DecisionLog) appendToFile(d AutonomousDecision) {
	if dl.stateDir == "" {
		return
	}
	if err := os.MkdirAll(dl.stateDir, 0755); err != nil {
		slog.Warn("failed to create decision log state dir", "dir", dl.stateDir, "error", err)
		return
	}

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
