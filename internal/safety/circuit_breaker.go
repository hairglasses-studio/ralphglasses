package safety

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// State represents the current state of a circuit breaker.
type State int

const (
	// StateClosed is the normal operating state. Calls pass through.
	StateClosed State = iota
	// StateOpen means the breaker has tripped. Calls are rejected immediately.
	StateOpen
	// StateHalfOpen is the recovery-testing state. A limited number of calls
	// are allowed through to probe whether the downstream has recovered.
	StateHalfOpen
)

// String returns a human-readable name for the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// ErrCircuitOpen is returned when Execute is called while the breaker is open.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Metrics holds cumulative counters for a circuit breaker's lifetime.
type Metrics struct {
	TotalCalls   atomic.Int64
	Successes    atomic.Int64
	Failures     atomic.Int64
	Rejections   atomic.Int64
	Transitions  atomic.Int64
}

// Snapshot is a point-in-time copy of Metrics, safe for serialization.
type Snapshot struct {
	TotalCalls  int64 `json:"total_calls"`
	Successes   int64 `json:"successes"`
	Failures    int64 `json:"failures"`
	Rejections  int64 `json:"rejections"`
	Transitions int64 `json:"transitions"`
}

// Config holds tunable parameters for a CircuitBreaker.
type Config struct {
	// FailureThreshold is the number of consecutive failures in the closed
	// state that will trip the breaker to open. Must be >= 1.
	FailureThreshold int

	// ResetTimeout is how long the breaker stays open before transitioning
	// to half-open for a recovery probe.
	ResetTimeout time.Duration

	// SuccessThreshold is the number of consecutive successes required in
	// the half-open state to close the breaker again. Must be >= 1.
	SuccessThreshold int
}

// DefaultConfig returns a reasonable default configuration.
func DefaultConfig() Config {
	return Config{
		FailureThreshold: 5,
		ResetTimeout:     30 * time.Second,
		SuccessThreshold: 2,
	}
}

// validate checks config invariants and returns an error if anything is wrong.
func (c Config) validate() error {
	if c.FailureThreshold < 1 {
		return fmt.Errorf("FailureThreshold must be >= 1, got %d", c.FailureThreshold)
	}
	if c.ResetTimeout <= 0 {
		return fmt.Errorf("ResetTimeout must be > 0, got %v", c.ResetTimeout)
	}
	if c.SuccessThreshold < 1 {
		return fmt.Errorf("SuccessThreshold must be >= 1, got %d", c.SuccessThreshold)
	}
	return nil
}

// CircuitBreaker wraps function calls with failure detection and automatic
// recovery probing. It moves through three states:
//
//	closed  -> (consecutive failures >= FailureThreshold) -> open
//	open    -> (ResetTimeout elapsed)                     -> half-open
//	half-open -> (consecutive successes >= SuccessThreshold) -> closed
//	half-open -> (any failure)                              -> open
type CircuitBreaker struct {
	mu sync.Mutex

	cfg   Config
	state State

	// consecutiveFailures tracks failures in closed state.
	consecutiveFailures int
	// consecutiveSuccesses tracks successes in half-open state.
	consecutiveSuccesses int

	// openDeadline is when the breaker should transition from open to half-open.
	openDeadline time.Time

	// now is a clock function, overridable for tests.
	now func() time.Time

	metrics Metrics
}

// New creates a CircuitBreaker with the given configuration.
// Returns an error if the configuration is invalid.
func New(cfg Config) (*CircuitBreaker, error) {
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("invalid circuit breaker config: %w", err)
	}
	return &CircuitBreaker{
		cfg:   cfg,
		state: StateClosed,
		now:   time.Now,
	}, nil
}

// MustNew is like New but panics on invalid config. Useful for package-level
// initialization where the config is known at compile time.
func MustNew(cfg Config) *CircuitBreaker {
	cb, err := New(cfg)
	if err != nil {
		panic(err)
	}
	return cb
}

// Execute runs fn if the circuit breaker allows it. If the breaker is open,
// Execute returns ErrCircuitOpen without calling fn. Otherwise fn is called
// and its error (or nil) drives the state machine forward.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}

	err := fn()

	cb.afterCall(err)
	return err
}

// beforeCall checks whether the call should be allowed and updates state
// if the reset timeout has elapsed. Returns ErrCircuitOpen when rejected.
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.metrics.TotalCalls.Add(1)

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		if cb.now().After(cb.openDeadline) {
			cb.transitionTo(StateHalfOpen)
			return nil
		}
		cb.metrics.Rejections.Add(1)
		return ErrCircuitOpen

	case StateHalfOpen:
		// Allow calls through during half-open -- we need successes to close.
		return nil
	}

	return nil
}

// afterCall records the outcome and drives state transitions.
func (cb *CircuitBreaker) afterCall(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err == nil {
		cb.onSuccess()
	} else {
		cb.onFailure()
	}
}

// onSuccess handles a successful call. Caller must hold cb.mu.
func (cb *CircuitBreaker) onSuccess() {
	cb.metrics.Successes.Add(1)

	switch cb.state {
	case StateClosed:
		cb.consecutiveFailures = 0

	case StateHalfOpen:
		cb.consecutiveSuccesses++
		if cb.consecutiveSuccesses >= cb.cfg.SuccessThreshold {
			cb.transitionTo(StateClosed)
		}
	}
}

// onFailure handles a failed call. Caller must hold cb.mu.
func (cb *CircuitBreaker) onFailure() {
	cb.metrics.Failures.Add(1)

	switch cb.state {
	case StateClosed:
		cb.consecutiveFailures++
		if cb.consecutiveFailures >= cb.cfg.FailureThreshold {
			cb.transitionTo(StateOpen)
		}

	case StateHalfOpen:
		// Any failure in half-open immediately re-opens.
		cb.transitionTo(StateOpen)
	}
}

// transitionTo moves the breaker to the target state, resetting counters
// as appropriate. Caller must hold cb.mu.
func (cb *CircuitBreaker) transitionTo(target State) {
	if cb.state == target {
		return
	}
	cb.metrics.Transitions.Add(1)
	cb.state = target

	switch target {
	case StateClosed:
		cb.consecutiveFailures = 0
		cb.consecutiveSuccesses = 0
	case StateOpen:
		cb.openDeadline = cb.now().Add(cb.cfg.ResetTimeout)
		cb.consecutiveSuccesses = 0
	case StateHalfOpen:
		cb.consecutiveSuccesses = 0
	}
}

// State returns the current breaker state. If the breaker is open but the
// reset timeout has elapsed, it reports StateHalfOpen.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateOpen && cb.now().After(cb.openDeadline) {
		return StateHalfOpen
	}
	return cb.state
}

// Reset forces the breaker back to closed, clearing all counters.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state != StateClosed {
		cb.metrics.Transitions.Add(1)
	}
	cb.state = StateClosed
	cb.consecutiveFailures = 0
	cb.consecutiveSuccesses = 0
	cb.openDeadline = time.Time{}
}

// Snapshot returns a point-in-time copy of the breaker's metrics.
func (cb *CircuitBreaker) Snapshot() Snapshot {
	return Snapshot{
		TotalCalls:  cb.metrics.TotalCalls.Load(),
		Successes:   cb.metrics.Successes.Load(),
		Failures:    cb.metrics.Failures.Load(),
		Rejections:  cb.metrics.Rejections.Load(),
		Transitions: cb.metrics.Transitions.Load(),
	}
}
