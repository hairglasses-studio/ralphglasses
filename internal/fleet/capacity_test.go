package fleet

import (
	"math"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestNewCapacityPlanner(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	if cp == nil {
		t.Fatal("expected non-nil planner")
	}
	if cp.budgetUSD != 100.0 {
		t.Errorf("budget = %f, want 100.0", cp.budgetUSD)
	}
	if cp.RemainingBudget() != 100.0 {
		t.Errorf("remaining = %f, want 100.0", cp.RemainingBudget())
	}
}

func TestRemainingBudget(t *testing.T) {
	cp := NewCapacityPlanner(50.0)
	cp.RecordSpend(20.0)
	if got := cp.RemainingBudget(); got != 30.0 {
		t.Errorf("remaining = %f, want 30.0", got)
	}

	cp.RecordSpend(40.0) // over-spend
	if got := cp.RemainingBudget(); got != 0.0 {
		t.Errorf("remaining = %f, want 0.0 (clamped)", got)
	}
}

func TestSetBudget(t *testing.T) {
	cp := NewCapacityPlanner(10.0)
	cp.SetBudget(200.0)
	if got := cp.RemainingBudget(); got != 200.0 {
		t.Errorf("remaining = %f, want 200.0", got)
	}
}

func TestPlanEmptyWorkload(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	plan := cp.Plan(Workload{TotalTasks: 0}, nil)
	if plan.RecommendedWorkers != 0 {
		t.Errorf("workers = %d, want 0 for empty workload", plan.RecommendedWorkers)
	}
	if plan.EstimatedCostUSD != 0 {
		t.Errorf("cost = %f, want 0", plan.EstimatedCostUSD)
	}
}

func TestPlanBasicWorkload(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		RequestsPerMinute:  60,
		ConcurrentSessions: 5,
		CostPerTaskUSD:     0.10,
	})

	workload := Workload{
		TotalTasks:       25,
		AvgTaskDurationS: 30,
		MaxLatencyS:      300,
		ProviderMix:      map[session.Provider]float64{session.ProviderClaude: 1.0},
		AvgTaskCostUSD:   0.10,
	}

	plan := cp.Plan(workload, nil)

	if plan.RecommendedWorkers < 1 {
		t.Error("expected at least 1 worker")
	}
	if plan.EstimatedCostUSD != 2.5 { // 25 * 0.10
		t.Errorf("cost = %f, want 2.5", plan.EstimatedCostUSD)
	}
	if !plan.BudgetFeasible {
		t.Error("expected budget to be feasible")
	}
}

func TestPlanBudgetConstrained(t *testing.T) {
	cp := NewCapacityPlanner(1.0) // very tight budget
	workload := Workload{
		TotalTasks:       100,
		AvgTaskDurationS: 10,
		MaxLatencyS:      60,
		AvgTaskCostUSD:   0.50,
	}

	plan := cp.Plan(workload, nil)

	if plan.BudgetFeasible {
		t.Error("expected budget to be infeasible (100 tasks * $0.50 = $50 > $1)")
	}
	if len(plan.Warnings) == 0 {
		t.Error("expected budget warning")
	}
}

func TestPlanRateLimitAware(t *testing.T) {
	cp := NewCapacityPlanner(1000.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderGemini,
		RequestsPerMinute:  10,
		ConcurrentSessions: 3,
		CostPerTaskUSD:     0.05,
	})

	workload := Workload{
		TotalTasks:       30,
		AvgTaskDurationS: 20,
		MaxLatencyS:      120,
		ProviderMix:      map[session.Provider]float64{session.ProviderGemini: 1.0},
	}

	plan := cp.Plan(workload, nil)

	// 30 tasks with concurrency 3 => need ceil(30/3)=10 workers
	if v, ok := plan.WorkersPerProvider[session.ProviderGemini]; ok {
		if v < 1 {
			t.Errorf("gemini workers = %d, expected >= 1", v)
		}
	}
	if plan.RecommendedWorkers < 1 {
		t.Error("expected at least 1 worker")
	}
}

func TestPlanMultiProvider(t *testing.T) {
	cp := NewCapacityPlanner(500.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		RequestsPerMinute:  60,
		ConcurrentSessions: 5,
		CostPerTaskUSD:     0.15,
	})
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderGemini,
		RequestsPerMinute:  30,
		ConcurrentSessions: 10,
		CostPerTaskUSD:     0.05,
	})

	workload := Workload{
		TotalTasks:       50,
		AvgTaskDurationS: 20,
		MaxLatencyS:      200,
		ProviderMix: map[session.Provider]float64{
			session.ProviderClaude: 0.4,
			session.ProviderGemini: 0.6,
		},
	}

	plan := cp.Plan(workload, nil)

	// Both providers should appear in workers-per-provider.
	if _, ok := plan.WorkersPerProvider[session.ProviderClaude]; !ok {
		t.Error("expected claude in workers-per-provider")
	}
	if _, ok := plan.WorkersPerProvider[session.ProviderGemini]; !ok {
		t.Error("expected gemini in workers-per-provider")
	}

	// Cost: 20 claude tasks * $0.15 + 30 gemini tasks * $0.05 = $3 + $1.50 = $4.50
	expectedCost := 20*0.15 + 30*0.05
	if math.Abs(plan.EstimatedCostUSD-expectedCost) > 0.01 {
		t.Errorf("cost = %f, want ~%f", plan.EstimatedCostUSD, expectedCost)
	}
	if !plan.BudgetFeasible {
		t.Error("expected budget feasible")
	}
}

func TestPlanLatencySLO(t *testing.T) {
	cp := NewCapacityPlanner(1000.0)

	// 100 tasks, each 60s, SLO 120s => need at least ceil(100*60/120) = 50 workers
	workload := Workload{
		TotalTasks:       100,
		AvgTaskDurationS: 60,
		MaxLatencyS:      120,
		AvgTaskCostUSD:   0.01,
	}

	plan := cp.Plan(workload, nil)

	if plan.RecommendedWorkers < 50 {
		t.Errorf("workers = %d, want >= 50 for latency SLO", plan.RecommendedWorkers)
	}
}

func TestPlanEstimatedDuration(t *testing.T) {
	cp := NewCapacityPlanner(1000.0)
	workload := Workload{
		TotalTasks:       10,
		AvgTaskDurationS: 30,
		AvgTaskCostUSD:   0.01,
	}

	plan := cp.Plan(workload, nil)

	if plan.EstimatedDurationS <= 0 {
		t.Error("expected positive estimated duration")
	}
	// Duration should be at most TotalTasks * AvgTaskDuration (serial case).
	maxDuration := float64(workload.TotalTasks) * workload.AvgTaskDurationS
	if plan.EstimatedDurationS > maxDuration {
		t.Errorf("duration %f exceeds serial max %f", plan.EstimatedDurationS, maxDuration)
	}
}

func TestProviderCapacity(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		ConcurrentSessions: 5,
	})

	got := cp.ProviderCapacity(session.ProviderClaude, 4)
	if got != 20 { // 5 * 4
		t.Errorf("capacity = %d, want 20", got)
	}

	// Unknown provider => 1 per worker.
	got = cp.ProviderCapacity(session.ProviderCodex, 3)
	if got != 3 {
		t.Errorf("unknown provider capacity = %d, want 3", got)
	}
}

func TestProviderThroughput(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:          session.ProviderGemini,
		RequestsPerMinute: 30,
	})

	got := cp.ProviderThroughput(session.ProviderGemini, 2)
	if got != 60.0 { // 30 * 2
		t.Errorf("throughput = %f, want 60.0", got)
	}

	// Unknown provider => 0.
	got = cp.ProviderThroughput(session.ProviderCodex, 1)
	if got != 0 {
		t.Errorf("unknown throughput = %f, want 0", got)
	}
}

func TestRankedProviders(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderClaude,
		CostPerTaskUSD: 0.15,
	})
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderGemini,
		CostPerTaskUSD: 0.05,
	})
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderCodex,
		CostPerTaskUSD: 0.10,
	})

	ranked := cp.RankedProviders()
	if len(ranked) != 3 {
		t.Fatalf("ranked len = %d, want 3", len(ranked))
	}
	if ranked[0].Provider != session.ProviderGemini {
		t.Errorf("cheapest = %s, want gemini", ranked[0].Provider)
	}
	if ranked[1].Provider != session.ProviderCodex {
		t.Errorf("second = %s, want codex", ranked[1].Provider)
	}
	if ranked[2].Provider != session.ProviderClaude {
		t.Errorf("third = %s, want claude", ranked[2].Provider)
	}
}

func TestRankedProvidersZeroCostSortsLast(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderClaude,
		CostPerTaskUSD: 0.10,
	})
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderGemini,
		CostPerTaskUSD: 0, // unknown cost
	})

	ranked := cp.RankedProviders()
	if len(ranked) != 2 {
		t.Fatalf("ranked len = %d, want 2", len(ranked))
	}
	if ranked[0].Provider != session.ProviderClaude {
		t.Errorf("first = %s, want claude (zero cost should sort last)", ranked[0].Provider)
	}
}

func TestOptimalMix(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		ConcurrentSessions: 5,
		CostPerTaskUSD:     0.15,
	})
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderGemini,
		ConcurrentSessions: 10,
		CostPerTaskUSD:     0.05,
	})

	mix := cp.OptimalMix(12, 0, 0)
	if mix == nil {
		t.Fatal("expected non-nil mix")
	}

	// Gemini is cheapest with capacity 10, so it should get most tasks.
	gemFrac, ok := mix[session.ProviderGemini]
	if !ok {
		t.Fatal("expected gemini in mix")
	}
	if gemFrac <= 0 {
		t.Errorf("gemini fraction = %f, want > 0", gemFrac)
	}

	// Total fractions should sum to ~1.0.
	var total float64
	for _, f := range mix {
		total += f
	}
	if math.Abs(total-1.0) > 0.01 {
		t.Errorf("mix fractions sum = %f, want ~1.0", total)
	}
}

func TestOptimalMixEmpty(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	mix := cp.OptimalMix(0, 0, 0)
	if mix != nil {
		t.Error("expected nil mix for zero tasks")
	}

	mix = cp.OptimalMix(10, 0, 0)
	if mix != nil {
		t.Error("expected nil mix with no rate limits configured")
	}
}

func TestPlanFromFleetState(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	state := FleetState{
		Workers: []WorkerInfo{
			{ID: "w1", Status: WorkerOnline, MaxSessions: 5, ActiveSessions: 2},
			{ID: "w2", Status: WorkerOnline, MaxSessions: 5, ActiveSessions: 0},
		},
	}

	workload := Workload{
		TotalTasks:       10,
		AvgTaskDurationS: 15,
		AvgTaskCostUSD:   0.05,
	}

	plan := cp.PlanFromFleetState(workload, state)
	if plan.RecommendedWorkers < 1 {
		t.Error("expected at least 1 worker")
	}
	if !plan.BudgetFeasible {
		t.Error("expected budget feasible")
	}
}

func TestSummary(t *testing.T) {
	cp := NewCapacityPlanner(200.0)
	cp.RecordSpend(50.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:       session.ProviderClaude,
		CostPerTaskUSD: 0.10,
	})

	s := cp.Summary()
	if s["budget_usd"] != 200.0 {
		t.Errorf("budget_usd = %v, want 200.0", s["budget_usd"])
	}
	if s["spent_usd"] != 50.0 {
		t.Errorf("spent_usd = %v, want 50.0", s["spent_usd"])
	}
	if s["provider_count"] != 1 {
		t.Errorf("provider_count = %v, want 1", s["provider_count"])
	}
}

func TestMax3(t *testing.T) {
	tests := []struct {
		a, b, c, want int
	}{
		{1, 2, 3, 3},
		{3, 2, 1, 3},
		{2, 3, 1, 3},
		{5, 5, 5, 5},
		{0, 0, 0, 0},
		{-1, -2, -3, -1},
	}
	for _, tt := range tests {
		if got := max3(tt.a, tt.b, tt.c); got != tt.want {
			t.Errorf("max3(%d, %d, %d) = %d, want %d", tt.a, tt.b, tt.c, got, tt.want)
		}
	}
}

func TestPlanBudgetExhausted(t *testing.T) {
	cp := NewCapacityPlanner(10.0)
	cp.RecordSpend(10.0) // fully exhausted

	workload := Workload{
		TotalTasks:       50,
		AvgTaskDurationS: 10,
		AvgTaskCostUSD:   1.0,
	}

	plan := cp.Plan(workload, nil)
	if plan.BudgetFeasible {
		t.Error("expected infeasible with exhausted budget")
	}
}

func TestConcurrentAccess(t *testing.T) {
	cp := NewCapacityPlanner(1000.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		ConcurrentSessions: 5,
		CostPerTaskUSD:     0.10,
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 100; i++ {
			cp.RecordSpend(0.01)
			_ = cp.RemainingBudget()
			_ = cp.Summary()
		}
	}()

	for i := 0; i < 100; i++ {
		workload := Workload{
			TotalTasks:       10,
			AvgTaskDurationS: 5,
			AvgTaskCostUSD:   0.10,
			ProviderMix:      map[session.Provider]float64{session.ProviderClaude: 1.0},
		}
		_ = cp.Plan(workload, nil)
		_ = cp.ProviderCapacity(session.ProviderClaude, 2)
		_ = cp.RankedProviders()
	}
	<-done
}
