package session

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

// RecordResult persists a cascade routing outcome and updates the bandit policy.
func (cr *CascadeRouter) RecordResult(result CascadeResult) {
	cr.mu.Lock()
	cr.results = append(cr.results, result)
	updateFn := cr.banditUpdate
	cr.mu.Unlock()

	cr.appendResult(result)

	// Feed the outcome back to the bandit policy if configured.
	if updateFn != nil {
		reward := 0.2 // escalated — cheap provider failed, low reward
		if !result.Escalated {
			reward = 1.0 // cheap succeeded — full reward
		}
		updateFn(string(result.UsedProvider), reward)
	}
}

// Stats computes summary statistics from all cascade results.
func (cr *CascadeRouter) Stats() CascadeStats {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	stats := CascadeStats{
		TotalDecisions: len(cr.results),
	}

	if len(cr.results) == 0 {
		return stats
	}

	var totalCheapCost float64
	for _, r := range cr.results {
		totalCheapCost += r.CheapCostUSD
		if r.Escalated {
			stats.Escalations++
		} else {
			stats.CostSavedUSD += r.CheapCostUSD
		}
	}

	stats.EscalationRate = float64(stats.Escalations) / float64(stats.TotalDecisions)
	stats.AvgCheapCost = totalCheapCost / float64(stats.TotalDecisions)

	return stats
}

// RecentResults returns the last N cascade results.
func (cr *CascadeRouter) RecentResults(limit int) []CascadeResult {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	if limit <= 0 {
		limit = 20
	}
	if len(cr.results) <= limit {
		out := make([]CascadeResult, len(cr.results))
		copy(out, cr.results)
		return out
	}
	out := make([]CascadeResult, limit)
	copy(out, cr.results[len(cr.results)-limit:])
	return out
}

func (cr *CascadeRouter) resultsPath() string {
	return filepath.Join(cr.stateDir, "cascade_results.jsonl")
}

func (cr *CascadeRouter) appendResult(r CascadeResult) {
	if cr.stateDir == "" {
		return
	}
	if err := os.MkdirAll(cr.stateDir, 0755); err != nil {
		slog.Warn("failed to create cascade state dir", "dir", cr.stateDir, "error", err)
		return
	}

	data, err := json.Marshal(r)
	if err != nil {
		return
	}
	data = append(data, '\n')

	f, err := os.OpenFile(cr.resultsPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		slog.Warn("failed to open cascade results file", "error", err)
		return
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		slog.Warn("failed to write cascade result", "error", err)
	}
}

func (cr *CascadeRouter) loadResults() {
	if cr.stateDir == "" {
		return
	}
	data, err := os.ReadFile(cr.resultsPath())
	if err != nil {
		return
	}

	var results []CascadeResult
	for _, line := range splitLines(data) {
		if len(line) == 0 {
			continue
		}
		var r CascadeResult
		if json.Unmarshal(line, &r) == nil {
			results = append(results, r)
		}
	}

	cr.mu.Lock()
	cr.results = results
	cr.mu.Unlock()
}

// logDecision records a cascade routing decision in the autonomy decision log.
func (cr *CascadeRouter) logDecision(taskType, action, rationale string, inputs map[string]any) bool {
	if cr.decisions == nil {
		return true // no decision log, allow everything
	}
	return cr.decisions.Propose(AutonomousDecision{
		Timestamp:     time.Now(),
		Category:      DecisionCascadeRoute,
		RequiredLevel: LevelAutoOptimize,
		Rationale:     rationale,
		Action:        action,
		Inputs:        inputs,
	})
}

// cascadeResultsFile is the file name for persisted cascade results.
const cascadeResultsFile = "cascade_results.jsonl"
