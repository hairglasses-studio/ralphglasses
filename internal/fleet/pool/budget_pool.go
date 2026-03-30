package pool

import (
	"fmt"
	"sync"
)

// BudgetPool manages a global fleet budget with per-session allocations.
// It acts as a reservation ledger: sessions must allocate budget before
// starting and release it when finished. This prevents over-commitment
// of the fleet-wide budget cap.
type BudgetPool struct {
	mu          sync.RWMutex
	globalLimit float64            // 0 = unlimited
	allocations map[string]float64 // session ID -> reserved budget
}

// NewBudgetPool creates a budget pool with the given fleet-wide limit.
// A limit of 0 means unlimited (all allocations succeed).
func NewBudgetPool(limitUSD float64) *BudgetPool {
	return &BudgetPool{
		globalLimit: limitUSD,
		allocations: make(map[string]float64),
	}
}

// SetGlobalBudget updates the fleet-wide budget limit.
// A value of 0 means unlimited.
func (p *BudgetPool) SetGlobalBudget(limitUSD float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.globalLimit = limitUSD
}

// GlobalBudget returns the current fleet-wide budget limit.
func (p *BudgetPool) GlobalBudget() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.globalLimit
}

// AllocateSession reserves budgetUSD for the given session.
// Returns an error if the allocation would exceed the remaining global budget.
// If the session already has an allocation, the old one is replaced.
func (p *BudgetPool) AllocateSession(sessionID string, budgetUSD float64) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if budgetUSD < 0 {
		return fmt.Errorf("budget_pool: negative allocation $%.2f for session %s", budgetUSD, sessionID)
	}

	// Unlimited mode: always succeeds.
	if p.globalLimit <= 0 {
		p.allocations[sessionID] = budgetUSD
		return nil
	}

	// Compute total allocated, excluding any existing allocation for this session
	// (in case of re-allocation / budget adjustment).
	totalAllocated := 0.0
	for id, alloc := range p.allocations {
		if id != sessionID {
			totalAllocated += alloc
		}
	}

	remaining := p.globalLimit - totalAllocated
	if budgetUSD > remaining {
		return fmt.Errorf("budget_pool: allocation $%.2f exceeds remaining $%.2f (limit $%.2f, allocated $%.2f)",
			budgetUSD, remaining, p.globalLimit, totalAllocated)
	}

	p.allocations[sessionID] = budgetUSD
	return nil
}

// ReleaseSession frees the budget allocation for the given session.
// No-op if the session has no allocation.
func (p *BudgetPool) ReleaseSession(sessionID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.allocations, sessionID)
}

// Remaining returns the unallocated budget.
// Returns math.MaxFloat64 conceptually if unlimited (practically returns 0
// when globalLimit is 0 to signal "no cap" — callers should check GlobalBudget first).
func (p *BudgetPool) Remaining() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.globalLimit <= 0 {
		return 0 // 0 signals unlimited; callers check GlobalBudget() == 0
	}

	totalAllocated := 0.0
	for _, alloc := range p.allocations {
		totalAllocated += alloc
	}
	remaining := p.globalLimit - totalAllocated
	if remaining < 0 {
		return 0
	}
	return remaining
}

// TotalAllocated returns the sum of all current session allocations.
func (p *BudgetPool) TotalAllocated() float64 {
	p.mu.RLock()
	defer p.mu.RUnlock()

	total := 0.0
	for _, alloc := range p.allocations {
		total += alloc
	}
	return total
}

// SessionCount returns the number of sessions with active allocations.
func (p *BudgetPool) SessionCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return len(p.allocations)
}

// Allocation returns the budget reserved for a specific session, and whether
// the session has an allocation.
func (p *BudgetPool) Allocation(sessionID string) (float64, bool) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	alloc, ok := p.allocations[sessionID]
	return alloc, ok
}
