package fleet

import (
	"testing"
	"time"
)

func TestNewRecommender(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)
	if r == nil {
		t.Fatal("expected non-nil recommender")
	}
	if r.predictor != p {
		t.Error("predictor not set")
	}
}

func TestAnalyze_InsufficientData(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	// No samples at all.
	recs := r.Analyze()
	if recs != nil {
		t.Errorf("expected nil with no samples, got %d recommendations", len(recs))
	}

	// One sample only.
	p.Record(CostSample{
		Timestamp: time.Now(),
		CostUSD:   1.0,
		Provider:  "claude",
		TaskType:  "code",
	})
	recs = r.Analyze()
	if recs != nil {
		t.Errorf("expected nil with 1 sample, got %d recommendations", len(recs))
	}
}

func TestAnalyze_ProviderSwitch(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// Claude is expensive for "lint" tasks.
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   0.50,
			Provider:  "claude",
			TaskType:  "lint",
		})
	}

	// Gemini is cheap for "lint" tasks.
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(10+i) * time.Minute),
			CostUSD:   0.05,
			Provider:  "gemini",
			TaskType:  "lint",
		})
	}

	recs := r.Analyze()
	if len(recs) == 0 {
		t.Fatal("expected at least one recommendation")
	}

	// Find the provider switch recommendation.
	var found bool
	for _, rec := range recs {
		if rec.Type == RecommendProviderSwitch {
			found = true
			if rec.Impact < 0.5 {
				t.Errorf("expected large savings fraction, got %.2f", rec.Impact)
			}
			if rec.Confidence <= 0 {
				t.Error("expected positive confidence")
			}
			cfg := rec.Config
			if cfg["from_provider"] != "claude" {
				t.Errorf("expected from_provider=claude, got %v", cfg["from_provider"])
			}
			if cfg["to_provider"] != "gemini" {
				t.Errorf("expected to_provider=gemini, got %v", cfg["to_provider"])
			}
			break
		}
	}
	if !found {
		t.Error("expected provider_switch recommendation")
	}
}

func TestAnalyze_ProviderSwitch_MinSamples(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// Only 2 samples per provider (below default threshold of 5).
	for i := range 2 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.0,
			Provider:  "claude",
			TaskType:  "code",
		})
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(2+i) * time.Minute),
			CostUSD:   0.1,
			Provider:  "gemini",
			TaskType:  "code",
		})
	}

	recs := r.Analyze()
	for _, rec := range recs {
		if rec.Type == RecommendProviderSwitch {
			t.Error("should not recommend provider switch with insufficient samples")
		}
	}
}

func TestAnalyze_BudgetPacing(t *testing.T) {
	p := NewCostPredictor(2.0)

	now := time.Now()
	// High burn rate: $10/hour.
	for i := range 60 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   10.0 / 60.0, // ~$0.167/min
			Provider:  "claude",
			TaskType:  "code",
		})
	}

	r := NewRecommender(p).WithConfig(RecommenderConfig{
		MinSamplesPerProvider: 5,
		AnomalyZThreshold:     2.0,
		CacheSavingsEstimate:  0.80,
		BudgetRemaining:       20.0, // only $20 left
		Concurrency:           4,
		BudgetHours:           8, // want 8 hours runway
	})

	recs := r.Analyze()

	var found bool
	for _, rec := range recs {
		if rec.Type == RecommendBudgetPacing {
			found = true
			if rec.Config["to_concurrency"] == nil {
				t.Error("expected to_concurrency in config")
			}
			toConcurrency, ok := rec.Config["to_concurrency"].(int)
			if !ok {
				t.Fatalf("to_concurrency wrong type: %T", rec.Config["to_concurrency"])
			}
			if toConcurrency >= 4 {
				t.Errorf("expected reduced concurrency, got %d", toConcurrency)
			}
			if toConcurrency < 1 {
				t.Errorf("concurrency should be at least 1, got %d", toConcurrency)
			}
			break
		}
	}
	if !found {
		t.Error("expected budget_pacing recommendation")
	}
}

func TestAnalyze_BudgetPacing_OnTrack(t *testing.T) {
	p := NewCostPredictor(2.0)

	now := time.Now()
	// Low burn rate: $1/hour.
	for i := range 60 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.0 / 60.0,
			Provider:  "claude",
			TaskType:  "code",
		})
	}

	r := NewRecommender(p).WithConfig(RecommenderConfig{
		MinSamplesPerProvider: 5,
		AnomalyZThreshold:     2.0,
		CacheSavingsEstimate:  0.80,
		BudgetRemaining:       100.0, // plenty
		Concurrency:           4,
		BudgetHours:           8,
	})

	recs := r.Analyze()
	for _, rec := range recs {
		if rec.Type == RecommendBudgetPacing {
			t.Error("should not recommend pacing changes when budget is on track")
		}
	}
}

func TestAnalyze_AnomalyResponse(t *testing.T) {
	p := NewCostPredictor(2.0)

	now := time.Now()

	// 25 normal samples around $0.10 each for claude.
	for i := range 25 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   0.10,
			Provider:  "claude",
			TaskType:  "code",
		})
	}

	// Then spike: 5 samples at $5.00 each (50x normal).
	for i := range 5 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(25+i) * time.Minute),
			CostUSD:   5.00,
			Provider:  "claude",
			TaskType:  "code",
		})
	}

	r := NewRecommender(p)
	recs := r.Analyze()

	var found bool
	for _, rec := range recs {
		if rec.Type == RecommendAnomalyResponse {
			found = true
			if rec.Config["provider"] != "claude" {
				t.Errorf("expected provider=claude, got %v", rec.Config["provider"])
			}
			if rec.Confidence <= 0 {
				t.Error("expected positive confidence")
			}
			break
		}
	}
	if !found {
		t.Error("expected anomaly_response recommendation")
	}
}

func TestAnalyze_CacheOptimize(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// Enough samples for claude to trigger cache recommendation.
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.00,
			Provider:  "claude",
			TaskType:  "code",
		})
	}

	recs := r.Analyze()

	var found bool
	for _, rec := range recs {
		if rec.Type == RecommendCacheOptimize {
			found = true
			if rec.Config["provider"] != "claude" {
				t.Errorf("expected provider=claude, got %v", rec.Config["provider"])
			}
			if rec.Config["enable_caching"] != true {
				t.Error("expected enable_caching=true")
			}
			if rec.Impact <= 0 {
				t.Error("expected positive impact")
			}
			break
		}
	}
	if !found {
		t.Error("expected cache_optimize recommendation")
	}
}

func TestAnalyze_CacheOptimize_UnknownProvider(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.00,
			Provider:  "unknown_provider",
			TaskType:  "code",
		})
	}

	recs := r.Analyze()
	for _, rec := range recs {
		if rec.Type == RecommendCacheOptimize && rec.Config["provider"] == "unknown_provider" {
			t.Error("should not recommend caching for unknown provider")
		}
	}
}

func TestAnalyze_ModelDowngrade(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// Claude doing "lint" tasks (eligible for downgrade).
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   0.50,
			Provider:  "claude",
			TaskType:  "lint",
		})
	}

	recs := r.Analyze()

	var found bool
	for _, rec := range recs {
		if rec.Type == RecommendModelDowngrade {
			found = true
			if rec.Config["task_type"] != "lint" {
				t.Errorf("expected task_type=lint, got %v", rec.Config["task_type"])
			}
			if rec.Config["to_model"] != "claude-haiku" {
				t.Errorf("expected to_model=claude-haiku, got %v", rec.Config["to_model"])
			}
			if rec.Impact < 0.5 {
				t.Errorf("expected significant savings, got %.2f", rec.Impact)
			}
			break
		}
	}
	if !found {
		t.Error("expected model_downgrade recommendation")
	}
}

func TestAnalyze_ModelDowngrade_ComplexTask(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// "architecture" tasks should not be downgraded.
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   2.00,
			Provider:  "claude",
			TaskType:  "architecture",
		})
	}

	recs := r.Analyze()
	for _, rec := range recs {
		if rec.Type == RecommendModelDowngrade && rec.Config["task_type"] == "architecture" {
			t.Error("should not recommend model downgrade for complex tasks")
		}
	}
}

func TestAnalyze_SortedByImpact(t *testing.T) {
	p := NewCostPredictor(2.0)

	now := time.Now()

	// Create data that triggers multiple recommendation types.
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.00,
			Provider:  "claude",
			TaskType:  "lint",
		})
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(10+i) * time.Minute),
			CostUSD:   0.05,
			Provider:  "gemini",
			TaskType:  "lint",
		})
	}

	r := NewRecommender(p)
	recs := r.Analyze()

	if len(recs) < 2 {
		t.Fatalf("expected multiple recommendations, got %d", len(recs))
	}

	// Verify sorted by impact descending.
	for i := 1; i < len(recs); i++ {
		if recs[i].Impact > recs[i-1].Impact {
			t.Errorf("recommendations not sorted: rec[%d].Impact=%.2f > rec[%d].Impact=%.2f",
				i, recs[i].Impact, i-1, recs[i-1].Impact)
		}
	}
}

func TestWithConfig(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	cfg := RecommenderConfig{
		MinSamplesPerProvider: 10,
		BudgetRemaining:       50.0,
		Concurrency:           8,
		BudgetHours:           24,
	}
	r2 := r.WithConfig(cfg)

	if r2 != r {
		t.Error("WithConfig should return same receiver")
	}
	if r.config.MinSamplesPerProvider != 10 {
		t.Errorf("expected MinSamplesPerProvider=10, got %d", r.config.MinSamplesPerProvider)
	}
}

func TestSamples_Accessor(t *testing.T) {
	p := NewCostPredictor(2.0)

	now := time.Now()
	p.Record(CostSample{Timestamp: now, CostUSD: 1.0, Provider: "claude"})
	p.Record(CostSample{Timestamp: now.Add(time.Minute), CostUSD: 2.0, Provider: "gemini"})

	samples := p.Samples()
	if len(samples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(samples))
	}

	// Verify it's a copy.
	samples[0].CostUSD = 999.0
	original := p.Samples()
	if original[0].CostUSD == 999.0 {
		t.Error("Samples() should return a copy, not a reference")
	}
}

func TestAnalyze_ProviderSwitch_TrivialSavings(t *testing.T) {
	p := NewCostPredictor(2.0)
	r := NewRecommender(p)

	now := time.Now()

	// Providers with nearly identical costs (< 5% difference).
	for i := range 10 {
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(i) * time.Minute),
			CostUSD:   1.00,
			Provider:  "claude",
			TaskType:  "code",
		})
		p.Record(CostSample{
			Timestamp: now.Add(time.Duration(10+i) * time.Minute),
			CostUSD:   0.98, // only 2% cheaper
			Provider:  "gemini",
			TaskType:  "code",
		})
	}

	recs := r.Analyze()
	for _, rec := range recs {
		if rec.Type == RecommendProviderSwitch && rec.Config["task_type"] == "code" {
			t.Error("should not recommend switch for trivial savings (< 5%)")
		}
	}
}

func TestDefaultRecommenderConfig(t *testing.T) {
	cfg := DefaultRecommenderConfig()
	if cfg.MinSamplesPerProvider != 5 {
		t.Errorf("expected MinSamplesPerProvider=5, got %d", cfg.MinSamplesPerProvider)
	}
	if cfg.AnomalyZThreshold != 2.0 {
		t.Errorf("expected AnomalyZThreshold=2.0, got %.1f", cfg.AnomalyZThreshold)
	}
	if cfg.CacheSavingsEstimate != 0.80 {
		t.Errorf("expected CacheSavingsEstimate=0.80, got %.2f", cfg.CacheSavingsEstimate)
	}
	if cfg.BudgetHours != 8 {
		t.Errorf("expected BudgetHours=8, got %.0f", cfg.BudgetHours)
	}
}

func TestStddev(t *testing.T) {
	values := []float64{2, 4, 4, 4, 5, 5, 7, 9}
	mean := 5.0
	sd := stddev(values, mean)
	if sd < 1.99 || sd > 2.01 {
		t.Errorf("expected stddev ~2.0, got %.4f", sd)
	}

	// Empty.
	if stddev(nil, 0) != 0 {
		t.Error("expected 0 for empty")
	}
}
