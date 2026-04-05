package promptdj

import (
	"math"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ConfidenceComponents holds the individual components of a confidence score.
type ConfidenceComponents struct {
	ClassificationConf float64 // 0.0-1.0 from ClassifyDetailed
	QualityScore       float64 // qualityScore / 100
	AffinityStrength   float64 // top affinity entry weight
	HistoricalSuccess  float64 // provider success rate or 0.5 default
	LatencyHealth      float64 // 1.0 healthy, 0.5 degraded, 0.0 down
	DomainSpecificity  float64 // 1.0 specific domain, 0.5 general
}

// Weights for each confidence component. Sum = 1.0.
var confidenceWeights = [6]float64{
	0.25, // classification confidence
	0.20, // quality score
	0.20, // affinity strength
	0.20, // historical success rate
	0.10, // latency health
	0.05, // domain specificity
}

// ComputeConfidence calculates a composite confidence score from components.
func ComputeConfidence(c ConfidenceComponents, banditReady bool, wasEnhanced bool, scoreDelta int) float64 {
	components := [6]float64{
		c.ClassificationConf,
		c.QualityScore,
		c.AffinityStrength,
		c.HistoricalSuccess,
		c.LatencyHealth,
		c.DomainSpecificity,
	}

	var raw float64
	for i, v := range components {
		raw += v * confidenceWeights[i]
	}

	// Penalty for cold-start (no bandit data)
	if !banditReady {
		raw *= 0.85
	}

	// Penalty for enhancement that didn't improve enough
	if wasEnhanced && scoreDelta < 10 {
		raw *= 0.90
	}

	return clamp(raw, 0.0, 1.0)
}

// LatencyHealthScore returns a 0-1 score for a provider's latency health.
func LatencyHealthScore(cascade *session.CascadeRouter, provider session.Provider) float64 {
	if cascade == nil {
		return 0.5 // unknown
	}
	lat := cascade.GetProviderLatency(string(provider))
	if lat == nil || lat.Samples == 0 {
		return 0.5 // no data
	}
	// Healthy if P95 < 30s, degraded if < 60s, unhealthy otherwise
	p95ms := lat.P95.Milliseconds()
	switch {
	case p95ms < 30_000:
		return 1.0
	case p95ms < 60_000:
		return 0.5
	default:
		return 0.0
	}
}

// DomainSpecificityScore returns 1.0 for a specific domain, 0.5 for general.
func DomainSpecificityScore(tags []string) float64 {
	if len(tags) == 0 {
		return 0.5
	}
	for _, t := range tags {
		if t != "general" && t != "" {
			return 1.0
		}
	}
	return 0.5
}

func clamp(v, lo, hi float64) float64 {
	return math.Max(lo, math.Min(hi, v))
}
