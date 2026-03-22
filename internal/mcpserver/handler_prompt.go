package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
)

func (s *Server) getEngine() *enhancer.HybridEngine {
	s.engineOnce.Do(func() {
		s.Engine = enhancer.NewHybridEngine(enhancer.LLMConfig{
			Enabled: os.Getenv("ANTHROPIC_API_KEY") != "",
			Model:   os.Getenv("PROMPT_IMPROVER_MODEL"),
		})
	})
	return s.Engine
}

func (s *Server) handlePromptAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	result := enhancer.Analyze(prompt)
	if tt := enhancer.ValidTaskType(getStringArg(req, "task_type")); tt != "" {
		result.TaskType = tt
	}
	return jsonResult(result), nil
}

func (s *Server) handlePromptEnhance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	taskType := enhancer.ValidTaskType(getStringArg(req, "task_type"))
	modeStr := getStringArg(req, "mode")

	// Load config from repo if specified
	var cfg enhancer.Config
	if repoName := getStringArg(req, "repo"); repoName != "" {
		if r := s.findRepo(repoName); r != nil {
			cfg = enhancer.LoadConfig(r.Path)
		}
	}

	mode := enhancer.ValidMode(modeStr)
	if mode == "" || mode == enhancer.ModeLocal {
		result := enhancer.EnhanceWithConfig(prompt, taskType, cfg)
		return jsonResult(result), nil
	}

	result := enhancer.EnhanceHybrid(ctx, prompt, taskType, cfg, s.getEngine(), mode)
	return jsonResult(result), nil
}

func (s *Server) handlePromptLint(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	results := enhancer.Lint(prompt)
	cacheResults := enhancer.VerifyCacheFriendlyOrder(prompt)
	return jsonResult(map[string]any{
		"findings":     results,
		"cache_checks": cacheResults,
		"total":        len(results) + len(cacheResults),
	}), nil
}

func (s *Server) handlePromptImprove(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	engine := s.getEngine()
	if engine == nil || engine.Client == nil {
		return errResult("LLM not available: set ANTHROPIC_API_KEY"), nil
	}
	taskType := enhancer.ValidTaskType(getStringArg(req, "task_type"))
	thinking := getBoolArg(req, "thinking_enabled")
	feedback := getStringArg(req, "feedback")

	result, err := engine.Client.Improve(ctx, prompt, enhancer.ImproveOptions{
		ThinkingEnabled: thinking,
		TaskType:        taskType,
		Feedback:        feedback,
	})
	if err != nil {
		return errResult(fmt.Sprintf("improve failed: %v", err)), nil
	}
	return jsonResult(result), nil
}

func (s *Server) handlePromptTemplates(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	templates := enhancer.ListTemplates()
	return jsonResult(templates), nil
}

func (s *Server) handlePromptTemplateFill(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "name")
	if name == "" {
		return errResult("name required"), nil
	}
	varsJSON := getStringArg(req, "vars")
	if varsJSON == "" {
		return errResult("vars required"), nil
	}

	var parsedVars map[string]string
	if err := json.Unmarshal([]byte(varsJSON), &parsedVars); err != nil {
		return errResult(fmt.Sprintf("invalid vars JSON: %v", err)), nil
	}

	tmpl := enhancer.GetTemplate(name)
	if tmpl == nil {
		return errResult(fmt.Sprintf("template %q not found", name)), nil
	}

	filled := enhancer.FillTemplate(tmpl, parsedVars)
	return textResult(filled), nil
}

func (s *Server) handleClaudeMDCheck(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return errResult("repo required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return errResult(fmt.Sprintf("repo %q not found", repoName)), nil
	}

	claudeMDPath := filepath.Join(r.Path, "CLAUDE.md")
	findings, err := enhancer.CheckClaudeMD(claudeMDPath)
	if err != nil {
		return errResult(fmt.Sprintf("check failed: %v", err)), nil
	}

	return jsonResult(findings), nil
}

func (s *Server) handlePromptClassify(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	taskType := enhancer.Classify(prompt)
	return jsonResult(map[string]any{"task_type": string(taskType)}), nil
}

func (s *Server) handlePromptShouldEnhance(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}

	var cfg enhancer.Config
	if repoName := getStringArg(req, "repo"); repoName != "" {
		if s.reposNil() {
			_ = s.scan()
		}
		if r := s.findRepo(repoName); r != nil {
			cfg = enhancer.LoadConfig(r.Path)
		}
	}

	should := enhancer.ShouldEnhance(prompt, cfg)

	reason := ""
	if !should {
		trimmed := strings.TrimSpace(prompt)
		minWords := cfg.Hook.MinWordCount
		if minWords <= 0 {
			minWords = 5
		}
		words := strings.Fields(trimmed)
		switch {
		case len(words) < minWords:
			reason = fmt.Sprintf("too short: %d words (minimum %d)", len(words), minWords)
		case isConversational(trimmed):
			reason = "conversational reply"
		case hasXMLStructure(trimmed):
			reason = "already has XML structure"
		default:
			reason = "matched skip pattern"
		}
	}

	return jsonResult(map[string]any{
		"should_enhance": should,
		"reason":         reason,
	}), nil
}

// isConversational checks if a prompt matches conversational patterns.
func isConversational(s string) bool {
	convPatterns := []string{
		"y", "n", "yes", "no", "ok", "k", "sure", "thanks", "done",
		"continue", "go ahead", "looks good", "lgtm", "ship it", "do it",
		"next", "proceed", "approve", "reject", "cancel", "stop", "undo",
		"revert", "nah",
	}
	lower := strings.ToLower(s)
	for _, p := range convPatterns {
		if lower == p {
			return true
		}
	}
	return false
}

// hasXMLStructure checks if a prompt already has XML structure tags.
func hasXMLStructure(s string) bool {
	lower := strings.ToLower(s)
	tags := []string{"<instructions", "<role", "<system", "<prompt"}
	for _, tag := range tags {
		if strings.Contains(lower, tag) {
			return true
		}
	}
	return false
}
