package session

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// JSONParseMetrics holds Prometheus collectors for JSON parsing instrumentation.
type JSONParseMetrics struct {
	parseTotal         *prometheus.CounterVec
	parseDuration      prometheus.Histogram
	retryBudgetRemain  prometheus.Gauge
}

// NewJSONParseMetrics creates and registers JSON parse metrics with the given
// Prometheus registerer. Pass prometheus.DefaultRegisterer for the global
// registry, or prometheus.NewRegistry() for isolated testing.
func NewJSONParseMetrics(reg prometheus.Registerer) *JSONParseMetrics {
	m := &JSONParseMetrics{
		parseTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ralphglasses_json_parse_total",
			Help: "Total JSON parse attempts, partitioned by status (success, failure, truncated).",
		}, []string{"status"}),
		parseDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "ralphglasses_json_parse_duration_seconds",
			Help:    "Histogram of JSON parse durations in seconds.",
			Buckets: prometheus.DefBuckets,
		}),
		retryBudgetRemain: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ralphglasses_retry_budget_remaining",
			Help: "Current number of remaining retries in the active retry budget.",
		}),
	}

	reg.MustRegister(m.parseTotal, m.parseDuration, m.retryBudgetRemain)

	// Initialize counter labels so they appear in /metrics even before any events.
	m.parseTotal.WithLabelValues("success")
	m.parseTotal.WithLabelValues("failure")
	m.parseTotal.WithLabelValues("truncated")

	return m
}

// RecordParseSuccess records a successful JSON parse with its duration.
func (m *JSONParseMetrics) RecordParseSuccess(duration time.Duration) {
	m.parseTotal.WithLabelValues("success").Inc()
	m.parseDuration.Observe(duration.Seconds())
}

// RecordParseFailure records a failed JSON parse with its duration.
func (m *JSONParseMetrics) RecordParseFailure(duration time.Duration) {
	m.parseTotal.WithLabelValues("failure").Inc()
	m.parseDuration.Observe(duration.Seconds())
}

// RecordParseTruncated records a parse that required truncation.
func (m *JSONParseMetrics) RecordParseTruncated(duration time.Duration) {
	m.parseTotal.WithLabelValues("truncated").Inc()
	m.parseDuration.Observe(duration.Seconds())
}

// SetRetryBudgetRemaining updates the retry budget remaining gauge.
func (m *JSONParseMetrics) SetRetryBudgetRemaining(remaining float64) {
	m.retryBudgetRemain.Set(remaining)
}
