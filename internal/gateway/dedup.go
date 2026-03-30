package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// DefaultDedupTTL is the default TTL for in-flight dedup entries.
const DefaultDedupTTL = 5 * time.Second

// dedupResult holds the result of an in-flight request shared among waiters.
type dedupResult struct {
	value any
	err   error
}

// dedupEntry tracks an in-flight request.
type dedupEntry struct {
	ch      chan struct{} // closed when result is ready
	result  dedupResult
	created time.Time
}

// DedupMiddleware deduplicates concurrent identical requests based on
// method + argument hash, coalescing duplicates into the first in-flight call.
type DedupMiddleware struct {
	inflight sync.Map // key string -> *dedupEntry
	ttl      time.Duration
}

// NewDedupMiddleware creates a DedupMiddleware with the given TTL.
// If ttl is 0 the DefaultDedupTTL is used.
func NewDedupMiddleware(ttl time.Duration) *DedupMiddleware {
	if ttl <= 0 {
		ttl = DefaultDedupTTL
	}
	return &DedupMiddleware{ttl: ttl}
}

// RequestKey computes a stable dedup key for the given method and arguments.
func RequestKey(method string, args any) (string, error) {
	b, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("dedup: marshal args: %w", err)
	}
	h := sha256.New()
	h.Write([]byte(method))
	h.Write(b)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Do executes fn for the first caller for key and returns the same result to
// all concurrent callers that arrive while fn is in-flight.
// Callers that arrive after the entry has expired will start a new call.
func (d *DedupMiddleware) Do(ctx context.Context, key string, fn func(context.Context) (any, error)) (any, error) {
	// Fast path: check for an existing non-expired entry.
	if v, ok := d.inflight.Load(key); ok {
		entry := v.(*dedupEntry)
		if time.Since(entry.created) < d.ttl {
			select {
			case <-entry.ch:
				// Result is ready.
				return entry.result.value, entry.result.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// Entry is stale; fall through to create a new one.
	}

	// Slow path: try to become the owner of the in-flight slot.
	entry := &dedupEntry{
		ch:      make(chan struct{}),
		created: time.Now(),
	}
	actual, loaded := d.inflight.LoadOrStore(key, entry)
	if loaded {
		// Another goroutine stored an entry first; wait on theirs.
		existing := actual.(*dedupEntry)
		if time.Since(existing.created) < d.ttl {
			select {
			case <-existing.ch:
				return existing.result.value, existing.result.err
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		// Stale — race to overwrite with a fresh entry.
		d.inflight.CompareAndDelete(key, existing)
		return d.Do(ctx, key, fn)
	}

	// We are the owner; execute fn and broadcast the result.
	val, err := fn(ctx)
	entry.result = dedupResult{value: val, err: err}
	close(entry.ch)

	// Schedule cleanup after TTL to avoid unbounded map growth.
	go func() {
		time.Sleep(d.ttl)
		d.inflight.CompareAndDelete(key, entry)
	}()

	return val, err
}

// Cleanup removes all entries older than the configured TTL.
// Call this periodically if you need proactive memory management.
func (d *DedupMiddleware) Cleanup() {
	now := time.Now()
	d.inflight.Range(func(k, v any) bool {
		entry := v.(*dedupEntry)
		if now.Sub(entry.created) >= d.ttl {
			d.inflight.Delete(k)
		}
		return true
	})
}
