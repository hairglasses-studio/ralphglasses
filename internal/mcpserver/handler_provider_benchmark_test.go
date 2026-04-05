package mcpserver

import (
	"testing"
)

func TestScorePromptQuality(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		prompt   string
		keywords []string
		provider string
		minScore float64
		maxScore float64
	}{
		{
			name:     "claude_base",
			prompt:   "Do something simple",
			keywords: []string{"func"},
			provider: "claude",
			minScore: 90,
			maxScore: 95,
		},
		{
			name:     "gemini_base",
			prompt:   "Do something simple",
			keywords: []string{"func"},
			provider: "gemini",
			minScore: 83,
			maxScore: 88,
		},
		{
			name:     "codex_base",
			prompt:   "Do something simple",
			keywords: []string{"func"},
			provider: "codex",
			minScore: 86,
			maxScore: 92,
		},
		{
			name:     "unknown_provider",
			prompt:   "hello",
			keywords: []string{"hello"},
			provider: "gpt4",
			minScore: 78,
			maxScore: 82,
		},
		{
			name:     "claude_explain_bonus",
			prompt:   "Explain the difference between channels and mutexes",
			keywords: []string{"channel", "mutex"},
			provider: "claude",
			minScore: 93,
			maxScore: 100,
		},
		{
			name:     "codex_write_bonus",
			prompt:   "Write a Go function that reverses a singly linked list",
			keywords: []string{"func", "ListNode"},
			provider: "codex",
			minScore: 88,
			maxScore: 95,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := scorePromptQuality(tt.prompt, tt.keywords, tt.provider)
			if got < tt.minScore || got > tt.maxScore {
				t.Errorf("scorePromptQuality() = %f, want in [%f, %f]", got, tt.minScore, tt.maxScore)
			}
		})
	}
}
