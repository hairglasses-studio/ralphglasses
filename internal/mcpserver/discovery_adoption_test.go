package mcpserver

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/config"
)

func TestDiscoveryAdoptionSummary_AggregatesResourcesPromptsAndSkills(t *testing.T) {
	t.Parallel()

	scanDir := t.TempDir()
	appSrv := NewServer(scanDir)
	discoveryDir := filepath.Join(scanDir, ".ralph")
	if err := os.MkdirAll(discoveryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	discoveryPath := filepath.Join(discoveryDir, "discovery_usage.jsonl")
	toolBenchPath := filepath.Join(discoveryDir, "tool_benchmarks.jsonl")
	appSrv.DiscoveryRecorder = NewDiscoveryUsageRecorder(discoveryPath)
	appSrv.ToolRecorder = NewToolCallRecorder(toolBenchPath, nil, 50)

	discoveryData := strings.Join([]string{
		`{"kind":"resource","name":"ralph:///catalog/server","ts":"2026-04-08T11:00:00Z"}`,
		`{"kind":"resource","name":"ralph:///catalog/skills","ts":"2026-04-08T11:05:00Z"}`,
		`{"kind":"resource","name":"ralph:///{repo}/status","actual_uri":"ralph:///demo/status","ts":"2026-04-08T11:10:00Z"}`,
		`{"kind":"prompt","name":"bootstrap-firstboot","ts":"2026-04-08T11:15:00Z"}`,
	}, "\n") + "\n"
	if err := os.WriteFile(discoveryPath, []byte(discoveryData), 0o644); err != nil {
		t.Fatal(err)
	}

	toolData := strings.Join([]string{
		`{"tool":"ralphglasses_tool_groups","ts":"2026-04-08T11:20:00Z","latency_ms":8,"ok":true}`,
		`{"tool":"ralphglasses_server_health","ts":"2026-04-08T11:25:00Z","latency_ms":9,"ok":true}`,
		`{"tool":"ralphglasses_marathon","ts":"2026-04-08T11:30:00Z","latency_ms":15,"ok":true}`,
	}, "\n") + "\n"
	if err := os.WriteFile(toolBenchPath, []byte(toolData), 0o644); err != nil {
		t.Fatal(err)
	}

	summary := appSrv.discoveryAdoptionSummary()
	if summary.ResourceSurfaces != len(staticResourceCatalog())+len(resourceTemplateCatalog()) {
		t.Fatalf("ResourceSurfaces = %d", summary.ResourceSurfaces)
	}
	if summary.ActiveResourceSurfaces != 3 {
		t.Fatalf("ActiveResourceSurfaces = %d, want 3", summary.ActiveResourceSurfaces)
	}
	if summary.ActivePromptSurfaces != 1 {
		t.Fatalf("ActivePromptSurfaces = %d, want 1", summary.ActivePromptSurfaces)
	}
	if summary.ActiveSkillSurfaces == 0 {
		t.Fatal("expected at least one active skill surface")
	}
	if len(summary.TopResources) == 0 || summary.TopResources[0].Name != "ralph:///catalog/server" {
		t.Fatalf("unexpected top resources: %+v", summary.TopResources)
	}
	if len(summary.TopPrompts) != 1 || summary.TopPrompts[0].Name != "bootstrap-firstboot" {
		t.Fatalf("unexpected top prompts: %+v", summary.TopPrompts)
	}
	if len(summary.InactiveSkills) == 0 {
		t.Fatal("expected inactive skills to be listed")
	}
}

func TestRegisterResourcesAndPrompts_RecordDiscoveryUsage(t *testing.T) {
	t.Parallel()

	appSrv, mcpSrv, _ := setupRepoForResources(t)
	RegisterPrompts(mcpSrv, appSrv)

	ctx := context.Background()
	mcpSrv.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`))

	resp := mcpSrv.HandleMessage(ctx, []byte(`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"ralph:///catalog/server"}}`))
	if _, ok := resp.(mcp.JSONRPCResponse); !ok {
		t.Fatalf("expected resources/read JSONRPCResponse, got %T", resp)
	}
	resp = mcpSrv.HandleMessage(ctx, []byte(fmt.Sprintf(
		`{"jsonrpc":"2.0","id":3,"method":"prompts/get","params":{"name":"bootstrap-firstboot","arguments":{"scan_path":"%s"}}}`,
		config.DefaultScanPath,
	)))
	if _, ok := resp.(mcp.JSONRPCResponse); !ok {
		t.Fatalf("expected prompts/get JSONRPCResponse, got %T", resp)
	}

	entries, err := appSrv.DiscoveryRecorder.LoadEntries(time.Time{})
	if err != nil {
		t.Fatalf("LoadEntries: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 discovery entries, got %d", len(entries))
	}
	kinds := map[DiscoveryUsageKind]bool{}
	names := map[string]bool{}
	for _, entry := range entries {
		kinds[entry.Kind] = true
		names[entry.Name] = true
	}
	if !kinds[DiscoveryUsageResource] || !kinds[DiscoveryUsagePrompt] {
		t.Fatalf("expected both resource and prompt entries: %+v", entries)
	}
	if !names["ralph:///catalog/server"] || !names["bootstrap-firstboot"] {
		t.Fatalf("unexpected discovery entry names: %+v", entries)
	}
}
