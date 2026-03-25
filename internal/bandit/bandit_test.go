package bandit

import (
	"testing"
	"time"
)

func testArms() []Arm {
	return []Arm{
		{ID: "a", Provider: "claude", Model: "opus"},
		{ID: "b", Provider: "gemini", Model: "pro"},
		{ID: "c", Provider: "codex", Model: "o3"},
	}
}

func TestNewThompsonSampling(t *testing.T) {
	ts := NewThompsonSampling(testArms(), 0)
	stats := ts.ArmStats()

	if len(stats) != 3 {
		t.Fatalf("expected 3 arms, got %d", len(stats))
	}
	for _, id := range []string{"a", "b", "c"} {
		s, ok := stats[id]
		if !ok {
			t.Fatalf("missing arm %q", id)
		}
		if s.Alpha != 1.0 || s.Beta != 1.0 {
			t.Errorf("arm %q: expected Alpha=1 Beta=1, got Alpha=%f Beta=%f", id, s.Alpha, s.Beta)
		}
		if s.Pulls != 0 {
			t.Errorf("arm %q: expected 0 pulls, got %d", id, s.Pulls)
		}
	}
}

func TestThompsonSelect(t *testing.T) {
	ts := NewThompsonSampling(testArms(), 0)
	counts := make(map[string]int)

	for i := 0; i < 100; i++ {
		arm := ts.Select(nil)
		counts[arm.ID]++
	}

	for _, id := range []string{"a", "b", "c"} {
		if counts[id] == 0 {
			t.Errorf("arm %q was never selected in 100 trials (exploration failure)", id)
		}
	}
}

func TestThompsonUpdate(t *testing.T) {
	ts := NewThompsonSampling(testArms(), 0)

	// Arm "a" gets 10 successes, arm "b" gets 10 failures.
	for i := 0; i < 10; i++ {
		ts.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
		ts.Update(Reward{ArmID: "b", Value: 0.0, Timestamp: time.Now()})
	}

	counts := make(map[string]int)
	for i := 0; i < 100; i++ {
		arm := ts.Select(nil)
		counts[arm.ID]++
	}

	if counts["a"] <= 80 {
		t.Errorf("expected arm 'a' selected >80 times, got %d", counts["a"])
	}
}

func TestThompsonSlidingWindow(t *testing.T) {
	ts := NewThompsonSampling(testArms(), 5)

	// 5 failures for arm "a".
	for i := 0; i < 5; i++ {
		ts.Update(Reward{ArmID: "a", Value: 0.0, Timestamp: time.Now()})
	}

	// Then 5 successes — should push failures out of the window.
	for i := 0; i < 5; i++ {
		ts.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
	}

	stats := ts.ArmStats()
	s := stats["a"]
	if s.Alpha <= s.Beta {
		t.Errorf("after window slide, expected alpha > beta, got alpha=%f beta=%f", s.Alpha, s.Beta)
	}
	if s.Pulls != 5 {
		t.Errorf("expected 5 pulls in window, got %d", s.Pulls)
	}
}

func TestThompsonArmStats(t *testing.T) {
	ts := NewThompsonSampling(testArms(), 0)

	ts.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
	ts.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
	ts.Update(Reward{ArmID: "b", Value: 0.0, Timestamp: time.Now()})

	stats := ts.ArmStats()

	sa := stats["a"]
	if sa.Pulls != 2 {
		t.Errorf("arm a: expected 2 pulls, got %d", sa.Pulls)
	}
	// Alpha should be 1 (prior) + 1.0 + 1.0 = 3.0, Beta stays at 1.0.
	if sa.Alpha != 3.0 {
		t.Errorf("arm a: expected alpha=3.0, got %f", sa.Alpha)
	}
	if sa.Beta != 1.0 {
		t.Errorf("arm a: expected beta=1.0, got %f", sa.Beta)
	}

	sb := stats["b"]
	if sb.Pulls != 1 {
		t.Errorf("arm b: expected 1 pull, got %d", sb.Pulls)
	}
	// Alpha stays 1.0, Beta = 1 (prior) + (1 - 0.0) = 2.0.
	if sb.Alpha != 1.0 {
		t.Errorf("arm b: expected alpha=1.0, got %f", sb.Alpha)
	}
	if sb.Beta != 2.0 {
		t.Errorf("arm b: expected beta=2.0, got %f", sb.Beta)
	}

	// Mean reward for arm a: 3.0 / (3.0 + 1.0) = 0.75.
	expectedMean := 3.0 / 4.0
	if sa.MeanReward != expectedMean {
		t.Errorf("arm a: expected mean=%f, got %f", expectedMean, sa.MeanReward)
	}
}

func TestThompsonEmptyArms(t *testing.T) {
	ts := NewThompsonSampling(nil, 0)
	arm := ts.Select(nil)

	if arm.ID != "" || arm.Provider != "" || arm.Model != "" {
		t.Errorf("expected empty Arm from nil arms, got %+v", arm)
	}

	stats := ts.ArmStats()
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}
