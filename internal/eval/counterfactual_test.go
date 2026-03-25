package eval

import (
	"math"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestEvaluatePolicyEmpty(t *testing.T) {
	result := EvaluatePolicy(nil, func(obs session.LoopObservation) float64 { return 1.0 })
	if result.SampleSize != 0 {
		t.Errorf("expected SampleSize=0, got %d", result.SampleSize)
	}
	if result.EstimatedCompletionRate != 0 {
		t.Errorf("expected EstimatedCompletionRate=0, got %f", result.EstimatedCompletionRate)
	}
	if result.EstimatedAvgCost != 0 {
		t.Errorf("expected EstimatedAvgCost=0, got %f", result.EstimatedAvgCost)
	}
	if result.EffectiveSampleSize != 0 {
		t.Errorf("expected EffectiveSampleSize=0, got %f", result.EffectiveSampleSize)
	}
}

func TestEvaluatePolicyUniform(t *testing.T) {
	observations := []session.LoopObservation{
		{VerifyPassed: true, TotalCostUSD: 0.10},
		{VerifyPassed: false, TotalCostUSD: 0.20},
		{VerifyPassed: true, TotalCostUSD: 0.30},
		{VerifyPassed: false, TotalCostUSD: 0.40},
	}

	// Uniform policy: all weights = 1.0.
	result := EvaluatePolicy(observations, func(obs session.LoopObservation) float64 { return 1.0 })

	if result.SampleSize != 4 {
		t.Errorf("expected SampleSize=4, got %d", result.SampleSize)
	}

	// With uniform weights, completion rate = 2/4 = 0.5.
	if math.Abs(result.EstimatedCompletionRate-0.5) > 1e-9 {
		t.Errorf("expected completion rate 0.5, got %f", result.EstimatedCompletionRate)
	}

	// Average cost = (0.10+0.20+0.30+0.40)/4 = 0.25.
	if math.Abs(result.EstimatedAvgCost-0.25) > 1e-9 {
		t.Errorf("expected avg cost 0.25, got %f", result.EstimatedAvgCost)
	}

	// Effective sample size with uniform weights = n.
	if math.Abs(result.EffectiveSampleSize-4.0) > 1e-9 {
		t.Errorf("expected ESS=4, got %f", result.EffectiveSampleSize)
	}
}

func TestCascadeThresholdPolicy(t *testing.T) {
	threshold := 0.6
	policy := CascadeThresholdPolicy(threshold)

	tests := []struct {
		name     string
		obs      session.LoopObservation
		expected float64
	}{
		{
			name:     "not_escalated_high_confidence_agrees",
			obs:      session.LoopObservation{CascadeEscalated: false, Confidence: 0.8},
			expected: 1.0, // conf >= threshold, not escalated → policy would not escalate → agrees
		},
		{
			name:     "escalated_low_confidence_agrees",
			obs:      session.LoopObservation{CascadeEscalated: true, Confidence: 0.4},
			expected: 1.0, // conf < threshold, escalated → policy would escalate → agrees
		},
		{
			name:     "not_escalated_low_confidence_disagrees",
			obs:      session.LoopObservation{CascadeEscalated: false, Confidence: 0.4},
			expected: 0.1, // conf < threshold, not escalated → policy would escalate → disagrees
		},
		{
			name:     "escalated_high_confidence_disagrees",
			obs:      session.LoopObservation{CascadeEscalated: true, Confidence: 0.8},
			expected: 0.1, // conf >= threshold, escalated → policy would not escalate → disagrees
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := policy(tc.obs)
			if math.Abs(got-tc.expected) > 1e-9 {
				t.Errorf("expected %f, got %f", tc.expected, got)
			}
		})
	}
}

func TestProviderRoutingPolicy(t *testing.T) {
	policy := ProviderRoutingPolicy("refactor", "gemini")

	tests := []struct {
		name     string
		obs      session.LoopObservation
		expected float64
	}{
		{
			name:     "different_task_type_unchanged",
			obs:      session.LoopObservation{TaskType: "bugfix", WorkerProvider: "claude"},
			expected: 1.0,
		},
		{
			name:     "matching_task_matching_provider",
			obs:      session.LoopObservation{TaskType: "refactor", WorkerProvider: "gemini"},
			expected: 1.0,
		},
		{
			name:     "matching_task_different_provider",
			obs:      session.LoopObservation{TaskType: "refactor", WorkerProvider: "claude"},
			expected: 0.1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := policy(tc.obs)
			if math.Abs(got-tc.expected) > 1e-9 {
				t.Errorf("expected %f, got %f", tc.expected, got)
			}
		})
	}
}

func TestCounterfactualWithSyntheticData(t *testing.T) {
	// 100 observations: 50 passed (conf=0.8), 50 failed (conf=0.4).
	var observations []session.LoopObservation
	for i := 0; i < 50; i++ {
		observations = append(observations, session.LoopObservation{
			VerifyPassed:     true,
			Confidence:       0.8,
			CascadeEscalated: false,
			TotalCostUSD:     0.05,
		})
	}
	for i := 0; i < 50; i++ {
		observations = append(observations, session.LoopObservation{
			VerifyPassed:     false,
			Confidence:       0.4,
			CascadeEscalated: false,
			TotalCostUSD:     0.10,
		})
	}

	// Lowering threshold to 0.5: the low-confidence (0.4) observations get
	// downweighted (policy would escalate them but they weren't escalated),
	// so the estimated completion rate should be higher than with threshold 0.9
	// which downweights the high-confidence (0.8) observations.
	lowThreshold := EvaluatePolicy(observations, CascadeThresholdPolicy(0.5))
	highThreshold := EvaluatePolicy(observations, CascadeThresholdPolicy(0.9))

	if lowThreshold.EstimatedCompletionRate <= highThreshold.EstimatedCompletionRate {
		t.Errorf("expected low threshold (0.5) to estimate higher completion rate than high threshold (0.9): got %f <= %f",
			lowThreshold.EstimatedCompletionRate, highThreshold.EstimatedCompletionRate)
	}

	// Both should have valid sample sizes.
	if lowThreshold.SampleSize != 100 {
		t.Errorf("expected SampleSize=100, got %d", lowThreshold.SampleSize)
	}

	// ESS should be less than n when weights are non-uniform.
	if highThreshold.EffectiveSampleSize >= 100.0 {
		t.Errorf("expected ESS < 100 with non-uniform weights, got %f", highThreshold.EffectiveSampleSize)
	}

	// CI should bracket the estimate.
	if lowThreshold.Confidence95[0] > lowThreshold.EstimatedCompletionRate {
		t.Errorf("CI lower bound %f exceeds estimate %f",
			lowThreshold.Confidence95[0], lowThreshold.EstimatedCompletionRate)
	}
	if lowThreshold.Confidence95[1] < lowThreshold.EstimatedCompletionRate {
		t.Errorf("CI upper bound %f below estimate %f",
			lowThreshold.Confidence95[1], lowThreshold.EstimatedCompletionRate)
	}
}
