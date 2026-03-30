package eval

import (
	"strings"
	"testing"
)

func TestBuildRecommendation_NoSignificance(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.6, ProbBBetter: 0.4}
	freq := &FrequentistResult{Significant: false}
	got := buildRecommendation(bayesian, freq, false, false, false, false, false)
	if !strings.Contains(got, "No significant difference") {
		t.Errorf("got %q, want 'No significant difference'", got)
	}
}

func TestBuildRecommendation_BothFavorA(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.97, ProbBBetter: 0.03}
	freq := &FrequentistResult{Significant: true}
	got := buildRecommendation(bayesian, freq, true, false, true, false, true)
	if !strings.Contains(got, "favor variant A") {
		t.Errorf("got %q, want 'favor variant A'", got)
	}
}

func TestBuildRecommendation_BothFavorB(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.03, ProbBBetter: 0.97}
	freq := &FrequentistResult{Significant: true}
	got := buildRecommendation(bayesian, freq, false, true, false, true, true)
	if !strings.Contains(got, "favor variant B") {
		t.Errorf("got %q, want 'favor variant B'", got)
	}
}

func TestBuildRecommendation_FreqSignificantBayesianUncertain(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.6, ProbBBetter: 0.4}
	freq := &FrequentistResult{Significant: true}
	got := buildRecommendation(bayesian, freq, false, false, true, false, false)
	if !strings.Contains(got, "Frequentist test is significant but Bayesian") {
		t.Errorf("got %q, want frequentist-significant message", got)
	}
}

func TestBuildRecommendation_BayesianFavorsAFreqNot(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.97, ProbBBetter: 0.03}
	freq := &FrequentistResult{Significant: false}
	got := buildRecommendation(bayesian, freq, true, false, false, false, false)
	if !strings.Contains(got, "Bayesian analysis favors variant A") {
		t.Errorf("got %q, want Bayesian-favors-A message", got)
	}
}

func TestBuildRecommendation_BayesianFavorsBFreqNot(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.03, ProbBBetter: 0.97}
	freq := &FrequentistResult{Significant: false}
	got := buildRecommendation(bayesian, freq, false, true, false, false, false)
	if !strings.Contains(got, "Bayesian analysis favors variant B") {
		t.Errorf("got %q, want Bayesian-favors-B message", got)
	}
}

func TestBuildRecommendation_MixedSignals(t *testing.T) {
	bayesian := &ABTestResult{ProbABetter: 0.8, ProbBBetter: 0.2}
	freq := &FrequentistResult{Significant: true}
	// freq.Significant=true, bayesianFavorsA=true, but agreement=false (freqFavorsB)
	// This falls through to "Mixed signals".
	got := buildRecommendation(bayesian, freq, true, false, false, true, false)
	if !strings.Contains(got, "Mixed signals") {
		t.Errorf("got %q, want 'Mixed signals'", got)
	}
}
