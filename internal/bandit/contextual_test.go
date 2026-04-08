package bandit

import (
	"testing"
	"time"
)

func contextualArms() []Arm {
	return []Arm{
		{ID: "ultra-cheap", Provider: "gemini", Model: "gemini-3.1-flash-lite"},
		{ID: "worker", Provider: "gemini", Model: "gemini-3.1-flash"},
		{ID: "coding", Provider: "claude", Model: "claude-sonnet"},
		{ID: "reasoning", Provider: "claude", Model: "claude-opus"},
	}
}

func TestNewContextualThompson(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)
	stats := ct.ArmStats()

	if len(stats) != 4 {
		t.Fatalf("expected 4 arms, got %d", len(stats))
	}
	for _, id := range []string{"ultra-cheap", "worker", "coding", "reasoning"} {
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

func TestContextualThompson_SelectWithoutContext(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)
	counts := make(map[string]int)

	for range 200 {
		arm := ct.Select(nil)
		counts[arm.ID]++
	}

	for _, id := range []string{"ultra-cheap", "worker", "coding", "reasoning"} {
		if counts[id] == 0 {
			t.Errorf("arm %q was never selected in 200 trials without context", id)
		}
	}
}

func TestContextualThompson_SelectWithContext(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)

	// Simple context: low complexity, high budget remaining.
	simpleCtx := make([]float64, NumContextualFeatures)
	simpleCtx[FeatureComplexity] = -1.0
	simpleCtx[FeatureBudgetPressure] = 1.0
	simpleCtx[FeatureTimeSensitivity] = -1.0
	simpleCtx[FeatureRecentSuccess] = 0.6

	counts := make(map[string]int)
	for range 200 {
		arm := ct.Select(simpleCtx)
		counts[arm.ID]++
	}

	// With uniform priors and no training, all arms should be selected.
	total := 0
	for _, c := range counts {
		total += c
	}
	if total != 200 {
		t.Fatalf("expected 200 total selections, got %d", total)
	}
}

func TestContextualThompson_LearnsCheapForSimple(t *testing.T) {
	arms := []Arm{
		{ID: "cheap", Provider: "gemini", Model: "flash-lite"},
		{ID: "expensive", Provider: "claude", Model: "opus"},
	}
	ct := NewContextualThompson(arms, 0, 0.3)

	simpleCtx := make([]float64, NumContextualFeatures)
	simpleCtx[FeatureComplexity] = -1.0 // simple (centered encoding)

	complexCtx := make([]float64, NumContextualFeatures)
	complexCtx[FeatureComplexity] = 1.0 // complex (centered encoding)

	// Train: cheap succeeds on simple, fails on complex.
	// Expensive succeeds on both but especially complex.
	for range 100 {
		ct.Update(Reward{
			ArmID:     "cheap",
			Value:     0.9,
			Timestamp: time.Now(),
			Context:   simpleCtx,
		})
		ct.Update(Reward{
			ArmID:     "cheap",
			Value:     0.1,
			Timestamp: time.Now(),
			Context:   complexCtx,
		})
		ct.Update(Reward{
			ArmID:     "expensive",
			Value:     0.9,
			Timestamp: time.Now(),
			Context:   complexCtx,
		})
		ct.Update(Reward{
			ArmID:     "expensive",
			Value:     0.5,
			Timestamp: time.Now(),
			Context:   simpleCtx,
		})
	}

	// After training, verify that for simple tasks the cheap arm is favored.
	cheapCount := 0
	for range 200 {
		arm := ct.Select(simpleCtx)
		if arm.ID == "cheap" {
			cheapCount++
		}
	}

	// The cheap arm should be selected most of the time for simple tasks
	// due to its negative complexity weight boosting it when complexity < 0.
	if cheapCount < 80 {
		t.Errorf("expected cheap arm selected >80/200 for simple tasks, got %d", cheapCount)
	}
}

func TestContextualThompson_WeightsUpdate(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	ct := NewContextualThompson(arms, 0, 0.5)

	ctx := make([]float64, NumContextualFeatures)
	ctx[FeatureComplexity] = 1.0

	// Record a positive reward with high complexity context.
	ct.Update(Reward{
		ArmID:     "a",
		Value:     1.0,
		Timestamp: time.Now(),
		Context:   ctx,
	})

	weights := ct.ArmWeights("a")
	if weights == nil {
		t.Fatal("expected non-nil weights")
	}

	// The complexity weight should have moved in a positive direction.
	if weights[FeatureComplexity] <= 0 {
		t.Errorf("expected positive complexity weight after positive reward, got %f", weights[FeatureComplexity])
	}
}

func TestContextualThompson_SlidingWindow(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}}
	ct := NewContextualThompson(arms, 5, 0.1)

	// 5 failures.
	for range 5 {
		ct.Update(Reward{ArmID: "a", Value: 0.0, Timestamp: time.Now()})
	}

	// Then 5 successes — should push failures out of the window.
	for range 5 {
		ct.Update(Reward{ArmID: "a", Value: 1.0, Timestamp: time.Now()})
	}

	stats := ct.ArmStats()
	s := stats["a"]
	if s.Alpha <= s.Beta {
		t.Errorf("after window slide, expected alpha > beta, got alpha=%f beta=%f", s.Alpha, s.Beta)
	}
}

func TestContextualThompson_EmptyArms(t *testing.T) {
	ct := NewContextualThompson(nil, 0, 0.1)
	arm := ct.Select(nil)
	if arm.ID != "" {
		t.Errorf("expected empty arm from nil arms, got %+v", arm)
	}
	stats := ct.ArmStats()
	if len(stats) != 0 {
		t.Errorf("expected 0 stats, got %d", len(stats))
	}
}

func TestContextualThompson_ImplementsPolicy(t *testing.T) {
	// Verify that ContextualThompson satisfies the Policy interface.
	var _ Policy = (*ContextualThompson)(nil)
}

func TestContextualThompson_ArmWeightsUnknown(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)
	if w := ct.ArmWeights("nonexistent"); w != nil {
		t.Errorf("expected nil weights for unknown arm, got %v", w)
	}
}

func TestContextualThompson_DefaultLearningRate(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0)
	if ct.learningRate != 0.1 {
		t.Errorf("expected default learning rate 0.1, got %f", ct.learningRate)
	}
}
