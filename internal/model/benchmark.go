package model

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BenchmarkEntry is a per-iteration log entry in benchmarks.jsonl.
type BenchmarkEntry struct {
	Timestamp    time.Time `json:"ts"`
	Loop         int       `json:"loop"`
	TaskID       string    `json:"task_id"`
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	DurationSec  int       `json:"duration_s"`
	Result       string    `json:"result"` // pass, fail, skip
	CostUSD      float64   `json:"cost_usd"`
	Model        string    `json:"model"`
	Spin         bool      `json:"spin"`
	SpinSignal   string    `json:"spin_signal,omitempty"`
}

// BenchmarkSummary is a per-session summary for benchmarks.md.
type BenchmarkSummary struct {
	SessionID           string    `json:"session_id"`
	StartedAt           time.Time `json:"started_at"`
	LoopCount           int       `json:"loop_count"`
	TotalTokens         int       `json:"total_tokens"`
	InputTokens         int       `json:"input_tokens"`
	OutputTokens        int       `json:"output_tokens"`
	WallTime            string    `json:"wall_time"`
	TasksCompleted      int       `json:"tasks_completed"`
	TasksTotal          int       `json:"tasks_total"`
	CostEstimate        float64   `json:"cost_estimate"`
	CostPerTask         float64   `json:"cost_per_task"`
	ExitReason          string    `json:"exit_reason"`
	CircuitBreakerOpens int       `json:"circuit_breaker_opens"`
	MaxNoProgress       int       `json:"max_consecutive_no_progress"`
	SpinEvents          int       `json:"spin_events"`
	Model               string    `json:"model"`
}

// AppendBenchmarkEntry appends a JSONL entry to .ralph/benchmarks.jsonl.
func AppendBenchmarkEntry(repoPath string, entry *BenchmarkEntry) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "benchmarks.jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(f, "%s\n", data)
	return err
}

// LoadBenchmarkEntries reads all entries from .ralph/benchmarks.jsonl.
func LoadBenchmarkEntries(repoPath string) ([]BenchmarkEntry, error) {
	data, err := os.ReadFile(filepath.Join(repoPath, ".ralph", "benchmarks.jsonl"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var entries []BenchmarkEntry
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if line == "" {
			continue
		}
		var e BenchmarkEntry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}

// GenerateSummary computes a BenchmarkSummary from a set of entries.
func GenerateSummary(sessionID string, entries []BenchmarkEntry) *BenchmarkSummary {
	if len(entries) == 0 {
		return &BenchmarkSummary{SessionID: sessionID}
	}

	s := &BenchmarkSummary{
		SessionID: sessionID,
		StartedAt: entries[0].Timestamp,
		LoopCount: len(entries),
	}

	for _, e := range entries {
		s.InputTokens += e.InputTokens
		s.OutputTokens += e.OutputTokens
		s.TotalTokens += e.InputTokens + e.OutputTokens
		s.CostEstimate += e.CostUSD
		if e.Result == "pass" {
			s.TasksCompleted++
		}
		s.TasksTotal++
		if e.Spin {
			s.SpinEvents++
		}
		if e.Model != "" {
			s.Model = e.Model
		}
	}

	if s.TasksCompleted > 0 {
		s.CostPerTask = s.CostEstimate / float64(s.TasksCompleted)
	}

	last := entries[len(entries)-1]
	dur := last.Timestamp.Sub(entries[0].Timestamp)
	hours := int(dur.Hours())
	mins := int(dur.Minutes()) % 60
	s.WallTime = fmt.Sprintf("%dh %dm", hours, mins)

	return s
}

// WriteBenchmarkMarkdown writes a markdown summary to .ralph/benchmarks.md.
func WriteBenchmarkMarkdown(repoPath string, summary *BenchmarkSummary) error {
	dir := filepath.Join(repoPath, ".ralph")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	completionRate := float64(0)
	if summary.TasksTotal > 0 {
		completionRate = float64(summary.TasksCompleted) / float64(summary.TasksTotal) * 100
	}

	md := fmt.Sprintf(`# Ralph Loop Benchmarks

## Session: %s

| Metric | Value |
|---|---|
| loop_count | %d |
| total_tokens | %d |
| input_tokens | %d |
| output_tokens | %d |
| wall_time | %s |
| tasks_completed | %d/%d (%.1f%%) |
| cost_estimate | $%.2f |
| cost_per_task | $%.2f |
| exit_reason | %s |
| circuit_breaker_opens | %d |
| spin_events | %d |
| model | %s |
`,
		summary.StartedAt.Format(time.RFC3339),
		summary.LoopCount,
		summary.TotalTokens,
		summary.InputTokens,
		summary.OutputTokens,
		summary.WallTime,
		summary.TasksCompleted, summary.TasksTotal, completionRate,
		summary.CostEstimate,
		summary.CostPerTask,
		summary.ExitReason,
		summary.CircuitBreakerOpens,
		summary.SpinEvents,
		summary.Model,
	)

	return os.WriteFile(filepath.Join(dir, "benchmarks.md"), []byte(md), 0644)
}

// HighScore tracks best-ever metrics across sessions.
type HighScore struct {
	Record    string `json:"record"`
	Value     string `json:"value"`
	SessionID string `json:"session_id"`
	Date      string `json:"date"`
}
