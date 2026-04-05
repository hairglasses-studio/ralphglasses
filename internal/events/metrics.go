package events

import (
	"sync"
	"sync/atomic"
	"time"
)

// metrics tracks observability counters for the event bus.
// All fields use atomic operations for lock-free reads.
type metrics struct {
	totalPublished  atomic.Int64
	totalDropped    atomic.Int64
	subscriberCount atomic.Int64

	// Per-type publish counts. Protected by mu because maps are not
	// safe for concurrent writes even with atomic value wrappers.
	mu              sync.RWMutex
	publishedByType map[EventType]*atomic.Int64

	// Latency histogram — fixed buckets in microseconds.
	// Each bucket counts observations <= that bucket boundary.
	// Buckets: 10us, 50us, 100us, 500us, 1ms, 5ms, 10ms, 50ms, 100ms, +Inf
	latencyBuckets [latencyBucketCount]atomic.Int64
	latencySum     atomic.Int64 // cumulative nanoseconds
	latencyCount   atomic.Int64 // total observations
}

// latencyBucketCount is the number of fixed histogram buckets.
const latencyBucketCount = 10

// latencyBoundaries defines the upper bound of each bucket in microseconds.
var latencyBoundaries = [latencyBucketCount]int64{
	10,      // 10 us
	50,      // 50 us
	100,     // 100 us
	500,     // 500 us
	1_000,   // 1 ms
	5_000,   // 5 ms
	10_000,  // 10 ms
	50_000,  // 50 ms
	100_000, // 100 ms
	-1,      // +Inf (catches everything)
}

// MetricsSnapshot is a point-in-time view of event bus metrics.
type MetricsSnapshot struct {
	// TotalPublished is the total number of events published.
	TotalPublished int64 `json:"total_published"`

	// PublishedByType maps each event type to its publish count.
	PublishedByType map[EventType]int64 `json:"published_by_type"`

	// TotalDropped is the total number of events dropped due to slow subscribers.
	TotalDropped int64 `json:"total_dropped"`

	// SubscriberCount is the current number of active subscribers.
	SubscriberCount int64 `json:"subscriber_count"`

	// Latency holds the processing latency histogram.
	Latency LatencyHistogram `json:"latency"`
}

// LatencyHistogram holds bucketed latency observations for event processing.
type LatencyHistogram struct {
	// Buckets maps an upper-bound label to the cumulative count of
	// observations that fell at or below that boundary.
	Buckets []LatencyBucket `json:"buckets"`

	// Sum is the total observed latency.
	Sum time.Duration `json:"sum_ns"`

	// Count is the total number of observations.
	Count int64 `json:"count"`
}

// LatencyBucket is a single histogram bucket.
type LatencyBucket struct {
	// Le is the upper boundary ("less than or equal") label, e.g. "1ms".
	Le string `json:"le"`

	// Count is the cumulative number of observations <= Le.
	Count int64 `json:"count"`
}

// latencyBucketLabels are human-readable labels matching latencyBoundaries.
var latencyBucketLabels = [latencyBucketCount]string{
	"10us",
	"50us",
	"100us",
	"500us",
	"1ms",
	"5ms",
	"10ms",
	"50ms",
	"100ms",
	"+Inf",
}

// newMetrics creates an initialized metrics instance.
func newMetrics() *metrics {
	return &metrics{
		publishedByType: make(map[EventType]*atomic.Int64),
	}
}

// recordPublish increments the total and per-type publish counters.
func (m *metrics) recordPublish(t EventType) {
	m.totalPublished.Add(1)

	m.mu.RLock()
	counter, ok := m.publishedByType[t]
	m.mu.RUnlock()

	if ok {
		counter.Add(1)
		return
	}

	// Slow path: first time seeing this type.
	m.mu.Lock()
	// Double-check after acquiring write lock.
	counter, ok = m.publishedByType[t]
	if !ok {
		counter = &atomic.Int64{}
		m.publishedByType[t] = counter
	}
	m.mu.Unlock()
	counter.Add(1)
}

// recordDrop increments the dropped counter by n.
func (m *metrics) recordDrop(n int64) {
	m.totalDropped.Add(n)
}

// recordSubscribe increments the subscriber gauge.
func (m *metrics) recordSubscribe() {
	m.subscriberCount.Add(1)
}

// recordUnsubscribe decrements the subscriber gauge.
func (m *metrics) recordUnsubscribe() {
	m.subscriberCount.Add(-1)
}

// recordLatency records a publish latency observation in the histogram.
func (m *metrics) recordLatency(d time.Duration) {
	ns := d.Nanoseconds()
	us := ns / 1000 // for bucket placement
	m.latencySum.Add(ns)
	m.latencyCount.Add(1)

	for i, boundary := range latencyBoundaries {
		if boundary < 0 || us <= boundary {
			m.latencyBuckets[i].Add(1)
			// Histogram buckets are cumulative, so increment all
			// remaining buckets as well.
			for j := i + 1; j < latencyBucketCount; j++ {
				m.latencyBuckets[j].Add(1)
			}
			return
		}
	}
}

// snapshot returns a point-in-time MetricsSnapshot.
func (m *metrics) snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		TotalPublished:  m.totalPublished.Load(),
		TotalDropped:    m.totalDropped.Load(),
		SubscriberCount: m.subscriberCount.Load(),
	}

	// Copy per-type map.
	m.mu.RLock()
	s.PublishedByType = make(map[EventType]int64, len(m.publishedByType))
	for t, c := range m.publishedByType {
		s.PublishedByType[t] = c.Load()
	}
	m.mu.RUnlock()

	// Copy histogram.
	s.Latency.Count = m.latencyCount.Load()
	s.Latency.Sum = time.Duration(m.latencySum.Load())
	s.Latency.Buckets = make([]LatencyBucket, latencyBucketCount)
	for i := range latencyBucketCount {
		s.Latency.Buckets[i] = LatencyBucket{
			Le:    latencyBucketLabels[i],
			Count: m.latencyBuckets[i].Load(),
		}
	}

	return s
}

// Metrics returns a point-in-time snapshot of event bus observability metrics.
func (b *Bus) Metrics() MetricsSnapshot {
	return b.metrics.snapshot()
}
