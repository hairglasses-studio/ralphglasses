package pool

import (
	"sync"
	"time"
)

// AlertLevel represents a budget usage threshold that triggers an alert.
type AlertLevel int

const (
	AlertLevel50  AlertLevel = 50
	AlertLevel75  AlertLevel = 75
	AlertLevel90  AlertLevel = 90
	AlertLevel100 AlertLevel = 100
)

// DefaultAlertThresholds are the standard fleet budget alert levels.
var DefaultAlertThresholds = []AlertLevel{
	AlertLevel50,
	AlertLevel75,
	AlertLevel90,
	AlertLevel100,
}

// BudgetAlert is emitted when fleet spend crosses a threshold.
type BudgetAlert struct {
	Level     AlertLevel `json:"level"`      // Threshold that was crossed (50, 75, 90, 100)
	SpentUSD  float64    `json:"spent_usd"`  // Current total spend
	BudgetUSD float64    `json:"budget_usd"` // Fleet budget cap
	Pct       float64    `json:"pct"`        // Actual spend percentage
	Timestamp time.Time  `json:"timestamp"`
}

// AlertCallback is invoked when a budget threshold is crossed.
type AlertCallback func(BudgetAlert)

// BudgetAlertWatcher monitors fleet spend against the budget cap and fires
// callbacks when configurable thresholds are crossed. Each threshold fires
// at most once per watcher lifetime; call Reset to re-arm all thresholds.
type BudgetAlertWatcher struct {
	mu         sync.Mutex
	thresholds []AlertLevel
	emitted    map[AlertLevel]bool
	callbacks  []AlertCallback
	nowFunc    func() time.Time // injectable clock for testing
}

// WatcherOption configures a BudgetAlertWatcher during construction.
type WatcherOption func(*BudgetAlertWatcher)

// WithThresholds overrides the default alert thresholds.
func WithThresholds(levels []AlertLevel) WatcherOption {
	return func(w *BudgetAlertWatcher) {
		w.thresholds = make([]AlertLevel, len(levels))
		copy(w.thresholds, levels)
	}
}

// WithClock injects a time source (useful for deterministic tests).
func WithClock(fn func() time.Time) WatcherOption {
	return func(w *BudgetAlertWatcher) {
		w.nowFunc = fn
	}
}

// NewBudgetAlertWatcher creates a watcher with optional configuration.
// Without WithThresholds, it uses DefaultAlertThresholds (50, 75, 90, 100).
func NewBudgetAlertWatcher(opts ...WatcherOption) *BudgetAlertWatcher {
	w := &BudgetAlertWatcher{
		thresholds: make([]AlertLevel, len(DefaultAlertThresholds)),
		emitted:    make(map[AlertLevel]bool),
		nowFunc:    time.Now,
	}
	copy(w.thresholds, DefaultAlertThresholds)
	for _, opt := range opts {
		opt(w)
	}
	// Ensure emitted map has entries for all configured thresholds.
	for _, lvl := range w.thresholds {
		w.emitted[lvl] = false
	}
	return w
}

// OnAlert registers a callback that fires when any threshold is crossed.
// Multiple callbacks can be registered; they are called in registration order.
func (w *BudgetAlertWatcher) OnAlert(cb AlertCallback) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, cb)
}

// Check evaluates current spend against the budget cap and fires callbacks
// for any newly crossed thresholds. Returns all alerts emitted during this
// check (may be zero or multiple if spend jumped across several thresholds).
// If budgetCapUSD is zero (unlimited), no alerts are emitted.
func (w *BudgetAlertWatcher) Check(spentUSD, budgetCapUSD float64) []BudgetAlert {
	if budgetCapUSD <= 0 {
		return nil
	}

	pct := (spentUSD / budgetCapUSD) * 100

	w.mu.Lock()
	defer w.mu.Unlock()

	var fired []BudgetAlert
	now := w.nowFunc()

	for _, lvl := range w.thresholds {
		if pct >= float64(lvl) && !w.emitted[lvl] {
			w.emitted[lvl] = true
			alert := BudgetAlert{
				Level:     lvl,
				SpentUSD:  spentUSD,
				BudgetUSD: budgetCapUSD,
				Pct:       pct,
				Timestamp: now,
			}
			fired = append(fired, alert)
			for _, cb := range w.callbacks {
				cb(alert)
			}
		}
	}
	return fired
}

// Emitted returns the set of thresholds that have already fired.
func (w *BudgetAlertWatcher) Emitted() map[AlertLevel]bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make(map[AlertLevel]bool, len(w.emitted))
	for k, v := range w.emitted {
		out[k] = v
	}
	return out
}

// Reset re-arms all thresholds so they can fire again.
// Useful after a budget cap increase or a new billing period.
func (w *BudgetAlertWatcher) Reset() {
	w.mu.Lock()
	defer w.mu.Unlock()
	for lvl := range w.emitted {
		w.emitted[lvl] = false
	}
}
