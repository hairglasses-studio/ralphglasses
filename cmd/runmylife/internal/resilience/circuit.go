// Package resilience provides fault-tolerance primitives for external API calls.
package resilience

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrCircuitOpen is returned when a circuit breaker is in the Open state.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	StateClosed   CircuitState = iota // normal operation
	StateOpen                         // failing, rejecting calls
	StateHalfOpen                     // testing if recovery is possible
)

func (s CircuitState) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// CircuitBreakerConfig configures a circuit breaker.
type CircuitBreakerConfig struct {
	FailureThreshold int           // failures before opening (default 5)
	SuccessThreshold int           // successes in half-open before closing (default 2)
	Timeout          time.Duration // how long to stay open before half-open (default 60s)
	HalfOpenMaxCalls int           // max concurrent calls in half-open (default 1)
}

// DefaultConfig returns sensible defaults for a circuit breaker.
func DefaultConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          60 * time.Second,
		HalfOpenMaxCalls: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu              sync.Mutex
	name            string
	config          CircuitBreakerConfig
	state           CircuitState
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCalls   int
}

// NewCircuitBreaker creates a new circuit breaker with the given config.
func NewCircuitBreaker(name string, cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.FailureThreshold <= 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold <= 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 60 * time.Second
	}
	if cfg.HalfOpenMaxCalls <= 0 {
		cfg.HalfOpenMaxCalls = 1
	}
	return &CircuitBreaker{
		name:   name,
		config: cfg,
		state:  StateClosed,
	}
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.checkTimeout()
	return cb.state
}

// Execute runs fn if the circuit allows it, tracking success/failure.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	cb.mu.Lock()
	cb.checkTimeout()

	switch cb.state {
	case StateOpen:
		cb.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrCircuitOpen, cb.name)

	case StateHalfOpen:
		if cb.halfOpenCalls >= cb.config.HalfOpenMaxCalls {
			cb.mu.Unlock()
			return fmt.Errorf("%w: %s (half-open limit reached)", ErrCircuitOpen, cb.name)
		}
		cb.halfOpenCalls++
		cb.mu.Unlock()

	case StateClosed:
		cb.mu.Unlock()
	}

	err := fn()

	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.recordFailure()
	} else {
		cb.recordSuccess()
	}
	return err
}

func (cb *CircuitBreaker) checkTimeout() {
	if cb.state == StateOpen && time.Since(cb.lastFailureTime) >= cb.config.Timeout {
		cb.state = StateHalfOpen
		cb.halfOpenCalls = 0
		cb.successes = 0
	}
}

func (cb *CircuitBreaker) recordFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.FailureThreshold {
			cb.state = StateOpen
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.halfOpenCalls = 0
	}
}

func (cb *CircuitBreaker) recordSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		if cb.successes >= cb.config.SuccessThreshold {
			cb.state = StateClosed
			cb.failures = 0
			cb.successes = 0
			cb.halfOpenCalls = 0
		}
	}
}

// CircuitBreakerRegistry manages named circuit breakers.
type CircuitBreakerRegistry struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	configs  map[string]CircuitBreakerConfig
}

// NewRegistry creates a new circuit breaker registry.
func NewRegistry() *CircuitBreakerRegistry {
	return &CircuitBreakerRegistry{
		breakers: make(map[string]*CircuitBreaker),
		configs:  make(map[string]CircuitBreakerConfig),
	}
}

// Configure sets the config for a named circuit breaker group.
func (r *CircuitBreakerRegistry) Configure(name string, cfg CircuitBreakerConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.configs[name] = cfg
}

// Get returns the circuit breaker for the given name, creating it if needed.
func (r *CircuitBreakerRegistry) Get(name string) *CircuitBreaker {
	r.mu.RLock()
	if cb, ok := r.breakers[name]; ok {
		r.mu.RUnlock()
		return cb
	}
	r.mu.RUnlock()

	r.mu.Lock()
	defer r.mu.Unlock()

	if cb, ok := r.breakers[name]; ok {
		return cb
	}

	cfg, ok := r.configs[name]
	if !ok {
		cfg = DefaultConfig()
	}
	cb := NewCircuitBreaker(name, cfg)
	r.breakers[name] = cb
	return cb
}

// Execute runs fn through the named circuit breaker.
func (r *CircuitBreakerRegistry) Execute(name string, fn func() error) error {
	return r.Get(name).Execute(fn)
}
