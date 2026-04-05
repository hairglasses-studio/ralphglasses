package session

import (
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"time"
)

var (
	// ErrRetryBudgetExhausted indicates no retries remain.
	ErrRetryBudgetExhausted = errors.New("retry budget exhausted")
)

const (
	// DefaultMaxRetries is the default per-session retry budget.
	DefaultMaxRetries = 5

	// DefaultRetryBaseDelay is the initial backoff delay.
	DefaultRetryBaseDelay = 500 * time.Millisecond

	// DefaultRetryMaxDelay is the maximum backoff delay.
	DefaultRetryMaxDelay = 30 * time.Second

	// DefaultRetryFactor is the exponential multiplier.
	DefaultRetryFactor = 2.0
)

// RetryBudgetConfig configures a RetryBudget.
type RetryBudgetConfig struct {
	// MaxRetries is the maximum number of retries (not counting the initial attempt).
	MaxRetries int
	// BaseDelay is the initial delay before the first retry.
	BaseDelay time.Duration
	// MaxDelay is the upper bound on any single delay.
	MaxDelay time.Duration
	// Factor is the exponential multiplier per attempt.
	Factor float64
}

// DefaultRetryBudgetConfig returns the default configuration.
func DefaultRetryBudgetConfig() RetryBudgetConfig {
	return RetryBudgetConfig{
		MaxRetries: DefaultMaxRetries,
		BaseDelay:  DefaultRetryBaseDelay,
		MaxDelay:   DefaultRetryMaxDelay,
		Factor:     DefaultRetryFactor,
	}
}

// RetryBudget tracks per-session retry usage with exponential backoff and jitter.
// Thread-safe via sync.Mutex.
type RetryBudget struct {
	mu sync.Mutex

	config    RetryBudgetConfig
	sessionID string

	totalAttempts int
	successes     int
	failures      int
	retriesUsed   int
}

// NewRetryBudget creates a RetryBudget for the given session with the specified config.
func NewRetryBudget(sessionID string, config RetryBudgetConfig) *RetryBudget {
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.BaseDelay <= 0 {
		config.BaseDelay = DefaultRetryBaseDelay
	}
	if config.MaxDelay <= 0 {
		config.MaxDelay = DefaultRetryMaxDelay
	}
	if config.Factor <= 0 {
		config.Factor = DefaultRetryFactor
	}
	return &RetryBudget{
		config:    config,
		sessionID: sessionID,
	}
}

// NewDefaultRetryBudget creates a RetryBudget with the default configuration.
func NewDefaultRetryBudget(sessionID string) *RetryBudget {
	return NewRetryBudget(sessionID, DefaultRetryBudgetConfig())
}

// RetryBudgetStats holds a snapshot of the retry budget state.
type RetryBudgetStats struct {
	SessionID     string  `json:"session_id"`
	MaxRetries    int     `json:"max_retries"`
	RetriesUsed   int     `json:"retries_used"`
	Remaining     int     `json:"remaining"`
	TotalAttempts int     `json:"total_attempts"`
	Successes     int     `json:"successes"`
	Failures      int     `json:"failures"`
	SuccessRate   float64 `json:"success_rate"`
}

// Stats returns a snapshot of the current retry budget state. Thread-safe.
func (b *RetryBudget) Stats() RetryBudgetStats {
	b.mu.Lock()
	defer b.mu.Unlock()

	remaining := b.config.MaxRetries - b.retriesUsed
	if remaining < 0 {
		remaining = 0
	}

	var rate float64
	if b.totalAttempts > 0 {
		rate = float64(b.successes) / float64(b.totalAttempts)
	}

	return RetryBudgetStats{
		SessionID:     b.sessionID,
		MaxRetries:    b.config.MaxRetries,
		RetriesUsed:   b.retriesUsed,
		Remaining:     remaining,
		TotalAttempts: b.totalAttempts,
		Successes:     b.successes,
		Failures:      b.failures,
		SuccessRate:   rate,
	}
}

// CanRetry returns true if retries remain in the budget. Thread-safe.
func (b *RetryBudget) CanRetry() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.retriesUsed < b.config.MaxRetries
}

// Remaining returns the number of retries left. Thread-safe.
func (b *RetryBudget) Remaining() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	r := b.config.MaxRetries - b.retriesUsed
	if r < 0 {
		return 0
	}
	return r
}

// RecordSuccess records a successful attempt. Thread-safe.
func (b *RetryBudget) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalAttempts++
	b.successes++
}

// RecordFailure records a failed attempt and consumes one retry.
// Returns ErrRetryBudgetExhausted if no retries remain.
// Thread-safe.
func (b *RetryBudget) RecordFailure() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.totalAttempts++
	b.failures++
	b.retriesUsed++

	if b.retriesUsed > b.config.MaxRetries {
		return fmt.Errorf("%w: session %s used %d/%d retries",
			ErrRetryBudgetExhausted, b.sessionID, b.retriesUsed, b.config.MaxRetries)
	}
	return nil
}

// NextDelay returns the backoff delay for the next retry attempt.
// The delay uses full jitter: uniform random in [0, min(maxDelay, baseDelay * factor^attempt)].
// Thread-safe.
func (b *RetryBudget) NextDelay() time.Duration {
	b.mu.Lock()
	attempt := b.retriesUsed
	cfg := b.config
	b.mu.Unlock()

	base := float64(cfg.BaseDelay) * math.Pow(cfg.Factor, float64(attempt))
	ceiling := math.Min(base, float64(cfg.MaxDelay))
	jittered := rand.Float64() * ceiling
	return time.Duration(jittered)
}

// Reset clears all counters and restores the full retry budget. Thread-safe.
func (b *RetryBudget) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.totalAttempts = 0
	b.successes = 0
	b.failures = 0
	b.retriesUsed = 0
}
