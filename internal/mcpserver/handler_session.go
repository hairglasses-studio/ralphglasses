package mcpserver

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSessionLaunch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	prompt := getStringArg(req, "prompt")
	if prompt == "" {
		return errResult("prompt required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.ProviderClaude
	}

	opts := session.LaunchOptions{
		Provider:     provider,
		RepoPath:     r.Path,
		Prompt:       prompt,
		Model:        getStringArg(req, "model"),
		MaxBudgetUSD: getNumberArg(req, "max_budget_usd", 0),
		MaxTurns:     int(getNumberArg(req, "max_turns", 0)),
		Agent:        getStringArg(req, "agent"),
		SystemPrompt: getStringArg(req, "system_prompt"),
		SessionName:  getStringArg(req, "session_name"),
		Worktree:     getStringArg(req, "worktree"),
	}
	if tools := getStringArg(req, "allowed_tools"); tools != "" {
		opts.AllowedTools = strings.Split(tools, ",")
	}

	enhanceMode := getStringArg(req, "enhance_prompt")
	if enhanceMode != "" {
		cfg := enhancer.LoadConfig(r.Path)
		if enhancer.ShouldEnhance(prompt, cfg) {
			mode := enhancer.ValidMode(enhanceMode)
			if mode == "" {
				mode = enhancer.ModeLocal
			}
			eResult := enhancer.EnhanceHybrid(ctx, prompt, "", cfg, s.getEngine(), mode)
			opts.Prompt = eResult.Enhanced
		}
	}

	sess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		return errResult(fmt.Sprintf("launch failed: %v", err)), nil
	}

	result := map[string]any{
		"session_id": sess.ID,
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

func (s *Server) handleSessionList(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")
	statusFilter := getStringArg(req, "status")

	var repoPath string
	if repoFilter != "" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return errResult(fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		r := s.findRepo(repoFilter)
		if r != nil {
			repoPath = r.Path
		}
	}

	sessions := s.SessMgr.List(repoPath)

	type sessionSummary struct {
		ID       string  `json:"id"`
		Provider string  `json:"provider"`
		Repo     string  `json:"repo"`
		Status   string  `json:"status"`
		Model    string  `json:"model,omitempty"`
		SpentUSD float64 `json:"spent_usd"`
		Turns    int     `json:"turns"`
		Agent    string  `json:"agent,omitempty"`
		Team     string  `json:"team,omitempty"`
	}

	var summaries []sessionSummary
	for _, sess := range sessions {
		sess.Lock()
		status := string(sess.Status)
		provider := string(sess.Provider)
		spent := sess.SpentUSD
		turns := sess.TurnCount
		sess.Unlock()

		if statusFilter != "" && status != statusFilter {
			continue
		}
		if providerFilter != "" && provider != providerFilter {
			continue
		}

		summaries = append(summaries, sessionSummary{
			ID:       sess.ID,
			Provider: provider,
			Repo:     sess.RepoName,
			Status:   status,
			Model:    sess.Model,
			SpentUSD: spent,
			Turns:    turns,
			Agent:    sess.AgentName,
			Team:     sess.TeamName,
		})
	}

	if summaries == nil {
		summaries = []sessionSummary{}
	}
	return jsonResult(summaries), nil
}

func (s *Server) handleSessionStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	detail := map[string]any{
		"id":                  sess.ID,
		"provider":            sess.Provider,
		"provider_session_id": sess.ProviderSessionID,
		"repo":                sess.RepoName,
		"repo_path":           sess.RepoPath,
		"status":              sess.Status,
		"prompt":              sess.Prompt,
		"model":               sess.Model,
		"agent":               sess.AgentName,
		"team":                sess.TeamName,
		"budget_usd":          sess.BudgetUSD,
		"spent_usd":           sess.SpentUSD,
		"turns":               sess.TurnCount,
		"max_turns":           sess.MaxTurns,
		"launched_at":         sess.LaunchedAt,
		"last_activity":       sess.LastActivity,
		"exit_reason":         sess.ExitReason,
		"last_output":         sess.LastOutput,
		"error":               sess.Error,
	}
	if sess.EndedAt != nil {
		detail["ended_at"] = sess.EndedAt
	}
	sess.Unlock()

	return jsonResult(detail), nil
}

func (s *Server) handleSessionResume(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	name := getStringArg(req, "repo")
	if name == "" {
		return errResult("repo name required"), nil
	}
	sessionID := getStringArg(req, "session_id")
	if sessionID == "" {
		return errResult("session_id required"), nil
	}
	if s.reposNil() {
		if err := s.scan(); err != nil {
			return errResult(fmt.Sprintf("scan failed: %v", err)), nil
		}
	}
	r := s.findRepo(name)
	if r == nil {
		return errResult(fmt.Sprintf("repo not found: %s", name)), nil
	}

	provider := session.Provider(getStringArg(req, "provider"))
	if provider == "" {
		provider = session.ProviderClaude
	}
	prompt := getStringArg(req, "prompt")
	sess, err := s.SessMgr.Resume(ctx, r.Path, provider, sessionID, prompt)
	if err != nil {
		return errResult(fmt.Sprintf("resume failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"session_id":        sess.ID,
		"resumed_from":      sessionID,
		"repo":              sess.RepoName,
		"status":            sess.Status,
	}), nil
}

func (s *Server) handleSessionStop(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	if err := s.SessMgr.Stop(id); err != nil {
		return errResult(fmt.Sprintf("stop failed: %v", err)), nil
	}
	return textResult(fmt.Sprintf("Stopped session %s", id)), nil
}

func (s *Server) handleSessionBudget(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	newBudget := getNumberArg(req, "budget", 0)
	if newBudget > 0 {
		sess.Lock()
		sess.BudgetUSD = newBudget
		sess.Unlock()
	}

	sess.Lock()
	info := map[string]any{
		"session_id": sess.ID,
		"budget_usd": sess.BudgetUSD,
		"spent_usd":  sess.SpentUSD,
		"remaining":  sess.BudgetUSD - sess.SpentUSD,
		"turns":      sess.TurnCount,
		"status":     sess.Status,
	}
	sess.Unlock()

	return jsonResult(info), nil
}

func (s *Server) handleSessionRetry(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	opts := session.LaunchOptions{
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
	if b := getNumberArg(req, "max_budget_usd", 0); b > 0 {
		opts.MaxBudgetUSD = b
	}

	newSess, err := s.SessMgr.Launch(ctx, opts)
	if err != nil {
		return errResult(fmt.Sprintf("retry failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"original_id": id,
		"new_id":      newSess.ID,
		"provider":    string(newSess.Provider),
		"status":      "launched",
	}), nil
}

func (s *Server) handleSessionCompare(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id1 := getStringArg(req, "id1")
	id2 := getStringArg(req, "id2")
	if id1 == "" || id2 == "" {
		return errResult("both id1 and id2 are required"), nil
	}

	s1, ok1 := s.SessMgr.Get(id1)
	s2, ok2 := s.SessMgr.Get(id2)
	if !ok1 {
		return errResult(fmt.Sprintf("session not found: %s", id1)), nil
	}
	if !ok2 {
		return errResult(fmt.Sprintf("session not found: %s", id2)), nil
	}

	extract := func(sess *session.Session) map[string]any {
		sess.Lock()
		defer sess.Unlock()
		dur := time.Since(sess.LaunchedAt)
		if sess.EndedAt != nil {
			dur = sess.EndedAt.Sub(sess.LaunchedAt)
		}
		costPerTurn := 0.0
		if sess.TurnCount > 0 {
			costPerTurn = sess.SpentUSD / float64(sess.TurnCount)
		}
		turnsPerMin := 0.0
		if dur.Minutes() > 0 {
			turnsPerMin = float64(sess.TurnCount) / dur.Minutes()
		}
		return map[string]any{
			"id":            sess.ID,
			"provider":      string(sess.Provider),
			"status":        string(sess.Status),
			"model":         sess.Model,
			"spent_usd":     sess.SpentUSD,
			"turns":         sess.TurnCount,
			"duration":      dur.String(),
			"cost_per_turn": costPerTurn,
			"turns_per_min": turnsPerMin,
		}
	}

	return jsonResult(map[string]any{
		"session_1": extract(s1),
		"session_2": extract(s2),
	}), nil
}

func (s *Server) handleSessionOutput(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return errResult("session id required"), nil
	}
	lines := int(getNumberArg(req, "lines", 20))
	if lines > 100 {
		lines = 100
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return errResult(fmt.Sprintf("session not found: %s", id)), nil
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	sess.Unlock()

	if len(history) > lines {
		history = history[len(history)-lines:]
	}

	return jsonResult(map[string]any{
		"session_id": id,
		"lines":      len(history),
		"output":     history,
	}), nil
}
