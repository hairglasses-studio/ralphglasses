package tui

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/e2e"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
	"github.com/hairglasses-studio/ralphglasses/internal/tui/views"
)

const (
	obsCacheTTL  = 10 * time.Second
	gateCacheTTL = 30 * time.Second
)

// GateCacheEntry wraps a gate report for TUI caching.
type GateCacheEntry struct {
	Report  *e2e.GateReport
	Summary *e2e.Summary
}

// refreshObsCache loads loop observations for all repos, gated by TTL.
func (m *Model) refreshObsCache() {
	if time.Since(m.ObsCacheTime) < obsCacheTTL {
		return
	}
	if m.ObsCache == nil {
		m.ObsCache = make(map[string][]session.LoopObservation)
	}
	since := time.Now().Add(-24 * time.Hour)
	for _, r := range m.Repos {
		obsPath := session.ObservationPath(r.Path)
		observations, err := session.LoadObservations(obsPath, since)
		if err != nil || len(observations) == 0 {
			delete(m.ObsCache, r.Path)
			continue
		}
		m.ObsCache[r.Path] = observations
	}
	m.ObsCacheTime = time.Now()
}

// refreshGateCache evaluates regression gates for repos with observations.
func (m *Model) refreshGateCache() {
	if time.Since(m.GateCacheExp) < gateCacheTTL {
		return
	}
	if m.GateCache == nil {
		m.GateCache = make(map[string]*GateCacheEntry)
	}
	if m.PrevGateVerdicts == nil {
		m.PrevGateVerdicts = make(map[string]string)
	}

	thresholds := e2e.DefaultGateThresholds()

	for repoPath, observations := range m.ObsCache {
		baseline := e2e.BuildBaseline(observations, 48)
		report := e2e.EvaluateGates(observations, baseline, thresholds)
		summary := e2e.AggregateSummary(observations)
		m.GateCache[repoPath] = &GateCacheEntry{
			Report:  report,
			Summary: &summary,
		}

		// Gate change notifications
		newVerdict := string(report.Overall)
		repoName := filepath.Base(repoPath)
		if prev, ok := m.PrevGateVerdicts[repoPath]; ok && prev != newVerdict {
			switch {
			case prev == "pass" && newVerdict == "warn":
				m.Notify.Show(fmt.Sprintf("⚠ %s: gate degraded to WARN", repoName), 8*time.Second)
			case newVerdict == "fail":
				m.Notify.Show(fmt.Sprintf("✗ %s: REGRESSION DETECTED", repoName), 10*time.Second)
			case prev != "pass" && newVerdict == "pass":
				m.Notify.Show(fmt.Sprintf("✓ %s: gate recovered to PASS", repoName), 5*time.Second)
			}
		}
		m.PrevGateVerdicts[repoPath] = newVerdict
	}

	// Remove stale entries for repos without observations
	for repoPath := range m.GateCache {
		if _, ok := m.ObsCache[repoPath]; !ok {
			delete(m.GateCache, repoPath)
			delete(m.PrevGateVerdicts, repoPath)
		}
	}

	m.GateCacheExp = time.Now()
}

// getObservations returns cached observations for a repo path.
func (m *Model) getObservations(repoPath string) []session.LoopObservation {
	if m.ObsCache == nil {
		return nil
	}
	return m.ObsCache[repoPath]
}

// getGateEntry returns cached gate report and summary for a repo path.
func (m *Model) getGateEntry(repoPath string) *GateCacheEntry {
	if m.GateCache == nil {
		return nil
	}
	return m.GateCache[repoPath]
}

// buildHealthData constructs per-repo health data for the overview table.
func (m *Model) buildHealthData() map[string]views.RepoHealthData {
	if len(m.GateCache) == 0 && len(m.ObsCache) == 0 {
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
