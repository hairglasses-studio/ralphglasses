package observability

import "testing"

func TestResolveGenAISystem(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		baseURL  string
		fallback string
		want     string
	}{
		{name: "remote fallback", baseURL: "https://api.openai.com", fallback: "openai", want: "openai"},
		{name: "empty fallback", baseURL: "", fallback: "", want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveGenAISystem(tt.baseURL, tt.fallback); got != tt.want {
				t.Fatalf("ResolveGenAISystem(%q, %q) = %q, want %q", tt.baseURL, tt.fallback, got, tt.want)
			}
		})
	}
}

func TestEstimateLLMCostUSD(t *testing.T) {
	t.Parallel()

	if got := EstimateLLMCostUSD("openai", "o3", 1000, 500); got <= 0 {
		t.Fatalf("EstimateLLMCostUSD remote model = %f, want > 0", got)
	}
}
