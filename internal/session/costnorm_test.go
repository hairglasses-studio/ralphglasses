package session

import (
	"math"
	"testing"
)

func TestNormalizeProviderCostZero(t *testing.T) {
	n := NormalizeProviderCost(ProviderClaude, 0, 0, 0)
	if n.NormalizedUSD != 0 {
		t.Errorf("zero raw cost: normalized = %.4f, want 0", n.NormalizedUSD)
	}
	if n.EfficiencyPct != 100 {
		t.Errorf("zero raw cost: efficiency = %.1f, want 100", n.EfficiencyPct)
	}
}

func TestNormalizeProviderCostClaudeIdentity(t *testing.T) {
	// Claude normalizes to itself; efficiency should be ~100%.
	n := NormalizeProviderCost(ProviderClaude, 1.00, 100_000, 50_000)
	if n.EfficiencyPct <= 0 {
		t.Errorf("claude efficiency = %.2f, want > 0", n.EfficiencyPct)
	}
}

func TestNormalizeProviderCostGeminiCheaperThanClaude(t *testing.T) {
	// Gemini is priced lower than Claude; efficiency should be < 100.
	n := NormalizeProviderCost(ProviderGemini, 0.05, 0, 0)
	if n.EfficiencyPct >= 100 {
		t.Errorf("gemini efficiency = %.2f, expected < 100 (cheaper than claude)", n.EfficiencyPct)
	}
}

func TestNormalizeProviderCostWithTokenCounts(t *testing.T) {
	// 1M input tokens at Claude rate = $3.00
	n := NormalizeProviderCost(ProviderGemini, 1.25, 1_000_000, 0)
	want := 3.00
	if math.Abs(n.NormalizedUSD-want) > 0.01 {
		t.Errorf("normalized = %.4f, want %.4f", n.NormalizedUSD, want)
	}
}

func TestNormalizeSessionCostLocked(t *testing.T) {
	s := &Session{
		Provider: ProviderCodex,
		SpentUSD: 0.25,
	}
	n := NormalizeSessionCost(s)
	if n.Provider != ProviderCodex {
		t.Errorf("provider = %q, want codex", n.Provider)
	}
	if n.RawCostUSD != 0.25 {
		t.Errorf("raw cost = %.4f, want 0.25", n.RawCostUSD)
	}
}

func TestProviderCostRatesAllProvidersPresent(t *testing.T) {
	for _, p := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex} {
		if _, ok := ProviderCostRates[p]; !ok {
			t.Errorf("missing cost rate for provider %q", p)
		}
	}
}
