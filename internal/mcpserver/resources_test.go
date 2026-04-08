package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/process"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// setupRepoForResources creates a temp directory with a fake repo structure
// that the Server can find, and returns the Server and repo name.
func setupRepoForResources(t *testing.T) (*Server, *server.MCPServer, string) {
	t.Helper()

	scanDir := t.TempDir()
	repoName := "testrepo"
	repoPath := filepath.Join(scanDir, repoName)

	// Create .ralph directory structure.
	ralphDir := filepath.Join(repoPath, ".ralph")
	logsDir := filepath.Join(ralphDir, "logs")
	if err := os.MkdirAll(logsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a .git dir so discovery finds it as a repo.
	if err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	appSrv := NewServer(scanDir)
	// Pre-populate repos to avoid needing real discovery.
	appSrv.Repos = []*model.Repo{{
		Name:     repoName,
		Path:     repoPath,
		HasRalph: true,
	}}

	mcpSrv := server.NewMCPServer("test", "0.0.0",
		server.WithToolCapabilities(true),
		server.WithResourceCapabilities(false, false),
	)
	RegisterResources(mcpSrv, appSrv)

	return appSrv, mcpSrv, repoName
}

func TestResourceRegistration(t *testing.T) {
	t.Parallel()

	_, mcpSrv, _ := setupRepoForResources(t)

	// Verify resource templates are registered by sending a resources/list request.
	// The MCPServer should have 4 resource templates registered.
	ctx := context.Background()
	rawReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`
	mcpSrv.HandleMessage(ctx, []byte(rawReq))

	listReq := `{"jsonrpc":"2.0","id":2,"method":"resources/templates/list","params":{}}`
	resp := mcpSrv.HandleMessage(ctx, []byte(listReq))

	// The response should be a JSONRPCResponse (not an error).
	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T: %+v", resp, resp)
	}

	// Parse the result to check templates.
	result, ok := rpcResp.Result.(mcp.ListResourceTemplatesResult)
	if !ok {
		t.Fatalf("expected ListResourceTemplatesResult, got %T", rpcResp.Result)
	}

	if len(result.ResourceTemplates) != 4 {
		t.Errorf("expected 4 resource templates, got %d", len(result.ResourceTemplates))
	}

	// Verify template URIs.
	uris := make(map[string]bool)
	for _, tmpl := range result.ResourceTemplates {
		uris[tmpl.URITemplate.Raw()] = true
	}
	for _, expected := range []string{
		"ralph:///{repo}/triage",
		"ralph:///{repo}/status",
		"ralph:///{repo}/progress",
		"ralph:///{repo}/logs",
	} {
		if !uris[expected] {
			t.Errorf("missing expected resource template URI: %s", expected)
		}
	}
}

func TestStaticResourceRegistration(t *testing.T) {
	t.Parallel()

	_, mcpSrv, _ := setupRepoForResources(t)

	ctx := context.Background()
	rawReq := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"test","version":"0.0.0"}}}`
	mcpSrv.HandleMessage(ctx, []byte(rawReq))

	listReq := `{"jsonrpc":"2.0","id":2,"method":"resources/list","params":{}}`
	resp := mcpSrv.HandleMessage(ctx, []byte(listReq))

	rpcResp, ok := resp.(mcp.JSONRPCResponse)
	if !ok {
		t.Fatalf("expected JSONRPCResponse, got %T: %+v", resp, resp)
	}

	result, ok := rpcResp.Result.(mcp.ListResourcesResult)
	if !ok {
		t.Fatalf("expected ListResourcesResult, got %T", rpcResp.Result)
	}

	if len(result.Resources) != 12 {
		t.Fatalf("expected 12 static resources, got %d", len(result.Resources))
	}

	uris := make(map[string]bool)
	for _, resource := range result.Resources {
		uris[resource.URI] = true
	}

	for _, expected := range []string{
		"ralph:///catalog/server",
		"ralph:///catalog/tool-groups",
		"ralph:///catalog/workflows",
		"ralph:///catalog/skills",
		"ralph:///catalog/cli-parity",
		"ralph:///catalog/discovery-adoption",
		"ralph:///catalog/adoption-priorities",
		"ralph:///bootstrap/checklist",
		"ralph:///runtime/recovery",
		"ralph:///runtime/sessions",
		"ralph:///runtime/operator",
		"ralph:///runtime/health",
	} {
		if !uris[expected] {
			t.Errorf("missing expected static resource URI: %s", expected)
		}
	}
}

func TestCatalogSkillsResource_ReturnsSkillCatalog(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)
	handler := makeCatalogSkillsHandler()
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///catalog/skills"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "ralphglasses-bootstrap") {
		t.Fatalf("expected bootstrap skill in catalog: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "ralphglasses-recovery-observability") {
		t.Fatalf("expected recovery skill in catalog: %s", textContent.Text)
	}
	_ = appSrv
}

func TestTriageResource_ReturnsAggregatedSummary(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)
	repo := appSrv.findRepo(repoName)
	if repo == nil {
		t.Fatal("expected repo in test server")
	}

	if err := os.WriteFile(filepath.Join(repo.Path, ".ralph", "status.json"), []byte(`{"phase":"running","healthy":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Path, ".ralph", "progress.json"), []byte(`{"step":3,"total_steps":5}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(process.LogFilePath(repo.Path), []byte("first line\nsecond line\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeTriageHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///" + repoName + "/triage"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	for _, expected := range []string{
		`"repo": "` + repoName + `"`,
		`"recommended_prompt": "repo-triage-brief"`,
		`"status": {`,
		`"progress": {`,
		`"recent_logs": "first line\nsecond line"`,
		`"highest_priority_workflow"`,
	} {
		if !strings.Contains(textContent.Text, expected) {
			t.Fatalf("expected %q in triage resource: %s", expected, textContent.Text)
		}
	}
}

func TestCLIParityResource_ReturnsCoverageSummary(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)
	benchDir := filepath.Join(appSrv.ScanPath, ".ralph")
	if err := os.MkdirAll(benchDir, 0o755); err != nil {
		t.Fatal(err)
	}
	benchPath := filepath.Join(benchDir, "tool_benchmarks.jsonl")
	if err := os.WriteFile(benchPath, []byte(`{"tool":"ralphglasses_doctor","ts":"2026-04-08T11:00:00Z","latency_ms":10,"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeCLIParityHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///catalog/cli-parity"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "\"bespoke_coverage_pct\": 87.5") {
		t.Fatalf("expected bespoke coverage summary in cli parity resource: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"command_only_by_design\": 3") {
		t.Fatalf("expected command-only count in cli parity resource: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"usage_telemetry\"") {
		t.Fatalf("expected usage telemetry block in cli parity resource: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"active_observable_surfaces\": 1") {
		t.Fatalf("expected active surface count in cli parity resource: %s", textContent.Text)
	}
}

func TestDiscoveryAdoptionResource_ReturnsTelemetrySummary(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)
	discoveryDir := filepath.Join(appSrv.ScanPath, ".ralph")
	if err := os.MkdirAll(discoveryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(discoveryDir, "discovery_usage.jsonl"), []byte(`{"kind":"resource","name":"ralph:///catalog/server","ts":"2026-04-08T11:00:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeDiscoveryAdoptionHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///catalog/discovery-adoption"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "\"discovery_telemetry_present\": true") {
		t.Fatalf("expected discovery telemetry presence in resource: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"resource_surfaces\"") {
		t.Fatalf("expected resource surfaces in resource: %s", textContent.Text)
	}
}

func TestAdoptionPrioritiesResource_ReturnsPrioritySummary(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)
	ralphDir := filepath.Join(appSrv.ScanPath, ".ralph")
	if err := os.MkdirAll(ralphDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "tool_benchmarks.jsonl"), []byte(`{"tool":"ralphglasses_doctor","ts":"2026-04-08T11:00:00Z","latency_ms":10,"ok":true}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ralphDir, "discovery_usage.jsonl"), []byte(`{"kind":"resource","name":"ralph:///catalog/server","ts":"2026-04-08T11:00:00Z"}`+"\n"+`{"kind":"prompt","name":"bootstrap-firstboot","ts":"2026-04-08T11:01:00Z"}`+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeAdoptionPrioritiesHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///catalog/adoption-priorities"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "\"workflow_candidate_count\"") {
		t.Fatalf("expected workflow candidate count in resource: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"top_surface_candidates\"") {
		t.Fatalf("expected top surface candidates in resource: %s", textContent.Text)
	}
	if strings.Contains(textContent.Text, "\"name\": \"ralph:///catalog/server\"") {
		t.Fatalf("active discovery surface should not be prioritized: %s", textContent.Text)
	}
}

func TestBootstrapChecklistResource_ReturnsChecklist(t *testing.T) {
	t.Parallel()

	handler := makeBootstrapChecklistHandler()
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///bootstrap/checklist"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "ralphglasses-bootstrap") {
		t.Fatalf("expected bootstrap skill in checklist: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "ralphglasses_firstboot_profile") {
		t.Fatalf("expected firstboot tool in checklist: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "ralph:///runtime/recovery") {
		t.Fatalf("expected runtime recovery resource in checklist: %s", textContent.Text)
	}
}

func TestRuntimeRecoveryResource_ReturnsAggregatedSummary(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)
	repo := appSrv.findRepo(repoName)
	if repo == nil {
		t.Fatal("expected repo in test server")
	}

	now := time.Now().UTC()
	appSrv.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "sess-errored",
		Provider:     session.ProviderCodex,
		RepoPath:     repo.Path,
		RepoName:     repo.Name,
		Status:       session.StatusErrored,
		Model:        "gpt-5.4",
		Prompt:       "resume this repo",
		SpentUSD:     2.75,
		TurnCount:    8,
		Error:        "rate limit retry later",
		LastOutput:   "partial result",
		LaunchedAt:   now.Add(-30 * time.Minute),
		LastActivity: now.Add(-20 * time.Minute),
	})
	appSrv.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "sess-stalled",
		Provider:     session.ProviderCodex,
		RepoPath:     repo.Path,
		RepoName:     repo.Name,
		Status:       session.StatusRunning,
		Model:        "gpt-5.4",
		LaunchedAt:   now.Add(-45 * time.Minute),
		LastActivity: now.Add(-10 * time.Minute),
	})

	handler := makeRuntimeRecoveryHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///runtime/recovery"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	for _, expected := range []string{
		`"recommended_skill": "ralphglasses-recovery-observability"`,
		`"stalled_session_ids": [`,
		`"sess-stalled"`,
		`"session_triage": {`,
		`"total_sessions": 1`,
		`"top_recovery_candidates": [`,
		`"sess-errored"`,
		`"highest_priority_workflow"`,
	} {
		if !strings.Contains(textContent.Text, expected) {
			t.Fatalf("expected %q in runtime recovery resource: %s", expected, textContent.Text)
		}
	}
}

func TestRuntimeSessionsResource_ReturnsExecutionFrontDoor(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)
	repo := appSrv.findRepo(repoName)
	if repo == nil {
		t.Fatal("expected repo in test server")
	}

	now := time.Now().UTC()
	appSrv.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "sess-running",
		Provider:     session.ProviderClaude,
		RepoPath:     repo.Path,
		RepoName:     repo.Name,
		Status:       session.StatusRunning,
		Model:        "sonnet",
		BudgetUSD:    5,
		SpentUSD:     2.5,
		TurnCount:    6,
		LastActivity: now.Add(-2 * time.Minute),
		LaunchedAt:   now.Add(-12 * time.Minute),
	})
	appSrv.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "sess-over-budget",
		Provider:     session.ProviderCodex,
		RepoPath:     repo.Path,
		RepoName:     repo.Name,
		Status:       session.StatusCompleted,
		Model:        "gpt-5.4",
		BudgetUSD:    1,
		SpentUSD:     1.2,
		TurnCount:    9,
		LastActivity: now.Add(-10 * time.Minute),
		LaunchedAt:   now.Add(-45 * time.Minute),
	})

	handler := makeRuntimeSessionsHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///runtime/sessions"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	for _, expected := range []string{
		`"recommended_skill": "ralphglasses-session-ops"`,
		`"highest_priority_workflow": "session-execution"`,
		`"active_session_count": 1`,
		`"over_budget_session_ids": [`,
		`"sess-over-budget"`,
		`"provider_breakdown": {`,
		`"claude": 1`,
		`"codex": 1`,
	} {
		if !strings.Contains(textContent.Text, expected) {
			t.Fatalf("expected %q in runtime sessions resource: %s", expected, textContent.Text)
		}
	}
}

func TestRuntimeOperatorResource_ReturnsControlPlaneFrontDoor(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)
	repo := appSrv.findRepo(repoName)
	if repo == nil {
		t.Fatal("expected repo in test server")
	}

	now := time.Now().UTC()
	appSrv.SessMgr.AddSessionForTesting(&session.Session{
		ID:           "sess-running",
		Provider:     session.ProviderCodex,
		RepoPath:     repo.Path,
		RepoName:     repo.Name,
		Status:       session.StatusRunning,
		Model:        "gpt-5.4",
		LastActivity: now.Add(-1 * time.Minute),
		LaunchedAt:   now.Add(-15 * time.Minute),
	})

	handler := makeRuntimeOperatorHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///runtime/operator"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	for _, expected := range []string{
		`"recommended_skill": "ralphglasses-operator"`,
		`"highest_priority_workflow": "operator-control-plane"`,
		`"fleet_runtime": {`,
		`"status": "idle"`,
		`"marathon_runtime": {`,
		`"runtime_sessions": "ralph:///runtime/sessions"`,
		`"active_session_count": 1`,
	} {
		if !strings.Contains(textContent.Text, expected) {
			t.Fatalf("expected %q in runtime operator resource: %s", expected, textContent.Text)
		}
	}
}

func TestRuntimeHealthResource_ReturnsServerHealthShape(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)
	handler := makeRuntimeHealthHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{URI: "ralph:///runtime/health"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}
	if !strings.Contains(textContent.Text, "\"server\": \"ralphglasses\"") {
		t.Fatalf("expected server name in runtime health: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"skill_count\"") {
		t.Fatalf("expected skill_count in runtime health: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"cli_parity_usage\"") {
		t.Fatalf("expected cli_parity_usage in runtime health: %s", textContent.Text)
	}
	if !strings.Contains(textContent.Text, "\"discovery_adoption_summary\"") {
		t.Fatalf("expected discovery_adoption_summary in runtime health: %s", textContent.Text)
	}
}

func TestStatusResource_ReturnsJSON(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	statusContent := `{"phase":"running","step":42,"healthy":true}`
	statusPath := filepath.Join(repo.Path, ".ralph", "status.json")
	if err := os.WriteFile(statusPath, []byte(statusContent), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeStatusHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/status",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	if textContent.Text != statusContent {
		t.Errorf("expected %q, got %q", statusContent, textContent.Text)
	}
	if textContent.MIMEType != "application/json" {
		t.Errorf("expected MIME type application/json, got %s", textContent.MIMEType)
	}
}

func TestStatusResource_NoFile(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	handler := makeStatusHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/status",
		},
	})

	if err == nil {
		t.Fatal("expected error for missing status.json")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestProgressResource_ReturnsJSON(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	progressContent := `{"completed":5,"total":10,"percent":50}`
	progressPath := filepath.Join(repo.Path, ".ralph", "progress.json")
	if err := os.WriteFile(progressPath, []byte(progressContent), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeProgressHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/progress",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	if textContent.Text != progressContent {
		t.Errorf("expected %q, got %q", progressContent, textContent.Text)
	}
}

func TestLogsResource_TailsOutput(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	repo := appSrv.findRepo(repoName)
	logPath := filepath.Join(repo.Path, ".ralph", "logs", "ralph.log")

	// Write 150 lines; we expect only the last 100.
	var lines []string
	for i := 1; i <= 150; i++ {
		lines = append(lines, strings.Repeat("x", 10)+" line "+string(rune('0'+i%10)))
	}
	if err := os.WriteFile(logPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	handler := makeLogsHandler(appSrv)
	results, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/logs",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	textContent, ok := results[0].(mcp.TextResourceContents)
	if !ok {
		t.Fatalf("expected TextResourceContents, got %T", results[0])
	}

	// Should contain last 100 lines.
	outputLines := strings.Split(textContent.Text, "\n")
	if len(outputLines) != 100 {
		t.Errorf("expected 100 lines, got %d", len(outputLines))
	}
	if textContent.MIMEType != "text/plain" {
		t.Errorf("expected MIME type text/plain, got %s", textContent.MIMEType)
	}
}

func TestLogsResource_NoFile(t *testing.T) {
	t.Parallel()

	appSrv, _, repoName := setupRepoForResources(t)

	handler := makeLogsHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///" + repoName + "/logs",
		},
	})

	if err == nil {
		t.Fatal("expected error for missing ralph.log")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestStatusResource_UnknownRepo(t *testing.T) {
	t.Parallel()

	appSrv, _, _ := setupRepoForResources(t)

	handler := makeStatusHandler(appSrv)
	_, err := handler(context.Background(), mcp.ReadResourceRequest{
		Params: mcp.ReadResourceParams{
			URI: "ralph:///nonexistent/status",
		},
	})

	if err == nil {
		t.Fatal("expected error for unknown repo")
	}
	if !strings.Contains(err.Error(), "repo not found") {
		t.Errorf("expected 'repo not found' in error, got: %v", err)
	}
}

func TestExtractRepoName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uri  string
		want string
	}{
		{"ralph:///myrepo/status", "myrepo"},
		{"ralph:///myrepo/progress", "myrepo"},
		{"ralph:///myrepo/logs", "myrepo"},
		{"ralph:///my-repo-name/status", "my-repo-name"},
		{"ralph:///", ""},
		{"http:///foo/bar", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractRepoName(tt.uri)
		if got != tt.want {
			t.Errorf("extractRepoName(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
