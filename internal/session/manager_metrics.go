package session

import (
	"sync"
	"sync/atomic"
	"time"
)

// ManagerMetrics tracks concurrent session counts, lock contention time,
// and operation latencies using sync/atomic counters. Safe for concurrent use.
type ManagerMetrics struct {
	// Session counters.
	activeSessions   atomic.Int64
	totalLaunched    atomic.Int64
	totalCompleted   atomic.Int64
	totalErrored     atomic.Int64

	// Lock contention tracking (nanoseconds).
	contentionNs     atomic.Int64
	contentionCount  atomic.Int64

	// Operation latency tracking (nanoseconds).
	launchLatencyNs  atomic.Int64
	launchCount      atomic.Int64
	queryLatencyNs   atomic.Int64
	queryCount       atomic.Int64

	// Peak concurrent sessions observed.
	peakSessions     atomic.Int64

	// Per-provider session counts. Uses sync.Map for lock-free reads
	// with string(Provider) keys and *atomic.Int64 values.
	providerCounts   sync.Map
}

// MetricsSnapshot is a point-in-time copy of all metrics, suitable for
// TUI rendering or JSON serialization.
type MetricsSnapshot struct {
	ActiveSessions int64             `json:"active_sessions"`
	PeakSessions   int64             `json:"peak_sessions"`
	TotalLaunched  int64             `json:"total_launched"`
	TotalCompleted int64             `json:"total_completed"`
	TotalErrored   int64             `json:"total_errored"`

	ContentionTotal time.Duration    `json:"contention_total"`
	ContentionCount int64            `json:"contention_count"`
	ContentionAvg   time.Duration    `json:"contention_avg"`

	LaunchLatencyTotal time.Duration `json:"launch_latency_total"`
	LaunchCount        int64         `json:"launch_count"`
	LaunchLatencyAvg   time.Duration `json:"launch_latency_avg"`

	QueryLatencyTotal time.Duration  `json:"query_latency_total"`
	QueryCount        int64          `json:"query_count"`
	QueryLatencyAvg   time.Duration  `json:"query_latency_avg"`

	ProviderCounts map[Provider]int64 `json:"provider_counts"`
}

// NewManagerMetrics returns an initialized ManagerMetrics.
func NewManagerMetrics() *ManagerMetrics {
	return &ManagerMetrics{}
}

// RecordLaunch increments the active, total-launched, and per-provider
// counters, and updates the peak watermark.
func (m *ManagerMetrics) RecordLaunch(p Provider) {
	m.totalLaunched.Add(1)
	cur := m.activeSessions.Add(1)
	m.getProviderCounter(p).Add(1)

	// Update peak using compare-and-swap loop.
	for {
		peak := m.peakSessions.Load()
		if cur <= peak {
			break
		}
		if m.peakSessions.CompareAndSwap(peak, cur) {
			break
		}
	}
}

// RecordComplete decrements the active counter and increments the
// completed counter, and decrements the per-provider counter.
func (m *ManagerMetrics) RecordComplete(p Provider) {
	m.activeSessions.Add(-1)
	m.totalCompleted.Add(1)
	m.getProviderCounter(p).Add(-1)
}

// RecordError decrements the active counter and increments the errored
// counter, and decrements the per-provider counter.
func (m *ManagerMetrics) RecordError(p Provider) {
	m.activeSessions.Add(-1)
	m.totalErrored.Add(1)
	m.getProviderCounter(p).Add(-1)
}

// RecordContention adds a lock-wait duration to the contention accumulators.
func (m *ManagerMetrics) RecordContention(d time.Duration) {
	m.contentionNs.Add(d.Nanoseconds())
	m.contentionCount.Add(1)
}

// RecordLaunchLatency adds a launch operation duration to the latency
// accumulators.
func (m *ManagerMetrics) RecordLaunchLatency(d time.Duration) {
	m.launchLatencyNs.Add(d.Nanoseconds())
	m.launchCount.Add(1)
}

// RecordQueryLatency adds a query operation duration to the latency
// accumulators.
func (m *ManagerMetrics) RecordQueryLatency(d time.Duration) {
	m.queryLatencyNs.Add(d.Nanoseconds())
	m.queryCount.Add(1)
}

// Snapshot returns a consistent point-in-time copy of all metrics.
func (m *ManagerMetrics) Snapshot() MetricsSnapshot {
	s := MetricsSnapshot{
		ActiveSessions:     m.activeSessions.Load(),
		PeakSessions:       m.peakSessions.Load(),
		TotalLaunched:      m.totalLaunched.Load(),
		TotalCompleted:     m.totalCompleted.Load(),
		TotalErrored:       m.totalErrored.Load(),
		ContentionTotal:    time.Duration(m.contentionNs.Load()),
		ContentionCount:    m.contentionCount.Load(),
		LaunchLatencyTotal: time.Duration(m.launchLatencyNs.Load()),
		LaunchCount:        m.launchCount.Load(),
		QueryLatencyTotal:  time.Duration(m.queryLatencyNs.Load()),
		QueryCount:         m.queryCount.Load(),
		ProviderCounts:     make(map[Provider]int64),
	}

	// Compute averages safely.
	if s.ContentionCount > 0 {
		s.ContentionAvg = s.ContentionTotal / time.Duration(s.ContentionCount)
	}
	if s.LaunchCount > 0 {
		s.LaunchLatencyAvg = s.LaunchLatencyTotal / time.Duration(s.LaunchCount)
	}
	if s.QueryCount > 0 {
		s.QueryLatencyAvg = s.QueryLatencyTotal / time.Duration(s.QueryCount)
	}

	// Copy per-provider counts.
	m.providerCounts.Range(func(key, val any) bool {
		p := key.(Provider)
		counter := val.(*atomic.Int64)
		s.ProviderCounts[p] = counter.Load()
		return true
	})

	return s
}

// getProviderCounter returns the atomic counter for a provider,
// creating it on first use via sync.Map's LoadOrStore.
func (m *ManagerMetrics) getProviderCounter(p Provider) *atomic.Int64 {
	val, _ := m.providerCounts.LoadOrStore(p, &atomic.Int64{})
	return val.(*atomic.Int64)
}
