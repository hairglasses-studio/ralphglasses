package mcpserver

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/model"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func TestSweepGenerate_DefaultAudit(t *testing.T) {
	s := &Server{Tasks: NewTaskRegistry()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := s.handleSweepGenerate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if text == "" {
		t.Fatal("expected non-empty result")
	}

	// The enhanced prompt should contain key structural elements.
	for _, want := range []string{"REPO_PLACEHOLDER", "audit", "findings"} {
		if !containsCI(text, want) {
			t.Errorf("result missing expected content %q", want)
		}
	}
}

func TestSweepGenerate_CustomPrompt(t *testing.T) {
	s := &Server{Tasks: NewTaskRegistry()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"custom_prompt": "Review the codebase for security issues. Focus on injection vulnerabilities and authentication gaps.",
	}

	result, err := s.handleSweepGenerate(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if text == "" {
		t.Fatal("expected non-empty result")
	}
}

func TestSweepLaunch_MissingPrompt(t *testing.T) {
	s := &Server{Tasks: NewTaskRegistry()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := s.handleSweepLaunch(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if !containsCI(text, "required") {
		t.Errorf("expected validation error for missing prompt, got: %s", text)
	}
}

func TestSweepLaunch_InvalidRepos(t *testing.T) {
	s := &Server{
		Tasks:   NewTaskRegistry(),
		SessMgr: session.NewManager(),
		Repos:   []*model.Repo{{Name: "test-repo", Path: "/tmp/test-repo"}},
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"prompt": "Audit this repo",
		"repos":  `["nonexistent-repo"]`,
	}

	result, err := s.handleSweepLaunch(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if !containsCI(text, "not found") {
		t.Errorf("expected repo not found error, got: %s", text)
	}
}

func TestSweepStatus_NoSessions(t *testing.T) {
	s := &Server{
		Tasks:   NewTaskRegistry(),
		SessMgr: session.NewManager(),
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sweep_id": "sweep-nonexistent",
	}

	result, err := s.handleSweepStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if !containsCI(text, "empty") {
		t.Errorf("expected empty result for unknown sweep, got: %s", text)
	}
}

func TestSweepStatus_MissingSweepID(t *testing.T) {
	s := &Server{Tasks: NewTaskRegistry()}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := s.handleSweepStatus(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if !containsCI(text, "required") {
		t.Errorf("expected validation error, got: %s", text)
	}
}

func TestSweepNudge_NoStalled(t *testing.T) {
	s := &Server{
		Tasks:   NewTaskRegistry(),
		SessMgr: session.NewManager(),
	}

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"sweep_id": "sweep-test",
	}

	result, err := s.handleSweepNudge(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	text := sweepExtractText(result)
	if !containsCI(text, `"nudged":0`) && !containsCI(text, `"nudged": 0`) {
		t.Errorf("expected zero nudged sessions, got: %s", text)
	}
}

func TestScoreGrade(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{95, "A"},
		{85, "B"},
		{70, "C"},
		{55, "D"},
		{30, "F"},
	}
	for _, tt := range tests {
		got := scoreGrade(enhancer.AnalyzeResult{Score: tt.score})
		if got != tt.want {
			t.Errorf("scoreGrade(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestResolveSweepRepos_JSONArray(t *testing.T) {
	s := &Server{
		Repos: []*model.Repo{
			{Name: "alpha", Path: "/tmp/alpha"},
			{Name: "beta", Path: "/tmp/beta"},
			{Name: "gamma", Path: "/tmp/gamma"},
		},
	}

	refs, err := s.resolveSweepRepos(`["alpha","gamma"]`, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d", len(refs))
	}
	if refs[0].Name != "alpha" || refs[1].Name != "gamma" {
		t.Errorf("unexpected repos: %v, %v", refs[0].Name, refs[1].Name)
	}
}

func TestResolveSweepRepos_Limit(t *testing.T) {
	s := &Server{
		Repos: []*model.Repo{
			{Name: "a", Path: "/tmp/a"},
			{Name: "b", Path: "/tmp/b"},
			{Name: "c", Path: "/tmp/c"},
		},
	}

	refs, err := s.resolveSweepRepos("all", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs (limited), got %d", len(refs))
	}
}

func TestResolveSweepRepos_NotFound(t *testing.T) {
	s := &Server{
		Repos: []*model.Repo{{Name: "real", Path: "/tmp/real"}},
	}

	_, err := s.resolveSweepRepos(`["fake"]`, 10)
	if err == nil {
		t.Fatal("expected error for nonexistent repo")
	}
}

func containsCI(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}

// extractText extracts the text content from an MCP CallToolResult.
func sweepExtractText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	for _, c := range result.Content {
		if tc, ok := c.(mcp.TextContent); ok {
			return tc.Text
		}
	}
	return ""
}
