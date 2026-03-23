package session

import "strings"

// PromptTemplate holds provider-specific prompt wrapping configuration.
type PromptTemplate struct {
	Prefix               string // prepended before the user prompt
	Suffix               string // appended after the user prompt
	SystemPromptAddition string // added to --append-system-prompt (Claude only)
}

// ProviderPromptTemplate returns the default prompt template for a provider.
// Claude returns an empty template (no wrapping needed — it handles context natively).
// Gemini and Codex templates orient the model toward the agentic coding task.
func ProviderPromptTemplate(p Provider) PromptTemplate {
	switch p {
	case ProviderGemini:
		return PromptTemplate{
			Prefix: "You are a software engineering assistant. Use available tools to complete the task below.\n\n",
		}
	case ProviderCodex:
		return PromptTemplate{
			Prefix: "Complete the following software engineering task:\n\n",
			Suffix: "\n\nProvide complete, working code changes only.",
		}
	default: // ProviderClaude — no wrapping required
		return PromptTemplate{}
	}
}

// ApplyProviderTemplate wraps a prompt with the provider's template.
// Returns the original prompt unchanged when the template is empty.
func ApplyProviderTemplate(p Provider, prompt string) string {
	tmpl := ProviderPromptTemplate(p)
	if tmpl.Prefix == "" && tmpl.Suffix == "" {
		return prompt
	}
	var b strings.Builder
	b.WriteString(tmpl.Prefix)
	b.WriteString(prompt)
	b.WriteString(tmpl.Suffix)
	return b.String()
}

// ApplyTemplateToOptions applies the provider's prompt template to LaunchOptions
// in-place. Modifies opts.Prompt and opts.SystemPrompt when the template has content.
func ApplyTemplateToOptions(opts *LaunchOptions) {
	if opts.Prompt == "" {
		return
	}
	p := opts.Provider
	if p == "" {
		p = ProviderClaude
	}
	tmpl := ProviderPromptTemplate(p)
	if tmpl.Prefix != "" || tmpl.Suffix != "" {
		opts.Prompt = ApplyProviderTemplate(p, opts.Prompt)
	}
	if tmpl.SystemPromptAddition != "" && opts.SystemPrompt == "" {
		opts.SystemPrompt = tmpl.SystemPromptAddition
	}
}
