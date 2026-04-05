// Package pool provides fleet-wide session budget pooling and metrics aggregation.
// It is a leaf package with no internal project dependencies, designed to be
// imported by both the session and fleet packages without creating import cycles.
package pool

import (
	"maps"
	"sync"
	"time"
)

// SessionSnapshot is a read-only copy of session fields needed for fleet aggregation.
// Created by session.Manager.SnapshotSessions() to avoid holding session locks.
type SessionSnapshot struct {
	ID        string
	Provider  string
	Status    string
	SpentUSD  float64
	BudgetUSD float64
	RepoPath  string
	StartedAt time.Time
}

// State holds fleet-wide aggregated metrics and budgets computed from
// per-session data. This is the local-node aggregation layer, complementing
// the distributed FleetState used by the fleet coordinator.
type State struct {
	mu sync.RWMutex

	// Budget pool
	TotalBudgetUSD float64
	TotalSpentUSD  float64
	BudgetCapUSD   float64 // fleet-wide hard cap (0 = unlimited)

	// Metrics
	ActiveSessions int
	TotalSessions  int
	ActiveLoops    int
	ProviderCounts map[string]int

	// Cost rate tracking (rolling window)
	CostRatePerHour float64
	CostHistory     []CostSample

	// Timestamps
	LastUpdate time.Time
}

// CostSample records a spend observation at a point in time.
type CostSample struct {
	Time     time.Time
	SpentUSD float64
}

// Summary is an immutable snapshot for rendering.
type Summary struct {
	TotalBudgetUSD  float64
	TotalSpentUSD   float64
	BudgetCapUSD    float64
	BudgetPct       float64 // spent/cap percentage (0 if no cap)
	ActiveSessions  int
	ActiveLoops     int
	CostRatePerHour float64
	ProviderCounts  map[string]int
	AtCapacity      bool // true if at or near budget cap (>=95%)
}

// NewState creates a fleet pool state with optional budget cap.
func NewState(budgetCapUSD float64) *State {
	return &State{
		BudgetCapUSD:   budgetCapUSD,
		ProviderCounts: make(map[string]int),
	}
}

// SetBudgetCap updates the fleet-wide hard budget cap.
// A value of 0 means unlimited.
func (s *State) SetBudgetCap(capUSD float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.BudgetCapUSD = capUSD
}

// Update refreshes fleet state from session snapshot data.
func (s *State) Update(sessions []SessionSnapshot) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.TotalSpentUSD = 0
	s.TotalBudgetUSD = 0
	s.ActiveSessions = 0
	s.TotalSessions = len(sessions)
	s.ProviderCounts = make(map[string]int, 3)

	for _, snap := range sessions {
		s.TotalSpentUSD += snap.SpentUSD
		s.TotalBudgetUSD += snap.BudgetUSD
		s.ProviderCounts[snap.Provider]++
		if snap.Status == "running" || snap.Status == "launching" {
			s.ActiveSessions++
		}
	}

	// Append cost sample for rate tracking
	now := time.Now()
	s.CostHistory = append(s.CostHistory, CostSample{
		Time:     now,
		SpentUSD: s.TotalSpentUSD,
	})

	// Keep at most 360 samples (~1h at 10s intervals)
	if len(s.CostHistory) > 360 {
		s.CostHistory = s.CostHistory[len(s.CostHistory)-360:]
	}

	s.CostRatePerHour = s.costRateLocked(time.Hour)
	s.LastUpdate = now
}

// CanSpend returns true if spending amount would not exceed fleet budget cap.
// Returns true if no cap is set (BudgetCapUSD == 0).
func (s *State) CanSpend(amountUSD float64) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.BudgetCapUSD <= 0 {
		return true
	}
	return s.TotalSpentUSD+amountUSD <= s.BudgetCapUSD
}

// CostRate returns the trailing cost rate in USD/hour over the given window.
func (s *State) CostRate(window time.Duration) float64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.costRateLocked(window)
}

// costRateLocked computes cost rate without acquiring the lock.
func (s *State) costRateLocked(window time.Duration) float64 {
	if len(s.CostHistory) < 2 {
		return 0
	}
	cutoff := time.Now().Add(-window)
	// Find the earliest sample within the window
	var first *CostSample
	for i := range s.CostHistory {
		if !s.CostHistory[i].Time.Before(cutoff) {
			first = &s.CostHistory[i]
			break
		}
	}
	if first == nil {
		// All samples are older than window — use oldest and newest
		first = &s.CostHistory[0]
	}
	last := s.CostHistory[len(s.CostHistory)-1]
	elapsed := last.Time.Sub(first.Time)
	if elapsed <= 0 {
		return 0
	}
	delta := last.SpentUSD - first.SpentUSD
	return delta / elapsed.Hours()
}

// GetSummary returns an immutable snapshot for display.
func (s *State) GetSummary() Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sum := Summary{
		TotalBudgetUSD:  s.TotalBudgetUSD,
		TotalSpentUSD:   s.TotalSpentUSD,
		BudgetCapUSD:    s.BudgetCapUSD,
		ActiveSessions:  s.ActiveSessions,
		ActiveLoops:     s.ActiveLoops,
		CostRatePerHour: s.CostRatePerHour,
		ProviderCounts:  make(map[string]int, len(s.ProviderCounts)),
	}
	maps.Copy(sum.ProviderCounts, s.ProviderCounts)
	if s.BudgetCapUSD > 0 {
		sum.BudgetPct = (s.TotalSpentUSD / s.BudgetCapUSD) * 100
		sum.AtCapacity = sum.BudgetPct >= 95
	}
	return sum
}
