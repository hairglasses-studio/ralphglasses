package session

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestResolveLaunchProviderSelection_ExplicitProviderWins(t *testing.T) {
	t.Parallel()

	selection := resolveLaunchProviderSelection(context.Background(), LaunchOptions{
		Provider: ProviderGemini,
		Prompt:   "plan the repo status",
	})
	if selection.Provider != ProviderGemini {
		t.Fatalf("provider = %q, want %q", selection.Provider, ProviderGemini)
	}
	if selection.AutoSelected {
		t.Fatal("explicit provider should not be marked auto-selected")
	}
}

func TestResolveLaunchProviderSelection_InferOllamaFromModel(t *testing.T) {
	t.Parallel()

	selection := resolveLaunchProviderSelection(context.Background(), LaunchOptions{
		Model:  "code-primary",
		Prompt: "implement the next task",
	})
	if selection.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", selection.Provider, ProviderOllama)
	}
	if !selection.AutoSelected {
		t.Fatal("model-implied ollama selection should be marked auto-selected")
	}
}

func TestResolveLaunchProviderSelection_InferCodexFromExplicitModel(t *testing.T) {
	t.Parallel()

	selection := resolveLaunchProviderSelection(context.Background(), LaunchOptions{
		Model:  "gpt-5.4",
		Prompt: "plan and summarize the next repo-health report",
	})
	if selection.Provider != ProviderCodex {
		t.Fatalf("provider = %q, want %q", selection.Provider, ProviderCodex)
	}
	if !selection.AutoSelected {
		t.Fatal("explicit model inference should be marked auto-selected")
	}
}

func TestResolveLaunchProviderSelection_LowRiskPromptUsesOllamaWhenInventoryReady(t *testing.T) {
	orig := discoverHybridRoutingOllamaInventory
	discoverHybridRoutingOllamaInventory = func(context.Context, time.Duration) OllamaInventory {
		return OllamaInventory{
			Reachable:           true,
			RequiredModels:      []string{"code-primary", "code-fast"},
			ReadyRequiredModels: []string{"code-primary", "code-fast"},
			ManagedAliases: []OllamaAliasInventory{
				{Alias: "code-primary", Status: "installed"},
				{Alias: "code-fast", Status: "installed"},
			},
		}
	}
	t.Cleanup(func() { discoverHybridRoutingOllamaInventory = orig })

	selection := resolveLaunchProviderSelection(context.Background(), LaunchOptions{
		Prompt: "plan and summarize the next repo-health report",
	})
	if selection.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", selection.Provider, ProviderOllama)
	}
	if !selection.AutoSelected {
		t.Fatal("expected auto-selected ollama routing")
	}
}

func TestResolveLaunchProviderSelection_LowRiskPromptFallsBackWhenInventoryDrifts(t *testing.T) {
	orig := discoverHybridRoutingOllamaInventory
	discoverHybridRoutingOllamaInventory = func(context.Context, time.Duration) OllamaInventory {
		return OllamaInventory{
			Reachable:             true,
			RequiredModels:        []string{"code-primary", "code-fast"},
			ReadyRequiredModels:   []string{"code-primary"},
			MissingRequiredModels: []string{"code-fast"},
			ManagedAliases: []OllamaAliasInventory{
				{Alias: "code-primary", Status: "installed"},
				{Alias: "code-fast", Status: "missing_alias"},
			},
		}
	}
	t.Cleanup(func() { discoverHybridRoutingOllamaInventory = orig })

	selection := resolveLaunchProviderSelection(context.Background(), LaunchOptions{
		Prompt: "summarize the latest audit status",
	})
	if selection.Provider != DefaultPrimaryProvider() {
		t.Fatalf("provider = %q, want %q", selection.Provider, DefaultPrimaryProvider())
	}
	if selection.AutoSelected {
		t.Fatal("inventory drift should prevent auto-selected ollama routing")
	}
}

func TestShouldPreferImplicitOllama(t *testing.T) {
	t.Parallel()

	if !shouldPreferImplicitOllama(LaunchOptions{Prompt: "research and summarize the roadmap drift"}) {
		t.Fatal("expected research/summary prompt to prefer ollama")
	}
	if shouldPreferImplicitOllama(LaunchOptions{Prompt: "implement the refactor and add tests"}) {
		t.Fatal("expected implementation-heavy prompt to stay on cloud default")
	}
}

func TestOllamaInventoryReadyForHybridRouting(t *testing.T) {
	t.Parallel()

	inventory := OllamaInventory{
		Reachable:           true,
		RequiredModels:      []string{"code-primary", "code-fast"},
		ReadyRequiredModels: []string{"code-primary", "code-fast"},
		ManagedAliases: []OllamaAliasInventory{
			{Alias: "code-primary", Status: "installed"},
			{Alias: "code-fast", Status: "installed"},
		},
	}
	if !ollamaInventoryReadyForHybridRouting(inventory) {
		t.Fatal("expected fully ready inventory to pass the hybrid routing gate")
	}

	inventory.ManagedAliases = append(inventory.ManagedAliases, OllamaAliasInventory{Alias: "code-long", Status: "missing_alias"})
	if ollamaInventoryReadyForHybridRouting(inventory) {
		t.Fatal("expected alias drift to fail the hybrid routing gate")
	}
}

func TestModelTargetsOllamaLane(t *testing.T) {
	t.Parallel()

	models := []string{"code-primary", "devstral-small-2", resolveOllamaCloudCodeModel()}
	for _, model := range models {
		if !modelTargetsOllamaLane(model) {
			t.Fatalf("expected %q to target the Ollama lane", model)
		}
	}
	if modelTargetsOllamaLane("gpt-5.4") {
		t.Fatal("gpt-5.4 should not target the Ollama lane")
	}
}

func TestApplyLaunchProviderSelectionPreservesExplicitModel(t *testing.T) {
	t.Parallel()

	opts := LaunchOptions{Model: "code-long", Prompt: "summarize repo status"}
	selection := applyLaunchProviderSelection(context.Background(), &opts)
	if opts.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", opts.Provider, ProviderOllama)
	}
	if opts.Model != "code-long" {
		t.Fatalf("model = %q, want code-long", opts.Model)
	}
	if !selection.AutoSelected {
		t.Fatal("expected auto-selected model lane routing")
	}
}

func TestManagerLaunchAutoSelectsOllamaForLowRiskPrompt(t *testing.T) {
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
			ID:         "sess-1",
			Provider:   opts.Provider,
			Model:      opts.Model,
			RepoPath:   opts.RepoPath,
			RepoName:   filepath.Base(opts.RepoPath),
			Status:     StatusRunning,
			LaunchedAt: time.Now(),
		}, nil
	}

	sess, err := mgr.Launch(context.Background(), LaunchOptions{
		RepoPath: repoPath,
		Prompt:   "plan and summarize the repo inventory",
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}
	if sess.Provider != ProviderOllama {
		t.Fatalf("provider = %q, want %q", sess.Provider, ProviderOllama)
	}
	if !sess.ProviderAutoSelected {
		t.Fatal("expected ProviderAutoSelected to be true")
	}
	if sess.ProviderSelectionReason == "" {
		t.Fatal("expected ProviderSelectionReason to be populated")
	}
}

func newLaunchTestRepo(t *testing.T) string {
	t.Helper()
	repoPath := t.TempDir()
	if err := os.Mkdir(filepath.Join(repoPath, ".git"), 0755); err != nil {
		t.Fatalf("mkdir .git: %v", err)
	}
	return repoPath
}

func TestOllamaInventoryAliasIssueNamesOrder(t *testing.T) {
	t.Parallel()

	inventory := OllamaInventory{
		ManagedAliases: []OllamaAliasInventory{
			{Alias: "code-primary", Status: "installed"},
			{Alias: "code-fast", Status: "missing_alias"},
			{Alias: "code-long", Status: "missing_source"},
		},
	}
	if got := inventory.AliasIssueNames(); !slices.Equal(got, []string{"code-fast", "code-long"}) {
		t.Fatalf("AliasIssueNames() = %v", got)
	}
}
