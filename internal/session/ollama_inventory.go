package session

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// OllamaAliasInventory reports installation state for a managed code-* alias.
type OllamaAliasInventory struct {
	Alias           string `json:"alias"`
	Source          string `json:"source"`
	AliasInstalled  bool   `json:"alias_installed"`
	SourceInstalled bool   `json:"source_installed"`
	Status          string `json:"status"`
	Detail          string `json:"detail,omitempty"`
}

// OllamaInventory reports the live local-model inventory for the shared Ollama lane.
type OllamaInventory struct {
	BaseURL               string                 `json:"base_url"`
	Reachable             bool                   `json:"reachable"`
	ChatModel             string                 `json:"chat_model"`
	FastModel             string                 `json:"fast_model"`
	CodeModel             string                 `json:"code_model"`
	HeavyCodeModel        string                 `json:"heavy_code_model"`
	HighContextCodeModel  string                 `json:"high_context_code_model"`
	EmbedModel            string                 `json:"embed_model"`
	RequiredModels        []string               `json:"required_models"`
	ReadyRequiredModels   []string               `json:"ready_required_models,omitempty"`
	MissingRequiredModels []string               `json:"missing_required_models,omitempty"`
	ManagedAliases        []OllamaAliasInventory `json:"managed_aliases,omitempty"`
	AvailableModelCount   int                    `json:"available_model_count"`
	PullCommands          []string               `json:"pull_commands,omitempty"`
	Error                 string                 `json:"error,omitempty"`
}

// DiscoverOllamaInventory returns the live local-model inventory used by Ralph's
// Ollama session provider and related operator surfaces.
func DiscoverOllamaInventory(ctx context.Context, timeout time.Duration) OllamaInventory {
	return discoverOllamaInventory(ctx, timeout, fetchOllamaModels)
}

func discoverOllamaInventory(ctx context.Context, timeout time.Duration, fetcher func(context.Context, time.Duration) ([]string, error)) OllamaInventory {
	inventory := OllamaInventory{
		BaseURL:              resolveOllamaBaseURL(),
		ChatModel:            resolveOllamaChatModel(),
		FastModel:            resolveOllamaFastModel(),
		CodeModel:            resolveOllamaCodeModel(),
		HeavyCodeModel:       resolveOllamaHeavyCodeModel(),
		HighContextCodeModel: resolveOllamaHighContextCodeModel(),
		EmbedModel:           resolveOllamaEmbedModel(),
	}
	inventory.RequiredModels = ollamaInventoryRequiredModels(inventory)
	inventory.PullCommands = ollamaInventoryPullCommands(inventory.RequiredModels)

	models, err := fetcher(ctx, timeout)
	if err != nil {
		inventory.Error = err.Error()
		return inventory
	}

	inventory.Reachable = true
	inventory.AvailableModelCount = len(uniqueOllamaModels(models))
	for _, model := range inventory.RequiredModels {
		if ollamaModelInstalledExact(model, models) {
			inventory.ReadyRequiredModels = append(inventory.ReadyRequiredModels, model)
			continue
		}
		inventory.MissingRequiredModels = append(inventory.MissingRequiredModels, model)
	}

	for _, alias := range ollamaInventoryManagedAliases(inventory) {
		aliasInstalled := ollamaModelInstalledExact(alias.Alias, models)
		sourceInstalled := ollamaModelInstalledExact(alias.Source, models)
		status := OllamaAliasInventory{
			Alias:           alias.Alias,
			Source:          alias.Source,
			AliasInstalled:  aliasInstalled,
			SourceInstalled: sourceInstalled,
		}
		switch {
		case aliasInstalled && sourceInstalled:
			status.Status = "installed"
		case sourceInstalled && !aliasInstalled:
			status.Status = "missing_alias"
			status.Detail = fmt.Sprintf("Backing model %q is installed but managed alias %q is missing; run `~/hairglasses-studio/dotfiles/scripts/hg-ollama-sync-aliases.sh`", alias.Source, alias.Alias)
		case aliasInstalled && !sourceInstalled:
			status.Status = "alias_only"
			status.Detail = fmt.Sprintf("Managed alias %q is installed without backing model %q; rebuild aliases or pull the backing model", alias.Alias, alias.Source)
		default:
			status.Status = "missing_source"
			status.Detail = fmt.Sprintf("Managed alias %q and backing model %q are both missing; pull with `ollama pull %s`", alias.Alias, alias.Source, ollamaPullHintModel(alias.Alias))
		}
		inventory.ManagedAliases = append(inventory.ManagedAliases, status)
	}

	return inventory
}

func ollamaInventoryRequiredModels(inventory OllamaInventory) []string {
	return dedupeOllamaModels(
		inventory.ChatModel,
		inventory.FastModel,
		inventory.CodeModel,
		inventory.HeavyCodeModel,
		inventory.HighContextCodeModel,
		inventory.EmbedModel,
	)
}

func ollamaInventoryManagedAliases(inventory OllamaInventory) []OllamaAliasInventory {
	type pair struct {
		alias  string
		source string
	}
	raw := []pair{
		{alias: inventory.ChatModel, source: ollamaAliasSourceModel(inventory.ChatModel)},
		{alias: inventory.FastModel, source: ollamaAliasSourceModel(inventory.FastModel)},
		{alias: inventory.CodeModel, source: ollamaAliasSourceModel(inventory.CodeModel)},
		{alias: inventory.HeavyCodeModel, source: ollamaAliasSourceModel(inventory.HeavyCodeModel)},
		{alias: inventory.HighContextCodeModel, source: ollamaAliasSourceModel(inventory.HighContextCodeModel)},
	}
	seen := make(map[string]struct{}, len(raw))
	aliases := make([]OllamaAliasInventory, 0, len(raw))
	for _, item := range raw {
		item.alias = strings.TrimSpace(item.alias)
		item.source = strings.TrimSpace(item.source)
		if item.alias == "" || item.source == "" {
			continue
		}
		if _, ok := seen[item.alias]; ok {
			continue
		}
		seen[item.alias] = struct{}{}
		aliases = append(aliases, OllamaAliasInventory{
			Alias:  item.alias,
			Source: item.source,
		})
	}
	return aliases
}

func ollamaInventoryPullCommands(models []string) []string {
	commands := make([]string, 0, len(models))
	seen := make(map[string]struct{}, len(models))
	for _, model := range models {
		command := fmt.Sprintf("ollama pull %s", ollamaPullHintModel(model))
		if _, ok := seen[command]; ok {
			continue
		}
		seen[command] = struct{}{}
		commands = append(commands, command)
	}
	return commands
}

func dedupeOllamaModels(models ...string) []string {
	seen := make(map[string]struct{}, len(models))
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" {
			continue
		}
		if _, ok := seen[model]; ok {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

func uniqueOllamaModels(models []string) []string {
	return dedupeOllamaModels(models...)
}
