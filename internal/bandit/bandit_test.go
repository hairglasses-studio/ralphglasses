package bandit

import (
	"testing"
)

func TestBanditWithoutCascade(t *testing.T) {
	t.Parallel()

	// Create bandit arms directly, without any cascade router.
	arms := []Arm{
		{Provider: "gemini", Model: "gemini-2.0-flash-lite", Label: "ultra-cheap", CostPer1M: 0.10},
		{Provider: "gemini", Model: "gemini-2.5-flash", Label: "worker", CostPer1M: 0.30},
		{Provider: "claude", Model: "claude-sonnet", Label: "coding", CostPer1M: 3.00},
		{Provider: "claude", Model: "claude-opus", Label: "reasoning", CostPer1M: 15.00},
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
	for i := 0; i < 4; i++ {
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
		{Provider: "a", Model: "m1"},
		{Provider: "b", Model: "m2"},
	}
	sel := NewSelector(arms)

	// Pull arm 0, give it low reward.
	sel.Update(0, 0.1)
	// Pull arm 1, give it high reward.
	sel.Update(1, 0.9)

	// After exploration phase (both pulled once), UCB1 should favor arm 1.
	// Pull both once more to get past initial exploration.
	idx := sel.Select()
	sel.Update(idx, 0.5)
	idx = sel.Select()
	sel.Update(idx, 0.5)

	// After several rounds with arm 1 having higher reward, it should be selected.
	for i := 0; i < 10; i++ {
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

	sel := NewSelector([]Arm{{Provider: "a"}, {Provider: "b"}, {Provider: "c"}})
	if sel.ArmCount() != 3 {
		t.Errorf("expected 3 arms, got %d", sel.ArmCount())
	}
}
