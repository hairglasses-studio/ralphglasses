package session

import (
	"math"
	"sync"
)

// SuccessCalibration holds sigmoid parameters for a provider's success probability
// as a function of prompt quality score. Calibrated via EMA from outcomes.
type SuccessCalibration struct {
	BaseRate float64 // minimum success probability (0-1)
	Midpoint float64 // quality score where success prob is ~50% of range above base
	Scale    float64 // sigmoid steepness
}

// CostEstimate is the output of the cost-aware predictor.
type CostEstimate struct {
	Provider           Provider `json:"provider"`
	ExpectedCostUSD    float64  `json:"expected_cost_usd"`
	BaseCostUSD        float64  `json:"base_cost_usd"`
	RetryMultiplier    float64  `json:"retry_multiplier"`
	CacheDiscountUSD   float64  `json:"cache_discount_usd"`
	SuccessProbability float64  `json:"success_probability"`
	ConfidenceWidth    float64  `json:"confidence_width"` // upper bound factor
}

// CostAwarePredictor wraps CostPredictor with prompt quality signals.
type CostAwarePredictor struct {
	base        *CostPredictor
	cache       *CacheManager
	mu          sync.RWMutex
	calibration map[Provider]*SuccessCalibration
}

// NewCostAwarePredictor creates a quality-aware cost predictor.
func NewCostAwarePredictor(base *CostPredictor, cache *CacheManager) *CostAwarePredictor {
	return &CostAwarePredictor{
		base:  base,
		cache: cache,
		calibration: map[Provider]*SuccessCalibration{
			ProviderGemini: {BaseRate: 0.30, Midpoint: 60, Scale: 12},
			ProviderClaude: {BaseRate: 0.50, Midpoint: 45, Scale: 15},
			ProviderCodex:  {BaseRate: 0.45, Midpoint: 50, Scale: 14},
		},
	}
}

// PredictCost estimates the expected cost for a prompt at a given provider,
// factoring in prompt quality (retry probability) and cache affinity.
func (p *CostAwarePredictor) PredictCost(prompt string, qualityScore int, taskType string, provider Provider) CostEstimate {
	// Base cost from historical average (or $1.00 default)
	baseCost := 1.0
	if p.base != nil {
		baseCost = p.base.Predict(taskType, string(provider))
	}

	// Success probability via sigmoid
	succProb := p.successProbability(qualityScore, provider)

	// Retry multiplier: expected total cost including retries
	retryMult := 1.0
	if succProb > 0.01 {
		retryMult = 1.0 / succProb
	}

	// Cache discount
	var cacheDiscount float64
	if p.cache != nil {
		prefixLen, hit := p.cache.LookupPrefix(provider, prompt)
		if hit {
			coverage := float64(prefixLen) / float64(max(len(prompt), 1))
			savingsRate := p.cacheSavingsRate(provider)
			cacheDiscount = baseCost * coverage * savingsRate
		}
	}

	expected := baseCost*retryMult - cacheDiscount
	if expected < 0 {
		expected = 0.001
	}

	// Confidence interval width based on observation count
	obsCount := 0
	if p.base != nil {
		obsCount = p.base.ObservationCount()
	}
	confWidth := 3.0 // cold-start: 3x
	if obsCount > 0 {
		confWidth = 1.0 + 2.0/math.Sqrt(float64(obsCount))
	}

	return CostEstimate{
		Provider:           provider,
		ExpectedCostUSD:    expected,
		BaseCostUSD:        baseCost,
		RetryMultiplier:    retryMult,
		CacheDiscountUSD:   cacheDiscount,
		SuccessProbability: succProb,
		ConfidenceWidth:    confWidth,
	}
}

// successProbability computes P(success) as a function of quality score using
// a sigmoid: baseRate + (1 - baseRate) * sigmoid((quality - midpoint) / scale)
func (p *CostAwarePredictor) successProbability(qualityScore int, provider Provider) float64 {
	p.mu.RLock()
	cal, ok := p.calibration[provider]
	p.mu.RUnlock()

	if !ok {
		// Default: moderate sigmoid
		cal = &SuccessCalibration{BaseRate: 0.50, Midpoint: 50, Scale: 15}
	}

	x := (float64(qualityScore) - cal.Midpoint) / cal.Scale
	sigmoid := 1.0 / (1.0 + math.Exp(-x))
	return cal.BaseRate + (1.0-cal.BaseRate)*sigmoid
}

// CalibrateFromOutcome updates sigmoid parameters via EMA from a single outcome.
func (p *CostAwarePredictor) CalibrateFromOutcome(provider Provider, qualityScore int, success bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	cal, ok := p.calibration[provider]
	if !ok {
		cal = &SuccessCalibration{BaseRate: 0.50, Midpoint: 50, Scale: 15}
		p.calibration[provider] = cal
	}

	alpha := 0.05 // learning rate

	if success {
		if float64(qualityScore) < cal.Midpoint {
			cal.Midpoint -= alpha * (cal.Midpoint - float64(qualityScore))
		}
		cal.BaseRate += alpha * (1.0 - cal.BaseRate) * 0.1
	} else {
		if float64(qualityScore) > cal.Midpoint {
			cal.Midpoint += alpha * (float64(qualityScore) - cal.Midpoint)
		}
		cal.BaseRate -= alpha * cal.BaseRate * 0.1
	}

	// Clamp
	cal.BaseRate = math.Max(0.05, math.Min(0.95, cal.BaseRate))
	cal.Midpoint = math.Max(10, math.Min(90, cal.Midpoint))
}

// cacheSavingsRate returns the per-provider cache savings rate.
func (p *CostAwarePredictor) cacheSavingsRate(provider Provider) float64 {
	rates := map[Provider]float64{
		ProviderClaude: 0.0,  // cache disabled (regressed in field)
		ProviderGemini: 0.75, // 75% input reduction
		ProviderCodex:  0.50, // automatic prefix caching
	}
	if r, ok := rates[provider]; ok {
		return r
	}
	return 0.0
}
