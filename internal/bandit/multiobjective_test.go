package bandit

import (
	"testing"
)

func TestParetoFront_OneDominates(t *testing.T) {
	t.Parallel()

	arms := []string{"good", "bad"}
	objectives := []Objective{
		{Name: "quality", Weight: 0.5, Direction: Maximize},
		{Name: "cost", Weight: 0.5, Direction: Minimize},
	}
	mob := NewMultiObjectiveBandit(arms, objectives)

	// "good" arm: high quality, low cost (both better).
	for range 50 {
		mob.Update("good", map[string]float64{"quality": 0.9, "cost": 0.1})
		mob.Update("bad", map[string]float64{"quality": 0.1, "cost": 0.9})
	}

	front := mob.ParetoFront()
	if len(front) != 2 {
		t.Fatalf("expected 2 arms in result, got %d", len(front))
	}

	// "good" should be rank 0 (non-dominated), "bad" rank 1.
	var goodStats, badStats MOArmStats
	for _, s := range front {
		switch s.Arm {
		case "good":
			goodStats = s
		case "bad":
			badStats = s
		}
	}

	if goodStats.ParetoRank != 0 {
		t.Errorf("expected 'good' Pareto rank 0, got %d", goodStats.ParetoRank)
	}
	if badStats.ParetoRank != 1 {
		t.Errorf("expected 'bad' Pareto rank 1, got %d", badStats.ParetoRank)
	}
}

func TestParetoFront_NonDominatedTradeoffs(t *testing.T) {
	t.Parallel()

	arms := []string{"highQ", "lowCost"}
	objectives := []Objective{
		{Name: "quality", Weight: 0.5, Direction: Maximize},
		{Name: "cost", Weight: 0.5, Direction: Minimize},
	}
	mob := NewMultiObjectiveBandit(arms, objectives)

	// "highQ": great quality but expensive.
	// "lowCost": cheap but lower quality.
	// Neither dominates the other.
	for range 50 {
		mob.Update("highQ", map[string]float64{"quality": 0.9, "cost": 0.8})
		mob.Update("lowCost", map[string]float64{"quality": 0.3, "cost": 0.1})
	}

	front := mob.ParetoFront()
	if len(front) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(front))
	}

	// Both should be Pareto rank 0 since neither dominates the other.
	for _, s := range front {
		if s.ParetoRank != 0 {
			t.Errorf("arm %q: expected Pareto rank 0 (non-dominated), got %d", s.Arm, s.ParetoRank)
		}
	}
}

func TestWeightAdjustment_ChangesPreference(t *testing.T) {
	t.Parallel()

	arms := []string{"highQ", "lowCost"}

	// "highQ": quality=0.9, cost=0.8 (expensive)
	// "lowCost": quality=0.3, cost=0.1 (cheap)
	trainArms := func(mob *MultiObjectiveBandit) {
		for range 100 {
			mob.Update("highQ", map[string]float64{"quality": 0.9, "cost": 0.8})
			mob.Update("lowCost", map[string]float64{"quality": 0.3, "cost": 0.1})
		}
	}

	// Quality-heavy weights: should prefer highQ.
	qualityHeavy := []Objective{
		{Name: "quality", Weight: 0.9, Direction: Maximize},
		{Name: "cost", Weight: 0.1, Direction: Minimize},
	}
	mobQ := NewMultiObjectiveBandit(arms, qualityHeavy)
	trainArms(mobQ)

	highQCount := 0
	for range 200 {
		arm, _ := mobQ.Select(ContextFeatures{})
		if arm == "highQ" {
			highQCount++
		}
	}

	// Cost-heavy weights: should prefer lowCost.
	costHeavy := []Objective{
		{Name: "quality", Weight: 0.1, Direction: Maximize},
		{Name: "cost", Weight: 0.9, Direction: Minimize},
	}
	mobC := NewMultiObjectiveBandit(arms, costHeavy)
	trainArms(mobC)

	lowCostCount := 0
	for range 200 {
		arm, _ := mobC.Select(ContextFeatures{})
		if arm == "lowCost" {
			lowCostCount++
		}
	}

	// With quality-heavy weights, highQ should be selected more.
	if highQCount < 100 {
		t.Errorf("quality-heavy: expected highQ selected >100/200, got %d", highQCount)
	}

	// With cost-heavy weights, lowCost should be selected more.
	if lowCostCount < 100 {
		t.Errorf("cost-heavy: expected lowCost selected >100/200, got %d", lowCostCount)
	}
}

func TestConvergence_ToParetoOptimal(t *testing.T) {
	t.Parallel()

	// Three arms: one dominates, two are dominated.
	arms := []string{"optimal", "mediocre", "worst"}
	objectives := []Objective{
		{Name: "quality", Weight: 0.6, Direction: Maximize},
		{Name: "cost", Weight: 0.3, Direction: Minimize},
		{Name: "latency", Weight: 0.1, Direction: Minimize},
	}
	mob := NewMultiObjectiveBandit(arms, objectives)

	// Simulate 1000 iterations: train and select.
	for range 1000 {
		mob.Update("optimal", map[string]float64{
			"quality": 0.9, "cost": 0.1, "latency": 0.2,
		})
		mob.Update("mediocre", map[string]float64{
			"quality": 0.5, "cost": 0.5, "latency": 0.5,
		})
		mob.Update("worst", map[string]float64{
			"quality": 0.2, "cost": 0.8, "latency": 0.9,
		})
	}

	// Check selection frequency over next 500 trials.
	counts := make(map[string]int)
	for range 500 {
		arm, _ := mob.Select(ContextFeatures{})
		counts[arm]++
	}

	// "optimal" should be selected the majority of the time.
	if counts["optimal"] < 250 {
		t.Errorf("expected 'optimal' selected >250/500, got %d (mediocre=%d, worst=%d)",
			counts["optimal"], counts["mediocre"], counts["worst"])
	}

	// Verify Pareto ranking.
	front := mob.ParetoFront()
	for _, s := range front {
		if s.Arm == "optimal" && s.ParetoRank != 0 {
			t.Errorf("expected 'optimal' at Pareto rank 0, got %d", s.ParetoRank)
		}
	}
}

func TestSingleObjective_ReducesToStandard(t *testing.T) {
	t.Parallel()

	arms := []string{"a", "b"}
	objectives := []Objective{
		{Name: "quality", Weight: 1.0, Direction: Maximize},
	}
	mob := NewMultiObjectiveBandit(arms, objectives)

	// Arm "a" is better.
	for range 100 {
		mob.Update("a", map[string]float64{"quality": 0.9})
		mob.Update("b", map[string]float64{"quality": 0.1})
	}

	// Arm "a" should be selected most of the time.
	aCount := 0
	for range 200 {
		arm, _ := mob.Select(ContextFeatures{})
		if arm == "a" {
			aCount++
		}
	}

	if aCount < 150 {
		t.Errorf("single objective: expected arm 'a' selected >150/200, got %d", aCount)
	}

	// Pareto front should show "a" at rank 0.
	front := mob.ParetoFront()
	for _, s := range front {
		if s.Arm == "a" && s.ParetoRank != 0 {
			t.Errorf("expected arm 'a' at Pareto rank 0, got %d", s.ParetoRank)
		}
		if s.Arm == "b" && s.ParetoRank != 1 {
			t.Errorf("expected arm 'b' at Pareto rank 1, got %d", s.ParetoRank)
		}
	}
}

func TestNewMultiObjectiveBandit_Defaults(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit([]string{"a", "b"}, nil)
	if len(mob.objectives) != 3 {
		t.Fatalf("expected 3 default objectives, got %d", len(mob.objectives))
	}

	names := make(map[string]bool)
	for _, o := range mob.objectives {
		names[o.Name] = true
	}
	for _, expected := range []string{"quality", "cost", "latency"} {
		if !names[expected] {
			t.Errorf("missing default objective %q", expected)
		}
	}
}

func TestSelect_EmptyArms(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit(nil, DefaultObjectives())
	arm, rationale := mob.Select(ContextFeatures{})
	if arm != "" {
		t.Errorf("expected empty arm from nil arms, got %q", arm)
	}
	if rationale != "no arms available" {
		t.Errorf("expected 'no arms available' rationale, got %q", rationale)
	}
}

func TestSelect_ReturnsRationale(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit([]string{"a"}, DefaultObjectives())
	mob.Update("a", map[string]float64{"quality": 0.8, "cost": 0.2, "latency": 0.3})

	arm, rationale := mob.Select(ContextFeatures{})
	if arm != "a" {
		t.Errorf("expected arm 'a', got %q", arm)
	}
	if rationale == "" {
		t.Error("expected non-empty rationale")
	}
}

func TestUpdate_UnknownArm(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit([]string{"a"}, DefaultObjectives())
	// Should not panic.
	mob.Update("nonexistent", map[string]float64{"quality": 0.5})
}

func TestUpdate_UnknownObjective(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit([]string{"a"}, DefaultObjectives())
	// Unknown objective should be silently ignored.
	mob.Update("a", map[string]float64{"unknown": 0.5})

	front := mob.ParetoFront()
	if len(front) != 1 {
		t.Fatalf("expected 1 arm, got %d", len(front))
	}
	// Known objectives should still have initial priors.
	for _, obj := range DefaultObjectives() {
		os := front[0].Objectives[obj.Name]
		if os.Count != 0 {
			t.Errorf("objective %q should have 0 observations, got %d", obj.Name, os.Count)
		}
	}
}

func TestUpdate_ClampsValues(t *testing.T) {
	t.Parallel()

	mob := NewMultiObjectiveBandit([]string{"a"}, []Objective{
		{Name: "q", Weight: 1.0, Direction: Maximize},
	})

	// Values outside [0, 1] should be clamped.
	mob.Update("a", map[string]float64{"q": 1.5})
	mob.Update("a", map[string]float64{"q": -0.5})

	front := mob.ParetoFront()
	os := front[0].Objectives["q"]
	if os.Count != 2 {
		t.Fatalf("expected 2 observations, got %d", os.Count)
	}
	// Mean should reflect clamped values (1.0 and 0.0).
	// Alpha = 1 + 1.0 = 2.0 (value=1.0 >= 0.5)
	// Beta = 1 + 1.0 = 2.0 (value=0.0 < 0.5, contribution = 1-0 = 1.0)
	if os.Alpha != 2.0 {
		t.Errorf("expected alpha=2.0 after clamped updates, got %f", os.Alpha)
	}
	if os.Beta != 2.0 {
		t.Errorf("expected beta=2.0 after clamped updates, got %f", os.Beta)
	}
}

func TestParetoFront_ThreeWayNonDominated(t *testing.T) {
	t.Parallel()

	// Three arms, each best in one objective.
	arms := []string{"fast", "cheap", "quality"}
	objectives := []Objective{
		{Name: "quality", Weight: 0.33, Direction: Maximize},
		{Name: "cost", Weight: 0.33, Direction: Minimize},
		{Name: "latency", Weight: 0.34, Direction: Minimize},
	}
	mob := NewMultiObjectiveBandit(arms, objectives)

	for range 100 {
		mob.Update("fast", map[string]float64{"quality": 0.3, "cost": 0.5, "latency": 0.1})
		mob.Update("cheap", map[string]float64{"quality": 0.3, "cost": 0.1, "latency": 0.5})
		mob.Update("quality", map[string]float64{"quality": 0.9, "cost": 0.5, "latency": 0.5})
	}

	front := mob.ParetoFront()
	rank0Count := 0
	for _, s := range front {
		if s.ParetoRank == 0 {
			rank0Count++
		}
	}

	// All three should be non-dominated (each excels in one dimension).
	if rank0Count != 3 {
		t.Errorf("expected 3 non-dominated arms, got %d", rank0Count)
		for _, s := range front {
			t.Logf("  arm=%s rank=%d objectives=%+v", s.Arm, s.ParetoRank, s.Objectives)
		}
	}
}

func TestDominates_Basic(t *testing.T) {
	t.Parallel()

	objectives := []Objective{
		{Name: "q", Weight: 0.5, Direction: Maximize},
		{Name: "c", Weight: 0.5, Direction: Minimize},
	}

	better := MOArmStats{
		Objectives: map[string]ObjectiveStats{
			"q": {Mean: 0.8},
			"c": {Mean: 0.2},
		},
	}
	worse := MOArmStats{
		Objectives: map[string]ObjectiveStats{
			"q": {Mean: 0.3},
			"c": {Mean: 0.7},
		},
	}

	if !dominates(better, worse, objectives) {
		t.Error("expected better to dominate worse")
	}
	if dominates(worse, better, objectives) {
		t.Error("expected worse NOT to dominate better")
	}
}

func TestDominates_EqualIsNotDomination(t *testing.T) {
	t.Parallel()

	objectives := []Objective{
		{Name: "q", Weight: 1.0, Direction: Maximize},
	}

	a := MOArmStats{Objectives: map[string]ObjectiveStats{"q": {Mean: 0.5}}}
	b := MOArmStats{Objectives: map[string]ObjectiveStats{"q": {Mean: 0.5}}}

	if dominates(a, b, objectives) {
		t.Error("equal arms should not dominate each other")
	}
}

func TestNormalizedWeights(t *testing.T) {
	t.Parallel()

	objectives := []Objective{
		{Name: "a", Weight: 2.0, Direction: Maximize},
		{Name: "b", Weight: 3.0, Direction: Maximize},
	}
	mob := NewMultiObjectiveBandit([]string{"x"}, objectives)

	// Weights should sum to 1.
	sum := 0.0
	for _, w := range mob.normWeights {
		sum += w
	}
	if sum < 0.999 || sum > 1.001 {
		t.Errorf("expected normalized weights sum ~1.0, got %f", sum)
	}

	// Check proportions.
	if mob.normWeights[0] < 0.39 || mob.normWeights[0] > 0.41 {
		t.Errorf("expected weight[0] ~0.4, got %f", mob.normWeights[0])
	}
	if mob.normWeights[1] < 0.59 || mob.normWeights[1] > 0.61 {
		t.Errorf("expected weight[1] ~0.6, got %f", mob.normWeights[1])
	}
}
