package session

import (
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/bandit"
)

// TaskComplexity categorizes task difficulty for bandit context.
type TaskComplexity int

const (
	TaskSimple  TaskComplexity = iota // lint, format, docs
	TaskMedium                        // config, review, bug_fix
	TaskComplex                       // architecture, analysis, planning
)

// BudgetPressure categorizes remaining budget for bandit context.
type BudgetPressure int

const (
	BudgetHigh   BudgetPressure = iota // >60% remaining
	BudgetMedium                       // 20-60% remaining
	BudgetLow                          // <20% remaining
)

// TimeSensitivity categorizes time constraints for bandit context.
type TimeSensitivity int

const (
	TimeBatch       TimeSensitivity = iota // batch/async, no urgency
	TimeNormal                             // standard interactive
	TimeInteractive                        // urgent, real-time
)

// CascadeContext provides contextual features for bandit-based routing decisions.
type CascadeContext struct {
	TaskType        string          // well-known task type (e.g. "lint", "feature", "architecture")
	Complexity      TaskComplexity  // derived from task type or explicitly set
	BudgetRemaining float64         // fraction of budget remaining (0.0-1.0)
	BudgetPressure  BudgetPressure  // categorized budget pressure
	TimeSensitivity TimeSensitivity // batch vs interactive
	RecentSuccess   float64         // recent success rate for the considered provider (0.0-1.0)
}

// BanditRouter wraps a contextual bandit policy for dynamic cascade routing.
// It replaces static threshold-based provider selection with learned,
// context-aware decisions that adapt to task characteristics and budget state.
type BanditRouter struct {
	mu     sync.Mutex
	policy *bandit.ContextualThompson
	arms   []bandit.Arm
	armMap map[string]bandit.Arm // arm ID -> Arm

	// minSamples is the minimum number of observations before the bandit
	// overrides static routing. Below this threshold, selections fall back
	// to the CascadeRouter's static tier logic.
	minSamples int
	totalPulls int

	// successTracker maintains a rolling success rate per provider.
	successTracker map[string]*successWindow
}

// successWindow tracks the last N outcomes for a provider.
type successWindow struct {
	outcomes []bool
	maxSize  int
}

func newSuccessWindow(size int) *successWindow {
	return &successWindow{maxSize: size}
}

func (sw *successWindow) Record(success bool) {
	sw.outcomes = append(sw.outcomes, success)
	if len(sw.outcomes) > sw.maxSize {
		sw.outcomes = sw.outcomes[len(sw.outcomes)-sw.maxSize:]
	}
}

func (sw *successWindow) Rate() float64 {
	if len(sw.outcomes) == 0 {
		return 0.5 // no data, assume neutral
	}
	successes := 0
	for _, o := range sw.outcomes {
		if o {
			successes++
		}
	}
	return float64(successes) / float64(len(sw.outcomes))
}

// BanditRouterConfig configures the contextual bandit router.
type BanditRouterConfig struct {
	// Window controls sliding-window decay for the bandit (0 = infinite memory).
	Window int `json:"window"`
	// LearningRate controls how fast context weights adapt (default 0.1).
	LearningRate float64 `json:"learning_rate"`
	// MinSamples is the minimum observations before bandit overrides static routing.
	MinSamples int `json:"min_samples"`
	// SuccessWindowSize is the number of recent outcomes tracked per provider.
	SuccessWindowSize int `json:"success_window_size"`
}

// DefaultBanditRouterConfig returns sensible defaults for the bandit router.
func DefaultBanditRouterConfig() BanditRouterConfig {
	return BanditRouterConfig{
		Window:            100,
		LearningRate:      0.1,
		MinSamples:        10,
		SuccessWindowSize: 50,
	}
}

// NewBanditRouter creates a contextual bandit router from the given model tiers.
// Each tier becomes a bandit arm. The router starts in exploration mode until
// minSamples observations have been recorded.
func NewBanditRouter(tiers []ModelTier, cfg BanditRouterConfig) *BanditRouter {
	if cfg.MinSamples <= 0 {
		cfg.MinSamples = 10
	}
	if cfg.SuccessWindowSize <= 0 {
		cfg.SuccessWindowSize = 50
	}

	arms := make([]bandit.Arm, len(tiers))
	armMap := make(map[string]bandit.Arm, len(tiers))
	for i, t := range tiers {
		arm := bandit.Arm{
			ID:       t.Label,
			Provider: string(t.Provider),
			Model:    t.Model,
		}
		arms[i] = arm
		armMap[arm.ID] = arm
	}

	successTracker := make(map[string]*successWindow)
	for _, arm := range arms {
		successTracker[arm.Provider] = newSuccessWindow(cfg.SuccessWindowSize)
	}

	return &BanditRouter{
		policy:         bandit.NewContextualThompson(arms, cfg.Window, cfg.LearningRate),
		arms:           arms,
		armMap:         armMap,
		minSamples:     cfg.MinSamples,
		successTracker: successTracker,
	}
}

// SelectProvider uses the contextual bandit to pick the best provider/model
// for the given context. Returns a zero Arm if the bandit has insufficient
// data (below minSamples), signaling the caller to fall back to static routing.
func (br *BanditRouter) SelectProvider(ctx CascadeContext) bandit.Arm {
	br.mu.Lock()
	pulls := br.totalPulls
	br.mu.Unlock()

	if pulls < br.minSamples {
		return bandit.Arm{} // not enough data, fall back to static
	}

	features := br.contextToFeatures(ctx)
	arm := br.policy.Select(features)

	slog.Debug("bandit: selected provider",
		"provider", arm.Provider,
		"model", arm.Model,
		"task_type", ctx.TaskType,
		"complexity", ctx.Complexity,
		"budget_pressure", ctx.BudgetPressure,
	)

	return arm
}

// RecordOutcome updates the bandit with the result of a routing decision.
// success indicates whether the task completed without escalation.
// cost is the USD cost of the task. quality is a 0.0-1.0 quality score.
func (br *BanditRouter) RecordOutcome(provider string, model string, success bool, cost float64, quality float64, ctx CascadeContext) {
	// Find the arm ID for this provider/model.
	armID := br.findArmID(provider, model)
	if armID == "" {
		return
	}

	// Compute composite reward: balance quality vs cost.
	reward := br.computeReward(success, cost, quality)

	features := br.contextToFeatures(ctx)

	br.policy.Update(bandit.Reward{
		ArmID:     armID,
		Value:     reward,
		Timestamp: time.Now(),
		Context:   features,
	})

	br.mu.Lock()
	br.totalPulls++
	if sw, ok := br.successTracker[provider]; ok {
		sw.Record(success)
	}
	br.mu.Unlock()

	slog.Debug("bandit: recorded outcome",
		"arm_id", armID,
		"success", success,
		"cost", cost,
		"quality", quality,
		"reward", reward,
	)
}

// Stats returns the bandit arm statistics.
func (br *BanditRouter) Stats() map[string]bandit.ArmStat {
	return br.policy.ArmStats()
}

// TotalPulls returns the total number of observations recorded.
func (br *BanditRouter) TotalPulls() int {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.totalPulls
}

// Ready returns true if the bandit has enough data to override static routing.
func (br *BanditRouter) Ready() bool {
	br.mu.Lock()
	defer br.mu.Unlock()
	return br.totalPulls >= br.minSamples
}

// ProviderSuccessRate returns the recent success rate for a provider.
func (br *BanditRouter) ProviderSuccessRate(provider string) float64 {
	br.mu.Lock()
	defer br.mu.Unlock()
	if sw, ok := br.successTracker[provider]; ok {
		return sw.Rate()
	}
	return 0.5
}

// contextToFeatures converts a CascadeContext into the float64 feature vector
// expected by the contextual bandit policy.
func (br *BanditRouter) contextToFeatures(ctx CascadeContext) []float64 {
	features := make([]float64, bandit.NumContextualFeatures)

	// Complexity: -1.0=simple, 0.0=medium, 1.0=complex.
	// Centered encoding so both directions of the weight create meaningful signal.
	switch ctx.Complexity {
	case TaskSimple:
		features[bandit.FeatureComplexity] = -1.0
	case TaskMedium:
		features[bandit.FeatureComplexity] = 0.0
	case TaskComplex:
		features[bandit.FeatureComplexity] = 1.0
	}

	// Budget pressure: -1.0=low remaining (pressure), 1.0=plenty remaining (no pressure).
	// Centered: map [0, 1] to [-1, 1].
	features[bandit.FeatureBudgetPressure] = 2.0*ctx.BudgetRemaining - 1.0

	// Time sensitivity: -1.0=batch, 0.0=normal, 1.0=interactive.
	switch ctx.TimeSensitivity {
	case TimeBatch:
		features[bandit.FeatureTimeSensitivity] = -1.0
	case TimeNormal:
		features[bandit.FeatureTimeSensitivity] = 0.0
	case TimeInteractive:
		features[bandit.FeatureTimeSensitivity] = 1.0
	}

	// Recent success rate: map [0, 1] to [-1, 1].
	features[bandit.FeatureRecentSuccess] = 2.0*ctx.RecentSuccess - 1.0

	return features
}

// findArmID matches a provider/model to a bandit arm ID.
func (br *BanditRouter) findArmID(provider, model string) string {
	for _, arm := range br.arms {
		if arm.Provider == provider && (model == "" || arm.Model == model) {
			return arm.ID
		}
	}
	// Try matching by provider alone if no model match.
	for _, arm := range br.arms {
		if arm.Provider == provider {
			return arm.ID
		}
	}
	return ""
}

// computeReward produces a 0.0-1.0 composite reward balancing quality and cost.
func (br *BanditRouter) computeReward(success bool, cost float64, quality float64) float64 {
	if !success {
		return 0.1 // small floor reward for exploration data
	}

	// Quality component (0-1): direct pass-through.
	qualityComponent := quality

	// Cost component (0-1): cheaper is better. Use a soft threshold.
	// $0 -> 1.0, $0.50 -> 0.75, $2.00 -> 0.33, $5.00 -> 0.17.
	costComponent := 1.0 / (1.0 + cost)

	// Weighted blend: 60% quality, 40% cost efficiency.
	return 0.6*qualityComponent + 0.4*costComponent
}

// ClassifyComplexity maps a task type string to a TaskComplexity level.
func ClassifyComplexity(taskType string) TaskComplexity {
	c := TaskTypeComplexity(taskType)
	switch {
	case c <= 1:
		return TaskSimple
	case c <= 2:
		return TaskMedium
	default:
		return TaskComplex
	}
}

// ClassifyBudgetPressure maps a remaining budget fraction to a BudgetPressure level.
func ClassifyBudgetPressure(remaining float64) BudgetPressure {
	switch {
	case remaining > 0.6:
		return BudgetHigh
	case remaining > 0.2:
		return BudgetMedium
	default:
		return BudgetLow
	}
}

// BuildCascadeContext constructs a CascadeContext from raw inputs.
func BuildCascadeContext(taskType string, budgetRemaining float64, timeSensitivity TimeSensitivity, recentSuccess float64) CascadeContext {
	return CascadeContext{
		TaskType:        taskType,
		Complexity:      ClassifyComplexity(taskType),
		BudgetRemaining: budgetRemaining,
		BudgetPressure:  ClassifyBudgetPressure(budgetRemaining),
		TimeSensitivity: timeSensitivity,
		RecentSuccess:   recentSuccess,
	}
}

// WireBanditRouter connects a BanditRouter to a CascadeRouter so that
// SelectTier consults the bandit when it has sufficient data.
// This preserves backward compatibility: the existing SetBanditHooks API
// is used under the hood, so all existing cascade logic continues to work.
func WireBanditRouter(cr *CascadeRouter, br *BanditRouter) {
	selectFn := func() (string, string) {
		// Without context, use a neutral default context.
		ctx := CascadeContext{
			Complexity:      TaskMedium,
			BudgetRemaining: 0.5,
			TimeSensitivity: TimeNormal,
			RecentSuccess:   0.5,
		}
		arm := br.SelectProvider(ctx)
		return arm.Provider, arm.Model
	}

	updateFn := func(provider string, reward float64) {
		// Map the simple reward to a full RecordOutcome call.
		success := reward >= 0.5
		quality := reward
		ctx := CascadeContext{
			Complexity:      TaskMedium,
			BudgetRemaining: 0.5,
		}
		br.RecordOutcome(provider, "", success, 0.0, quality, ctx)
	}

	cr.SetBanditHooks(selectFn, updateFn)
}
