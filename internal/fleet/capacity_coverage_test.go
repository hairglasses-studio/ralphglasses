package fleet

import (
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestWorkersFromParallelism_ZeroTasks(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	w := Workload{TotalTasks: 0}
	got := cp.workersFromParallelism(w)
	if got != 0 {
		t.Errorf("workersFromParallelism(0 tasks) = %d, want 0", got)
	}
}

func TestWorkersFromParallelism_FewTasks(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	w := Workload{TotalTasks: 4}
	// sqrt(4) = 2
	got := cp.workersFromParallelism(w)
	if got != 2 {
		t.Errorf("workersFromParallelism(4 tasks) = %d, want 2", got)
	}
}

func TestWorkersFromParallelism_ManyTasks(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	w := Workload{TotalTasks: 100}
	// sqrt(100) = 10
	got := cp.workersFromParallelism(w)
	if got != 10 {
		t.Errorf("workersFromParallelism(100 tasks) = %d, want 10", got)
	}
}

func TestWorkersFromRateLimits_EmptyProviderMix(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	w := Workload{TotalTasks: 10, ProviderMix: map[session.Provider]float64{}}
	got := cp.workersFromRateLimits(w)
	if len(got) != 0 {
		t.Errorf("workersFromRateLimits(empty mix) = %v, want empty", got)
	}
}

func TestWorkersFromRateLimits_NoRateLimitInfo(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	w := Workload{
		TotalTasks:  10,
		ProviderMix: map[session.Provider]float64{session.ProviderClaude: 1.0},
	}
	// No rate limits registered, should default to 1 worker.
	got := cp.workersFromRateLimits(w)
	if got[session.ProviderClaude] != 1 {
		t.Errorf("workersFromRateLimits(no rate limit) = %v, want claude=1", got)
	}
}

func TestWorkersFromRateLimits_WithConcurrentLimit(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	cp.SetRateLimit(ProviderRateLimit{
		Provider:           session.ProviderClaude,
		ConcurrentSessions: 5,
	})
	w := Workload{
		TotalTasks:  10,
		ProviderMix: map[session.Provider]float64{session.ProviderClaude: 1.0},
	}
	// 10 tasks / 5 concurrent = 2 workers
	got := cp.workersFromRateLimits(w)
	if got[session.ProviderClaude] != 2 {
		t.Errorf("workersFromRateLimits = %v, want claude=2", got)
	}
}

func TestWorkersFromRateLimits_ZeroFraction(t *testing.T) {
	cp := NewCapacityPlanner(100.0)
	// Provider with 0 fraction of tasks gets skipped.
	w := Workload{
		TotalTasks: 10,
		ProviderMix: map[session.Provider]float64{
			session.ProviderClaude: 0.0,
		},
	}
	got := cp.workersFromRateLimits(w)
	if _, ok := got[session.ProviderClaude]; ok {
		t.Errorf("workersFromRateLimits: provider with 0 fraction should be skipped")
	}
}
