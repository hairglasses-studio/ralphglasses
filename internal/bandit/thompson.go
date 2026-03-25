package bandit

import (
	"math/rand/v2"
	"sync"
	"time"

	"gonum.org/v1/gonum/stat/distuv"
)

// ThompsonSampling implements Thompson Sampling with Beta-Bernoulli priors
// and optional sliding-window decay.
type ThompsonSampling struct {
	mu     sync.Mutex
	arms   map[string]*betaArm
	order  []string // arm IDs in insertion order for deterministic iteration
	window int      // sliding window size (0 = infinite memory)
}

type betaArm struct {
	arm     Arm
	alpha   float64   // successes + 1 (prior)
	beta    float64   // failures + 1 (prior)
	history []float64 // for sliding window (most recent rewards)
}

// NewThompsonSampling creates a new Thompson Sampling policy.
// Each arm is initialized with a Beta(1, 1) uniform prior.
// window controls sliding-window decay: 0 means infinite memory.
func NewThompsonSampling(arms []Arm, window int) *ThompsonSampling {
	ts := &ThompsonSampling{
		arms:   make(map[string]*betaArm, len(arms)),
		order:  make([]string, 0, len(arms)),
		window: window,
	}
	for _, a := range arms {
		ts.arms[a.ID] = &betaArm{
			arm:   a,
			alpha: 1.0,
			beta:  1.0,
		}
		ts.order = append(ts.order, a.ID)
	}
	return ts
}

// Select chooses an arm using Thompson Sampling. The ctx parameter is ignored
// (reserved for contextual bandit extensions).
func (ts *ThompsonSampling) Select(_ []float64) Arm {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	if len(ts.order) == 0 {
		return Arm{}
	}

	seed := uint64(time.Now().UnixNano())
	var bestArm Arm
	bestSample := -1.0

	for _, id := range ts.order {
		ba := ts.arms[id]
		dist := distuv.Beta{
			Alpha: ba.alpha,
			Beta:  ba.beta,
			Src:   rand.NewPCG(seed, seed+1),
		}
		sample := dist.Rand()
		if sample > bestSample {
			bestSample = sample
			bestArm = ba.arm
		}
		seed += 2
	}

	return bestArm
}

// Update records a reward for an arm.
func (ts *ThompsonSampling) Update(reward Reward) {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ba, ok := ts.arms[reward.ArmID]
	if !ok {
		return
	}

	ba.history = append(ba.history, reward.Value)

	if ts.window > 0 && len(ba.history) > ts.window {
		// Slide the window: keep only the most recent `window` rewards.
		ba.history = ba.history[len(ba.history)-ts.window:]
		// Recalculate alpha/beta from the window.
		ba.alpha = 1.0
		ba.beta = 1.0
		for _, v := range ba.history {
			if v >= 0.5 {
				ba.alpha += v
			} else {
				ba.beta += (1.0 - v)
			}
		}
	} else {
		// Incremental update.
		if reward.Value >= 0.5 {
			ba.alpha += reward.Value
		} else {
			ba.beta += (1.0 - reward.Value)
		}
	}
}

// ArmStats returns summary statistics for each arm.
func (ts *ThompsonSampling) ArmStats() map[string]ArmStat {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	stats := make(map[string]ArmStat, len(ts.arms))
	for _, id := range ts.order {
		ba := ts.arms[id]
		stats[id] = ArmStat{
			Pulls:      len(ba.history),
			MeanReward: ba.alpha / (ba.alpha + ba.beta),
			Alpha:      ba.alpha,
			Beta:       ba.beta,
		}
	}
	return stats
}
