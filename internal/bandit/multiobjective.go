package bandit

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"strings"
	"sync"
	"time"
)

// Direction indicates whether an objective should be maximized or minimized.
type Direction int

const (
	Maximize Direction = iota
	Minimize
)

// Objective defines a single optimization dimension.
type Objective struct {
	Name      string    `json:"name"`
	Weight    float64   `json:"weight"`    // relative importance (weights are normalized internally)
	Direction Direction `json:"direction"` // Maximize or Minimize
}

// DefaultObjectives returns the standard Quality/Cost/Latency objectives.
func DefaultObjectives() []Objective {
	return []Objective{
		{Name: "quality", Weight: 0.6, Direction: Maximize},
		{Name: "cost", Weight: 0.3, Direction: Minimize},
		{Name: "latency", Weight: 0.1, Direction: Minimize},
	}
}

// ContextFeatures carries task-level context for selection decisions.
type ContextFeatures struct {
	Complexity      float64 // -1.0=simple, 1.0=complex
	BudgetPressure  float64 // -1.0=low remaining, 1.0=high remaining
	TimeSensitivity float64 // -1.0=batch, 1.0=interactive/urgent
}

// MOArmStats holds per-arm summary statistics across all objectives.
type MOArmStats struct {
	Arm            string                    `json:"arm"`
	Objectives     map[string]ObjectiveStats `json:"objectives"`
	Selections     int                       `json:"selections"`
	ParetoRank     int                       `json:"pareto_rank"` // 0 = Pareto-optimal (non-dominated)
	ScalarizedMean float64                   `json:"scalarized_mean"`
}

// ObjectiveStats holds per-objective statistics for a single arm.
type ObjectiveStats struct {
	Mean     float64 `json:"mean"`
	Variance float64 `json:"variance"`
	Alpha    float64 `json:"alpha"` // Beta distribution parameter
	Beta     float64 `json:"beta"`  // Beta distribution parameter
	Count    int     `json:"count"`
}

// moArm tracks per-objective Beta posteriors for a single arm.
type moArm struct {
	id         string
	objectives map[string]*moObjState
	selections int
}

// moObjState holds the Beta posterior for one objective of one arm.
type moObjState struct {
	alpha float64 // successes + 1 (prior)
	beta  float64 // failures + 1 (prior)
	sum   float64 // sum of normalized rewards
	sumSq float64 // sum of squared normalized rewards
	count int
}

// MultiObjectiveBandit optimizes across multiple reward dimensions using
// Thompson sampling with Pareto-optimal scalarization. Each objective
// maintains an independent Beta posterior per arm. Selection uses a
// weighted sum of Thompson samples, with minimization objectives flipped
// so that lower raw values produce higher scalarized scores.
type MultiObjectiveBandit struct {
	mu          sync.Mutex
	arms        map[string]*moArm
	order       []string
	objectives  []Objective
	normWeights []float64 // normalized objective weights (sum to 1)
}

// NewMultiObjectiveBandit creates a multi-objective bandit.
// arms are the selectable provider/model IDs.
// objectives define the optimization dimensions.
func NewMultiObjectiveBandit(arms []string, objectives []Objective) *MultiObjectiveBandit {
	if len(objectives) == 0 {
		objectives = DefaultObjectives()
	}

	// Normalize weights.
	totalWeight := 0.0
	for _, o := range objectives {
		totalWeight += o.Weight
	}
	normWeights := make([]float64, len(objectives))
	for i, o := range objectives {
		if totalWeight > 0 {
			normWeights[i] = o.Weight / totalWeight
		} else {
			normWeights[i] = 1.0 / float64(len(objectives))
		}
	}

	armMap := make(map[string]*moArm, len(arms))
	order := make([]string, len(arms))
	for i, id := range arms {
		objStates := make(map[string]*moObjState, len(objectives))
		for _, o := range objectives {
			objStates[o.Name] = &moObjState{alpha: 1.0, beta: 1.0}
		}
		armMap[id] = &moArm{id: id, objectives: objStates}
		order[i] = id
	}

	return &MultiObjectiveBandit{
		arms:        armMap,
		order:       order,
		objectives:  objectives,
		normWeights: normWeights,
	}
}

// Select chooses an arm using scalarized Thompson sampling over multiple
// objectives. Returns the selected arm ID and a human-readable rationale.
func (mob *MultiObjectiveBandit) Select(ctx ContextFeatures) (string, string) {
	mob.mu.Lock()
	defer mob.mu.Unlock()

	if len(mob.order) == 0 {
		return "", "no arms available"
	}

	seed := uint64(time.Now().UnixNano())
	bestArm := ""
	bestScore := math.Inf(-1)
	var bestSamples map[string]float64

	for _, id := range mob.order {
		arm := mob.arms[id]
		score := 0.0
		samples := make(map[string]float64, len(mob.objectives))

		for i, obj := range mob.objectives {
			state := arm.objectives[obj.Name]
			if state == nil {
				continue
			}

			// Thompson sample from Beta posterior.
			dist := distuv_Beta{
				alpha: state.alpha,
				beta:  state.beta,
				rng:   rand.New(rand.NewPCG(seed, seed+1)),
			}
			sample := dist.Rand()
			seed += 2

			// For minimization: flip the sample so lower raw reward = higher score.
			if obj.Direction == Minimize {
				sample = 1.0 - sample
			}

			samples[obj.Name] = sample
			score += mob.normWeights[i] * sample
		}

		// Context modulation: budget pressure shifts cost weight.
		if ctx.BudgetPressure < -0.5 {
			// Low budget remaining: boost cost sensitivity.
			for i, obj := range mob.objectives {
				if obj.Name == "cost" && obj.Direction == Minimize {
					score += mob.normWeights[i] * 0.2 * samples[obj.Name]
				}
			}
		}

		if score > bestScore {
			bestScore = score
			bestArm = id
			bestSamples = samples
		}
	}

	if bestArm != "" {
		mob.arms[bestArm].selections++
	}

	rationale := mob.buildRationale(bestArm, bestSamples)
	return bestArm, rationale
}

// buildRationale generates a human-readable explanation for the selection.
func (mob *MultiObjectiveBandit) buildRationale(arm string, samples map[string]float64) string {
	if arm == "" {
		return "no arms available"
	}
	var parts strings.Builder
	parts.WriteString(fmt.Sprintf("selected %s:", arm))
	for _, obj := range mob.objectives {
		s, ok := samples[obj.Name]
		if !ok {
			continue
		}
		dir := "max"
		if obj.Direction == Minimize {
			dir = "min"
		}
		parts.WriteString(fmt.Sprintf(" %s=%.2f(%s)", obj.Name, s, dir))
	}
	return parts.String()
}

// Update records observed rewards for an arm across one or more objectives.
// rewards maps objective name to observed value in [0, 1].
// For minimization objectives (e.g., cost), lower values are better;
// the bandit flips them internally during selection.
func (mob *MultiObjectiveBandit) Update(arm string, rewards map[string]float64) {
	mob.mu.Lock()
	defer mob.mu.Unlock()

	a, ok := mob.arms[arm]
	if !ok {
		return
	}

	for name, value := range rewards {
		state, ok := a.objectives[name]
		if !ok {
			continue
		}

		// Clamp to [0, 1].
		if value < 0 {
			value = 0
		}
		if value > 1 {
			value = 1
		}

		// Update Beta posterior.
		if value >= 0.5 {
			state.alpha += value
		} else {
			state.beta += (1.0 - value)
		}

		// Track running statistics for variance.
		state.count++
		state.sum += value
		state.sumSq += value * value
	}
}

// ParetoFront returns arm statistics sorted by Pareto rank.
// Rank 0 arms are non-dominated (Pareto-optimal).
func (mob *MultiObjectiveBandit) ParetoFront() []MOArmStats {
	mob.mu.Lock()
	defer mob.mu.Unlock()

	stats := make([]MOArmStats, 0, len(mob.order))
	for _, id := range mob.order {
		arm := mob.arms[id]
		objStats := make(map[string]ObjectiveStats, len(mob.objectives))
		for _, obj := range mob.objectives {
			state := arm.objectives[obj.Name]
			if state == nil {
				continue
			}
			mean := state.alpha / (state.alpha + state.beta)
			var variance float64
			if state.count > 1 {
				meanObs := state.sum / float64(state.count)
				variance = (state.sumSq/float64(state.count) - meanObs*meanObs)
				if variance < 0 {
					variance = 0
				}
			}
			objStats[obj.Name] = ObjectiveStats{
				Mean:     mean,
				Variance: variance,
				Alpha:    state.alpha,
				Beta:     state.beta,
				Count:    state.count,
			}
		}

		s := MOArmStats{
			Arm:        id,
			Objectives: objStats,
			Selections: arm.selections,
		}

		// Scalarized mean: weighted sum of objective means.
		for i, obj := range mob.objectives {
			os, ok := objStats[obj.Name]
			if !ok {
				continue
			}
			v := os.Mean
			if obj.Direction == Minimize {
				v = 1.0 - v
			}
			s.ScalarizedMean += mob.normWeights[i] * v
		}

		stats = append(stats, s)
	}

	// Compute Pareto ranks via non-dominated sorting.
	assignParetoRanks(stats, mob.objectives)

	// Sort by rank, then descending scalarized mean.
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].ParetoRank != stats[j].ParetoRank {
			return stats[i].ParetoRank < stats[j].ParetoRank
		}
		return stats[i].ScalarizedMean > stats[j].ScalarizedMean
	})

	return stats
}

// dominates returns true if arm a dominates arm b across all objectives.
// An arm dominates another if it is at least as good in all objectives
// and strictly better in at least one.
func dominates(a, b MOArmStats, objectives []Objective) bool {
	atLeastAsGood := true
	strictlyBetter := false

	for _, obj := range objectives {
		aVal := a.Objectives[obj.Name].Mean
		bVal := b.Objectives[obj.Name].Mean

		var aBetter, bBetter bool
		if obj.Direction == Maximize {
			aBetter = aVal > bVal
			bBetter = bVal > aVal
		} else {
			// For minimize, lower is better.
			aBetter = aVal < bVal
			bBetter = bVal < aVal
		}

		if bBetter {
			atLeastAsGood = false
			break
		}
		if aBetter {
			strictlyBetter = true
		}
	}

	return atLeastAsGood && strictlyBetter
}

// assignParetoRanks performs non-dominated sorting and assigns ParetoRank
// to each arm. Rank 0 = non-dominated front.
func assignParetoRanks(stats []MOArmStats, objectives []Objective) {
	n := len(stats)
	ranked := make([]bool, n)
	rank := 0

	for {
		front := make([]int, 0)
		for i := range n {
			if ranked[i] {
				continue
			}
			dominated := false
			for j := range n {
				if i == j || ranked[j] {
					continue
				}
				if dominates(stats[j], stats[i], objectives) {
					dominated = true
					break
				}
			}
			if !dominated {
				front = append(front, i)
			}
		}

		if len(front) == 0 {
			break
		}

		for _, idx := range front {
			stats[idx].ParetoRank = rank
			ranked[idx] = true
		}
		rank++
	}
}
