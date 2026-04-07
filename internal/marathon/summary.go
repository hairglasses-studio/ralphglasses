package marathon

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// CycleResult captures the outcome of a single marathon cycle.
type CycleResult struct {
	SessionID    string                        `json:"session_id"`
	Success      bool                          `json:"success"`
	CostUSD      float64                       `json:"cost_usd"`
	Duration     time.Duration                 `json:"duration"`
	ExitCode     int                           `json:"exit_code"`
	Productivity *session.ProductivitySnapshot `json:"productivity,omitempty"`
}

// SessionBreakdown aggregates stats for a single session within a marathon run.
type SessionBreakdown struct {
	SessionID string        `json:"session_id"`
	Cycles    int           `json:"cycles"`
	Successes int           `json:"successes"`
	Failures  int           `json:"failures"`
	TotalCost float64       `json:"total_cost_usd"`
	TotalTime time.Duration `json:"total_time"`
}

// RunSummary collects and renders marathon run statistics.
type RunSummary struct {
	mu sync.Mutex

	startedAt              time.Time
	totalCycles            int
	successes              int
	failures               int
	totalCost              float64
	totalTime              time.Duration
	productiveCycles       int
	unproductiveCycles     int
	totalProductivityScore int
	latestProductivity     *session.ProductivitySnapshot
	sessions               map[string]*SessionBreakdown
}

// NewRunSummary creates a RunSummary, recording the start time.
func NewRunSummary() *RunSummary {
	return &RunSummary{
		startedAt: time.Now(),
		sessions:  make(map[string]*SessionBreakdown),
	}
}

// RecordCycle records the outcome of a single cycle.
func (rs *RunSummary) RecordCycle(result CycleResult) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.totalCycles++
	rs.totalCost += result.CostUSD
	rs.totalTime += result.Duration

	if result.Success {
		rs.successes++
	} else {
		rs.failures++
	}
	if result.Productivity != nil {
		rs.totalProductivityScore += result.Productivity.Score
		if result.Productivity.Productive {
			rs.productiveCycles++
		} else {
			rs.unproductiveCycles++
		}
		clone := *result.Productivity
		rs.latestProductivity = &clone
	}

	sb, ok := rs.sessions[result.SessionID]
	if !ok {
		sb = &SessionBreakdown{SessionID: result.SessionID}
		rs.sessions[result.SessionID] = sb
	}
	sb.Cycles++
	sb.TotalCost += result.CostUSD
	sb.TotalTime += result.Duration
	if result.Success {
		sb.Successes++
	} else {
		sb.Failures++
	}
}

// summarySnapshot is the JSON-serialisable form of RunSummary.
type summarySnapshot struct {
	WallTime             time.Duration                 `json:"wall_time"`
	TotalCycles          int                           `json:"total_cycles"`
	Successes            int                           `json:"successes"`
	Failures             int                           `json:"failures"`
	SuccessRate          float64                       `json:"success_rate"`
	TotalCost            float64                       `json:"total_cost_usd"`
	TotalTime            time.Duration                 `json:"total_cycle_time"`
	ProductiveCycles     int                           `json:"productive_cycles"`
	UnproductiveCycles   int                           `json:"unproductive_cycles"`
	AvgProductivityScore float64                       `json:"avg_productivity_score"`
	LatestProductivity   *session.ProductivitySnapshot `json:"latest_productivity,omitempty"`
	Sessions             []*SessionBreakdown           `json:"sessions"`
}

func (rs *RunSummary) snapshot() summarySnapshot {
	var rate float64
	if rs.totalCycles > 0 {
		rate = float64(rs.successes) / float64(rs.totalCycles) * 100
	}
	var avgProductivity float64
	if totalRated := rs.productiveCycles + rs.unproductiveCycles; totalRated > 0 {
		avgProductivity = float64(rs.totalProductivityScore) / float64(totalRated)
	}
	sessions := make([]*SessionBreakdown, 0, len(rs.sessions))
	for _, sb := range rs.sessions {
		clone := *sb
		sessions = append(sessions, &clone)
	}
	return summarySnapshot{
		WallTime:             time.Since(rs.startedAt),
		TotalCycles:          rs.totalCycles,
		Successes:            rs.successes,
		Failures:             rs.failures,
		SuccessRate:          rate,
		TotalCost:            rs.totalCost,
		TotalTime:            rs.totalTime,
		ProductiveCycles:     rs.productiveCycles,
		UnproductiveCycles:   rs.unproductiveCycles,
		AvgProductivityScore: avgProductivity,
		LatestProductivity:   rs.latestProductivity,
		Sessions:             sessions,
	}
}

// Render returns a human-readable text summary.
func (rs *RunSummary) Render() string {
	rs.mu.Lock()
	snap := rs.snapshot()
	rs.mu.Unlock()

	var b strings.Builder
	b.WriteString("=== Marathon Run Summary ===\n")
	fmt.Fprintf(&b, "Wall time:    %s\n", snap.WallTime.Truncate(time.Second))
	fmt.Fprintf(&b, "Total cycles: %d\n", snap.TotalCycles)
	fmt.Fprintf(&b, "Successes:    %d\n", snap.Successes)
	fmt.Fprintf(&b, "Failures:     %d\n", snap.Failures)
	fmt.Fprintf(&b, "Success rate: %.1f%%\n", snap.SuccessRate)
	fmt.Fprintf(&b, "Total cost:   $%.4f\n", snap.TotalCost)
	fmt.Fprintf(&b, "Cycle time:   %s\n", snap.TotalTime.Truncate(time.Second))
	fmt.Fprintf(&b, "Productive:   %d\n", snap.ProductiveCycles)
	fmt.Fprintf(&b, "Unproductive:%d\n", snap.UnproductiveCycles)
	fmt.Fprintf(&b, "Avg score:    %.1f\n", snap.AvgProductivityScore)

	if len(snap.Sessions) > 0 {
		b.WriteString("\n--- Per-Session Breakdown ---\n")
		for _, sb := range snap.Sessions {
			fmt.Fprintf(&b, "  %s: %d cycles (%d ok, %d fail) $%.4f %s\n",
				sb.SessionID, sb.Cycles, sb.Successes, sb.Failures,
				sb.TotalCost, sb.TotalTime.Truncate(time.Second))
		}
	}

	return b.String()
}

// JSON returns the summary as a JSON-encoded byte slice.
func (rs *RunSummary) JSON() ([]byte, error) {
	rs.mu.Lock()
	snap := rs.snapshot()
	rs.mu.Unlock()

	return json.MarshalIndent(snap, "", "  ")
}
