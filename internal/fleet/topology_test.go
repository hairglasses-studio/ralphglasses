package fleet

import (
	"testing"
	"time"
)

func testWorkers() []WorkerCandidate {
	return []WorkerCandidate{
		{ID: "w1", Provider: "claude", ActiveTasks: 2, HealthState: HealthHealthy, CostRate: 0.05, BudgetRemaining: 10},
		{ID: "w2", Provider: "gemini", ActiveTasks: 0, HealthState: HealthHealthy, CostRate: 0.02, BudgetRemaining: 10},
		{ID: "w3", Provider: "claude", ActiveTasks: 5, HealthState: HealthHealthy, CostRate: 0.05, BudgetRemaining: 10},
	}
}

func testWorkItems() []WorkItem {
	return []WorkItem{
		{ID: "item1", RepoName: "repo-a", Priority: 2},
		{ID: "item2", RepoName: "repo-b", Priority: 1},
		{ID: "item3", RepoName: "repo-a", Priority: 3},
		{ID: "item4", RepoName: "repo-c", Priority: 1},
		{ID: "item5", RepoName: "repo-b", Priority: 2},
		{ID: "item6", RepoName: "repo-a", Priority: 1},
	}
}

func testAnalytics() *FleetAnalytics {
	fa := NewFleetAnalytics(1000, time.Hour)
	now := time.Now()
	fa.recordCompletionAt(now.Add(-5*time.Minute), "w1", "claude", 3*time.Second, 0.05)
	fa.recordCompletionAt(now.Add(-4*time.Minute), "w2", "gemini", 2*time.Second, 0.02)
	fa.recordCompletionAt(now.Add(-3*time.Minute), "w1", "claude", 4*time.Second, 0.06)
	fa.recordCompletionAt(now.Add(-2*time.Minute), "w3", "claude", 5*time.Second, 0.05)
	fa.recordCompletionAt(now.Add(-1*time.Minute), "w2", "gemini", 2*time.Second, 0.01)
	return fa
}

func TestRoundRobinTopology_Evaluate(t *testing.T) {
	s := &RoundRobinTopology{}
	workers := testWorkers()
	items := testWorkItems()

	score := s.Evaluate(workers, items, nil)
	if score.Strategy != "round_robin" {
		t.Errorf("expected strategy name round_robin, got %s", score.Strategy)
	}
	if score.Total == 0 {
		t.Error("expected non-zero total score")
	}
	// With 6 items and 3 workers, load should be perfectly balanced
	if score.LoadBalance != 0 {
		t.Errorf("expected perfect load balance (0), got %f", score.LoadBalance)
	}
}

func TestRoundRobinTopology_Assign(t *testing.T) {
	s := &RoundRobinTopology{}
	workers := testWorkers()
	item := WorkItem{ID: "item1", RepoName: "repo-a"}

	id1, sc1 := s.Assign(workers, item, nil)
	if id1 == "" {
		t.Fatal("expected a worker assignment")
	}
	if sc1 < 0 {
		t.Errorf("expected non-negative score, got %d", sc1)
	}

	id2, _ := s.Assign(workers, item, nil)
	if id1 == id2 {
		t.Error("expected round-robin to rotate to a different worker")
	}
}

func TestRoundRobinTopology_NoWorkers(t *testing.T) {
	s := &RoundRobinTopology{}
	score := s.Evaluate(nil, testWorkItems(), nil)
	if score.Total != 0 {
		t.Errorf("expected zero score with no workers, got %f", score.Total)
	}

	id, sc := s.Assign(nil, WorkItem{}, nil)
	if id != "" || sc != -1 {
		t.Errorf("expected empty assignment with no workers, got id=%s score=%d", id, sc)
	}
}

func TestAffinityTopology_Evaluate(t *testing.T) {
	s := &AffinityTopology{
		RepoHistory: map[string]map[string]bool{
			"w1": {"repo-a": true},
			"w2": {"repo-b": true},
			"w3": {"repo-c": true},
		},
	}
	workers := testWorkers()
	items := testWorkItems()

	score := s.Evaluate(workers, items, nil)
	if score.Strategy != "affinity" {
		t.Errorf("expected strategy name affinity, got %s", score.Strategy)
	}
	// Items: repo-a x3, repo-b x2, repo-c x1 -> w1 has repo-a, w2 has repo-b, w3 has repo-c
	// Should get at least some affinity hits
	if score.AffinityHits == 0 {
		t.Error("expected some affinity hits with matching repo history")
	}
	if score.Total == 0 {
		t.Error("expected non-zero total score")
	}
}

func TestAffinityTopology_Assign(t *testing.T) {
	s := &AffinityTopology{
		RepoHistory: map[string]map[string]bool{
			"w1": {"repo-a": true},
			"w2": {"repo-b": true},
		},
	}
	workers := testWorkers()

	// Assign repo-a item -- should prefer w1
	id, sc := s.Assign(workers, WorkItem{ID: "x", RepoName: "repo-a"}, nil)
	if id != "w1" {
		t.Errorf("expected w1 for repo-a affinity, got %s", id)
	}
	if sc <= 50 {
		t.Errorf("expected affinity bonus to raise score above 50, got %d", sc)
	}

	// Assign repo-b item -- should prefer w2
	id, _ = s.Assign(workers, WorkItem{ID: "y", RepoName: "repo-b"}, nil)
	if id != "w2" {
		t.Errorf("expected w2 for repo-b affinity, got %s", id)
	}
}

func TestAffinityTopology_NoHistory(t *testing.T) {
	s := &AffinityTopology{}
	workers := testWorkers()
	items := testWorkItems()

	score := s.Evaluate(workers, items, nil)
	if score.AffinityHits != 0 {
		t.Errorf("expected 0 affinity hits with no history, got %d", score.AffinityHits)
	}
}

func TestLoadBalancedTopology_Evaluate(t *testing.T) {
	s := &LoadBalancedTopology{}
	workers := testWorkers() // w1=2, w2=0, w3=5
	items := testWorkItems()

	score := s.Evaluate(workers, items, nil)
	if score.Strategy != "load_balanced" {
		t.Errorf("expected strategy name load_balanced, got %s", score.Strategy)
	}
	if score.Total == 0 {
		t.Error("expected non-zero total score")
	}
}

func TestLoadBalancedTopology_PrefersLeastLoaded(t *testing.T) {
	s := &LoadBalancedTopology{}
	workers := []WorkerCandidate{
		{ID: "w1", ActiveTasks: 10, HealthState: HealthHealthy},
		{ID: "w2", ActiveTasks: 0, HealthState: HealthHealthy},
		{ID: "w3", ActiveTasks: 5, HealthState: HealthHealthy},
	}

	id, _ := s.Assign(workers, WorkItem{ID: "x"}, nil)
	if id != "w2" {
		t.Errorf("expected least-loaded worker w2, got %s", id)
	}
}

func TestLoadBalancedTopology_SkipsUnhealthy(t *testing.T) {
	s := &LoadBalancedTopology{}
	workers := []WorkerCandidate{
		{ID: "w1", ActiveTasks: 0, HealthState: HealthUnhealthy},
		{ID: "w2", ActiveTasks: 5, HealthState: HealthHealthy},
	}

	id, _ := s.Assign(workers, WorkItem{ID: "x"}, nil)
	if id != "w2" {
		t.Errorf("expected healthy worker w2, got %s", id)
	}
}

func TestTopologyOptimizer_Simulate(t *testing.T) {
	opt := NewTopologyOptimizer()
	workers := testWorkers()
	items := testWorkItems()
	analytics := testAnalytics()

	result := opt.Simulate(workers, items, analytics)

	if result.BestStrategy == "" {
		t.Error("expected a best strategy to be selected")
	}
	if len(result.Scores) != 3 {
		t.Errorf("expected 3 strategy scores, got %d", len(result.Scores))
	}
	if result.WorkerCount != 3 {
		t.Errorf("expected 3 healthy workers, got %d", result.WorkerCount)
	}
	if result.WorkItems != 6 {
		t.Errorf("expected 6 work items, got %d", result.WorkItems)
	}
	if len(result.Assignments) != 6 {
		t.Errorf("expected 6 assignments, got %d", len(result.Assignments))
	}

	// Verify scores are sorted descending by Total
	for i := 1; i < len(result.Scores); i++ {
		if result.Scores[i].Total > result.Scores[i-1].Total {
			t.Errorf("scores not sorted: index %d (%f) > index %d (%f)",
				i, result.Scores[i].Total, i-1, result.Scores[i-1].Total)
		}
	}
}

func TestTopologyOptimizer_LastResult(t *testing.T) {
	opt := NewTopologyOptimizer()
	if opt.LastResult() != nil {
		t.Error("expected nil before any simulation")
	}

	opt.Simulate(testWorkers(), testWorkItems(), nil)
	if opt.LastResult() == nil {
		t.Error("expected non-nil after simulation")
	}
}

func TestTopologyOptimizer_AddStrategy(t *testing.T) {
	opt := NewTopologyOptimizer()
	opt.AddStrategy(&LoadBalancedTopology{})

	result := opt.Simulate(testWorkers(), testWorkItems(), nil)
	// 3 default + 1 added = 4
	if len(result.Scores) != 4 {
		t.Errorf("expected 4 scores after adding strategy, got %d", len(result.Scores))
	}
}

func TestOptimalAssignment(t *testing.T) {
	workers := testWorkers()
	items := testWorkItems()
	analytics := testAnalytics()

	assignments := OptimalAssignment(workers, items, analytics)
	if len(assignments) != len(items) {
		t.Errorf("expected %d assignments, got %d", len(items), len(assignments))
	}

	// Verify all items are assigned
	assigned := make(map[string]bool)
	for _, a := range assignments {
		assigned[a.WorkItemID] = true
		if a.WorkerID == "" {
			t.Errorf("item %s has empty worker assignment", a.WorkItemID)
		}
	}
	for _, item := range items {
		if !assigned[item.ID] {
			t.Errorf("item %s was not assigned", item.ID)
		}
	}
}

func TestOptimalAssignment_EmptyWorkers(t *testing.T) {
	items := testWorkItems()
	assignments := OptimalAssignment(nil, items, nil)
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments with no workers, got %d", len(assignments))
	}
}

func TestOptimalAssignment_EmptyItems(t *testing.T) {
	workers := testWorkers()
	assignments := OptimalAssignment(workers, nil, nil)
	if len(assignments) != 0 {
		t.Errorf("expected 0 assignments with no items, got %d", len(assignments))
	}
}

func TestLoadStdDev(t *testing.T) {
	// Perfect balance: all workers with 2 items
	loads := map[string]int{"w1": 2, "w2": 2, "w3": 2}
	if sd := loadStdDev(loads, 3); sd != 0 {
		t.Errorf("expected 0 stddev for perfect balance, got %f", sd)
	}

	// Imbalanced: one worker has all items
	loads = map[string]int{"w1": 6}
	sd := loadStdDev(loads, 3)
	if sd == 0 {
		t.Error("expected non-zero stddev for imbalanced load")
	}
}

func TestComputeComposite(t *testing.T) {
	// Higher throughput should yield higher score
	a := computeComposite(TopologyScore{ThroughputEst: 10, CostEfficiency: 1, LatencyEstMs: 1000})
	b := computeComposite(TopologyScore{ThroughputEst: 5, CostEfficiency: 1, LatencyEstMs: 1000})
	if a <= b {
		t.Errorf("higher throughput should yield higher score: %f vs %f", a, b)
	}

	// Higher load imbalance should yield lower score
	c := computeComposite(TopologyScore{ThroughputEst: 5, CostEfficiency: 1, LoadBalance: 0})
	d := computeComposite(TopologyScore{ThroughputEst: 5, CostEfficiency: 1, LoadBalance: 5})
	if c <= d {
		t.Errorf("lower imbalance should yield higher score: %f vs %f", c, d)
	}
}

func TestSimulateWithAnalytics(t *testing.T) {
	opt := NewTopologyOptimizer()
	analytics := testAnalytics()
	workers := testWorkers()
	items := testWorkItems()

	result := opt.Simulate(workers, items, analytics)

	// With analytics, latency and throughput estimates should be non-default
	for _, score := range result.Scores {
		if score.LatencyEstMs == 0 {
			t.Errorf("strategy %s: expected non-zero latency estimate with analytics", score.Strategy)
		}
		if score.ThroughputEst == 0 {
			t.Errorf("strategy %s: expected non-zero throughput estimate with analytics", score.Strategy)
		}
	}
}
