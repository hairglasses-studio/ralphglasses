package fleet

import (
	"sync"
)

// BudgetManager tracks per-worker budget limits and spend.
type BudgetManager struct {
	mu              sync.RWMutex
	defaultLimitUSD float64
	workerLimits    map[string]float64 // per-worker budget limits
	workerSpent     map[string]float64 // per-worker cumulative spend
}

// NewBudgetManager creates a BudgetManager with the given default per-worker limit.
func NewBudgetManager(defaultLimitUSD float64) *BudgetManager {
	return &BudgetManager{
		defaultLimitUSD: defaultLimitUSD,
		workerLimits:    make(map[string]float64),
		workerSpent:     make(map[string]float64),
	}
}

// SetWorkerLimit sets a custom budget limit for a specific worker.
func (b *BudgetManager) SetWorkerLimit(workerID string, limitUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.workerLimits[workerID] = limitUSD
}

// RecordSpend adds to a worker's cumulative spend.
func (b *BudgetManager) RecordSpend(workerID string, amountUSD float64) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.workerSpent[workerID] += amountUSD
}

// Remaining returns the budget remaining for a worker.
func (b *BudgetManager) Remaining(workerID string) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	limit := b.defaultLimitUSD
	if custom, ok := b.workerLimits[workerID]; ok {
		limit = custom
	}
	spent := b.workerSpent[workerID]

	remaining := limit - spent
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Spent returns the total spend for a worker.
func (b *BudgetManager) Spent(workerID string) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.workerSpent[workerID]
}

// Limit returns the budget limit for a worker.
func (b *BudgetManager) Limit(workerID string) float64 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if custom, ok := b.workerLimits[workerID]; ok {
		return custom
	}
	return b.defaultLimitUSD
}
