package process

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CircuitState represents the state of the circuit breaker.
type CircuitState string

const (
	CircuitClosed   CircuitState = "closed"    // healthy, allow spawns
	CircuitOpen     CircuitState = "open"       // broken, refuse spawns
	CircuitHalfOpen CircuitState = "half-open"  // recovering, allow one probe
)

const (
	defaultMaxFailures   = 3
	defaultResetTimeout  = 5 * time.Minute
	defaultFailureWindow = 60 * time.Second
	circuitStateFile   = "circuit-state.json"
)

var coordDir = filepath.Join(os.TempDir(), "ralphglasses-coordination")

// CircuitBreaker prevents cascading failures by tracking spawn failures
// and refusing new spawns when the failure rate exceeds a threshold.
type CircuitBreaker struct {
	mu            sync.Mutex
	state         CircuitState
	failures      int
	lastFailure   time.Time
	openUntil     time.Time
	maxFailures   int
	resetTimeout  time.Duration
	failureWindow time.Duration
}

// circuitStateJSON is the on-disk representation of circuit breaker state.
type circuitStateJSON struct {
	State       CircuitState `json:"state"`
	Failures    int          `json:"failures"`
	LastFailure *time.Time   `json:"last_failure,omitempty"`
	OpenUntil   *time.Time   `json:"open_until,omitempty"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// NewCircuitBreaker creates a circuit breaker with the given thresholds.
// Zero values fall back to defaults (3 failures, 5min reset, 60s window).
func NewCircuitBreaker(maxFailures int, resetTimeout, failureWindow time.Duration) *CircuitBreaker {
	if maxFailures <= 0 {
		maxFailures = defaultMaxFailures
	}
	if resetTimeout <= 0 {
		resetTimeout = defaultResetTimeout
	}
	if failureWindow <= 0 {
		failureWindow = defaultFailureWindow
	}
	return &CircuitBreaker{
		state:         CircuitClosed,
		maxFailures:   maxFailures,
		resetTimeout:  resetTimeout,
		failureWindow: failureWindow,
	}
}

// AllowSpawn returns true if a new session spawn is permitted.
// In the half-open state, exactly one probe is allowed before re-evaluating.
func (cb *CircuitBreaker) AllowSpawn() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		if time.Now().After(cb.openUntil) {
			cb.state = CircuitHalfOpen
			return true
		}
		return false
	case CircuitHalfOpen:
		// Only one probe allowed; the next RecordSuccess or RecordFailure
		// will transition state. Allow it.
		return true
	default:
		return false
	}
}

// RecordSuccess resets the circuit breaker to the closed state.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = CircuitClosed
	cb.failures = 0
	cb.lastFailure = time.Time{}
	cb.openUntil = time.Time{}
}

// RecordFailure records a spawn failure. If the failure count within the
// window exceeds maxFailures, the circuit opens for resetTimeout.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// If the last failure is outside the window, reset the counter.
	if !cb.lastFailure.IsZero() && now.Sub(cb.lastFailure) > cb.failureWindow {
		cb.failures = 0
	}

	cb.failures++
	cb.lastFailure = now

	if cb.failures >= cb.maxFailures {
		cb.state = CircuitOpen
		cb.openUntil = now.Add(cb.resetTimeout)
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Check if an open circuit should transition to half-open.
	if cb.state == CircuitOpen && time.Now().After(cb.openUntil) {
		cb.state = CircuitHalfOpen
	}
	return cb.state
}

// WriteStateFile persists the circuit breaker state to the coordination directory.
func (cb *CircuitBreaker) WriteStateFile() error {
	cb.mu.Lock()
	s := circuitStateJSON{
		State:     cb.state,
		Failures:  cb.failures,
		UpdatedAt: time.Now(),
	}
	if !cb.lastFailure.IsZero() {
		t := cb.lastFailure
		s.LastFailure = &t
	}
	if !cb.openUntil.IsZero() {
		t := cb.openUntil
		s.OpenUntil = &t
	}
	cb.mu.Unlock()

	if err := os.MkdirAll(coordDir, 0755); err != nil {
		return fmt.Errorf("create coordination dir: %w", err)
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal circuit state: %w", err)
	}

	target := filepath.Join(coordDir, circuitStateFile)
	tmp, err := os.CreateTemp(coordDir, circuitStateFile+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename state file: %w", err)
	}
	return nil
}
