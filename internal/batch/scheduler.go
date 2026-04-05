package batch

import (
	"context"
	"log"
	"sync"
	"time"
)

// Scheduler auto-flushes a BatchManager based on a time interval
// or when the queue reaches a size threshold.
type Scheduler struct {
	manager *BatchManager
	cfg     BatchManagerConfig

	mu      sync.Mutex
	cancel  context.CancelFunc
	stopped chan struct{}
	running bool

	// OnFlush is called after each successful auto-flush. Optional, used for testing.
	OnFlush func(result *BatchManagerResult)
	// OnError is called when an auto-flush fails. Optional.
	OnError func(err error)
}

// NewScheduler creates a Scheduler for the given BatchManager.
func NewScheduler(manager *BatchManager, cfg BatchManagerConfig) *Scheduler {
	return &Scheduler{
		manager: manager,
		cfg:     cfg,
	}
}

// Start begins the auto-flush loop. It flushes when:
//   - The flush interval elapses and the queue is non-empty.
//   - The queue reaches MaxBatchSize (checked on each tick).
//
// Start is safe to call multiple times; duplicate calls are no-ops.
func (s *Scheduler) Start(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return
	}

	flushCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.stopped = make(chan struct{})
	s.running = true

	go s.loop(flushCtx)
}

// Stop halts the auto-flush loop and waits for it to exit.
// Safe to call multiple times or when not started.
func (s *Scheduler) Stop() {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return
	}
	cancel := s.cancel
	stopped := s.stopped
	s.running = false
	s.mu.Unlock()

	cancel()
	<-stopped
}

// Running reports whether the scheduler loop is active.
func (s *Scheduler) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *Scheduler) loop(ctx context.Context) {
	defer close(s.stopped)

	interval := s.cfg.flushInterval()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Also check queue size at a faster rate so we flush promptly
	// when the size threshold is hit between timer ticks.
	checkInterval := max(interval/10, 100*time.Millisecond)
	if checkInterval > 500*time.Millisecond {
		checkInterval = 500 * time.Millisecond
	}
	sizeTicker := time.NewTicker(checkInterval)
	defer sizeTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Drain remaining items on shutdown.
			s.tryFlush(context.Background())
			return

		case <-ticker.C:
			s.tryFlush(ctx)

		case <-sizeTicker.C:
			if s.manager.QueueLen() >= s.cfg.maxBatchSize() {
				s.tryFlush(ctx)
			}
		}
	}
}

func (s *Scheduler) tryFlush(ctx context.Context) {
	if s.manager.QueueLen() == 0 {
		return
	}

	result, err := s.manager.Flush(ctx)
	if err != nil {
		if s.OnError != nil {
			s.OnError(err)
		} else {
			log.Printf("batch scheduler: flush error: %v", err)
		}
		return
	}

	if result != nil && s.OnFlush != nil {
		s.OnFlush(result)
	}
}
