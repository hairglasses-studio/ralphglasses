package session

import "testing"

func TestListModels(t *testing.T) {
	all := ListModels("")
	if len(all) == 0 {
		t.Fatal("expected non-empty model list")
	}

	claude := ListModels(ProviderClaude)
	if len(claude) == 0 {
		t.Fatal("expected Claude models")
	}
	for _, m := range claude {
		if m.Provider != ProviderClaude {
			t.Errorf("expected Claude provider, got %s", m.Provider)
		}
	}

	gemini := ListModels(ProviderGemini)
	if len(gemini) == 0 {
		t.Fatal("expected Gemini models")
	}
}

func TestLookupModel(t *testing.T) {
	m := LookupModel("claude-sonnet-4-20250514")
	if m == nil {
		t.Fatal("expected to find claude-sonnet-4")
	}
	if m.Provider != ProviderClaude {
		t.Errorf("wrong provider: %s", m.Provider)
	}
	if m.ContextWindow != 200000 {
		t.Errorf("wrong context window: %d", m.ContextWindow)
	}

	if LookupModel("nonexistent-model") != nil {
		t.Error("expected nil for nonexistent model")
	}
}

func TestCheapestModel(t *testing.T) {
	cheapClaude := CheapestModel(ProviderClaude)
	if cheapClaude == nil {
		t.Fatal("expected cheapest Claude model")
	}
	// Haiku should be cheapest
	if cheapClaude.CostPerMTokIn >= 1.0 {
		t.Errorf("expected cheap model, got cost %f", cheapClaude.CostPerMTokIn)
	}
}

func TestHasCapability(t *testing.T) {
	m := ModelInfo{Capabilities: []string{"code", "vision"}}
	if !m.HasCapability("code") {
		t.Error("expected code capability")
	}
	if m.HasCapability("reasoning") {
		t.Error("did not expect reasoning capability")
	}
}
