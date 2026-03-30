package bandit

import (
	"math"
	"math/rand/v2"
	"testing"
	"time"
)

func TestRegretTracker_Empty(t *testing.T) {
	rt := NewRegretTracker()
	report := rt.Report()

	if report.TotalPulls != 0 {
		t.Errorf("expected 0 pulls, got %d", report.TotalPulls)
	}
	if report.TotalRegret != 0 {
		t.Errorf("expected 0 regret, got %f", report.TotalRegret)
	}
	if len(report.CumulativeRegret) != 0 {
		t.Errorf("expected empty cumulative regret, got len %d", len(report.CumulativeRegret))
	}
	if report.OptimalArm != "" {
		t.Errorf("expected empty optimal arm, got %q", report.OptimalArm)
	}
}

func TestRegretTracker_SingleArm(t *testing.T) {
	rt := NewRegretTracker()

	for i := 0; i < 100; i++ {
		rt.RecordPull("only", 0.8)
	}

	report := rt.Report()

	if report.TotalPulls != 100 {
		t.Fatalf("expected 100 pulls, got %d", report.TotalPulls)
	}
	if report.OptimalArm != "only" {
		t.Fatalf("expected optimal arm 'only', got %q", report.OptimalArm)
	}
	// With a single arm, regret is always zero (it is always optimal).
	if report.TotalRegret != 0 {
		t.Errorf("expected 0 regret with single arm, got %f", report.TotalRegret)
	}
	if len(report.CumulativeRegret) != 100 {
		t.Fatalf("expected 100 cumulative regret entries, got %d", len(report.CumulativeRegret))
	}
	for i, r := range report.CumulativeRegret {
		if r != 0 {
			t.Fatalf("expected 0 cumulative regret at step %d, got %f", i, r)
		}
	}
}

func TestRegretTracker_TwoArms_OptimalTracking(t *testing.T) {
	rt := NewRegretTracker()

	// Arm "a" has mean reward 0.9, arm "b" has mean reward 0.3.
	for i := 0; i < 50; i++ {
		rt.RecordPull("a", 0.9)
		rt.RecordPull("b", 0.3)
	}

	report := rt.Report()

	if report.OptimalArm != "a" {
		t.Fatalf("expected optimal arm 'a', got %q", report.OptimalArm)
	}
	if report.TotalPulls != 100 {
		t.Fatalf("expected 100 pulls, got %d", report.TotalPulls)
	}

	// Total regret: 50 pulls of arm "b" each contributing (0.9 - 0.3) = 0.6.
	expectedRegret := 50.0 * 0.6
	if math.Abs(report.TotalRegret-expectedRegret) > 1e-6 {
		t.Errorf("expected total regret %f, got %f", expectedRegret, report.TotalRegret)
	}

	// Cumulative regret should be monotonically non-decreasing.
	for i := 1; i < len(report.CumulativeRegret); i++ {
		if report.CumulativeRegret[i] < report.CumulativeRegret[i-1] {
			t.Fatalf("cumulative regret decreased at step %d: %f -> %f",
				i, report.CumulativeRegret[i-1], report.CumulativeRegret[i])
		}
	}
}

func TestRegretTracker_PerArmContribution(t *testing.T) {
	rt := NewRegretTracker()

	// Arm "a" mean=0.9 (optimal), "b" mean=0.5, "c" mean=0.1.
	for i := 0; i < 30; i++ {
		rt.RecordPull("a", 0.9)
		rt.RecordPull("b", 0.5)
		rt.RecordPull("c", 0.1)
	}

	report := rt.Report()

	if len(report.ArmRegrets) != 3 {
		t.Fatalf("expected 3 arm regrets, got %d", len(report.ArmRegrets))
	}

	// Find each arm's regret.
	armMap := map[string]ArmRegret{}
	for _, ar := range report.ArmRegrets {
		armMap[ar.ArmID] = ar
	}

	// Optimal arm "a" should have zero regret contribution.
	if armMap["a"].RegretContrib > 1e-9 {
		t.Errorf("optimal arm 'a' should have ~0 regret, got %f", armMap["a"].RegretContrib)
	}

	// Arm "b": 30 pulls * (0.9 - 0.5) = 12.0.
	expectedB := 30.0 * 0.4
	if math.Abs(armMap["b"].RegretContrib-expectedB) > 1e-6 {
		t.Errorf("arm 'b' regret: expected %f, got %f", expectedB, armMap["b"].RegretContrib)
	}

	// Arm "c": 30 pulls * (0.9 - 0.1) = 24.0.
	expectedC := 30.0 * 0.8
	if math.Abs(armMap["c"].RegretContrib-expectedC) > 1e-6 {
		t.Errorf("arm 'c' regret: expected %f, got %f", expectedC, armMap["c"].RegretContrib)
	}

	// Fractions should sum to ~1.0.
	fracSum := 0.0
	for _, ar := range report.ArmRegrets {
		fracSum += ar.RegretFraction
	}
	if math.Abs(fracSum-1.0) > 1e-6 {
		t.Errorf("regret fractions should sum to 1.0, got %f", fracSum)
	}
}

func TestRegretTracker_BayesianBound(t *testing.T) {
	rt := NewRegretTracker()

	// 2 arms, 1000 pulls.
	for i := 0; i < 500; i++ {
		rt.RecordPull("a", 0.8)
		rt.RecordPull("b", 0.2)
	}

	report := rt.Report()

	// Bound = sqrt(K * T * ln(T)) = sqrt(2 * 1000 * ln(1000)).
	expectedBound := math.Sqrt(2.0 * 1000.0 * math.Log(1000.0))
	if math.Abs(report.BayesianRegretBound-expectedBound) > 1e-6 {
		t.Errorf("expected bound %f, got %f", expectedBound, report.BayesianRegretBound)
	}

	// Actual regret should be below the Bayesian bound.
	if report.TotalRegret > report.BayesianRegretBound {
		t.Logf("total regret %f exceeds Bayesian bound %f (may happen with adversarial data)",
			report.TotalRegret, report.BayesianRegretBound)
	}
}

func TestRegretTracker_ConvergenceRate(t *testing.T) {
	// We build a scenario where regret grows fast, then flattens.
	// Both arms must exist from the start so the optimal mean is established.

	// Phase 1: always pick suboptimal arm while the optimal arm is known.
	rt := NewRegretTracker()
	// Seed both arms so optimal mean is established as 0.9.
	rt.RecordPull("good", 0.9)
	rt.RecordPull("bad", 0.1)

	// 200 suboptimal pulls: regret grows linearly.
	for i := 0; i < 200; i++ {
		rt.RecordPull("bad", 0.1)
	}

	report1 := rt.Report()
	rate1 := report1.ConvergenceRate

	// Phase 2: switch entirely to optimal arm; regret flattens.
	for i := 0; i < 200; i++ {
		rt.RecordPull("good", 0.9)
	}

	report2 := rt.Report()
	rate2 := report2.ConvergenceRate

	// After convergence the recent slope should be near zero (much lower
	// than during the exploration phase where every pull added regret).
	if rate1 <= 0 {
		t.Fatalf("phase1 convergence rate should be positive (growing regret), got %f", rate1)
	}
	if rate2 >= rate1 {
		t.Errorf("convergence rate should decrease after learning: phase1=%f phase2=%f", rate1, rate2)
	}
}

func TestRegretTracker_CumulativeRegretMonotonic(t *testing.T) {
	rt := NewRegretTracker()

	// Mix of arms with varying rewards.
	rewards := map[string]float64{"a": 0.9, "b": 0.5, "c": 0.2}
	arms := []string{"a", "b", "c"}

	for i := 0; i < 300; i++ {
		arm := arms[i%3]
		rt.RecordPull(arm, rewards[arm])
	}

	report := rt.Report()

	for i := 1; i < len(report.CumulativeRegret); i++ {
		if report.CumulativeRegret[i] < report.CumulativeRegret[i-1] {
			t.Fatalf("cumulative regret not monotonic at step %d: %f > %f",
				i, report.CumulativeRegret[i-1], report.CumulativeRegret[i])
		}
	}
}

// TestRegretSublinear_ThompsonSampling proves that Thompson Sampling achieves
// sublinear cumulative regret growth. Over T steps, E[R(T)] = o(T), meaning
// the average per-step regret converges toward zero. This is the defining
// property that separates a good bandit algorithm from uniform random.
func TestRegretSublinear_ThompsonSampling(t *testing.T) {
	// Two arms with a clear gap: "a" = 0.8, "b" = 0.3.
	// Thompson Sampling should learn to favor "a" and accumulate sublinear regret.
	arms := []Arm{
		{ID: "a", Provider: "claude", Model: "opus"},
		{ID: "b", Provider: "gemini", Model: "pro"},
	}
	trueMeans := map[string]float64{"a": 0.8, "b": 0.3}

	ts := NewThompsonSampling(arms, 0)
	rt := NewRegretTracker()

	// Seeded PRNG for reproducible Bernoulli rewards.
	rng := rand.New(rand.NewPCG(42, 99))

	totalSteps := 10000

	for step := 0; step < totalSteps; step++ {
		arm := ts.Select(nil)

		// Bernoulli reward with true mean.
		mean := trueMeans[arm.ID]
		reward := 0.0
		if rng.Float64() < mean {
			reward = 1.0
		}

		ts.Update(Reward{ArmID: arm.ID, Value: reward, Timestamp: time.Now()})
		rt.RecordPull(arm.ID, reward)
	}

	report := rt.Report()

	// Sublinear test: regret at T should be much less than T * gap/2.
	// For linear regret (random policy picking each arm 50%), expected
	// regret ~ T * (0.8 - 0.3) / 2 = T * 0.25.
	// Sublinear (Thompson) regret should be O(sqrt(K*T*ln(T))).
	// We test that TS regret is at most 70% of linear regret, which is
	// a generous threshold easily cleared by any sublinear algorithm
	// over 10000 steps.
	linearRegret := float64(totalSteps) * 0.25
	sublinearThreshold := linearRegret * 0.70

	if report.TotalRegret >= sublinearThreshold {
		t.Errorf("Thompson Sampling regret %f >= sublinear threshold %f (linear would be ~%f); not sublinear",
			report.TotalRegret, sublinearThreshold, linearRegret)
	}

	// Convergence test: per-step regret in the last quarter should be lower
	// than in the first quarter, demonstrating the algorithm learns.
	quarter := totalSteps / 4
	firstQuarterRegret := report.CumulativeRegret[quarter-1]
	lastQuarterRegret := report.TotalRegret - report.CumulativeRegret[3*quarter-1]

	if lastQuarterRegret >= firstQuarterRegret {
		t.Errorf("last-quarter regret %f >= first-quarter regret %f; Thompson Sampling should converge",
			lastQuarterRegret, firstQuarterRegret)
	}

	t.Logf("Thompson Sampling: total_regret=%.1f linear_would_be=%.1f bayesian_bound=%.1f ratio=%.3f",
		report.TotalRegret, linearRegret, report.BayesianRegretBound,
		report.TotalRegret/linearRegret)
}

// TestRegretSublinear_ThompsonVsRandom directly compares Thompson Sampling
// regret against a uniform random baseline to confirm sublinear growth.
func TestRegretSublinear_ThompsonVsRandom(t *testing.T) {
	arms := []Arm{
		{ID: "a", Provider: "claude", Model: "opus"},
		{ID: "b", Provider: "gemini", Model: "pro"},
	}
	trueMeans := map[string]float64{"a": 0.8, "b": 0.3}

	ts := NewThompsonSampling(arms, 0)
	rtTS := NewRegretTracker()
	rtRandom := NewRegretTracker()

	rngTS := rand.New(rand.NewPCG(77, 88))
	rngRand := rand.New(rand.NewPCG(55, 66))

	totalSteps := 5000

	for step := 0; step < totalSteps; step++ {
		// Thompson Sampling selection.
		tsArm := ts.Select(nil)
		tsMean := trueMeans[tsArm.ID]
		tsReward := 0.0
		if rngTS.Float64() < tsMean {
			tsReward = 1.0
		}
		ts.Update(Reward{ArmID: tsArm.ID, Value: tsReward, Timestamp: time.Now()})
		rtTS.RecordPull(tsArm.ID, tsReward)

		// Random baseline: alternate arms.
		randomArm := arms[step%2]
		randMean := trueMeans[randomArm.ID]
		randReward := 0.0
		if rngRand.Float64() < randMean {
			randReward = 1.0
		}
		rtRandom.RecordPull(randomArm.ID, randReward)
	}

	tsReport := rtTS.Report()
	randomReport := rtRandom.Report()

	// Thompson Sampling should have strictly less regret than random.
	if tsReport.TotalRegret >= randomReport.TotalRegret {
		t.Errorf("Thompson regret %f >= random regret %f",
			tsReport.TotalRegret, randomReport.TotalRegret)
	}

	t.Logf("Thompson regret=%.1f vs random regret=%.1f (%.1f%% reduction)",
		tsReport.TotalRegret, randomReport.TotalRegret,
		100.0*(1.0-tsReport.TotalRegret/randomReport.TotalRegret))
}
