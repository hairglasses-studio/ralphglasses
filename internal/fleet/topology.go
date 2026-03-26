package fleet

import (
	"math"
	"sort"
	"sync"
	"time"
)

// TopologyScore holds the result of evaluating a topology strategy.
type TopologyScore struct {
	Strategy       string  `json:"strategy"`
	ThroughputEst  float64 `json:"throughput_est"`  // estimated tasks/min
	LatencyEstMs   float64 `json:"latency_est_ms"`  // estimated avg latency
	CostEfficiency float64 `json:"cost_efficiency"` // completions per dollar
	LoadBalance    float64 `json:"load_balance"`    // 0=perfect, higher=worse (stddev of load)
	AffinityHits   int     `json:"affinity_hits"`   // items placed on workers with repo history
	Total          float64 `json:"total"`           // weighted composite score (higher=better)
}

// Assignment maps a work item to a worker.
type Assignment struct {
	WorkItemID string `json:"work_item_id"`
	WorkerID   string `json:"worker_id"`
	Score      int    `json:"score"`
}

// SimulationResult holds the output of a topology optimization run.
type SimulationResult struct {
	BestStrategy string           `json:"best_strategy"`
	Scores       []TopologyScore  `json:"scores"`
	Assignments  []Assignment     `json:"assignments"`
	SimulatedAt  time.Time        `json:"simulated_at"`
	WorkerCount  int              `json:"worker_count"`
	WorkItems    int              `json:"work_items"`
}

// TopologyStrategy evaluates a fleet layout and produces a score.
type TopologyStrategy interface {
	Name() string
	// Evaluate scores this strategy given the current workers, work items, and analytics.
	Evaluate(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) TopologyScore
	// Assign returns the best worker ID for a given work item.
	Assign(worker []WorkerCandidate, item WorkItem, analytics *FleetAnalytics) (string, int)
}

// --- RoundRobinTopology ---

// RoundRobinTopology distributes items evenly across workers in order.
type RoundRobinTopology struct {
	mu      sync.Mutex
	counter int
}

func (s *RoundRobinTopology) Name() string { return "round_robin" }

func (s *RoundRobinTopology) Evaluate(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) TopologyScore {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return TopologyScore{Strategy: s.Name()}
	}

	// Simulate assignment distribution
	loads := make(map[string]int, len(healthy))
	for i := range items {
		w := healthy[i%len(healthy)]
		loads[w.ID]++
	}

	score := TopologyScore{
		Strategy:    s.Name(),
		LoadBalance: loadStdDev(loads, len(healthy)),
	}

	applyAnalyticsEstimates(&score, healthy, analytics)
	score.Total = computeComposite(score)
	return score
}

func (s *RoundRobinTopology) Assign(workers []WorkerCandidate, item WorkItem, _ *FleetAnalytics) (string, int) {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return "", -1
	}
	s.mu.Lock()
	idx := s.counter
	s.counter++
	s.mu.Unlock()
	w := healthy[idx%len(healthy)]
	return w.ID, 50 // neutral score
}

// --- AffinityTopology ---

// AffinityTopology prefers workers that have previously handled similar repos,
// using analytics completion history to identify affinity.
type AffinityTopology struct {
	// RepoHistory maps worker ID -> set of repo names previously handled.
	// Populated from analytics or externally before evaluation.
	RepoHistory map[string]map[string]bool
}

func (s *AffinityTopology) Name() string { return "affinity" }

func (s *AffinityTopology) Evaluate(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) TopologyScore {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return TopologyScore{Strategy: s.Name()}
	}

	history := s.repoHistory(analytics)
	loads := make(map[string]int, len(healthy))
	affinityHits := 0

	for _, item := range items {
		bestID := ""
		bestScore := -1
		for _, w := range healthy {
			sc := 0
			if repos, ok := history[w.ID]; ok && repos[item.RepoName] {
				sc += 20
			}
			// Prefer less loaded workers as tiebreaker
			sc += max(0, 10-(w.ActiveTasks+loads[w.ID]))
			if sc > bestScore {
				bestScore = sc
				bestID = w.ID
			}
		}
		if bestID != "" {
			loads[bestID]++
			if repos, ok := history[bestID]; ok && repos[item.RepoName] {
				affinityHits++
			}
		}
	}

	score := TopologyScore{
		Strategy:     s.Name(),
		AffinityHits: affinityHits,
		LoadBalance:  loadStdDev(loads, len(healthy)),
	}
	applyAnalyticsEstimates(&score, healthy, analytics)
	// Affinity bonus: each hit improves throughput estimate slightly
	if len(items) > 0 {
		score.ThroughputEst *= 1.0 + 0.05*float64(affinityHits)/float64(len(items))
	}
	score.Total = computeComposite(score)
	return score
}

func (s *AffinityTopology) Assign(workers []WorkerCandidate, item WorkItem, analytics *FleetAnalytics) (string, int) {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return "", -1
	}

	history := s.repoHistory(analytics)
	bestID := ""
	bestScore := -1
	for _, w := range healthy {
		sc := 50
		if repos, ok := history[w.ID]; ok && repos[item.RepoName] {
			sc += 30
		}
		sc += max(0, 10-(w.ActiveTasks))
		if sc > bestScore {
			bestScore = sc
			bestID = w.ID
		}
	}
	return bestID, bestScore
}

// repoHistory builds the repo history map from analytics if not externally set.
func (s *AffinityTopology) repoHistory(analytics *FleetAnalytics) map[string]map[string]bool {
	if s.RepoHistory != nil {
		return s.RepoHistory
	}
	// Build from analytics completions if available
	if analytics == nil {
		return nil
	}
	analytics.mu.RLock()
	defer analytics.mu.RUnlock()

	history := make(map[string]map[string]bool)
	for _, c := range analytics.completions {
		if _, ok := history[c.WorkerID]; !ok {
			history[c.WorkerID] = make(map[string]bool)
		}
		// Provider field is used as repo proxy in completions (the analytics
		// struct doesn't record repo names, so we derive from provider).
		// In real usage, RepoHistory should be populated externally.
		history[c.WorkerID][c.Provider] = true
	}
	return history
}

// --- LoadBalancedTopology ---

// LoadBalancedTopology assigns items to the least-loaded worker at each step.
type LoadBalancedTopology struct{}

func (s *LoadBalancedTopology) Name() string { return "load_balanced" }

func (s *LoadBalancedTopology) Evaluate(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) TopologyScore {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return TopologyScore{Strategy: s.Name()}
	}

	// Simulate greedy least-loaded assignment
	loads := make(map[string]int, len(healthy))
	for _, w := range healthy {
		loads[w.ID] = w.ActiveTasks
	}

	for range items {
		// Pick least loaded
		minLoad := math.MaxInt
		minID := ""
		for _, w := range healthy {
			if loads[w.ID] < minLoad {
				minLoad = loads[w.ID]
				minID = w.ID
			}
		}
		if minID != "" {
			loads[minID]++
		}
	}

	score := TopologyScore{
		Strategy:    s.Name(),
		LoadBalance: loadStdDev(loads, len(healthy)),
	}
	applyAnalyticsEstimates(&score, healthy, analytics)
	score.Total = computeComposite(score)
	return score
}

func (s *LoadBalancedTopology) Assign(workers []WorkerCandidate, item WorkItem, _ *FleetAnalytics) (string, int) {
	healthy := filterHealthy(workers)
	if len(healthy) == 0 {
		return "", -1
	}

	sort.Slice(healthy, func(i, j int) bool {
		return healthy[i].ActiveTasks < healthy[j].ActiveTasks
	})
	return healthy[0].ID, 50 + max(0, 10-healthy[0].ActiveTasks)
}

// --- TopologyOptimizer ---

// TopologyOptimizer runs simulations across multiple strategies to find the best
// fleet topology for the current workload.
type TopologyOptimizer struct {
	mu         sync.Mutex
	strategies []TopologyStrategy
	lastResult *SimulationResult
}

// NewTopologyOptimizer creates an optimizer with the default strategy set.
func NewTopologyOptimizer() *TopologyOptimizer {
	return &TopologyOptimizer{
		strategies: []TopologyStrategy{
			&RoundRobinTopology{},
			&AffinityTopology{},
			&LoadBalancedTopology{},
		},
	}
}

// AddStrategy registers an additional topology strategy for comparison.
func (o *TopologyOptimizer) AddStrategy(s TopologyStrategy) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.strategies = append(o.strategies, s)
}

// Simulate evaluates all registered strategies and returns the full result.
func (o *TopologyOptimizer) Simulate(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) SimulationResult {
	o.mu.Lock()
	strategies := make([]TopologyStrategy, len(o.strategies))
	copy(strategies, o.strategies)
	o.mu.Unlock()

	scores := make([]TopologyScore, 0, len(strategies))
	for _, s := range strategies {
		scores = append(scores, s.Evaluate(workers, items, analytics))
	}

	// Sort by total score descending
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].Total > scores[j].Total
	})

	bestName := ""
	if len(scores) > 0 {
		bestName = scores[0].Strategy
	}

	// Generate assignments using the best strategy
	var bestStrategy TopologyStrategy
	for _, s := range strategies {
		if s.Name() == bestName {
			bestStrategy = s
			break
		}
	}

	assignments := make([]Assignment, 0, len(items))
	if bestStrategy != nil {
		for _, item := range items {
			wID, sc := bestStrategy.Assign(workers, item, analytics)
			if wID != "" {
				assignments = append(assignments, Assignment{
					WorkItemID: item.ID,
					WorkerID:   wID,
					Score:      sc,
				})
			}
		}
	}

	result := SimulationResult{
		BestStrategy: bestName,
		Scores:       scores,
		Assignments:  assignments,
		SimulatedAt:  time.Now(),
		WorkerCount:  len(filterHealthy(workers)),
		WorkItems:    len(items),
	}

	o.mu.Lock()
	o.lastResult = &result
	o.mu.Unlock()

	return result
}

// LastResult returns the most recent simulation result, or nil if none has been run.
func (o *TopologyOptimizer) LastResult() *SimulationResult {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastResult
}

// OptimalAssignment runs a simulation and returns the best worker-to-item mapping.
func OptimalAssignment(workers []WorkerCandidate, items []WorkItem, analytics *FleetAnalytics) []Assignment {
	opt := NewTopologyOptimizer()
	result := opt.Simulate(workers, items, analytics)
	return result.Assignments
}

// --- Helpers ---

// loadStdDev computes the standard deviation of load distribution.
func loadStdDev(loads map[string]int, workerCount int) float64 {
	if workerCount == 0 {
		return 0
	}
	total := 0
	for _, l := range loads {
		total += l
	}
	mean := float64(total) / float64(workerCount)

	sumSq := 0.0
	for _, l := range loads {
		diff := float64(l) - mean
		sumSq += diff * diff
	}
	// Include zero-load workers not in the map
	for i := 0; i < workerCount-len(loads); i++ {
		sumSq += mean * mean
	}

	return math.Sqrt(sumSq / float64(workerCount))
}

// applyAnalyticsEstimates fills in throughput, latency, and cost estimates from analytics.
func applyAnalyticsEstimates(score *TopologyScore, workers []WorkerCandidate, analytics *FleetAnalytics) {
	if analytics == nil {
		// Default estimates when no analytics available
		score.ThroughputEst = float64(len(workers)) * 2.0 // 2 tasks/min per worker baseline
		score.LatencyEstMs = 5000                           // 5s default
		score.CostEfficiency = 1.0
		return
	}

	snap := analytics.Snapshot(30 * time.Minute)
	if snap.TotalCompletions > 0 {
		// Throughput: completions per minute in the window, scaled by worker count
		minutesInWindow := snap.Window.Minutes()
		if minutesInWindow > 0 {
			score.ThroughputEst = float64(snap.TotalCompletions) / minutesInWindow
		}
		score.LatencyEstMs = snap.LatencyP50Ms
		if snap.TotalCostUSD > 0 {
			score.CostEfficiency = float64(snap.TotalCompletions) / snap.TotalCostUSD
		}
	} else {
		score.ThroughputEst = float64(len(workers)) * 2.0
		score.LatencyEstMs = 5000
		score.CostEfficiency = 1.0
	}
}

// computeComposite calculates a weighted composite score.
// Higher is better: throughput and cost efficiency are positive,
// latency and load imbalance are negative.
func computeComposite(s TopologyScore) float64 {
	total := 0.0
	total += s.ThroughputEst * 10.0         // throughput weight
	total += s.CostEfficiency * 5.0          // cost efficiency weight
	total += float64(s.AffinityHits) * 3.0   // affinity weight
	total -= s.LoadBalance * 8.0             // penalize imbalance
	if s.LatencyEstMs > 0 {
		total -= (s.LatencyEstMs / 1000.0) * 2.0 // penalize latency (in seconds)
	}
	return total
}
