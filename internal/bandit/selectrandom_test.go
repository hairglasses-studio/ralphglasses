package bandit

import "testing"

func TestSelectRandom_Empty(t *testing.T) {
	t.Parallel()
	sel := NewSelector(nil)
	if idx := sel.SelectRandom(); idx != -1 {
		t.Errorf("expected -1 for empty selector, got %d", idx)
	}
}

func TestSelectRandom_SingleArm(t *testing.T) {
	t.Parallel()
	sel := NewSelector([]Arm{{ID: "only", Provider: "claude", Model: "opus"}})
	for range 50 {
		idx := sel.SelectRandom()
		if idx != 0 {
			t.Fatalf("expected 0 for single arm, got %d", idx)
		}
	}
}

func TestSelectRandom_MultipleArms(t *testing.T) {
	t.Parallel()
	arms := []Arm{
		{ID: "a", Provider: "claude", Model: "opus"},
		{ID: "b", Provider: "gemini", Model: "pro"},
		{ID: "c", Provider: "codex", Model: "o3"},
	}
	sel := NewSelector(arms)

	// Over 300 trials, each arm should be selected at least once.
	counts := make(map[int]int)
	for range 300 {
		idx := sel.SelectRandom()
		if idx < 0 || idx >= len(arms) {
			t.Fatalf("SelectRandom returned out-of-range index %d", idx)
		}
		counts[idx]++
	}
	for i := range arms {
		if counts[i] == 0 {
			t.Errorf("arm %d was never selected in 300 trials", i)
		}
	}
}
