package session

import (
	"strings"
	"testing"
)

func TestApplyProviderTemplateClaudeNoOp(t *testing.T) {
	prompt := "do something"
	got := ApplyProviderTemplate(ProviderClaude, prompt)
	if got != prompt {
		t.Errorf("claude template modified prompt: got %q, want %q", got, prompt)
	}
}

func TestApplyProviderTemplateGeminiWraps(t *testing.T) {
	prompt := "fix the bug"
	got := ApplyProviderTemplate(ProviderGemini, prompt)
	if !strings.Contains(got, prompt) {
		t.Errorf("gemini template dropped prompt: %q", got)
	}
	if !strings.HasPrefix(got, "You are") {
		t.Errorf("gemini template missing prefix: %q", got)
	}
}

func TestApplyProviderTemplateCodexWraps(t *testing.T) {
	prompt := "refactor the module"
	got := ApplyProviderTemplate(ProviderCodex, prompt)
	if !strings.Contains(got, prompt) {
		t.Errorf("codex template dropped prompt: %q", got)
	}
	if !strings.HasPrefix(got, "Complete") {
		t.Errorf("codex template missing prefix: %q", got)
	}
	if !strings.Contains(got, "working code") {
		t.Errorf("codex template missing suffix: %q", got)
	}
}

func TestApplyTemplateToOptionsModifiesPrompt(t *testing.T) {
	opts := &LaunchOptions{
		Provider: ProviderCodex,
		Prompt:   "write tests",
	}
	ApplyTemplateToOptions(opts)
	if opts.Prompt == "write tests" {
		t.Error("ApplyTemplateToOptions did not modify codex prompt")
	}
	if !strings.Contains(opts.Prompt, "write tests") {
		t.Error("ApplyTemplateToOptions dropped original prompt")
	}
}

func TestApplyTemplateToOptionsDefaultProvider(t *testing.T) {
	opts := &LaunchOptions{Prompt: "write tests"}
	ApplyTemplateToOptions(opts)
	if opts.Prompt == "write tests" {
		t.Fatal("default provider should apply the codex prompt template")
	}
	if !strings.HasPrefix(opts.Prompt, "Complete") {
		t.Errorf("default provider template missing codex prefix: %q", opts.Prompt)
	}
	if !strings.Contains(opts.Prompt, "write tests") {
		t.Errorf("default provider template dropped original prompt: %q", opts.Prompt)
	}
}

func TestApplyTemplateToOptionsSystemPrompt(t *testing.T) {
	opts := &LaunchOptions{Provider: ProviderGemini, Prompt: "do it"}
	ApplyTemplateToOptions(opts)
	// Gemini should set system prompt addition if template has one
	// Just verify no panic and prompt was wrapped
	if !strings.Contains(opts.Prompt, "do it") {
		t.Error("prompt should contain original text")
	}
}

func TestApplyTemplateToOptionsEmptyPromptNoOp(t *testing.T) {
	opts := &LaunchOptions{Provider: ProviderCodex, Prompt: ""}
	ApplyTemplateToOptions(opts)
	if opts.Prompt != "" {
		t.Error("expected empty prompt to stay empty")
	}
}
