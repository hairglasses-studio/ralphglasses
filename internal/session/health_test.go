package session

import "testing"

func TestCheckProviderHealthUnknown(t *testing.T) {
	h := CheckProviderHealth(Provider("unknown"))
	if h.Available {
		t.Error("unknown provider should not be available")
	}
	if h.Error == "" {
		t.Error("expected error for unknown provider")
	}
}

func TestCheckProviderHealthBinaryMissing(t *testing.T) {
	// gemini/codex binaries are unlikely to be on PATH in CI.
	// We just verify the struct fields are consistent.
	h := CheckProviderHealth(ProviderGemini)
	if h.Binary != "gemini" {
		t.Errorf("binary = %q, want %q", h.Binary, "gemini")
	}
	if h.Provider != ProviderGemini {
		t.Errorf("provider = %q, want %q", h.Provider, ProviderGemini)
	}
	// If not available, error must be set.
	if !h.Available && h.Error == "" {
		t.Error("unavailable provider should have error set")
	}
}

func TestProviderHealthHealthy(t *testing.T) {
	healthy := ProviderHealth{Available: true, EnvOK: true}
	if !healthy.Healthy() {
		t.Error("Available+EnvOK should be Healthy")
	}
	notHealthy := ProviderHealth{Available: true, EnvOK: false}
	if notHealthy.Healthy() {
		t.Error("missing EnvOK should not be Healthy")
	}
}

func TestHealthyProviders(t *testing.T) {
	health := map[Provider]ProviderHealth{
		ProviderClaude: {Provider: ProviderClaude, Available: true, EnvOK: true},
		ProviderGemini: {Provider: ProviderGemini, Available: false},
		ProviderCodex:  {Provider: ProviderCodex, Available: true, EnvOK: false},
	}
	healthy := HealthyProviders(health)
	if len(healthy) != 1 {
		t.Fatalf("expected 1 healthy provider, got %d: %v", len(healthy), healthy)
	}
	if healthy[0] != ProviderClaude {
		t.Errorf("expected claude, got %q", healthy[0])
	}
}

func TestCheckAllProviderHealthReturnsAllProviders(t *testing.T) {
	health := CheckAllProviderHealth()
	for _, p := range []Provider{ProviderClaude, ProviderGemini, ProviderCodex} {
		if _, ok := health[p]; !ok {
			t.Errorf("missing health entry for provider %q", p)
		}
	}
}
