package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const hybridRoutingInventoryTimeout = 1500 * time.Millisecond

var discoverHybridRoutingOllamaInventory = DiscoverOllamaInventory

type launchProviderSelection struct {
	Provider     Provider
	Model        string
	AutoSelected bool
	Reason       string
}

func resolveLaunchProviderSelection(ctx context.Context, opts LaunchOptions) launchProviderSelection {
	if provider := normalizeSessionProvider(opts.Provider); provider != "" {
		return launchProviderSelection{
			Provider: provider,
			Model:    firstNonBlankModel(opts.Model, ProviderDefaults(provider)),
			Reason:   fmt.Sprintf("explicit provider %q requested", provider),
		}
	}

	if modelTargetsOllamaLane(opts.Model) {
		return launchProviderSelection{
			Provider:     ProviderOllama,
			Model:        firstNonBlankModel(opts.Model, ProviderDefaults(ProviderOllama)),
			AutoSelected: true,
			Reason:       fmt.Sprintf("inferred Ollama provider from model %q", strings.TrimSpace(opts.Model)),
		}
	}

	if shouldPreferImplicitOllama(opts) {
		inventory := discoverHybridRoutingOllamaInventory(ctx, hybridRoutingInventoryTimeout)
		if ollamaInventoryReadyForHybridRouting(inventory) {
			return launchProviderSelection{
				Provider:     ProviderOllama,
				Model:        firstNonBlankModel(opts.Model, ProviderDefaults(ProviderOllama)),
				AutoSelected: true,
				Reason:       "auto-selected Ollama for a low-risk planning/reporting task because the local inventory is fully ready",
			}
		}
	}

	defaultProvider := DefaultPrimaryProvider()
	return launchProviderSelection{
		Provider: defaultProvider,
		Model:    firstNonBlankModel(opts.Model, ProviderDefaults(defaultProvider)),
		Reason:   fmt.Sprintf("using default primary provider %q", defaultProvider),
	}
}

func applyLaunchProviderSelection(ctx context.Context, opts *LaunchOptions) launchProviderSelection {
	if opts == nil {
		return launchProviderSelection{}
	}
	selection := resolveLaunchProviderSelection(ctx, *opts)
	opts.Provider = selection.Provider
	if strings.TrimSpace(opts.Model) == "" {
		opts.Model = selection.Model
	}
	return selection
}

func shouldPreferImplicitOllama(opts LaunchOptions) bool {
	text := strings.ToLower(strings.Join([]string{
		strings.TrimSpace(opts.SessionName),
		strings.TrimSpace(opts.Agent),
		strings.TrimSpace(opts.TeamName),
		strings.TrimSpace(opts.Prompt),
	}, "\n"))
	if strings.TrimSpace(text) == "" {
		return false
	}

	deny := []string{
		"implement",
		"implementation",
		"feature",
		"fix",
		"bug",
		"patch",
		"refactor",
		"test",
		"worker",
		"write code",
		"edit file",
		"compile",
		"build",
		"merge",
		"commit",
		"release",
		"ship",
	}
	for _, token := range deny {
		if strings.Contains(text, token) {
			return false
		}
	}

	allow := []string{
		"plan",
		"planner",
		"planning",
		"summary",
		"summarize",
		"report",
		"score",
		"scoring",
		"audit",
		"analyze",
		"analysis",
		"triage",
		"inventory",
		"doctor",
		"research",
		"docs",
		"documentation",
		"review",
		"read-only",
	}
	for _, token := range allow {
		if strings.Contains(text, token) {
			return true
		}
	}

	switch ClassifyTaskType(text) {
	case TaskDocs, TaskResearch:
		return true
	default:
		return false
	}
}

func modelTargetsOllamaLane(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" {
		return false
	}
	if strings.HasPrefix(model, "code-") {
		return true
	}
	if isOllamaCloudModel(model) || ollamaAliasSourceModel(model) != "" {
		return true
	}
	for _, candidate := range []string{
		resolveOllamaChatModel(),
		resolveOllamaFastModel(),
		resolveOllamaCodeModel(),
		resolveOllamaHeavyCodeModel(),
		resolveOllamaHighContextCodeModel(),
		resolveOllamaCloudCodeModel(),
		resolveOllamaCloudVerifiedCodeModel(),
		resolveOllamaMultilingualCodeModel(),
		resolveOllamaThinkingCodeModel(),
		resolveOllamaEmbedModel(),
	} {
		if strings.TrimSpace(candidate) == model {
			return true
		}
		if source := strings.TrimSpace(ollamaAliasSourceModel(candidate)); source == model {
			return true
		}
	}
	return false
}

func ollamaInventoryReadyForHybridRouting(inventory OllamaInventory) bool {
	if !inventory.Reachable || inventory.Error != "" {
		return false
	}
	if len(inventory.RequiredModels) == 0 {
		return false
	}
	if inventory.ReadyRequiredCount() != len(inventory.RequiredModels) {
		return false
	}
	return len(inventory.AliasIssues()) == 0
}

func firstNonBlankModel(current, fallback string) string {
	if strings.TrimSpace(current) != "" {
		return current
	}
	return strings.TrimSpace(fallback)
}
