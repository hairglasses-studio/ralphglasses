package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CascadeConfig configures the cheap-then-expensive routing strategy.
type CascadeConfig struct {
	CheapProvider       Provider            `json:"cheap_provider"`
	ExpensiveProvider   Provider            `json:"expensive_provider"`
	ConfidenceThreshold float64             `json:"confidence_threshold"` // 0.0-1.0, default 0.7
	MaxCheapBudgetUSD   float64             `json:"max_cheap_budget_usd"`
	MaxCheapTurns       int                 `json:"max_cheap_turns"`
	TaskTypeOverrides   map[string]Provider `json:"task_type_overrides"`
}

// DefaultCascadeConfig returns sensible defaults.
func DefaultCascadeConfig() CascadeConfig {
	return CascadeConfig{
		CheapProvider:       ProviderGemini,
		ExpensiveProvider:   ProviderClaude,
		ConfidenceThreshold: 0.7,
		MaxCheapBudgetUSD:   2.00,
		MaxCheapTurns:       15,
		TaskTypeOverrides:   make(map[string]Provider),
	}
}

// CascadeResult records the outcome of a cascade routing decision.
type CascadeResult struct {
	Timestamp       time.Time `json:"ts"`
	TaskType        string    `json:"task_type"`
	TaskTitle       string    `json:"task_title"`
	UsedProvider    Provider  `json:"used_provider"`
	Escalated       bool      `json:"escalated"`
	CheapConfidence float64   `json:"cheap_confidence"`
	CheapCostUSD    float64   `json:"cheap_cost_usd"`
	TotalCostUSD    float64   `json:"total_cost_usd"`
	Reason          string    `json:"escalation_reason"` // "low_confidence", "verify_failed", "error", ""
}

// CascadeStats summarizes cascade routing outcomes.
type CascadeStats struct {
	TotalDecisions int     `json:"total_decisions"`
	Escalations    int     `json:"escalations"`
	EscalationRate float64 `json:"escalation_rate"`
	CostSavedUSD   float64 `json:"cost_saved_usd"` // sum of cheap costs for non-escalated
	AvgCheapCost   float64 `json:"avg_cheap_cost"`
}

// ModelTier represents a model with its cost and capability profile.
type ModelTier struct {
	Provider      Provider `json:"provider"`
	Model         string   `json:"model"`
	MaxComplexity int      `json:"max_complexity"` // 1-4 scale
	CostPer1M     float64  `json:"cost_per_1m"`    // input cost per 1M tokens
	Label         string   `json:"label"`           // e.g. "ultra-cheap", "worker", "coding", "reasoning"
}

// DefaultModelTiers returns the built-in tier list ordered by cost.
func DefaultModelTiers() []ModelTier {
	return []ModelTier{
		{Provider: ProviderGemini, Model: "gemini-2.0-flash-lite", MaxComplexity: 1, CostPer1M: CostGeminiFlashLiteInput, Label: "ultra-cheap"},
		{Provider: ProviderGemini, Model: "gemini-2.5-flash", MaxComplexity: 2, CostPer1M: CostGeminiFlashInput, Label: "worker"},
		{Provider: ProviderClaude, Model: "claude-sonnet", MaxComplexity: 3, CostPer1M: CostClaudeSonnetInput, Label: "coding"},
		{Provider: ProviderClaude, Model: "claude-opus", MaxComplexity: 4, CostPer1M: CostClaudeOpusInput, Label: "reasoning"},
	}
}

// taskTypeComplexity maps well-known task types to their complexity level (1-4).
var taskTypeComplexity = map[string]int{
	"lint":         1,
	"format":       1,
	"classify":     1,
	"codegen":      3,
	"test":         3,
	"architecture": 4,
	"analysis":     4,
	"planning":     4,
}

// TaskTypeComplexity returns the complexity for a known task type, or 0 if unknown.
func TaskTypeComplexity(taskType string) int {
	return taskTypeComplexity[taskType]
}

// CascadeRouter implements try-cheap-then-escalate provider routing.
type CascadeRouter struct {
	mu        sync.Mutex
	config    CascadeConfig
	feedback  *FeedbackAnalyzer
	decisions *DecisionLog
	results   []CascadeResult
	stateDir  string
	tiers     []ModelTier
}

// NewCascadeRouter creates a cascade router, loading any persisted results.
func NewCascadeRouter(config CascadeConfig, feedback *FeedbackAnalyzer, decisions *DecisionLog, stateDir string) *CascadeRouter {
	cr := &CascadeRouter{
		config:    config,
		feedback:  feedback,
		decisions: decisions,
		stateDir:  stateDir,
		tiers:     DefaultModelTiers(),
	}
	cr.loadResults()
	return cr
}

// ShouldCascade returns true if the task should attempt cheap-first routing.
// Returns false if the task type has an override or if the cheap provider
// is already proven reliable for this task type.
func (cr *CascadeRouter) ShouldCascade(taskType string, prompt string) bool {
	// If task type has an override, skip cascading — use the override directly
	if _, ok := cr.config.TaskTypeOverrides[taskType]; ok {
		return false
	}

	// No feedback data available — default to cascading
	if cr.feedback == nil {
		return true
	}

	// Check if the cheap provider is already reliable for this task type
	profile, ok := cr.feedback.GetProviderProfile(string(cr.config.CheapProvider), taskType)
	if ok && profile.CompletionRate > 90 && profile.SampleCount >= 5 {
		return false
	}

	return true
}

// ResolveProvider returns the provider to use for a given task type.
// If there is an override, returns the override. If cheap is reliable,
// returns cheap. Otherwise returns expensive (caller should use cascade logic).
func (cr *CascadeRouter) ResolveProvider(taskType string) Provider {
	// Check for task type override
	if override, ok := cr.config.TaskTypeOverrides[taskType]; ok {
		return override
	}

	// If cascading is not needed (cheap is reliable), use cheap directly
	if !cr.ShouldCascade(taskType, "") {
		return cr.config.CheapProvider
	}

	// Default to expensive — caller will use cascade logic to try cheap first
	return cr.config.ExpensiveProvider
}

// SetTiers replaces the default model tiers with a custom list.
func (cr *CascadeRouter) SetTiers(tiers []ModelTier) {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	cr.tiers = tiers
}

// Tiers returns the current model tier list.
func (cr *CascadeRouter) Tiers() []ModelTier {
	cr.mu.Lock()
	defer cr.mu.Unlock()
	out := make([]ModelTier, len(cr.tiers))
	copy(out, cr.tiers)
	return out
}

// SelectTier picks the cheapest model tier that can handle the given complexity.
// If taskType is recognized, its mapped complexity is used (the complexity arg
// is ignored). If taskType is unrecognized and complexity <= 0, the highest tier
// is returned. Returns an empty ModelTier if no tiers are configured.
func (cr *CascadeRouter) SelectTier(taskType string, complexity int) ModelTier {
	cr.mu.Lock()
	tiers := cr.tiers
	cr.mu.Unlock()

	if len(tiers) == 0 {
		return ModelTier{}
	}

	// Use task-type mapping if available; otherwise use the provided complexity.
	if mapped, ok := taskTypeComplexity[taskType]; ok {
		complexity = mapped
	}

	// If complexity is still unknown, default to highest tier.
	if complexity <= 0 {
		return tiers[len(tiers)-1]
	}

	// Find cheapest tier that can handle the complexity.
	// Tiers from DefaultModelTiers are sorted by cost ascending, but
	// callers can set custom tiers, so we scan all and pick the cheapest match.
	var best *ModelTier
	for i := range tiers {
		if tiers[i].MaxComplexity >= complexity {
			if best == nil || tiers[i].CostPer1M < best.CostPer1M {
				best = &tiers[i]
			}
		}
	}

	if best != nil {
		return *best
	}

	// No tier can handle the complexity — return the most capable tier.
	highest := tiers[0]
	for _, t := range tiers[1:] {
		if t.MaxComplexity > highest.MaxComplexity {
			highest = t
		}
	}
	return highest
}

// CheapLaunchOpts returns launch options modified for the cheap provider.
func (cr *CascadeRouter) CheapLaunchOpts(base LaunchOptions) LaunchOptions {
	opts := base
	opts.Provider = cr.config.CheapProvider

	if cr.config.MaxCheapBudgetUSD > 0 {
		opts.MaxBudgetUSD = cr.config.MaxCheapBudgetUSD
	}
	if cr.config.MaxCheapTurns > 0 {
		opts.MaxTurns = cr.config.MaxCheapTurns
	}

	opts.SessionName = opts.SessionName + "-cheap"

	return opts
}

// EvaluateCheapResult examines a completed cheap session and decides whether
// to escalate to the expensive provider.
func (cr *CascadeRouter) EvaluateCheapResult(s *Session, expectedTurns int, verification []LoopVerification) (escalate bool, confidence float64, reason string) {
	s.Lock()
	errMsg := s.Error
	turnCount := s.TurnCount
	lastOutput := s.LastOutput
	s.Unlock()

	// Session errored out
	if errMsg != "" {
		return true, 0, "error"
	}

	// Check verification results
	if len(verification) > 0 {
		allPassed := true
		for _, v := range verification {
			if v.ExitCode != 0 {
				allPassed = false
				break
			}
		}
		if !allPassed {
			return true, computeConfidence(turnCount, expectedTurns, lastOutput, false), "verify_failed"
		}
	}

	// Compute confidence score
	conf := computeConfidence(turnCount, expectedTurns, lastOutput, true)
	if conf < cr.config.ConfidenceThreshold {
		return true, conf, "low_confidence"
	}

	return false, conf, ""
}

// computeConfidence produces a 0.0-1.0 confidence score from session signals.
func computeConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64 {
	score := 0.0
	components := 0

	// Turn efficiency: did we finish within expected turns?
	if expectedTurns > 0 && turnCount > 0 {
		components++
		ratio := float64(turnCount) / float64(expectedTurns)
		if ratio <= 1.0 {
			score += 1.0 // finished within budget
		} else if ratio <= 2.0 {
			score += 0.5
		}
		// > 2x expected turns → 0 contribution
	}

	// Verification passed
	components++
	if verifyPassed {
		score += 1.0
	}

	// Hedging language in output (indicates uncertainty)
	components++
	hedgeScore := 1.0
	if lastOutput != "" {
		lower := strings.ToLower(lastOutput)
		hedgeWords := []string{"i'm not sure", "might not", "possibly", "uncertain", "i think", "maybe", "not confident"}
		hedgeCount := 0
		for _, hw := range hedgeWords {
			if strings.Contains(lower, hw) {
				hedgeCount++
			}
		}
		if hedgeCount >= 3 {
			hedgeScore = 0.0
		} else if hedgeCount >= 1 {
			hedgeScore = 0.5
		}
	}
	score += hedgeScore

	// Error-free output
	components++
	if lastOutput != "" {
		lower := strings.ToLower(lastOutput)
		if strings.Contains(lower, "error:") || strings.Contains(lower, "failed:") || strings.Contains(lower, "panic:") {
			// error signals in output
		} else {
			score += 1.0
		}
	} else {
		score += 1.0 // no output to check, assume ok
	}

	if components == 0 {
		return 0.0
	}
	return score / float64(components)
}

// RecordResult persists a cascade routing outcome.
func (cr *CascadeRouter) RecordResult(result CascadeResult) {
	cr.mu.Lock()
	cr.results = append(cr.results, result)
	cr.mu.Unlock()

	cr.appendResult(result)
}

// Stats computes summary statistics from all cascade results.
func (cr *CascadeRouter) Stats() CascadeStats {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	stats := CascadeStats{
		TotalDecisions: len(cr.results),
	}

	if len(cr.results) == 0 {
		return stats
	}

	var totalCheapCost float64
	for _, r := range cr.results {
		totalCheapCost += r.CheapCostUSD
		if r.Escalated {
			stats.Escalations++
		} else {
			stats.CostSavedUSD += r.CheapCostUSD
		}
	}

	stats.EscalationRate = float64(stats.Escalations) / float64(stats.TotalDecisions)
	stats.AvgCheapCost = totalCheapCost / float64(stats.TotalDecisions)

	return stats
}

// RecentResults returns the last N cascade results.
func (cr *CascadeRouter) RecentResults(limit int) []CascadeResult {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	if len(cr.results) <= limit {
		out := make([]CascadeResult, len(cr.results))
		copy(out, cr.results)
		return out
	}
	out := make([]CascadeResult, limit)
	copy(out, cr.results[len(cr.results)-limit:])
	return out
}

func (cr *CascadeRouter) resultsPath() string {
	return filepath.Join(cr.stateDir, "cascade_results.jsonl")
}

func (cr *CascadeRouter) appendResult(r CascadeResult) {
	if cr.stateDir == "" {
		return
	}
	_ = os.MkdirAll(cr.stateDir, 0755)

	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(cr.resultsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(data)
}

func (cr *CascadeRouter) loadResults() {
	if cr.stateDir == "" {
		return
	}
	data, err := os.ReadFile(cr.resultsPath())
	if err != nil {
		return
	}

	var results []CascadeResult
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var r CascadeResult
		if json.Unmarshal(line, &r) == nil {
			results = append(results, r)
		}
	}

	cr.mu.Lock()
	cr.results = results
	cr.mu.Unlock()
}

// logDecision records a cascade routing decision in the autonomy decision log.
func (cr *CascadeRouter) logDecision(taskType, action, rationale string, inputs map[string]any) bool {
	if cr.decisions == nil {
		return true // no decision log, allow everything
	}
	return cr.decisions.Propose(AutonomousDecision{
		Timestamp:     time.Now(),
		Category:      DecisionCascadeRoute,
		RequiredLevel: LevelAutoOptimize,
		Rationale:     rationale,
		Action:        action,
		Inputs:        inputs,
	})
}

// cascadeResultsFile is the file name for persisted cascade results.
const cascadeResultsFile = "cascade_results.jsonl"

func init() {
	// Ensure cascadeResultsFile matches resultsPath logic.
	_ = cascadeResultsFile
	_ = fmt.Sprintf // ensure fmt is used
}
