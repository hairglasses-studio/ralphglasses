package session

import "slices"

// ModelInfo describes an LLM model's capabilities and cost profile.
type ModelInfo struct {
	ID             string   `json:"id"`
	Provider       Provider `json:"provider"`
	DisplayName    string   `json:"display_name"`
	ContextWindow  int      `json:"context_window"`
	CostPerMTokIn  float64  `json:"cost_per_mtok_in"`  // USD per 1M input tokens
	CostPerMTokOut float64  `json:"cost_per_mtok_out"` // USD per 1M output tokens
	MaxOutputTok   int      `json:"max_output_tok"`
	Capabilities   []string `json:"capabilities"` // e.g. "code", "reasoning", "vision"
}

// modelRegistry is the built-in registry of known models.
var modelRegistry = []ModelInfo{
	// Claude models
	{ID: "claude-opus-4-20250514", Provider: ProviderClaude, DisplayName: "Claude Opus 4", ContextWindow: 200000, CostPerMTokIn: 15.0, CostPerMTokOut: 75.0, MaxOutputTok: 32000, Capabilities: []string{"code", "reasoning", "vision"}},
	{ID: "claude-sonnet-4-20250514", Provider: ProviderClaude, DisplayName: "Claude Sonnet 4", ContextWindow: 200000, CostPerMTokIn: 3.0, CostPerMTokOut: 15.0, MaxOutputTok: 16000, Capabilities: []string{"code", "reasoning", "vision"}},
	{ID: "claude-haiku-3-5-20241022", Provider: ProviderClaude, DisplayName: "Claude Haiku 3.5", ContextWindow: 200000, CostPerMTokIn: 0.80, CostPerMTokOut: 4.0, MaxOutputTok: 8192, Capabilities: []string{"code", "vision"}},

	// Gemini models
	{ID: "gemini-3.1-pro", Provider: ProviderGemini, DisplayName: "Gemini 2.5 Pro", ContextWindow: 1000000, CostPerMTokIn: 1.25, CostPerMTokOut: 10.0, MaxOutputTok: 65536, Capabilities: []string{"code", "reasoning", "vision"}},
	{ID: "gemini-3.1-flash", Provider: ProviderGemini, DisplayName: "Gemini 2.5 Flash", ContextWindow: 1000000, CostPerMTokIn: 0.15, CostPerMTokOut: 0.60, MaxOutputTok: 65536, Capabilities: []string{"code", "vision"}},

	// OpenAI / Codex models
	{ID: "gpt-5.4", Provider: ProviderCodex, DisplayName: "OpenAI GPT-5.4", ContextWindow: 1050000, CostPerMTokIn: 2.50, CostPerMTokOut: 15.0, MaxOutputTok: 128000, Capabilities: []string{"code", "reasoning", "vision"}},
	{ID: "o3", Provider: ProviderCodex, DisplayName: "OpenAI o3", ContextWindow: 200000, CostPerMTokIn: 2.0, CostPerMTokOut: 8.0, MaxOutputTok: 100000, Capabilities: []string{"code", "reasoning"}},
	{ID: "o4-mini", Provider: ProviderCodex, DisplayName: "OpenAI o4-mini", ContextWindow: 200000, CostPerMTokIn: 1.10, CostPerMTokOut: 4.40, MaxOutputTok: 100000, Capabilities: []string{"code", "reasoning"}},

	// Shared local Ollama defaults
	{ID: "code-fast", Provider: ProviderOllama, DisplayName: "Code Fast", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "code-compact", Provider: ProviderOllama, DisplayName: "Code Compact", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "code-primary", Provider: ProviderOllama, DisplayName: "Code Primary", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"chat", "code", "reasoning"}},
	{ID: "code-reasoner", Provider: ProviderOllama, DisplayName: "Code Reasoner", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "code-long", Provider: ProviderOllama, DisplayName: "Code Long", ContextWindow: 262144, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 32768, Capabilities: []string{"code", "reasoning"}},
	{ID: "code-heavy", Provider: ProviderOllama, DisplayName: "Code Heavy", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "qwen3:8b", Provider: ProviderOllama, DisplayName: "Qwen3 8B", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"chat", "reasoning"}},
	{ID: "qwen2.5-coder:7b", Provider: ProviderOllama, DisplayName: "Qwen2.5 Coder 7B", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "devstral-small-2", Provider: ProviderOllama, DisplayName: "Devstral Small 2", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "devstral-2", Provider: ProviderOllama, DisplayName: "Devstral 2", ContextWindow: 131072, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 16384, Capabilities: []string{"code", "reasoning"}},
	{ID: "qwen3-coder-next", Provider: ProviderOllama, DisplayName: "Qwen3 Coder Next", ContextWindow: 262144, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 32768, Capabilities: []string{"code", "reasoning"}},
	{ID: "glm-5.1:cloud", Provider: ProviderOllama, DisplayName: "GLM 5.1 Cloud", ContextWindow: 0, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 0, Capabilities: []string{"code", "reasoning", "cloud"}},
	{ID: "glm-5:cloud", Provider: ProviderOllama, DisplayName: "GLM 5 Cloud", ContextWindow: 0, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 0, Capabilities: []string{"code", "reasoning", "cloud"}},
	{ID: "minimax-m2.1:cloud", Provider: ProviderOllama, DisplayName: "MiniMax M2.1 Cloud", ContextWindow: 0, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 0, Capabilities: []string{"code", "reasoning", "multilingual", "cloud"}},
	{ID: "kimi-k2-thinking:cloud", Provider: ProviderOllama, DisplayName: "Kimi K2 Thinking Cloud", ContextWindow: 0, CostPerMTokIn: 0, CostPerMTokOut: 0, MaxOutputTok: 0, Capabilities: []string{"code", "reasoning", "cloud"}},
}

// ListModels returns all known models, optionally filtered by provider.
func ListModels(provider Provider) []ModelInfo {
	if provider == "" {
		result := make([]ModelInfo, len(modelRegistry))
		copy(result, modelRegistry)
		return result
	}
	var result []ModelInfo
	for _, m := range modelRegistry {
		if m.Provider == provider {
			result = append(result, m)
		}
	}
	return result
}

// LookupModel finds a model by ID. Returns nil if not found.
func LookupModel(id string) *ModelInfo {
	for i := range modelRegistry {
		if modelRegistry[i].ID == id {
			return &modelRegistry[i]
		}
	}
	return nil
}

// CheapestModel returns the cheapest model for a provider by input token cost.
func CheapestModel(provider Provider) *ModelInfo {
	var best *ModelInfo
	for i := range modelRegistry {
		if modelRegistry[i].Provider != provider {
			continue
		}
		if best == nil || modelRegistry[i].CostPerMTokIn < best.CostPerMTokIn {
			best = &modelRegistry[i]
		}
	}
	return best
}

// HasCapability checks if a model has a specific capability.
func (m ModelInfo) HasCapability(cap string) bool {
	return slices.Contains(m.Capabilities, cap)
}
