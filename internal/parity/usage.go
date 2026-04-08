package parity

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const DefaultCLIParityUsageWindow = 30 * 24 * time.Hour

type CLIParityUsageOptions struct {
	BenchPath string
	Since     time.Time
	Until     time.Time
}

type CLIParityUsageSurface struct {
	Surface       string          `json:"surface"`
	Status        CLIParityStatus `json:"status"`
	CallCount     int             `json:"call_count"`
	LastSeen      string          `json:"last_seen,omitempty"`
	ObservedTools []string        `json:"observed_tools,omitempty"`
}

type CLIParityUsageSummary struct {
	Source                     string                  `json:"source"`
	BenchPath                  string                  `json:"bench_path"`
	TelemetryAvailable         bool                    `json:"telemetry_available"`
	LoadError                  string                  `json:"load_error,omitempty"`
	WindowStart                string                  `json:"window_start"`
	WindowEnd                  string                  `json:"window_end"`
	WindowDays                 int                     `json:"window_days"`
	TotalToolCalls             int                     `json:"total_tool_calls"`
	MatchedToolCalls           int                     `json:"matched_tool_calls"`
	MatchedCallPct             float64                 `json:"matched_call_pct"`
	ObservableSurfaces         int                     `json:"observable_surfaces"`
	ActiveObservableSurfaces   int                     `json:"active_observable_surfaces"`
	ObservableCoveragePct      float64                 `json:"observable_coverage_pct"`
	UninstrumentedCovered      []string                `json:"uninstrumented_covered_surfaces,omitempty"`
	InactiveObservableSurfaces []string                `json:"inactive_observable_surfaces,omitempty"`
	TopActiveSurfaces          []CLIParityUsageSurface `json:"top_active_surfaces,omitempty"`
}

type toolBenchEntry struct {
	ToolName  string    `json:"tool"`
	Timestamp time.Time `json:"ts"`
}

type usageAggregate struct {
	callCount     int
	lastSeen      time.Time
	observedTools map[string]struct{}
}

func DefaultCLIParityUsageOptions(scanPath string) CLIParityUsageOptions {
	until := time.Now().UTC()
	return CLIParityUsageOptions{
		BenchPath: filepath.Join(scanPath, ".ralph", "tool_benchmarks.jsonl"),
		Since:     until.Add(-DefaultCLIParityUsageWindow),
		Until:     until,
	}
}

func CLIParityUsage(opts CLIParityUsageOptions) CLIParityUsageSummary {
	until := opts.Until.UTC()
	if until.IsZero() {
		until = time.Now().UTC()
	}
	since := opts.Since.UTC()
	if since.IsZero() {
		since = until.Add(-DefaultCLIParityUsageWindow)
	}
	benchPath := opts.BenchPath
	if benchPath == "" {
		benchPath = filepath.Join(".", ".ralph", "tool_benchmarks.jsonl")
	}

	summary := CLIParityUsageSummary{
		Source:             "tool_benchmarks.jsonl",
		BenchPath:          benchPath,
		WindowStart:        since.Format(time.RFC3339),
		WindowEnd:          until.Format(time.RFC3339),
		WindowDays:         int(until.Sub(since).Hours() / 24),
		ObservableSurfaces: len(observableCLIParityEntries()),
	}

	for _, entry := range cliParityEntries {
		if entry.Status != CLIParityCommandOnlyDesign && len(entry.UsageSignals) == 0 {
			summary.UninstrumentedCovered = append(summary.UninstrumentedCovered, entry.Surface)
		}
	}
	sort.Strings(summary.UninstrumentedCovered)

	entries, available, err := loadToolBenchEntries(benchPath, since, until)
	summary.TelemetryAvailable = available
	if err != nil {
		summary.LoadError = err.Error()
		return summary
	}
	summary.TotalToolCalls = len(entries)

	signalToSurfaces := make(map[string][]int)
	for idx, entry := range cliParityEntries {
		for _, signal := range entry.UsageSignals {
			signalToSurfaces[signal] = append(signalToSurfaces[signal], idx)
		}
	}

	aggregates := make(map[int]*usageAggregate)
	for _, entry := range entries {
		matches := signalToSurfaces[entry.ToolName]
		if len(matches) == 0 {
			continue
		}
		summary.MatchedToolCalls++
		for _, idx := range matches {
			agg := aggregates[idx]
			if agg == nil {
				agg = &usageAggregate{observedTools: make(map[string]struct{})}
				aggregates[idx] = agg
			}
			agg.callCount++
			if entry.Timestamp.After(agg.lastSeen) {
				agg.lastSeen = entry.Timestamp
			}
			agg.observedTools[entry.ToolName] = struct{}{}
		}
	}

	summary.MatchedCallPct = roundPct(summary.MatchedToolCalls, summary.TotalToolCalls)

	for _, entry := range observableCLIParityEntries() {
		idx := cliParityEntryIndex(entry.Surface)
		agg := aggregates[idx]
		if agg == nil || agg.callCount == 0 {
			summary.InactiveObservableSurfaces = append(summary.InactiveObservableSurfaces, entry.Surface)
			continue
		}
		summary.ActiveObservableSurfaces++
		summary.TopActiveSurfaces = append(summary.TopActiveSurfaces, CLIParityUsageSurface{
			Surface:       entry.Surface,
			Status:        entry.Status,
			CallCount:     agg.callCount,
			LastSeen:      agg.lastSeen.UTC().Format(time.RFC3339),
			ObservedTools: sortedKeys(agg.observedTools),
		})
	}

	sort.Strings(summary.InactiveObservableSurfaces)
	sort.Slice(summary.TopActiveSurfaces, func(i, j int) bool {
		if summary.TopActiveSurfaces[i].CallCount == summary.TopActiveSurfaces[j].CallCount {
			return summary.TopActiveSurfaces[i].Surface < summary.TopActiveSurfaces[j].Surface
		}
		return summary.TopActiveSurfaces[i].CallCount > summary.TopActiveSurfaces[j].CallCount
	})

	summary.ObservableCoveragePct = roundPct(summary.ActiveObservableSurfaces, summary.ObservableSurfaces)
	return summary
}

func CLIParityDocumentWithUsage(opts CLIParityUsageOptions) map[string]any {
	doc := CLIParityDocument()
	doc["usage_telemetry"] = CLIParityUsage(opts)
	return doc
}

func cliParityEntryIndex(surface string) int {
	for idx, entry := range cliParityEntries {
		if entry.Surface == surface {
			return idx
		}
	}
	return -1
}

func loadToolBenchEntries(path string, since, until time.Time) ([]toolBenchEntry, bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	defer f.Close()

	var entries []toolBenchEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var entry toolBenchEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if !since.IsZero() && entry.Timestamp.Before(since) {
			continue
		}
		if !until.IsZero() && entry.Timestamp.After(until) {
			continue
		}
		entries = append(entries, entry)
	}
	if err := scanner.Err(); err != nil {
		return nil, true, err
	}
	return entries, true, nil
}

func sortedKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
