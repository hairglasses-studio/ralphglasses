package fleet

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// RecommendationType categorizes a config recommendation.
type RecommendationType string

const (
	RecommendProviderSwitch  RecommendationType = "provider_switch"
	RecommendBudgetPacing    RecommendationType = "budget_pacing"
	RecommendAnomalyResponse RecommendationType = "anomaly_response"
	RecommendCacheOptimize   RecommendationType = "cache_optimize"
	RecommendModelDowngrade  RecommendationType = "model_downgrade"
)

// Recommendation is an actionable config change derived from cost history.
type Recommendation struct {
	Type        RecommendationType `json:"type"`
	Description string             `json:"description"`
	Impact      float64            `json:"impact"`     // estimated savings as a fraction (0.0-1.0)
	Confidence  float64            `json:"confidence"` // 0.0-1.0
	Config      map[string]any     `json:"config"`     // suggested config changes
}

// RecommenderConfig tunes recommendation thresholds.
type RecommenderConfig struct {
	// MinSamplesPerProvider is the minimum samples required for a provider
	// to participate in cost comparison. Default: 5.
	MinSamplesPerProvider int

	// AnomalyZThreshold is the z-score above which a provider is flagged
	// for anomalous cost. Default: 2.0.
	AnomalyZThreshold float64

	// CacheSavingsEstimate is the estimated fraction of input cost saved
	// by enabling prompt caching. Default: 0.80.
	CacheSavingsEstimate float64

	// ModelDowngradeQualityThreshold is the max acceptable quality delta
	// for a model downgrade recommendation. Default: 0.05 (5%).
	ModelDowngradeQualityThreshold float64

	// BudgetRemaining is the remaining budget in USD. When > 0, pacing
	// recommendations are generated.
	BudgetRemaining float64

	// Concurrency is the current session concurrency. Used for pacing
	// recommendations.
	Concurrency int

	// BudgetHours is the desired runway in hours. Default: 8.
	BudgetHours float64
}

// DefaultRecommenderConfig returns sensible defaults.
func DefaultRecommenderConfig() RecommenderConfig {
	return RecommenderConfig{
		MinSamplesPerProvider:          5,
		AnomalyZThreshold:              2.0,
		CacheSavingsEstimate:           0.80,
		ModelDowngradeQualityThreshold: 0.05,
		BudgetHours:                    8,
	}
}

// Recommender analyzes cost history from a CostPredictor and generates
// actionable configuration recommendations.
type Recommender struct {
	predictor *CostPredictor
	config    RecommenderConfig
}

// NewRecommender creates a Recommender backed by the given predictor.
func NewRecommender(predictor *CostPredictor) *Recommender {
	return &Recommender{
		predictor: predictor,
		config:    DefaultRecommenderConfig(),
	}
}

// WithConfig returns the Recommender with the given config applied.
func (r *Recommender) WithConfig(cfg RecommenderConfig) *Recommender {
	r.config = cfg
	return r
}

// providerStats holds aggregated cost statistics for a single provider.
type providerStats struct {
	Provider   string
	Samples    int
	TotalCost  float64
	MeanCost   float64
	StddevCost float64
	TaskTypes  map[string]taskStats
}

// taskStats holds per-task-type cost statistics within a provider.
type taskStats struct {
	Samples   int
	TotalCost float64
	MeanCost  float64
}

// Analyze inspects the predictor's cost history and returns prioritized
// recommendations. Returns nil if there is insufficient data.
func (r *Recommender) Analyze() []Recommendation {
	samples := r.predictor.Samples()
	if len(samples) < 2 {
		return nil
	}

	stats := r.buildStats(samples)

	var recs []Recommendation

	// 1. Provider cost comparison per task type.
	recs = append(recs, r.providerSwitchRecs(stats)...)

	// 2. Budget pacing.
	recs = append(recs, r.budgetPacingRecs(samples)...)

	// 3. Anomaly response.
	recs = append(recs, r.anomalyResponseRecs(stats)...)

	// 4. Cache optimization.
	recs = append(recs, r.cacheOptimizeRecs(stats)...)

	// 5. Model downgrade.
	recs = append(recs, r.modelDowngradeRecs(stats)...)

	// Sort by impact descending, then confidence descending.
	sort.Slice(recs, func(i, j int) bool {
		if recs[i].Impact != recs[j].Impact {
			return recs[i].Impact > recs[j].Impact
		}
		return recs[i].Confidence > recs[j].Confidence
	})

	return recs
}

// buildStats aggregates samples into per-provider, per-task statistics.
func (r *Recommender) buildStats(samples []CostSample) map[string]*providerStats {
	stats := make(map[string]*providerStats)

	for _, s := range samples {
		ps, ok := stats[s.Provider]
		if !ok {
			ps = &providerStats{
				Provider:  s.Provider,
				TaskTypes: make(map[string]taskStats),
			}
			stats[s.Provider] = ps
		}
		ps.Samples++
		ps.TotalCost += s.CostUSD

		if s.TaskType != "" {
			ts := ps.TaskTypes[s.TaskType]
			ts.Samples++
			ts.TotalCost += s.CostUSD
			ps.TaskTypes[s.TaskType] = ts
		}
	}

	// Compute means and stddevs.
	for _, ps := range stats {
		if ps.Samples > 0 {
			ps.MeanCost = ps.TotalCost / float64(ps.Samples)
		}
		for tt, ts := range ps.TaskTypes {
			if ts.Samples > 0 {
				ts.MeanCost = ts.TotalCost / float64(ts.Samples)
			}
			ps.TaskTypes[tt] = ts
		}
	}

	// Compute stddev per provider from raw samples.
	providerSamples := make(map[string][]float64)
	for _, s := range samples {
		providerSamples[s.Provider] = append(providerSamples[s.Provider], s.CostUSD)
	}
	for prov, costs := range providerSamples {
		if ps, ok := stats[prov]; ok {
			ps.StddevCost = stddev(costs, ps.MeanCost)
		}
	}

	return stats
}

// providerSwitchRecs recommends switching task types to cheaper providers.
func (r *Recommender) providerSwitchRecs(stats map[string]*providerStats) []Recommendation {
	// Gather all task types seen across providers.
	taskProviders := make(map[string][]struct {
		provider string
		mean     float64
		samples  int
	})

	for _, ps := range stats {
		for tt, ts := range ps.TaskTypes {
			if ts.Samples >= r.config.MinSamplesPerProvider {
				taskProviders[tt] = append(taskProviders[tt], struct {
					provider string
					mean     float64
					samples  int
				}{ps.Provider, ts.MeanCost, ts.Samples})
			}
		}
	}

	var recs []Recommendation
	for taskType, providers := range taskProviders {
		if len(providers) < 2 {
			continue
		}

		// Sort by mean cost ascending.
		sort.Slice(providers, func(i, j int) bool {
			return providers[i].mean < providers[j].mean
		})

		cheapest := providers[0]
		for _, p := range providers[1:] {
			if p.mean <= cheapest.mean {
				continue
			}
			savingsFrac := (p.mean - cheapest.mean) / p.mean
			if savingsFrac < 0.05 {
				continue // ignore trivial savings
			}

			// Confidence based on sample counts.
			minSamples := min(p.samples, cheapest.samples)
			confidence := math.Min(float64(minSamples)/20.0, 1.0)

			recs = append(recs, Recommendation{
				Type: RecommendProviderSwitch,
				Description: fmt.Sprintf(
					"Switch task type %q from %s to %s for ~%.0f%% savings (avg $%.4f vs $%.4f per call)",
					taskType, p.provider, cheapest.provider,
					savingsFrac*100, cheapest.mean, p.mean,
				),
				Impact:     savingsFrac,
				Confidence: confidence,
				Config: map[string]any{
					"task_type":     taskType,
					"from_provider": p.provider,
					"to_provider":   cheapest.provider,
					"savings_pct":   math.Round(savingsFrac*1000) / 10,
				},
			})
		}
	}

	return recs
}

// budgetPacingRecs suggests concurrency adjustments to stay within budget.
func (r *Recommender) budgetPacingRecs(samples []CostSample) []Recommendation {
	if r.config.BudgetRemaining <= 0 || r.config.Concurrency <= 0 {
		return nil
	}

	forecast := r.predictor.Forecast(r.config.BudgetRemaining)
	if forecast.BurnRatePerHour <= 0 {
		return nil
	}

	desiredHours := r.config.BudgetHours
	if desiredHours <= 0 {
		desiredHours = 8
	}

	hoursRemaining := r.config.BudgetRemaining / forecast.BurnRatePerHour
	if hoursRemaining >= desiredHours {
		return nil // on track
	}

	// How much do we need to reduce burn rate?
	targetBurnRate := r.config.BudgetRemaining / desiredHours
	reductionFrac := 1.0 - (targetBurnRate / forecast.BurnRatePerHour)

	// Suggest reducing concurrency proportionally.
	newConcurrency := max(int(math.Ceil(float64(r.config.Concurrency)*(1.0-reductionFrac))), 1)
	if newConcurrency >= r.config.Concurrency {
		return nil // no change needed
	}

	confidence := math.Min(float64(len(samples))/50.0, 0.9)

	return []Recommendation{{
		Type: RecommendBudgetPacing,
		Description: fmt.Sprintf(
			"Reduce concurrency from %d to %d to stay within budget (%.0fh remaining at current rate, target %.0fh)",
			r.config.Concurrency, newConcurrency, hoursRemaining, desiredHours,
		),
		Impact:     reductionFrac,
		Confidence: confidence,
		Config: map[string]any{
			"from_concurrency":  r.config.Concurrency,
			"to_concurrency":    newConcurrency,
			"current_burn_rate": forecast.BurnRatePerHour,
			"target_burn_rate":  targetBurnRate,
			"hours_remaining":   math.Round(hoursRemaining*10) / 10,
		},
	}}
}

// anomalyResponseRecs flags providers with cost anomalies.
func (r *Recommender) anomalyResponseRecs(stats map[string]*providerStats) []Recommendation {
	anomalies := r.predictor.DetectAnomalies()
	if len(anomalies) == 0 {
		return nil
	}

	// Count anomalies per provider (from recent samples).
	samples := r.predictor.Samples()
	providerAnomalyCount := make(map[string]int)
	anomalyTimestamps := make(map[string]bool)
	for _, a := range anomalies {
		anomalyTimestamps[a.Timestamp.String()] = true
	}
	for _, s := range samples {
		if anomalyTimestamps[s.Timestamp.String()] {
			providerAnomalyCount[s.Provider]++
		}
	}

	var recs []Recommendation
	for provider, count := range providerAnomalyCount {
		ps := stats[provider]
		if ps == nil {
			continue
		}

		anomalyRate := float64(count) / float64(ps.Samples)
		if anomalyRate < 0.05 {
			continue // less than 5% anomalous, not worth flagging
		}

		confidence := math.Min(anomalyRate*2, 0.95)

		recs = append(recs, Recommendation{
			Type: RecommendAnomalyResponse,
			Description: fmt.Sprintf(
				"Provider %s showing cost anomalies (%d of %d samples, %.0f%% anomaly rate), consider routing away",
				provider, count, ps.Samples, anomalyRate*100,
			),
			Impact:     anomalyRate,
			Confidence: confidence,
			Config: map[string]any{
				"provider":      provider,
				"anomaly_count": count,
				"anomaly_rate":  math.Round(anomalyRate*1000) / 10,
				"action":        "route_away",
			},
		})
	}

	return recs
}

// cacheOptimizeRecs recommends enabling prompt caching for high-cost providers.
func (r *Recommender) cacheOptimizeRecs(stats map[string]*providerStats) []Recommendation {
	// Known providers that support prompt caching.
	cacheable := map[string]bool{
		"claude": true,
		"gemini": true,
		"openai": true,
	}

	var recs []Recommendation
	for _, ps := range stats {
		if !cacheable[strings.ToLower(ps.Provider)] {
			continue
		}
		if ps.Samples < r.config.MinSamplesPerProvider {
			continue
		}

		// Estimate savings: caching typically saves ~80% of input costs,
		// and input costs are ~60-70% of total for prompt-heavy workloads.
		inputCostFrac := 0.65 // conservative estimate
		savingsFrac := r.config.CacheSavingsEstimate * inputCostFrac
		estimatedSavings := ps.TotalCost * savingsFrac

		if estimatedSavings < 0.01 {
			continue // not worth it
		}

		confidence := math.Min(float64(ps.Samples)/30.0, 0.85)

		recs = append(recs, Recommendation{
			Type: RecommendCacheOptimize,
			Description: fmt.Sprintf(
				"Enable prompt caching for %s (estimated %.0f%% input cost reduction, ~$%.2f savings from $%.2f total)",
				ps.Provider, r.config.CacheSavingsEstimate*100, estimatedSavings, ps.TotalCost,
			),
			Impact:     savingsFrac,
			Confidence: confidence,
			Config: map[string]any{
				"provider":          ps.Provider,
				"enable_caching":    true,
				"estimated_savings": math.Round(estimatedSavings*100) / 100,
				"savings_pct":       math.Round(savingsFrac*1000) / 10,
			},
		})
	}

	return recs
}

// modelDowngradeRecs suggests using cheaper models for suitable task types.
func (r *Recommender) modelDowngradeRecs(stats map[string]*providerStats) []Recommendation {
	// Model cost tiers (approximate relative cost multipliers).
	// Higher-tier models cost more but may offer better quality.
	modelTiers := map[string]struct {
		cheaper string
		ratio   float64 // cheaper/expensive cost ratio
	}{
		"claude": {cheaper: "claude-haiku", ratio: 0.15},
		"openai": {cheaper: "gpt-4o-mini", ratio: 0.10},
		"gemini": {cheaper: "gemini-flash", ratio: 0.12},
	}

	// Task types generally safe for cheaper models.
	cheapTaskTypes := map[string]bool{
		"lint":        true,
		"format":      true,
		"test":        true,
		"docs":        true,
		"refactor":    true,
		"simple":      true,
		"boilerplate": true,
	}

	var recs []Recommendation
	for _, ps := range stats {
		tier, ok := modelTiers[strings.ToLower(ps.Provider)]
		if !ok {
			continue
		}

		for taskType, ts := range ps.TaskTypes {
			if !cheapTaskTypes[strings.ToLower(taskType)] {
				continue
			}
			if ts.Samples < r.config.MinSamplesPerProvider {
				continue
			}

			savingsFrac := 1.0 - tier.ratio
			confidence := math.Min(float64(ts.Samples)/25.0, 0.80)

			recs = append(recs, Recommendation{
				Type: RecommendModelDowngrade,
				Description: fmt.Sprintf(
					"Use %s instead of %s for %q tasks (~%.0f%% savings, quality impact < %.0f%%)",
					tier.cheaper, ps.Provider, taskType,
					savingsFrac*100, r.config.ModelDowngradeQualityThreshold*100,
				),
				Impact:     savingsFrac,
				Confidence: confidence,
				Config: map[string]any{
					"task_type":     taskType,
					"from_provider": ps.Provider,
					"to_model":      tier.cheaper,
					"savings_pct":   math.Round(savingsFrac*1000) / 10,
					"quality_risk":  "low",
				},
			})
		}
	}

	return recs
}

// stddev computes the standard deviation of values given the mean.
func stddev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= float64(len(values))
	return math.Sqrt(variance)
}
