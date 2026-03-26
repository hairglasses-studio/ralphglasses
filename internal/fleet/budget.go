package fleet

import (
	"sync"
)

// WorkerBudget tracks cost budget for a single worker.
type WorkerBudget struct {
	Limit float64 // maximum allowed cost in USD
	Spent float64 // total cost consumed so far
}

// Remaining returns the budget remaining.
func (wb WorkerBudget) Remaining() float64 {
	r := wb.Limit - wb.Spent
	if r < 0 {
		return 0
	}
	return r
}

// CanAcceptWork returns true if the worker has enough budget for the estimated cost.
func (wb WorkerBudget) CanAcceptWork(estimatedCost float64) bool {
	return wb.Remaining() >= estimatedCost
}

// BudgetManager tracks budgets across all workers.
type BudgetManager struct {
	mu           sync.RWMutex
	budgets      map[string]*WorkerBudget // worker ID -> budget
	defaultLimit float64
}

// NewBudgetManager creates a manager with a default per-worker limit.
func NewBudgetManager(defaultLimit float64) *BudgetManager {
	return &BudgetManager{
		budgets:      make(map[string]*WorkerBudget),
		defaultLimit: defaultLimit,
	}
}

// SetBudget sets a specific budget limit for a worker.
func (bm *BudgetManager) SetBudget(workerID string, limit float64) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if b, ok := bm.budgets[workerID]; ok {
		b.Limit = limit
	} else {
		bm.budgets[workerID] = &WorkerBudget{Limit: limit}
	}
}

// RecordCost adds cost to a worker's spent total.
func (bm *BudgetManager) RecordCost(workerID string, cost float64) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	b := bm.getOrCreate(workerID)
	b.Spent += cost
}

// CanAcceptWork checks if a worker can accept work at the estimated cost.
func (bm *BudgetManager) CanAcceptWork(workerID string, estimatedCost float64) bool {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	if b, ok := bm.budgets[workerID]; ok {
		return b.CanAcceptWork(estimatedCost)
	}
	// No budget set = use default limit
	return estimatedCost <= bm.defaultLimit
}

// GetBudget returns a copy of the worker's budget.
func (bm *BudgetManager) GetBudget(workerID string) WorkerBudget {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	if b, ok := bm.budgets[workerID]; ok {
		return *b
	}
	return WorkerBudget{Limit: bm.defaultLimit}
}

// Summary returns budget info for all tracked workers.
func (bm *BudgetManager) Summary() map[string]WorkerBudget {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	result := make(map[string]WorkerBudget, len(bm.budgets))
	for id, b := range bm.budgets {
		result[id] = *b
	}
	return result
}

func (bm *BudgetManager) getOrCreate(workerID string) *WorkerBudget {
	if b, ok := bm.budgets[workerID]; ok {
		return b
	}
	b := &WorkerBudget{Limit: bm.defaultLimit}
	bm.budgets[workerID] = b
	return b
}
