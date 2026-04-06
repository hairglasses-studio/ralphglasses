package mcpserver

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestHandlePrefetchStatus_DefaultHooks(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	result, err := srv.handlePrefetchStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handlePrefetchStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handlePrefetchStatus returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	var data struct {
		HookCount int `json:"hook_count"`
		Hooks     []struct {
			Name string `json:"name"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	if data.HookCount != 4 {
		t.Errorf("expected 4 default hooks, got %d", data.HookCount)
	}

	expectedNames := []string{"git_status", "claude_md", "test_inventory", "dir_structure"}
	for i, want := range expectedNames {
		if i >= len(data.Hooks) {
			t.Errorf("missing hook %q at index %d", want, i)
			continue
		}
		if data.Hooks[i].Name != want {
			t.Errorf("hook[%d] = %q, want %q", i, data.Hooks[i].Name, want)
		}
	}
}

func TestHandlePrefetchStatus_CustomRunner(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	// Set a custom runner with fewer hooks.
	runner := session.NewPrefetchRunner(0)
	runner.Register(session.GitStatusHook{})
	srv.PrefetchRunnerInstance = runner

	result, err := srv.handlePrefetchStatus(context.Background(), makeRequest(nil))
	if err != nil {
		t.Fatalf("handlePrefetchStatus: %v", err)
	}
	if result.IsError {
		t.Fatalf("handlePrefetchStatus returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	var data struct {
		HookCount int `json:"hook_count"`
	}
	if err := json.Unmarshal([]byte(text), &data); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if data.HookCount != 1 {
		t.Errorf("expected 1 hook, got %d", data.HookCount)
	}
}

func TestBuildPrefetchGroup(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)
	group := srv.buildPrefetchGroup()

	if group.Name != "prefetch" {
		t.Errorf("group name = %q, want %q", group.Name, "prefetch")
	}
	if len(group.Tools) != 1 {
		t.Fatalf("expected 1 tool in prefetch group, got %d", len(group.Tools))
	}
	if group.Tools[0].Tool.Name != "ralphglasses_prefetch_status" {
		t.Errorf("tool name = %q, want ralphglasses_prefetch_status", group.Tools[0].Tool.Name)
	}
}

func TestPrefetchStatus_InToolGroups(t *testing.T) {
	t.Parallel()
	srv, _ := setupTestServer(t)

	found := false
	for _, g := range srv.ToolGroups() {
		for _, entry := range g.Tools {
			if entry.Tool.Name == "ralphglasses_prefetch_status" {
				found = true
			}
		}
	}
	if !found {
		t.Error("ralphglasses_prefetch_status not found in tool groups")
	}
}

func TestPrefetchStatus_ViaHarness(t *testing.T) {
	t.Parallel()
	h := NewTestHarness(t)

	// Verify the tool is registered.
	names := h.ToolNames()
	found := false
	for _, n := range names {
		if strings.Contains(n, "prefetch") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected prefetch tool to be registered in harness")
	}

	result, err := h.CallTool("ralphglasses_prefetch_status", nil)
	if err != nil {
		t.Fatalf("CallTool error: %v", err)
	}
	if result.IsError {
		t.Fatalf("tool returned error: %s", getResultText(result))
	}

	text := getResultText(result)
	if !strings.Contains(text, "hook_count") {
		t.Errorf("expected hook_count in result, got: %s", text)
	}
}
