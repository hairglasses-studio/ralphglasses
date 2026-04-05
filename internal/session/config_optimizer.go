package session

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"time"
)

// ConfigOptimizer analyzes session outcomes (success rate, cost, duration)
// and suggests configuration changes using bandit-style exploration.
// It tracks per-config-arm performance and balances exploitation of known-good
// configs with exploration of alternatives.
type ConfigOptimizer struct {
	mu   sync.Mutex
	arms map[string]*ConfigArm // keyed by arm ID (e.g. "claude:feature", "gemini:lint")

	// exploration controls the epsilon-greedy exploration rate (0.0-1.0).
	// Higher values explore more; lower values exploit known-good configs.
	exploration float64

	// minTrials is the minimum number of trials per arm before it is
	// considered for exploitation. Below this, the arm is always explored.
	minTrials int

	// windowSize is the sliding window size used when creating new arms.
	windowSize int

	// suggestions accumulates pending suggestions until consumed.
	suggestions []ConfigSuggestion

	// rng is the random source for exploration decisions.
	rng *rand.Rand
}

// ConfigArm tracks the performance of a specific configuration choice
// (provider + task type combination). Uses a windowed average to adapt
// to changing conditions.
type ConfigArm struct {
	ID           string    `json:"id"`
	Provider     string    `json:"provider"`
	TaskType     string    `json:"task_type"`
	Trials       int       `json:"trials"`
	Successes    int       `json:"successes"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	TotalDurSec  float64   `json:"total_duration_sec"`
	LastUsed     time.Time `json:"last_used"`

	// Windowed stats for recency-weighted decisions.
	recentOutcomes []configOutcome
	windowSize     int
}

// configOutcome records a single trial result.
type configOutcome struct {
	Success  bool    `json:"success"`
	CostUSD  float64 `json:"cost_usd"`
	DurSec   float64 `json:"duration_sec"`
	RecordAt time.Time `json:"recorded_at"`
}

// ConfigSuggestion is a recommended configuration change.
type ConfigSuggestion struct {
	ID          string    `json:"id"`
	Timestamp   time.Time `json:"timestamp"`
	Category    string    `json:"category"` // "provider", "concurrency", "budget"
	TaskType    string    `json:"task_type"`
	Current     string    `json:"current"`
	Suggested   string    `json:"suggested"`
	Rationale   string    `json:"rationale"`
	Confidence  float64   `json:"confidence"` // 0.0-1.0
	ExpectedGain float64  `json:"expected_gain"` // estimated improvement fraction
}

// ConfigOptimizerConfig holds tuning parameters.
type ConfigOptimizerConfig struct {
	Exploration float64 `json:"exploration"`  // epsilon for exploration (default 0.15)
	MinTrials   int     `json:"min_trials"`   // minimum trials before exploitation (default 5)
	WindowSize  int     `json:"window_size"`  // sliding window for recency weighting (default 20)
}

// DefaultConfigOptimizerConfig returns sensible defaults.
func DefaultConfigOptimizerConfig() ConfigOptimizerConfig {
	return ConfigOptimizerConfig{
		Exploration: 0.15,
		MinTrials:   5,
		WindowSize:  20,
	}
}

// NewConfigOptimizer creates a config optimizer with the given settings.
func NewConfigOptimizer(cfg ConfigOptimizerConfig) *ConfigOptimizer {
	if cfg.Exploration <= 0 {
		cfg.Exploration = 0.15
	}
	if cfg.MinTrials <= 0 {
		cfg.MinTrials = 5
	}
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 20
	}
	return &ConfigOptimizer{
		arms:        make(map[string]*ConfigArm),
		exploration: cfg.Exploration,
		minTrials:   cfg.MinTrials,
		windowSize:  cfg.WindowSize,
		rng:         rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// armKey builds a composite key for provider + task type.
func armKey(provider, taskType string) string {
	return provider + ":" + taskType
}

// RecordOutcome records the result of a session with the given configuration.
func (co *ConfigOptimizer) RecordOutcome(provider, taskType string, success bool, costUSD, durationSec float64) {
	co.mu.Lock()
	defer co.mu.Unlock()

	key := armKey(provider, taskType)
	arm, ok := co.arms[key]
	if !ok {
		arm = &ConfigArm{
			ID:         key,
			Provider:   provider,
			TaskType:   taskType,
			windowSize: co.windowSize,
		}
		co.arms[key] = arm
	}

	arm.Trials++
	if success {
		arm.Successes++
	}
	arm.TotalCostUSD += costUSD
	arm.TotalDurSec += durationSec
	arm.LastUsed = time.Now()

	outcome := configOutcome{
		Success:  success,
		CostUSD:  costUSD,
		DurSec:   durationSec,
		RecordAt: time.Now(),
	}
	arm.recentOutcomes = append(arm.recentOutcomes, outcome)
	if len(arm.recentOutcomes) > arm.windowSize {
		arm.recentOutcomes = arm.recentOutcomes[len(arm.recentOutcomes)-arm.windowSize:]
	}
}

// SelectProvider picks the best provider for the given task type using
// epsilon-greedy exploration. Returns the selected provider and whether
// it was an exploration choice.
func (co *ConfigOptimizer) SelectProvider(taskType string, candidates []string) (string, bool) {
	co.mu.Lock()
	defer co.mu.Unlock()

	if len(candidates) == 0 {
		return "", false
	}
	if len(candidates) == 1 {
		return candidates[0], false
	}

	// Epsilon-greedy: explore with probability epsilon.
	if co.rng.Float64() < co.exploration {
		idx := co.rng.Intn(len(candidates))
		return candidates[idx], true
	}

	// Exploit: pick the candidate with the best composite score.
	bestProvider := candidates[0]
	bestScore := -1.0

	for _, provider := range candidates {
		key := armKey(provider, taskType)
		arm, ok := co.arms[key]
		if !ok || arm.Trials < co.minTrials {
			// Insufficient data — treat as exploratory.
			return provider, true
		}
		score := co.armScore(arm)
		if score > bestScore {
			bestScore = score
			bestProvider = provider
		}
	}

	return bestProvider, false
}

// armScore computes a composite score from windowed outcomes.
// Balances success rate (60%), cost efficiency (25%), and speed (15%).
func (co *ConfigOptimizer) armScore(arm *ConfigArm) float64 {
	outcomes := arm.recentOutcomes
	if len(outcomes) == 0 {
		if arm.Trials == 0 {
			return 0.5
		}
		successRate := float64(arm.Successes) / float64(arm.Trials)
		return successRate
	}

	var successes int
	var totalCost, totalDur float64
	for _, o := range outcomes {
		if o.Success {
			successes++
		}
		totalCost += o.CostUSD
		totalDur += o.DurSec
	}
	n := float64(len(outcomes))
	successRate := float64(successes) / n
	avgCost := totalCost / n
	avgDur := totalDur / n

	// Cost efficiency: cheaper is better. Soft threshold at $1.
	costScore := 1.0 / (1.0 + avgCost)

	// Duration efficiency: faster is better. Soft threshold at 60s.
	durScore := 1.0 / (1.0 + avgDur/60.0)

	return 0.60*successRate + 0.25*costScore + 0.15*durScore
}

// SuggestChanges analyzes all arms and generates config change suggestions.
// Call periodically (e.g. after every N sessions) to get optimization hints.
func (co *ConfigOptimizer) SuggestChanges() []ConfigSuggestion {
	co.mu.Lock()
	defer co.mu.Unlock()

	var suggestions []ConfigSuggestion

	// Group arms by task type to find cross-provider comparisons.
	byTask := make(map[string][]*ConfigArm)
	for _, arm := range co.arms {
		byTask[arm.TaskType] = append(byTask[arm.TaskType], arm)
	}

	for taskType, arms := range byTask {
		if len(arms) < 2 {
			continue
		}

		// Find best and current (most-used) arm.
		var bestArm, mostUsedArm *ConfigArm
		bestScore := -1.0
		mostTrials := 0

		for _, arm := range arms {
			if arm.Trials < co.minTrials {
				continue
			}
			score := co.armScore(arm)
			if score > bestScore {
				bestScore = score
				bestArm = arm
			}
			if arm.Trials > mostTrials || (arm.Trials == mostTrials && mostUsedArm != nil && co.armScore(arm) < co.armScore(mostUsedArm)) {
				mostTrials = arm.Trials
				mostUsedArm = arm
			}
		}

		if bestArm == nil || mostUsedArm == nil || bestArm.ID == mostUsedArm.ID {
			continue
		}

		currentScore := co.armScore(mostUsedArm)
		gain := bestScore - currentScore
		if gain < 0.05 {
			continue // not significant enough
		}

		suggestions = append(suggestions, ConfigSuggestion{
			ID:        fmt.Sprintf("cfg-%s-%d", taskType, time.Now().UnixNano()),
			Timestamp: time.Now(),
			Category:  "provider",
			TaskType:  taskType,
			Current:   mostUsedArm.Provider,
			Suggested: bestArm.Provider,
			Rationale: fmt.Sprintf(
				"%s outperforms %s for %s tasks: score %.2f vs %.2f (%d trials each)",
				bestArm.Provider, mostUsedArm.Provider, taskType,
				bestScore, currentScore, bestArm.Trials,
			),
			Confidence:  math.Min(1.0, float64(bestArm.Trials)/20.0),
			ExpectedGain: gain,
		})

		// Budget suggestion: if the best arm is significantly cheaper.
		if bestArm.TotalCostUSD > 0 && mostUsedArm.TotalCostUSD > 0 {
			bestAvgCost := bestArm.TotalCostUSD / float64(bestArm.Trials)
			currentAvgCost := mostUsedArm.TotalCostUSD / float64(mostUsedArm.Trials)
			if bestAvgCost < currentAvgCost*0.7 {
				suggestions = append(suggestions, ConfigSuggestion{
					ID:        fmt.Sprintf("budget-%s-%d", taskType, time.Now().UnixNano()),
					Timestamp: time.Now(),
					Category:  "budget",
					TaskType:  taskType,
					Current:   fmt.Sprintf("$%.2f avg", currentAvgCost),
					Suggested: fmt.Sprintf("$%.2f avg with %s", bestAvgCost, bestArm.Provider),
					Rationale: fmt.Sprintf(
						"switching to %s for %s saves %.0f%% on average cost",
						bestArm.Provider, taskType,
						(1.0-bestAvgCost/currentAvgCost)*100,
					),
					Confidence:  math.Min(1.0, float64(bestArm.Trials)/20.0),
					ExpectedGain: (currentAvgCost - bestAvgCost) / currentAvgCost,
				})
			}
		}
	}

	co.suggestions = append(co.suggestions, suggestions...)
	return suggestions
}

// PendingSuggestions returns and clears accumulated suggestions.
func (co *ConfigOptimizer) PendingSuggestions() []ConfigSuggestion {
	co.mu.Lock()
	defer co.mu.Unlock()
	result := co.suggestions
	co.suggestions = nil
	return result
}

// ArmStats returns a snapshot of all arm statistics for serialization.
func (co *ConfigOptimizer) ArmStats() map[string]ConfigArmSnapshot {
	co.mu.Lock()
	defer co.mu.Unlock()

	result := make(map[string]ConfigArmSnapshot, len(co.arms))
	for key, arm := range co.arms {
		result[key] = ConfigArmSnapshot{
			ID:          arm.ID,
			Provider:    arm.Provider,
			TaskType:    arm.TaskType,
			Trials:      arm.Trials,
			Successes:   arm.Successes,
			SuccessRate: safeDiv(float64(arm.Successes), float64(arm.Trials)),
			AvgCostUSD:  safeDiv(arm.TotalCostUSD, float64(arm.Trials)),
			AvgDurSec:   safeDiv(arm.TotalDurSec, float64(arm.Trials)),
			Score:       co.armScore(arm),
			LastUsed:    arm.LastUsed,
		}
	}
	return result
}

// ConfigArmSnapshot is a JSON-serializable view of an arm's state.
type ConfigArmSnapshot struct {
	ID          string    `json:"id"`
	Provider    string    `json:"provider"`
	TaskType    string    `json:"task_type"`
	Trials      int       `json:"trials"`
	Successes   int       `json:"successes"`
	SuccessRate float64   `json:"success_rate"`
	AvgCostUSD  float64   `json:"avg_cost_usd"`
	AvgDurSec   float64   `json:"avg_duration_sec"`
	Score       float64   `json:"score"`
	LastUsed    time.Time `json:"last_used"`
}

// MarshalJSON implements json.Marshaler for ConfigOptimizer state export.
// Builds the snapshot inline to avoid deadlock (ArmStats also acquires co.mu).
func (co *ConfigOptimizer) MarshalJSON() ([]byte, error) {
	co.mu.Lock()

	// Build arm snapshot inline (don't call ArmStats which also locks).
	arms := make(map[string]ConfigArmSnapshot, len(co.arms))
	for key, arm := range co.arms {
		arms[key] = ConfigArmSnapshot{
			ID:          arm.ID,
			Provider:    arm.Provider,
			TaskType:    arm.TaskType,
			Trials:      arm.Trials,
			Successes:   arm.Successes,
			SuccessRate: safeDiv(float64(arm.Successes), float64(arm.Trials)),
			AvgCostUSD:  safeDiv(arm.TotalCostUSD, float64(arm.Trials)),
			AvgDurSec:   safeDiv(arm.TotalDurSec, float64(arm.Trials)),
			Score:       co.armScore(arm),
			LastUsed:    arm.LastUsed,
		}
	}

	export := struct {
		Arms        map[string]ConfigArmSnapshot `json:"arms"`
		Exploration float64                      `json:"exploration"`
		MinTrials   int                          `json:"min_trials"`
		Suggestions []ConfigSuggestion           `json:"pending_suggestions,omitempty"`
	}{
		Arms:        arms,
		Exploration: co.exploration,
		MinTrials:   co.minTrials,
		Suggestions: co.suggestions,
	}
	co.mu.Unlock()

	return json.Marshal(export)
}

// safeDiv divides a by b, returning 0 if b is zero.
func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}
