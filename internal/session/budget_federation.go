package session

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrFederationSessionNotFound is returned when a session ID is not in the federation.
var ErrFederationSessionNotFound = errors.New("session not found in federation")

// ErrFederationBudgetExceeded is returned when an allocation or spend would exceed the total budget.
var ErrFederationBudgetExceeded = errors.New("federation budget exceeded")

// ErrFederationSessionExhausted is returned when a session's allocated budget is exhausted.
var ErrFederationSessionExhausted = errors.New("session budget exhausted")

// FedOption configures a FederatedBudget.
type FedOption func(*FederatedBudget)

// WithReservePercent sets the fraction of total budget held in reserve
// for redistribution (default 0, meaning all budget is allocatable).
func WithReservePercent(pct float64) FedOption {
	return func(fb *FederatedBudget) {
		if pct >= 0 && pct < 1 {
			fb.reservePct = pct
		}
	}
}

// fedSessionEntry tracks a single session's allocation and spend.
type fedSessionEntry struct {
	allocated float64
	spent     float64
	finished  bool
}

// FederatedBudget manages a total budget across multiple sessions with
// per-session allocations, spend tracking, redistribution, and exhaustion callbacks.
// Thread-safe via sync.RWMutex.
type FederatedBudget struct {
	mu         sync.RWMutex
	totalUSD   float64
	reservePct float64
	sessions   map[string]*fedSessionEntry
	onExhaust  []func(sessionID string)
}

// NewFederatedBudget creates a federated budget manager with the given total USD ceiling.
func NewFederatedBudget(totalUSD float64, opts ...FedOption) *FederatedBudget {
	fb := &FederatedBudget{
		totalUSD: totalUSD,
		sessions: make(map[string]*fedSessionEntry),
	}
	for _, o := range opts {
		o(fb)
	}
	return fb
}

// Allocate reserves budget for a session. If the session already has an allocation,
// the amount is added to the existing allocation. Returns ErrFederationBudgetExceeded
// if the allocation would exceed the allocatable ceiling.
func (fb *FederatedBudget) Allocate(sessionID string, amountUSD float64) error {
	if amountUSD < 0 {
		return fmt.Errorf("allocation amount must be non-negative, got %.4f", amountUSD)
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()

	allocatable := fb.allocatableLocked()
	currentAllocated := fb.totalAllocatedLocked()
	if currentAllocated+amountUSD > allocatable {
		return fmt.Errorf("%w: allocating $%.4f would exceed allocatable $%.4f (already allocated $%.4f)",
			ErrFederationBudgetExceeded, amountUSD, allocatable, currentAllocated)
	}

	entry, ok := fb.sessions[sessionID]
	if !ok {
		entry = &fedSessionEntry{}
		fb.sessions[sessionID] = entry
	}
	entry.allocated += amountUSD
	return nil
}

// Spend records actual spend for a session. Returns ErrFederationSessionNotFound if
// the session has not been allocated. Returns ErrFederationSessionExhausted if the
// spend would exceed the session's allocation.
func (fb *FederatedBudget) Spend(sessionID string, amountUSD float64) error {
	if amountUSD < 0 {
		return fmt.Errorf("spend amount must be non-negative, got %.4f", amountUSD)
	}

	fb.mu.Lock()
	defer fb.mu.Unlock()

	entry, ok := fb.sessions[sessionID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrFederationSessionNotFound, sessionID)
	}

	if entry.allocated > 0 && entry.spent+amountUSD > entry.allocated {
		// Record the spend up to the allocation, then fire callback.
		entry.spent += amountUSD
		// Copy callbacks while holding lock, fire after release.
		callbacks := make([]func(string), len(fb.onExhaust))
		copy(callbacks, fb.onExhaust)
		fb.mu.Unlock()
		for _, fn := range callbacks {
			fn(sessionID)
		}
		fb.mu.Lock()
		return fmt.Errorf("%w: session %s spent $%.4f of $%.4f allocation",
			ErrFederationSessionExhausted, sessionID, entry.spent, entry.allocated)
	}

	entry.spent += amountUSD

	// Check if this spend crosses the threshold.
	if entry.allocated > 0 && entry.spent >= entry.allocated {
		callbacks := make([]func(string), len(fb.onExhaust))
		copy(callbacks, fb.onExhaust)
		fb.mu.Unlock()
		for _, fn := range callbacks {
			fn(sessionID)
		}
		fb.mu.Lock()
	}

	return nil
}

// Remaining returns the remaining budget for a specific session.
// Returns 0 if the session is not found or has no allocation.
func (fb *FederatedBudget) Remaining(sessionID string) float64 {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	entry, ok := fb.sessions[sessionID]
	if !ok {
		return 0
	}
	rem := entry.allocated - entry.spent
	if rem < 0 {
		return 0
	}
	return rem
}

// TotalRemaining returns the remaining budget across all sessions,
// calculated as total budget minus total spend.
func (fb *FederatedBudget) TotalRemaining() float64 {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	return fb.totalUSD - fb.totalSpentLocked()
}

// FinishSession marks a session as finished, making its unspent budget
// available for redistribution.
func (fb *FederatedBudget) FinishSession(sessionID string) error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	entry, ok := fb.sessions[sessionID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrFederationSessionNotFound, sessionID)
	}
	entry.finished = true
	return nil
}

// Redistribute rebalances unspent budget from finished sessions equally
// among active (non-finished) sessions. Returns ErrFederationSessionNotFound
// if there are no active sessions to redistribute to.
func (fb *FederatedBudget) Redistribute() error {
	fb.mu.Lock()
	defer fb.mu.Unlock()

	// Collect unspent from finished sessions.
	var surplus float64
	for _, entry := range fb.sessions {
		if entry.finished {
			unspent := entry.allocated - entry.spent
			if unspent > 0 {
				surplus += unspent
				entry.allocated = entry.spent // shrink to actual spend
			}
		}
	}

	if surplus <= 0 {
		return nil
	}

	// Count active sessions.
	var activeIDs []string
	for id, entry := range fb.sessions {
		if !entry.finished {
			activeIDs = append(activeIDs, id)
		}
	}

	if len(activeIDs) == 0 {
		return fmt.Errorf("%w: no active sessions for redistribution", ErrFederationSessionNotFound)
	}

	// Distribute equally.
	share := surplus / float64(len(activeIDs))
	for _, id := range activeIDs {
		fb.sessions[id].allocated += share
	}

	return nil
}

// OnBudgetExhausted registers a callback that fires when any session's spend
// reaches or exceeds its allocation. Multiple callbacks may be registered.
// Callbacks are invoked outside the lock and must not call back into the FederatedBudget.
func (fb *FederatedBudget) OnBudgetExhausted(fn func(sessionID string)) {
	fb.mu.Lock()
	defer fb.mu.Unlock()
	fb.onExhaust = append(fb.onExhaust, fn)
}

// FederatedSummary holds a point-in-time snapshot of the federated budget.
type FederatedSummary struct {
	TotalBudget    float64                          `json:"total_budget_usd"`
	TotalAllocated float64                          `json:"total_allocated_usd"`
	TotalSpent     float64                          `json:"total_spent_usd"`
	TotalRemaining float64                          `json:"total_remaining_usd"`
	ReservePct     float64                          `json:"reserve_pct"`
	Sessions       map[string]FederatedSessionInfo  `json:"sessions"`
	GeneratedAt    time.Time                        `json:"generated_at"`
}

// FederatedSessionInfo holds per-session budget data within the federation.
type FederatedSessionInfo struct {
	Allocated float64 `json:"allocated_usd"`
	Spent     float64 `json:"spent_usd"`
	Remaining float64 `json:"remaining_usd"`
	Finished  bool    `json:"finished"`
}

// Summary returns a point-in-time snapshot of the federation state.
func (fb *FederatedBudget) Summary() FederatedSummary {
	fb.mu.RLock()
	defer fb.mu.RUnlock()

	sessions := make(map[string]FederatedSessionInfo, len(fb.sessions))
	for id, e := range fb.sessions {
		rem := e.allocated - e.spent
		if rem < 0 {
			rem = 0
		}
		sessions[id] = FederatedSessionInfo{
			Allocated: e.allocated,
			Spent:     e.spent,
			Remaining: rem,
			Finished:  e.finished,
		}
	}

	totalSpent := fb.totalSpentLocked()
	totalAllocated := fb.totalAllocatedLocked()

	return FederatedSummary{
		TotalBudget:    fb.totalUSD,
		TotalAllocated: totalAllocated,
		TotalSpent:     totalSpent,
		TotalRemaining: fb.totalUSD - totalSpent,
		ReservePct:     fb.reservePct,
		Sessions:       sessions,
		GeneratedAt:    time.Now(),
	}
}

// allocatableLocked returns the portion of total budget available for allocation
// (total minus reserve). Caller must hold at least a read lock.
func (fb *FederatedBudget) allocatableLocked() float64 {
	return fb.totalUSD * (1 - fb.reservePct)
}

// totalAllocatedLocked returns total allocated across all sessions.
// Caller must hold at least a read lock.
func (fb *FederatedBudget) totalAllocatedLocked() float64 {
	var total float64
	for _, e := range fb.sessions {
		total += e.allocated
	}
	return total
}

// totalSpentLocked returns total spent across all sessions.
// Caller must hold at least a read lock.
func (fb *FederatedBudget) totalSpentLocked() float64 {
	var total float64
	for _, e := range fb.sessions {
		total += e.spent
	}
	return total
}
