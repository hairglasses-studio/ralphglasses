package cloud

import (
	"fmt"
	"sync"
	"time"
)

// Provider identifies a cloud/LLM provider for cost tracking.
type Provider string

const (
	ProviderClaude Provider = "claude"
	ProviderGemini Provider = "gemini"
	ProviderOpenAI Provider = "openai"
)

// CostEntry represents a single recorded cost event.
type CostEntry struct {
	Provider     Provider
	InstanceType string  // e.g. "opus-4", "flash-2", "o3-pro"
	Amount       float64 // USD
	Timestamp    time.Time
}

// ProviderBreakdown summarises spend for a single provider.
type ProviderBreakdown struct {
	Provider      Provider
	TotalSpend    float64            // USD
	ByInstance    map[string]float64 // instance-type -> USD
	EntryCount    int
	FirstSeen     time.Time
	LastSeen      time.Time
}

// CostReport is a point-in-time snapshot of aggregated cost data.
type CostReport struct {
	TotalSpend        float64                      // USD across all providers
	ByProvider        map[Provider]ProviderBreakdown
	BurnRatePerHour   float64                      // USD/hour over the observation window
	BudgetLimit       float64                      // configured budget cap (0 = unlimited)
	BudgetRemaining   float64                      // budgetLimit - totalSpend (negative if over)
	ExhaustionTime    time.Time                    // projected time budget hits zero (zero value if unlimited or no burn)
	ObservationWindow time.Duration                // wall-clock span of recorded data
	GeneratedAt       time.Time
}

// CostTrackerOption configures a CostTracker.
type CostTrackerOption func(*CostTracker)

// WithBudget sets the total budget cap in USD.
func WithBudget(usd float64) CostTrackerOption {
	return func(ct *CostTracker) {
		if usd >= 0 {
			ct.budget = usd
		}
	}
}

// withClock injects a fake clock for testing.
func withClock(fn func() time.Time) CostTrackerOption {
	return func(ct *CostTracker) { ct.now = fn }
}

// CostTracker aggregates per-provider spend, computes burn rate, and
// projects budget exhaustion time. It is safe for concurrent use.
type CostTracker struct {
	mu      sync.Mutex
	entries []CostEntry
	budget  float64        // 0 = unlimited
	now     func() time.Time
}

// NewCostTracker creates a CostTracker with the given options.
func NewCostTracker(opts ...CostTrackerOption) *CostTracker {
	ct := &CostTracker{
		now: time.Now,
	}
	for _, o := range opts {
		o(ct)
	}
	return ct
}

// Record adds a cost event. Amount must be non-negative; negative values
// are silently ignored.
func (ct *CostTracker) Record(provider Provider, instanceType string, amount float64) {
	if amount < 0 {
		return
	}
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.entries = append(ct.entries, CostEntry{
		Provider:     provider,
		InstanceType: instanceType,
		Amount:       amount,
		Timestamp:    ct.now(),
	})
}

// TotalSpend returns the sum of all recorded costs.
func (ct *CostTracker) TotalSpend() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	var total float64
	for _, e := range ct.entries {
		total += e.Amount
	}
	return total
}

// BurnRate returns the spend rate in USD per hour, computed over the
// wall-clock span between the first and last recorded entries.
// Returns 0 if fewer than two entries exist or the span is zero.
func (ct *CostTracker) BurnRate() float64 {
	ct.mu.Lock()
	defer ct.mu.Unlock()
	return ct.burnRateLocked()
}

// burnRateLocked computes burn rate. Caller must hold ct.mu.
func (ct *CostTracker) burnRateLocked() float64 {
	if len(ct.entries) < 2 {
		return 0
	}

	first := ct.entries[0].Timestamp
	last := ct.entries[len(ct.entries)-1].Timestamp
	span := last.Sub(first)
	if span <= 0 {
		return 0
	}

	var total float64
	for _, e := range ct.entries {
		total += e.Amount
	}

	hours := span.Hours()
	if hours == 0 {
		return 0
	}
	return total / hours
}

// ExhaustionTime projects when the budget will be exhausted based on the
// current burn rate. Returns the zero time if there is no budget, no data,
// or the burn rate is zero.
func (ct *CostTracker) ExhaustionTime() time.Time {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	if ct.budget <= 0 {
		return time.Time{}
	}

	var total float64
	for _, e := range ct.entries {
		total += e.Amount
	}

	remaining := ct.budget - total
	if remaining <= 0 {
		// Already exhausted.
		return ct.now()
	}

	rate := ct.burnRateLocked() // USD/hour
	if rate <= 0 {
		return time.Time{}
	}

	hoursLeft := remaining / rate
	return ct.now().Add(time.Duration(hoursLeft * float64(time.Hour)))
}

// Report generates a full CostReport snapshot.
func (ct *CostTracker) Report() CostReport {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	now := ct.now()
	report := CostReport{
		ByProvider:  make(map[Provider]ProviderBreakdown),
		BudgetLimit: ct.budget,
		GeneratedAt: now,
	}

	if len(ct.entries) == 0 {
		return report
	}

	// Single pass to build provider breakdowns and totals.
	var globalFirst, globalLast time.Time
	for i, e := range ct.entries {
		report.TotalSpend += e.Amount

		pb, ok := report.ByProvider[e.Provider]
		if !ok {
			pb = ProviderBreakdown{
				Provider:   e.Provider,
				ByInstance: make(map[string]float64),
				FirstSeen:  e.Timestamp,
				LastSeen:   e.Timestamp,
			}
		}
		pb.TotalSpend += e.Amount
		pb.ByInstance[e.InstanceType] += e.Amount
		pb.EntryCount++
		if e.Timestamp.Before(pb.FirstSeen) {
			pb.FirstSeen = e.Timestamp
		}
		if e.Timestamp.After(pb.LastSeen) {
			pb.LastSeen = e.Timestamp
		}
		report.ByProvider[e.Provider] = pb

		if i == 0 || e.Timestamp.Before(globalFirst) {
			globalFirst = e.Timestamp
		}
		if i == 0 || e.Timestamp.After(globalLast) {
			globalLast = e.Timestamp
		}
	}

	report.ObservationWindow = globalLast.Sub(globalFirst)
	report.BurnRatePerHour = ct.burnRateLocked()
	report.BudgetRemaining = ct.budget - report.TotalSpend

	if ct.budget > 0 && report.BurnRatePerHour > 0 {
		remaining := ct.budget - report.TotalSpend
		if remaining <= 0 {
			report.ExhaustionTime = now
		} else {
			hoursLeft := remaining / report.BurnRatePerHour
			report.ExhaustionTime = now.Add(time.Duration(hoursLeft * float64(time.Hour)))
		}
	}

	return report
}

// String returns a human-readable summary of a CostReport.
func (r CostReport) String() string {
	s := fmt.Sprintf("Cost Report (generated %s)\n", r.GeneratedAt.Format(time.RFC3339))
	s += fmt.Sprintf("  Total Spend:    $%.4f\n", r.TotalSpend)
	s += fmt.Sprintf("  Burn Rate:      $%.4f/hr\n", r.BurnRatePerHour)
	s += fmt.Sprintf("  Observation:    %s\n", r.ObservationWindow)

	if r.BudgetLimit > 0 {
		s += fmt.Sprintf("  Budget:         $%.4f (remaining $%.4f)\n", r.BudgetLimit, r.BudgetRemaining)
		if !r.ExhaustionTime.IsZero() {
			s += fmt.Sprintf("  Exhaustion:     %s\n", r.ExhaustionTime.Format(time.RFC3339))
		}
	}

	for prov, pb := range r.ByProvider {
		s += fmt.Sprintf("  [%s] $%.4f (%d entries)\n", prov, pb.TotalSpend, pb.EntryCount)
		for inst, cost := range pb.ByInstance {
			s += fmt.Sprintf("    %-20s $%.4f\n", inst, cost)
		}
	}
	return s
}
