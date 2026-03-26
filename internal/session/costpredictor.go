package session

import (
	"encoding/json"
	"log/slog"
	"math"
	"os"
	"path/filepath"
	"sync"
)

// CostObservation records the actual cost of a task execution.
type CostObservation struct {
	TaskType string  `json:"task_type"`
	Provider string  `json:"provider"`
	CostUSD  float64 `json:"cost_usd"`
	TurnCount int    `json:"turn_count"`
}

// CostPredictor estimates the cost of future tasks based on historical observations.
type CostPredictor struct {
	mu           sync.Mutex
	observations []CostObservation
	// byKey maps "taskType:provider" to aggregated stats
	byKey    map[string]*costStats
	stateDir string
}

type costStats struct {
	count    int
	totalUSD float64
}

// NewCostPredictor creates a cost predictor, loading any persisted observations.
func NewCostPredictor(stateDir string) *CostPredictor {
	cp := &CostPredictor{
		byKey:    make(map[string]*costStats),
		stateDir: stateDir,
	}
	cp.load()
	return cp
}

// Record adds a cost observation.
func (cp *CostPredictor) Record(obs CostObservation) {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	cp.observations = append(cp.observations, obs)
	key := obs.TaskType + ":" + obs.Provider
	stats, ok := cp.byKey[key]
	if !ok {
		stats = &costStats{}
		cp.byKey[key] = stats
	}
	stats.count++
	stats.totalUSD += obs.CostUSD

	cp.save()
}

// Predict estimates the cost for a task type and provider.
// Returns the average observed cost, or a default estimate if no data.
func (cp *CostPredictor) Predict(taskType, provider string) float64 {
	cp.mu.Lock()
	defer cp.mu.Unlock()

	key := taskType + ":" + provider
	stats, ok := cp.byKey[key]
	if !ok || stats.count == 0 {
		return 1.0 // default $1 estimate
	}
	return math.Round(stats.totalUSD/float64(stats.count)*100) / 100
}

// ObservationCount returns the total number of recorded observations.
func (cp *CostPredictor) ObservationCount() int {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	return len(cp.observations)
}

func (cp *CostPredictor) load() {
	path := filepath.Join(cp.stateDir, "cost_observations.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var obs []CostObservation
	if err := json.Unmarshal(data, &obs); err == nil {
		cp.observations = obs
		for _, o := range obs {
			key := o.TaskType + ":" + o.Provider
			stats, ok := cp.byKey[key]
			if !ok {
				stats = &costStats{}
				cp.byKey[key] = stats
			}
			stats.count++
			stats.totalUSD += o.CostUSD
		}
	}
}

func (cp *CostPredictor) save() {
	if cp.stateDir == "" {
		return
	}
	if err := os.MkdirAll(cp.stateDir, 0755); err != nil {
		slog.Warn("failed to create cost predictor state dir", "dir", cp.stateDir, "error", err)
		return
	}
	data, err := json.Marshal(cp.observations)
	if err != nil {
		return
	}
	path := filepath.Join(cp.stateDir, "cost_observations.json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		slog.Warn("failed to write cost observations", "path", path, "error", err)
	}
}
