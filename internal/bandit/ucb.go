package bandit

import (
	"math"
	"sync"
)

// DiscountedUCB1 implements the Discounted UCB1 algorithm for non-stationary
// reward distributions. Recent rewards are weighted more heavily than old ones
// via an exponential discount factor gamma applied at every step.
type DiscountedUCB1 struct {
	mu     sync.Mutex
	arms   map[string]*ucbArm
	order  []string // arm IDs in insertion order
	gamma  float64  // discount factor per step (e.g., 0.99)
	totalN float64  // discounted total pulls
	step   int      // global step counter
}

type ucbArm struct {
	arm           Arm
	discountedN   float64 // discounted pull count
	discountedSum float64 // discounted reward sum
	pulls         int     // actual pull count
}

// NewDiscountedUCB1 creates a new Discounted UCB1 policy.
// gamma must be in (0, 1]; values outside that range are clamped to 0.99.
func NewDiscountedUCB1(arms []Arm, gamma float64) *DiscountedUCB1 {
	if gamma <= 0 || gamma > 1 {
		gamma = 0.99
	}
	m := make(map[string]*ucbArm, len(arms))
	order := make([]string, 0, len(arms))
	for _, a := range arms {
		m[a.ID] = &ucbArm{arm: a}
		order = append(order, a.ID)
	}
	return &DiscountedUCB1{
		arms:  m,
		order: order,
		gamma: gamma,
	}
}

// Select returns the arm with the highest UCB score. Arms that have never
// been pulled are returned first (round-robin exploration). The ctx parameter
// is ignored — Discounted UCB1 is context-free.
func (d *DiscountedUCB1) Select(_ []float64) Arm {
	d.mu.Lock()
	defer d.mu.Unlock()

	if len(d.order) == 0 {
		return Arm{}
	}

	// Explore: return first unpulled arm.
	for _, id := range d.order {
		if d.arms[id].pulls == 0 {
			return d.arms[id].arm
		}
	}

	// Exploit: pick arm with highest UCB score.
	bestID := d.order[0]
	bestScore := math.Inf(-1)
	logTotal := math.Log(d.totalN)

	for _, id := range d.order {
		a := d.arms[id]
		mu := a.discountedSum / a.discountedN
		exploration := math.Sqrt(2 * logTotal / a.discountedN)
		score := mu + exploration
		if score > bestScore {
			bestScore = score
			bestID = id
		}
	}
	return d.arms[bestID].arm
}

// Update records a reward observation. It applies the discount factor to all
// arms before crediting the selected arm, so that recent observations carry
// more weight in future selections.
func (d *DiscountedUCB1) Update(reward Reward) {
	d.mu.Lock()
	defer d.mu.Unlock()

	a, ok := d.arms[reward.ArmID]
	if !ok {
		return
	}

	d.step++

	// Apply discount to all arms.
	for _, arm := range d.arms {
		arm.discountedN *= d.gamma
		arm.discountedSum *= d.gamma
	}
	d.totalN *= d.gamma

	// Credit the selected arm.
	a.discountedN += 1
	a.discountedSum += reward.Value
	d.totalN += 1
	a.pulls++
}

// ArmStats returns summary statistics for every arm. Alpha and Beta are set
// to 0 because they are not applicable to UCB policies.
func (d *DiscountedUCB1) ArmStats() map[string]ArmStat {
	d.mu.Lock()
	defer d.mu.Unlock()

	stats := make(map[string]ArmStat, len(d.arms))
	for id, a := range d.arms {
		var mean float64
		if a.discountedN > 0 {
			mean = a.discountedSum / a.discountedN
		}
		stats[id] = ArmStat{
			Pulls:      a.pulls,
			MeanReward: mean,
		}
	}
	return stats
}
