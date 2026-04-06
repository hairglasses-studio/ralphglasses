package session

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

const (
	// failedJSONPreviewLen is the maximum number of characters of failed JSON
	// to include in debug log entries, for diagnosing retry failures.
	failedJSONPreviewLen = 200
)

// JSONParseMetrics holds Prometheus collectors for JSON parsing instrumentation.
// It tracks parse attempts (success/failure/truncated), retry rates, response
// sizes, and logs previews of failed JSON for debugging.
type JSONParseMetrics struct {
	parseTotal         *prometheus.CounterVec
	parseDuration      prometheus.Histogram
	retryBudgetRemain  prometheus.Gauge
	retryRate          prometheus.Gauge
	retryTotal         *prometheus.CounterVec
	responseSizeBytes  *prometheus.HistogramVec
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
		retryRate: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "ralphglasses_json_retry_rate",
			Help: "Current JSON retry rate as a ratio (0.0-1.0). Calculated as failures / total attempts.",
		}),
		retryTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "ralphglasses_json_retry_total",
			Help: "Total JSON retries, partitioned by cause (response_too_large, parse_error, repair_failed).",
		}, []string{"cause"}),
		responseSizeBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "ralphglasses_response_size_bytes",
			Help:    "Histogram of tool response sizes in bytes, partitioned by outcome.",
			Buckets: []float64{256, 512, 1024, 2048, 4096, 8192, 16384, 32768, 65536, 131072},
		}, []string{"outcome"}),
	}

	reg.MustRegister(
		m.parseTotal, m.parseDuration, m.retryBudgetRemain,
		m.retryRate, m.retryTotal, m.responseSizeBytes,
	)

	// Initialize counter labels so they appear in /metrics even before any events.
	m.parseTotal.WithLabelValues("success")
	m.parseTotal.WithLabelValues("failure")
	m.parseTotal.WithLabelValues("truncated")
	m.retryTotal.WithLabelValues("response_too_large")
	m.retryTotal.WithLabelValues("parse_error")
	m.retryTotal.WithLabelValues("repair_failed")
	m.responseSizeBytes.WithLabelValues("ok")
	m.responseSizeBytes.WithLabelValues("truncated")
	m.responseSizeBytes.WithLabelValues("failed")

	return m
}

// RecordParseSuccess records a successful JSON parse with its duration.
func (m *JSONParseMetrics) RecordParseSuccess(duration time.Duration) {
	m.parseTotal.WithLabelValues("success").Inc()
	m.parseDuration.Observe(duration.Seconds())
}

// RecordParseFailure records a failed JSON parse with its duration.
// It also logs the first 200 chars of the failed input for debugging.
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

// RecordRetry increments the retry counter for the given cause and updates
// the overall retry rate gauge. Valid causes: "response_too_large",
// "parse_error", "repair_failed".
func (m *JSONParseMetrics) RecordRetry(cause string) {
	m.retryTotal.WithLabelValues(cause).Inc()
}

// RecordResponseSize records the size of a tool response and its outcome.
// outcome should be "ok", "truncated", or "failed".
func (m *JSONParseMetrics) RecordResponseSize(sizeBytes int, outcome string) {
	m.responseSizeBytes.WithLabelValues(outcome).Observe(float64(sizeBytes))
}

// UpdateRetryRate recalculates the retry rate from total successes and failures.
// Call this after each parse attempt to keep the gauge current.
func (m *JSONParseMetrics) UpdateRetryRate(totalAttempts, totalFailures int) {
	if totalAttempts == 0 {
		m.retryRate.Set(0)
		return
	}
	rate := float64(totalFailures) / float64(totalAttempts)
	m.retryRate.Set(rate)
}

// LogFailedJSON logs the first 200 characters of failed JSON input at WARN
// level for debugging retry failures. This is the primary diagnostic tool
// for understanding why parses fail.
func (m *JSONParseMetrics) LogFailedJSON(sessionID string, input string, inputSize int, err error) {
	preview := input
	if len(preview) > failedJSONPreviewLen {
		preview = preview[:failedJSONPreviewLen]
	}
	slog.Warn("json parse failure",
		"session_id", sessionID,
		"input_size", inputSize,
		"input_preview", preview,
		"error", err.Error(),
	)
}
