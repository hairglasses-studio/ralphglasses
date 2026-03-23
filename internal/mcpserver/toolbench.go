package mcpserver

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/tracing"
)

// ToolCallEntry records a single MCP tool invocation.
type ToolCallEntry struct {
	ToolName   string    `json:"tool"`
	Timestamp  time.Time `json:"ts"`
	LatencyMs  int64     `json:"latency_ms"`
	Success    bool      `json:"ok"`
	ErrorMsg   string    `json:"error,omitempty"`
	InputSize  int       `json:"in_bytes"`
	OutputSize int       `json:"out_bytes"`
}

// ToolBenchmarkSummary aggregates metrics for a single tool.
type ToolBenchmarkSummary struct {
	ToolName     string  `json:"tool"`
	CallCount    int     `json:"calls"`
	SuccessRate  float64 `json:"success_rate_pct"`
	AvgLatencyMs float64 `json:"avg_latency_ms"`
	P50LatencyMs int64   `json:"p50_latency_ms"`
	P95LatencyMs int64   `json:"p95_latency_ms"`
	MaxLatencyMs int64   `json:"max_latency_ms"`
	ErrorCount   int     `json:"errors"`
}

// ToolRegression flags a metric that has degraded relative to a baseline.
type ToolRegression struct {
	ToolName      string  `json:"tool"`
	Metric        string  `json:"metric"`
	BaselineValue float64 `json:"baseline"`
	CurrentValue  float64 `json:"current"`
	DeltaPct      float64 `json:"delta_pct"`
	Severity      string  `json:"severity"` // "ok", "warning", "regression"
}

// ToolCallRecorder buffers tool call entries and flushes them to a JSONL file.
type ToolCallRecorder struct {
	mu       sync.Mutex
	buf      []ToolCallEntry
	filePath string                      // JSONL path; empty = in-memory only
	prom     *tracing.PrometheusRecorder // nil = no prometheus
	maxBuf   int                         // flush threshold
}

// NewToolCallRecorder creates a recorder. filePath="" means in-memory only.
// maxBuf <= 0 defaults to 50.
func NewToolCallRecorder(filePath string, prom *tracing.PrometheusRecorder, maxBuf int) *ToolCallRecorder {
	if maxBuf <= 0 {
		maxBuf = 50
	}
	return &ToolCallRecorder{
		filePath: filePath,
		prom:     prom,
		maxBuf:   maxBuf,
	}
}

// Record appends an entry. Nil-safe (no-op on nil receiver).
func (r *ToolCallRecorder) Record(e ToolCallEntry) {
	if r == nil {
		return
	}

	// Push to Prometheus (non-blocking).
	RecordToolCallPrometheus(r.prom, e.ToolName, e.LatencyMs, e.Success)

	r.mu.Lock()
	r.buf = append(r.buf, e)
	needFlush := len(r.buf) >= r.maxBuf
	r.mu.Unlock()

	if needFlush {
		r.flush()
	}
}

// Close flushes remaining buffered entries.
func (r *ToolCallRecorder) Close() {
	if r == nil {
		return
	}
	r.flush()
}

// flush writes buffered entries to the JSONL file.
func (r *ToolCallRecorder) flush() {
	if r.filePath == "" {
		return
	}

	r.mu.Lock()
	if len(r.buf) == 0 {
		r.mu.Unlock()
		return
	}
	entries := r.buf
	r.buf = nil
	r.mu.Unlock()

	dir := filepath.Dir(r.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return // best-effort
	}

	f, err := os.OpenFile(r.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()

	for _, e := range entries {
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		fmt.Fprintf(f, "%s\n", data)
	}
}

// Entries returns all in-memory entries (for testing).
func (r *ToolCallRecorder) Entries() []ToolCallEntry {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	cp := make([]ToolCallEntry, len(r.buf))
	copy(cp, r.buf)
	return cp
}

// LoadEntries reads entries from the JSONL file, filtered by since.
func (r *ToolCallRecorder) LoadEntries(since time.Time) ([]ToolCallEntry, error) {
	if r == nil || r.filePath == "" {
		return nil, nil
	}

	// Flush first so on-disk data is current.
	r.flush()

	f, err := os.Open(r.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var entries []ToolCallEntry
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e ToolCallEntry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if !e.Timestamp.Before(since) {
			entries = append(entries, e)
		}
	}
	return entries, scanner.Err()
}

// Summarize groups entries by tool and computes aggregate stats.
func Summarize(entries []ToolCallEntry) map[string]*ToolBenchmarkSummary {
	grouped := make(map[string][]ToolCallEntry)
	for _, e := range entries {
		grouped[e.ToolName] = append(grouped[e.ToolName], e)
	}

	result := make(map[string]*ToolBenchmarkSummary, len(grouped))
	for tool, toolEntries := range grouped {
		s := &ToolBenchmarkSummary{
			ToolName:  tool,
			CallCount: len(toolEntries),
		}

		var totalLatency int64
		latencies := make([]int64, 0, len(toolEntries))
		for _, e := range toolEntries {
			totalLatency += e.LatencyMs
			latencies = append(latencies, e.LatencyMs)
			if !e.Success {
				s.ErrorCount++
			}
			if e.LatencyMs > s.MaxLatencyMs {
				s.MaxLatencyMs = e.LatencyMs
			}
		}

		s.AvgLatencyMs = float64(totalLatency) / float64(len(toolEntries))
		s.SuccessRate = float64(len(toolEntries)-s.ErrorCount) / float64(len(toolEntries)) * 100

		sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
		s.P50LatencyMs = percentile(latencies, 50)
		s.P95LatencyMs = percentile(latencies, 95)

		result[tool] = s
	}
	return result
}

// percentile returns the p-th percentile from a sorted slice.
func percentile(sorted []int64, p int) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// CompareRuns detects regressions between baseline and current summaries.
func CompareRuns(baseline, current map[string]*ToolBenchmarkSummary) []ToolRegression {
	var regressions []ToolRegression

	for tool, cur := range current {
		base, ok := baseline[tool]
		if !ok {
			continue // new tool, no baseline
		}

		// P95 latency regression
		if base.P95LatencyMs > 0 {
			delta := float64(cur.P95LatencyMs-base.P95LatencyMs) / float64(base.P95LatencyMs) * 100
			sev := "ok"
			if delta >= 100 {
				sev = "regression"
			} else if delta >= 50 {
				sev = "warning"
			}
			if sev != "ok" {
				regressions = append(regressions, ToolRegression{
					ToolName:      tool,
					Metric:        "p95_latency",
					BaselineValue: float64(base.P95LatencyMs),
					CurrentValue:  float64(cur.P95LatencyMs),
					DeltaPct:      delta,
					Severity:      sev,
				})
			}
		}

		// Success rate regression
		if base.SuccessRate > 0 {
			delta := base.SuccessRate - cur.SuccessRate // positive = worse
			sev := "ok"
			if delta >= 10 {
				sev = "regression"
			} else if delta >= 5 {
				sev = "warning"
			}
			if sev != "ok" {
				regressions = append(regressions, ToolRegression{
					ToolName:      tool,
					Metric:        "success_rate",
					BaselineValue: base.SuccessRate,
					CurrentValue:  cur.SuccessRate,
					DeltaPct:      -delta, // negative = degraded
					Severity:      sev,
				})
			}
		}
	}

	sort.Slice(regressions, func(i, j int) bool {
		// regressions first, then warnings
		if regressions[i].Severity != regressions[j].Severity {
			return regressions[i].Severity == "regression"
		}
		return regressions[i].ToolName < regressions[j].ToolName
	})

	return regressions
}
