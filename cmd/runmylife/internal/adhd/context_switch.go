package adhd

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// ContextSnapshot represents the saved mental state when switching tasks.
type ContextSnapshot struct {
	TaskID      string   `json:"task_id,omitempty"`
	Category    string   `json:"category"`
	Notes       string   `json:"notes"`       // "where was I" freeform text
	NextStep    string   `json:"next_step"`    // what to do when resuming
	OpenFiles   []string `json:"open_files"`   // relevant files/links
	SavedAt     string   `json:"saved_at"`
}

// SwitchCostEstimate estimates the cognitive cost of switching between categories.
type SwitchCostEstimate struct {
	FromCategory string
	ToCategory   string
	CostMinutes  int
	Reason       string
}

// contextSwitchCosts maps category pairs to estimated switch cost in minutes.
// Higher cost = more cognitively distant domains.
var contextSwitchCosts = map[string]map[string]int{
	"personal":  {"arthouse": 5, "growth": 10, "partner": 5, "studio": 15, "social": 5, "wellness": 5},
	"arthouse":  {"personal": 5, "growth": 10, "partner": 8, "studio": 12, "social": 5, "wellness": 5},
	"growth":    {"personal": 10, "arthouse": 10, "partner": 10, "studio": 8, "social": 10, "wellness": 8},
	"partner":   {"personal": 5, "arthouse": 8, "growth": 10, "studio": 12, "social": 5, "wellness": 5},
	"studio":    {"personal": 15, "arthouse": 12, "growth": 8, "partner": 12, "social": 15, "wellness": 10},
	"social":    {"personal": 5, "arthouse": 5, "growth": 10, "partner": 5, "studio": 15, "wellness": 5},
	"wellness":  {"personal": 5, "arthouse": 5, "growth": 8, "partner": 5, "studio": 10, "social": 5},
}

// EstimateSwitchCost returns the estimated cognitive cost of switching categories.
func EstimateSwitchCost(from, to string) SwitchCostEstimate {
	est := SwitchCostEstimate{FromCategory: from, ToCategory: to}
	if from == to {
		est.CostMinutes = 2
		est.Reason = "same category — minimal context load"
		return est
	}
	if costs, ok := contextSwitchCosts[from]; ok {
		if cost, ok := costs[to]; ok {
			est.CostMinutes = cost
			if cost >= 12 {
				est.Reason = "high cognitive distance — consider batching tasks in the same category"
			} else if cost >= 8 {
				est.Reason = "moderate switch — take a brief break before starting"
			} else {
				est.Reason = "low switch cost — related domains"
			}
			return est
		}
	}
	est.CostMinutes = 10
	est.Reason = "unknown category pair — defaulting to moderate"
	return est
}

// SaveContext saves a context snapshot before switching tasks.
func SaveContext(ctx context.Context, db *sql.DB, snap ContextSnapshot) error {
	snap.SavedAt = time.Now().Format(time.RFC3339)
	data, err := json.Marshal(snap)
	if err != nil {
		return fmt.Errorf("marshal context snapshot: %w", err)
	}

	_, err = db.ExecContext(ctx,
		`INSERT INTO context_switches (from_task, from_category, context_snapshot)
		 VALUES (?, ?, ?)`,
		snap.TaskID, snap.Category, string(data),
	)
	return err
}

// RecordSwitch records a full context switch with cost estimation.
func RecordSwitch(ctx context.Context, db *sql.DB, fromTask, fromCategory, toTask, toCategory string, snapshot string) error {
	cost := EstimateSwitchCost(fromCategory, toCategory)
	_, err := db.ExecContext(ctx,
		`INSERT INTO context_switches (from_task, from_category, to_task, to_category, context_snapshot, switch_cost_minutes)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		fromTask, fromCategory, toTask, toCategory, snapshot, cost.CostMinutes,
	)
	return err
}

// RestoreContext loads the most recent context snapshot for a category or task.
func RestoreContext(ctx context.Context, db *sql.DB, category string) (*ContextSnapshot, error) {
	var data string
	err := db.QueryRowContext(ctx,
		`SELECT context_snapshot FROM context_switches
		 WHERE from_category = ? AND context_snapshot != ''
		 ORDER BY switched_at DESC LIMIT 1`,
		category,
	).Scan(&data)
	if err != nil {
		return nil, err
	}

	var snap ContextSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal context: %w", err)
	}
	return &snap, nil
}

// RestoreContextForTask loads the most recent snapshot involving a specific task.
func RestoreContextForTask(ctx context.Context, db *sql.DB, taskID string) (*ContextSnapshot, error) {
	var data string
	err := db.QueryRowContext(ctx,
		`SELECT context_snapshot FROM context_switches
		 WHERE from_task = ? AND context_snapshot != ''
		 ORDER BY switched_at DESC LIMIT 1`,
		taskID,
	).Scan(&data)
	if err != nil {
		return nil, err
	}

	var snap ContextSnapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return nil, fmt.Errorf("unmarshal context: %w", err)
	}
	return &snap, nil
}

// SwitchStats holds context switching metrics for a period.
type SwitchStats struct {
	TotalSwitches    int
	AvgCostMinutes   int
	TotalCostMinutes int
	MostFrequentPair string // "category→category"
}

// GetSwitchStats returns context switching statistics.
func GetSwitchStats(ctx context.Context, db *sql.DB, since string) (*SwitchStats, error) {
	stats := &SwitchStats{}

	db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(AVG(switch_cost_minutes), 0), COALESCE(SUM(switch_cost_minutes), 0)
		 FROM context_switches WHERE switched_at >= ?`, since,
	).Scan(&stats.TotalSwitches, &stats.AvgCostMinutes, &stats.TotalCostMinutes)

	db.QueryRowContext(ctx,
		`SELECT from_category || ' → ' || to_category
		 FROM context_switches WHERE switched_at >= ?
		 GROUP BY from_category, to_category
		 ORDER BY COUNT(*) DESC LIMIT 1`, since,
	).Scan(&stats.MostFrequentPair)

	return stats, nil
}
