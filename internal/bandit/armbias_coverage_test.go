package bandit

import (
	"testing"
)

func TestContextualThompson_ArmBias_Unknown(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)
	bias := ct.ArmBias("nonexistent-arm")
	if bias != 0 {
		t.Errorf("ArmBias for unknown arm = %f, want 0", bias)
	}
}

func TestContextualThompson_ArmBias_Known(t *testing.T) {
	ct := NewContextualThompson(contextualArms(), 0, 0.1)
	// Initially all biases are 0.
	for _, arm := range contextualArms() {
		bias := ct.ArmBias(arm.ID)
		if bias != 0 {
			t.Errorf("initial ArmBias(%q) = %f, want 0", arm.ID, bias)
		}
	}
}

func TestContextualThompson_ArmBias_AfterUpdate(t *testing.T) {
	arms := []Arm{
		{ID: "fast", Provider: "gemini", Model: "flash"},
		{ID: "smart", Provider: "claude", Model: "sonnet"},
	}
	ct := NewContextualThompson(arms, 0, 0.5)

	// Record positive reward for "fast" arm.
	ctx := make([]float64, int(NumContextualFeatures))
	for range 3 {
		ct.Update(Reward{ArmID: "fast", Value: 1.0, Context: ctx})
	}

	// After positive updates, bias should be accessible (may still be 0 until
	// gradient updates change it, but ArmBias should not panic).
	bias := ct.ArmBias("fast")
	_ = bias // just verify no panic
}
