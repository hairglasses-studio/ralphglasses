package fleet

import (
	"math"
	"sync"
	"time"
)

const defaultMaxSamples = 1000

// CostSample records a single cost observation.
type CostSample struct {
	Timestamp time.Time
	CostUSD   float64
	Provider  string
	TaskType  string
}

// CostForecast is the output of a cost prediction.
type CostForecast struct {
	BurnRatePerHour float64        `json:"burn_rate_per_hour"`
	ExhaustionETA   *time.Time     `json:"exhaustion_eta,omitempty"`
	Anomalies       []CostAnomaly  `json:"anomalies,omitempty"`
	TrendDirection  string         `json:"trend_direction"` // "increasing", "decreasing", "stable"
	WindowSize      int            `json:"window_size"`
	SampleCount     int            `json:"sample_count"`
}

// CostAnomaly flags a sample that deviates significantly from recent history.
type CostAnomaly struct {
	Timestamp   time.Time `json:"timestamp"`
	ExpectedUSD float64   `json:"expected_usd"`
	ActualUSD   float64   `json:"actual_usd"`
	ZScore      float64   `json:"z_score"`
}

// CostPredictor maintains a sliding window of cost samples and provides
// burn-rate forecasting, trend detection, and anomaly flagging.
type CostPredictor struct {
	mu               sync.Mutex
	samples          []CostSample
	maxSamples       int
	anomalyThreshold float64
}

// NewCostPredictor creates a predictor. If anomalyThreshold <= 0 it defaults
// to 2.5 standard deviations.
func NewCostPredictor(anomalyThreshold float64) *CostPredictor {
	if anomalyThreshold <= 0 {
		anomalyThreshold = 2.5
	}
	return &CostPredictor{
		samples:          make([]CostSample, 0, 64),
		maxSamples:       defaultMaxSamples,
		anomalyThreshold: anomalyThreshold,
	}
}

// Record adds a cost sample, evicting the oldest if the window is full.
func (p *CostPredictor) Record(s CostSample) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.samples = append(p.samples, s)
	if len(p.samples) > p.maxSamples {
		// Drop the oldest sample.
		p.samples = p.samples[1:]
	}
}

// Forecast computes burn rate, exhaustion ETA, trend direction, and anomalies
// given the remaining budget in USD. Pass <= 0 for budgetRemaining if there is
// no hard limit.
func (p *CostPredictor) Forecast(budgetRemaining float64) CostForecast {
	p.mu.Lock()
	defer p.mu.Unlock()

	n := len(p.samples)
	f := CostForecast{
		WindowSize:     p.maxSamples,
		SampleCount:    n,
		TrendDirection: "stable",
	}

	if n < 2 {
		return f
	}

	f.BurnRatePerHour = p.burnRateLocked()

	// Exhaustion ETA.
	if f.BurnRatePerHour > 0 && budgetRemaining > 0 {
		hoursLeft := budgetRemaining / f.BurnRatePerHour
		eta := time.Now().Add(time.Duration(hoursLeft * float64(time.Hour)))
		f.ExhaustionETA = &eta
	}

	// Trend: compare first-half average cost to second-half average cost.
	mid := n / 2
	firstAvg := avgCost(p.samples[:mid])
	secondAvg := avgCost(p.samples[mid:])
	if secondAvg > firstAvg*1.1 {
		f.TrendDirection = "increasing"
	} else if secondAvg < firstAvg*0.9 {
		f.TrendDirection = "decreasing"
	}

	// Anomalies.
	f.Anomalies = p.detectAnomaliesLocked()

	return f
}

// DetectAnomalies returns cost samples whose z-score (relative to the
// preceding 20 samples) exceeds the configured threshold.
func (p *CostPredictor) DetectAnomalies() []CostAnomaly {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.detectAnomaliesLocked()
}

func (p *CostPredictor) detectAnomaliesLocked() []CostAnomaly {
	const lookback = 20
	var anomalies []CostAnomaly

	for i := lookback; i < len(p.samples); i++ {
		window := p.samples[i-lookback : i]
		mean, stddev := meanStddev(window)
		diff := math.Abs(p.samples[i].CostUSD - mean)
		if stddev == 0 {
			// All preceding samples identical. Any deviation is anomalous.
			if diff == 0 {
				continue
			}
			// Treat as infinite z-score; use a large sentinel.
			anomalies = append(anomalies, CostAnomaly{
				Timestamp:   p.samples[i].Timestamp,
				ExpectedUSD: mean,
				ActualUSD:   p.samples[i].CostUSD,
				ZScore:      math.Inf(1),
			})
			continue
		}
		z := diff / stddev
		if z > p.anomalyThreshold {
			anomalies = append(anomalies, CostAnomaly{
				Timestamp:   p.samples[i].Timestamp,
				ExpectedUSD: mean,
				ActualUSD:   p.samples[i].CostUSD,
				ZScore:      z,
			})
		}
	}
	return anomalies
}

// BurnRate returns the cost per hour across the current sample window.
func (p *CostPredictor) BurnRate() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.burnRateLocked()
}

func (p *CostPredictor) burnRateLocked() float64 {
	n := len(p.samples)
	if n < 2 {
		return 0
	}
	first := p.samples[0].Timestamp
	last := p.samples[n-1].Timestamp
	span := last.Sub(first).Hours()
	if span <= 0 {
		return 0
	}
	var total float64
	for _, s := range p.samples {
		total += s.CostUSD
	}
	return total / span
}

// Len returns the number of recorded samples.
func (p *CostPredictor) Len() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.samples)
}

// Samples returns a copy of all recorded cost samples.
func (p *CostPredictor) Samples() []CostSample {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]CostSample, len(p.samples))
	copy(out, p.samples)
	return out
}

// ---------- helpers ----------

func avgCost(samples []CostSample) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += s.CostUSD
	}
	return sum / float64(len(samples))
}

func meanStddev(samples []CostSample) (mean, stddev float64) {
	n := float64(len(samples))
	if n == 0 {
		return 0, 0
	}
	for _, s := range samples {
		mean += s.CostUSD
	}
	mean /= n
	var variance float64
	for _, s := range samples {
		d := s.CostUSD - mean
		variance += d * d
	}
	variance /= n
	return mean, math.Sqrt(variance)
}
