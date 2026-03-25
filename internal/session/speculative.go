package session

// SpeculativeResult holds the outcome of a speculative parallel execution.
type SpeculativeResult struct {
	CheapSession      *Session
	ExpensiveSession  *Session
	Winner            string // "cheap" or "expensive"
	CheapDone         bool
	ExpensiveDone     bool
	CheapVerified     bool
	ExpensiveVerified bool
	CostSavedUSD      float64
}

// ResolveSpeculative picks the winner from two parallel sessions.
// Rules:
//  1. If both done and both verified: pick cheap (saves cost)
//  2. If only one verified: pick the verified one
//  3. If neither verified but both done: pick expensive (higher capability)
//  4. If only one done: pick the done one
func ResolveSpeculative(cheap, expensive *Session, cheapVerified, expensiveVerified bool) SpeculativeResult {
	cheapDone := cheap != nil && isSessionDone(cheap)
	expensiveDone := expensive != nil && isSessionDone(expensive)

	result := SpeculativeResult{
		CheapSession:      cheap,
		ExpensiveSession:  expensive,
		CheapDone:         cheapDone,
		ExpensiveDone:     expensiveDone,
		CheapVerified:     cheapVerified,
		ExpensiveVerified: expensiveVerified,
	}

	switch {
	// Rule 1: both done and both verified — cheap wins (saves cost)
	case cheapDone && expensiveDone && cheapVerified && expensiveVerified:
		result.Winner = "cheap"
		result.CostSavedUSD = expensiveCost(expensive) - cheapCost(cheap)

	// Rule 2: only one verified — pick the verified one
	case cheapVerified && !expensiveVerified:
		result.Winner = "cheap"
		if expensiveDone {
			result.CostSavedUSD = expensiveCost(expensive) - cheapCost(cheap)
		}
	case expensiveVerified && !cheapVerified:
		result.Winner = "expensive"

	// Rule 3: neither verified but both done — expensive wins (higher capability)
	case cheapDone && expensiveDone:
		result.Winner = "expensive"

	// Rule 4: only one done — pick the done one
	case cheapDone:
		result.Winner = "cheap"
	case expensiveDone:
		result.Winner = "expensive"

	// Fallback: neither done — expensive by default
	default:
		result.Winner = "expensive"
	}

	return result
}

// isSessionDone checks whether a session has reached a terminal status.
func isSessionDone(s *Session) bool {
	s.Lock()
	defer s.Unlock()
	switch s.Status {
	case StatusCompleted, StatusErrored, StatusStopped:
		return true
	}
	return false
}

// cheapCost returns the spent USD for a session, safe for nil.
func cheapCost(s *Session) float64 {
	if s == nil {
		return 0
	}
	s.Lock()
	defer s.Unlock()
	return s.SpentUSD
}

// expensiveCost returns the spent USD for a session, safe for nil.
func expensiveCost(s *Session) float64 {
	if s == nil {
		return 0
	}
	s.Lock()
	defer s.Unlock()
	return s.SpentUSD
}
