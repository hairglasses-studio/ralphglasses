package bandit

import (
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

// NeuralUCBConfig holds hyperparameters for the NeuralUCB policy.
type NeuralUCBConfig struct {
	HiddenSize   int     // hidden layer width (default 32)
	LearningRate float64 // SGD step size (default 0.01)
	ExplorationC float64 // UCB exploration coefficient (default 1.0)
	BudgetWeight float64 // how much to penalize cost in reward (default 0.3)
}

// neuralArm holds the single-hidden-layer network and pull statistics for one arm.
type neuralArm struct {
	arm   Arm
	pulls int
	// Running reward statistics.
	totalReward float64

	// Network weights: input(features) -> hidden -> output(1).
	// Forward: output = sigmoid(ReLU(x * W1 + B1) * W2 + B2)
	w1 [][]float64 // [features][hidden]
	b1 []float64   // [hidden]
	w2 []float64   // [hidden]
	b2 float64
}

// NeuralUCB implements a neural contextual bandit policy inspired by
// NeuralUCB (ArXiv 2603.30035). Each arm has an independent single-hidden-layer
// network that predicts reward from context features. Selection uses the
// predicted reward plus a UCB exploration bonus.
type NeuralUCB struct {
	mu       sync.Mutex
	arms     map[string]*neuralArm
	order    []string // arm IDs in insertion order for deterministic iteration
	cfg      NeuralUCBConfig
	features int
}

func defaultNeuralUCBConfig(cfg NeuralUCBConfig) NeuralUCBConfig {
	if cfg.HiddenSize <= 0 {
		cfg.HiddenSize = 32
	}
	if cfg.LearningRate <= 0 {
		cfg.LearningRate = 0.01
	}
	if cfg.ExplorationC <= 0 {
		cfg.ExplorationC = 1.0
	}
	if cfg.BudgetWeight <= 0 {
		cfg.BudgetWeight = 0.3
	}
	return cfg
}

// NewNeuralUCB creates a NeuralUCB policy.
// arms are the selectable provider/model combinations.
// numFeatures is the context vector dimensionality.
// cfg controls hyperparameters; zero values are replaced with defaults.
func NewNeuralUCB(arms []Arm, numFeatures int, cfg NeuralUCBConfig) *NeuralUCB {
	cfg = defaultNeuralUCBConfig(cfg)
	if numFeatures <= 0 {
		numFeatures = int(NumContextualFeatures)
	}

	n := &NeuralUCB{
		arms:     make(map[string]*neuralArm, len(arms)),
		order:    make([]string, 0, len(arms)),
		cfg:      cfg,
		features: numFeatures,
	}

	rng := rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 42))
	for _, a := range arms {
		na := &neuralArm{
			arm: a,
			w1:  make([][]float64, numFeatures),
			b1:  make([]float64, cfg.HiddenSize),
			w2:  make([]float64, cfg.HiddenSize),
			b2:  0.0,
		}
		// Xavier initialization: scale = sqrt(2 / (fan_in + fan_out)).
		scaleW1 := math.Sqrt(2.0 / float64(numFeatures+cfg.HiddenSize))
		for i := 0; i < numFeatures; i++ {
			na.w1[i] = make([]float64, cfg.HiddenSize)
			for j := 0; j < cfg.HiddenSize; j++ {
				na.w1[i][j] = rng.NormFloat64() * scaleW1
			}
		}
		scaleW2 := math.Sqrt(2.0 / float64(cfg.HiddenSize+1))
		for j := 0; j < cfg.HiddenSize; j++ {
			na.w2[j] = rng.NormFloat64() * scaleW2
		}
		n.arms[a.ID] = na
		n.order = append(n.order, a.ID)
	}

	return n
}

// forward computes the network output for a given context vector.
// Returns (output, hiddenActivations) for use in backprop.
// output = sigmoid(ReLU(x * W1 + B1) * W2 + B2)
func (na *neuralArm) forward(ctx []float64, numFeatures int) (float64, []float64) {
	hidden := make([]float64, len(na.b1))
	nf := min(len(ctx), numFeatures)

	// Hidden layer: ReLU(x * W1 + B1)
	for j := range hidden {
		sum := na.b1[j]
		for i := range nf {
			sum += ctx[i] * na.w1[i][j]
		}
		hidden[j] = relu(sum)
	}

	// Output layer: sigmoid(hidden * W2 + B2)
	out := na.b2
	for j, h := range hidden {
		out += h * na.w2[j]
	}
	return sigmoid(out), hidden
}

// Select chooses the arm with the highest UCB score: predicted_reward + UCB_bonus.
// ctx is a feature vector; if nil or empty, a zero vector is used.
func (n *NeuralUCB) Select(ctx []float64) Arm {
	n.mu.Lock()
	defer n.mu.Unlock()

	if len(n.order) == 0 {
		return Arm{}
	}

	// Compute total pulls across all arms for the exploration term.
	totalPulls := 0
	for _, id := range n.order {
		totalPulls += n.arms[id].pulls
	}

	// If any arm has never been pulled, explore it first (round-robin).
	for _, id := range n.order {
		if n.arms[id].pulls == 0 {
			return n.arms[id].arm
		}
	}

	// Pad or default the context vector.
	features := n.padContext(ctx)

	bestScore := math.Inf(-1)
	var bestArm Arm

	logTotal := math.Log(float64(totalPulls))
	for _, id := range n.order {
		na := n.arms[id]
		predicted, _ := na.forward(features, n.features)

		// UCB bonus: exploration_c * sqrt(log(total_pulls) / arm_pulls)
		bonus := n.cfg.ExplorationC * math.Sqrt(logTotal/float64(na.pulls))
		score := predicted + bonus

		if score > bestScore {
			bestScore = score
			bestArm = na.arm
		}
	}

	return bestArm
}

// Update records a reward and performs one SGD step on the selected arm's network.
// reward.Context should contain the feature vector used during selection.
// reward.Value is the raw outcome in [0, 1].
func (n *NeuralUCB) Update(reward Reward) {
	n.mu.Lock()
	defer n.mu.Unlock()

	na, ok := n.arms[reward.ArmID]
	if !ok {
		return
	}

	na.pulls++
	na.totalReward += reward.Value

	features := n.padContext(reward.Context)

	// Forward pass.
	predicted, hidden := na.forward(features, n.features)

	// Prediction error.
	err := reward.Value - predicted

	// Backprop through sigmoid output: d_loss/d_pre_sigmoid = err * sigmoid'(out)
	// sigmoid'(out) = predicted * (1 - predicted)
	outputGrad := err * predicted * (1.0 - predicted)

	nf := min(len(features), n.features)

	// Update W2 and B2 (output layer).
	for j, h := range hidden {
		na.w2[j] += n.cfg.LearningRate * outputGrad * h
	}
	na.b2 += n.cfg.LearningRate * outputGrad

	// Backprop through ReLU hidden layer.
	for j := range hidden {
		if hidden[j] <= 0 {
			continue // ReLU gate is closed
		}
		hiddenGrad := outputGrad * na.w2[j]
		for i := range nf {
			na.w1[i][j] += n.cfg.LearningRate * hiddenGrad * features[i]
		}
		na.b1[j] += n.cfg.LearningRate * hiddenGrad
	}
}

// ArmStats returns summary statistics for each arm.
// Alpha and Beta are set to 0 (not applicable to neural policies).
func (n *NeuralUCB) ArmStats() map[string]ArmStat {
	n.mu.Lock()
	defer n.mu.Unlock()

	stats := make(map[string]ArmStat, len(n.arms))
	for _, id := range n.order {
		na := n.arms[id]
		var mean float64
		if na.pulls > 0 {
			mean = na.totalReward / float64(na.pulls)
		}
		stats[id] = ArmStat{
			Pulls:      na.pulls,
			MeanReward: mean,
		}
	}
	return stats
}

// Predict returns the neural network's predicted reward for a given arm and
// context vector, without selecting or updating. Returns 0 if the arm is not found.
func (n *NeuralUCB) Predict(armID string, ctx []float64) float64 {
	n.mu.Lock()
	defer n.mu.Unlock()

	na, ok := n.arms[armID]
	if !ok {
		return 0
	}
	features := n.padContext(ctx)
	predicted, _ := na.forward(features, n.features)
	return predicted
}

// BudgetAdjustedReward computes a composite reward that penalizes cost.
// raw is the quality reward in [0, 1], cost is the normalized cost in [0, 1].
// Returns: raw * (1 - budget_weight) + (1 - cost) * budget_weight
func (n *NeuralUCB) BudgetAdjustedReward(raw, cost float64) float64 {
	return raw*(1.0-n.cfg.BudgetWeight) + (1.0-cost)*n.cfg.BudgetWeight
}

// padContext returns a feature vector of exactly n.features length,
// zero-padding or truncating ctx as needed.
func (n *NeuralUCB) padContext(ctx []float64) []float64 {
	features := make([]float64, n.features)
	copy(features, ctx)
	return features
}

// relu returns max(0, x).
func relu(x float64) float64 {
	if x > 0 {
		return x
	}
	return 0
}
