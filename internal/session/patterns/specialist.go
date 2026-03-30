package patterns

import (
	"sort"
	"sync"
)

// Specialist represents a session with declared skill tags and capacity.
type Specialist struct {
	SessionID string   `json:"session_id"`
	SkillTags []string `json:"skill_tags"` // e.g., "frontend", "backend", "testing"
	Capacity  int      `json:"capacity"`   // max concurrent tasks (0 = unlimited)
	Active    int      `json:"active"`     // current active task count
}

// SkillMatch holds a routing result with a match score.
type SkillMatch struct {
	Specialist Specialist `json:"specialist"`
	Score      float64    `json:"score"` // 0.0-1.0 match quality
}

// SkillRouter routes tasks to specialized sessions based on skill tag matching.
type SkillRouter struct {
	mu          sync.RWMutex
	specialists []Specialist
}

// NewSkillRouter creates a router with the given specialists.
func NewSkillRouter(specialists []Specialist) *SkillRouter {
	s := make([]Specialist, len(specialists))
	copy(s, specialists)
	return &SkillRouter{specialists: s}
}

// Register adds a specialist to the router.
func (sr *SkillRouter) Register(s Specialist) {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	// Replace if session ID already registered.
	for i := range sr.specialists {
		if sr.specialists[i].SessionID == s.SessionID {
			sr.specialists[i] = s
			return
		}
	}
	sr.specialists = append(sr.specialists, s)
}

// Unregister removes a specialist by session ID.
func (sr *SkillRouter) Unregister(sessionID string) bool {
	sr.mu.Lock()
	defer sr.mu.Unlock()
	for i := range sr.specialists {
		if sr.specialists[i].SessionID == sessionID {
			sr.specialists = append(sr.specialists[:i], sr.specialists[i+1:]...)
			return true
		}
	}
	return false
}

// Route finds the best specialist for a task given required skill tags.
// Returns all matches sorted by score descending. Returns ErrNoMatchingSpecialist
// if no specialist has any overlapping skills.
func (sr *SkillRouter) Route(requiredTags []string) ([]SkillMatch, error) {
	sr.mu.RLock()
	defer sr.mu.RUnlock()

	if len(requiredTags) == 0 {
		// No skill requirements: return all with capacity, scored 1.0.
		var matches []SkillMatch
		for _, s := range sr.specialists {
			if s.Capacity > 0 && s.Active >= s.Capacity {
				continue
			}
			matches = append(matches, SkillMatch{Specialist: s, Score: 1.0})
		}
		if len(matches) == 0 {
			return nil, ErrNoMatchingSpecialist
		}
		return matches, nil
	}

	reqSet := make(map[string]bool, len(requiredTags))
	for _, t := range requiredTags {
		reqSet[t] = true
	}

	var matches []SkillMatch
	for _, s := range sr.specialists {
		// Skip at-capacity specialists.
		if s.Capacity > 0 && s.Active >= s.Capacity {
			continue
		}
		overlap := 0
		for _, tag := range s.SkillTags {
			if reqSet[tag] {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		// Score = fraction of required tags matched.
		score := float64(overlap) / float64(len(requiredTags))
		matches = append(matches, SkillMatch{Specialist: s, Score: score})
	}

	if len(matches) == 0 {
		return nil, ErrNoMatchingSpecialist
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		// Prefer less-loaded specialists as tiebreaker.
		return matches[i].Specialist.Active < matches[j].Specialist.Active
	})

	return matches, nil
}

// Best returns the single best matching specialist for the given tags.
func (sr *SkillRouter) Best(requiredTags []string) (*SkillMatch, error) {
	matches, err := sr.Route(requiredTags)
	if err != nil {
		return nil, err
	}
	return &matches[0], nil
}

// Specialists returns a snapshot of all registered specialists.
func (sr *SkillRouter) Specialists() []Specialist {
	sr.mu.RLock()
	defer sr.mu.RUnlock()
	out := make([]Specialist, len(sr.specialists))
	copy(out, sr.specialists)
	return out
}

// SpecialistPattern combines a SkillRouter with shared memory for a complete
// specialist-based orchestration.
type SpecialistPattern struct {
	Router *SkillRouter
	Memory *SharedMemory
}

// NewSpecialistPattern creates a specialist pattern with the given specialists.
func NewSpecialistPattern(specialists []Specialist, mem *SharedMemory) *SpecialistPattern {
	if mem == nil {
		mem = NewSharedMemory()
	}
	return &SpecialistPattern{
		Router: NewSkillRouter(specialists),
		Memory: mem,
	}
}

// RouteTask finds the best specialist for a TaskAssignment and returns
// the match along with an Envelope ready to send.
func (sp *SpecialistPattern) RouteTask(fromID string, task TaskAssignment) (*SkillMatch, *Envelope, error) {
	match, err := sp.Router.Best(task.SkillTags)
	if err != nil {
		return nil, nil, err
	}
	env, err := MarshalEnvelope(task.TaskID, fromID, match.Specialist.SessionID, task)
	if err != nil {
		return nil, nil, err
	}
	return match, env, nil
}
