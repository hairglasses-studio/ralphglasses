package gateway

import (
	"context"
	"errors"
)

// DefaultSemaphoreMax is the default maximum number of concurrent MCP handler
// executions.
const DefaultSemaphoreMax = 32

// ErrSemaphoreExhausted is returned by TryAcquire when no permits are available.
var ErrSemaphoreExhausted = errors.New("semaphore: no permits available")

// Semaphore is a bounded counting semaphore that limits concurrent executions.
// Acquire blocks until a permit is available or the context is cancelled.
// TryAcquire is a non-blocking alternative.
// Each successful Acquire or TryAcquire must be paired with a Release.
type Semaphore struct {
	ch chan struct{}
}

// NewSemaphore creates a Semaphore with the given maximum concurrency.
// If max <= 0 the DefaultSemaphoreMax is used.
func NewSemaphore(max int) *Semaphore {
	if max <= 0 {
		max = DefaultSemaphoreMax
	}
	ch := make(chan struct{}, max)
	for i := 0; i < max; i++ {
		ch <- struct{}{}
	}
	return &Semaphore{ch: ch}
}

// Acquire waits until a permit is available or ctx is cancelled.
// Returns ctx.Err() if the context is cancelled before a permit is obtained.
func (s *Semaphore) Acquire(ctx context.Context) error {
	select {
	case <-s.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release returns one permit to the semaphore. It must be called exactly once
// per successful Acquire or TryAcquire.
func (s *Semaphore) Release() {
	s.ch <- struct{}{}
}

// TryAcquire attempts to acquire a permit without blocking.
// Returns ErrSemaphoreExhausted if no permit is immediately available.
func (s *Semaphore) TryAcquire() error {
	select {
	case <-s.ch:
		return nil
	default:
		return ErrSemaphoreExhausted
	}
}

// Available returns the number of permits currently available.
func (s *Semaphore) Available() int {
	return len(s.ch)
}

// Max returns the total capacity of the semaphore.
func (s *Semaphore) Max() int {
	return cap(s.ch)
}
