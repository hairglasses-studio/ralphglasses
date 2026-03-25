package bandit

import (
	"testing"
	"time"
)

var ucbTestArms = []Arm{
	{ID: "a", Provider: "claude", Model: "opus"},
	{ID: "b", Provider: "gemini", Model: "pro"},
	{ID: "c", Provider: "openai", Model: "o3"},
}

func TestNewDiscountedUCB1(t *testing.T) {
	d := NewDiscountedUCB1(ucbTestArms, 0.99)
	if len(d.arms) != 3 {
		t.Fatalf("expected 3 arms, got %d", len(d.arms))
	}
	if d.gamma != 0.99 {
		t.Fatalf("expected gamma 0.99, got %f", d.gamma)
	}
	if d.totalN != 0 {
		t.Fatal("expected totalN 0")
	}
	stats := d.ArmStats()
	for _, id := range []string{"a", "b", "c"} {
		if stats[id].Pulls != 0 {
			t.Fatalf("arm %s should have 0 pulls", id)
		}
	}
}

func TestUCB1ExploresFirst(t *testing.T) {
	d := NewDiscountedUCB1(ucbTestArms, 0.99)
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		arm := d.Select(nil)
		if seen[arm.ID] {
			t.Fatalf("arm %s selected twice during exploration", arm.ID)
		}
		seen[arm.ID] = true
		d.Update(Reward{ArmID: arm.ID, Value: 0.5, Timestamp: time.Now()})
	}
	if len(seen) != 3 {
		t.Fatalf("expected 3 unique arms, got %d", len(seen))
	}
}

func TestUCB1ConvergesOnBest(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}, {ID: "b", Provider: "p", Model: "m"}}
	d := NewDiscountedUCB1(arms, 0.99)

	// Warm up: pull each arm once.
	for _, id := range []string{"a", "b"} {
		d.Update(Reward{ArmID: id, Value: map[string]float64{"a": 0.9, "b": 0.1}[id], Timestamp: time.Now()})
	}

	// Train for 100 steps.
	for i := 0; i < 100; i++ {
		arm := d.Select(nil)
		val := 0.1
		if arm.ID == "a" {
			val = 0.9
		}
		d.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now()})
	}

	// Check next 100 selections.
	aCount := 0
	for i := 0; i < 100; i++ {
		arm := d.Select(nil)
		if arm.ID == "a" {
			aCount++
		}
		val := 0.1
		if arm.ID == "a" {
			val = 0.9
		}
		d.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now()})
	}
	if aCount < 80 {
		t.Fatalf("expected arm 'a' selected >80 times, got %d", aCount)
	}
}

func TestUCB1DiscountAdapts(t *testing.T) {
	arms := []Arm{{ID: "a", Provider: "p", Model: "m"}, {ID: "b", Provider: "p", Model: "m"}}
	d := NewDiscountedUCB1(arms, 0.95)

	// Phase 1: arm "a" is best.
	for i := 0; i < 50; i++ {
		arm := d.Select(nil)
		val := 0.1
		if arm.ID == "a" {
			val = 0.9
		}
		d.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now()})
	}

	// Phase 2: arm "b" becomes best.
	for i := 0; i < 50; i++ {
		arm := d.Select(nil)
		val := 0.1
		if arm.ID == "b" {
			val = 0.9
		}
		d.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now()})
	}

	// After adaptation, "b" should dominate next 50 selections.
	bCount := 0
	for i := 0; i < 50; i++ {
		arm := d.Select(nil)
		if arm.ID == "b" {
			bCount++
		}
		val := 0.1
		if arm.ID == "b" {
			val = 0.9
		}
		d.Update(Reward{ArmID: arm.ID, Value: val, Timestamp: time.Now()})
	}
	if bCount < 25 {
		t.Fatalf("expected arm 'b' selected >25/50 times after switch, got %d", bCount)
	}
}

func TestUCB1EmptyArms(t *testing.T) {
	d := NewDiscountedUCB1(nil, 0.99)
	arm := d.Select(nil)
	if arm.ID != "" {
		t.Fatalf("expected empty arm, got %q", arm.ID)
	}
}

func TestUCB1GammaClamp(t *testing.T) {
	d1 := NewDiscountedUCB1(nil, 0)
	if d1.gamma != 0.99 {
		t.Fatalf("gamma=0 should clamp to 0.99, got %f", d1.gamma)
	}
	d2 := NewDiscountedUCB1(nil, 1.5)
	if d2.gamma != 0.99 {
		t.Fatalf("gamma=1.5 should clamp to 0.99, got %f", d2.gamma)
	}
	d3 := NewDiscountedUCB1(nil, 1.0)
	if d3.gamma != 1.0 {
		t.Fatalf("gamma=1.0 should be valid, got %f", d3.gamma)
	}
}
