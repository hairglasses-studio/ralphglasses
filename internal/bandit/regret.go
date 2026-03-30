package bandit

import (
	"math"
	"sync"
)

// PullRecord stores one arm pull for regret analysis.
type PullRecord struct {
	Arm    string  `json:"arm"`
	Reward float64 `json:"reward"`
	Step   int     `json:"step"`
}

// ArmRegret holds per-arm regret statistics.
type ArmRegret struct {
	ArmID          string  `json:"arm_id"`
	Pulls          int     `json:"pulls"`
	TotalReward    float64 `json:"total_reward"`
	MeanReward     float64 `json:"mean_reward"`
	RegretContrib  float64 `json:"regret_contribution"`  // sum of (optimal_mean - arm_mean) over pulls
	RegretFraction float64 `json:"regret_fraction"`       // fraction of total regret from this arm
}

// RegretReport contains visualization-ready regret data.
type RegretReport struct {
	// Cumulative regret at each timestep (length = total pulls).
	CumulativeRegret []float64 `json:"cumulative_regret"`

	// Per-arm regret breakdown.
	ArmRegrets []ArmRegret `json:"arm_regrets"`

	// Convergence rate: slope of regret over the last window of pulls.
	// A value near zero indicates the policy has converged.
	ConvergenceRate float64 `json:"convergence_rate"`

	// BayesianRegretBound is the theoretical upper bound on expected regret
	// for a K-arm Beta-Bernoulli Thompson Sampling policy after T steps:
	// O(sqrt(K * T * ln(T))). Zero if no pulls have been recorded.
	BayesianRegretBound float64 `json:"bayesian_regret_bound"`

	// OptimalArm is the arm with the highest observed mean reward.
	OptimalArm string `json:"optimal_arm"`

	// TotalRegret is the final cumulative regret value.
	TotalRegret float64 `json:"total_regret"`

	// TotalPulls is the number of recorded pulls.
	TotalPulls int `json:"total_pulls"`
}

// RegretTracker computes cumulative regret vs the empirically optimal arm,
// per-arm regret contribution, regret convergence rate, and Bayesian regret
// bounds. It is safe for concurrent use.
type RegretTracker struct {
	mu      sync.Mutex
	history []PullRecord

	// Per-arm running totals for O(1) mean computation.
	armPulls  map[string]int
	armReward map[string]float64
}

// NewRegretTracker creates a new regret tracker.
func NewRegretTracker() *RegretTracker {
	return &RegretTracker{
		armPulls:  make(map[string]int),
		armReward: make(map[string]float64),
	}
}

// RecordPull records an arm pull and its observed reward.
func (rt *RegretTracker) RecordPull(arm string, reward float64) {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	step := len(rt.history)
	rt.history = append(rt.history, PullRecord{
		Arm:    arm,
		Reward: reward,
		Step:   step,
	})
	rt.armPulls[arm]++
	rt.armReward[arm] += reward
}

// Report computes and returns visualization-ready regret data.
func (rt *RegretTracker) Report() RegretReport {
	rt.mu.Lock()
	defer rt.mu.Unlock()

	n := len(rt.history)
	if n == 0 {
		return RegretReport{}
	}

	// Find the empirically optimal arm (highest mean reward).
	optimalArm := ""
	optimalMean := math.Inf(-1)
	for arm, total := range rt.armReward {
		mean := total / float64(rt.armPulls[arm])
		if mean > optimalMean {
			optimalMean = mean
			optimalArm = arm
		}
	}

	// Build cumulative regret series and per-arm regret contribution.
	cumRegret := make([]float64, n)

	// We compute instantaneous regret at each step as (optimalMean - reward_t).
	// This is the standard "pseudo-regret" where the optimal arm's mean is the
	// empirically observed mean over all its pulls.
	running := 0.0
	for i, rec := range rt.history {
		instantaneous := optimalMean - rec.Reward
		if instantaneous < 0 {
			instantaneous = 0
		}
		running += instantaneous
		cumRegret[i] = running
	}
	totalRegret := running

	// Per-arm regret breakdown.
	armRegrets := make([]ArmRegret, 0, len(rt.armPulls))
	for arm, pulls := range rt.armPulls {
		meanReward := rt.armReward[arm] / float64(pulls)
		// Regret contribution: pulls * (optimalMean - armMean).
		contrib := float64(pulls) * (optimalMean - meanReward)
		if contrib < 0 {
			contrib = 0
		}
		frac := 0.0
		if totalRegret > 0 {
			frac = contrib / totalRegret
		}
		armRegrets = append(armRegrets, ArmRegret{
			ArmID:          arm,
			Pulls:          pulls,
			TotalReward:    rt.armReward[arm],
			MeanReward:     meanReward,
			RegretContrib:  contrib,
			RegretFraction: frac,
		})
	}

	// Convergence rate: linear regression slope over the last window of
	// cumulative regret values. A flattening slope indicates convergence.
	convergenceRate := rt.convergenceRate(cumRegret)

	// Bayesian regret bound for K-arm Thompson Sampling:
	// E[R(T)] <= C * sqrt(K * T * ln(T)) for Beta-Bernoulli TS.
	// We use C=1 as the constant factor for a clean upper bound.
	k := float64(len(rt.armPulls))
	t := float64(n)
	bayesianBound := 0.0
	if t > 1 {
		bayesianBound = math.Sqrt(k * t * math.Log(t))
	}

	return RegretReport{
		CumulativeRegret:    cumRegret,
		ArmRegrets:          armRegrets,
		ConvergenceRate:     convergenceRate,
		BayesianRegretBound: bayesianBound,
		OptimalArm:          optimalArm,
		TotalRegret:         totalRegret,
		TotalPulls:          n,
	}
}

// convergenceRate computes the slope of cumulative regret over the last
// min(len, 100) steps using ordinary least squares. A decreasing slope
// (approaching zero) indicates the policy is converging on the optimal arm.
func (rt *RegretTracker) convergenceRate(cumRegret []float64) float64 {
	n := len(cumRegret)
	if n < 2 {
		return 0
	}

	window := 100
	if n < window {
		window = n
	}
	tail := cumRegret[n-window:]

	// OLS: slope = (n*sum(x*y) - sum(x)*sum(y)) / (n*sum(x^2) - sum(x)^2)
	w := float64(len(tail))
	var sumX, sumY, sumXY, sumX2 float64
	for i, y := range tail {
		x := float64(i)
		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
	}
	denom := w*sumX2 - sumX*sumX
	if denom == 0 {
		return 0
	}
	return (w*sumXY - sumX*sumY) / denom
}
