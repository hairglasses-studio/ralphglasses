package promptdj

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// RoutingStrategy defines a provider routing configuration to A/B test.
type RoutingStrategy struct {
	Name           string  `json:"name"`
	Description    string  `json:"description"`
	EnhanceThreshold int   `json:"enhance_threshold"` // quality gate threshold
	MinConfidence  float64 `json:"min_confidence"`    // escalation threshold
	PreferProvider string  `json:"prefer_provider"`   // provider bias (empty = use affinity)
}

// ABRoutingTest defines an A/B comparison between two routing strategies.
type ABRoutingTest struct {
	Name       string           `json:"name"`
	StrategyA  RoutingStrategy  `json:"strategy_a"`
	StrategyB  RoutingStrategy  `json:"strategy_b"`
	SampleSize int              `json:"sample_size"` // prompts to test each strategy
	Prompts    []string         `json:"prompts"`     // test corpus
}

// ABRoutingResult holds the outcome of a routing A/B test.
type ABRoutingResult struct {
	TestName   string  `json:"test_name"`
	StrategyA  ABStrategyResult `json:"strategy_a"`
	StrategyB  ABStrategyResult `json:"strategy_b"`
	Winner     string  `json:"winner"` // "A", "B", or "tie"
	Confidence float64 `json:"confidence"`
	Duration   time.Duration `json:"duration"`
}

// ABStrategyResult holds metrics for one strategy variant.
type ABStrategyResult struct {
	Name             string  `json:"name"`
	AvgConfidence    float64 `json:"avg_confidence"`
	AvgScore         float64 `json:"avg_score"`
	AvgCostEstimate  float64 `json:"avg_cost_estimate"`
	EnhancedPct      float64 `json:"enhanced_pct"`
	ProviderDistribution map[string]int `json:"provider_distribution"`
}

// ABRoutingRunner executes A/B routing strategy comparisons.
type ABRoutingRunner struct {
	router *PromptDJRouter
	mu     sync.Mutex
}

// NewABRoutingRunner creates a runner attached to a DJ router.
func NewABRoutingRunner(router *PromptDJRouter) *ABRoutingRunner {
	return &ABRoutingRunner{router: router}
}

// Run executes the A/B test by routing each prompt under both strategies
// and comparing the routing decisions.
func (r *ABRoutingRunner) Run(ctx context.Context, test ABRoutingTest) (*ABRoutingResult, error) {
	start := time.Now()

	if len(test.Prompts) == 0 {
		return nil, fmt.Errorf("no prompts provided for A/B test")
	}

	prompts := test.Prompts
	if test.SampleSize > 0 && test.SampleSize < len(prompts) {
		prompts = prompts[:test.SampleSize]
	}

	resultA := r.runStrategy(ctx, test.StrategyA, prompts)
	resultB := r.runStrategy(ctx, test.StrategyB, prompts)

	// Determine winner
	winner := "tie"
	confidence := 0.5

	// Score each strategy: weighted composite of confidence, cost efficiency, enhancement rate
	scoreA := resultA.AvgConfidence*0.4 + (1.0-resultA.EnhancedPct)*0.3 + (1.0/(1.0+resultA.AvgCostEstimate))*0.3
	scoreB := resultB.AvgConfidence*0.4 + (1.0-resultB.EnhancedPct)*0.3 + (1.0/(1.0+resultB.AvgCostEstimate))*0.3

	delta := scoreA - scoreB
	if delta > 0.05 {
		winner = "A"
		confidence = 0.5 + delta*5 // scale to 0.5-1.0
	} else if delta < -0.05 {
		winner = "B"
		confidence = 0.5 - delta*5
	}
	if confidence > 1.0 {
		confidence = 1.0
	}

	return &ABRoutingResult{
		TestName:   test.Name,
		StrategyA:  resultA,
		StrategyB:  resultB,
		Winner:     winner,
		Confidence: confidence,
		Duration:   time.Since(start),
	}, nil
}

// runStrategy routes all prompts under a given strategy configuration.
func (r *ABRoutingRunner) runStrategy(ctx context.Context, strategy RoutingStrategy, prompts []string) ABStrategyResult {
	result := ABStrategyResult{
		Name:                 strategy.Name,
		ProviderDistribution: make(map[string]int),
	}

	// Temporarily override router config
	r.mu.Lock()
	origConfig := r.router.config
	testConfig := origConfig
	if strategy.EnhanceThreshold > 0 {
		testConfig.EnhanceThreshold = strategy.EnhanceThreshold
	}
	if strategy.MinConfidence > 0 {
		testConfig.MinConfidence = strategy.MinConfidence
	}
	r.router.config = testConfig
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		r.router.config = origConfig
		r.mu.Unlock()
	}()

	var totalConf, totalScore, totalCost float64
	var enhancedCount int

	for _, prompt := range prompts {
		d, err := r.router.Route(ctx, RoutingRequest{Prompt: prompt})
		if err != nil {
			continue
		}
		totalConf += d.Confidence
		totalScore += float64(d.OriginalScore)
		totalCost += d.EstimatedCostUSD
		if d.WasEnhanced {
			enhancedCount++
		}
		result.ProviderDistribution[string(d.Provider)]++
	}

	n := float64(len(prompts))
	if n > 0 {
		result.AvgConfidence = totalConf / n
		result.AvgScore = totalScore / n
		result.AvgCostEstimate = totalCost / n
		result.EnhancedPct = float64(enhancedCount) / n
	}

	return result
}
