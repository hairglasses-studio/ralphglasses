// Package bench provides a benchmark results dashboard for tracking
// performance over time, comparing runs, and detecting regressions.
package bench

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// Result holds a single benchmark measurement.
type Result struct {
	Name       string    `json:"name"`
	NsPerOp    int64     `json:"ns_per_op"`
	AllocBytes int64     `json:"alloc_bytes"`
	AllocCount int64     `json:"alloc_count"`
	Timestamp  time.Time `json:"timestamp"`
	GitCommit  string    `json:"git_commit"`
	RunLabel   string    `json:"run_label"`
}

// Comparison holds the delta between two benchmark results.
type Comparison struct {
	Name          string  `json:"name"`
	BaselineNs    int64   `json:"baseline_ns"`
	CurrentNs     int64   `json:"current_ns"`
	DeltaNs       int64   `json:"delta_ns"`
	DeltaPercent  float64 `json:"delta_percent"`
	BaselineAlloc int64   `json:"baseline_alloc"`
	CurrentAlloc  int64   `json:"current_alloc"`
	Verdict       string  `json:"verdict"` // "faster", "slower", "same"
}

// TrendPoint represents one data point in a benchmark trend.
type TrendPoint struct {
	RunLabel  string    `json:"run_label"`
	NsPerOp   int64     `json:"ns_per_op"`
	Timestamp time.Time `json:"timestamp"`
	GitCommit string    `json:"git_commit"`
}

// DashboardSummary provides an overview of all benchmark results.
type DashboardSummary struct {
	TotalBenchmarks int          `json:"total_benchmarks"`
	TotalRuns       int          `json:"total_runs"`
	Fastest         *Result      `json:"fastest,omitempty"`
	Slowest         *Result      `json:"slowest,omitempty"`
	MostImproved    *Comparison  `json:"most_improved,omitempty"`
	Regressions     []Comparison `json:"regressions,omitempty"`
}

// Dashboard stores benchmark results over time and provides analysis.
type Dashboard struct {
	mu      sync.RWMutex
	Results []Result `json:"results"`
}

// NewDashboard creates an empty Dashboard.
func NewDashboard() *Dashboard {
	return &Dashboard{}
}

// AddResult records a benchmark result. The runLabel groups results from a
// single benchmark run; gitCommit is the commit hash at the time of the run.
func (d *Dashboard) AddResult(name string, nsPerOp, allocBytes, allocCount int64) {
	d.AddResultWithMeta(name, nsPerOp, allocBytes, allocCount, "", "")
}

// AddResultWithMeta records a benchmark result with optional run label and git commit.
func (d *Dashboard) AddResultWithMeta(name string, nsPerOp, allocBytes, allocCount int64, runLabel, gitCommit string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.Results = append(d.Results, Result{
		Name:       name,
		NsPerOp:    nsPerOp,
		AllocBytes: allocBytes,
		AllocCount: allocCount,
		Timestamp:  time.Now(),
		GitCommit:  gitCommit,
		RunLabel:   runLabel,
	})
}

// resultsByRun returns results grouped by run label for the given benchmark name.
func (d *Dashboard) resultsByRun(runLabel string) map[string]Result {
	out := make(map[string]Result)
	for _, r := range d.Results {
		if r.RunLabel == runLabel {
			out[r.Name] = r
		}
	}
	return out
}

// Compare compares two benchmark runs identified by their run labels.
// The threshold for "same" is 5%.
func (d *Dashboard) Compare(baseline, current string) []Comparison {
	d.mu.RLock()
	defer d.mu.RUnlock()

	baseMap := d.resultsByRun(baseline)
	currMap := d.resultsByRun(current)

	var comps []Comparison
	for name, base := range baseMap {
		curr, ok := currMap[name]
		if !ok {
			continue
		}
		delta := curr.NsPerOp - base.NsPerOp
		var pct float64
		if base.NsPerOp != 0 {
			pct = float64(delta) / float64(base.NsPerOp) * 100
		}
		verdict := "same"
		if math.Abs(pct) > 5 {
			if delta < 0 {
				verdict = "faster"
			} else {
				verdict = "slower"
			}
		}
		comps = append(comps, Comparison{
			Name:          name,
			BaselineNs:    base.NsPerOp,
			CurrentNs:     curr.NsPerOp,
			DeltaNs:       delta,
			DeltaPercent:  pct,
			BaselineAlloc: base.AllocBytes,
			CurrentAlloc:  curr.AllocBytes,
			Verdict:       verdict,
		})
	}
	sort.Slice(comps, func(i, j int) bool {
		return comps[i].Name < comps[j].Name
	})
	return comps
}

// Trend returns the last N data points for the named benchmark, ordered
// oldest-first. If last <= 0 all points are returned.
func (d *Dashboard) Trend(name string, last int) []TrendPoint {
	d.mu.RLock()
	defer d.mu.RUnlock()

	var pts []TrendPoint
	for _, r := range d.Results {
		if r.Name == name {
			pts = append(pts, TrendPoint{
				RunLabel:  r.RunLabel,
				NsPerOp:   r.NsPerOp,
				Timestamp: r.Timestamp,
				GitCommit: r.GitCommit,
			})
		}
	}
	// Sort by timestamp ascending.
	sort.Slice(pts, func(i, j int) bool {
		return pts[i].Timestamp.Before(pts[j].Timestamp)
	})
	if last > 0 && len(pts) > last {
		pts = pts[len(pts)-last:]
	}
	return pts
}

// Summary returns an overall DashboardSummary. MostImproved and Regressions
// are computed by comparing the first and last run labels found, if at least
// two distinct runs exist.
func (d *Dashboard) Summary() DashboardSummary {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if len(d.Results) == 0 {
		return DashboardSummary{}
	}

	names := make(map[string]struct{})
	runLabels := make(map[string]struct{})
	var fastest, slowest *Result
	for i := range d.Results {
		r := &d.Results[i]
		names[r.Name] = struct{}{}
		if r.RunLabel != "" {
			runLabels[r.RunLabel] = struct{}{}
		}
		if fastest == nil || r.NsPerOp < fastest.NsPerOp {
			fastest = r
		}
		if slowest == nil || r.NsPerOp > slowest.NsPerOp {
			slowest = r
		}
	}

	s := DashboardSummary{
		TotalBenchmarks: len(names),
		TotalRuns:       len(runLabels),
		Fastest:         fastest,
		Slowest:         slowest,
	}

	// Derive improvements/regressions from first vs last run label by timestamp.
	if len(runLabels) >= 2 {
		type labelTime struct {
			label string
			ts    time.Time
		}
		var lts []labelTime
		earliest := make(map[string]time.Time)
		for _, r := range d.Results {
			if r.RunLabel == "" {
				continue
			}
			if t, ok := earliest[r.RunLabel]; !ok || r.Timestamp.Before(t) {
				earliest[r.RunLabel] = r.Timestamp
			}
		}
		for l, t := range earliest {
			lts = append(lts, labelTime{l, t})
		}
		sort.Slice(lts, func(i, j int) bool { return lts[i].ts.Before(lts[j].ts) })

		first := lts[0].label
		last := lts[len(lts)-1].label

		// Temporarily unlock for Compare (which takes its own lock).
		d.mu.RUnlock()
		comps := d.Compare(first, last)
		d.mu.RLock()

		var bestImprove *Comparison
		for i := range comps {
			c := &comps[i]
			if c.Verdict == "slower" {
				s.Regressions = append(s.Regressions, *c)
			}
			if c.Verdict == "faster" {
				if bestImprove == nil || c.DeltaPercent < bestImprove.DeltaPercent {
					bestImprove = c
				}
			}
		}
		if bestImprove != nil {
			s.MostImproved = bestImprove
		}
	}

	return s
}

// FormatTable formats the most recent result per benchmark as an ASCII table.
func (d *Dashboard) FormatTable() string {
	d.mu.RLock()
	defer d.mu.RUnlock()

	// Collect the latest result per benchmark name.
	latest := make(map[string]Result)
	for _, r := range d.Results {
		if prev, ok := latest[r.Name]; !ok || r.Timestamp.After(prev.Timestamp) {
			latest[r.Name] = r
		}
	}
	if len(latest) == 0 {
		return "(no benchmark results)"
	}

	// Sort names.
	var names []string
	for n := range latest {
		names = append(names, n)
	}
	sort.Strings(names)

	// Determine column widths.
	nameW := len("Benchmark")
	for _, n := range names {
		if len(n) > nameW {
			nameW = len(n)
		}
	}

	var b strings.Builder
	hdr := fmt.Sprintf("%-*s  %12s  %12s  %12s", nameW, "Benchmark", "ns/op", "B/op", "allocs/op")
	b.WriteString(hdr)
	b.WriteByte('\n')
	b.WriteString(strings.Repeat("-", len(hdr)))
	b.WriteByte('\n')
	for _, n := range names {
		r := latest[n]
		fmt.Fprintf(&b, "%-*s  %12d  %12d  %12d\n", nameW, r.Name, r.NsPerOp, r.AllocBytes, r.AllocCount)
	}
	return b.String()
}

// SaveJSON persists the dashboard to a JSON file at path.
func (d *Dashboard) SaveJSON(path string) error {
	d.mu.RLock()
	defer d.mu.RUnlock()

	data, err := json.MarshalIndent(d.Results, "", "  ")
	if err != nil {
		return fmt.Errorf("bench: marshal: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("bench: write %s: %w", path, err)
	}
	return nil
}

// LoadJSON loads dashboard results from a JSON file at path.
func (d *Dashboard) LoadJSON(path string) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("bench: read %s: %w", path, err)
	}
	var results []Result
	if err := json.Unmarshal(data, &results); err != nil {
		return fmt.Errorf("bench: unmarshal: %w", err)
	}
	d.Results = results
	return nil
}
