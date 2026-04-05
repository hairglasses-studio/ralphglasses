package loop

import (
	"testing"
)

func TestController_FullLifecycle(t *testing.T) {
	c := NewController(100.0)

	// Start: Idle -> Planning
	if err := c.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	if c.State() != StatePlanning {
		t.Fatalf("expected planning, got %s", c.State())
	}

	// Advance: Planning -> Executing
	if err := c.Advance(); err != nil {
		t.Fatalf("advance to executing: %v", err)
	}
	if c.State() != StateExecuting {
		t.Fatalf("expected executing, got %s", c.State())
	}

	// Advance: Executing -> Evaluating
	if err := c.Advance(); err != nil {
		t.Fatalf("advance to evaluating: %v", err)
	}
	if c.State() != StateEvaluating {
		t.Fatalf("expected evaluating, got %s", c.State())
	}

	// Complete: Evaluating -> Complete
	if err := c.Complete(); err != nil {
		t.Fatalf("complete: %v", err)
	}
	if c.State() != StateComplete {
		t.Fatalf("expected complete, got %s", c.State())
	}
}

func TestController_ImprovementLoop(t *testing.T) {
	c := NewController(100.0)

	_ = c.Start() // -> Planning

	// Run two full improvement cycles
	for cycle := 0; cycle < 2; cycle++ {
		if err := c.Advance(); err != nil { // -> Executing
			t.Fatalf("cycle %d, executing: %v", cycle, err)
		}
		if err := c.Advance(); err != nil { // -> Evaluating
			t.Fatalf("cycle %d, evaluating: %v", cycle, err)
		}
		if err := c.Advance(); err != nil { // -> Improving
			t.Fatalf("cycle %d, improving: %v", cycle, err)
		}
		if err := c.Advance(); err != nil { // -> Planning (loop back)
			t.Fatalf("cycle %d, planning: %v", cycle, err)
		}
	}

	if c.State() != StatePlanning {
		t.Fatalf("expected planning after improvement loops, got %s", c.State())
	}
}

func TestController_BudgetTriggeredCooldown(t *testing.T) {
	c := NewController(100.0)
	_ = c.Start() // -> Planning

	// Record spend at 91% -> should trigger cooldown
	c.RecordSpend(91.0)

	if c.State() != StateCooldown {
		t.Fatalf("expected cooldown after 91%% spend, got %s", c.State())
	}

	// Verify progress snapshot reflects the spend
	p := c.Progress()
	if p.State != "cooldown" {
		t.Fatalf("progress state: expected 'cooldown', got %q", p.State)
	}
	if p.SpentUSD < 90.0 {
		t.Fatalf("progress spent: expected >= 90, got %f", p.SpentUSD)
	}
	if p.BudgetUSD != 100.0 {
		t.Fatalf("progress budget: expected 100, got %f", p.BudgetUSD)
	}
}

func TestController_CooldownDoesNotTriggerFromTerminal(t *testing.T) {
	c := NewController(100.0)

	// Go to complete state
	_ = c.Start()
	_ = c.Advance() // Executing
	_ = c.Advance() // Evaluating
	_ = c.Complete() // Complete

	// Record massive spend from complete state: should stay complete
	c.RecordSpend(200.0)
	if c.State() != StateComplete {
		t.Fatalf("expected complete (not cooldown) from terminal state, got %s", c.State())
	}
}

func TestController_Cancel(t *testing.T) {
	c := NewController(100.0)
	_ = c.Start() // -> Planning

	c.Cancel()

	if c.State() != StateExit {
		t.Fatalf("expected exit after cancel, got %s", c.State())
	}
}

func TestController_CancelFromIdle(t *testing.T) {
	c := NewController(100.0)
	c.Cancel()

	if c.State() != StateExit {
		t.Fatalf("expected exit after cancel from idle, got %s", c.State())
	}
}

func TestController_CancelIdempotent(t *testing.T) {
	c := NewController(100.0)
	c.Cancel()
	c.Cancel() // second call should not panic

	if c.State() != StateExit {
		t.Fatalf("expected exit, got %s", c.State())
	}
}

func TestController_AdvanceFromIdleFails(t *testing.T) {
	c := NewController(100.0)

	if err := c.Advance(); err == nil {
		t.Fatal("expected error when advancing from idle, got nil")
	}
}

func TestController_Progress(t *testing.T) {
	c := NewController(50.0)
	_ = c.Start()
	c.RecordSpend(10.0)

	p := c.Progress()
	if p.State != "planning" {
		t.Fatalf("expected 'planning', got %q", p.State)
	}
	if p.BudgetUSD != 50.0 {
		t.Fatalf("expected budget 50, got %f", p.BudgetUSD)
	}
	if p.SpentUSD != 10.0 {
		t.Fatalf("expected spent 10, got %f", p.SpentUSD)
	}
	if p.Transitions != 1 { // Idle -> Planning
		t.Fatalf("expected 1 transition, got %d", p.Transitions)
	}
}

func TestController_WarnDoesNotCooldown(t *testing.T) {
	c := NewController(100.0)
	_ = c.Start() // -> Planning

	// 80% triggers warn, not cooldown
	c.RecordSpend(80.0)

	if c.State() != StatePlanning {
		t.Fatalf("warn threshold should not trigger cooldown, got %s", c.State())
	}
}

func TestController_ZeroBudgetNeverCooldown(t *testing.T) {
	c := NewController(0) // unlimited
	_ = c.Start()

	c.RecordSpend(10000.0)
	if c.State() != StatePlanning {
		t.Fatalf("zero budget should never cooldown, got %s", c.State())
	}
}
