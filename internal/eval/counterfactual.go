package eval

import (
	"math"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// PolicyFunc returns the probability that the proposed policy would have
// selected the same action as was actually taken for this observation.
// Returns a value in (0, 1]. Values close to 0 mean the new policy would
// rarely take this action; close to 1 means it would often take it.
type PolicyFunc func(obs session.LoopObservation) float64

// CounterfactualResult holds the estimated outcome under a hypothetical policy.
type CounterfactualResult struct {
	EstimatedCompletionRate float64    `json:"estimated_completion_rate"`
	EstimatedAvgCost        float64    `json:"estimated_avg_cost"`
	SampleSize              int        `json:"sample_size"`
	EffectiveSampleSize     float64    `json:"effective_sample_size"` // accounts for IPS weight variance
	Confidence95            [2]float64 `json:"confidence_95"`        // 95% CI for completion rate
}

// EvaluatePolicy uses self-normalized inverse propensity scoring to estimate
// the outcome of a counterfactual policy from logged observation data.
func EvaluatePolicy(observations []session.LoopObservation, policy PolicyFunc) CounterfactualResult {
	n := len(observations)
	if n == 0 {
		return CounterfactualResult{}
	}

	// Step 1: compute raw importance weights.
	weights := make([]float64, n)
	var sumW, sumW2 float64
	for i, obs := range observations {
		w := policy(obs)
		if w <= 0 {
			w = 1e-10 // clamp to avoid zero
		}
		if w > 1 {
			w = 1
		}
		weights[i] = w
		sumW += w
		sumW2 += w * w
	}

	// Step 2: self-normalized weights and weighted averages.
	var estCompletion, estCost float64
	for i, obs := range observations {
		wNorm := weights[i] / sumW
		if obs.VerifyPassed {
			estCompletion += wNorm
		}
		estCost += wNorm * obs.TotalCostUSD
	}

	// Step 3: effective sample size.
	ess := (sumW * sumW) / sumW2

	// Step 4: 95% CI using normal approximation on the weighted mean.
	// Compute weighted variance of the completion indicator.
	var sumWVar float64
	for i, obs := range observations {
		wNorm := weights[i] / sumW
		y := 0.0
		if obs.VerifyPassed {
			y = 1.0
		}
		diff := y - estCompletion
		sumWVar += wNorm * diff * diff
	}
	// Standard error using effective sample size.
	se := math.Sqrt(sumWVar / ess)
	ci := [2]float64{
		estCompletion - 1.96*se,
		estCompletion + 1.96*se,
	}

	return CounterfactualResult{
		EstimatedCompletionRate: estCompletion,
		EstimatedAvgCost:        estCost,
		SampleSize:              n,
		EffectiveSampleSize:     ess,
		Confidence95:            ci,
	}
}

// CascadeThresholdPolicy returns a PolicyFunc that simulates changing the
// cascade confidence threshold. It assigns high probability (1.0) when the
// new policy agrees with the logged action, and low probability (0.1) when
// it disagrees.
func CascadeThresholdPolicy(newThreshold float64) PolicyFunc {
	return func(obs session.LoopObservation) float64 {
		wouldEscalate := obs.Confidence < newThreshold
		if obs.CascadeEscalated == wouldEscalate {
			return 1.0 // policy agrees with logged action
		}
		return 0.1 // policy disagrees
	}
}

// ProviderRoutingPolicy returns a PolicyFunc simulating routing all tasks of
// a given type to a specific provider. Observations with non-matching task
// types are left unchanged (weight 1.0).
func ProviderRoutingPolicy(taskType string, provider string) PolicyFunc {
	return func(obs session.LoopObservation) float64 {
		if obs.TaskType != taskType {
			return 1.0 // not affected by this policy
		}
		if obs.WorkerProvider == provider {
			return 1.0 // policy agrees
		}
		return 0.1 // policy disagrees
	}
}
