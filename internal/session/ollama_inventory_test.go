package session

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"
)

func TestDiscoverOllamaInventoryReportsManagedAliases(t *testing.T) {
	t.Setenv("OLLAMA_CHAT_MODEL", "code-primary")
	t.Setenv("OLLAMA_FAST_MODEL", "code-fast")
	t.Setenv("OLLAMA_CODE_MODEL", "code-primary")
	t.Setenv("OLLAMA_HEAVY_CODE_MODEL", "code-heavy")
	t.Setenv("OLLAMA_HIGH_CONTEXT_CODE_MODEL", "code-long")
	t.Setenv("OLLAMA_EMBED_MODEL", "nomic-embed-text:v1.5")

	inventory := discoverOllamaInventory(context.Background(), time.Second, func(context.Context, time.Duration) ([]string, error) {
		return []string{
			"code-primary",
			"devstral-small-2",
			"code-fast",
			"qwen2.5-coder:7b",
			"qwen3-coder-next",
			"nomic-embed-text:v1.5",
		}, nil
	})

	if !inventory.Reachable {
		t.Fatal("expected reachable inventory")
	}
	if inventory.AvailableModelCount != 6 {
		t.Fatalf("available_model_count = %d, want 6", inventory.AvailableModelCount)
	}
	if !slices.Contains(inventory.ReadyRequiredModels, "code-primary") {
		t.Fatalf("ready_required_models = %v, want code-primary present", inventory.ReadyRequiredModels)
	}
	if !slices.Contains(inventory.MissingRequiredModels, "code-heavy") {
		t.Fatalf("missing_required_models = %v, want code-heavy present", inventory.MissingRequiredModels)
	}
	if !slices.Contains(inventory.MissingRequiredModels, "code-long") {
		t.Fatalf("missing_required_models = %v, want code-long present", inventory.MissingRequiredModels)
	}

	var primaryStatus OllamaAliasInventory
	var heavyStatus OllamaAliasInventory
	for _, alias := range inventory.ManagedAliases {
		switch alias.Alias {
		case "code-primary":
			primaryStatus = alias
		case "code-heavy":
			heavyStatus = alias
		}
	}
	if primaryStatus.Status != "installed" {
		t.Fatalf("code-primary status = %q, want installed", primaryStatus.Status)
	}
	if heavyStatus.Status != "missing_source" {
		t.Fatalf("code-heavy status = %q, want missing_source", heavyStatus.Status)
	}
}

func TestDiscoverOllamaInventoryFlagsMissingAlias(t *testing.T) {
	t.Setenv("OLLAMA_CODE_MODEL", "code-primary")

	inventory := discoverOllamaInventory(context.Background(), time.Second, func(context.Context, time.Duration) ([]string, error) {
		return []string{"devstral-small-2"}, nil
	})

	for _, alias := range inventory.ManagedAliases {
		if alias.Alias == "code-primary" {
			if alias.Status != "missing_alias" {
				t.Fatalf("code-primary status = %q, want missing_alias", alias.Status)
			}
			return
		}
	}
	t.Fatal("expected code-primary managed alias status")
}

func TestDiscoverOllamaInventoryReportsFetchError(t *testing.T) {
	inventory := discoverOllamaInventory(context.Background(), time.Second, func(context.Context, time.Duration) ([]string, error) {
		return nil, errors.New("boom")
	})

	if inventory.Reachable {
		t.Fatal("expected unreachable inventory")
	}
	if inventory.Error != "boom" {
		t.Fatalf("error = %q, want %q", inventory.Error, "boom")
	}
	if len(inventory.PullCommands) == 0 {
		t.Fatal("expected pull commands even when discovery fails")
	}
}

func TestOllamaInventoryAliasIssueHelpers(t *testing.T) {
	inventory := OllamaInventory{
		ReadyRequiredModels: []string{"code-primary", "code-fast"},
		ManagedAliases: []OllamaAliasInventory{
			{Alias: "code-primary", Status: "installed"},
			{Alias: "code-fast", Status: "missing_alias"},
			{Alias: "code-heavy", Status: "missing_source"},
		},
	}

	if got := inventory.ReadyRequiredCount(); got != 2 {
		t.Fatalf("ReadyRequiredCount() = %d, want 2", got)
	}

	issues := inventory.AliasIssues()
	if len(issues) != 2 {
		t.Fatalf("len(AliasIssues()) = %d, want 2", len(issues))
	}

	names := inventory.AliasIssueNames()
	if !slices.Equal(names, []string{"code-fast", "code-heavy"}) {
		t.Fatalf("AliasIssueNames() = %v, want [code-fast code-heavy]", names)
	}
}

func TestFetchOllamaModelsCachesRecentResults(t *testing.T) {
	resetOllamaModelsCache()
	t.Cleanup(resetOllamaModelsCache)

	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path = %q, want /api/tags", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"code-primary"},{"name":"devstral-small-2"}]}`))
	}))
	defer srv.Close()

	t.Setenv("OLLAMA_BASE_URL", srv.URL)

	first, err := fetchOllamaModels(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("fetchOllamaModels() first call error = %v", err)
	}
	second, err := fetchOllamaModels(context.Background(), time.Second)
	if err != nil {
		t.Fatalf("fetchOllamaModels() second call error = %v", err)
	}
	if !slices.Equal(first, second) {
		t.Fatalf("cached models = %v, want %v", second, first)
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
}
