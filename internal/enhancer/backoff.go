package enhancer

import (
	"context"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"
	"time"
)

// BackoffConfig holds parameters for exponential backoff with jitter.
type BackoffConfig struct {
	// BaseDelay is the initial delay before the first retry (default 500ms).
	BaseDelay time.Duration

	// MaxDelay is the upper bound on any single delay (default 30s).
	MaxDelay time.Duration

	// Factor is the exponential multiplier per attempt (default 2.0).
	Factor float64

	// MaxRetries is the maximum number of retry attempts (default 3).
	// 0 means no retries (single attempt only).
	MaxRetries int
}

// DefaultBackoff returns a BackoffConfig with the standard parameters:
// base 500ms, max 30s, factor 2.0, 3 retries.
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{
		BaseDelay:  500 * time.Millisecond,
		MaxDelay:   30 * time.Second,
		Factor:     2.0,
		MaxRetries: 3,
	}
}

// delay calculates the backoff duration for the given attempt (0-indexed).
// Uses full jitter: uniform random in [0, min(maxDelay, baseDelay * factor^attempt)].
// Full jitter is preferred over equal jitter because it reduces contention
// across concurrent callers (see AWS Architecture Blog on exponential backoff).
func (b BackoffConfig) delay(attempt int) time.Duration {
	base := float64(b.BaseDelay) * math.Pow(b.Factor, float64(attempt))
	ceiling := math.Min(base, float64(b.MaxDelay))
	jittered := rand.Float64() * ceiling
	return time.Duration(jittered)
}

// sleepFunc is the function used to wait between retries.
// Overridden in tests to avoid real delays.
var sleepFunc = func(ctx context.Context, d time.Duration) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(d):
		return nil
	}
}

// isRetryableError determines if an error from an LLM API call should be retried.
// Retryable conditions: rate limits (429), server errors (5xx), transient network errors.
// Non-retryable: auth errors (401/403), bad request (400), context cancellation.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errMsg := err.Error()

	// Context cancellation/deadline is not retryable.
	if strings.Contains(errMsg, "context canceled") ||
		strings.Contains(errMsg, "context deadline exceeded") {
		return false
	}

	// Rate limiting is always retryable.
	if strings.Contains(errMsg, "status 429") ||
		strings.Contains(errMsg, "rate_limit") ||
		strings.Contains(errMsg, "RESOURCE_EXHAUSTED") {
		return true
	}

	// Server errors (5xx) are retryable.
	if strings.Contains(errMsg, "status 500") ||
		strings.Contains(errMsg, "status 502") ||
		strings.Contains(errMsg, "status 503") ||
		strings.Contains(errMsg, "status 504") {
		return true
	}

	// Check for HTTP status codes embedded by the Anthropic SDK.
	for code := 500; code <= 599; code++ {
		if text := http.StatusText(code); text != "" && strings.Contains(errMsg, text) {
			return true
		}
	}

	// Transient network errors are retryable.
	if strings.Contains(errMsg, "connection refused") ||
		strings.Contains(errMsg, "connection reset") ||
		strings.Contains(errMsg, "no such host") ||
		strings.Contains(errMsg, "i/o timeout") ||
		strings.Contains(errMsg, "EOF") ||
		strings.Contains(errMsg, "broken pipe") {
		return true
	}

	// Default: not retryable (auth errors, bad requests, etc.).
	return false
}

// retryImprove calls the PromptImprover with exponential backoff and jitter.
// Returns the result from the first successful call, or the last error
// if all attempts (1 initial + MaxRetries retries) are exhausted.
func retryImprove(ctx context.Context, client PromptImprover, prompt string, opts ImproveOptions, cfg BackoffConfig) (*ImproveResult, error) {
	totalAttempts := 1 + cfg.MaxRetries

	var lastErr error
	for attempt := range totalAttempts {
		result, err := client.Improve(ctx, prompt, opts)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Don't retry if this is the last attempt or error is not retryable.
		if attempt >= totalAttempts-1 || !isRetryableError(err) {
			break
		}

		// Wait with exponential backoff + jitter.
		d := cfg.delay(attempt)
		if err := sleepFunc(ctx, d); err != nil {
			// Context was cancelled during sleep.
			return nil, lastErr
		}
	}

	return nil, lastErr
}
