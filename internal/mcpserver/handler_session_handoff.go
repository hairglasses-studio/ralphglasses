package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

func (s *Server) handleSessionHandoff(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	sourceID := getStringArg(req, "source_session_id")
	if sourceID == "" {
		return codedError(ErrInvalidParams, "source_session_id required"), nil
	}

	if s.SessMgr == nil {
		return codedError(ErrFilesystem, "session manager not available"), nil
	}

	src, ok := s.SessMgr.Get(sourceID)
	if !ok {
		return codedError(ErrInvalidParams, fmt.Sprintf("session %s not found", sourceID)), nil
	}

	targetProvider := getStringArg(req, "target_provider")
	explicitTargetProvider := targetProvider != ""
	// Default include_context to true.
	includeContext := true
	if getBoolArg(req, "include_context") {
		includeContext = true
	}
	// Check if explicitly set to false via the string value.
	if v := getStringArg(req, "include_context"); v == "false" {
		includeContext = false
	}
	stopSource := getBoolArg(req, "stop_source")
	contextLines := int(getNumberArg(req, "context_lines", 5))
	handoffReason := getStringArg(req, "handoff_reason")

	// Read exported fields — these are set at launch and mostly immutable.
	prompt := src.Prompt
	repoPath := src.RepoPath
	spentUSD := src.SpentUSD
	budgetUSD := src.BudgetUSD
	maxTurns := src.MaxTurns
	provider := src.Provider
	model := src.Model
	teamName := src.TeamName

	// Build handoff context from observations and scratchpad.
	var contextPayload string
	if includeContext {
		var parts []string

		// Read last N observations.
		obsPath := filepath.Join(repoPath, ".ralph", "logs", "loop_observations.jsonl")
		if data, err := os.ReadFile(obsPath); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			start := len(lines) - contextLines
			if start < 0 {
				start = 0
			}
			for _, line := range lines[start:] {
				parts = append(parts, line)
			}
		}

		// Build context header with cost info.
		header := fmt.Sprintf("## Handoff Context (from session %s)\n\nCost so far: $%.4f", sourceID[:8], spentUSD)
		if handoffReason != "" {
			header += fmt.Sprintf("\nReason: %s", handoffReason)
		}

		if len(parts) > 0 {
			contextPayload = fmt.Sprintf("%s\n\nPrevious observations:\n%s\n\nOriginal prompt: %s",
				header, strings.Join(parts, "\n"), prompt)
		} else {
			// No observations — still include cost and prompt context.
			contextPayload = fmt.Sprintf("%s\n\nNo prior observations available.\n\nOriginal prompt: %s",
				header, prompt)
		}
	}

	// Determine target provider.
	tp, _, err := parseOptionalLaunchProvider(targetProvider)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown provider: %s", targetProvider)), nil
	}
	if tp == "" {
		tp = provider // same provider when known; otherwise allow runtime selection
	}
	rerouteReason := ""
	tp, rerouteReason = s.rerouteClaudeProviderForCacheHealth(repoPath, tp, explicitTargetProvider)

	// Build enriched prompt.
	handoffPrompt := prompt
	if contextPayload != "" {
		handoffPrompt = contextPayload + "\n\n---\n\n" + prompt
	}
	if rerouteReason != "" {
		handoffPrompt = rerouteReason + "\n\n---\n\n" + handoffPrompt
	}

	remaining := budgetUSD - spentUSD
	if remaining < 0.5 {
		remaining = 0.5 // minimum $0.50 for handoff
	}

	opts := session.LaunchOptions{
		Provider:     tp,
		RepoPath:     repoPath,
		Prompt:       handoffPrompt,
		Model:        model,
		MaxBudgetUSD: remaining,
		MaxTurns:     maxTurns,
		TeamName:     teamName,
	}
	if opts.Provider != "" && strings.TrimSpace(opts.Model) == "" {
		opts.Model = session.ProviderDefaults(opts.Provider)
	}

	// Stop source if requested.
	sourceStopped := false
	if stopSource {
		if err := s.SessMgr.Stop(sourceID); err == nil {
			sourceStopped = true
		}
	}

	newSession, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrFilesystem, fmt.Sprintf("launch handoff session: %v", err)), nil
	}

	result := map[string]any{
		"new_session_id":       newSession.ID,
		"source_session_id":    sourceID,
		"source_stopped":       sourceStopped,
		"target_provider":      string(newSession.Provider),
		"transferred_context":  includeContext,
		"context_size_bytes":   len(contextPayload),
		"remaining_budget":     remaining,
		"cost_so_far":          spentUSD,
		"handoff_reason":       handoffReason,
		"handoff_at":           time.Now().UTC().Format(time.RFC3339),
		"cache_reroute_reason": rerouteReason,
	}
	if explicitTargetProvider {
		result["requested_target_provider"] = strings.ToLower(strings.TrimSpace(targetProvider))
	}
	if newSession.ProviderAutoSelected {
		result["provider_auto_selected"] = true
	}
	if strings.TrimSpace(newSession.ProviderSelectionReason) != "" {
		result["provider_selection_reason"] = newSession.ProviderSelectionReason
	}

	// Save handoff record.
	handoffDir := filepath.Join(repoPath, ".ralph", "handoffs")
	os.MkdirAll(handoffDir, 0o755)
	if data, err := json.MarshalIndent(result, "", "  "); err == nil {
		os.WriteFile(filepath.Join(handoffDir, fmt.Sprintf("handoff-%d.json", time.Now().Unix())), data, 0o644)
	}

	return jsonResult(result), nil
}
