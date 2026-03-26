package session

import (
	"sync"
	"time"
)

// StallDetector monitors a worker session for output activity.
// If no activity is recorded within the timeout, it signals a stall.
type StallDetector struct {
	mu         sync.Mutex
	timeout    time.Duration
	lastActive time.Time
	stallCount int
	done       chan struct{}
}

// NewStallDetector creates a detector with the given timeout.
// A timeout of 0 disables detection.
func NewStallDetector(timeout time.Duration) *StallDetector {
	return &StallDetector{
		timeout:    timeout,
		lastActive: time.Now(),
		done:       make(chan struct{}),
	}
}

// RecordActivity updates the last-active timestamp. Call this whenever
// the worker produces output.
func (sd *StallDetector) RecordActivity() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	sd.lastActive = time.Now()
}

// IsStalled returns true if the elapsed time since last activity exceeds timeout.
// Always returns false when timeout is 0 (disabled).
func (sd *StallDetector) IsStalled() bool {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	if sd.timeout <= 0 {
		return false
	}
	return time.Since(sd.lastActive) >= sd.timeout
}

// StallCount returns the number of stall events detected.
func (sd *StallDetector) StallCount() int {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	return sd.stallCount
}

// Start begins background stall monitoring. Returns a channel that receives
// true when a stall is detected. Close the done channel via Stop to halt monitoring.
func (sd *StallDetector) Start() <-chan bool {
	ch := make(chan bool, 1)
	if sd.timeout <= 0 {
		// Disabled — return a channel that never fires.
		return ch
	}

	// Check interval: 1/20th of the timeout, clamped to [100ms, 30s].
	interval := sd.timeout / 20
	if interval < 100*time.Millisecond {
		interval = 100 * time.Millisecond
	}
	if interval > 30*time.Second {
		interval = 30 * time.Second
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-sd.done:
				return
			case <-ticker.C:
				if sd.IsStalled() {
					sd.mu.Lock()
					sd.stallCount++
					sd.mu.Unlock()
					select {
					case ch <- true:
					default:
						// Don't block if receiver is slow.
					}
				}
			}
		}
	}()

	return ch
}

// Stop halts stall monitoring.
func (sd *StallDetector) Stop() {
	sd.mu.Lock()
	defer sd.mu.Unlock()
	select {
	case <-sd.done:
		// Already stopped.
	default:
		close(sd.done)
	}
}
