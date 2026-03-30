package session

import "testing"

func TestSuggestCheaperModel_BelowThreshold(t *testing.T) {
	model, changed := SuggestCheaperModel(ProviderClaude, "claude-opus-4-20250514", 0.30)
	if !changed {
		t.Fatal("expected cheaper model suggestion for low budget")
	}
	if model == "claude-opus-4-20250514" {
		t.Error("should have suggested a different model")
	}
}

func TestSuggestCheaperModel_AboveThreshold(t *testing.T) {
	model, changed := SuggestCheaperModel(ProviderClaude, "claude-opus-4-20250514", 5.00)
	if changed {
		t.Error("should not suggest cheaper model above threshold")
	}
	if model != "claude-opus-4-20250514" {
		t.Error("should return original model")
	}
}

func TestSuggestCheaperModel_AlreadyCheapest(t *testing.T) {
	model, changed := SuggestCheaperModel(ProviderClaude, "claude-haiku-3-5-20241022", 0.10)
	if changed {
		t.Error("already cheapest model should not change")
	}
	if model != "claude-haiku-3-5-20241022" {
		t.Error("should return original model")
	}
}
