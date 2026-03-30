package gateway

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// CircuitState represents the operational state of a circuit breaker.
type CircuitState int

const (
	// StateClosed is the normal operating state: requests flow through.
	StateClosed CircuitState = iota
	// StateOpen means the circuit is tripped; requests are rejected immediately.
	StateOpen
	// StateHalfOpen is a probe state: a limited number of requests are allowed
	// to test whether the downstream has recovered.
	StateHalfOpen
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
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// ErrCircuitOpen is returned when a call is rejected because the circuit is open.
var ErrCircuitOpen = errors.New("circuit breaker open")

// CircuitBreakerConfig holds tunable parameters for a CircuitBreaker.
type CircuitBreakerConfig struct {
	// FailureThreshold is the number of consecutive failures required to open
	// the circuit. Default: 5.
	FailureThreshold int
	// Timeout is how long the circuit stays open before entering half-open.
	// Default: 60s.
	Timeout time.Duration
	// HalfOpenMaxRequests is the maximum number of probe requests allowed in
	// half-open state before a decision is made. Default: 1.
	HalfOpenMaxRequests int
}

func (c *CircuitBreakerConfig) applyDefaults() {
	if c.FailureThreshold <= 0 {
		c.FailureThreshold = 5
	}
	if c.Timeout <= 0 {
		c.Timeout = 60 * time.Second
	}
	if c.HalfOpenMaxRequests <= 0 {
		c.HalfOpenMaxRequests = 1
	}
}

// CircuitBreakerStatus is a snapshot of circuit breaker state for observability.
type CircuitBreakerStatus struct {
	State              CircuitState
	Failures           int
	Successes          int
	HalfOpenRequests   int
	LastStateChange    time.Time
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	mu     sync.Mutex
	cfg    CircuitBreakerConfig
	state  CircuitState

	failures         int
	successes        int
	halfOpenRequests int
	lastStateChange  time.Time
	openedAt         time.Time
}

// NewCircuitBreaker creates a CircuitBreaker with the given configuration.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	cfg.applyDefaults()
	return &CircuitBreaker{
		cfg:             cfg,
		state:           StateClosed,
		lastStateChange: time.Now(),
	}
}

// Allow returns nil if the call should proceed, or ErrCircuitOpen if it should
// be rejected. Callers MUST call RecordSuccess or RecordFailure when done.
func (cb *CircuitBreaker) Allow() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		if time.Since(cb.openedAt) >= cb.cfg.Timeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenRequests = 1
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		if cb.halfOpenRequests < cb.cfg.HalfOpenMaxRequests {
			cb.halfOpenRequests++
			return nil
		}
		return ErrCircuitOpen
	}
	return nil
}

// RecordSuccess records a successful call result.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.successes++
	cb.failures = 0

	if cb.state == StateHalfOpen {
		cb.setState(StateClosed)
		cb.halfOpenRequests = 0
	}
}

// RecordFailure records a failed call result.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failures++
	cb.successes = 0

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.cfg.FailureThreshold {
			cb.setState(StateOpen)
			cb.openedAt = time.Now()
		}
	case StateHalfOpen:
		// Any failure in half-open re-opens the circuit.
		cb.setState(StateOpen)
		cb.openedAt = time.Now()
		cb.halfOpenRequests = 0
	}
}

// Status returns an observability snapshot of the circuit breaker.
func (cb *CircuitBreaker) Status() CircuitBreakerStatus {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return CircuitBreakerStatus{
		State:            cb.state,
		Failures:         cb.failures,
		Successes:        cb.successes,
		HalfOpenRequests: cb.halfOpenRequests,
		LastStateChange:  cb.lastStateChange,
	}
}

// Reset forces the circuit breaker back to the closed state and clears all
// counters. Useful for manual recovery or testing.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.successes = 0
	cb.halfOpenRequests = 0
	cb.setState(StateClosed)
}

func (cb *CircuitBreaker) setState(s CircuitState) {
	cb.state = s
	cb.lastStateChange = time.Now()
}

// ProviderCircuitBreakers manages per-provider circuit breakers.
type ProviderCircuitBreakers struct {
	mu       sync.RWMutex
	breakers map[string]*CircuitBreaker
	cfg      CircuitBreakerConfig
}

// NewProviderCircuitBreakers creates circuit breakers for Claude, Gemini, and
// OpenAI with a shared configuration. Additional providers can be added via
// Register.
func NewProviderCircuitBreakers(cfg CircuitBreakerConfig) *ProviderCircuitBreakers {
	pcb := &ProviderCircuitBreakers{
		breakers: make(map[string]*CircuitBreaker),
		cfg:      cfg,
	}
	for _, p := range []string{"claude", "gemini", "openai"} {
		pcb.breakers[p] = NewCircuitBreaker(cfg)
	}
	return pcb
}

// Register adds a new named circuit breaker (or replaces an existing one).
func (p *ProviderCircuitBreakers) Register(name string, cfg CircuitBreakerConfig) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.breakers[name] = NewCircuitBreaker(cfg)
}

// Get returns the circuit breaker for the named provider.
// If the provider is unknown a new breaker with the default config is created.
func (p *ProviderCircuitBreakers) Get(name string) *CircuitBreaker {
	p.mu.RLock()
	cb, ok := p.breakers[name]
	p.mu.RUnlock()
	if ok {
		return cb
	}
	// Lazily create for unknown providers.
	p.mu.Lock()
	defer p.mu.Unlock()
	if cb, ok = p.breakers[name]; ok {
		return cb
	}
	cb = NewCircuitBreaker(p.cfg)
	p.breakers[name] = cb
	return cb
}

// Status returns a map of provider name -> CircuitBreakerStatus.
func (p *ProviderCircuitBreakers) Status() map[string]CircuitBreakerStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	out := make(map[string]CircuitBreakerStatus, len(p.breakers))
	for name, cb := range p.breakers {
		out[name] = cb.Status()
	}
	return out
}
