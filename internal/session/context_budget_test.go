package session

import "testing"

func TestContextBudget_DefaultClaude(t *testing.T) {
	t.Parallel()
	b := DefaultContextBudget(ProviderClaude)
	if b.MaxTokens != 200000 {
		t.Errorf("expected 200000, got %d", b.MaxTokens)
	}
	if b.ResponseReserve != 20000 {
		t.Errorf("expected 20000 reserve, got %d", b.ResponseReserve)
	}
	if b.Available() != 180000 {
		t.Errorf("expected 180000 available, got %d", b.Available())
	}
}

func TestContextBudget_Allocate(t *testing.T) {
	t.Parallel()
	b := DefaultContextBudget(ProviderClaude)

	if err := b.Allocate("system_prompt", 5000); err != nil {
		t.Fatalf("allocate system_prompt: %v", err)
	}
	if b.SystemPrompt != 5000 {
		t.Errorf("expected 5000, got %d", b.SystemPrompt)
	}
	if b.Used() != 5000 {
		t.Errorf("expected 5000 used, got %d", b.Used())
	}
	if b.Available() != 175000 {
		t.Errorf("expected 175000 available, got %d", b.Available())
	}
}

func TestContextBudget_ExceedsFails(t *testing.T) {
	t.Parallel()
	b := ContextBudget{MaxTokens: 1000, ResponseReserve: 200}
	if err := b.Allocate("history", 801); err == nil {
		t.Error("expected error when exceeding budget")
	}
	if err := b.Allocate("history", 800); err != nil {
		t.Errorf("expected 800 to fit: %v", err)
	}
}

func TestContextBudget_Fits(t *testing.T) {
	t.Parallel()
	b := ContextBudget{MaxTokens: 1000, ResponseReserve: 100}
	if !b.Fits(900) {
		t.Error("900 should fit in 900 available")
	}
	if b.Fits(901) {
		t.Error("901 should not fit in 900 available")
	}
}

func TestContextBudget_Utilization(t *testing.T) {
	t.Parallel()
	b := ContextBudget{MaxTokens: 1000, ResponseReserve: 0}
	_ = b.Allocate("history", 500)
	pct := b.UtilizationPct()
	if pct != 50.0 {
		t.Errorf("expected 50%%, got %.1f%%", pct)
	}
}

func TestContextBudget_UnknownSection(t *testing.T) {
	t.Parallel()
	b := DefaultContextBudget(ProviderGemini)
	if err := b.Allocate("bogus", 100); err == nil {
		t.Error("expected error for unknown section")
	}
}

func TestContextBudget_Summary(t *testing.T) {
	t.Parallel()
	b := DefaultContextBudget(ProviderCodex)
	_ = b.Allocate("system_prompt", 3000)
	_ = b.Allocate("history", 7000)
	s := b.Summary()
	if s["used"].(int64) != 10000 {
		t.Errorf("expected 10000 used, got %v", s["used"])
	}
}
