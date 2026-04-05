package session

import (
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func newTestRegistry() (*prometheus.Registry, *JSONParseMetrics) {
	reg := prometheus.NewRegistry()
	m := NewJSONParseMetrics(reg)
	return reg, m
}

func getCounterValue(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if matchLabels(m.GetLabel(), labels) {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func getGaugeValue(t *testing.T, reg *prometheus.Registry, name string) float64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			return m.GetGauge().GetValue()
		}
	}
	return 0
}

func getHistogramCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			return m.GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func matchLabels(pairs []*dto.LabelPair, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	got := make(map[string]string, len(pairs))
	for _, p := range pairs {
		got[p.GetName()] = p.GetValue()
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

func TestJSONParseMetrics_RecordSuccess(t *testing.T) {
	reg, m := newTestRegistry()

	m.RecordParseSuccess(5 * time.Millisecond)
	m.RecordParseSuccess(10 * time.Millisecond)

	val := getCounterValue(t, reg, "ralphglasses_json_parse_total", map[string]string{"status": "success"})
	if val != 2 {
		t.Errorf("success counter = %f, want 2", val)
	}

	count := getHistogramCount(t, reg, "ralphglasses_json_parse_duration_seconds")
	if count != 2 {
		t.Errorf("histogram count = %d, want 2", count)
	}
}

func TestJSONParseMetrics_RecordFailure(t *testing.T) {
	reg, m := newTestRegistry()

	m.RecordParseFailure(1 * time.Millisecond)

	val := getCounterValue(t, reg, "ralphglasses_json_parse_total", map[string]string{"status": "failure"})
	if val != 1 {
		t.Errorf("failure counter = %f, want 1", val)
	}
}

func TestJSONParseMetrics_RecordTruncated(t *testing.T) {
	reg, m := newTestRegistry()

	m.RecordParseTruncated(2 * time.Millisecond)
	m.RecordParseTruncated(3 * time.Millisecond)
	m.RecordParseTruncated(4 * time.Millisecond)

	val := getCounterValue(t, reg, "ralphglasses_json_parse_total", map[string]string{"status": "truncated"})
	if val != 3 {
		t.Errorf("truncated counter = %f, want 3", val)
	}
}

func TestJSONParseMetrics_RetryBudgetGauge(t *testing.T) {
	reg, m := newTestRegistry()

	m.SetRetryBudgetRemaining(4)
	val := getGaugeValue(t, reg, "ralphglasses_retry_budget_remaining")
	if val != 4 {
		t.Errorf("retry budget gauge = %f, want 4", val)
	}

	m.SetRetryBudgetRemaining(0)
	val = getGaugeValue(t, reg, "ralphglasses_retry_budget_remaining")
	if val != 0 {
		t.Errorf("retry budget gauge = %f, want 0", val)
	}
}

func TestJSONParseMetrics_AllStatusLabelsInitialized(t *testing.T) {
	reg, _ := newTestRegistry()

	// All three status labels should be present even before recording events.
	for _, status := range []string{"success", "failure", "truncated"} {
		val := getCounterValue(t, reg, "ralphglasses_json_parse_total", map[string]string{"status": status})
		if val != 0 {
			t.Errorf("initial %s counter = %f, want 0", status, val)
		}
	}
}
