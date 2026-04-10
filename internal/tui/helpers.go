package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

const (
	obsCacheTTL        = 10 * time.Second
	gateCacheTTL       = 30 * time.Second
	ollamaInventoryTTL = 30 * time.Second
)

var discoverTUIOllamaInventory = session.DiscoverOllamaInventory

// GateCacheEntry wraps a gate report for TUI caching.
type GateCacheEntry struct {
	Report  *e2e.GateReport
	Summary *e2e.Summary
}

// refreshObsCache loads loop observations for all repos, gated by TTL.
func (m *Model) refreshObsCache() {
	if time.Since(m.Cache.ObsTime) < obsCacheTTL {
		return
	}
	if m.Cache.Obs == nil {
		m.Cache.Obs = make(map[string][]session.LoopObservation)
	}
	since := time.Now().Add(-24 * time.Hour)
	for _, r := range m.Repos {
		obsPath := session.ObservationPath(r.Path)
		observations, err := session.LoadObservations(obsPath, since)
		if err != nil || len(observations) == 0 {
			delete(m.Cache.Obs, r.Path)
			continue
		}
		m.Cache.Obs[r.Path] = observations
	}
	m.Cache.ObsTime = time.Now()
}

// refreshGateCache evaluates regression gates for repos with observations.
func (m *Model) refreshGateCache() {
	if time.Since(m.Cache.GateExp) < gateCacheTTL {
		return
	}
	if m.Cache.Gate == nil {
		m.Cache.Gate = make(map[string]*GateCacheEntry)
	}
	if m.Cache.PrevGateVerdicts == nil {
		m.Cache.PrevGateVerdicts = make(map[string]string)
	}

	thresholds := e2e.DefaultGateThresholds()

	for repoPath, observations := range m.Cache.Obs {
		baseline := e2e.BuildBaseline(observations, 48)
		report := e2e.EvaluateGates(observations, baseline, thresholds)
		summary := e2e.AggregateSummary(observations)
		m.Cache.Gate[repoPath] = &GateCacheEntry{
			Report:  report,
			Summary: &summary,
		}

		// Gate change notifications
		newVerdict := string(report.Overall)
		repoName := filepath.Base(repoPath)
		if prev, ok := m.Cache.PrevGateVerdicts[repoPath]; ok && prev != newVerdict {
			switch {
			case prev == "pass" && newVerdict == "warn":
				m.Notify.Show(fmt.Sprintf("⚠ %s: gate degraded to WARN", repoName), 8*time.Second)
			case newVerdict == "fail":
				m.Notify.Show(fmt.Sprintf("✗ %s: REGRESSION DETECTED", repoName), 10*time.Second)
			case prev != "pass" && newVerdict == "pass":
				m.Notify.Show(fmt.Sprintf("✓ %s: gate recovered to PASS", repoName), 5*time.Second)
			}
		}
		m.Cache.PrevGateVerdicts[repoPath] = newVerdict
	}

	// Remove stale entries for repos without observations
	for repoPath := range m.Cache.Gate {
		if _, ok := m.Cache.Obs[repoPath]; !ok {
			delete(m.Cache.Gate, repoPath)
			delete(m.Cache.PrevGateVerdicts, repoPath)
		}
	}

	m.Cache.GateExp = time.Now()
}

// refreshOllamaInventoryCache loads the shared local-model inventory on a TTL.
func (m *Model) refreshOllamaInventoryCache() {
	if time.Since(m.Cache.OllamaInvTime) < ollamaInventoryTTL {
		return
	}
	inventory := discoverTUIOllamaInventory(context.Background(), 5*time.Second)
	m.Cache.OllamaInventory = &inventory
	m.Cache.OllamaInvTime = time.Now()
}

// getObservations returns cached observations for a repo path.
func (m *Model) getObservations(repoPath string) []session.LoopObservation {
	if m.Cache.Obs == nil {
		return nil
	}
	return m.Cache.Obs[repoPath]
}

// getGateEntry returns cached gate report and summary for a repo path.
func (m *Model) getGateEntry(repoPath string) *GateCacheEntry {
	if m.Cache.Gate == nil {
		return nil
	}
	return m.Cache.Gate[repoPath]
}

// getOllamaInventory returns the cached local-model inventory used by the TUI.
func (m *Model) getOllamaInventory() *session.OllamaInventory {
	return m.Cache.OllamaInventory
}

// buildRepoDetailHealth constructs cached repo detail health data for the selected repo.
func (m *Model) buildRepoDetailHealth(repoPath string) *views.RepoDetailHealth {
	var health views.RepoDetailHealth
	var have bool

	if obs := m.getObservations(repoPath); len(obs) > 0 {
		health.Observations = obs
		have = true
	}
	if entry := m.getGateEntry(repoPath); entry != nil {
		health.GateReport = entry.Report
		have = true
	}
	if m.SessMgr != nil {
		if profiles := m.SessMgr.ProviderProfiles(); len(profiles) > 0 {
			health.ProviderProfiles = profiles
			have = true
		}
	}
	if inventory := m.getOllamaInventory(); inventory != nil {
		health.OllamaInventory = inventory
		have = true
	}
	if !have {
		return nil
	}
	return &health
}

// buildHealthData constructs per-repo health data for the overview table.
func (m *Model) buildHealthData() map[string]views.RepoHealthData {
	if len(m.Cache.Gate) == 0 && len(m.Cache.Obs) == 0 {
		return nil
	}
	data := make(map[string]views.RepoHealthData, len(m.Repos))
	thresholds := e2e.DefaultGateThresholds()
	costThreshold := thresholds.CostPerIterWarn // ratio threshold; use as sparkline color break

	for _, r := range m.Repos {
		entry := m.getGateEntry(r.Path)
		if entry == nil || entry.Report == nil {
			continue
		}
		hd := views.RepoHealthData{
			Verdict:       string(entry.Report.Overall),
			CostThreshold: costThreshold,
		}
		// Extract cost history from observations
		if obs := m.getObservations(r.Path); len(obs) > 0 {
			costs := make([]float64, 0, len(obs))
			for _, o := range obs {
				costs = append(costs, o.TotalCostUSD)
			}
			hd.CostHistory = costs
		}
		data[r.Path] = hd
	}
	return data
}

// drainRegressionEvents checks recent bus events for loop regressions and notifies.
func (m *Model) drainRegressionEvents() {
	if m.EventBus == nil {
		return
	}
	// Check events since last drain (use gate cache expiry as approximate marker)
	recent := m.EventBus.History(events.LoopRegression, 5)
	cutoff := time.Now().Add(-3 * time.Second) // only show very recent regressions
	for _, ev := range recent {
		if ev.Timestamp.Before(cutoff) {
			continue
		}
		repo := ev.RepoName
		if repo == "" {
			if r, ok := ev.Data["repo"]; ok {
				repo = fmt.Sprint(r)
			}
		}
		metric := ""
		if met, ok := ev.Data["metric"]; ok {
			metric = fmt.Sprint(met)
		}
		msg := fmt.Sprintf("✗ %s: regression on %s", repo, metric)
		if metric == "" {
			msg = fmt.Sprintf("✗ %s: loop regression detected", repo)
		}
		m.Notify.Show(msg, 8*time.Second)
	}
}
