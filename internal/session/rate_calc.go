package session

import (
	"time"
)

// RateCalculator computes cost rates ($/hr, $/min) from ledger data using a
// sliding time window. It is safe for concurrent use because the underlying
// CostLedger serialises access.
type RateCalculator struct {
	ledger *CostLedger
	window time.Duration
}

// NewRateCalculator creates a RateCalculator that examines costs within the
// given sliding window duration. A typical window is 1 hour.
func NewRateCalculator(ledger *CostLedger, window time.Duration) *RateCalculator {
	return &RateCalculator{
		ledger: ledger,
		window: window,
	}
}

// CurrentRate returns the cost rate in USD per hour across all sessions,
// computed from entries within the sliding window. Returns 0 if there are
// fewer than two entries or no time has elapsed in the window.
func (rc *RateCalculator) CurrentRate() float64 {
	return rc.rateFor("", time.Now())
}

// SessionRate returns the cost rate in USD per hour for a single session,
// computed from entries within the sliding window. Returns 0 if there are
// fewer than two entries or no time has elapsed.
func (rc *RateCalculator) SessionRate(sessionID string) float64 {
	return rc.rateFor(sessionID, time.Now())
}

// ProjectedCost estimates total cost over the given future duration based on
// the current aggregate rate.
func (rc *RateCalculator) ProjectedCost(duration time.Duration) float64 {
	rate := rc.CurrentRate() // $/hr
	hours := duration.Hours()
	return rate * hours
}

// rateFor computes $/hr for entries matching sessionID (or all if empty)
// within the window ending at now.
func (rc *RateCalculator) rateFor(sessionID string, now time.Time) float64 {
	since := now.Add(-rc.window)
	entries := rc.ledger.EntriesSince(since)

	var sum float64
	var earliest, latest time.Time
	count := 0

	for _, e := range entries {
		if sessionID != "" && e.SessionID != sessionID {
			continue
		}
		if e.Timestamp.After(now) {
			continue
		}
		sum += e.Amount
		if count == 0 || e.Timestamp.Before(earliest) {
			earliest = e.Timestamp
		}
		if count == 0 || e.Timestamp.After(latest) {
			latest = e.Timestamp
		}
		count++
	}

	if count < 2 {
		return 0
	}

	elapsed := latest.Sub(earliest)
	if elapsed <= 0 {
		return 0
	}

	return sum / elapsed.Hours()
}
