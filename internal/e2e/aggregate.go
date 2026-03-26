package e2e

import (
	"fmt"
	"sort"
	"strings"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Summary holds aggregated metrics across observations.
type Summary struct {
	TotalObservations int                      `json:"total_observations"`
	PerScenario       map[string]*ScenarioStat `json:"per_scenario"`
	CrossScenario     *CrossStat               `json:"cross_scenario"`
}

// ScenarioStat holds per-scenario aggregate metrics.
type ScenarioStat struct {
	Name           string  `json:"name"`
	Count          int     `json:"count"`
	AvgCostUSD     float64 `json:"avg_cost_usd"`
	AvgLatencyMs   float64 `json:"avg_latency_ms"`
	CompletionRate float64 `json:"completion_rate"`
	VerifyPassRate float64 `json:"verify_pass_rate"`
	AvgFilesChanged float64 `json:"avg_files_changed"`
	AvgLinesAdded   float64 `json:"avg_lines_added"`
}

// CrossStat holds cross-scenario aggregate metrics.
type CrossStat struct {
	AvgCostUSD     float64            `json:"avg_cost_usd"`
	AvgLatencyMs   float64            `json:"avg_latency_ms"`
	CompletionRate float64            `json:"completion_rate"`
	VerifyPassRate float64            `json:"verify_pass_rate"`
	TaskTypeDist   map[string]int     `json:"task_type_distribution"`
}

// AggregateSummary computes per-scenario and cross-scenario statistics.
func AggregateSummary(observations []session.LoopObservation) Summary {
	s := Summary{
		TotalObservations: len(observations),
		PerScenario:       make(map[string]*ScenarioStat),
	}

	if len(observations) == 0 {
		return s
	}

	// Group by scenario (TaskTitle)
	type accum struct {
		costs     []float64
		latencies []float64
		completed int
		verified  int
		files     []float64
		lines     []float64
	}
	groups := make(map[string]*accum)
	taskTypes := make(map[string]int)

	for _, obs := range observations {
		key := obs.TaskTitle
		if key == "" {
			key = "unknown"
		}
		g, ok := groups[key]
		if !ok {
			g = &accum{}
			groups[key] = g
		}
		g.costs = append(g.costs, obs.TotalCostUSD)
		g.latencies = append(g.latencies, float64(obs.TotalLatencyMs))
		g.files = append(g.files, float64(obs.FilesChanged))
		g.lines = append(g.lines, float64(obs.LinesAdded))
		if obs.Status == "idle" {
			g.completed++
		}
		if obs.VerifyPassed || (obs.Status != "failed" && obs.Error == "") {
			g.verified++
		}
		if obs.TaskType != "" {
			taskTypes[obs.TaskType]++
		}
	}

	var totalCost, totalLatency float64
	var totalCompleted, totalVerified int

	for name, g := range groups {
		n := float64(len(g.costs))
		stat := &ScenarioStat{
			Name:           name,
			Count:          len(g.costs),
			AvgCostUSD:     sum(g.costs) / n,
			AvgLatencyMs:   sum(g.latencies) / n,
			CompletionRate: float64(g.completed) / n,
			VerifyPassRate: float64(g.verified) / n,
			AvgFilesChanged: sum(g.files) / n,
			AvgLinesAdded:   sum(g.lines) / n,
		}
		s.PerScenario[name] = stat

		totalCost += sum(g.costs)
		totalLatency += sum(g.latencies)
		totalCompleted += g.completed
		totalVerified += g.verified
	}

	n := float64(len(observations))
	s.CrossScenario = &CrossStat{
		AvgCostUSD:     totalCost / n,
		AvgLatencyMs:   totalLatency / n,
		CompletionRate: float64(totalCompleted) / n,
		VerifyPassRate: float64(totalVerified) / n,
		TaskTypeDist:   taskTypes,
	}

	return s
}

// FormatMarkdown renders a summary as a markdown table.
func FormatMarkdown(s Summary) string {
	var b strings.Builder

	b.WriteString(fmt.Sprintf("# E2E Summary (%d observations)\n\n", s.TotalObservations))

	if s.CrossScenario != nil {
		b.WriteString("## Cross-Scenario\n\n")
		b.WriteString("| Metric | Value |\n|--------|-------|\n")
		b.WriteString(fmt.Sprintf("| Avg Cost | $%.3f |\n", s.CrossScenario.AvgCostUSD))
		b.WriteString(fmt.Sprintf("| Avg Latency | %.0fms |\n", s.CrossScenario.AvgLatencyMs))
		b.WriteString(fmt.Sprintf("| Completion Rate | %.1f%% |\n", s.CrossScenario.CompletionRate*100))
		b.WriteString(fmt.Sprintf("| Verify Pass Rate | %.1f%% |\n", s.CrossScenario.VerifyPassRate*100))
		b.WriteString("\n")
	}

	if len(s.PerScenario) > 0 {
		b.WriteString("## Per-Scenario\n\n")
		b.WriteString("| Scenario | N | Avg Cost | Avg Latency | Completion | Verify |\n")
		b.WriteString("|----------|---|----------|-------------|------------|--------|\n")

		// Sort by name for determinism
		names := make([]string, 0, len(s.PerScenario))
		for name := range s.PerScenario {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			stat := s.PerScenario[name]
			b.WriteString(fmt.Sprintf("| %s | %d | $%.3f | %.0fms | %.0f%% | %.0f%% |\n",
				stat.Name, stat.Count, stat.AvgCostUSD, stat.AvgLatencyMs,
				stat.CompletionRate*100, stat.VerifyPassRate*100))
		}
	}

	return b.String()
}

func sum(vals []float64) float64 {
	var s float64
	for _, v := range vals {
		s += v
	}
	return s
}
