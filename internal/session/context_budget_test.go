package session

import (
	"sync"
	"testing"
)

func TestContextBudget_NewDefaults(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(200000)
	if b.ModelLimit != 200000 {
		t.Errorf("ModelLimit = %d, want 200000", b.ModelLimit)
	}
	if b.UsedTokens != 0 {
		t.Errorf("UsedTokens = %d, want 0", b.UsedTokens)
	}
	if b.WarningThreshold != 0.8 {
		t.Errorf("WarningThreshold = %f, want 0.8", b.WarningThreshold)
	}
	if b.CriticalThreshold != 0.95 {
		t.Errorf("CriticalThreshold = %f, want 0.95", b.CriticalThreshold)
	}
}

func TestContextBudget_Record(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(100000)
	b.Record(5000)
	if b.UsedTokens != 5000 {
		t.Errorf("UsedTokens = %d, want 5000", b.UsedTokens)
	}
	b.Record(3000)
	if b.UsedTokens != 8000 {
		t.Errorf("UsedTokens = %d, want 8000", b.UsedTokens)
	}
}

func TestContextBudget_Usage(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(200000)
	b.Record(100000)

	used, limit, pct := b.Usage()
	if used != 100000 {
		t.Errorf("used = %d, want 100000", used)
	}
	if limit != 200000 {
		t.Errorf("limit = %d, want 200000", limit)
	}
	if pct != 0.5 {
		t.Errorf("percent = %f, want 0.5", pct)
	}
}

func TestContextBudget_UsageZeroLimit(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(0)
	b.Record(100)
	_, _, pct := b.Usage()
	if pct != 0 {
		t.Errorf("percent = %f, want 0 for zero limit", pct)
	}
}

func TestContextBudget_IsWarning(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(100)

	b.Record(80)
	if b.IsWarning() {
		t.Error("80/100 should not be warning (threshold is > 0.8)")
	}

	b.Record(1) // now 81/100 = 0.81
	if !b.IsWarning() {
		t.Error("81/100 should be warning")
	}
}

func TestContextBudget_IsCritical(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(100)

	b.Record(95)
	if b.IsCritical() {
		t.Error("95/100 should not be critical (threshold is > 0.95)")
	}

	b.Record(1) // now 96/100 = 0.96
	if !b.IsCritical() {
		t.Error("96/100 should be critical")
	}
}

func TestContextBudget_IsWarningZeroLimit(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(0)
	b.Record(100)
	if b.IsWarning() {
		t.Error("should not be warning with zero limit")
	}
	if b.IsCritical() {
		t.Error("should not be critical with zero limit")
	}
}

func TestContextBudget_Reset(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(100000)
	b.Record(50000)
	if b.UsedTokens != 50000 {
		t.Fatalf("UsedTokens = %d, want 50000", b.UsedTokens)
	}
	b.Reset()
	if b.UsedTokens != 0 {
		t.Errorf("UsedTokens after Reset = %d, want 0", b.UsedTokens)
	}
}

func TestContextBudget_Remaining(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(1000)
	b.Record(300)
	if r := b.Remaining(); r != 700 {
		t.Errorf("Remaining = %d, want 700", r)
	}
}

func TestContextBudget_RemainingOverflow(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(100)
	b.Record(150)
	if r := b.Remaining(); r != 0 {
		t.Errorf("Remaining = %d, want 0 when over limit", r)
	}
}

func TestContextBudget_Status(t *testing.T) {
	t.Parallel()

	t.Run("ok", func(t *testing.T) {
		t.Parallel()
		b := NewContextBudget(100)
		b.Record(50)
		if s := b.Status(); s != "ok" {
			t.Errorf("Status = %q, want ok", s)
		}
	})

	t.Run("warning", func(t *testing.T) {
		t.Parallel()
		b := NewContextBudget(100)
		b.Record(85)
		if s := b.Status(); s != "warning" {
			t.Errorf("Status = %q, want warning", s)
		}
	})

	t.Run("critical", func(t *testing.T) {
		t.Parallel()
		b := NewContextBudget(100)
		b.Record(96)
		if s := b.Status(); s != "critical" {
			t.Errorf("Status = %q, want critical", s)
		}
	})
}

func TestContextBudget_ProviderDefaults(t *testing.T) {
	t.Parallel()

	tests := []struct {
		provider Provider
		limit    int
	}{
		{ProviderClaude, DefaultClaudeLimit},
		{ProviderGemini, DefaultGeminiLimit},
		{ProviderCodex, DefaultCodexLimit},
		{Provider("unknown"), DefaultCodexLimit},
	}

	for _, tt := range tests {
		if got := ModelLimitForProvider(tt.provider); got != tt.limit {
			t.Errorf("ModelLimitForProvider(%q) = %d, want %d", tt.provider, got, tt.limit)
		}
	}
}

func TestContextBudget_ConcurrentAccess(t *testing.T) {
	t.Parallel()
	b := NewContextBudget(1000000)

	var wg sync.WaitGroup
	const goroutines = 100
	const tokensPerGoroutine = 1000

	// Concurrent writers.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Record(tokensPerGoroutine)
		}()
	}

	// Concurrent readers.
	for range goroutines {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b.Usage()
			b.IsWarning()
			b.IsCritical()
			b.Remaining()
			b.Status()
		}()
	}

	wg.Wait()

	used, _, _ := b.Usage()
	expected := goroutines * tokensPerGoroutine
	if used != expected {
		t.Errorf("used = %d, want %d after concurrent writes", used, expected)
	}
}
