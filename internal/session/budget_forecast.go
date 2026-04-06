package session

import (
	"math"
	"time"
)

// SpendSample records a timestamped cost observation for velocity tracking.
type SpendSample struct {
	Timestamp time.Time `json:"timestamp"`
	TotalUSD  float64   `json:"total_usd"`
}

// BudgetForecast projects remaining budget based on spend velocity.
// It follows 12-Factor Agent principle #3 (own your context) by providing
// deterministic budget awareness to the LLM's context window.
type BudgetForecast struct {
	BudgetUSD     float64   `json:"budget_usd"`
	SpentUSD      float64   `json:"spent_usd"`
	RemainingUSD  float64   `json:"remaining_usd"`
	VelocityPerHr float64   `json:"velocity_per_hr"` // $/hour spend rate
	EstExhaustAt  time.Time `json:"est_exhaust_at,omitempty"`
	EstHoursLeft  float64   `json:"est_hours_left"`
	ConfidencePct float64   `json:"confidence_pct"` // 0-100 based on sample count
	Warnings      []string  `json:"warnings,omitempty"`
}

// BudgetForecaster computes budget projections from spend samples.
type BudgetForecaster struct {
	samples  []SpendSample
	maxAge   time.Duration // only consider samples within this window
}

// NewBudgetForecaster creates a forecaster that uses the given time window.
func NewBudgetForecaster(windowDuration time.Duration) *BudgetForecaster {
	if windowDuration <= 0 {
		windowDuration = time.Hour
	}
	return &BudgetForecaster{
		maxAge: windowDuration,
	}
}

// AddSample records a spend data point.
func (f *BudgetForecaster) AddSample(s SpendSample) {
	f.samples = append(f.samples, s)
}

// Forecast computes the budget projection.
func (f *BudgetForecaster) Forecast(budgetUSD, spentUSD float64) BudgetForecast {
	remaining := budgetUSD - spentUSD
	if remaining < 0 {
		remaining = 0
	}

	fc := BudgetForecast{
		BudgetUSD:    budgetUSD,
		SpentUSD:     spentUSD,
		RemainingUSD: remaining,
	}

	if remaining <= 0 {
		fc.Warnings = append(fc.Warnings, "budget exhausted")
		return fc
	}

	// Filter to recent samples within the window.
	cutoff := time.Now().Add(-f.maxAge)
	var recent []SpendSample
	for _, s := range f.samples {
		if s.Timestamp.After(cutoff) {
			recent = append(recent, s)
		}
	}

	if len(recent) < 2 {
		fc.ConfidencePct = 10
		fc.Warnings = append(fc.Warnings, "insufficient data for velocity estimate")
		return fc
	}

	// Compute velocity using linear regression on recent samples.
	velocity := forecastVelocity(recent)
	fc.VelocityPerHr = velocity

	if velocity <= 0 {
		fc.ConfidencePct = 50
		fc.EstHoursLeft = math.Inf(1)
		return fc
	}

	hoursLeft := remaining / velocity
	fc.EstHoursLeft = hoursLeft
	fc.EstExhaustAt = time.Now().Add(time.Duration(hoursLeft * float64(time.Hour)))

	// Confidence based on sample count and consistency.
	fc.ConfidencePct = forecastConfidence(recent, velocity)

	// Add warnings.
	if hoursLeft < 1 {
		fc.Warnings = append(fc.Warnings, "budget exhaustion estimated within 1 hour")
	} else if hoursLeft < 4 {
		fc.Warnings = append(fc.Warnings, "budget exhaustion estimated within 4 hours")
	}

	if budgetUSD > 0 && spentUSD/budgetUSD > 0.9 {
		fc.Warnings = append(fc.Warnings, "over 90% of budget consumed")
	}

	return fc
}

// forecastVelocity estimates $/hour from samples using simple linear slope.
func forecastVelocity(samples []SpendSample) float64 {
	if len(samples) < 2 {
		return 0
	}
	first := samples[0]
	last := samples[len(samples)-1]

	elapsed := last.Timestamp.Sub(first.Timestamp).Hours()
	if elapsed <= 0 {
		return 0
	}

	delta := last.TotalUSD - first.TotalUSD
	if delta < 0 {
		return 0 // spend should be monotonically increasing
	}

	return delta / elapsed
}

// forecastConfidence returns a 0-100 confidence score based on sample count
// and velocity consistency (coefficient of variation).
func forecastConfidence(samples []SpendSample, avgVelocity float64) float64 {
	n := float64(len(samples))

	// Base confidence from sample count (logistic growth).
	countConf := 100 * (1 - math.Exp(-n/5))

	// Adjust for velocity consistency.
	if len(samples) < 3 || avgVelocity <= 0 {
		return math.Min(countConf, 50)
	}

	// Compute per-interval velocities.
	var velocities []float64
	for i := 1; i < len(samples); i++ {
		dt := samples[i].Timestamp.Sub(samples[i-1].Timestamp).Hours()
		if dt > 0 {
			v := (samples[i].TotalUSD - samples[i-1].TotalUSD) / dt
			velocities = append(velocities, v)
		}
	}

	if len(velocities) == 0 {
		return math.Min(countConf, 40)
	}

	// Coefficient of variation (lower = more consistent).
	var sum, sumSq float64
	for _, v := range velocities {
		sum += v
		sumSq += v * v
	}
	mean := sum / float64(len(velocities))
	if mean <= 0 {
		return math.Min(countConf, 40)
	}
	variance := sumSq/float64(len(velocities)) - mean*mean
	if variance < 0 {
		variance = 0
	}
	cv := math.Sqrt(variance) / mean

	// High CV = inconsistent = lower confidence.
	consistencyPenalty := math.Min(cv*30, 40) // max 40% penalty
	return math.Max(10, math.Min(100, countConf-consistencyPenalty))
}
