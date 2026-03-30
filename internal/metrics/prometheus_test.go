package metrics

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// newTestMetrics creates a Metrics backed by a fresh registry so tests
// don't conflict with each other or the global registry.
func newTestMetrics(t *testing.T) (*Metrics, *prometheus.Registry) {
	t.Helper()
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	return m, reg
}

func TestNewMetrics_Registration(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := NewMetrics(reg)
	if m == nil {
		t.Fatal("NewMetrics returned nil")
	}

	// Gather and verify all expected metric families are present.
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	want := map[string]bool{
		"rg_sessions_total":          false,
		"rg_sessions_active":         false,
		"rg_session_duration_seconds": false,
		"rg_session_cost_usd":        false,
		"rg_iterations_total":        false,
		"rg_workers_active":          false,
		"rg_worker_queue_depth":      false,
		"rg_worker_health_score":     false,
		"rg_events_total":            false,
		"rg_event_bus_subscribers":   false,
	}

	for _, fam := range families {
		if _, ok := want[fam.GetName()]; ok {
			want[fam.GetName()] = true
		}
	}

	// Counters/histograms with labels won't appear until first observation.
	// We only require the label-free gauges to appear on gather.
	mustAppear := []string{
		"rg_workers_active",
		"rg_worker_queue_depth",
		"rg_event_bus_subscribers",
	}
	for _, name := range mustAppear {
		if !want[name] {
			t.Errorf("expected metric family %q in gather output", name)
		}
	}
}

func TestNewMetrics_DoubleRegistrationPanics(t *testing.T) {
	reg := prometheus.NewRegistry()
	_ = NewMetrics(reg)

	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on double registration, got none")
		}
	}()
	_ = NewMetrics(reg) // must panic
}

func TestRecordSessionLifecycle(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.RecordSessionStart("claude")
	m.RecordSessionStart("claude")
	m.RecordSessionStart("gemini")

	// Active should be 2 claude, 1 gemini.
	assertGaugeVec(t, reg, "rg_sessions_active", map[string]string{"provider": "claude"}, 2)
	assertGaugeVec(t, reg, "rg_sessions_active", map[string]string{"provider": "gemini"}, 1)

	// End one claude session.
	m.RecordSessionEnd("claude", 30*time.Second, 0.05, "completed")

	assertGaugeVec(t, reg, "rg_sessions_active", map[string]string{"provider": "claude"}, 1)
	assertCounterVec(t, reg, "rg_session_cost_usd", map[string]string{"provider": "claude"}, 0.05)
	assertCounterVec(t, reg, "rg_sessions_total", map[string]string{"provider": "claude", "status": "completed"}, 1)
}

func TestRecordIteration(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.RecordIteration("claude", "sess-1")
	m.RecordIteration("claude", "sess-1")
	m.RecordIteration("claude", "sess-2")

	assertCounterVec(t, reg, "rg_iterations_total",
		map[string]string{"provider": "claude", "session_id": "sess-1"}, 2)
	assertCounterVec(t, reg, "rg_iterations_total",
		map[string]string{"provider": "claude", "session_id": "sess-2"}, 1)
}

func TestRecordEvent(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.RecordEvent("session.started")
	m.RecordEvent("session.started")
	m.RecordEvent("cost.update")

	assertCounterVec(t, reg, "rg_events_total",
		map[string]string{"event_type": "session.started"}, 2)
	assertCounterVec(t, reg, "rg_events_total",
		map[string]string{"event_type": "cost.update"}, 1)
}

func TestWorkerMetrics(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.SetWorkersActive(3)
	m.SetWorkerQueueDepth(7)
	m.SetWorkerHealthScore("w-1", 0.95)
	m.SetWorkerHealthScore("w-2", 0.80)

	assertGauge(t, reg, "rg_workers_active", 3)
	assertGauge(t, reg, "rg_worker_queue_depth", 7)
	assertGaugeVec(t, reg, "rg_worker_health_score", map[string]string{"worker_id": "w-1"}, 0.95)
	assertGaugeVec(t, reg, "rg_worker_health_score", map[string]string{"worker_id": "w-2"}, 0.80)
}

func TestSetEventBusSubscribers(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.SetEventBusSubscribers(5)
	assertGauge(t, reg, "rg_event_bus_subscribers", 5)

	m.SetEventBusSubscribers(2)
	assertGauge(t, reg, "rg_event_bus_subscribers", 2)
}

func TestSessionDurationHistogram(t *testing.T) {
	m, reg := newTestMetrics(t)

	m.RecordSessionEnd("claude", 10*time.Second, 0, "completed")
	m.RecordSessionEnd("claude", 60*time.Second, 0, "completed")

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	for _, fam := range families {
		if fam.GetName() == "rg_session_duration_seconds" {
			for _, metric := range fam.GetMetric() {
				h := metric.GetHistogram()
				if h == nil {
					t.Fatal("expected histogram, got nil")
				}
				if h.GetSampleCount() != 2 {
					t.Errorf("histogram sample count = %d, want 2", h.GetSampleCount())
				}
				return
			}
		}
	}
	t.Error("rg_session_duration_seconds not found in gathered metrics")
}

func TestLabelCardinality(t *testing.T) {
	m, reg := newTestMetrics(t)

	// Create metrics with distinct label combos.
	providers := []string{"claude", "gemini", "codex"}
	for _, p := range providers {
		m.RecordSessionStart(p)
		m.RecordSessionEnd(p, time.Second, 0.01, "completed")
		m.RecordEvent("session.started")
	}

	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}

	// sessions_active should have exactly 3 label combos (one per provider).
	for _, fam := range families {
		if fam.GetName() == "rg_sessions_active" {
			if got := len(fam.GetMetric()); got != len(providers) {
				t.Errorf("rg_sessions_active label series = %d, want %d", got, len(providers))
			}
		}
	}
}

func TestHandler_ServesMetrics(t *testing.T) {
	m, _ := newTestMetrics(t)

	m.RecordSessionStart("claude")
	m.SetWorkersActive(1)

	srv := httptest.NewServer(m.Handler())
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	text := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	for _, needle := range []string{
		"rg_sessions_total",
		"rg_workers_active",
	} {
		if !strings.Contains(text, needle) {
			t.Errorf("response body missing %q", needle)
		}
	}
}

// ---------------------------------------------------------------------------
// test helpers
// ---------------------------------------------------------------------------

func gatherFamily(t *testing.T, reg *prometheus.Registry, name string) *dto.MetricFamily {
	t.Helper()
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, fam := range families {
		if fam.GetName() == name {
			return fam
		}
	}
	return nil
}

func findMetric(fam *dto.MetricFamily, labels map[string]string) *dto.Metric {
	if fam == nil {
		return nil
	}
outer:
	for _, m := range fam.GetMetric() {
		lps := m.GetLabel()
		if len(lps) != len(labels) {
			continue
		}
		for _, lp := range lps {
			v, ok := labels[lp.GetName()]
			if !ok || v != lp.GetValue() {
				continue outer
			}
		}
		return m
	}
	return nil
}

func assertGauge(t *testing.T, reg *prometheus.Registry, name string, want float64) {
	t.Helper()
	fam := gatherFamily(t, reg, name)
	if fam == nil {
		t.Fatalf("metric %q not found", name)
	}
	for _, m := range fam.GetMetric() {
		if g := m.GetGauge(); g != nil {
			if g.GetValue() != want {
				t.Errorf("%s = %f, want %f", name, g.GetValue(), want)
			}
			return
		}
	}
	t.Fatalf("%s: no gauge value found", name)
}

func assertGaugeVec(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string, want float64) {
	t.Helper()
	fam := gatherFamily(t, reg, name)
	m := findMetric(fam, labels)
	if m == nil {
		t.Fatalf("metric %s{%v} not found", name, labels)
	}
	if g := m.GetGauge(); g != nil {
		if g.GetValue() != want {
			t.Errorf("%s{%v} = %f, want %f", name, labels, g.GetValue(), want)
		}
		return
	}
	t.Fatalf("%s{%v}: no gauge value", name, labels)
}

func assertCounterVec(t *testing.T, reg *prometheus.Registry, name string, labels map[string]string, want float64) {
	t.Helper()
	fam := gatherFamily(t, reg, name)
	m := findMetric(fam, labels)
	if m == nil {
		t.Fatalf("metric %s{%v} not found", name, labels)
	}
	if c := m.GetCounter(); c != nil {
		if c.GetValue() != want {
			t.Errorf("%s{%v} = %f, want %f", name, labels, c.GetValue(), want)
		}
		return
	}
	t.Fatalf("%s{%v}: no counter value", name, labels)
}
