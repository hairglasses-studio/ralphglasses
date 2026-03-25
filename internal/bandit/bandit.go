package bandit

import (
	"math"
	"math/rand"
	"sync"
)

// selectorArm wraps an Arm with UCB1 state.
type selectorArm struct {
	arm         Arm
	pulls       int
	totalReward float64
}

// Selector implements an Upper Confidence Bound (UCB1) multi-armed bandit
// for provider selection. It is independent of cascade routing.
type Selector struct {
	mu   sync.Mutex
	arms []selectorArm
}

// NewSelector creates a bandit selector from the given arms.
func NewSelector(arms []Arm) *Selector {
	sa := make([]selectorArm, len(arms))
	for i, a := range arms {
		sa[i] = selectorArm{arm: a}
	}
	return &Selector{arms: sa}
}

// Select returns the index of the arm to pull using UCB1.
// Returns -1 if there are no arms.
func (s *Selector) Select() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.arms) == 0 {
		return -1
	}

	totalPulls := 0
	for _, a := range s.arms {
		totalPulls += a.pulls
	}

	// Pull each arm at least once.
	for i, a := range s.arms {
		if a.pulls == 0 {
			return i
		}
	}

	// UCB1 selection.
	bestIdx := 0
	bestScore := math.Inf(-1)
	logTotal := math.Log(float64(totalPulls))

	for i, a := range s.arms {
		avg := a.totalReward / float64(a.pulls)
		exploration := math.Sqrt(2 * logTotal / float64(a.pulls))
		score := avg + exploration
		if score > bestScore {
			bestScore = score
			bestIdx = i
		}
	}

	return bestIdx
}

// Update records a reward for the given arm index.
func (s *Selector) Update(armIdx int, reward float64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if armIdx < 0 || armIdx >= len(s.arms) {
		return
	}
	s.arms[armIdx].pulls++
	s.arms[armIdx].totalReward += reward
}

// GetArm returns the arm at the given index, or nil if out of range.
func (s *Selector) GetArm(idx int) *Arm {
	s.mu.Lock()
	defer s.mu.Unlock()

	if idx < 0 || idx >= len(s.arms) {
		return nil
	}
	a := s.arms[idx].arm
	return &a
}

// ArmCount returns the number of arms.
func (s *Selector) ArmCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.arms)
}

// SelectProvider returns the provider string of the selected arm.
// Returns empty string if no arms are configured.
func (s *Selector) SelectProvider() string {
	idx := s.Select()
	if arm := s.GetArm(idx); arm != nil {
		return arm.Provider
	}
	return ""
}

// SelectRandom returns a random arm index (for epsilon-greedy variants).
func (s *Selector) SelectRandom() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	if len(s.arms) == 0 {
		return -1
	}
	return rand.Intn(len(s.arms))
}
