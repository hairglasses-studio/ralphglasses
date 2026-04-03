package bandit

import (
	"math"
	"math/rand/v2"
	"sync"
	"testing"
	"time"
)

var neuralArms = []Arm{
	{ID: "cheap", Provider: "gemini", Model: "flash-lite"},
	{ID: "worker", Provider: "gemini", Model: "flash"},
	{ID: "coding", Provider: "claude", Model: "sonnet"},
	{ID: "reasoning", Provider: "claude", Model: "opus"},
}

func TestNewNeuralUCB(t *testing.T) {
	n := NewNeuralUCB(neuralArms, int(NumContextualFeatures), NeuralUCBConfig{})
	stats := n.ArmStats()

	if len(stats) != 4 {
		t.Fatalf("expected 4 arms, got %d", len(stats))
	}
	for _, id := range []string{"cheap", "worker", "coding", "reasoning"} {
		s, ok := stats[id]
		if !ok {
			t.Fatalf("missing arm %q", id)
		}
		if s.Pulls != 0 {
			t.Errorf("arm %q: expected 0 pulls, got %d", id, s.Pulls)
		}
		if s.MeanReward != 0 {
			t.Errorf("arm %q: expected mean 0, got %f", id, s.MeanReward)
		}
	}
}

func TestNeuralUCB_DefaultConfig(t *testing.T) {
	n := NewNeuralUCB(neuralArms, 0, NeuralUCBConfig{})
	if n.cfg.HiddenSize != 32 {
		t.Errorf("expected default HiddenSize 32, got %d", n.cfg.HiddenSize)
	}
	if n.cfg.LearningRate != 0.01 {
		t.Errorf("expected default LearningRate 0.01, got %f", n.cfg.LearningRate)
	}
	if n.cfg.ExplorationC != 1.0 {
		t.Errorf("expected default ExplorationC 1.0, got %f", n.cfg.ExplorationC)
	}
	if n.cfg.BudgetWeight != 0.3 {
		t.Errorf("expected default BudgetWeight 0.3, got %f", n.cfg.BudgetWeight)
	}
	if n.features != int(NumContextualFeatures) {
		t.Errorf("expected features=%d with numFeatures=0, got %d", NumContextualFeatures, n.features)
	}
}

func TestNeuralUCB_ImplementsPolicy(t *testing.T) {
	var _ Policy = (*NeuralUCB)(nil)
}

func TestNeuralUCB_ExploresUnpulledFirst(t *testing.T) {
	n := NewNeuralUCB(neuralArms, int(NumContextualFeatures), NeuralUCBConfig{})
	seen := map[string]bool{}

	for i := 0; i < 4; i++ {
		arm := n.Select(nil)
		if seen[arm.ID] {
			t.Fatalf("arm %s selected twice during exploration", arm.ID)
		}
		seen[arm.ID] = true
		n.Update(Reward{ArmID: arm.ID, Value: 0.5, Timestamp: time.Now()})
	}
	if len(seen) != 4 {
		t.Fatalf("expected 4 unique arms during exploration, got %d", len(seen))
	}
}

func TestNeuralUCB_SelectEmptyArms(t *testing.T) {
	n := NewNeuralUCB(nil, 4, NeuralUCBConfig{})
	arm := n.Select(nil)
	if arm.ID != "" {
		t.Errorf("expected empty arm from nil arms, got %+v", arm)
	}
	stats := n.ArmStats()
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}

func TestNeuralUCB_UpdateUnknownArm(t *testing.T) {
	n := NewNeuralUCB(neuralArms, 4, NeuralUCBConfig{})
	// Should not panic.
	n.Update(Reward{ArmID: "nonexistent", Value: 1.0, Timestamp: time.Now()})
	stats := n.ArmStats()
	for _, s := range stats {
		if s.Pulls != 0 {
			t.Error("no arm should have been updated")
		}
	}
}

func TestNeuralUCB_PredictOutput(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{HiddenSize: 8})

	ctx := []float64{0.5, -0.5, 0.0, 1.0}
	pred := n.Predict("a", ctx)

	// Sigmoid output should be in (0, 1).
	if pred <= 0 || pred >= 1 {
		t.Errorf("expected prediction in (0,1), got %f", pred)
	}

	// Unknown arm should return 0.
	if p := n.Predict("unknown", ctx); p != 0 {
		t.Errorf("expected 0 for unknown arm, got %f", p)
	}
}

func TestNeuralUCB_ForwardSigmoidBounds(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{HiddenSize: 16})

	// Test with various extreme context values.
	cases := [][]float64{
		{0, 0, 0, 0},
		{1, 1, 1, 1},
		{-1, -1, -1, -1},
		{100, -100, 50, -50},
	}
	for _, ctx := range cases {
		pred := n.Predict("a", ctx)
		if pred < 0 || pred > 1 {
			t.Errorf("prediction out of [0,1] range for ctx=%v: %f", ctx, pred)
		}
	}
}

func TestNeuralUCB_SGDReducesError(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{
		HiddenSize:   16,
		LearningRate: 0.05,
		ExplorationC: 0.1,
	})

	ctx := []float64{1.0, 0.0, -1.0, 0.5}
	target := 0.8

	// Measure initial prediction error.
	initialPred := n.Predict("a", ctx)
	initialErr := math.Abs(target - initialPred)

	// Train for 200 steps on a fixed (context, reward) pair.
	for i := 0; i < 200; i++ {
		n.Update(Reward{
			ArmID:     "a",
			Value:     target,
			Timestamp: time.Now(),
			Context:   ctx,
		})
	}

	finalPred := n.Predict("a", ctx)
	finalErr := math.Abs(target - finalPred)

	if finalErr >= initialErr {
		t.Errorf("SGD did not reduce error: initial=%f final=%f (target=%f)", initialErr, finalErr, target)
	}
}

func TestNeuralUCB_LearnsContextDifference(t *testing.T) {
	arms := []Arm{
		{ID: "cheap", Provider: "gemini", Model: "flash"},
		{ID: "expensive", Provider: "claude", Model: "opus"},
	}
	n := NewNeuralUCB(arms, int(NumContextualFeatures), NeuralUCBConfig{
		HiddenSize:   16,
		LearningRate: 0.05,
		ExplorationC: 0.01, // Low exploration to test exploitation.
	})

	simpleCtx := make([]float64, NumContextualFeatures)
	simpleCtx[FeatureComplexity] = -1.0

	complexCtx := make([]float64, NumContextualFeatures)
	complexCtx[FeatureComplexity] = 1.0

	// Train: cheap arm good on simple, bad on complex.
	// Expensive arm good on complex, mediocre on simple.
	for i := 0; i < 300; i++ {
		n.Update(Reward{ArmID: "cheap", Value: 0.9, Timestamp: time.Now(), Context: simpleCtx})
		n.Update(Reward{ArmID: "cheap", Value: 0.1, Timestamp: time.Now(), Context: complexCtx})
		n.Update(Reward{ArmID: "expensive", Value: 0.9, Timestamp: time.Now(), Context: complexCtx})
		n.Update(Reward{ArmID: "expensive", Value: 0.4, Timestamp: time.Now(), Context: simpleCtx})
	}

	// After training, cheap should predict higher for simple context.
	cheapSimple := n.Predict("cheap", simpleCtx)
	cheapComplex := n.Predict("cheap", complexCtx)
	if cheapSimple <= cheapComplex {
		t.Errorf("expected cheap to predict higher for simple (%.3f) than complex (%.3f)",
			cheapSimple, cheapComplex)
	}

	// Expensive should predict higher for complex context.
	expensiveComplex := n.Predict("expensive", complexCtx)
	expensiveSimple := n.Predict("expensive", simpleCtx)
	if expensiveComplex <= expensiveSimple {
		t.Errorf("expected expensive to predict higher for complex (%.3f) than simple (%.3f)",
			expensiveComplex, expensiveSimple)
	}
}

func TestNeuralUCB_ConvergesOnBestArm(t *testing.T) {
	arms := []Arm{
		{ID: "good", Provider: "p", Model: "m"},
		{ID: "bad", Provider: "p", Model: "m"},
	}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{
		HiddenSize:   8,
		LearningRate: 0.02,
		ExplorationC: 0.5,
	})

	ctx := []float64{0.5, 0.5, 0.5, 0.5}

	// Train phase: good arm always returns high reward.
	for i := 0; i < 200; i++ {
		arm := n.Select(ctx)
		val := 0.1
		if arm.ID == "good" {
			val = 0.9
		}
		n.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now(), Context: ctx})
	}

	// Evaluation: good arm should be selected most of the time.
	goodCount := 0
	for i := 0; i < 100; i++ {
		arm := n.Select(ctx)
		if arm.ID == "good" {
			goodCount++
		}
		val := 0.1
		if arm.ID == "good" {
			val = 0.9
		}
		n.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now(), Context: ctx})
	}

	if goodCount < 60 {
		t.Errorf("expected good arm selected >60/100 times, got %d", goodCount)
	}
}

func TestNeuralUCB_BudgetAdjustedReward(t *testing.T) {
	n := NewNeuralUCB(nil, 4, NeuralUCBConfig{BudgetWeight: 0.3})

	// Perfect quality, zero cost.
	r := n.BudgetAdjustedReward(1.0, 0.0)
	expected := 1.0*0.7 + 1.0*0.3 // = 1.0
	if math.Abs(r-expected) > 1e-9 {
		t.Errorf("expected %.2f, got %.2f", expected, r)
	}

	// Perfect quality, maximum cost.
	r = n.BudgetAdjustedReward(1.0, 1.0)
	expected = 1.0*0.7 + 0.0*0.3 // = 0.7
	if math.Abs(r-expected) > 1e-9 {
		t.Errorf("expected %.2f, got %.2f", expected, r)
	}

	// Zero quality, zero cost.
	r = n.BudgetAdjustedReward(0.0, 0.0)
	expected = 0.0*0.7 + 1.0*0.3 // = 0.3
	if math.Abs(r-expected) > 1e-9 {
		t.Errorf("expected %.2f, got %.2f", expected, r)
	}

	// Half quality, half cost.
	r = n.BudgetAdjustedReward(0.5, 0.5)
	expected = 0.5*0.7 + 0.5*0.3 // = 0.5
	if math.Abs(r-expected) > 1e-9 {
		t.Errorf("expected %.2f, got %.2f", expected, r)
	}
}

func TestNeuralUCB_BudgetWeightZero(t *testing.T) {
	n := NewNeuralUCB(nil, 4, NeuralUCBConfig{BudgetWeight: 0})
	// BudgetWeight=0 should use default 0.3.
	r := n.BudgetAdjustedReward(1.0, 1.0)
	expected := 0.7
	if math.Abs(r-expected) > 1e-9 {
		t.Errorf("expected %.2f with default budget weight, got %.2f", expected, r)
	}
}

func TestNeuralUCB_ArmStatsAfterUpdates(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{})

	for i := 0; i < 10; i++ {
		n.Update(Reward{ArmID: "a", Value: 0.8, Timestamp: time.Now()})
	}

	stats := n.ArmStats()
	s := stats["a"]
	if s.Pulls != 10 {
		t.Errorf("expected 10 pulls, got %d", s.Pulls)
	}
	if math.Abs(s.MeanReward-0.8) > 1e-9 {
		t.Errorf("expected mean 0.8, got %f", s.MeanReward)
	}
}

func TestNeuralUCB_ContextPadding(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	n := NewNeuralUCB(arms, 4, NeuralUCBConfig{HiddenSize: 4})

	// Short context should be zero-padded.
	short := []float64{1.0}
	p1 := n.Predict("a", short)
	if p1 <= 0 || p1 >= 1 {
		t.Errorf("prediction should be in (0,1), got %f", p1)
	}

	// Long context should be truncated.
	long := []float64{1, 2, 3, 4, 5, 6, 7, 8}
	p2 := n.Predict("a", long)
	if p2 <= 0 || p2 >= 1 {
		t.Errorf("prediction should be in (0,1), got %f", p2)
	}

	// Nil context should work.
	p3 := n.Predict("a", nil)
	if p3 <= 0 || p3 >= 1 {
		t.Errorf("prediction should be in (0,1), got %f", p3)
	}
}

func TestNeuralUCB_ConcurrentSelectUpdate(t *testing.T) {
	n := NewNeuralUCB(neuralArms, int(NumContextualFeatures), NeuralUCBConfig{})

	var wg sync.WaitGroup
	armIDs := []string{"cheap", "worker", "coding", "reasoning"}

	// 20 goroutines selecting.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := []float64{0.1, -0.1, 0.5, 0.0}
			for j := 0; j < 50; j++ {
				arm := n.Select(ctx)
				if arm.ID == "" {
					t.Error("Select returned empty arm")
				}
			}
		}()
	}

	// 20 goroutines updating.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(seed int) {
			defer wg.Done()
			rng := rand.New(rand.NewPCG(uint64(seed), uint64(seed+1)))
			for j := 0; j < 50; j++ {
				n.Update(Reward{
					ArmID:     armIDs[rng.IntN(len(armIDs))],
					Value:     rng.Float64(),
					Timestamp: time.Now(),
					Context:   []float64{rng.Float64(), rng.Float64(), rng.Float64(), rng.Float64()},
				})
			}
		}(i)
	}

	wg.Wait()

	stats := n.ArmStats()
	if len(stats) != 4 {
		t.Errorf("expected 4 arm stats, got %d", len(stats))
	}
}

func TestRelu(t *testing.T) {
	cases := []struct {
		in, out float64
	}{
		{-1.0, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{100.0, 100.0},
	}
	for _, tc := range cases {
		got := relu(tc.in)
		if got != tc.out {
			t.Errorf("relu(%f) = %f, want %f", tc.in, got, tc.out)
		}
	}
}
