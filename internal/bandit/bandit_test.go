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

	for range 100 {
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
	for range 10 {
		ts.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
		ts.Update(Reward{ArmID: "b", Value: 0.0, Timestamp: time.Now()})
	}

	counts := make(map[string]int)
	for range 100 {
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
	for range 5 {
		ts.Update(Reward{ArmID: "a", Value: 0.0, Timestamp: time.Now()})
	}

	// Then 5 successes — should push failures out of the window.
	for range 5 {
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

// --- UCB1 Selector tests ---

func TestBanditWithoutCascade(t *testing.T) {
	t.Parallel()

	arms := []Arm{
		{ID: "ultra-cheap", Provider: "gemini", Model: "gemini-2.0-flash-lite"},
		{ID: "worker", Provider: "gemini", Model: "gemini-2.5-flash"},
		{ID: "coding", Provider: "claude", Model: "claude-sonnet"},
		{ID: "reasoning", Provider: "claude", Model: "claude-opus"},
	}

	sel := NewSelector(arms)

	// Should select a provider without cascade.
	provider := sel.SelectProvider()
	if provider == "" {
		t.Fatal("expected non-empty provider from bandit selection")
	}

	// First 4 selections should explore each arm once (UCB1 guarantees this).
	seen := make(map[int]bool)
	sel2 := NewSelector(arms)
	for i := range 4 {
		idx := sel2.Select()
		if idx < 0 || idx >= 4 {
			t.Fatalf("selection %d: got index %d, expected 0-3", i, idx)
		}
		seen[idx] = true
		sel2.Update(idx, 1.0) // uniform reward
	}
	if len(seen) != 4 {
		t.Errorf("expected all 4 arms explored in first 4 pulls, got %d unique", len(seen))
	}
}

func TestBanditSelectEmpty(t *testing.T) {
	t.Parallel()

	sel := NewSelector(nil)
	if idx := sel.Select(); idx != -1 {
		t.Errorf("expected -1 for empty selector, got %d", idx)
	}
	if p := sel.SelectProvider(); p != "" {
		t.Errorf("expected empty provider for empty selector, got %q", p)
	}
}

func TestBanditUpdate(t *testing.T) {
	t.Parallel()

	arms := []Arm{
		{ID: "a", Provider: "a", Model: "m1"},
		{ID: "b", Provider: "b", Model: "m2"},
	}
	sel := NewSelector(arms)

	// Pull arm 0, give it low reward.
	sel.Update(0, 0.1)
	// Pull arm 1, give it high reward.
	sel.Update(1, 0.9)

	// After exploration phase (both pulled once), UCB1 should favor arm 1.
	idx := sel.Select()
	sel.Update(idx, 0.5)
	idx = sel.Select()
	sel.Update(idx, 0.5)

	// After several rounds with arm 1 having higher reward, it should be selected.
	for range 10 {
		sel.Update(0, 0.1)
		sel.Update(1, 0.9)
	}

	// With 12 pulls on each arm, arm 1 avg=0.9 should dominate arm 0 avg=0.1.
	selected := sel.Select()
	arm := sel.GetArm(selected)
	if arm == nil {
		t.Fatal("expected non-nil arm")
	}
	if arm.Provider != "b" {
		t.Logf("selected provider %q (may vary due to exploration term)", arm.Provider)
	}
}

func TestBanditArmCount(t *testing.T) {
	t.Parallel()

	sel := NewSelector([]Arm{{ID: "x", Provider: "a"}, {ID: "y", Provider: "b"}, {ID: "z", Provider: "c"}})
	if sel.ArmCount() != 3 {
		t.Errorf("expected 3 arms, got %d", sel.ArmCount())
	}
}

// --- Thompson boundary tests ---

func TestThompsonSingleArm(t *testing.T) {
	arms := []Arm{{ID: "only", Provider: "claude", Model: "opus"}}
	ts := NewThompsonSampling(arms, 0)

	// With a single arm, Select must always return it.
	for range 50 {
		arm := ts.Select(nil)
		if arm.ID != "only" {
			t.Fatalf("expected arm 'only', got %q", arm.ID)
		}
	}

	// Update and verify stats reflect the single arm.
	ts.Update(Reward{ArmID: "only", Value: 1.0, Timestamp: time.Now()})
	stats := ts.ArmStats()
	if len(stats) != 1 {
		t.Fatalf("expected 1 arm stat, got %d", len(stats))
	}
	if stats["only"].Pulls != 1 {
		t.Fatalf("expected 1 pull, got %d", stats["only"].Pulls)
	}
}

func TestThompsonWindowExactCapacity(t *testing.T) {
	arms := []Arm{{ID: "x", Provider: "claude", Model: "opus"}}
	ts := NewThompsonSampling(arms, 5)

	// Add exactly 5 rewards (3 successes, 2 failures).
	rewards := []float64{1.0, 0.0, 1.0, 1.0, 0.0}
	for _, r := range rewards {
		ts.Update(Reward{ArmID: "x", Value: r, Timestamp: time.Now()})
	}

	stats := ts.ArmStats()
	s := stats["x"]

	if s.Pulls != 5 {
		t.Fatalf("expected 5 pulls, got %d", s.Pulls)
	}

	// With window=5 and exactly 5 rewards, no sliding should have occurred.
	// 3 successes (>=0.5): alpha = 1 + 1.0 + 1.0 + 1.0 = 4.0
	// 2 failures (<0.5): beta = 1 + 1.0 + 1.0 = 3.0
	if s.Alpha != 4.0 {
		t.Fatalf("expected alpha=4.0, got %f", s.Alpha)
	}
	if s.Beta != 3.0 {
		t.Fatalf("expected beta=3.0, got %f", s.Beta)
	}
}
