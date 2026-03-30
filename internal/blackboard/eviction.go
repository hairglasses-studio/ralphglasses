package blackboard

import "time"

// SetTTL updates the TTL of an existing entry identified by namespace and key.
// The entry's UpdatedAt is reset so the new TTL counts from now.
// It returns false if the entry does not exist.
func (bb *Blackboard) SetTTL(namespace, key string, d time.Duration) bool {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	ck := compositeKey(namespace, key)
	e, ok := bb.entries[ck]
	if !ok {
		return false
	}
	e.TTL = d
	e.UpdatedAt = time.Now()
	return true
}

// StartEvictor launches a background goroutine that calls GC at the given
// interval. It is safe to call multiple times; subsequent calls are no-ops
// until StopEvictor is called. The returned channel is closed when the
// evictor goroutine has fully stopped (same channel returned by StopEvictor).
func (bb *Blackboard) StartEvictor(interval time.Duration) <-chan struct{} {
	bb.mu.Lock()
	defer bb.mu.Unlock()

	// Already running.
	if bb.evictStop != nil {
		return bb.evictDone
	}

	bb.evictStop = make(chan struct{})
	bb.evictDone = make(chan struct{})

	go bb.evictLoop(interval, bb.evictStop, bb.evictDone)
	return bb.evictDone
}

// StopEvictor signals the background evictor to stop and blocks until it
// has finished. It is safe to call when no evictor is running.
func (bb *Blackboard) StopEvictor() {
	bb.mu.Lock()
	stop := bb.evictStop
	done := bb.evictDone
	bb.mu.Unlock()

	if stop == nil {
		return
	}

	close(stop)
	<-done

	bb.mu.Lock()
	bb.evictStop = nil
	bb.evictDone = nil
	bb.mu.Unlock()
}

// evictLoop is the background goroutine. It ticks at the given interval
// and runs GC on each tick until the stop channel is closed.
func (bb *Blackboard) evictLoop(interval time.Duration, stop <-chan struct{}, done chan<- struct{}) {
	defer close(done)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			// Run one final GC to clean up anything that expired while
			// we were waiting for the tick.
			bb.GC()
			return
		case <-ticker.C:
			bb.GC()
		}
	}
}
