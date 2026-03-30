package fleet

import (
	"math"
	"sync"
	"time"
)

const (
	defaultMaxDataPoints = 10000
	minDataPointsForCI   = 3
)

// SpendPoint is a timestamped spend observation.
type SpendPoint struct {
	Timestamp time.Time
	AmountUSD float64
}

// ForecastReport is the output of a budget forecast.
type ForecastReport struct {
	BurnRatePerHour    float64    `json:"burn_rate_per_hour"`
	ExhaustionTime     *time.Time `json:"exhaustion_time,omitempty"`
	ConfidenceInterval [2]float64 `json:"confidence_interval"` // [low, high] burn rate
	Trend              string     `json:"trend"`               // "accelerating", "stable", "decelerating"
	ProjectedSpend     float64    `json:"projected_spend"`     // total USD by end of horizon
}

// Forecaster tracks spend velocity over time windows and produces budget
// exhaustion forecasts with confidence intervals and trend detection.
// All methods are safe for concurrent use.
type Forecaster struct {
	mu            sync.RWMutex
	points        []SpendPoint
	maxDataPoints int
}

// NewForecaster creates a Forecaster with default capacity.
func NewForecaster() *Forecaster {
	return &Forecaster{
		points:        make([]SpendPoint, 0, 256),
		maxDataPoints: defaultMaxDataPoints,
	}
}

// RecordSpend adds a spend data point. Points should arrive roughly in
// chronological order; out-of-order points are accepted but may reduce
// accuracy of windowed calculations.
func (f *Forecaster) RecordSpend(timestamp time.Time, amountUSD float64) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.points = append(f.points, SpendPoint{
		Timestamp: timestamp,
		AmountUSD: amountUSD,
	})
	if len(f.points) > f.maxDataPoints {
		f.points = f.points[1:]
	}
}

// BurnRate returns the spend rate in USD/hour over the given window,
// measured backward from the most recent data point. If window is zero
// or negative, all data points are used.
func (f *Forecaster) BurnRate(window time.Duration) float64 {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.burnRateLocked(window)
}

func (f *Forecaster) burnRateLocked(window time.Duration) float64 {
	pts := f.windowedPointsLocked(window)
	if len(pts) < 2 {
		return 0
	}

	first := pts[0].Timestamp
	last := pts[len(pts)-1].Timestamp
	span := last.Sub(first).Hours()
	if span <= 0 {
		return 0
	}

	var total float64
	for _, p := range pts {
		total += p.AmountUSD
	}
	return total / span
}

// TimeToExhaustion returns the predicted duration until the remaining budget
// runs out, based on current burn rate across all data. Returns 0 if the burn
// rate is zero or there is insufficient data.
func (f *Forecaster) TimeToExhaustion(remainingUSD float64) time.Duration {
	f.mu.RLock()
	defer f.mu.RUnlock()

	rate := f.burnRateLocked(0) // all data
	if rate <= 0 || remainingUSD <= 0 {
		return 0
	}
	hours := remainingUSD / rate
	return time.Duration(hours * float64(time.Hour))
}

// Forecast produces a full ForecastReport for the given remaining budget
// and prediction horizon.
func (f *Forecaster) Forecast(remainingUSD float64, horizon time.Duration) ForecastReport {
	f.mu.RLock()
	defer f.mu.RUnlock()

	report := ForecastReport{
		Trend: "stable",
	}

	if len(f.points) < 2 {
		return report
	}

	// Overall burn rate (all data).
	report.BurnRatePerHour = f.burnRateLocked(0)

	// Exhaustion time.
	if report.BurnRatePerHour > 0 && remainingUSD > 0 {
		hours := remainingUSD / report.BurnRatePerHour
		if horizon <= 0 || time.Duration(hours*float64(time.Hour)) <= horizon {
			t := time.Now().Add(time.Duration(hours * float64(time.Hour)))
			report.ExhaustionTime = &t
		}
	}

	// Projected spend over horizon.
	if horizon > 0 {
		report.ProjectedSpend = report.BurnRatePerHour * horizon.Hours()
	}

	// Trend detection: compare recent 15m burn rate vs 1h average.
	report.Trend = f.trendLocked()

	// Confidence interval from per-minute spend standard deviation.
	report.ConfidenceInterval = f.confidenceIntervalLocked()

	return report
}

// trendLocked compares the 15-minute burn rate to the 1-hour burn rate.
// Must be called with at least f.mu.RLock held.
func (f *Forecaster) trendLocked() string {
	rate1h := f.burnRateLocked(time.Hour)
	rate15m := f.burnRateLocked(15 * time.Minute)

	// Need both windows to have meaningful data.
	if rate1h <= 0 {
		return "stable"
	}

	ratio := rate15m / rate1h
	switch {
	case ratio > 1.2:
		return "accelerating"
	case ratio < 0.8:
		return "decelerating"
	default:
		return "stable"
	}
}

// confidenceIntervalLocked computes a 95% confidence interval for the burn
// rate using the standard deviation of per-minute spend buckets.
// Must be called with at least f.mu.RLock held.
func (f *Forecaster) confidenceIntervalLocked() [2]float64 {
	buckets := f.minuteBucketsLocked()
	if len(buckets) < minDataPointsForCI {
		rate := f.burnRateLocked(0)
		return [2]float64{rate, rate}
	}

	mean, stddev := bucketMeanStddev(buckets)

	// Convert per-minute rate to per-hour and apply 1.96 * stddev (95% CI).
	meanPerHour := mean * 60
	stddevPerHour := stddev * 60

	low := meanPerHour - 1.96*stddevPerHour
	if low < 0 {
		low = 0
	}
	high := meanPerHour + 1.96*stddevPerHour

	return [2]float64{low, high}
}

// minuteBucketsLocked aggregates spend into per-minute buckets.
// Must be called with at least f.mu.RLock held.
func (f *Forecaster) minuteBucketsLocked() []float64 {
	if len(f.points) == 0 {
		return nil
	}

	// Truncate each timestamp to minute, sum spend per minute.
	bucketMap := make(map[int64]float64)
	for _, p := range f.points {
		key := p.Timestamp.Truncate(time.Minute).Unix()
		bucketMap[key] += p.AmountUSD
	}

	buckets := make([]float64, 0, len(bucketMap))
	for _, v := range bucketMap {
		buckets = append(buckets, v)
	}
	return buckets
}

// windowedPointsLocked returns points within the given window measured backward
// from the most recent point. If window <= 0, all points are returned.
// Must be called with at least f.mu.RLock held.
func (f *Forecaster) windowedPointsLocked(window time.Duration) []SpendPoint {
	if len(f.points) == 0 {
		return nil
	}
	if window <= 0 {
		return f.points
	}

	cutoff := f.points[len(f.points)-1].Timestamp.Add(-window)
	for i, p := range f.points {
		if !p.Timestamp.Before(cutoff) {
			return f.points[i:]
		}
	}
	return nil
}

// Len returns the number of recorded data points.
func (f *Forecaster) Len() int {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return len(f.points)
}

// ---------- helpers ----------

func bucketMeanStddev(buckets []float64) (mean, stddev float64) {
	n := float64(len(buckets))
	if n == 0 {
		return 0, 0
	}
	for _, v := range buckets {
		mean += v
	}
	mean /= n

	var variance float64
	for _, v := range buckets {
		d := v - mean
		variance += d * d
	}
	variance /= n
	return mean, math.Sqrt(variance)
}
