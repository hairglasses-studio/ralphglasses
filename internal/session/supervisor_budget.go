package session

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// BudgetEnvelope tracks spending in real-time and enforces budget limits.
type BudgetEnvelope struct {
	TotalBudgetUSD    float64
	PerCycleBudgetUSD float64 // 0 = TotalBudget/10 as default
	spentUSD          float64
	mu                sync.Mutex
}

// NewBudgetEnvelope creates a budget envelope with the given total budget.
// If perCycleBudget is 0, it defaults to TotalBudget/10.
func NewBudgetEnvelope(totalBudget float64) *BudgetEnvelope {
	return &BudgetEnvelope{
		TotalBudgetUSD: totalBudget,
	}
}

// CanSpend returns true if remaining budget >= estimatedCost.
func (be *BudgetEnvelope) CanSpend(estimatedCost float64) bool {
	be.mu.Lock()
	defer be.mu.Unlock()
	return be.TotalBudgetUSD-be.spentUSD >= estimatedCost
}

// RecordSpend adds cost to spent total. Thread-safe.
func (be *BudgetEnvelope) RecordSpend(amount float64) {
	be.mu.Lock()
	defer be.mu.Unlock()
	be.spentUSD += amount
}

// Remaining returns remaining budget.
func (be *BudgetEnvelope) Remaining() float64 {
	be.mu.Lock()
	defer be.mu.Unlock()
	return be.TotalBudgetUSD - be.spentUSD
}

// Spent returns total spent so far.
func (be *BudgetEnvelope) Spent() float64 {
	be.mu.Lock()
	defer be.mu.Unlock()
	return be.spentUSD
}

// PerCycleCap returns the per-cycle budget cap.
// If PerCycleBudgetUSD is 0, defaults to TotalBudgetUSD/10.
func (be *BudgetEnvelope) PerCycleCap() float64 {
	be.mu.Lock()
	defer be.mu.Unlock()
	if be.PerCycleBudgetUSD > 0 {
		return be.PerCycleBudgetUSD
	}
	return be.TotalBudgetUSD / 10
}

// MarshalJSON implements json.Marshaler for persistence in supervisor_state.json.
func (be *BudgetEnvelope) MarshalJSON() ([]byte, error) {
	be.mu.Lock()
	defer be.mu.Unlock()
	return json.Marshal(struct {
		TotalBudgetUSD    float64 `json:"total_budget_usd"`
		PerCycleBudgetUSD float64 `json:"per_cycle_budget_usd"`
		SpentUSD          float64 `json:"spent_usd"`
		RemainingUSD      float64 `json:"remaining_usd"`
	}{
		TotalBudgetUSD:    be.TotalBudgetUSD,
		PerCycleBudgetUSD: be.PerCycleBudgetUSD,
		SpentUSD:          be.spentUSD,
		RemainingUSD:      be.TotalBudgetUSD - be.spentUSD,
	})
}

// LoadFromState restores budget state from persisted data.
func (be *BudgetEnvelope) LoadFromState(spent float64) {
	be.mu.Lock()
	defer be.mu.Unlock()
	be.spentUSD = spent
}

// SubscribeToBus listens for cost events and auto-records spending.
// Call this in a goroutine. It blocks until ctx is cancelled.
func (be *BudgetEnvelope) SubscribeToBus(ctx context.Context, bus *events.Bus) {
	if bus == nil {
		<-ctx.Done()
		return
	}
	ch := bus.SubscribeFiltered("budget-envelope", events.CostUpdate)
	defer bus.Unsubscribe("budget-envelope")

	// Track per-session cumulative spend to compute deltas.
	sessionSpent := make(map[string]float64)

	for {
		select {
		case <-ctx.Done():
			return
		case evt, ok := <-ch:
			if !ok {
				return
			}
			be.handleCostEvent(evt, sessionSpent)
		}
	}
}

// handleCostEvent extracts cost data from a CostUpdate event and records the delta.
func (be *BudgetEnvelope) handleCostEvent(evt events.Event, sessionSpent map[string]float64) {
	if evt.Data == nil {
		return
	}

	// CostUpdate events carry "spent_usd" as the cumulative session spend.
	spentRaw, ok := evt.Data["spent_usd"]
	if !ok {
		return
	}
	spent, ok := spentRaw.(float64)
	if !ok {
		return
	}

	sid := evt.SessionID
	prev := sessionSpent[sid]
	if spent > prev {
		delta := spent - prev
		sessionSpent[sid] = spent
		be.RecordSpend(delta)
		slog.Debug("budget: recorded spend", "session", sid, "delta", delta, "total_spent", be.Spent())
	}
}
