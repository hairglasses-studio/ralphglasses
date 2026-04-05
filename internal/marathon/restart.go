package marathon

import (
	"math"
	"sync"
	"time"
)

// RestartOption configures a RestartPolicy.
type RestartOption func(*RestartPolicy)

// RestartStats holds restart accounting information.
type RestartStats struct {
	RestartCount int           `json:"restart_count"`
	MaxRestarts  int           `json:"max_restarts"`
	NextBackoff  time.Duration `json:"next_backoff"`
	LastRestart  time.Time     `json:"last_restart"`
}

// RestartPolicy determines whether a marathon session should be restarted
// after failure. It supports configurable max restarts, exponential backoff,
// minimum elapsed time, and exit-code-based conditions.
type RestartPolicy struct {
	mu sync.Mutex

	maxRestarts   int
	baseBackoff   time.Duration
	maxBackoff    time.Duration
	backoffFactor float64
	minElapsed    time.Duration
	restartableFn func(exitCode int) bool
	restartCount  int
	lastRestartAt time.Time
}

// NewRestartPolicy creates a RestartPolicy with sensible defaults.
// Use RestartOption functions to customise behaviour.
func NewRestartPolicy(opts ...RestartOption) *RestartPolicy {
	rp := &RestartPolicy{
		maxRestarts:   5,
		baseBackoff:   1 * time.Second,
		maxBackoff:    5 * time.Minute,
		backoffFactor: 2.0,
	}
	for _, opt := range opts {
		opt(rp)
	}
	return rp
}

// WithMaxRestarts sets the maximum number of restarts before giving up.
func WithMaxRestarts(n int) RestartOption {
	return func(rp *RestartPolicy) { rp.maxRestarts = n }
}

// WithBaseBackoff sets the initial backoff duration.
func WithBaseBackoff(d time.Duration) RestartOption {
	return func(rp *RestartPolicy) { rp.baseBackoff = d }
}

// WithMaxBackoff sets the ceiling for exponential backoff.
func WithMaxBackoff(d time.Duration) RestartOption {
	return func(rp *RestartPolicy) { rp.maxBackoff = d }
}

// WithBackoffFactor sets the exponential multiplier (default 2.0).
func WithBackoffFactor(f float64) RestartOption {
	return func(rp *RestartPolicy) { rp.backoffFactor = f }
}

// WithMinElapsed sets the minimum session runtime to qualify for restart.
// Sessions that exit faster than this are considered immediate crashes and
// still count toward the restart limit, but this can be used as an
// additional guard in ShouldRestart.
func WithMinElapsed(d time.Duration) RestartOption {
	return func(rp *RestartPolicy) { rp.minElapsed = d }
}

// WithRestartableExitCodes limits restarts to specific exit codes.
// The provided function returns true for exit codes that warrant a restart.
func WithRestartableExitCodes(fn func(exitCode int) bool) RestartOption {
	return func(rp *RestartPolicy) { rp.restartableFn = fn }
}

// ShouldRestart returns true if the policy allows another restart given the
// exit code and how long the session ran.
func (rp *RestartPolicy) ShouldRestart(exitCode int, elapsed time.Duration) bool {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	// Exhausted restart budget.
	if rp.restartCount >= rp.maxRestarts {
		return false
	}

	// Exit code 0 means clean exit — no restart needed.
	if exitCode == 0 {
		return false
	}

	// Check minimum elapsed time if configured.
	if rp.minElapsed > 0 && elapsed < rp.minElapsed {
		return false
	}

	// Check exit-code filter if configured.
	if rp.restartableFn != nil && !rp.restartableFn(exitCode) {
		return false
	}

	return true
}

// RecordRestart increments the restart counter and records the timestamp.
func (rp *RestartPolicy) RecordRestart() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.restartCount++
	rp.lastRestartAt = time.Now()
}

// ResetCount resets the restart counter back to zero (e.g. after a
// successful cycle).
func (rp *RestartPolicy) ResetCount() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.restartCount = 0
	rp.lastRestartAt = time.Time{}
}

// Backoff returns the current backoff duration based on the restart count.
// It uses exponential backoff: base * factor^(restartCount-1), capped at maxBackoff.
// Returns 0 if no restarts have been recorded.
func (rp *RestartPolicy) Backoff() time.Duration {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.backoffLocked()
}

func (rp *RestartPolicy) backoffLocked() time.Duration {
	if rp.restartCount == 0 {
		return rp.baseBackoff
	}
	exp := math.Pow(rp.backoffFactor, float64(rp.restartCount-1))
	if math.IsInf(exp, 1) || math.IsNaN(exp) {
		return rp.maxBackoff
	}
	d := time.Duration(float64(rp.baseBackoff) * exp)
	if d > rp.maxBackoff || d < 0 {
		return rp.maxBackoff
	}
	return d
}

// Stats returns a snapshot of restart accounting.
func (rp *RestartPolicy) Stats() RestartStats {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return RestartStats{
		RestartCount: rp.restartCount,
		MaxRestarts:  rp.maxRestarts,
		NextBackoff:  rp.backoffLocked(),
		LastRestart:  rp.lastRestartAt,
	}
}
