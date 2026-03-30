package session

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrBudgetCeiling is returned when an allocation would exceed the pool ceiling.
var ErrBudgetCeiling = errors.New("budget ceiling reached")

// ErrBudgetSessionNotFound is returned when a session ID is not in the pool.
var ErrBudgetSessionNotFound = errors.New("session not found in budget pool")

// BudgetSummary holds a point-in-time snapshot of the budget pool.
type BudgetSummary struct {
	TotalCeiling float64                      `json:"total_ceiling_usd"`
	TotalSpent   float64                      `json:"total_spent_usd"`
	Remaining    float64                      `json:"remaining_usd"`
	Sessions     map[string]SessionBudgetInfo `json:"sessions"`
	GeneratedAt  time.Time                    `json:"generated_at"`
}

// SessionBudgetInfo holds per-session budget data within a pool.
type SessionBudgetInfo struct {
	Allocated float64 `json:"allocated_usd"`
	Spent     float64 `json:"spent_usd"`
}

// BudgetPool manages a total budget ceiling across all sessions.
// Thread-safe via sync.RWMutex.
type BudgetPool struct {
	mu           sync.RWMutex
	totalCeiling float64
	sessions     map[string]*budgetPoolEntry
}

type budgetPoolEntry struct {
	allocated float64
	spent     float64
}

// NewBudgetPool creates a pool with the given total ceiling in USD.
func NewBudgetPool(totalCeiling float64) *BudgetPool {
	return &BudgetPool{
		totalCeiling: totalCeiling,
		sessions:     make(map[string]*budgetPoolEntry),
	}
}

// Allocate reserves budget for a session. Returns ErrBudgetCeiling if
// the allocation would exceed the pool ceiling.
func (bp *BudgetPool) Allocate(sessionID string, amount float64) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	totalAllocated := bp.totalAllocatedLocked()
	if totalAllocated+amount > bp.totalCeiling {
		return fmt.Errorf("%w: allocating $%.2f would exceed ceiling $%.2f (already allocated $%.2f)",
			ErrBudgetCeiling, amount, bp.totalCeiling, totalAllocated)
	}

	entry, ok := bp.sessions[sessionID]
	if !ok {
		entry = &budgetPoolEntry{}
		bp.sessions[sessionID] = entry
	}
	entry.allocated += amount
	return nil
}

// Record records actual spend for a session. The session must have been
// previously added via Allocate.
func (bp *BudgetPool) Record(sessionID string, cost float64) {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	entry, ok := bp.sessions[sessionID]
	if !ok {
		// Auto-create entry for sessions that record without explicit allocation.
		entry = &budgetPoolEntry{}
		bp.sessions[sessionID] = entry
	}
	entry.spent += cost
}

// Remaining returns how much budget remains before hitting the ceiling,
// based on actual spend (not allocations).
func (bp *BudgetPool) Remaining() float64 {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.totalCeiling - bp.totalSpentLocked()
}

// ShouldPause returns true if the session's spend has reached or exceeded
// its allocation, or if the pool's total spend has reached the ceiling.
func (bp *BudgetPool) ShouldPause(sessionID string) bool {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	// Pool-level ceiling check.
	if bp.totalSpentLocked() >= bp.totalCeiling {
		return true
	}

	// Per-session allocation check.
	entry, ok := bp.sessions[sessionID]
	if !ok {
		return false
	}
	if entry.allocated > 0 && entry.spent >= entry.allocated {
		return true
	}
	return false
}

// Summary returns a point-in-time snapshot of the pool state.
func (bp *BudgetPool) Summary() BudgetSummary {
	bp.mu.RLock()
	defer bp.mu.RUnlock()

	sessions := make(map[string]SessionBudgetInfo, len(bp.sessions))
	for id, e := range bp.sessions {
		sessions[id] = SessionBudgetInfo{
			Allocated: e.allocated,
			Spent:     e.spent,
		}
	}

	totalSpent := bp.totalSpentLocked()
	return BudgetSummary{
		TotalCeiling: bp.totalCeiling,
		TotalSpent:   totalSpent,
		Remaining:    bp.totalCeiling - totalSpent,
		Sessions:     sessions,
		GeneratedAt:  time.Now(),
	}
}

// TotalCeiling returns the pool's total budget ceiling.
func (bp *BudgetPool) TotalCeiling() float64 {
	bp.mu.RLock()
	defer bp.mu.RUnlock()
	return bp.totalCeiling
}

// totalAllocatedLocked returns total allocated across all sessions.
// Caller must hold at least a read lock.
func (bp *BudgetPool) totalAllocatedLocked() float64 {
	var total float64
	for _, e := range bp.sessions {
		total += e.allocated
	}
	return total
}

// totalSpentLocked returns total spent across all sessions.
// Caller must hold at least a read lock.
func (bp *BudgetPool) totalSpentLocked() float64 {
	var total float64
	for _, e := range bp.sessions {
		total += e.spent
	}
	return total
}
