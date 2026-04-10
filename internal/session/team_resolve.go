package session

import (
	"context"
	"fmt"
	"strings"
)

// TeamResolveOptions tunes how runtime defaults are projected onto a team
// launch request.
type TeamResolveOptions struct {
	BackendConfigured bool
}

// ResolvedTeamConfig is the effective team configuration after provider,
// runtime, backend, and model defaults are applied.
type ResolvedTeamConfig struct {
	Config                  TeamConfig
	Runtime                 string
	ProviderAutoSelected    bool
	ProviderSelectionReason string
}

// ResolveTeamConfig applies the shared launch-selection contract to team
// launches so dry-run previews and live launches report the same effective
// provider/runtime defaults.
func ResolveTeamConfig(ctx context.Context, config TeamConfig, opts TeamResolveOptions) ResolvedTeamConfig {
	resolved := config
	resolved.TenantID = NormalizeTenantID(resolved.TenantID)

	selection := resolveLaunchProviderSelection(ctx, LaunchOptions{
		Provider:    config.Provider,
		RepoPath:    config.RepoPath,
		Prompt:      teamRoutingPrompt(config),
		Model:       config.Model,
		Agent:       config.LeadAgent,
		SessionName: config.Name,
		TeamName:    config.Name,
	})
	resolved.Provider = selection.Provider
	if strings.TrimSpace(resolved.Model) == "" {
		resolved.Model = selection.Model
	}

	resolved.WorkerProvider = resolveTeamWorkerProvider(config, resolved.Provider)
	if strings.TrimSpace(resolved.WorkerModel) == "" {
		resolved.WorkerModel = ProviderDefaults(resolved.WorkerProvider)
	}

	runtime := TeamRuntimeLegacyLead
	if resolved.Provider == ProviderCodex {
		runtime = TeamRuntimeStructuredCodex
	}

	if strings.TrimSpace(resolved.ExecutionBackend) == "" {
		if runtime == TeamRuntimeStructuredCodex && opts.BackendConfigured {
			resolved.ExecutionBackend = TeamExecutionBackendFleet
		} else {
			resolved.ExecutionBackend = TeamExecutionBackendLocal
		}
	}
	if strings.TrimSpace(resolved.WorktreePolicy) == "" {
		if runtime == TeamRuntimeStructuredCodex {
			resolved.WorktreePolicy = TeamWorktreePolicyPerWorker
		} else {
			resolved.WorktreePolicy = TeamWorktreePolicyShared
		}
	}
	if strings.TrimSpace(resolved.TargetBranch) == "" {
		resolved.TargetBranch = "main"
	}
	if resolved.MaxConcurrency <= 0 {
		resolved.MaxConcurrency = defaultTeamMaxConcurrency
	}
	if resolved.MaxRetries <= 0 {
		resolved.MaxRetries = defaultTeamMaxRetries
	}
	if !config.AutoStartConfigured {
		resolved.AutoStart = runtime == TeamRuntimeStructuredCodex
	}

	return ResolvedTeamConfig{
		Config:                  resolved,
		Runtime:                 runtime,
		ProviderAutoSelected:    selection.AutoSelected,
		ProviderSelectionReason: selection.Reason,
	}
}

func resolveTeamWorkerProvider(config TeamConfig, fallback Provider) Provider {
	if provider := normalizeSessionProvider(config.WorkerProvider); provider != "" {
		return provider
	}
	if inferred, ok := InferProviderFromModel(config.WorkerModel); ok {
		return inferred
	}
	if fallback != "" {
		return fallback
	}
	return DefaultPrimaryProvider()
}

func teamRoutingPrompt(config TeamConfig) string {
	var sections []string
	if name := strings.TrimSpace(config.Name); name != "" {
		sections = append(sections, fmt.Sprintf("team: %s", name))
	}
	if lead := strings.TrimSpace(config.LeadAgent); lead != "" {
		sections = append(sections, fmt.Sprintf("lead agent: %s", lead))
	}
	if len(config.Tasks) > 0 {
		tasks := make([]string, 0, len(config.Tasks))
		for _, task := range config.Tasks {
			task = strings.TrimSpace(task)
			if task == "" {
				continue
			}
			tasks = append(tasks, "- "+task)
		}
		if len(tasks) > 0 {
			sections = append(sections, "tasks:\n"+strings.Join(tasks, "\n"))
		}
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}
