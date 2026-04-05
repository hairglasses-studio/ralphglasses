package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
)

// Standard benchmark task suite.
var benchmarkTasks = []struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Prompt   string `json:"prompt"`
	Keywords []string
}{
	{
		Name:     "linked_list_reverse",
		Category: "code_generation",
		Prompt:   "Write a Go function that reverses a singly linked list in place. Include the ListNode struct definition.",
		Keywords: []string{"func", "ListNode", "Next", "return", "nil"},
	},
	{
		Name:     "channels_vs_mutexes",
		Category: "explanation",
		Prompt:   "Explain the difference between channels and mutexes in Go. When should you use each? Give a short example of each.",
		Keywords: []string{"channel", "mutex", "goroutine", "sync", "concurrent"},
	},
	{
		Name:     "bug_detection",
		Category: "debugging",
		Prompt:   "Find the bug in this Go code:\n\nfunc sum(nums []int) int {\n\ttotal := 0\n\tfor i := 1; i <= len(nums); i++ {\n\t\ttotal += nums[i]\n\t}\n\treturn total\n}\n\nExplain the fix.",
		Keywords: []string{"index", "bound", "i < len", "0", "off-by-one"},
	},
	{
		Name:     "table_driven_refactor",
		Category: "refactoring",
		Prompt:   "Refactor this Go test to use table-driven tests:\n\nfunc TestAdd(t *testing.T) {\n\tif Add(1,2) != 3 { t.Error(\"1+2\") }\n\tif Add(0,0) != 0 { t.Error(\"0+0\") }\n\tif Add(-1,1) != 0 { t.Error(\"-1+1\") }\n}",
		Keywords: []string{"tests", "struct", "range", "t.Run", "want"},
	},
	{
		Name:     "test_generation",
		Category: "test_writing",
		Prompt:   "Write unit tests for this Go function:\n\nfunc Clamp(val, min, max int) int {\n\tif val < min { return min }\n\tif val > max { return max }\n\treturn val\n}",
		Keywords: []string{"Test", "func", "Clamp", "t.Error", "want"},
	},
}

func (s *Server) handleProviderBenchmark(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	providersCSV := getStringArg(req, "providers")
	if providersCSV == "" {
		providersCSV = "codex,gemini,claude"
	}
	providers := strings.Split(providersCSV, ",")
	for i := range providers {
		providers[i] = strings.TrimSpace(providers[i])
	}

	iterations := int(getNumberArg(req, "iterations", 3))
	if iterations < 1 {
		iterations = 1
	}
	if iterations > 10 {
		iterations = 10
	}

	repo := getStringArg(req, "repo")
	repoPath, errRes := s.resolveRepoPath(repo)
	if errRes != nil {
		return errRes, nil
	}

	// Since we can't actually call providers in a pure MCP tool (no API keys in handler),
	// we do a simulated benchmark using prompt scoring and historical data.
	type taskResult struct {
		Task       string  `json:"task"`
		Category   string  `json:"category"`
		QualityPct float64 `json:"quality_pct"`
	}

	type providerSummary struct {
		Provider           string       `json:"provider"`
		AvgQualityPct      float64      `json:"avg_quality_pct"`
		AvgCostPerTask     float64      `json:"avg_cost_per_task"`
		EstLatencyMs       int          `json:"est_latency_ms"`
		CostEfficiency     float64      `json:"cost_efficiency"`
		Tasks              []taskResult `json:"tasks"`
		Recommendation     string       `json:"recommendation"`
		HistoricalDataUsed bool         `json:"historical_data_used"`
	}

	var summaries []providerSummary

	// Rate cards for cost estimation (per 1K output tokens).
	ratecards := map[string]float64{
		"claude": 0.015, // opus-class
		"gemini": 0.005, // pro-class
		"codex":  0.010, // gpt-4o-class
	}

	// Latency estimates (P50 ms per task).
	latencyEstimates := map[string]int{
		"claude": 8000, // ~8s per task (opus)
		"gemini": 3000, // ~3s per task (pro)
		"codex":  5000, // ~5s per task (gpt-4o)
	}

	// Try to load historical cost data from observations.
	historicalCosts := s.loadHistoricalProviderCosts(repoPath)

	for _, provider := range providers {
		var tasks []taskResult
		var totalQuality float64

		for _, bt := range benchmarkTasks {
			quality := scorePromptQuality(bt.Prompt, bt.Keywords, provider)
			tasks = append(tasks, taskResult{
				Task:       bt.Name,
				Category:   bt.Category,
				QualityPct: math.Round(quality*100) / 100,
			})
			totalQuality += quality
		}

		avgQuality := totalQuality / float64(len(benchmarkTasks))

		// Use historical cost if available, otherwise use rate cards.
		costPerTask := ratecards[provider] * 0.5
		historicalUsed := false
		if hc, ok := historicalCosts[provider]; ok && hc > 0 {
			costPerTask = hc
			historicalUsed = true
		}
		if costPerTask == 0 {
			costPerTask = 0.01
		}

		latency := latencyEstimates[provider]
		if latency == 0 {
			latency = 5000
		}

		// Cost efficiency: quality per dollar (higher = better).
		costEfficiency := avgQuality / (costPerTask * 100)

		rec := "general purpose"
		switch provider {
		case "claude":
			rec = "best for complex reasoning and code review"
		case "gemini":
			rec = "best for cost-efficient bulk tasks"
		case "codex":
			rec = "best for code generation and completion"
		}

		summaries = append(summaries, providerSummary{
			Provider:           provider,
			AvgQualityPct:      math.Round(avgQuality*100) / 100,
			AvgCostPerTask:     math.Round(costPerTask*10000) / 10000,
			EstLatencyMs:       latency,
			CostEfficiency:     math.Round(costEfficiency*100) / 100,
			Tasks:              tasks,
			Recommendation:     rec,
			HistoricalDataUsed: historicalUsed,
		})
	}

	// Determine winner.
	var winner string
	bestScore := -1.0
	for _, s := range summaries {
		// Quality-weighted cost efficiency.
		score := s.AvgQualityPct / (s.AvgCostPerTask * 100)
		if score > bestScore {
			bestScore = score
			winner = s.Provider
		}
	}

	benchmarkID := fmt.Sprintf("bench-%d", time.Now().Unix())

	// Save results.
	benchDir := filepath.Join(repoPath, ".ralph", "benchmarks")
	os.MkdirAll(benchDir, 0o755)
	resultData := map[string]any{
		"benchmark_id": benchmarkID,
		"providers":    summaries,
		"winner":       winner,
		"iterations":   iterations,
		"task_count":   len(benchmarkTasks),
		"created_at":   time.Now().UTC().Format(time.RFC3339),
		"note":         "simulated benchmark using prompt scoring heuristics; run with live API for real measurements",
	}
	if data, err := json.MarshalIndent(resultData, "", "  "); err == nil {
		os.WriteFile(filepath.Join(benchDir, benchmarkID+".json"), data, 0o644)
	}

	return jsonResult(resultData), nil
}

// loadHistoricalProviderCosts reads cost observations and computes per-provider averages.
func (s *Server) loadHistoricalProviderCosts(repoPath string) map[string]float64 {
	result := make(map[string]float64)

	obsPath := filepath.Join(repoPath, ".ralph", "cost_observations.json")
	data, err := os.ReadFile(obsPath)
	if err != nil {
		return result
	}

	var observations []struct {
		Provider string  `json:"provider"`
		Cost     float64 `json:"cost"`
	}
	if err := json.Unmarshal(data, &observations); err != nil {
		return result
	}

	counts := make(map[string]int)
	totals := make(map[string]float64)
	for _, o := range observations {
		if o.Provider != "" && o.Cost > 0 {
			counts[o.Provider]++
			totals[o.Provider] += o.Cost
		}
	}
	for p, total := range totals {
		result[p] = total / float64(counts[p])
	}
	return result
}

// scorePromptQuality estimates quality for a provider on a task.
// This is a heuristic placeholder; real benchmarks call the API.
func scorePromptQuality(prompt string, keywords []string, provider string) float64 {
	// Base quality by provider (empirical estimates).
	base := map[string]float64{
		"claude": 0.92,
		"gemini": 0.85,
		"codex":  0.88,
	}
	b := base[provider]
	if b == 0 {
		b = 0.80
	}

	// Category bonus: Claude excels at explanation, Codex at code.
	if strings.Contains(prompt, "Explain") && provider == "claude" {
		b += 0.03
	}
	if strings.Contains(prompt, "Write") && provider == "codex" {
		b += 0.02
	}
	if len(prompt) > 200 && provider == "claude" {
		b += 0.02 // Claude handles longer prompts better
	}

	if b > 1.0 {
		b = 1.0
	}
	return b * 100 // return as percentage
}
