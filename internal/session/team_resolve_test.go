package session

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestResolveTeamConfig_InfersOllamaFromExplicitModel(t *testing.T) {
	t.Parallel()

	resolved := ResolveTeamConfig(context.Background(), TeamConfig{
		Name:     "docs-team",
		RepoPath: "/tmp/repo",
		Tasks:    []string{"summarize the latest docs drift"},
		Model:    "code-primary",
	}, TeamResolveOptions{})

	if resolved.Config.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", resolved.Config.Provider, ProviderOllama)
	}
	if resolved.Config.WorkerProvider != ProviderOllama {
		t.Fatalf("worker_provider = %q, want %q", resolved.Config.WorkerProvider, ProviderOllama)
	}
	if resolved.Config.WorkerModel != ProviderDefaults(ProviderOllama) {
		t.Fatalf("worker_model = %q, want %q", resolved.Config.WorkerModel, ProviderDefaults(ProviderOllama))
	}
	if resolved.Runtime != TeamRuntimeLegacyLead {
		t.Fatalf("runtime = %q, want %q", resolved.Runtime, TeamRuntimeLegacyLead)
	}
	if resolved.Config.ExecutionBackend != TeamExecutionBackendLocal {
		t.Fatalf("execution_backend = %q, want %q", resolved.Config.ExecutionBackend, TeamExecutionBackendLocal)
	}
	if resolved.Config.WorktreePolicy != TeamWorktreePolicyShared {
		t.Fatalf("worktree_policy = %q, want %q", resolved.Config.WorktreePolicy, TeamWorktreePolicyShared)
	}
	if resolved.Config.AutoStart {
		t.Fatal("expected autostart to remain false for legacy lead teams")
	}
	if !resolved.ProviderAutoSelected {
		t.Fatal("expected provider selection to be marked auto-selected")
	}
	if !strings.Contains(resolved.ProviderSelectionReason, "code-primary") {
		t.Fatalf("provider_selection_reason = %q, want model inference detail", resolved.ProviderSelectionReason)
	}
}

func TestResolveTeamConfig_InfersWorkerProviderFromWorkerModel(t *testing.T) {
	t.Parallel()

	resolved := ResolveTeamConfig(context.Background(), TeamConfig{
		Name:        "hybrid-team",
		RepoPath:    "/tmp/repo",
		Tasks:       []string{"implement the worker changes"},
		Provider:    ProviderClaude,
		WorkerModel: "code-fast",
	}, TeamResolveOptions{})

	if resolved.Config.Provider != ProviderClaude {
		t.Fatalf("provider = %q, want %q", resolved.Config.Provider, ProviderClaude)
	}
	if resolved.Config.WorkerProvider != ProviderOllama {
		t.Fatalf("worker_provider = %q, want %q", resolved.Config.WorkerProvider, ProviderOllama)
	}
	if resolved.Config.WorktreePolicy != TeamWorktreePolicyShared {
		t.Fatalf("worktree_policy = %q, want %q", resolved.Config.WorktreePolicy, TeamWorktreePolicyShared)
	}
}

func TestLaunchTeam_AutoSelectsOllamaForLowRiskTeam(t *testing.T) {
	repoPath := newLaunchTestRepo(t)
	orig := discoverHybridRoutingOllamaInventory
	discoverHybridRoutingOllamaInventory = func(context.Context, time.Duration) OllamaInventory {
		return OllamaInventory{
			Reachable:           true,
			RequiredModels:      []string{"code-primary"},
			ReadyRequiredModels: []string{"code-primary"},
			ManagedAliases: []OllamaAliasInventory{
				{Alias: "code-primary", Status: "installed"},
			},
		}
	}
	t.Cleanup(func() { discoverHybridRoutingOllamaInventory = orig })

	mgr := NewManager()
	mgr.launchSession = func(_ context.Context, opts LaunchOptions) (*Session, error) {
		return &Session{
			ID:         "team-lead-1",
			Provider:   opts.Provider,
			Model:      opts.Model,
			RepoPath:   opts.RepoPath,
			RepoName:   "repo",
			Status:     StatusRunning,
			LaunchedAt: time.Now(),
		}, nil
	}

	team, err := mgr.LaunchTeam(context.Background(), TeamConfig{
		Name:     "analysis-team",
		RepoPath: repoPath,
		Tasks:    []string{"summarize the repo inventory and report blockers"},
	})
	if err != nil {
		t.Fatalf("LaunchTeam() error = %v", err)
	}

	if team.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", team.Provider, ProviderOllama)
	}
	if team.WorkerProvider != ProviderOllama {
		t.Fatalf("worker_provider = %q, want %q", team.WorkerProvider, ProviderOllama)
	}
	if team.Runtime != TeamRuntimeLegacyLead {
		t.Fatalf("runtime = %q, want %q", team.Runtime, TeamRuntimeLegacyLead)
	}
	if team.ExecutionBackend != TeamExecutionBackendLocal {
		t.Fatalf("execution_backend = %q, want %q", team.ExecutionBackend, TeamExecutionBackendLocal)
	}
	if team.WorktreePolicy != TeamWorktreePolicyShared {
		t.Fatalf("worktree_policy = %q, want %q", team.WorktreePolicy, TeamWorktreePolicyShared)
	}
	if !team.ProviderAutoSelected {
		t.Fatal("expected provider_auto_selected to be true")
	}
	if team.ProviderSelectionReason == "" {
		t.Fatal("expected provider_selection_reason to be populated")
	}
}
