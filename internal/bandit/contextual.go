package bandit

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// ContextualFeature identifies a named feature in the context vector.
type ContextualFeature int

const (
	FeatureComplexity      ContextualFeature = iota // -1.0=simple, 0.0=medium, 1.0=complex
	FeatureBudgetPressure                           // -1.0=low remaining, 0.0=medium, 1.0=high remaining
	FeatureTimeSensitivity                          // -1.0=batch, 0.0=normal, 1.0=interactive/urgent
	FeatureRecentSuccess                            // -1.0=low success, 0.0=neutral, 1.0=high success
	FeaturePromptQuality                            // -1.0=F grade, 0.0=C grade, 1.0=A grade (Prompt DJ)
	FeatureCacheAffinity                            // -1.0=no cache, 0.0=unknown, 1.0=warm cache hit (Prompt DJ)
	NumContextualFeatures                           // sentinel: total feature count
)

// ContextualArm extends an Arm with per-feature weight tracking.
type ContextualArm struct {
	Arm
	weights []float64 // one weight per feature
	bias    float64   // baseline reward estimate
	pulls   int
	alpha   float64 // Beta dist successes + 1
	beta    float64 // Beta dist failures + 1
}

// ContextualThompson implements a contextual Thompson Sampling policy.
// Each arm maintains per-feature weights that modulate the Beta prior
// based on context. This enables context-dependent arm selection:
// e.g., cheaper providers are favored for simple tasks with low budget pressure.
type ContextualThompson struct {
	mu           sync.Mutex
	arms         map[string]*ContextualArm
	order        []string
	window       int // sliding window size (0 = infinite)
	history      map[string][]contextualReward
	learningRate float64 // weight update step size
}

type contextualReward struct {
	value   float64
	context []float64
}

// NewContextualThompson creates a contextual Thompson Sampling policy.
// arms are the selectable provider/model combinations.
// window controls sliding-window decay (0 = infinite memory).
// learningRate controls how fast weights adapt (0.1 is a good default).
func NewContextualThompson(arms []Arm, window int, learningRate float64) *ContextualThompson {
	if learningRate <= 0 {
		learningRate = 0.1
	}
	ct := &ContextualThompson{
		arms:         make(map[string]*ContextualArm, len(arms)),
		order:        make([]string, 0, len(arms)),
		window:       window,
		history:      make(map[string][]contextualReward),
		learningRate: learningRate,
	}
	for _, a := range arms {
		ct.arms[a.ID] = &ContextualArm{
			Arm:     a,
			weights: make([]float64, NumContextualFeatures),
			bias:    0.0,
			alpha:   1.0,
			beta:    1.0,
		}
		ct.order = append(ct.order, a.ID)
	}
	return ct
}

// Select chooses an arm using contextual Thompson Sampling.
// ctx is a feature vector of length NumContextualFeatures.
// If ctx is nil, falls back to standard Thompson Sampling (context-free).
func (ct *ContextualThompson) Select(ctx []float64) Arm {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if len(ct.order) == 0 {
		return Arm{}
	}

	seed := uint64(time.Now().UnixNano())
	var bestArm Arm
	bestScore := math.Inf(-1)

	for _, id := range ct.order {
		ca := ct.arms[id]

		// Sample from the Beta posterior.
		dist := distuv_Beta{
			alpha: ca.alpha,
			beta:  ca.beta,
			rng:   rand.New(rand.NewPCG(seed, seed+1)),
		}
		sample := dist.Rand()
		seed += 2

		// Modulate by context if available. The context score is added to
		// the Beta sample so that learned weights can shift arm rankings
		// based on task features (e.g., prefer cheap arms for simple tasks).
		if len(ctx) > 0 {
			contextScore := ca.bias
			n := min(len(ctx), len(ca.weights))
			for i := range n {
				contextScore += ca.weights[i] * ctx[i]
			}
			// Additive modulation: tanh keeps the bonus in [-1, 1],
			// which is enough to shift rankings between arms with
			// similar Beta posteriors.
			sample += math.Tanh(contextScore)
		}

		if sample > bestScore {
			bestScore = sample
			bestArm = ca.Arm
		}
	}

	return bestArm
}

// Update records a reward for an arm with its context.
func (ct *ContextualThompson) Update(reward Reward) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ca, ok := ct.arms[reward.ArmID]
	if !ok {
		return
	}

	// Update Beta distribution parameters.
	if reward.Value >= 0.5 {
		ca.alpha += reward.Value
	} else {
		ca.beta += (1.0 - reward.Value)
	}
	ca.pulls++

	// Update context weights via gradient step.
	if len(reward.Context) > 0 {
		// Compute predicted reward from context.
		predicted := ca.bias
		n := min(len(reward.Context), len(ca.weights))
		for i := range n {
			predicted += ca.weights[i] * reward.Context[i]
		}
		predicted = sigmoid(predicted)

		// Gradient: error * sigmoid_derivative * feature.
		err := reward.Value - predicted
		grad := err * predicted * (1.0 - predicted)
		for i := range n {
			ca.weights[i] += ct.learningRate * grad * reward.Context[i]
		}
		ca.bias += ct.learningRate * grad
	}

	// Track history for sliding window.
	cr := contextualReward{value: reward.Value, context: reward.Context}
	ct.history[reward.ArmID] = append(ct.history[reward.ArmID], cr)

	if ct.window > 0 && len(ct.history[reward.ArmID]) > ct.window {
		ct.history[reward.ArmID] = ct.history[reward.ArmID][len(ct.history[reward.ArmID])-ct.window:]
		// Recalculate alpha/beta from window.
		ca.alpha = 1.0
		ca.beta = 1.0
		for _, h := range ct.history[reward.ArmID] {
			if h.value >= 0.5 {
				ca.alpha += h.value
			} else {
				ca.beta += (1.0 - h.value)
			}
		}
	}
}

// ArmStats returns summary statistics for each arm, including weight norms.
func (ct *ContextualThompson) ArmStats() map[string]ArmStat {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	stats := make(map[string]ArmStat, len(ct.arms))
	for _, id := range ct.order {
		ca := ct.arms[id]
		stats[id] = ArmStat{
			Pulls:      ca.pulls,
			MeanReward: ca.alpha / (ca.alpha + ca.beta),
			Alpha:      ca.alpha,
			Beta:       ca.beta,
		}
	}
	return stats
}

// ArmWeights returns the context weights for a specific arm.
// Returns nil if the arm is not found.
func (ct *ContextualThompson) ArmWeights(armID string) []float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ca, ok := ct.arms[armID]
	if !ok {
		return nil
	}
	w := make([]float64, len(ca.weights))
	copy(w, ca.weights)
	return w
}

// ArmBias returns the bias term for a specific arm.
func (ct *ContextualThompson) ArmBias(armID string) float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ca, ok := ct.arms[armID]
	if !ok {
		return 0
	}
	return ca.bias
}

// sigmoid maps any real number to (0, 1).
func sigmoid(x float64) float64 {
	return 1.0 / (1.0 + math.Exp(-x))
}

// distuv_Beta is a minimal Beta distribution sampler that avoids the gonum
// import for this file. It uses the Kumaraswamy approximation via inverse CDF.
// For production accuracy the gonum Beta is preferred, but this keeps the
// contextual extension self-contained.
type distuv_Beta struct {
	alpha float64
	beta  float64
	rng   *rand.Rand
}

// Rand samples from the Beta distribution using the ratio of Gamma variates.
func (b distuv_Beta) Rand() float64 {
	// Use the standard Gamma sampling trick: Beta(a,b) = G(a) / (G(a)+G(b)).
	x := gammaVariate(b.rng, b.alpha)
	y := gammaVariate(b.rng, b.beta)
	if x+y == 0 {
		return 0.5
	}
	return x / (x + y)
}

// gammaVariate generates a Gamma(shape, 1) variate using Marsaglia & Tsang's method.
func gammaVariate(rng *rand.Rand, shape float64) float64 {
	if shape < 1 {
		// For shape < 1, use the relation: Gamma(a) = Gamma(a+1) * U^(1/a).
		return gammaVariate(rng, shape+1) * math.Pow(rng.Float64(), 1.0/shape)
	}
	d := shape - 1.0/3.0
	c := 1.0 / math.Sqrt(9.0*d)
	for {
		var x, v float64
		for {
			x = rng.NormFloat64()
			v = 1.0 + c*x
			if v > 0 {
				break
			}
		}
		v = v * v * v
		u := rng.Float64()
		if u < 1.0-0.0331*(x*x)*(x*x) {
			return d * v
		}
		if math.Log(u) < 0.5*x*x+d*(1.0-v+math.Log(v)) {
			return d * v
		}
	}
}
