package fleet

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ProviderRateLimit describes the rate constraints for a single provider.
type ProviderRateLimit struct {
	Provider           session.Provider
	RequestsPerMinute  int
	ConcurrentSessions int
	CostPerTaskUSD     float64 // estimated average cost per task
}

// Workload describes an incoming batch of work to plan capacity for.
type Workload struct {
	TotalTasks        int
	AvgTaskDurationS  float64            // estimated seconds per task
	MaxLatencyS       float64            // SLO: max acceptable queue wait + execution time
	ProviderMix       map[session.Provider]float64 // provider -> fraction of tasks (sums to 1.0)
	AvgTaskCostUSD    float64            // estimated cost per task if provider mix unknown
}

// CapacityPlan is the output of a capacity planning evaluation.
type CapacityPlan struct {
	RecommendedWorkers int                         `json:"recommended_workers"`
	WorkersPerProvider map[session.Provider]int     `json:"workers_per_provider"`
	EstimatedCostUSD   float64                     `json:"estimated_cost_usd"`
	EstimatedDurationS float64                     `json:"estimated_duration_seconds"`
	BudgetFeasible     bool                        `json:"budget_feasible"`
	RateLimitFeasible  bool                        `json:"rate_limit_feasible"`
	Bottleneck         string                      `json:"bottleneck,omitempty"`
	Warnings           []string                    `json:"warnings,omitempty"`
}

// CapacityPlanner models fleet capacity based on available workers, provider
// rate limits, and budget constraints. It recommends optimal fleet size for
// a given workload without duplicating existing fleet types.
type CapacityPlanner struct {
	mu         sync.RWMutex
	rateLimits map[session.Provider]ProviderRateLimit
	budgetUSD  float64
	spentUSD   float64
}

// NewCapacityPlanner creates a planner with a total budget.
func NewCapacityPlanner(budgetUSD float64) *CapacityPlanner {
	return &CapacityPlanner{
		rateLimits: make(map[session.Provider]ProviderRateLimit),
		budgetUSD:  budgetUSD,
	}
}

// SetRateLimit configures rate limits for a provider.
func (cp *CapacityPlanner) SetRateLimit(rl ProviderRateLimit) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.rateLimits[rl.Provider] = rl
}

// SetBudget updates the total budget.
func (cp *CapacityPlanner) SetBudget(budgetUSD float64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.budgetUSD = budgetUSD
}

// RecordSpend adds to the cumulative spend.
func (cp *CapacityPlanner) RecordSpend(amountUSD float64) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	cp.spentUSD += amountUSD
}

// RemainingBudget returns the unspent budget.
func (cp *CapacityPlanner) RemainingBudget() float64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()
	r := cp.budgetUSD - cp.spentUSD
	if r < 0 {
		return 0
	}
	return r
}

// Plan evaluates a workload and returns a capacity recommendation.
// It considers rate limits, budget, and latency SLO.
func (cp *CapacityPlanner) Plan(workload Workload, currentWorkers []WorkerSnapshot) CapacityPlan {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	plan := CapacityPlan{
		WorkersPerProvider: make(map[session.Provider]int),
		BudgetFeasible:     true,
		RateLimitFeasible:  true,
	}

	if workload.TotalTasks <= 0 {
		return plan
	}

	remaining := cp.budgetUSD - cp.spentUSD
	if remaining < 0 {
		remaining = 0
	}

	// Estimate total cost.
	plan.EstimatedCostUSD = cp.estimateCost(workload)
	if plan.EstimatedCostUSD > remaining {
		plan.BudgetFeasible = false
		plan.Warnings = append(plan.Warnings, "estimated cost exceeds remaining budget")
	}

	// Determine workers needed per provider from rate limits.
	providerWorkers := cp.workersFromRateLimits(workload)
	totalFromRateLimits := 0
	for p, count := range providerWorkers {
		plan.WorkersPerProvider[p] = count
		totalFromRateLimits += count
	}

	// Workers needed from latency SLO.
	workersFromLatency := cp.workersFromLatencySLO(workload)

	// Workers needed from parallelism (tasks / avg duration fit in wall-clock).
	workersFromParallelism := cp.workersFromParallelism(workload)

	// Take the max of all constraints.
	plan.RecommendedWorkers = max3(totalFromRateLimits, workersFromLatency, workersFromParallelism)
	if plan.RecommendedWorkers < 1 {
		plan.RecommendedWorkers = 1
	}

	// Budget-constrain: cap workers if budget is tight.
	if plan.EstimatedCostUSD > 0 && remaining > 0 {
		budgetCapWorkers := cp.budgetConstrainedWorkers(workload, remaining)
		if budgetCapWorkers < plan.RecommendedWorkers {
			plan.RecommendedWorkers = budgetCapWorkers
			if budgetCapWorkers < 1 {
				plan.RecommendedWorkers = 1
			}
			plan.Bottleneck = "budget"
			plan.BudgetFeasible = false
		}
	}

	// Check rate limit feasibility.
	if !cp.rateLimitsFeasible(workload, plan.RecommendedWorkers) {
		plan.RateLimitFeasible = false
		if plan.Bottleneck == "" {
			plan.Bottleneck = "rate_limit"
		}
		plan.Warnings = append(plan.Warnings, "rate limits may throttle throughput")
	}

	// Estimate wall-clock duration.
	if plan.RecommendedWorkers > 0 && workload.AvgTaskDurationS > 0 {
		tasksPerWorker := math.Ceil(float64(workload.TotalTasks) / float64(plan.RecommendedWorkers))
		plan.EstimatedDurationS = tasksPerWorker * workload.AvgTaskDurationS
	}

	// Distribute across providers if not already done by rate-limit pass.
	if len(plan.WorkersPerProvider) == 0 && len(workload.ProviderMix) > 0 {
		for p, frac := range workload.ProviderMix {
			count := int(math.Ceil(frac * float64(plan.RecommendedWorkers)))
			if count < 1 && frac > 0 {
				count = 1
			}
			plan.WorkersPerProvider[p] = count
		}
	}

	return plan
}

// PlanFromFleetState is a convenience that builds worker snapshots from a
// FleetState and plans for the given workload.
func (cp *CapacityPlanner) PlanFromFleetState(workload Workload, state FleetState) CapacityPlan {
	snapshots := make([]WorkerSnapshot, len(state.Workers))
	for i, w := range state.Workers {
		snapshots[i] = WorkerSnapshot{
			ID:             w.ID,
			Status:         w.Status,
			ActiveSessions: w.ActiveSessions,
			MaxSessions:    w.MaxSessions,
		}
	}
	return cp.Plan(workload, snapshots)
}

// estimateCost computes the estimated total cost for the workload.
func (cp *CapacityPlanner) estimateCost(w Workload) float64 {
	if len(w.ProviderMix) == 0 {
		return float64(w.TotalTasks) * w.AvgTaskCostUSD
	}

	var total float64
	for provider, frac := range w.ProviderMix {
		tasks := frac * float64(w.TotalTasks)
		costPerTask := w.AvgTaskCostUSD
		if rl, ok := cp.rateLimits[provider]; ok && rl.CostPerTaskUSD > 0 {
			costPerTask = rl.CostPerTaskUSD
		}
		total += tasks * costPerTask
	}
	return total
}

// workersFromRateLimits calculates workers needed per provider so that
// tasks can be dispatched within rate limits in reasonable time.
func (cp *CapacityPlanner) workersFromRateLimits(w Workload) map[session.Provider]int {
	result := make(map[session.Provider]int)
	if len(w.ProviderMix) == 0 {
		return result
	}

	for provider, frac := range w.ProviderMix {
		tasks := int(math.Ceil(frac * float64(w.TotalTasks)))
		if tasks == 0 {
			continue
		}

		rl, ok := cp.rateLimits[provider]
		if !ok {
			// No rate limit info; assume 1 worker needed per provider.
			result[provider] = 1
			continue
		}

		// Workers bounded by concurrent session limit.
		if rl.ConcurrentSessions > 0 {
			needed := int(math.Ceil(float64(tasks) / float64(rl.ConcurrentSessions)))
			if needed > tasks {
				needed = tasks
			}
			result[provider] = needed
		} else {
			result[provider] = 1
		}
	}
	return result
}

// workersFromLatencySLO calculates the minimum workers needed to meet
// the latency SLO, assuming tasks are processed in parallel.
func (cp *CapacityPlanner) workersFromLatencySLO(w Workload) int {
	if w.MaxLatencyS <= 0 || w.AvgTaskDurationS <= 0 {
		return 0
	}

	// Each worker can process ceil(TotalTasks / workers) tasks sequentially.
	// Wall-clock = ceil(tasks/workers) * avgDuration <= maxLatency
	// => workers >= tasks * avgDuration / maxLatency
	needed := math.Ceil(float64(w.TotalTasks) * w.AvgTaskDurationS / w.MaxLatencyS)
	return int(needed)
}

// workersFromParallelism returns the number of workers needed to run
// all tasks with reasonable concurrency (at most 1 task per worker at a time).
func (cp *CapacityPlanner) workersFromParallelism(w Workload) int {
	if w.TotalTasks <= 0 {
		return 0
	}
	// Square root heuristic: balance between 1 worker and TotalTasks workers.
	// This avoids over-provisioning for large batches.
	sqrtN := int(math.Ceil(math.Sqrt(float64(w.TotalTasks))))
	if sqrtN < 1 {
		sqrtN = 1
	}
	return sqrtN
}

// budgetConstrainedWorkers returns the max workers affordable given that
// each worker will handle roughly TotalTasks/workers tasks.
func (cp *CapacityPlanner) budgetConstrainedWorkers(w Workload, remainingUSD float64) int {
	costPerTask := w.AvgTaskCostUSD
	if costPerTask <= 0 {
		// Try to derive from rate limits.
		costPerTask = cp.avgCostPerTask(w)
	}
	if costPerTask <= 0 {
		// Can't estimate; no budget constraint.
		return math.MaxInt32
	}

	affordableTasks := int(remainingUSD / costPerTask)
	if affordableTasks <= 0 {
		return 0
	}
	if affordableTasks >= w.TotalTasks {
		// Budget covers all tasks; no constraint on workers.
		return math.MaxInt32
	}

	// If we can only afford N tasks, we don't need more workers than N.
	return affordableTasks
}

// avgCostPerTask computes a weighted average cost per task from rate limit info.
func (cp *CapacityPlanner) avgCostPerTask(w Workload) float64 {
	if len(w.ProviderMix) == 0 {
		return 0
	}
	var totalCost float64
	var totalFrac float64
	for provider, frac := range w.ProviderMix {
		if rl, ok := cp.rateLimits[provider]; ok && rl.CostPerTaskUSD > 0 {
			totalCost += frac * rl.CostPerTaskUSD
			totalFrac += frac
		}
	}
	if totalFrac <= 0 {
		return 0
	}
	return totalCost / totalFrac
}

// rateLimitsFeasible checks whether the recommended worker count can
// sustain throughput within provider rate limits.
func (cp *CapacityPlanner) rateLimitsFeasible(w Workload, workers int) bool {
	if len(w.ProviderMix) == 0 || workers <= 0 {
		return true
	}

	for provider, frac := range w.ProviderMix {
		tasks := int(math.Ceil(frac * float64(w.TotalTasks)))
		if tasks == 0 {
			continue
		}
		rl, ok := cp.rateLimits[provider]
		if !ok {
			continue
		}

		// Check RPM: tasks/minute must not exceed RPM * workers.
		if rl.RequestsPerMinute > 0 && w.AvgTaskDurationS > 0 {
			tasksPerMinutePerWorker := 60.0 / w.AvgTaskDurationS
			totalTPM := tasksPerMinutePerWorker * float64(workers)
			if totalTPM > float64(rl.RequestsPerMinute) {
				return false
			}
		}
	}
	return true
}

// ProviderCapacity returns the maximum concurrent tasks a provider can
// handle across N workers, given its rate limits.
func (cp *CapacityPlanner) ProviderCapacity(provider session.Provider, workers int) int {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	rl, ok := cp.rateLimits[provider]
	if !ok {
		return workers // no limit info; assume 1 task per worker
	}
	if rl.ConcurrentSessions > 0 {
		return rl.ConcurrentSessions * workers
	}
	return workers
}

// ProviderThroughput returns the estimated tasks/minute a provider can sustain
// across N workers.
func (cp *CapacityPlanner) ProviderThroughput(provider session.Provider, workers int) float64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	rl, ok := cp.rateLimits[provider]
	if !ok || rl.RequestsPerMinute <= 0 {
		return 0
	}
	return float64(rl.RequestsPerMinute * workers)
}

// RankedProviders returns providers sorted by cost efficiency (cheapest first).
func (cp *CapacityPlanner) RankedProviders() []ProviderRateLimit {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	ranked := make([]ProviderRateLimit, 0, len(cp.rateLimits))
	for _, rl := range cp.rateLimits {
		ranked = append(ranked, rl)
	}
	sort.Slice(ranked, func(i, j int) bool {
		ci := ranked[i].CostPerTaskUSD
		cj := ranked[j].CostPerTaskUSD
		// Zero cost sorts last (unknown).
		if ci == 0 {
			return false
		}
		if cj == 0 {
			return true
		}
		return ci < cj
	})
	return ranked
}

// OptimalMix computes the cheapest provider mix that satisfies the total
// task count and latency SLO. It greedily assigns tasks to the cheapest
// provider first, respecting each provider's concurrency limit.
func (cp *CapacityPlanner) OptimalMix(totalTasks int, maxLatencyS float64, avgTaskDurationS float64) map[session.Provider]float64 {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	if totalTasks <= 0 || len(cp.rateLimits) == 0 {
		return nil
	}

	// Sort by cost ascending.
	ranked := make([]ProviderRateLimit, 0, len(cp.rateLimits))
	for _, rl := range cp.rateLimits {
		ranked = append(ranked, rl)
	}
	sort.Slice(ranked, func(i, j int) bool {
		ci := ranked[i].CostPerTaskUSD
		cj := ranked[j].CostPerTaskUSD
		if ci == 0 {
			return false
		}
		if cj == 0 {
			return true
		}
		return ci < cj
	})

	mix := make(map[session.Provider]float64)
	remaining := totalTasks

	for _, rl := range ranked {
		if remaining <= 0 {
			break
		}

		// How many tasks can this provider handle?
		capacity := rl.ConcurrentSessions
		if capacity <= 0 {
			capacity = remaining // unlimited
		}

		// If latency SLO matters, cap by what fits in the time window.
		if maxLatencyS > 0 && avgTaskDurationS > 0 {
			tasksInWindow := int(maxLatencyS / avgTaskDurationS)
			if tasksInWindow < 1 {
				tasksInWindow = 1
			}
			maxByTime := tasksInWindow * capacity
			if maxByTime < capacity {
				capacity = maxByTime
			}
		}

		assigned := remaining
		if assigned > capacity {
			assigned = capacity
		}
		mix[rl.Provider] = float64(assigned) / float64(totalTasks)
		remaining -= assigned
	}

	return mix
}

// Summary returns a snapshot of the planner's configuration.
func (cp *CapacityPlanner) Summary() map[string]any {
	cp.mu.RLock()
	defer cp.mu.RUnlock()

	limits := make(map[string]ProviderRateLimit, len(cp.rateLimits))
	for p, rl := range cp.rateLimits {
		limits[string(p)] = rl
	}

	return map[string]any{
		"budget_usd":      cp.budgetUSD,
		"spent_usd":       cp.spentUSD,
		"remaining_usd":   cp.budgetUSD - cp.spentUSD,
		"rate_limits":     limits,
		"provider_count":  len(cp.rateLimits),
		"updated_at":      time.Now(),
	}
}

func max3(a, b, c int) int {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	return m
}
