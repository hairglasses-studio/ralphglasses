package session

import (
	"context"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/util"
)

// TransientErrorPatterns are error strings that indicate a retryable failure.
var TransientErrorPatterns = []string{
	"connection reset",
	"timeout",
	"rate limit",
	"429",
	"503",
	"502",
	"ECONNREFUSED",
	"ECONNRESET",
	"ETIMEDOUT",
	"overloaded",
	"temporary failure",
	"internal server error",
}

// AutoRecoveryConfig configures the auto-recovery behavior.
type AutoRecoveryConfig struct {
	MaxRetries     int           // max consecutive auto-restarts (default 3)
	CooldownPeriod time.Duration // minimum time between retries (default 30s)
	BackoffFactor  float64       // exponential backoff multiplier (default 2.0)
}

// DefaultAutoRecoveryConfig returns the default recovery configuration.
func DefaultAutoRecoveryConfig() AutoRecoveryConfig {
	return AutoRecoveryConfig{
		MaxRetries:     3,
		CooldownPeriod: 30 * time.Second,
		BackoffFactor:  2.0,
	}
}

// AutoRecovery manages automatic session restart for transient errors.
type AutoRecovery struct {
	config      AutoRecoveryConfig
	manager     *Manager
	decisions   *DecisionLog
	hitl        *HITLTracker
	retryState  map[string]*retryInfo // session ID → retry state
}

type retryInfo struct {
	count     int
	lastRetry time.Time
}

// NewAutoRecovery creates an auto-recovery handler.
func NewAutoRecovery(mgr *Manager, decisions *DecisionLog, hitl *HITLTracker, config AutoRecoveryConfig) *AutoRecovery {
	return &AutoRecovery{
		config:     config,
		manager:    mgr,
		decisions:  decisions,
		hitl:       hitl,
		retryState: make(map[string]*retryInfo),
	}
}

// HandleSessionError evaluates a failed session and potentially auto-restarts it.
// Returns the new session if auto-recovery was executed, nil otherwise.
func (ar *AutoRecovery) HandleSessionError(ctx context.Context, s *Session) *Session {
	s.Lock()
	sessionID := s.ID
	errMsg := s.Error
	exitReason := s.ExitReason
	provider := s.Provider
	repoPath := s.RepoPath
	repoName := s.RepoName
	prompt := s.Prompt
	model := s.Model
	budget := s.BudgetUSD
	spent := s.SpentUSD
	maxTurns := s.MaxTurns
	teamName := s.TeamName
	s.Unlock()

	errText := errMsg
	if errText == "" {
		errText = exitReason
	}

	// Check if error is transient
	if !isTransientError(errText) {
		util.Debug.Debugf("session %s error not transient: %s", sessionID, errText)
		return nil
	}

	// Check retry limits
	state, ok := ar.retryState[sessionID]
	if !ok {
		state = &retryInfo{}
		ar.retryState[sessionID] = state
	}

	if state.count >= ar.config.MaxRetries {
		util.Debug.Debugf("session %s exceeded max retries (%d)", sessionID, ar.config.MaxRetries)
		return nil
	}

	// Check cooldown
	cooldown := ar.config.CooldownPeriod
	for i := 0; i < state.count; i++ {
		cooldown = time.Duration(float64(cooldown) * ar.config.BackoffFactor)
	}
	if time.Since(state.lastRetry) < cooldown {
		return nil
	}

	// Propose the decision
	decision := AutonomousDecision{
		Category:      DecisionRestart,
		RequiredLevel: LevelAutoRecover,
		Rationale:     "Transient error detected: " + errText,
		Action:        "auto-restart session with same parameters",
		SessionID:     sessionID,
		RepoName:      repoName,
		Inputs: map[string]any{
			"error":       errText,
			"retry_count": state.count,
			"provider":    string(provider),
		},
	}

	if !ar.decisions.Propose(decision) {
		// Level too low — log but don't execute
		if ar.hitl != nil {
			ar.hitl.RecordAuto(MetricAutoRecovery, sessionID, repoName,
				"would have auto-restarted but autonomy level insufficient")
		}
		return nil
	}

	// Execute: relaunch with remaining budget
	remaining := budget - spent
	if remaining < 0 {
		remaining = 0
	}

	opts := LaunchOptions{
		Provider:     provider,
		RepoPath:     repoPath,
		Prompt:       prompt,
		Model:        model,
		MaxBudgetUSD: remaining,
		MaxTurns:     maxTurns,
		TeamName:     teamName,
	}

	newSess, err := ar.manager.Launch(ctx, opts)
	if err != nil {
		util.Debug.Debugf("auto-recovery launch failed: %v", err)
		ar.decisions.RecordOutcome(decision.ID, DecisionOutcome{
			EvaluatedAt: time.Now(),
			Success:     false,
			Details:     "relaunch failed: " + err.Error(),
		})
		return nil
	}

	state.count++
	state.lastRetry = time.Now()

	if ar.hitl != nil {
		ar.hitl.RecordAuto(MetricAutoRecovery, newSess.ID, repoName,
			"auto-restarted from "+sessionID)
	}

	ar.decisions.RecordOutcome(decision.ID, DecisionOutcome{
		EvaluatedAt: time.Now(),
		Success:     true,
		Details:     "relaunched as " + newSess.ID,
	})

	return newSess
}

// isTransientError checks if an error message matches known transient patterns.
func isTransientError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	for _, pattern := range TransientErrorPatterns {
		if strings.Contains(lower, strings.ToLower(pattern)) {
			return true
		}
	}
	return false
}

// ClearRetryState removes retry tracking for a session (e.g., after successful completion).
func (ar *AutoRecovery) ClearRetryState(sessionID string) {
	delete(ar.retryState, sessionID)
}
