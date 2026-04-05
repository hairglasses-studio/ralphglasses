package fleet

import (
	"fmt"
	"sync"
	"time"
)

// TeamBudget tracks spending and enforces budget caps for a team.
type TeamBudget struct {
	Name       string    `json:"name"`
	CapUSD     float64   `json:"cap_usd"`
	SpentUSD   float64   `json:"spent_usd"`
	AlertSent  map[int]bool `json:"alert_sent"` // threshold% → sent
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// BudgetStore manages team budgets with thread-safe access.
type BudgetStore struct {
	mu      sync.RWMutex
	budgets map[string]*TeamBudget
}

// NewBudgetStore creates an empty budget store.
func NewBudgetStore() *BudgetStore {
	return &BudgetStore{
		budgets: make(map[string]*TeamBudget),
	}
}

// SetBudget creates or updates a team budget.
func (s *BudgetStore) SetBudget(name string, capUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if b, ok := s.budgets[name]; ok {
		b.CapUSD = capUSD
		b.UpdatedAt = time.Now()
	} else {
		s.budgets[name] = &TeamBudget{
			Name:      name,
			CapUSD:    capUSD,
			AlertSent: make(map[int]bool),
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
	}
}

// RecordSpend adds cost to a team's spend and returns any alert thresholds crossed.
func (s *BudgetStore) RecordSpend(name string, costUSD float64) []BudgetAlert {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, ok := s.budgets[name]
	if !ok {
		return nil
	}

	b.SpentUSD += costUSD
	b.UpdatedAt = time.Now()

	// Check thresholds
	var alerts []BudgetAlert
	thresholds := []int{50, 75, 90, 99}
	for _, pct := range thresholds {
		if b.CapUSD > 0 && b.SpentUSD >= b.CapUSD*float64(pct)/100 && !b.AlertSent[pct] {
			b.AlertSent[pct] = true
			alerts = append(alerts, BudgetAlert{
				Team:      name,
				Threshold: pct,
				SpentUSD:  b.SpentUSD,
				CapUSD:    b.CapUSD,
				Message:   fmt.Sprintf("Team %q has used %d%% of budget ($%.2f / $%.2f)", name, pct, b.SpentUSD, b.CapUSD),
			})
		}
	}

	return alerts
}

// CanSpend checks if a team can spend the given amount without exceeding their cap.
func (s *BudgetStore) CanSpend(name string, costUSD float64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.budgets[name]
	if !ok {
		return true // No budget = no limit
	}
	if b.CapUSD <= 0 {
		return true // Zero cap = unlimited
	}
	return b.SpentUSD+costUSD <= b.CapUSD
}

// GetBudget returns a copy of the team budget.
func (s *BudgetStore) GetBudget(name string) (TeamBudget, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	b, ok := s.budgets[name]
	if !ok {
		return TeamBudget{}, false
	}
	return *b, true
}

// ListBudgets returns all team budgets.
func (s *BudgetStore) ListBudgets() []TeamBudget {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]TeamBudget, 0, len(s.budgets))
	for _, b := range s.budgets {
		result = append(result, *b)
	}
	return result
}

// BudgetAlert represents a threshold alert.
type BudgetAlert struct {
	Team      string  `json:"team"`
	Threshold int     `json:"threshold_pct"`
	SpentUSD  float64 `json:"spent_usd"`
	CapUSD    float64 `json:"cap_usd"`
	Message   string  `json:"message"`
}
