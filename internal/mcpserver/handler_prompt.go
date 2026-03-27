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
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) getEngine() *enhancer.HybridEngine {
	s.engineOnce.Do(func() {
		provider := os.Getenv("PROMPT_IMPROVER_PROVIDER")
		if provider == "" {
			provider = "claude"
		}
		s.Engine = enhancer.NewHybridEngine(enhancer.LLMConfig{
			Enabled:  hasAPIKeyForProvider(provider),
			Model:    os.Getenv("PROMPT_IMPROVER_MODEL"),
			Provider: provider,
		})
	})
	return s.Engine
}

// hasAPIKeyForProvider checks whether the environment has an API key for the given provider.
func hasAPIKeyForProvider(provider string) bool {
	switch provider {
	case "gemini":
		return os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != ""
	case "openai":
		return os.Getenv("OPENAI_API_KEY") != ""
	default:
		return os.Getenv("ANTHROPIC_API_KEY") != ""
	}
}

func (s *Server) handlePromptAnalyze(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
	}
	result := enhancer.Analyze(prompt)
	if tt := enhancer.ValidTaskType(getStringArg(req, "task_type")); tt != "" {
		result.TaskType = tt
	}
	// Re-score with target provider if specified
	if tp := getStringArg(req, "target_provider"); tp != "" {
		targetProvider := enhancer.ProviderName(tp)
		lints := enhancer.Lint(prompt)
		report := enhancer.Score(prompt, result.TaskType, lints, &result, targetProvider)
		result.ScoreReport = report
		legacyScore := report.Overall / 10
		if legacyScore < 1 {
			legacyScore = 1
		}
		if legacyScore > 10 {
			legacyScore = 10
		}
		result.Score = legacyScore
	}
	return jsonResult(result), nil
}

func (s *Server) handlePromptEnhance(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
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

	// Apply target provider if specified
	if tp := getStringArg(req, "target_provider"); tp != "" {
		cfg.TargetProvider = enhancer.ProviderName(tp)
	}

	mode := enhancer.ValidMode(modeStr)
	if mode == "" || mode == enhancer.ModeLocal {
		result := enhancer.EnhanceWithConfig(prompt, taskType, cfg)
		return jsonResult(result), nil
	}

	result := enhancer.EnhanceHybrid(ctx, prompt, taskType, cfg, s.getEngine(), mode, cfg.TargetProvider)
	return jsonResult(result), nil
}

func (s *Server) handlePromptLint(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
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
		return codedError(ErrInvalidParams, "prompt required"), nil
	}

	// If a specific provider is requested, create a one-off client for it
	providerStr := getStringArg(req, "provider")
	var client enhancer.PromptImprover
	if providerStr != "" && providerStr != "claude" {
		client = enhancer.NewPromptImprover(enhancer.LLMConfig{
			Enabled:  true,
			Provider: providerStr,
		})
	} else {
		engine := s.getEngine()
		if engine != nil {
			client = engine.Client
		}
	}

	if client == nil {
		apiHint := "ANTHROPIC_API_KEY"
		switch providerStr {
		case "gemini":
			apiHint = "GOOGLE_API_KEY"
		case "openai":
			apiHint = "OPENAI_API_KEY"
		}
		return codedError(ErrProviderUnavailable, fmt.Sprintf("LLM not available: set %s", apiHint)), nil
	}

	taskType := enhancer.ValidTaskType(getStringArg(req, "task_type"))
	thinking := getBoolArg(req, "thinking_enabled")
	feedback := getStringArg(req, "feedback")

	result, err := client.Improve(ctx, prompt, enhancer.ImproveOptions{
		ThinkingEnabled: thinking,
		TaskType:        taskType,
		Feedback:        feedback,
		Provider:        client.Provider(),
	})
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("improve failed: %v", err)), nil
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
		return codedError(ErrInvalidParams, "name required"), nil
	}
	varsJSON := getStringArg(req, "vars")
	if varsJSON == "" {
		return codedError(ErrInvalidParams, "vars required"), nil
	}

	var parsedVars map[string]string
	if err := json.Unmarshal([]byte(varsJSON), &parsedVars); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid vars JSON: %v", err)), nil
	}

	tmpl := enhancer.GetTemplate(name)
	if tmpl == nil {
		return codedError(ErrInternal, fmt.Sprintf("template %q not found", name)), nil
	}

	filled := enhancer.FillTemplate(tmpl, parsedVars)
	return textResult(filled), nil
}

func (s *Server) handleClaudeMDCheck(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoName := getStringArg(req, "repo")
	if repoName == "" {
		return codedError(ErrInvalidParams, "repo required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(repoName)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo %q not found", repoName)), nil
	}

	claudeMDPath := filepath.Join(r.Path, "CLAUDE.md")
	findings, err := enhancer.CheckClaudeMD(claudeMDPath)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("check failed: %v", err)), nil
	}

	if findings == nil {
		return emptyResult("claudemd_issues"), nil
	}
	return jsonResult(findings), nil
}

func (s *Server) handlePromptClassify(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
	}
	best, alts := enhancer.ClassifyDetailed(prompt)

	altList := make([]map[string]any, 0, len(alts))
	for _, a := range alts {
		altList = append(altList, map[string]any{
			"task_type":  string(a.TaskType),
			"confidence": a.Confidence,
		})
	}

	return jsonResult(map[string]any{
		"task_type":    string(best.TaskType),
		"confidence":   best.Confidence,
		"alternatives": altList,
	}), nil
}

func (s *Server) handlePromptShouldEnhance(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
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
	} else {
		// Build a reason from scoring dimensions so the caller knows why enhancement is recommended
		ar := enhancer.Analyze(prompt)
		lints := enhancer.Lint(prompt)
		report := enhancer.Score(prompt, ar.TaskType, lints, &ar, cfg.TargetProvider)

		var weakParts []string
		for _, dim := range report.Dimensions {
			if dim.Grade == "D" || dim.Grade == "F" {
				weakParts = append(weakParts, fmt.Sprintf("weak %s (%s)", strings.ToLower(dim.Name), dim.Grade))
			}
		}
		wordCount := len(strings.Fields(strings.TrimSpace(prompt)))
		if wordCount < 20 {
			weakParts = append(weakParts, fmt.Sprintf("under 20 words (%d)", wordCount))
		}

		if len(weakParts) > 0 {
			reason = fmt.Sprintf("score %d/100: %s", report.Overall, strings.Join(weakParts, ", "))
		} else {
			reason = fmt.Sprintf("score %d/100: could benefit from enhancement", report.Overall)
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

// mapSessionProvider maps a session provider to an enhancer provider name.
func mapSessionProvider(p session.Provider) enhancer.ProviderName {
	switch p {
	case session.ProviderGemini:
		return enhancer.ProviderGemini
	case session.ProviderCodex:
		return enhancer.ProviderOpenAI
	default:
		return enhancer.ProviderClaude
	}
}
