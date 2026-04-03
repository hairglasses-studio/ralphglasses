package safety

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// Fleet-level anomaly types extending the per-session types in anomaly.go.
const (
	// MultiRepoSpendSpike detects when total org-wide spend exceeds threshold
	// within a time window, indicating runaway fleet costs.
	MultiRepoSpendSpike AnomalyType = "multi_repo_spend_spike"

	// ModelDegradation detects when a provider's success rate drops sharply
	// across multiple sessions, suggesting an API outage or quality regression.
	ModelDegradation AnomalyType = "model_degradation"

	// FleetBudgetExhaustion detects when the global fleet budget is projected
	// to be exhausted before the planned duration ends.
	FleetBudgetExhaustion AnomalyType = "fleet_budget_exhaustion"

	// WorkerSaturation detects when all workers are at capacity with a
	// growing queue, indicating need for scaling or throttling.
	WorkerSaturation AnomalyType = "worker_saturation"
)

// FleetAnomalyConfig controls fleet-level anomaly detection thresholds.
type FleetAnomalyConfig struct {
	// SpendSpikeWindowMinutes is the time window for aggregating total spend.
	SpendSpikeWindowMinutes int `json:"spend_spike_window_minutes"` // default 15

	// SpendSpikeThresholdUSD is the dollar amount that triggers an alert.
	SpendSpikeThresholdUSD float64 `json:"spend_spike_threshold_usd"` // default 50.0

	// DegradationWindowMinutes is the time window for measuring provider success rate.
	DegradationWindowMinutes int `json:"degradation_window_minutes"` // default 30

	// DegradationThreshold is the success rate below which degradation is flagged (0-1).
	DegradationThreshold float64 `json:"degradation_threshold"` // default 0.5

	// BudgetExhaustionHorizonHours is how far ahead to project budget exhaustion.
	BudgetExhaustionHorizonHours int `json:"budget_exhaustion_horizon_hours"` // default 4

	// WorkerQueueDepthThreshold triggers saturation when queue exceeds this.
	WorkerQueueDepthThreshold int `json:"worker_queue_depth_threshold"` // default 20

	// KillSwitchOnCritical auto-engages kill switch for critical fleet anomalies.
	KillSwitchOnCritical bool `json:"kill_switch_on_critical"`
}

// DefaultFleetAnomalyConfig returns sensible defaults.
func DefaultFleetAnomalyConfig() FleetAnomalyConfig {
	return FleetAnomalyConfig{
		SpendSpikeWindowMinutes:      15,
		SpendSpikeThresholdUSD:       50.0,
		DegradationWindowMinutes:     30,
		DegradationThreshold:         0.5,
		BudgetExhaustionHorizonHours: 4,
		WorkerQueueDepthThreshold:    20,
		KillSwitchOnCritical:         false,
	}
}

// providerOutcome tracks individual session outcomes for degradation detection.
type providerOutcome struct {
	Provider  string
	Success   bool
	Timestamp time.Time
}

// spendEvent tracks fleet-wide spend events.
type spendEvent struct {
	CostUSD   float64
	Timestamp time.Time
}

// FleetAnomalyDetector monitors fleet-wide patterns that span multiple
// sessions, providers, and repos. It extends the per-session AnomalyDetector
// with org-level detection.
//
// Informed by research:
// - Scaling Agent Systems (2512.08296): error amplification detection
// - AI Agent Index (2602.17753): fleet-level safety audit
type FleetAnomalyDetector struct {
	mu        sync.Mutex
	bus       *events.Bus
	cfg       FleetAnomalyConfig
	ks        *KillSwitch
	callbacks []func(Anomaly)

	// Fleet-level tracking
	recentSpend    []spendEvent      // rolling window of spend events
	providerStats  []providerOutcome // rolling window of provider outcomes
	queueDepth     int               // current queue depth (updated by fleet events)
	totalBudgetUSD float64           // total fleet budget
	spentUSD       float64           // total fleet spend

	// Lifecycle
	cancel context.CancelFunc
	done   chan struct{}
}

// NewFleetAnomalyDetector creates a fleet-level anomaly detector.
func NewFleetAnomalyDetector(bus *events.Bus, cfg FleetAnomalyConfig, ks *KillSwitch) *FleetAnomalyDetector {
	return &FleetAnomalyDetector{
		bus:  bus,
		cfg:  cfg,
		ks:   ks,
		done: make(chan struct{}),
	}
}

// OnAnomaly registers a callback for fleet anomaly detection.
func (d *FleetAnomalyDetector) OnAnomaly(fn func(Anomaly)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callbacks = append(d.callbacks, fn)
}

// SetBudget updates the total fleet budget for exhaustion projection.
func (d *FleetAnomalyDetector) SetBudget(totalUSD, spentUSD float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.totalBudgetUSD = totalUSD
	d.spentUSD = spentUSD
}

// RecordSpend records a fleet spend event for spike detection.
func (d *FleetAnomalyDetector) RecordSpend(costUSD float64) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.recentSpend = append(d.recentSpend, spendEvent{
		CostUSD:   costUSD,
		Timestamp: time.Now(),
	})
	d.spentUSD += costUSD
	d.trimSpend()
}

// RecordProviderOutcome records a provider success/failure for degradation detection.
func (d *FleetAnomalyDetector) RecordProviderOutcome(provider string, success bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.providerStats = append(d.providerStats, providerOutcome{
		Provider:  provider,
		Success:   success,
		Timestamp: time.Now(),
	})
	d.trimProviderStats()
}

// SetQueueDepth updates the current worker queue depth.
func (d *FleetAnomalyDetector) SetQueueDepth(depth int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queueDepth = depth
}

// Check runs all fleet-level anomaly checks and returns detected anomalies.
func (d *FleetAnomalyDetector) Check() []Anomaly {
	d.mu.Lock()
	defer d.mu.Unlock()

	var anomalies []Anomaly

	// 1. Multi-repo spend spike
	if a := d.checkSpendSpike(); a != nil {
		anomalies = append(anomalies, *a)
	}

	// 2. Provider degradation
	for _, a := range d.checkProviderDegradation() {
		anomalies = append(anomalies, a)
	}

	// 3. Budget exhaustion projection
	if a := d.checkBudgetExhaustion(); a != nil {
		anomalies = append(anomalies, *a)
	}

	// 4. Worker saturation
	if a := d.checkWorkerSaturation(); a != nil {
		anomalies = append(anomalies, *a)
	}

	// Fire callbacks and optionally engage kill switch
	for _, a := range anomalies {
		for _, cb := range d.callbacks {
			cb(a)
		}

		if a.Severity == SeverityCritical && d.cfg.KillSwitchOnCritical && d.ks != nil {
			slog.Warn("fleet anomaly: engaging kill switch",
				"type", a.Type,
				"description", a.Description,
			)
			d.ks.Engage(fmt.Sprintf("fleet anomaly: %s", a.Description))
		}
	}

	return anomalies
}

// Start begins periodic fleet anomaly checking.
func (d *FleetAnomalyDetector) Start(ctx context.Context, interval time.Duration) {
	ctx, d.cancel = context.WithCancel(ctx)
	go func() {
		defer close(d.done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				anomalies := d.Check()
				if len(anomalies) > 0 {
					slog.Info("fleet anomaly check",
						"detected", len(anomalies),
					)
				}
			}
		}
	}()
}

// Stop halts the periodic checking.
func (d *FleetAnomalyDetector) Stop() {
	if d.cancel != nil {
		d.cancel()
	}
	<-d.done
}

// --- Internal checks ---

func (d *FleetAnomalyDetector) checkSpendSpike() *Anomaly {
	window := time.Duration(d.cfg.SpendSpikeWindowMinutes) * time.Minute
	cutoff := time.Now().Add(-window)

	var windowSpend float64
	for _, e := range d.recentSpend {
		if e.Timestamp.After(cutoff) {
			windowSpend += e.CostUSD
		}
	}

	if windowSpend > d.cfg.SpendSpikeThresholdUSD {
		return &Anomaly{
			Type:              MultiRepoSpendSpike,
			Severity:          SeverityCritical,
			Description:       fmt.Sprintf("Fleet spend $%.2f in %dm exceeds threshold $%.2f", windowSpend, d.cfg.SpendSpikeWindowMinutes, d.cfg.SpendSpikeThresholdUSD),
			Timestamp:         time.Now(),
			RecommendedAction: "Throttle session launches, review budget allocation",
		}
	}
	return nil
}

func (d *FleetAnomalyDetector) checkProviderDegradation() []Anomaly {
	window := time.Duration(d.cfg.DegradationWindowMinutes) * time.Minute
	cutoff := time.Now().Add(-window)

	// Group by provider
	type stats struct {
		total, successes int
	}
	byProvider := make(map[string]*stats)
	for _, o := range d.providerStats {
		if o.Timestamp.Before(cutoff) {
			continue
		}
		s, ok := byProvider[o.Provider]
		if !ok {
			s = &stats{}
			byProvider[o.Provider] = s
		}
		s.total++
		if o.Success {
			s.successes++
		}
	}

	var anomalies []Anomaly
	for provider, s := range byProvider {
		if s.total < 5 { // need minimum sample size
			continue
		}
		rate := float64(s.successes) / float64(s.total)
		if rate < d.cfg.DegradationThreshold {
			anomalies = append(anomalies, Anomaly{
				Type:              ModelDegradation,
				Severity:          SeverityWarning,
				Description:       fmt.Sprintf("Provider %s success rate %.1f%% (%d/%d) below threshold %.0f%%", provider, rate*100, s.successes, s.total, d.cfg.DegradationThreshold*100),
				Timestamp:         time.Now(),
				RecommendedAction: fmt.Sprintf("Route away from %s, check API status", provider),
			})
		}
	}
	return anomalies
}

func (d *FleetAnomalyDetector) checkBudgetExhaustion() *Anomaly {
	if d.totalBudgetUSD <= 0 {
		return nil
	}

	remaining := d.totalBudgetUSD - d.spentUSD
	if remaining <= 0 {
		return &Anomaly{
			Type:              FleetBudgetExhaustion,
			Severity:          SeverityCritical,
			Description:       fmt.Sprintf("Fleet budget exhausted: $%.2f spent of $%.2f total", d.spentUSD, d.totalBudgetUSD),
			Timestamp:         time.Now(),
			RecommendedAction: "Stop all sessions, increase budget allocation",
		}
	}

	// Project exhaustion based on recent spend rate
	window := time.Duration(d.cfg.SpendSpikeWindowMinutes) * time.Minute
	cutoff := time.Now().Add(-window)
	var windowSpend float64
	for _, e := range d.recentSpend {
		if e.Timestamp.After(cutoff) {
			windowSpend += e.CostUSD
		}
	}

	if windowSpend > 0 {
		ratePerHour := windowSpend / window.Hours()
		hoursRemaining := remaining / ratePerHour
		horizonHours := float64(d.cfg.BudgetExhaustionHorizonHours)

		if hoursRemaining < horizonHours {
			return &Anomaly{
				Type:              FleetBudgetExhaustion,
				Severity:          SeverityWarning,
				Description:       fmt.Sprintf("Fleet budget projected exhausted in %.1f hours (rate: $%.2f/hr, remaining: $%.2f)", hoursRemaining, ratePerHour, remaining),
				Timestamp:         time.Now(),
				RecommendedAction: fmt.Sprintf("Reduce fleet size or switch to cheaper providers (burn rate: $%.2f/hr)", ratePerHour),
			}
		}
	}

	return nil
}

func (d *FleetAnomalyDetector) checkWorkerSaturation() *Anomaly {
	if d.queueDepth > d.cfg.WorkerQueueDepthThreshold {
		return &Anomaly{
			Type:              WorkerSaturation,
			Severity:          SeverityWarning,
			Description:       fmt.Sprintf("Worker queue depth %d exceeds threshold %d", d.queueDepth, d.cfg.WorkerQueueDepthThreshold),
			Timestamp:         time.Now(),
			RecommendedAction: "Scale up workers or throttle task submission",
		}
	}
	return nil
}

// --- Trim helpers ---

func (d *FleetAnomalyDetector) trimSpend() {
	cutoff := time.Now().Add(-time.Duration(d.cfg.SpendSpikeWindowMinutes*2) * time.Minute)
	filtered := d.recentSpend[:0]
	for _, e := range d.recentSpend {
		if e.Timestamp.After(cutoff) {
			filtered = append(filtered, e)
		}
	}
	d.recentSpend = filtered
}

func (d *FleetAnomalyDetector) trimProviderStats() {
	cutoff := time.Now().Add(-time.Duration(d.cfg.DegradationWindowMinutes*2) * time.Minute)
	filtered := d.providerStats[:0]
	for _, o := range d.providerStats {
		if o.Timestamp.After(cutoff) {
			filtered = append(filtered, o)
		}
	}
	d.providerStats = filtered
}
