package mcpserver

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// Session CRUD and status handlers

func (s *Server) handleSessionList(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	providerFilter := getStringArg(req, "provider")
	statusFilter := getStringArg(req, "status")
	includeEnded := getBoolArg(req, "include_ended")

	var repoPath string
	if repoFilter != "" {
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
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
		Stalled  bool    `json:"stalled,omitempty"`
		Source   string  `json:"source,omitempty"` // "live" or "store"
	}

	// Build a set of stalled session IDs using the default threshold.
	stalledIDs := make(map[string]bool)
	for _, id := range s.SessMgr.DetectStalls(session.DefaultStallThreshold) {
		stalledIDs[id] = true
	}

	// Track live session IDs so store results don't duplicate them.
	liveIDs := make(map[string]bool)

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

		liveIDs[sess.ID] = true
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
			Stalled:  stalledIDs[sess.ID],
			Source:   "live",
		})
	}

	// When include_ended=true and a Store is available, merge historical sessions.
	if includeEnded && s.SessMgr.Store() != nil {
		opts := session.ListOpts{
			RepoPath: repoPath,
		}
		if statusFilter != "" {
			opts.Status = session.SessionStatus(statusFilter)
		}
		stored, err := s.SessMgr.Store().ListSessions(ctx, opts)
		if err == nil {
			for _, sess := range stored {
				if liveIDs[sess.ID] {
					continue // live session takes precedence
				}
				sess.Lock()
				status := string(sess.Status)
				provider := string(sess.Provider)
				spent := sess.SpentUSD
				turns := sess.TurnCount
				sess.Unlock()

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
					Source:   "store",
				})
			}
		}
	}

	if len(summaries) == 0 {
		return emptyResult("sessions"), nil
	}
	return jsonResult(summaries), nil
}

func (s *Server) handleSessionStatus(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
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

func (s *Server) handleSessionOutput(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}
	lines := int(getNumberArg(req, "lines", 20))
	if lines > 100 {
		lines = 100
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
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

func (s *Server) handleSessionBudget(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
	}

	newBudget := getNumberArg(req, "budget_usd", 0)
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

	// Add historical cost breakdown if Store is available.
	if store := s.SessMgr.Store(); store != nil {
		since := time.Now().Add(-24 * time.Hour) // last 24 hours
		if costByProvider, err := store.AggregateCostByProvider(ctx, since); err == nil && len(costByProvider) > 0 {
			info["historical_cost"] = costByProvider
		}
	}

	return jsonResult(info), nil
}

func (s *Server) handleSessionCompare(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id1 := getStringArg(req, "id1")
	id2 := getStringArg(req, "id2")
	if id1 == "" || id2 == "" {
		return codedError(ErrInvalidParams, "both id1 and id2 are required"), nil
	}

	s1, ok1 := s.SessMgr.Get(id1)
	s2, ok2 := s.SessMgr.Get(id2)
	if !ok1 || !ok2 {
		if !ok1 {
			return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id1)), nil
		}
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id2)), nil
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

func (s *Server) handleSessionTail(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}
	lines := int(getNumberArg(req, "lines", 30))
	if lines > 100 {
		lines = 100
	}
	if lines < 1 {
		lines = 30
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	totalCount := sess.TotalOutputCount
	status := sess.Status
	lastActivity := sess.LastActivity
	sess.Unlock()

	cursorStr := getStringArg(req, "cursor")
	var output []string

	if cursorStr != "" {
		cursor, err := strconv.Atoi(cursorStr)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
		// cursor is the TotalOutputCount at the time of last call.
		// New lines since cursor = totalCount - cursor.
		newLines := totalCount - cursor
		if newLines <= 0 {
			output = nil
		} else {
			startIdx := len(history) - newLines
			if startIdx < 0 {
				startIdx = 0
			}
			output = history[startIdx:]
			if len(output) > lines {
				output = output[len(output)-lines:]
			}
		}
	} else {
		// No cursor: return last N lines
		if len(history) > lines {
			output = history[len(history)-lines:]
		} else {
			output = history
		}
	}

	idleSeconds := time.Since(lastActivity).Seconds()

	return jsonResult(map[string]any{
		"session_id":     id,
		"status":         string(status),
		"output":         output,
		"lines_returned": len(output),
		"next_cursor":    strconv.Itoa(totalCount),
		"is_active":      status == session.StatusRunning || status == session.StatusLaunching,
		"idle_seconds":   int(idleSeconds),
	}), nil
}

func (s *Server) handleSessionDiff(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	id := getStringArg(req, "id")
	if id == "" {
		return codedError(ErrInvalidParams, "session id required"), nil
	}

	sess, ok := s.SessMgr.Get(id)
	if !ok {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %s not found — use ralphglasses_session_list to find active sessions", id)), nil
	}

	sess.Lock()
	repoPath := sess.RepoPath
	repoName := sess.RepoName
	launchedAt := sess.LaunchedAt
	endedAt := sess.EndedAt
	sess.Unlock()

	until := time.Now()
	if endedAt != nil {
		until = *endedAt
	}

	statOnly := getStringArg(req, "stat_only") != "false"
	maxLines := int(getNumberArg(req, "max_lines", 200))

	commits, err := session.GitLogSince(repoPath, launchedAt, until)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("git log: %v", err)), nil
	}

	diffText, stat, truncated, err := session.GitDiffWindow(repoPath, launchedAt, until, statOnly, maxLines)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("git diff: %v", err)), nil
	}

	duration := until.Sub(launchedAt).Round(time.Second).String()

	result := map[string]any{
		"session_id": id,
		"repo":       repoName,
		"window": map[string]any{
			"started":  launchedAt.Format(time.RFC3339),
			"ended":    until.Format(time.RFC3339),
			"duration": duration,
		},
		"commits":   commits,
		"stat":      stat,
		"truncated": truncated,
	}
	if diffText != "" {
		result["diff"] = diffText
	}

	return jsonResult(result), nil
}

func (s *Server) handleSessionErrors(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	repoFilter := getStringArg(req, "repo")
	severityFilter := getStringArg(req, "severity")
	limit := int(getNumberArg(req, "limit", 50))
	if limit < 1 {
		limit = 50
	}

	allSessions := s.SessMgr.List("")

	type errorEntry struct {
		SessionID string `json:"session_id"`
		Repo      string `json:"repo"`
		Provider  string `json:"provider"`
		Severity  string `json:"severity"`
		Type      string `json:"type"`
		Message   string `json:"message"`
		Timestamp string `json:"timestamp"`
	}

	errors := make([]errorEntry, 0)
	byType := make(map[string]int)
	bySeverity := make(map[string]int)
	healthySessions := 0
	sessionsWithErrors := 0

	for _, sess := range allSessions {
		sess.Lock()
		repo := sess.RepoName
		provider := string(sess.Provider)
		hasError := false

		if repoFilter != "" && repo != repoFilter {
			sess.Unlock()
			continue
		}

		ts := sess.LastActivity.Format(time.RFC3339)

		// Critical: errored sessions
		if sess.Error != "" || sess.Status == session.StatusErrored {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "critical",
				Type:      "session_error",
				Message:   truncateForAlert(firstNonEmptyStr(sess.Error, sess.ExitReason, "unknown error"), 200),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["session_error"]++
			bySeverity["critical"]++
		}

		// Warning: stream parse errors
		if sess.StreamParseErrors > 0 {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "warning",
				Type:      "stream_parse",
				Message:   fmt.Sprintf("%d parse errors", sess.StreamParseErrors),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["stream_parse"]++
			bySeverity["warning"]++
		}

		// Warning: budget warning
		if sess.BudgetUSD > 0 && sess.SpentUSD/sess.BudgetUSD >= 0.80 {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "warning",
				Type:      "budget_warning",
				Message:   fmt.Sprintf("%.0f%% of budget used ($%.2f/$%.2f)", sess.SpentUSD/sess.BudgetUSD*100, sess.SpentUSD, sess.BudgetUSD),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["budget_warning"]++
			bySeverity["warning"]++
		}

		// Info: stopped with reason
		if sess.Status == session.StatusStopped && sess.ExitReason != "" {
			hasError = true
			e := errorEntry{
				SessionID: sess.ID,
				Repo:      repo,
				Provider:  provider,
				Severity:  "info",
				Type:      "session_stopped",
				Message:   truncateForAlert(sess.ExitReason, 200),
				Timestamp: ts,
			}
			errors = append(errors, e)
			byType["session_stopped"]++
			bySeverity["info"]++
		}

		if hasError {
			sessionsWithErrors++
		} else {
			healthySessions++
		}
		sess.Unlock()
	}

	// Filter by severity
	if severityFilter != "" {
		var filtered []errorEntry
		for _, e := range errors {
			if e.Severity == severityFilter {
				filtered = append(filtered, e)
			}
		}
		errors = filtered
	}

	// Sort: critical first, then warning, then info
	severityOrder := map[string]int{"critical": 0, "warning": 1, "info": 2}
	for i := 0; i < len(errors); i++ {
		for j := i + 1; j < len(errors); j++ {
			if severityOrder[errors[i].Severity] > severityOrder[errors[j].Severity] {
				errors[i], errors[j] = errors[j], errors[i]
			}
		}
	}

	// Cap at limit
	if len(errors) > limit {
		errors = errors[:limit]
	}

	return jsonResult(map[string]any{
		"total_errors":         len(errors),
		"by_type":              byType,
		"by_severity":          bySeverity,
		"errors":               errors,
		"sessions_with_errors": sessionsWithErrors,
		"healthy_sessions":     healthySessions,
	}), nil
}

func (s *Server) handleSessionReplayDiff(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pathA := getStringArg(req, "path_a")
	pathB := getStringArg(req, "path_b")
	if pathA == "" || pathB == "" {
		return codedError(ErrInvalidParams, "both path_a and path_b are required"), nil
	}

	playerA, err := session.NewPlayer(pathA)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("cannot load replay A: %v", err)), nil
	}
	playerB, err := session.NewPlayer(pathB)
	if err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("cannot load replay B: %v", err)), nil
	}

	diff, err := session.DiffSessions(playerA, playerB)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("diff failed: %v", err)), nil
	}

	// Cap events in the response to avoid oversized payloads.
	maxEvents := int(getNumberArg(req, "max_events", 100))
	if maxEvents < 1 {
		maxEvents = 100
	}
	truncated := false
	events := diff.Events
	if len(events) > maxEvents {
		events = events[:maxEvents]
		truncated = true
	}

	// Build event summaries.
	eventSummaries := make([]map[string]any, len(events))
	for i, de := range events {
		entry := map[string]any{
			"status": string(de.Status),
		}
		if de.EventA != nil {
			entry["type_a"] = string(de.EventA.Type)
			entry["data_a"] = truncateForAlert(de.EventA.Data, 200)
			entry["offset_a"] = de.OffsetA.String()
		}
		if de.EventB != nil {
			entry["type_b"] = string(de.EventB.Type)
			entry["data_b"] = truncateForAlert(de.EventB.Data, 200)
			entry["offset_b"] = de.OffsetB.String()
		}
		eventSummaries[i] = entry
	}

	return jsonResult(map[string]any{
		"session_id_a": diff.SessionIDA,
		"session_id_b": diff.SessionIDB,
		"total_a":      diff.TotalA,
		"total_b":      diff.TotalB,
		"matched":      diff.Matched,
		"modified":     diff.Modified,
		"only_a":       diff.OnlyA,
		"only_b":       diff.OnlyB,
		"similarity":   diff.Similarity,
		"duration_a":   diff.DurationA.String(),
		"duration_b":   diff.DurationB.String(),
		"events":       eventSummaries,
		"truncated":    truncated,
	}), nil
}

func truncateForAlert(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func firstNonEmptyStr(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
