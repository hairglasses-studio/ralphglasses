package tui

import (
	"testing"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- getObservations ---

func TestGetObservationsNilCache(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.Obs = nil
	if got := m.getObservations("/tmp/repo"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetObservationsEmptyCache(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.Obs = make(map[string][]session.LoopObservation)
	if got := m.getObservations("/tmp/repo"); got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestGetObservationsHit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	obs := []session.LoopObservation{
		{RepoName: "build", TotalCostUSD: 1.0},
		{RepoName: "test", TotalCostUSD: 2.0},
	}
	m.Cache.Obs = map[string][]session.LoopObservation{"/tmp/repo": obs}
	got := m.getObservations("/tmp/repo")
	if len(got) != 2 {
		t.Errorf("expected 2 observations, got %d", len(got))
	}
}

// --- getGateEntry ---

func TestGetGateEntryNilCache(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.Gate = nil
	if got := m.getGateEntry("/tmp/repo"); got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestGetGateEntryMiss(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.Gate = make(map[string]*GateCacheEntry)
	if got := m.getGateEntry("/tmp/repo"); got != nil {
		t.Errorf("expected nil for missing key, got %v", got)
	}
}

func TestGetGateEntryHit(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	entry := &GateCacheEntry{
		Report:  &e2e.GateReport{Overall: e2e.VerdictPass},
		Summary: &e2e.Summary{TotalObservations: 5},
	}
	m.Cache.Gate = map[string]*GateCacheEntry{"/tmp/repo": entry}
	got := m.getGateEntry("/tmp/repo")
	if got == nil {
		t.Fatal("expected non-nil entry")
	}
	if got.Report.Overall != e2e.VerdictPass {
		t.Errorf("expected pass verdict, got %s", got.Report.Overall)
	}
}

// --- buildHealthData ---

func TestBuildHealthDataEmptyCaches(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	// Both caches nil/empty -> nil
	got := m.buildHealthData()
	if got != nil {
		t.Errorf("expected nil for empty caches, got %v", got)
	}
}

func TestBuildHealthDataNoGateForRepo(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
	}
	// ObsCache non-empty so we don't short-circuit, but no gate entry
	m.Cache.Obs = map[string][]session.LoopObservation{
		"/tmp/alpha": {{RepoName: "build", TotalCostUSD: 1.0}},
	}
	m.Cache.Gate = make(map[string]*GateCacheEntry)
	got := m.buildHealthData()
	// No gate entry for alpha, so data map should be empty
	if len(got) != 0 {
		t.Errorf("expected empty health data, got %d entries", len(got))
	}
}

func TestBuildHealthDataWithGateAndObs(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
		{Name: "beta", Path: "/tmp/beta"},
	}
	m.Cache.Obs = map[string][]session.LoopObservation{
		"/tmp/alpha": {
			{RepoName: "build", TotalCostUSD: 0.50},
			{RepoName: "test", TotalCostUSD: 0.75},
		},
	}
	m.Cache.Gate = map[string]*GateCacheEntry{
		"/tmp/alpha": {
			Report:  &e2e.GateReport{Overall: e2e.VerdictWarn},
			Summary: &e2e.Summary{TotalObservations: 2},
		},
	}
	got := m.buildHealthData()
	if got == nil {
		t.Fatal("expected non-nil health data")
	}
	hd, ok := got["/tmp/alpha"]
	if !ok {
		t.Fatal("expected health data for /tmp/alpha")
	}
	if hd.Verdict != "warn" {
		t.Errorf("verdict = %q, want warn", hd.Verdict)
	}
	if len(hd.CostHistory) != 2 {
		t.Errorf("cost history len = %d, want 2", len(hd.CostHistory))
	}
	if hd.CostHistory[0] != 0.50 || hd.CostHistory[1] != 0.75 {
		t.Errorf("cost history = %v, want [0.50, 0.75]", hd.CostHistory)
	}
	// beta has no gate entry, should not appear
	if _, ok := got["/tmp/beta"]; ok {
		t.Error("beta should not have health data (no gate entry)")
	}
}

func TestBuildHealthDataNilReport(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Repos = []*model.Repo{
		{Name: "alpha", Path: "/tmp/alpha"},
	}
	m.Cache.Gate = map[string]*GateCacheEntry{
		"/tmp/alpha": {Report: nil, Summary: nil},
	}
	got := m.buildHealthData()
	if len(got) != 0 {
		t.Errorf("expected empty data when Report is nil, got %d", len(got))
	}
}

// --- drainRegressionEvents ---

func TestDrainRegressionEventsNilBus(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.EventBus = nil
	// Should not panic
	m.drainRegressionEvents()
}

func TestDrainRegressionEventsNoRecentEvents(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	// Publish old regression (>3s ago)
	bus.Publish(events.Event{
		Type:      events.LoopRegression,
		Timestamp: time.Now().Add(-10 * time.Second),
		RepoName:  "old-repo",
		Data:      map[string]any{"metric": "cost"},
	})
	// Should not panic, and Notify should not have been called (no crash)
	m.drainRegressionEvents()
}

func TestDrainRegressionEventsRecentWithMetric(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	bus.Publish(events.Event{
		Type:      events.LoopRegression,
		Timestamp: time.Now(),
		RepoName:  "my-repo",
		Data:      map[string]any{"metric": "pass_rate"},
	})
	// Should not panic -- exercises the metric path
	m.drainRegressionEvents()
}

func TestDrainRegressionEventsRecentNoMetric(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	bus.Publish(events.Event{
		Type:      events.LoopRegression,
		Timestamp: time.Now(),
		RepoName:  "my-repo",
		Data:      map[string]any{},
	})
	m.drainRegressionEvents()
}

func TestDrainRegressionEventsRepoFromData(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	bus := events.NewBus(100)
	m.EventBus = bus
	bus.Publish(events.Event{
		Type:      events.LoopRegression,
		Timestamp: time.Now(),
		RepoName:  "", // empty repo name
		Data:      map[string]any{"repo": "fallback-repo", "metric": "duration"},
	})
	m.drainRegressionEvents()
}

// --- refreshGateCache ---

func TestRefreshGateCacheTTLSkip(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.GateExp = time.Now() // just refreshed
	m.Cache.Gate = map[string]*GateCacheEntry{
		"/tmp/repo": {Report: &e2e.GateReport{Overall: e2e.VerdictPass}},
	}
	m.refreshGateCache()
	// Should not have been cleared since TTL not elapsed
	if len(m.Cache.Gate) != 1 {
		t.Errorf("cache should be unchanged, got %d entries", len(m.Cache.Gate))
	}
}

func TestRefreshGateCacheRemovesStaleEntries(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.GateExp = time.Time{} // force refresh
	m.Cache.Obs = map[string][]session.LoopObservation{
		// Only /tmp/alpha has observations
		"/tmp/alpha": {
			{RepoName: "build", TotalCostUSD: 0.5, Timestamp: time.Now()},
		},
	}
	m.Cache.Gate = map[string]*GateCacheEntry{
		"/tmp/alpha": {Report: &e2e.GateReport{Overall: e2e.VerdictPass}},
		"/tmp/stale": {Report: &e2e.GateReport{Overall: e2e.VerdictFail}},
	}
	m.Cache.PrevGateVerdicts = map[string]string{
		"/tmp/alpha": "pass",
		"/tmp/stale": "fail",
	}
	m.refreshGateCache()
	// /tmp/stale should be removed since it has no observations
	if _, ok := m.Cache.Gate["/tmp/stale"]; ok {
		t.Error("stale entry should have been removed")
	}
	if _, ok := m.Cache.PrevGateVerdicts["/tmp/stale"]; ok {
		t.Error("stale verdict should have been removed")
	}
}

func TestRefreshGateCacheInitializesMaps(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.GateExp = time.Time{}
	m.Cache.Gate = nil
	m.Cache.PrevGateVerdicts = nil
	m.Cache.Obs = map[string][]session.LoopObservation{}
	m.refreshGateCache()
	if m.Cache.Gate == nil {
		t.Error("GateCache should be initialized")
	}
	if m.Cache.PrevGateVerdicts == nil {
		t.Error("PrevGateVerdicts should be initialized")
	}
}

// --- refreshObsCache ---

func TestRefreshObsCacheTTLSkip(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.ObsTime = time.Now() // just refreshed
	existing := map[string][]session.LoopObservation{
		"/tmp/x": {{RepoName: "s1"}},
	}
	m.Cache.Obs = existing
	m.refreshObsCache()
	if len(m.Cache.Obs) != 1 {
		t.Errorf("cache should be unchanged during TTL, got %d", len(m.Cache.Obs))
	}
}

func TestRefreshObsCacheInitializesMap(t *testing.T) {
	m := NewModel("/tmp/test", nil)
	m.Cache.ObsTime = time.Time{}
	m.Cache.Obs = nil
	m.Repos = nil
	m.refreshObsCache()
	if m.Cache.Obs == nil {
		t.Error("ObsCache should be initialized")
	}
}
