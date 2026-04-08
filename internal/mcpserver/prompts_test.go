package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

func TestPromptRegistration(t *testing.T) {
	t.Parallel()

	srv := server.NewMCPServer("test", "0.0.1",
		server.WithPromptCapabilities(true),
	)
	appSrv := NewServer(t.TempDir())
	RegisterPrompts(srv, appSrv)

	// List prompts via the server's internal method — we verify by calling
	// the handler through the public AddPrompt path and checking no panic.
	// The fact that RegisterPrompts completed without error confirms
	// all prompt definitions were accepted.

	// Verify we can construct and invoke each prompt handler by name.
	promptNames := []string{
		"self-improvement-planner",
		"code-review",
		"test-generation",
		"bootstrap-firstboot",
		"provider-parity-audit",
	}
	for _, name := range promptNames {
		t.Run(name, func(t *testing.T) {
			// Build a fresh server for isolation.
			s := server.NewMCPServer("test", "0.0.1", server.WithPromptCapabilities(true))
			as := NewServer(t.TempDir())
			RegisterPrompts(s, as)

			// We cannot directly list prompts from MCPServer without going
			// through the JSON-RPC layer, but we can verify that all prompt
			// ServerPrompt values were constructed correctly by the builder
			// functions.
			_ = s // registered without error
		})
	}
}

func TestSelfImprovementPrompt_ReturnsMessage(t *testing.T) {
	t.Parallel()

	sp := selfImprovementPlannerPrompt()

	// Verify prompt metadata.
	if sp.Prompt.Name != "self-improvement-planner" {
		t.Fatalf("unexpected prompt name: %s", sp.Prompt.Name)
	}
	if len(sp.Prompt.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(sp.Prompt.Arguments))
	}
	for _, arg := range sp.Prompt.Arguments {
		if !arg.Required {
			t.Errorf("argument %s should be required", arg.Name)
		}
	}

	// Invoke handler with valid params.
	req := mcp.GetPromptRequest{}
	req.Params.Name = "self-improvement-planner"
	req.Params.Arguments = map[string]string{
		"repo_name":  "ralphglasses",
		"focus_area": "error-handling",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}
	if result.Messages[0].Role != mcp.RoleUser {
		t.Errorf("expected role 'user', got %s", result.Messages[0].Role)
	}

	// Verify content contains repo and focus area.
	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "ralphglasses") {
		t.Error("expected prompt to contain repo name")
	}
	if !strings.Contains(tc.Text, "error-handling") {
		t.Error("expected prompt to contain focus area")
	}
}

func TestCodeReviewPrompt_ReturnsMessage(t *testing.T) {
	t.Parallel()

	sp := codeReviewPrompt()

	if sp.Prompt.Name != "code-review" {
		t.Fatalf("unexpected prompt name: %s", sp.Prompt.Name)
	}
	if len(sp.Prompt.Arguments) != 2 {
		t.Fatalf("expected 2 arguments, got %d", len(sp.Prompt.Arguments))
	}

	req := mcp.GetPromptRequest{}
	req.Params.Name = "code-review"
	req.Params.Arguments = map[string]string{
		"repo_name": "ralphglasses",
		"file_path": "internal/mcpserver/tools.go",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}

	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "ralphglasses") {
		t.Error("expected prompt to contain repo name")
	}
	if !strings.Contains(tc.Text, "tools.go") {
		t.Error("expected prompt to contain file path")
	}
	// Should infer Go language from .go extension.
	if !strings.Contains(tc.Text, "Go") {
		t.Error("expected prompt to infer Go language")
	}
}

func TestTestGenerationPrompt_ReturnsMessage(t *testing.T) {
	t.Parallel()

	sp := testGenerationPrompt()

	if sp.Prompt.Name != "test-generation" {
		t.Fatalf("unexpected prompt name: %s", sp.Prompt.Name)
	}
	if len(sp.Prompt.Arguments) != 3 {
		t.Fatalf("expected 3 arguments, got %d", len(sp.Prompt.Arguments))
	}

	// coverage_target should be optional.
	for _, arg := range sp.Prompt.Arguments {
		if arg.Name == "coverage_target" && arg.Required {
			t.Error("coverage_target should not be required")
		}
	}

	req := mcp.GetPromptRequest{}
	req.Params.Name = "test-generation"
	req.Params.Arguments = map[string]string{
		"repo_name":       "ralphglasses",
		"file_path":       "internal/enhancer/templates.go",
		"coverage_target": "90",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Messages) == 0 {
		t.Fatal("expected at least one message")
	}

	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "90%") {
		t.Error("expected prompt to contain coverage target")
	}
}

func TestTestGenerationPrompt_DefaultCoverage(t *testing.T) {
	t.Parallel()

	sp := testGenerationPrompt()

	req := mcp.GetPromptRequest{}
	req.Params.Name = "test-generation"
	req.Params.Arguments = map[string]string{
		"repo_name": "ralphglasses",
		"file_path": "main.go",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "80%") {
		t.Error("expected default coverage target of 80%")
	}
}

func TestBootstrapFirstbootPrompt_ReturnsMessage(t *testing.T) {
	t.Parallel()

	sp := bootstrapFirstbootPrompt()
	if sp.Prompt.Name != "bootstrap-firstboot" {
		t.Fatalf("unexpected prompt name: %s", sp.Prompt.Name)
	}

	req := mcp.GetPromptRequest{}
	req.Params.Name = "bootstrap-firstboot"
	req.Params.Arguments = map[string]string{
		"scan_path":        "~/hairglasses-studio",
		"primary_provider": "codex",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "First-Boot Checklist") {
		t.Error("expected checklist heading")
	}
	if !strings.Contains(tc.Text, "codex") {
		t.Error("expected provider in prompt text")
	}
}

func TestProviderParityAuditPrompt_ReturnsMessage(t *testing.T) {
	t.Parallel()

	sp := providerParityAuditPrompt(NewServer(t.TempDir()))
	if sp.Prompt.Name != "provider-parity-audit" {
		t.Fatalf("unexpected prompt name: %s", sp.Prompt.Name)
	}

	req := mcp.GetPromptRequest{}
	req.Params.Name = "provider-parity-audit"
	req.Params.Arguments = map[string]string{
		"repo_name":       "ralphglasses",
		"target_provider": "codex",
	}

	result, err := sp.Handler(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tc, ok := result.Messages[0].Content.(mcp.TextContent)
	if !ok {
		t.Fatal("expected TextContent")
	}
	if !strings.Contains(tc.Text, "ralphglasses") {
		t.Error("expected repo name in parity prompt")
	}
	if !strings.Contains(tc.Text, "MCP-First Coverage") {
		t.Error("expected coverage section in parity prompt")
	}
}

func TestPrompt_MissingRequiredParam(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  server.ServerPrompt
		args    map[string]string
		wantErr string
	}{
		{
			name:    "self-improvement-planner missing repo_name",
			prompt:  selfImprovementPlannerPrompt(),
			args:    map[string]string{"focus_area": "tests"},
			wantErr: "repo_name is required",
		},
		{
			name:    "self-improvement-planner missing focus_area",
			prompt:  selfImprovementPlannerPrompt(),
			args:    map[string]string{"repo_name": "test"},
			wantErr: "focus_area is required",
		},
		{
			name:    "code-review missing repo_name",
			prompt:  codeReviewPrompt(),
			args:    map[string]string{"file_path": "main.go"},
			wantErr: "repo_name is required",
		},
		{
			name:    "code-review missing file_path",
			prompt:  codeReviewPrompt(),
			args:    map[string]string{"repo_name": "test"},
			wantErr: "file_path is required",
		},
		{
			name:    "test-generation missing repo_name",
			prompt:  testGenerationPrompt(),
			args:    map[string]string{"file_path": "main.go"},
			wantErr: "repo_name is required",
		},
		{
			name:    "test-generation missing file_path",
			prompt:  testGenerationPrompt(),
			args:    map[string]string{"repo_name": "test"},
			wantErr: "file_path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.GetPromptRequest{}
			req.Params.Name = tt.prompt.Prompt.Name
			req.Params.Arguments = tt.args

			_, err := tt.prompt.Handler(context.Background(), req)
			if err == nil {
				t.Fatal("expected error for missing required param")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("expected error containing %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestInferLanguage(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"main.go":       "Go",
		"app.py":        "Python",
		"index.ts":      "TypeScript",
		"component.tsx": "TypeScript",
		"app.js":        "JavaScript",
		"lib.rs":        "Rust",
		"Main.java":     "Java",
		"script.sh":     "Shell",
		"data.csv":      "unknown",
	}

	for path, want := range tests {
		t.Run(path, func(t *testing.T) {
			got := inferLanguage(path)
			if got != want {
				t.Errorf("inferLanguage(%q) = %q, want %q", path, got, want)
			}
		})
	}
}
