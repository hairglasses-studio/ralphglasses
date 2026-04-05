package fleet

import (
	"sync"
)

// HealthAlertWatcher monitors worker health state transitions and fires
// callbacks when a worker's state changes. Each (workerID, state) pair
// fires at most once until the state changes again — duplicate
// notifications for the same state are suppressed.
type HealthAlertWatcher struct {
	mu        sync.Mutex
	tracker   *HealthTracker
	callbacks []func(workerID string, from, to HealthState)
	fired     map[string]HealthState // dedup: only fire on state change
}

// NewHealthAlertWatcher creates a watcher bound to the given tracker.
func NewHealthAlertWatcher(tracker *HealthTracker) *HealthAlertWatcher {
	return &HealthAlertWatcher{
		tracker: tracker,
		fired:   make(map[string]HealthState),
	}
}

// OnTransition registers a callback that fires when any worker transitions
// between health states. Multiple callbacks can be registered; they are
// called in registration order.
func (w *HealthAlertWatcher) OnTransition(fn func(workerID string, from, to HealthState)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.callbacks = append(w.callbacks, fn)
}

// Check polls the tracker for the given worker's current state and fires
// registered callbacks if the state differs from the last observed state.
// The first check for an unknown worker always fires (from="" equivalent
// is represented by the absence of a prior entry).
func (w *HealthAlertWatcher) Check(workerID string) {
	current := w.tracker.GetState(workerID)

	w.mu.Lock()
	last, seen := w.fired[workerID]
	if seen && last == current {
		w.mu.Unlock()
		return
	}
	w.fired[workerID] = current

	// Copy callbacks under the lock so we can invoke them without holding it.
	cbs := make([]func(string, HealthState, HealthState), len(w.callbacks))
	copy(cbs, w.callbacks)
	w.mu.Unlock()

	from := last
	if !seen {
		from = "" // no prior state
	}
	for _, cb := range cbs {
		cb(workerID, from, current)
	}
}

// CheckAll polls every worker currently tracked by the underlying
// HealthTracker and fires callbacks for any state changes.
func (w *HealthAlertWatcher) CheckAll() {
	ids := w.tracker.WorkerIDs()
	for _, id := range ids {
		w.Check(id)
	}
}
