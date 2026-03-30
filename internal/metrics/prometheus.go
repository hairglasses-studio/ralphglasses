// Package metrics provides Prometheus instrumentation for the ralphglasses fleet.
package metrics

import (
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Metrics holds all Prometheus collectors for the ralphglasses fleet.
type Metrics struct {
	reg prometheus.Registerer

	// Fleet / session metrics
	SessionsTotal          *prometheus.CounterVec
	SessionsActive         *prometheus.GaugeVec
	SessionDurationSeconds *prometheus.HistogramVec
	SessionCostUSD         *prometheus.CounterVec
	IterationsTotal        *prometheus.CounterVec

	// Worker metrics
	WorkersActive     prometheus.Gauge
	WorkerQueueDepth  prometheus.Gauge
	WorkerHealthScore *prometheus.GaugeVec

	// Event metrics
	EventsTotal        *prometheus.CounterVec
	EventBusSubscribers prometheus.Gauge
}

// NewMetrics creates a Metrics instance and registers all collectors with reg.
// Pass prometheus.DefaultRegisterer for the global registry, or a
// prometheus.NewRegistry() for isolated testing.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{reg: reg}

	m.SessionsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rg_sessions_total",
		Help: "Total sessions launched, partitioned by provider and terminal status.",
	}, []string{"provider", "status"})

	m.SessionsActive = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rg_sessions_active",
		Help: "Number of currently active (running/launching) sessions per provider.",
	}, []string{"provider"})

	m.SessionDurationSeconds = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "rg_session_duration_seconds",
		Help:    "Observed session wall-clock duration in seconds.",
		Buckets: prometheus.ExponentialBuckets(1, 2, 14), // 1s .. ~4.5h
	}, []string{"provider"})

	m.SessionCostUSD = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rg_session_cost_usd",
		Help: "Cumulative session cost in USD, partitioned by provider.",
	}, []string{"provider"})

	m.IterationsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rg_iterations_total",
		Help: "Total loop iterations, partitioned by provider and session.",
	}, []string{"provider", "session_id"})

	m.WorkersActive = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rg_workers_active",
		Help: "Number of currently registered and online workers.",
	})

	m.WorkerQueueDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rg_worker_queue_depth",
		Help: "Number of pending items in the fleet work queue.",
	})

	m.WorkerHealthScore = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "rg_worker_health_score",
		Help: "Health score (0.0-1.0) of each worker.",
	}, []string{"worker_id"})

	m.EventsTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "rg_events_total",
		Help: "Total events published to the event bus, partitioned by type.",
	}, []string{"event_type"})

	m.EventBusSubscribers = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "rg_event_bus_subscribers",
		Help: "Current number of event bus subscribers.",
	})

	reg.MustRegister(
		m.SessionsTotal,
		m.SessionsActive,
		m.SessionDurationSeconds,
		m.SessionCostUSD,
		m.IterationsTotal,
		m.WorkersActive,
		m.WorkerQueueDepth,
		m.WorkerHealthScore,
		m.EventsTotal,
		m.EventBusSubscribers,
	)

	return m
}

// Handler returns an http.Handler that serves the /metrics endpoint.
// If reg implements prometheus.Gatherer (e.g. *prometheus.Registry), that
// gatherer is used; otherwise the default gatherer is returned.
func (m *Metrics) Handler() http.Handler {
	if g, ok := m.reg.(prometheus.Gatherer); ok {
		return promhttp.HandlerFor(g, promhttp.HandlerOpts{})
	}
	return promhttp.Handler()
}

// ---------------------------------------------------------------------------
// Convenience recording methods
// ---------------------------------------------------------------------------

// RecordSessionStart increments the sessions-total counter with status
// "started" and bumps the active-sessions gauge for the given provider.
func (m *Metrics) RecordSessionStart(provider string) {
	m.SessionsTotal.WithLabelValues(provider, "started").Inc()
	m.SessionsActive.WithLabelValues(provider).Inc()
}

// RecordSessionEnd decrements the active gauge, records duration and cost,
// and increments sessions-total with the terminal status.
func (m *Metrics) RecordSessionEnd(provider string, duration time.Duration, costUSD float64, status string) {
	m.SessionsActive.WithLabelValues(provider).Dec()
	m.SessionDurationSeconds.WithLabelValues(provider).Observe(duration.Seconds())
	m.SessionCostUSD.WithLabelValues(provider).Add(costUSD)
	m.SessionsTotal.WithLabelValues(provider, status).Inc()
}

// RecordIteration increments the iteration counter for a provider/session pair.
func (m *Metrics) RecordIteration(provider, sessionID string) {
	m.IterationsTotal.WithLabelValues(provider, sessionID).Inc()
}

// RecordEvent increments the event counter for the given event type.
func (m *Metrics) RecordEvent(eventType string) {
	m.EventsTotal.WithLabelValues(eventType).Inc()
}

// SetWorkersActive sets the active-workers gauge.
func (m *Metrics) SetWorkersActive(n float64) {
	m.WorkersActive.Set(n)
}

// SetWorkerQueueDepth sets the queue-depth gauge.
func (m *Metrics) SetWorkerQueueDepth(n float64) {
	m.WorkerQueueDepth.Set(n)
}

// SetWorkerHealthScore sets the health score for a specific worker.
func (m *Metrics) SetWorkerHealthScore(workerID string, score float64) {
	m.WorkerHealthScore.WithLabelValues(workerID).Set(score)
}

// SetEventBusSubscribers sets the subscriber count gauge.
func (m *Metrics) SetEventBusSubscribers(n float64) {
	m.EventBusSubscribers.Set(n)
}
