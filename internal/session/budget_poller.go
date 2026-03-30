package session

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// PauseFunc is called by the poller when a session should be paused.
// It receives the session ID and returns an error if the pause fails.
type PauseFunc func(sessionID string) error

// BudgetPoller periodically checks budget usage and pauses sessions at ceiling.
type BudgetPoller struct {
	pool     *BudgetPool
	interval time.Duration
	pauseFn  PauseFunc

	mu      sync.Mutex
	cancel  context.CancelFunc
	stopped chan struct{}
}

// NewBudgetPoller creates a poller that checks the pool at the given interval.
// pauseFn is called for each session that should be paused.
func NewBudgetPoller(pool *BudgetPool, interval time.Duration, pauseFn PauseFunc) *BudgetPoller {
	return &BudgetPoller{
		pool:     pool,
		interval: interval,
		pauseFn:  pauseFn,
	}
}

// Start begins the polling loop. It blocks until ctx is cancelled or Stop is called.
func (bp *BudgetPoller) Start(ctx context.Context) {
	bp.mu.Lock()
	ctx, cancel := context.WithCancel(ctx)
	bp.cancel = cancel
	bp.stopped = make(chan struct{})
	bp.mu.Unlock()

	defer close(bp.stopped)

	ticker := time.NewTicker(bp.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			bp.checkAndPause()
		}
	}
}

// Stop signals the poller to stop and waits for it to finish.
func (bp *BudgetPoller) Stop() {
	bp.mu.Lock()
	cancel := bp.cancel
	stopped := bp.stopped
	bp.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if stopped != nil {
		<-stopped
	}
}

// checkAndPause examines all sessions in the pool and pauses those at ceiling.
func (bp *BudgetPoller) checkAndPause() {
	summary := bp.pool.Summary()

	for sessionID := range summary.Sessions {
		if bp.pool.ShouldPause(sessionID) {
			slog.Info("budget poller: pausing session at ceiling",
				"session", sessionID,
				"remaining", summary.Remaining,
				"total_spent", summary.TotalSpent,
			)
			if bp.pauseFn != nil {
				if err := bp.pauseFn(sessionID); err != nil {
					slog.Warn("budget poller: failed to pause session",
						"session", sessionID,
						"error", err,
					)
				}
			}
		}
	}
}
