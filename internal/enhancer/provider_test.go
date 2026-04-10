package enhancer

import (
	"testing"
)

func TestNewPromptImprover_DefaultsToOpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	client := NewPromptImprover(LLMConfig{Enabled: true})
	if client == nil {
		t.Fatal("expected non-nil client for default provider")
	}
	if client.Provider() != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, client.Provider())
	}
}

func TestNewPromptImprover_Gemini(t *testing.T) {
	t.Setenv("GOOGLE_API_KEY", "test-gemini-key")
	client := NewPromptImprover(LLMConfig{Enabled: true, Provider: "gemini"})
	if client == nil {
		t.Fatal("expected non-nil client for gemini")
	}
	if client.Provider() != ProviderGemini {
		t.Errorf("expected provider %q, got %q", ProviderGemini, client.Provider())
	}
}

func TestNewPromptImprover_OpenAI(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-openai-key")
	client := NewPromptImprover(LLMConfig{Enabled: true, Provider: "openai"})
	if client == nil {
		t.Fatal("expected non-nil client for openai")
	}
	if client.Provider() != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, client.Provider())
	}
}

func TestNewPromptImprover_OllamaAlias(t *testing.T) {
	t.Setenv("OLLAMA_BASE_URL", "http://127.0.0.1:11434")
	t.Setenv("OLLAMA_API_KEY", "")
	client := NewPromptImprover(LLMConfig{Enabled: true, Provider: "ollama"})
	if client == nil {
		t.Fatal("expected non-nil client for ollama")
	}
	if client.Provider() != ProviderOpenAI {
		t.Errorf("expected provider %q, got %q", ProviderOpenAI, client.Provider())
	}
}

func TestNewPromptImprover_NilWhenNoKey(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	for _, provider := range []string{"", "claude", "gemini", "openai"} {
		client := NewPromptImprover(LLMConfig{Provider: provider})
		if client != nil {
			t.Errorf("expected nil client for provider %q with no API key", provider)
		}
	}
}

func TestCacheKeyDiffersAcrossProviders(t *testing.T) {
	t.Parallel()
	cache := NewPromptCache()

	opts1 := ImproveOptions{Provider: ProviderClaude}
	opts2 := ImproveOptions{Provider: ProviderGemini}
	opts3 := ImproveOptions{Provider: ProviderOpenAI}

	prompt := "fix the bug in authentication"

	k1 := cache.key(prompt, opts1)
	k2 := cache.key(prompt, opts2)
	k3 := cache.key(prompt, opts3)

	if k1 == k2 {
		t.Error("cache keys for claude and gemini should differ")
	}
	if k1 == k3 {
		t.Error("cache keys for claude and openai should differ")
	}
	if k2 == k3 {
		t.Error("cache keys for gemini and openai should differ")
	}
}

func TestDefaultTargetProviderForLLM_Codex(t *testing.T) {
	got := defaultTargetProviderForLLM("codex")
	if got != ProviderOpenAI {
		t.Errorf("defaultTargetProviderForLLM(codex) = %q, want %q", got, ProviderOpenAI)
	}
	got = defaultTargetProviderForLLM("openai")
	if got != ProviderOpenAI {
		t.Errorf("defaultTargetProviderForLLM(openai) = %q, want %q", got, ProviderOpenAI)
	}
	got = defaultTargetProviderForLLM("ollama")
	if got != ProviderOpenAI {
		t.Errorf("defaultTargetProviderForLLM(ollama) = %q, want %q", got, ProviderOpenAI)
	}
}
