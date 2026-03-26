package fleet

import (
	"errors"
	"testing"
)

func healthyWorkers() []WorkerCandidate {
	return []WorkerCandidate{
		{ID: "w1", Provider: "claude", ActiveTasks: 3, BudgetRemaining: 100, HealthState: HealthHealthy, CostRate: 0.10},
		{ID: "w2", Provider: "gemini", ActiveTasks: 1, BudgetRemaining: 50, HealthState: HealthHealthy, CostRate: 0.05},
		{ID: "w3", Provider: "claude", ActiveTasks: 5, BudgetRemaining: 200, HealthState: HealthDegraded, CostRate: 0.10},
	}
}

func TestRoundRobinDistributes(t *testing.T) {
	r := &RoundRobinRouter{}
	workers := healthyWorkers()

	counts := map[string]int{}
	for i := 0; i < 9; i++ {
		id, err := r.SelectWorker(workers)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		counts[id]++
	}

	// 9 calls across 3 workers = 3 each
	for _, w := range workers {
		if counts[w.ID] != 3 {
			t.Errorf("worker %s got %d assignments, want 3", w.ID, counts[w.ID])
		}
	}
}

func TestLeastLoadedPicksMin(t *testing.T) {
	r := &LeastLoadedRouter{}
	workers := healthyWorkers()

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "w2" {
		t.Errorf("got %s, want w2 (least loaded with 1 task)", id)
	}
}

func TestCostOptimalPicksLowestCostWithBudget(t *testing.T) {
	r := &CostOptimalRouter{}
	workers := healthyWorkers()

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// w2 has CostRate 0.05 (lowest) and BudgetRemaining > 0
	if id != "w2" {
		t.Errorf("got %s, want w2 (lowest cost rate 0.05)", id)
	}
}

func TestCostOptimalFallsBackWhenNoBudget(t *testing.T) {
	r := &CostOptimalRouter{}
	workers := []WorkerCandidate{
		{ID: "w1", CostRate: 0.10, BudgetRemaining: 0, HealthState: HealthHealthy},
		{ID: "w2", CostRate: 0.05, BudgetRemaining: 0, HealthState: HealthHealthy},
	}

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Falls back to all eligible, w2 is cheapest
	if id != "w2" {
		t.Errorf("got %s, want w2 (lowest cost fallback)", id)
	}
}

func TestProviderAffinityPrefersCorrectProvider(t *testing.T) {
	r := &ProviderAffinityRouter{
		PreferredProvider: "gemini",
		Fallback:          &LeastLoadedRouter{},
	}
	workers := healthyWorkers()

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "w2" {
		t.Errorf("got %s, want w2 (gemini provider)", id)
	}
}

func TestProviderAffinityFallsBack(t *testing.T) {
	r := &ProviderAffinityRouter{
		PreferredProvider: "openai",
		Fallback:          &LeastLoadedRouter{},
	}
	workers := healthyWorkers()

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// No openai workers, falls back to least loaded = w2
	if id != "w2" {
		t.Errorf("got %s, want w2 (fallback to least loaded)", id)
	}
}

func TestCompositeRouterScores(t *testing.T) {
	r := &CompositeRouter{
		Weights: map[string]float64{
			"load": 10.0,
			"cost": 5.0,
		},
	}
	workers := healthyWorkers()

	id, err := r.SelectWorker(workers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// w2 has lowest load (1 task) and lowest cost (0.05), should score highest
	if id != "w2" {
		t.Errorf("got %s, want w2 (best composite score)", id)
	}
}

func TestAllRoutersReturnErrNoWorkersWhenEmpty(t *testing.T) {
	routers := map[string]Router{
		"round_robin":       &RoundRobinRouter{},
		"least_loaded":      &LeastLoadedRouter{},
		"cost_optimal":      &CostOptimalRouter{},
		"provider_affinity": &ProviderAffinityRouter{PreferredProvider: "claude"},
		"composite":         &CompositeRouter{Weights: map[string]float64{"load": 1.0}},
	}

	for name, r := range routers {
		t.Run(name+"_empty", func(t *testing.T) {
			_, err := r.SelectWorker(nil)
			if !errors.Is(err, ErrNoWorkers) {
				t.Errorf("got err=%v, want ErrNoWorkers", err)
			}
		})

		t.Run(name+"_all_unhealthy", func(t *testing.T) {
			workers := []WorkerCandidate{
				{ID: "w1", HealthState: HealthUnhealthy},
				{ID: "w2", HealthState: HealthUnhealthy},
			}
			_, err := r.SelectWorker(workers)
			if !errors.Is(err, ErrNoWorkers) {
				t.Errorf("got err=%v, want ErrNoWorkers", err)
			}
		})
	}
}

func TestFilterHealthyExcludesUnhealthy(t *testing.T) {
	workers := []WorkerCandidate{
		{ID: "w1", HealthState: HealthHealthy},
		{ID: "w2", HealthState: HealthUnhealthy},
		{ID: "w3", HealthState: HealthDegraded},
		{ID: "w4", HealthState: HealthUnhealthy},
	}

	result := filterHealthy(workers)
	if len(result) != 2 {
		t.Fatalf("got %d workers, want 2", len(result))
	}
	ids := map[string]bool{}
	for _, w := range result {
		ids[w.ID] = true
	}
	if !ids["w1"] || !ids["w3"] {
		t.Errorf("expected w1 and w3, got %v", ids)
	}
}
