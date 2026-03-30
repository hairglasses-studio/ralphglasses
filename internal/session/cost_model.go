package session

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CostRecord is a single historical cost data point.
type CostRecord struct {
	Timestamp time.Time `json:"timestamp"`
	SessionID string    `json:"session_id"`
	Provider  string    `json:"provider"`
	Model     string    `json:"model"`
	CostUSD   float64   `json:"cost_usd"`
	Turns     int       `json:"turns"`
	Duration  string    `json:"duration"`
}

// CostHistory manages historical cost data for projection.
type CostHistory struct {
	Records []CostRecord `json:"records"`
	path    string
}

// NewCostHistory creates or loads a cost history from .ralph/.
func NewCostHistory(repoPath string) *CostHistory {
	ch := &CostHistory{
		path: filepath.Join(repoPath, ".ralph", "cost_history.json"),
	}
	ch.load()
	return ch
}

// Add records a new cost observation.
func (ch *CostHistory) Add(r CostRecord) {
	if r.Timestamp.IsZero() {
		r.Timestamp = time.Now()
	}
	ch.Records = append(ch.Records, r)
	// Keep last 500 records
	if len(ch.Records) > 500 {
		ch.Records = ch.Records[len(ch.Records)-500:]
	}
	ch.save()
}

// AverageCostPerTurn returns the mean cost per turn across all records.
func (ch *CostHistory) AverageCostPerTurn() float64 {
	var totalCost float64
	var totalTurns int
	for _, r := range ch.Records {
		totalCost += r.CostUSD
		totalTurns += r.Turns
	}
	if totalTurns == 0 {
		return 0
	}
	return totalCost / float64(totalTurns)
}

// AverageCostPerSession returns the mean cost per session.
func (ch *CostHistory) AverageCostPerSession() float64 {
	if len(ch.Records) == 0 {
		return 0
	}
	var total float64
	for _, r := range ch.Records {
		total += r.CostUSD
	}
	return total / float64(len(ch.Records))
}

// ProjectBudget estimates how many more sessions can be run within the given budget.
func (ch *CostHistory) ProjectBudget(remainingBudget float64) BudgetProjection {
	avg := ch.AverageCostPerSession()
	if avg <= 0 {
		return BudgetProjection{
			RemainingBudget:   remainingBudget,
			EstimatedSessions: -1, // unknown
		}
	}
	return BudgetProjection{
		RemainingBudget:   remainingBudget,
		AvgCostPerSession: avg,
		EstimatedSessions: int(math.Floor(remainingBudget / avg)),
		AvgCostPerTurn:    ch.AverageCostPerTurn(),
	}
}

// BudgetProjection is the result of a budget forecast.
type BudgetProjection struct {
	RemainingBudget   float64 `json:"remaining_budget"`
	AvgCostPerSession float64 `json:"avg_cost_per_session"`
	AvgCostPerTurn    float64 `json:"avg_cost_per_turn"`
	EstimatedSessions int     `json:"estimated_sessions"`
}

// RecentRecords returns the last N records.
func (ch *CostHistory) RecentRecords(n int) []CostRecord {
	if n <= 0 || len(ch.Records) == 0 {
		return nil
	}
	if n > len(ch.Records) {
		n = len(ch.Records)
	}
	result := make([]CostRecord, n)
	copy(result, ch.Records[len(ch.Records)-n:])
	return result
}

// ByProvider returns records filtered by provider.
func (ch *CostHistory) ByProvider(provider string) []CostRecord {
	var result []CostRecord
	for _, r := range ch.Records {
		if r.Provider == provider {
			result = append(result, r)
		}
	}
	return result
}

// SortByTime sorts records chronologically.
func (ch *CostHistory) SortByTime() {
	sort.Slice(ch.Records, func(i, j int) bool {
		return ch.Records[i].Timestamp.Before(ch.Records[j].Timestamp)
	})
}

func (ch *CostHistory) load() {
	data, err := os.ReadFile(ch.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &ch.Records)
}

func (ch *CostHistory) save() {
	dir := filepath.Dir(ch.path)
	_ = os.MkdirAll(dir, 0755)
	data, _ := json.MarshalIndent(ch.Records, "", "  ")
	_ = os.WriteFile(ch.path, data, 0644)
}
