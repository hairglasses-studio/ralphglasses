package session

import (
	"context"
	"time"
)

// SetAutoOptimizer attaches the self-improvement engine (Level 2+).
// When set, Launch will consult FeedbackAnalyzer for provider and budget
// suggestions, and session completion will feed back into profiles.
func (m *Manager) SetAutoOptimizer(opt *AutoOptimizer) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.optimizer = opt
}

// SetReflexionStore attaches the reflexion loop subsystem.
func (m *Manager) SetReflexionStore(rs *ReflexionStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reflexion = rs
}

// SetEpisodicMemory attaches the episodic memory subsystem.
func (m *Manager) SetEpisodicMemory(em *EpisodicMemory) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.episodic = em
}

// SetCascadeRouter attaches the cascade routing subsystem.
// If bandit hooks are already configured, they are forwarded to the new router.
func (m *Manager) SetCascadeRouter(cr *CascadeRouter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cascade = cr
	if cr != nil && m.banditSelect != nil {
		cr.SetBanditHooks(m.banditSelect, m.banditUpdate)
	}
}

// SetCurriculumSorter attaches the curriculum learning subsystem.
func (m *Manager) SetCurriculumSorter(cs *CurriculumSorter) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.curriculum = cs
}

// HasReflexion returns true if a ReflexionStore is already attached.
func (m *Manager) HasReflexion() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reflexion != nil
}

// HasEpisodicMemory returns true if an EpisodicMemory is already attached.
func (m *Manager) HasEpisodicMemory() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.episodic != nil
}

// GetEpisodicMemory returns the attached EpisodicMemory, or nil.
func (m *Manager) GetEpisodicMemory() *EpisodicMemory {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.episodic
}

// SetDepthEstimator attaches the adaptive iteration depth subsystem.
func (m *Manager) SetDepthEstimator(de *DepthEstimator) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.depthEstimator = de
}

// DepthEstimator returns the attached DepthEstimator, or nil.
func (m *Manager) DepthEstimator() *DepthEstimator {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.depthEstimator
}

// HasCascadeRouter returns true if a CascadeRouter is already attached.
func (m *Manager) HasCascadeRouter() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cascade != nil
}

// HasCurriculumSorter returns true if a CurriculumSorter is already attached.
func (m *Manager) HasCurriculumSorter() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.curriculum != nil
}

// SetBanditHooks attaches bandit-based provider selection functions to the manager
// and forwards them to the cascade router if one is attached.
func (m *Manager) SetBanditHooks(selectFn func() (string, string), updateFn func(string, float64)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.banditSelect = selectFn
	m.banditUpdate = updateFn
	if m.cascade != nil {
		m.cascade.SetBanditHooks(selectFn, updateFn)
	}
}

// HasBandit returns true if bandit hooks have been configured.
func (m *Manager) HasBandit() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.banditSelect != nil
}

// GetCascadeRouter returns the attached CascadeRouter, or nil.
func (m *Manager) GetCascadeRouter() *CascadeRouter {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cascade
}

// SetBlackboard attaches the shared blackboard subsystem.
func (m *Manager) SetBlackboard(bb *Blackboard) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blackboard = bb
}

// HasBlackboard returns true if a Blackboard is already attached.
func (m *Manager) HasBlackboard() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.blackboard != nil
}

// GetBlackboard returns the attached Blackboard, or nil.
func (m *Manager) GetBlackboard() *Blackboard {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.blackboard
}

// SetCostPredictor attaches the cost prediction subsystem.
func (m *Manager) SetCostPredictor(cp *CostPredictor) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.costPredictor = cp
}

// HasCostPredictor returns true if a CostPredictor is already attached.
func (m *Manager) HasCostPredictor() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.costPredictor != nil
}

// GetCostPredictor returns the attached CostPredictor, or nil.
func (m *Manager) GetCostPredictor() *CostPredictor {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.costPredictor
}

// GetReflexionStore returns the attached ReflexionStore, or nil.
func (m *Manager) GetReflexionStore() *ReflexionStore {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.reflexion
}

// SetHooksForTesting overrides session launch/wait behavior. Intended for tests.
func (m *Manager) SetHooksForTesting(
	launch func(context.Context, LaunchOptions) (*Session, error),
	wait func(context.Context, *Session) error,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.launchSession = launch
	m.waitSession = wait
}

// SetHealthCheckForTesting overrides the provider health check function.
// Intended for tests that need to control health check results.
func (m *Manager) SetHealthCheckForTesting(fn func(Provider) ProviderHealth) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthCheck = fn
}

// checkHealth returns the health of a provider, using the injectable function
// if set, otherwise falling back to CheckProviderHealth.
func (m *Manager) checkHealth(p Provider) ProviderHealth {
	m.mu.RLock()
	fn := m.healthCheck
	m.mu.RUnlock()
	if fn != nil {
		return fn(p)
	}
	return CheckProviderHealth(p)
}

// AddSessionForTesting inserts a pre-built session into the manager. Intended for tests.
func (m *Manager) AddSessionForTesting(s *Session) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.ID] = s
}

// AddTeamForTesting inserts a pre-built team into the manager. Intended for tests.
func (m *Manager) AddTeamForTesting(t *TeamStatus) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.teams[t.Name] = t
}

// HITLSnapshot returns the current HITL score over a 24h window.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) HITLSnapshot() *HITLSnapshot {
	m.mu.RLock()
	opt := m.optimizer
	m.mu.RUnlock()
	if opt == nil || opt.hitl == nil {
		return nil
	}
	snap := opt.hitl.CurrentScore(24 * time.Hour)
	return &snap
}

// FeedbackProfiles returns all prompt profiles from the feedback analyzer.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) FeedbackProfiles() []PromptProfile {
	m.mu.RLock()
	opt := m.optimizer
	m.mu.RUnlock()
	if opt == nil || opt.feedback == nil {
		return nil
	}
	return opt.feedback.AllPromptProfiles()
}

// ProviderProfiles returns all provider profiles from the feedback analyzer.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) ProviderProfiles() []ProviderProfile {
	m.mu.RLock()
	opt := m.optimizer
	m.mu.RUnlock()
	if opt == nil || opt.feedback == nil {
		return nil
	}
	return opt.feedback.AllProviderProfiles()
}

// RecentDecisions returns the last n autonomous decisions.
// Returns nil if no AutoOptimizer is configured.
func (m *Manager) RecentDecisions(n int) []AutonomousDecision {
	m.mu.RLock()
	opt := m.optimizer
	m.mu.RUnlock()
	if opt == nil || opt.decisions == nil {
		return nil
	}
	return opt.decisions.Recent(n)
}

// GetAutonomyLevel returns the current autonomy level.
// Returns LevelObserve if no AutoOptimizer is configured.
func (m *Manager) GetAutonomyLevel() AutonomyLevel {
	m.mu.RLock()
	opt := m.optimizer
	m.mu.RUnlock()
	if opt == nil || opt.decisions == nil {
		return LevelObserve
	}
	return opt.decisions.Level()
}
