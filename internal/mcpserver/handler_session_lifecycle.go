package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Session lifecycle handlers (launch, stop, resume, retry)

func (s *Server) handleSessionLaunch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	p := NewParams(req)

	name, errResult := p.RequireString("repo")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrRepoNameInvalid, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	prompt, errResult := p.RequireString("prompt")
	if errResult != nil {
		return errResult, nil
	}
	if err := ValidateStringLength(prompt, MaxPromptLength, "prompt"); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(p.OptionalString("provider", ""))
	if provider == "" {
		provider = session.DefaultPrimaryProvider()
	}
	systemPrompt := p.OptionalString("system_prompt", "")
	if err := ValidateStringLength(systemPrompt, MaxPromptLength, "system_prompt"); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}

	opts := session.LaunchOptions{
		TenantID:               session.NormalizeTenantID(p.OptionalString("tenant_id", "")),
		Provider:               provider,
		RepoPath:               r.Path,
		Prompt:                 prompt,
		Model:                  p.OptionalString("model", ""),
		MaxBudgetUSD:           p.OptionalNumber("budget_usd", 0),
		MaxTurns:               int(p.OptionalNumber("max_turns", 0)),
		Agent:                  p.OptionalString("agent", ""),
		SystemPrompt:           systemPrompt,
		SessionName:            p.OptionalString("session_name", ""),
		Worktree:               p.OptionalString("worktree", ""),
		StrictProviderContract: true,
	}
	if p.OptionalBool("bare", false) {
		opts.Bare = true
	}
	if effort := p.OptionalString("effort", ""); effort != "" {
		opts.Effort = effort
	}
	if fallback := p.OptionalString("fallback_model", ""); fallback != "" {
		opts.FallbackModel = fallback
	}
	if tools := p.OptionalString("allowed_tools", ""); tools != "" {
		opts.AllowedTools = strings.Split(tools, ",")
	}
	if schema := p.OptionalString("output_schema", ""); schema != "" {
		if !json.Valid([]byte(schema)) {
			return codedError(ErrInvalidParams, "output_schema must be valid JSON"), nil
		}
		opts.OutputSchema = json.RawMessage(schema)
	}
	if err := session.ValidateProvider(provider); err != nil {
		return codedError(ErrProviderUnavailable, fmt.Sprintf("invalid provider %q: %v", provider, err)), nil
	}

	// Inject improvement context from journal
	if p.OptionalString("no_journal", "") != "true" {
		journal, _ := session.ReadRecentJournal(r.Path, 5)
		if len(journal) > 0 {
			journalCtx := session.SynthesizeContext(journal)
			if journalCtx != "" {
				opts.Prompt = journalCtx + "\n\n---\n\n" + opts.Prompt
			}
		}
	}

	// Auto-enhance prompt if requested
	enhanceMode := p.OptionalString("enhance_prompt", "")
	if enhanceMode != "" {
		cfg := enhancer.LoadConfig(r.Path)
		if enhancer.ShouldEnhance(prompt, cfg) {
			mode := enhancer.ValidMode(enhanceMode)
			if mode == "" {
				mode = enhancer.ModeLocal
			}
			targetProvider := enhancer.ProviderName(p.OptionalString("target_provider", ""))
			if targetProvider == "" {
				targetProvider = mapSessionProvider(provider)
			}
			eResult := enhancer.EnhanceHybrid(ctx, prompt, "", cfg, s.getEngine(), mode, targetProvider)
			opts.Prompt = eResult.Enhanced
		}
	}

	// Use background context so sessions outlive the MCP tool call.
	sess, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("launch failed: %v", err)), nil
	}

	result := map[string]any{
		"session_id": sess.ID,
		"tenant_id":  sess.TenantID,
		"provider":   sess.Provider,
		"repo":       sess.RepoName,
		"status":     sess.Status,
		"model":      sess.Model,
		"budget_usd": sess.BudgetUSD,
	}
	if warnings := session.UnsupportedOptionsWarnings(provider, opts); len(warnings) > 0 {
		result["warnings"] = warnings
	}
	if enhanceMode != "" && opts.Prompt != prompt {
		result["prompt_enhanced"] = true
		result["original_prompt"] = prompt
		if s.EventBus != nil {
			s.EventBus.Publish(events.Event{
				Type: events.PromptEnhanced,
				Data: map[string]any{"session_id": sess.ID, "repo": name},
			})
		}
	}

	return jsonResult(result), nil
}

func (s *Server) handleSessionStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	if _, ok := s.SessMgr.GetForTenant(id, tenantID); !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found in tenant %s — use ralphglasses_session_list to find active sessions", id, tenantID)), nil
	}

	if err := s.SessMgr.Stop(id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
		}
		return codedError(ErrInternal, fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped session %s", id)), nil
}

func (s *Server) handleSessionStopAll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	// Count running sessions before stopping
	sessions := s.SessMgr.ListByTenant("", tenantID)
	running := 0
	for _, sess := range sessions {
		sess.Lock()
		if sess.Status == session.StatusRunning || sess.Status == session.StatusLaunching {
			running++
		}
		sess.Unlock()
	}

	s.SessMgr.StopAllForTenant(tenantID)

	return textResult(fmt.Sprintf("Stopped %d running session(s) in tenant %s", running, tenantID)), nil
}

func (s *Server) handleSessionResume(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	sessionID := getStringArg(req, "session_id")
	if sessionID == "" {
		return codedError(ErrInvalidParams, "session_id required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.DefaultPrimaryProvider()
	}
	prompt := getStringArg(req, "prompt")
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))
	sess, err := s.SessMgr.ResumeWithTenant(ctx, tenantID, r.Path, provider, sessionID, prompt)
	if err != nil {
		return codedError(ErrLaunchFailed, fmt.Sprintf("resume failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"session_id":   sess.ID,
		"tenant_id":    sess.TenantID,
		"resumed_from": sessionID,
		"repo":         sess.RepoName,
		"status":       sess.Status,
	}), nil
}

func (s *Server) handleSessionRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}
	tenantID := session.NormalizeTenantID(getStringArg(req, "tenant_id"))

	sess, ok := s.SessMgr.GetForTenant(id, tenantID)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
	}

	sess.Lock()
	opts := session.LaunchOptions{
		TenantID:     sess.TenantID,
		Provider:     sess.Provider,
		RepoPath:     sess.RepoPath,
		Prompt:       sess.Prompt,
		Model:        sess.Model,
		MaxBudgetUSD: sess.BudgetUSD,
		MaxTurns:     sess.MaxTurns,
		Agent:        sess.AgentName,
		TeamName:     sess.TeamName,
	}
	sess.Unlock()

	// Apply overrides
	if m := getStringArg(req, "model"); m != "" {
		opts.Model = m
	}
	if b := getNumberArg(req, "budget_usd", 0); b > 0 {
		opts.MaxBudgetUSD = b
	}

	newSess, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrLaunchFailed, fmt.Sprintf("retry failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"original_id": id,
		"new_id":      newSess.ID,
		"tenant_id":   newSess.TenantID,
		"provider":    string(newSess.Provider),
		"status":      "launched",
	}), nil
}
