package session

import (
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"time"
)

// ConfidenceFeatures is the feature vector for the calibrated confidence model.
type ConfidenceFeatures struct {
	TaskTypeHash      float64 // hashed to [0,1] range
	ProviderID        float64 // 0.0=claude, 0.33=gemini, 0.66=codex
	TurnRatio         float64 // actual/expected turns, clamped to [0, 3]
	HedgeCount        float64 // normalized 0-1 (count/10, clamped)
	VerifyPassed      float64 // 0.0 or 1.0
	ErrorFree         float64 // 0.0 or 1.0
	QuestionCount     float64 // normalized 0-1 (count/5, clamped)
	OutputLength      float64 // log-normalized: log(1+length)/10, clamped to [0,1]
	DifficultyScore   float64 // 0.0-1.0 from curriculum
	EpisodesAvailable float64 // normalized: min(count, 10)/10
}

// DecisionModel provides calibrated confidence predictions using logistic
// regression trained on LoopObservation data. When untrained it falls back
// to the legacy heuristic weights.
type DecisionModel struct {
	mu          sync.Mutex
	weights     [10]float64
	bias        float64
	calibration []calibrationBin
	trained     bool
	trainedAt   time.Time
	sampleCount int
	minSamples  int
}

type calibrationBin struct {
	Lower    float64
	Upper    float64
	Observed float64 // observed success rate in this bin
}

// NewDecisionModel returns an untrained model with initial weights that
// approximate the existing heuristic scoring.
func NewDecisionModel() *DecisionModel {
	return &DecisionModel{
		// indices: TaskTypeHash, ProviderID, TurnRatio, HedgeCount, VerifyPassed,
		//          ErrorFree, QuestionCount, OutputLength, DifficultyScore, EpisodesAvailable
		weights:    [10]float64{0.0, 0.0, 0.20, -0.25, 0.30, 0.15, -0.10, 0.0, 0.0, 0.0},
		bias:       0.0,
		trained:    false,
		minSamples: 50,
	}
}

// Predict returns a calibrated confidence score in [0, 1].
// When the model is untrained it uses a deterministic heuristic fallback.
func (dm *DecisionModel) Predict(features ConfidenceFeatures) float64 {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	if !dm.trained {
		return dm.heuristicPredict(features)
	}

	fv := featureVector(features)
	z := dm.bias
	for i := 0; i < 10; i++ {
		z += dm.weights[i] * fv[i]
	}
	score := sigmoid(z)

	// Apply isotonic calibration if available.
	if len(dm.calibration) > 0 {
		score = dm.applyCalibration(score)
	}

	return clamp01(score)
}

// heuristicPredict matches the legacy ExtractConfidence formula.
func (dm *DecisionModel) heuristicPredict(f ConfidenceFeatures) float64 {
	score := 0.30*f.VerifyPassed +
		0.25*(1-f.HedgeCount) +
		0.20*f.TurnRatio +
		0.15*f.ErrorFree +
		0.10*(1-f.QuestionCount)
	return clamp01(score)
}

// applyCalibration performs isotonic calibration lookup.
func (dm *DecisionModel) applyCalibration(raw float64) float64 {
	for _, bin := range dm.calibration {
		if raw >= bin.Lower && raw < bin.Upper {
			return bin.Observed
		}
	}
	// If above all bins, return last bin's observed rate.
	if len(dm.calibration) > 0 {
		return dm.calibration[len(dm.calibration)-1].Observed
	}
	return raw
}

// Train fits logistic regression weights via gradient descent on the given
// observations. It requires at least dm.minSamples observations.
func (dm *DecisionModel) Train(observations []LoopObservation) error {
	if len(observations) < dm.minSamples {
		return fmt.Errorf("insufficient data: need %d observations, got %d", dm.minSamples, len(observations))
	}

	type sample struct {
		features [10]float64
		label    float64
	}

	samples := make([]sample, len(observations))
	for i, obs := range observations {
		cf := observationToFeatures(obs)
		label := 0.0
		if obs.VerifyPassed {
			label = 1.0
		}
		samples[i] = sample{features: featureVector(cf), label: label}
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	// Initialize from current weights.
	w := dm.weights
	b := dm.bias
	lr := 0.01
	epochs := 50
	n := float64(len(samples))

	for epoch := 0; epoch < epochs; epoch++ {
		var gradW [10]float64
		var gradB float64

		for _, s := range samples {
			z := b
			for j := 0; j < 10; j++ {
				z += w[j] * s.features[j]
			}
			pred := sigmoid(z)
			diff := pred - s.label

			gradB += diff
			for j := 0; j < 10; j++ {
				gradW[j] += diff * s.features[j]
			}
		}

		b -= lr * (gradB / n)
		for j := 0; j < 10; j++ {
			w[j] -= lr * (gradW[j] / n)
		}
	}

	dm.weights = w
	dm.bias = b
	dm.trained = true
	dm.trainedAt = time.Now()
	dm.sampleCount = len(observations)

	// Auto-calibrate using the training data.
	dm.mu.Unlock()
	err := dm.Calibrate(observations)
	dm.mu.Lock()
	return err
}

// Calibrate builds isotonic calibration bins from predictions on the given
// observations. The model must be trained first.
func (dm *DecisionModel) Calibrate(observations []LoopObservation) error {
	if len(observations) == 0 {
		return fmt.Errorf("no observations for calibration")
	}

	type scored struct {
		pred  float64
		label float64
	}

	items := make([]scored, len(observations))
	for i, obs := range observations {
		cf := observationToFeatures(obs)
		pred := dm.Predict(cf)
		label := 0.0
		if obs.VerifyPassed {
			label = 1.0
		}
		items[i] = scored{pred: pred, label: label}
	}

	// Sort by predicted score (insertion sort — observations are typically small).
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].pred < items[j-1].pred; j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}

	// Create 10 equal-frequency bins.
	numBins := 10
	if len(items) < numBins {
		numBins = len(items)
	}
	binSize := len(items) / numBins

	dm.mu.Lock()
	defer dm.mu.Unlock()

	dm.calibration = make([]calibrationBin, 0, numBins)
	for b := 0; b < numBins; b++ {
		start := b * binSize
		end := start + binSize
		if b == numBins-1 {
			end = len(items) // last bin gets remainder
		}
		if start >= len(items) {
			break
		}

		sumLabels := 0.0
		count := 0
		for k := start; k < end; k++ {
			sumLabels += items[k].label
			count++
		}

		lower := items[start].pred
		upper := 1.01 // sentinel for last bin
		if b < numBins-1 && end < len(items) {
			upper = items[end].pred
		}

		observed := 0.0
		if count > 0 {
			observed = sumLabels / float64(count)
		}

		dm.calibration = append(dm.calibration, calibrationBin{
			Lower:    lower,
			Upper:    upper,
			Observed: observed,
		})
	}

	return nil
}

// AdaptThreshold finds the escalation threshold that minimizes a weighted
// cost function over the observations. False negatives (missed failures)
// are penalised 2x versus false positives.
func (dm *DecisionModel) AdaptThreshold(observations []LoopObservation) float64 {
	if len(observations) == 0 {
		return 0.7
	}

	type predLabel struct {
		pred  float64
		label float64
	}

	items := make([]predLabel, len(observations))
	for i, obs := range observations {
		cf := observationToFeatures(obs)
		items[i] = predLabel{
			pred:  dm.Predict(cf),
			label: boolToFloat(obs.VerifyPassed),
		}
	}

	bestThreshold := 0.7
	bestCost := math.MaxFloat64

	for t := 0.30; t <= 0.90+1e-9; t += 0.05 {
		var fn, fp float64
		for _, it := range items {
			if it.pred >= t && it.label == 0.0 {
				fn++ // predicted confident but actually failed
			}
			if it.pred < t && it.label == 1.0 {
				fp++ // predicted low-confidence but actually passed
			}
		}
		cost := 2*fn + fp
		if cost < bestCost {
			bestCost = cost
			bestThreshold = math.Round(t*100) / 100 // round to 2 decimal places
		}
	}

	return bestThreshold
}

// IsTrained reports whether the model has been fitted to data.
func (dm *DecisionModel) IsTrained() bool {
	dm.mu.Lock()
	defer dm.mu.Unlock()
	return dm.trained
}

// PredictConfidence adapts the DecisionModel to the CascadeRouter's expected
// interface: (turnCount, expectedTurns, lastOutput, verifyPassed) → confidence.
// It constructs ConfidenceFeatures from the available parameters and delegates
// to Predict.
func (dm *DecisionModel) PredictConfidence(turnCount, expectedTurns int, lastOutput string, verifyPassed bool) float64 {
	var turnRatio float64
	if expectedTurns > 0 {
		turnRatio = float64(turnCount) / float64(expectedTurns)
		if turnRatio > 3 {
			turnRatio = 3
		}
	}

	var vp float64
	if verifyPassed {
		vp = 1.0
	}

	// Estimate hedge count from output text (simple heuristic).
	hedge := float64(countSubstrings(lastOutput, []string{
		"maybe", "possibly", "might", "could be", "not sure",
		"i think", "perhaps", "unclear", "uncertain", "roughly",
	})) / 10.0
	if hedge > 1 {
		hedge = 1
	}

	outputLen := math.Log1p(float64(len(lastOutput))) / 10.0
	if outputLen > 1 {
		outputLen = 1
	}

	features := ConfidenceFeatures{
		TurnRatio:    turnRatio,
		HedgeCount:   hedge,
		VerifyPassed: vp,
		ErrorFree:    vp, // proxy: verify passed ≈ error free
		OutputLength: outputLen,
	}
	return dm.Predict(features)
}

// Stats returns model metadata suitable for JSON serialization.
func (dm *DecisionModel) Stats() map[string]any {
	dm.mu.Lock()
	defer dm.mu.Unlock()

	w := make([]float64, 10)
	copy(w, dm.weights[:])

	return map[string]any{
		"weights":      w,
		"bias":         dm.bias,
		"trained":      dm.trained,
		"trained_at":   dm.trainedAt,
		"sample_count": dm.sampleCount,
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func featureVector(f ConfidenceFeatures) [10]float64 {
	return [10]float64{
		f.TaskTypeHash,
		f.ProviderID,
		f.TurnRatio,
		f.HedgeCount,
		f.VerifyPassed,
		f.ErrorFree,
		f.QuestionCount,
		f.OutputLength,
		f.DifficultyScore,
		f.EpisodesAvailable,
	}
}

func sigmoid(z float64) float64 {
	if z < -500 {
		return 0
	}
	if z > 500 {
		return 1
	}
	return 1.0 / (1.0 + math.Exp(-z))
}

func hashTaskType(taskType string) float64 {
	h := fnv.New32a()
	h.Write([]byte(taskType))
	return float64(h.Sum32()%100) / 100.0
}

func providerToFloat(provider string) float64 {
	switch provider {
	case "claude":
		return 0.0
	case "gemini":
		return 0.33
	case "codex":
		return 0.66
	default:
		return 0.0
	}
}

func observationToFeatures(obs LoopObservation) ConfidenceFeatures {
	verify := 0.0
	if obs.VerifyPassed {
		verify = 1.0
	}
	errorFree := 1.0
	if obs.Error != "" {
		errorFree = 0.0
	}

	// Approximate hedge count from stored confidence: low confidence implies hedging.
	hedge := 0.0
	if obs.Confidence < 0.5 {
		hedge = (0.5 - obs.Confidence) * 2
	}

	// Turn ratio approximation: use latency ratio if available.
	turnRatio := 1.0
	if obs.TotalLatencyMs > 0 && obs.WorkerLatencyMs > 0 {
		turnRatio = float64(obs.TotalLatencyMs) / float64(obs.WorkerLatencyMs)
		if turnRatio > 3 {
			turnRatio = 3
		}
	}

	episodes := float64(obs.EpisodesUsed)
	if episodes > 10 {
		episodes = 10
	}

	return ConfidenceFeatures{
		TaskTypeHash:      hashTaskType(obs.TaskType),
		ProviderID:        providerToFloat(obs.WorkerProvider),
		TurnRatio:         turnRatio,
		HedgeCount:        clamp01(hedge),
		VerifyPassed:      verify,
		ErrorFree:         errorFree,
		QuestionCount:     0,   // not available from observation
		OutputLength:      0.5, // not available from observation
		DifficultyScore:   obs.DifficultyScore,
		EpisodesAvailable: episodes / 10.0,
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func boolToFloat(b bool) float64 {
	if b {
		return 1.0
	}
	return 0.0
}

// countSubstrings counts how many of the given substrings appear in s (case-insensitive).
func countSubstrings(s string, subs []string) int {
	lower := strings.ToLower(s)
	count := 0
	for _, sub := range subs {
		if strings.Contains(lower, sub) {
			count++
		}
	}
	return count
}
