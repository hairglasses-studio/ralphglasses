package fleet

import (
	"errors"
	"fmt"
	"sync"

	"github.com/hairglasses-studio/ralphglasses/internal/safety"
)

// ErrWorkerUnhealthy is returned when work is dispatched to an unhealthy worker.
var ErrWorkerUnhealthy = errors.New("worker is unhealthy: circuit breaker open")

// HealthCircuitBreaker returns a dispatch middleware that rejects work for
// unhealthy workers, allows work with a warning log for degraded workers,
// and passes through normally for healthy workers.
//
// Internally it maintains one safety.CircuitBreaker per workerID. The
// breaker state is driven by the HealthTracker rather than by call
// outcomes: unhealthy maps to OPEN, degraded to HALF-OPEN, and healthy
// to CLOSED.
func HealthCircuitBreaker(tracker *HealthTracker) func(workerID string, fn func() error) error {
	var mu sync.Mutex
	breakers := make(map[string]*safety.CircuitBreaker)

	getBreaker := func(workerID string) *safety.CircuitBreaker {
		mu.Lock()
		defer mu.Unlock()
		cb, ok := breakers[workerID]
		if !ok {
			cb = safety.MustNew(safety.Config{
				FailureThreshold: 1, // we drive transitions externally
				ResetTimeout:     1, // minimal; we reset manually
				SuccessThreshold: 1,
			})
			breakers[workerID] = cb
		}
		return cb
	}

	return func(workerID string, fn func() error) error {
		cb := getBreaker(workerID)
		state := tracker.GetState(workerID)

		// Sync the circuit breaker state with the health tracker state.
		switch state {
		case HealthUnhealthy:
			// Reject immediately — do not call fn.
			return fmt.Errorf("%w: %s", ErrWorkerUnhealthy, workerID)

		case HealthDegraded:
			// Allow through but reset breaker to half-open semantics.
			// We execute via the breaker so metrics are tracked, but
			// first ensure it is not stuck open.
			cb.Reset()
			return cb.Execute(fn)

		default:
			// Healthy or unknown — pass through with closed breaker.
			cb.Reset()
			return cb.Execute(fn)
		}
	}
}
