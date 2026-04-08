package mcpserver

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// ── session_triage ──────────────────────────────────────────────────────────

func (s *Server) handleSessionTriage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)

	since, err := parseTimeParam(pp.String("since"), time.Hour)
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	until, err := parseTimeParam(pp.String("until"), 0)
	if err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	if until.IsZero() {
		until = time.Now()
	}
	repoFilter := pp.String("repo")
	statusFilter := pp.String("status")
	if statusFilter == "" {
		statusFilter = "interrupted,errored"
	}

	statuses := strings.Split(statusFilter, ",")

	return jsonResult(s.buildSessionTriageSummary(ctx, repoFilter, statuses, since, until)), nil
}

func (s *Server) collectTriagedSessions(ctx context.Context, repoFilter string, statuses []string, since, until time.Time) []*session.Session {
	all := make([]*session.Session, 0)
	seen := make(map[string]struct{})

	for _, st := range statuses {
		st = strings.TrimSpace(st)
		if st == "" {
			continue
		}
		opts := session.ListOpts{
			Status:   session.SessionStatus(st),
			RepoName: repoFilter,
			Since:    since,
			Until:    until,
		}
		if s.SessMgr != nil && s.SessMgr.Store() != nil {
			sessions, _ := s.SessMgr.Store().ListSessions(ctx, opts)
			for _, sess := range sessions {
				if sess == nil {
					continue
				}
				if _, ok := seen[sess.ID]; ok {
					continue
				}
				seen[sess.ID] = struct{}{}
				all = append(all, sess)
			}
		}
	}

	if s.SessMgr != nil {
		for _, sess := range s.SessMgr.List("") {
			if sess == nil {
				continue
			}
			if _, ok := seen[sess.ID]; ok {
				continue
			}
			isMatch := false
			for _, st := range statuses {
				if string(sess.Status) == strings.TrimSpace(st) {
					isMatch = true
					break
				}
			}
			if !isMatch {
				continue
			}
			if repoFilter != "" && sess.RepoName != repoFilter {
				continue
			}
			if sess.LaunchedAt.Before(since) || sess.LaunchedAt.After(until) {
				continue
			}
			seen[sess.ID] = struct{}{}
			all = append(all, sess)
		}
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].LaunchedAt.Equal(all[j].LaunchedAt) {
			return all[i].ID < all[j].ID
		}
		return all[i].LaunchedAt.After(all[j].LaunchedAt)
	})
	return all
}

func (s *Server) buildSessionTriageSummary(ctx context.Context, repoFilter string, statuses []string, since, until time.Time) map[string]any {
	all := s.collectTriagedSessions(ctx, repoFilter, statuses, since, until)

	byReason := map[string]int{}
	byRepo := map[string]int{}
	byProvider := map[string]int{}
	var totalCost float64
	summaries := make([]map[string]any, 0, len(all))

	for _, sess := range all {
		reason := classifySessionKillReason(sess)
		byReason[reason]++
		byRepo[sess.RepoName]++
		byProvider[string(sess.Provider)]++
		totalCost += sess.SpentUSD
		summaries = append(summaries, map[string]any{
			"id":          sess.ID,
			"repo":        sess.RepoName,
			"provider":    sess.Provider,
			"model":       sess.Model,
			"status":      sess.Status,
			"cost_usd":    sess.SpentUSD,
			"turns":       sess.TurnCount,
			"error":       truncateStr(sess.Error, 200),
			"last_output": truncateStr(sess.LastOutput, 200),
			"launched_at": sess.LaunchedAt.Format(time.RFC3339),
			"kill_reason": reason,
		})
	}

	return map[string]any{
		"incident_window":       map[string]string{"since": since.Format(time.RFC3339), "until": until.Format(time.RFC3339)},
		"total_sessions":        len(all),
		"total_cost_wasted_usd": totalCost,
		"by_kill_reason":        byReason,
		"by_repo":               byRepo,
		"by_provider":           byProvider,
		"sessions":              summaries,
	}
}

// ── session_salvage ─────────────────────────────────────────────────────────

func (s *Server) handleSessionSalvage(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	id, errResult := pp.StringErr("id")
	if errResult != nil {
		return errResult, nil
	}
	genPrompt := pp.OptionalBool("generate_prompt", true)
	saveDomain := pp.String("save_to_docs")

	sess := s.findSession(ctx, id)
	if sess == nil {
		return codedError(ErrSessionNotFound, fmt.Sprintf("session %q not found", id)), nil
	}

	// Classify completeness.
	assessment := classifySalvage(sess)

	result := map[string]any{
		"session_id":      sess.ID,
		"repo":            sess.RepoName,
		"provider":        string(sess.Provider),
		"model":           sess.Model,
		"status":          string(sess.Status),
		"turns_completed": sess.TurnCount,
		"cost_spent_usd":  sess.SpentUSD,
		"assessment":      assessment,
		"original_prompt": truncateStr(sess.Prompt, 500),
		"salvaged_output": truncateStr(sess.LastOutput, 2000),
		"error":           sess.Error,
	}

	if genPrompt {
		result["recovery_prompt"] = buildRecoveryPrompt(sess, assessment)
	}

	if saveDomain != "" {
		// Validate domain as a path component — reject traversal.
		if err := validateSafePath(saveDomain); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid save_to_docs domain: %v", err)), nil
		}
		docsPath := filepath.Join(s.docsRoot(), "research", saveDomain)
		if err := os.MkdirAll(docsPath, 0o755); err == nil {
			filename := fmt.Sprintf("salvaged-%s.md", sess.ID[:8])
			content := fmt.Sprintf("# Salvaged Session: %s\n\n**Repo:** %s\n**Provider:** %s\n**Status:** %s\n**Assessment:** %s\n**Cost:** $%.2f\n\n## Original Prompt\n\n%s\n\n## Salvaged Output\n\n%s\n\n## Error\n\n%s\n",
				sess.ID[:8], sess.RepoName, sess.Provider, sess.Status, assessment,
				sess.SpentUSD, sess.Prompt, sess.LastOutput, sess.Error)
			fullPath := filepath.Join(docsPath, filename)
			if err := os.WriteFile(fullPath, []byte(content), 0o644); err == nil {
				result["docs_path"] = fullPath
			}
		}
	}

	return jsonResult(result), nil
}

// ── recovery_plan ───────────────────────────────────────────────────────────

func (s *Server) handleRecoveryPlan(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	sessionIDs := pp.String("session_ids")
	budgetCap := pp.FloatOr("budget_cap_usd", 50.0)
	strategy := pp.String("strategy")
	if strategy == "" {
		strategy = "cost-aware"
	}

	var sessions []*session.Session

	if sessionIDs != "" {
		for _, id := range strings.Split(sessionIDs, ",") {
			id = strings.TrimSpace(id)
			if sess := s.findSession(ctx, id); sess != nil {
				sessions = append(sessions, sess)
			}
		}
	} else {
		since, _ := parseTimeParam(pp.String("since"), time.Hour)
		opts := session.ListOpts{Since: since, Until: time.Now()}
		for _, st := range []session.SessionStatus{session.StatusErrored, session.StatusInterrupted} {
			opts.Status = st
			if s.SessMgr != nil && s.SessMgr.Store() != nil {
				found, _ := s.SessMgr.Store().ListSessions(ctx, opts)
				sessions = append(sessions, found...)
			}
		}
	}

	if len(sessions) == 0 {
		return jsonResult(map[string]any{
			"plan_id":  "rec-empty",
			"strategy": strategy,
			"actions":  []any{},
			"summary":  map[string]int{"retry": 0, "salvage_and_close": 0, "escalate": 0},
		}), nil
	}

	planID := "rec-" + uuid.New().String()[:8]
	var actions []map[string]any
	var totalRetryCost float64
	counts := map[string]int{"retry": 0, "salvage_and_close": 0, "escalate": 0}

	for _, sess := range sessions {
		assessment := classifySalvage(sess)
		isTransient := isTransientError(sess.Error)
		priority := scorePriority(sess)
		estimatedCost := estimateRetryCost(sess)

		var action string
		switch {
		case isTransient && totalRetryCost+estimatedCost <= budgetCap && (strategy != "conservative" || estimatedCost < 5.0):
			action = "retry"
			totalRetryCost += estimatedCost
		case sess.SpentUSD > 5.0 && !isTransient:
			action = "escalate"
		default:
			action = "salvage_and_close"
		}

		entry := map[string]any{
			"action":         action,
			"session_id":     sess.ID,
			"repo":           sess.RepoName,
			"provider":       string(sess.Provider),
			"model":          sess.Model,
			"priority":       priority,
			"assessment":     assessment,
			"cost_spent_usd": sess.SpentUSD,
		}
		if action == "retry" {
			entry["prompt"] = sess.Prompt
			entry["budget_usd"] = estimatedCost
		}
		if action == "salvage_and_close" {
			entry["salvaged_output"] = truncateStr(sess.LastOutput, 500)
			entry["reason"] = classifySessionKillReason(sess)
		}
		if action == "escalate" {
			entry["reason"] = "high_cost_unclear_failure"
		}
		actions = append(actions, entry)
		counts[action]++
	}

	// Sort by priority descending.
	sort.Slice(actions, func(i, j int) bool {
		pi, _ := actions[i]["priority"].(float64)
		pj, _ := actions[j]["priority"].(float64)
		return pi > pj
	})

	return jsonResult(map[string]any{
		"plan_id":                  planID,
		"strategy":                 strategy,
		"budget_cap_usd":           budgetCap,
		"estimated_retry_cost_usd": totalRetryCost,
		"actions":                  actions,
		"summary":                  counts,
	}), nil
}

// ── recovery_execute ────────────────────────────────────────────────────────

func (s *Server) handleRecoveryExecute(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	planJSON, errResult := pp.StringErr("plan_json")
	if errResult != nil {
		return errResult, nil
	}
	budgetCap := pp.FloatOr("budget_cap_usd", 50.0)
	concurrency := int(pp.FloatOr("concurrency", 5))
	modelOverride := pp.String("model_override")

	var actions []map[string]any
	if err := json.Unmarshal([]byte(planJSON), &actions); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid plan_json: %v", err)), nil
	}

	// Filter to retry actions only.
	var retries []map[string]any
	var totalBudget float64
	for _, a := range actions {
		if a["action"] == "retry" {
			budget, _ := a["budget_usd"].(float64)
			totalBudget += budget
			retries = append(retries, a)
		}
	}

	if totalBudget > budgetCap {
		return codedError(ErrInvalidParams, fmt.Sprintf(
			"estimated retry cost $%.2f exceeds budget cap $%.2f", totalBudget, budgetCap)), nil
	}

	if s.SessMgr == nil {
		return codedError(ErrProviderUnavailable, "session manager not available"), nil
	}

	sweepID := "recovery-" + uuid.New().String()[:8]
	sem := make(chan struct{}, concurrency)
	var launched, skipped int

	for _, retry := range retries {
		repoName, _ := retry["repo"].(string)
		prompt, _ := retry["prompt"].(string)
		provider, _ := retry["provider"].(string)
		model, _ := retry["model"].(string)
		budget, _ := retry["budget_usd"].(float64)

		if prompt == "" || repoName == "" {
			skipped++
			continue
		}

		if modelOverride != "" {
			model = modelOverride
		}

		repo := s.findRepo(repoName)
		if repo == nil {
			skipped++
			continue
		}

		sem <- struct{}{}
		go func() {
			defer func() { <-sem }()
			s.SessMgr.Launch(ctx, session.LaunchOptions{
				Provider:     session.Provider(provider),
				Model:        model,
				RepoPath:     repo.Path,
				Prompt:       prompt,
				MaxBudgetUSD: budget,
			})
		}()
		launched++
	}

	// Drain semaphore.
	for i := 0; i < concurrency; i++ {
		sem <- struct{}{}
	}

	return jsonResult(map[string]any{
		"sweep_id":         sweepID,
		"launched":         launched,
		"skipped":          skipped,
		"total_budget_usd": totalBudget,
	}), nil
}

// ── incident_report ─────────────────────────────────────────────────────────

func (s *Server) handleIncidentReport(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	title, errResult := pp.StringErr("title")
	if errResult != nil {
		return errResult, nil
	}
	if err := validateSafePath(title); err != nil {
		return codedError(ErrInvalidParams, err.Error()), nil
	}
	cause := pp.String("cause")
	recoverySweepID := pp.String("recovery_sweep_id")
	sessionIDs := pp.String("session_ids")

	var sessions []*session.Session
	if sessionIDs != "" {
		for _, id := range strings.Split(sessionIDs, ",") {
			if sess := s.findSession(ctx, strings.TrimSpace(id)); sess != nil {
				sessions = append(sessions, sess)
			}
		}
	} else {
		since, _ := parseTimeParam(pp.String("since"), time.Hour)
		for _, st := range []session.SessionStatus{session.StatusErrored, session.StatusInterrupted} {
			if s.SessMgr != nil && s.SessMgr.Store() != nil {
				found, _ := s.SessMgr.Store().ListSessions(ctx, session.ListOpts{
					Status: st, Since: since, Until: time.Now(),
				})
				sessions = append(sessions, found...)
			}
		}
	}

	// Build markdown.
	var md strings.Builder
	md.WriteString(fmt.Sprintf("# Incident: %s\n\n", title))
	md.WriteString(fmt.Sprintf("**Date:** %s\n", time.Now().Format("2006-01-02 15:04 MST")))
	if cause != "" {
		md.WriteString(fmt.Sprintf("**Root Cause:** %s\n", cause))
	}
	md.WriteString(fmt.Sprintf("**Sessions Affected:** %d\n\n", len(sessions)))

	var totalCost float64
	md.WriteString("## Affected Sessions\n\n")
	md.WriteString("| ID | Repo | Provider | Status | Cost | Turns | Error |\n")
	md.WriteString("|---|---|---|---|---|---|---|\n")
	for _, sess := range sessions {
		totalCost += sess.SpentUSD
		md.WriteString(fmt.Sprintf("| %s | %s | %s | %s | $%.2f | %d | %s |\n",
			sess.ID[:8], sess.RepoName, sess.Provider, sess.Status,
			sess.SpentUSD, sess.TurnCount, truncateStr(sess.Error, 50)))
	}
	md.WriteString(fmt.Sprintf("\n**Total Cost Impacted:** $%.2f\n\n", totalCost))

	if recoverySweepID != "" {
		md.WriteString(fmt.Sprintf("## Recovery\n\nRecovery sweep launched: `%s`\n\n", recoverySweepID))
	}

	md.WriteString("## Lessons Learned\n\n")
	killReasons := map[string]int{}
	for _, sess := range sessions {
		killReasons[classifySessionKillReason(sess)]++
	}
	for reason, count := range killReasons {
		md.WriteString(fmt.Sprintf("- **%s** (%d sessions): ", reason, count))
		switch reason {
		case "oom":
			md.WriteString("Consider memory limits or model downgrades for large sessions\n")
		case "timeout":
			md.WriteString("Review timeout settings; increase for long-running tasks\n")
		case "signal_killed":
			md.WriteString("External kill (e.g., compositor crash) — consider session checkpointing\n")
		default:
			md.WriteString("Investigate root cause\n")
		}
	}

	// Write to docs/research/incidents/.
	incidentDir := filepath.Join(s.docsRoot(), "research", "incidents")
	if err := os.MkdirAll(incidentDir, 0o755); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("create incidents dir: %v", err)), nil
	}
	filename := title + ".md"
	fullPath := filepath.Join(incidentDir, filename)
	if err := os.WriteFile(fullPath, []byte(md.String()), 0o644); err != nil {
		return codedError(ErrInternal, fmt.Sprintf("write incident: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"path":              fullPath,
		"sessions_affected": len(sessions),
		"total_cost_usd":    totalCost,
	}), nil
}

// ── session_discover ────────────────────────────────────────────────────────

func (s *Server) handleSessionDiscover(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParserFromRequest(req)
	scanPath := pp.String("scan_path")
	if scanPath == "" {
		scanPath = s.ScanPath
	} else {
		// Validate user-supplied scan_path — reject traversal and escapes.
		if err := ValidatePath(scanPath, s.ScanPath); err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid scan_path: %v", err)), nil
		}
	}
	includeClaude := pp.OptionalBool("include_claude_projects", true)
	checkProcs := pp.OptionalBool("check_processes", true)

	type discovered struct {
		SessionID    string `json:"session_id"`
		Repo         string `json:"repo"`
		Source       string `json:"source"`
		InStore      bool   `json:"in_store"`
		ProcessAlive bool   `json:"process_alive"`
		Status       string `json:"status,omitempty"`
	}

	var results []discovered
	reposScanned := 0

	// Scan .ralph/sessions/ in each repo.
	entries, _ := os.ReadDir(scanPath)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		reposScanned++
		sessDir := filepath.Join(scanPath, entry.Name(), ".ralph", "sessions")
		files, err := os.ReadDir(sessDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if !strings.HasSuffix(f.Name(), ".json") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(sessDir, f.Name()))
			if err != nil {
				continue
			}
			var meta struct {
				ID     string `json:"id"`
				Status string `json:"status"`
				PID    int    `json:"pid"`
			}
			if json.Unmarshal(data, &meta) != nil || meta.ID == "" {
				continue
			}

			inStore := false
			if s.SessMgr != nil {
				if _, ok := s.SessMgr.Get(meta.ID); ok {
					inStore = true
				}
			}

			alive := false
			if checkProcs && meta.PID > 0 {
				alive = isProcessAlive(meta.PID)
			}

			results = append(results, discovered{
				SessionID:    meta.ID,
				Repo:         entry.Name(),
				Source:       filepath.Join(".ralph", "sessions", f.Name()),
				InStore:      inStore,
				ProcessAlive: alive,
				Status:       meta.Status,
			})
		}
	}

	// Scan ~/.claude/projects/ for session metadata.
	if includeClaude {
		homeDir, _ := os.UserHomeDir()
		claudeProjects := filepath.Join(homeDir, ".claude", "projects")
		projEntries, _ := os.ReadDir(claudeProjects)
		for _, pe := range projEntries {
			if !pe.IsDir() {
				continue
			}
			// Look for session JSON files.
			projDir := filepath.Join(claudeProjects, pe.Name())
			files, _ := filepath.Glob(filepath.Join(projDir, "*.json"))
			for _, f := range files {
				base := filepath.Base(f)
				// Skip non-session files.
				if base == "MEMORY.md" || strings.HasPrefix(base, "memory") {
					continue
				}
				// Extract repo name from project directory name.
				repoName := extractRepoFromProjectDir(pe.Name())
				results = append(results, discovered{
					SessionID:    strings.TrimSuffix(base, ".json"),
					Repo:         repoName,
					Source:       filepath.Join("~/.claude/projects", pe.Name(), base),
					InStore:      false,
					ProcessAlive: false,
					Status:       "unknown",
				})
			}
		}
	}

	notInStore := 0
	stillRunning := 0
	for _, r := range results {
		if !r.InStore {
			notInStore++
		}
		if r.ProcessAlive {
			stillRunning++
		}
	}

	return jsonResult(map[string]any{
		"discovered":    results,
		"total":         len(results),
		"not_in_store":  notInStore,
		"still_running": stillRunning,
		"repos_scanned": reposScanned,
	}), nil
}

// ── helpers ─────────────────────────────────────────────────────────────────

// parseTimeParam parses a time string as either RFC3339 or a relative duration
// (e.g., "2h", "30m", "1d"). If empty, returns now minus defaultDuration.
func parseTimeParam(s string, defaultDuration time.Duration) (time.Time, error) {
	if s == "" {
		if defaultDuration == 0 {
			return time.Time{}, nil
		}
		return time.Now().Add(-defaultDuration), nil
	}
	s = strings.TrimSpace(s)
	// Try RFC3339 first.
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t, nil
	}
	// Handle "Nd" for days.
	if strings.HasSuffix(s, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(s, "d"))
		if err == nil && days > 0 {
			return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}
	// Standard Go duration.
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid time %q: use RFC3339 or relative (e.g., '2h', '30m', '1d')", s)
	}
	return time.Now().Add(-d), nil
}

// findSession looks up a session from both the live manager and the store.
func (s *Server) findSession(ctx context.Context, id string) *session.Session {
	if s.SessMgr != nil {
		if sess, ok := s.SessMgr.Get(id); ok {
			return sess
		}
		if s.SessMgr.Store() != nil {
			sess, _ := s.SessMgr.Store().GetSession(ctx, id)
			return sess
		}
	}
	return nil
}

// classifySessionKillReason returns a human-readable kill reason from session state.
func classifySessionKillReason(sess *session.Session) string {
	if sess.Error == "" && sess.ExitReason == "" {
		return "unknown"
	}
	errStr := strings.ToLower(sess.Error + " " + sess.ExitReason)
	switch {
	case strings.Contains(errStr, "oom") || strings.Contains(errStr, "out of memory"):
		return "oom"
	case strings.Contains(errStr, "timeout") || strings.Contains(errStr, "deadline"):
		return "timeout"
	case strings.Contains(errStr, "signal: killed") || strings.Contains(errStr, "sigkill"):
		return "signal_killed"
	case strings.Contains(errStr, "budget") || strings.Contains(errStr, "spend limit"):
		return "budget_exceeded"
	case strings.Contains(errStr, "rate limit") || strings.Contains(errStr, "429"):
		return "rate_limited"
	case strings.Contains(errStr, "connection") || strings.Contains(errStr, "network"):
		return "network_error"
	default:
		return "other"
	}
}

// classifySalvage determines how much useful work a killed session produced.
func classifySalvage(sess *session.Session) string {
	if sess.LastOutput == "" && sess.TurnCount == 0 {
		return "no_useful_output"
	}
	if sess.TurnCount > 0 && sess.MaxTurns > 0 && float64(sess.TurnCount)/float64(sess.MaxTurns) > 0.8 {
		return "nearly_complete"
	}
	lower := strings.ToLower(sess.LastOutput)
	if strings.Contains(lower, "commit") || strings.Contains(lower, "created") ||
		strings.Contains(lower, "fixed") || strings.Contains(lower, "pass") {
		return "partial_completion"
	}
	if sess.TurnCount > 0 {
		return "partial_completion"
	}
	return "no_useful_output"
}

// buildRecoveryPrompt generates a continuation prompt from a killed session.
func buildRecoveryPrompt(sess *session.Session, assessment string) string {
	var b strings.Builder
	b.WriteString("<context>\n")
	fmt.Fprintf(&b, "This is a recovery session. The previous session (%s) in repo %q was killed (%s).\n",
		sess.ID[:8], sess.RepoName, sess.Status)
	fmt.Fprintf(&b, "It completed %d turns and spent $%.2f before termination.\n", sess.TurnCount, sess.SpentUSD)
	if assessment == "partial_completion" || assessment == "nearly_complete" {
		b.WriteString("\nLast output before kill:\n```\n")
		b.WriteString(truncateStr(sess.LastOutput, 1000))
		b.WriteString("\n```\n")
	}
	b.WriteString("</context>\n\n")

	b.WriteString("<instructions>\n")
	b.WriteString("Continue the following task from where the previous session left off.\n")
	b.WriteString("Do not repeat work that was already completed.\n\n")
	b.WriteString("Original task:\n")
	b.WriteString(sess.Prompt)
	b.WriteString("\n</instructions>\n\n")

	b.WriteString("<verification>\n")
	b.WriteString("- go vet ./...\n")
	b.WriteString("- go test ./... -count=1\n")
	b.WriteString("</verification>")
	return b.String()
}

// scorePriority scores a session for recovery priority (0.0 to 1.0).
func scorePriority(sess *session.Session) float64 {
	score := 0.5
	// Higher cost = higher priority to recover investment.
	if sess.SpentUSD > 1.0 {
		score += 0.2
	}
	// More turns completed = more investment to preserve.
	if sess.TurnCount > 5 {
		score += 0.15
	}
	// Transient errors are more likely to succeed on retry.
	if isTransientError(sess.Error) {
		score += 0.1
	}
	if score > 1.0 {
		score = 1.0
	}
	return score
}

// estimateRetryCost estimates the cost of retrying a session.
func estimateRetryCost(sess *session.Session) float64 {
	if sess.BudgetUSD > 0 {
		return sess.BudgetUSD
	}
	// Default: roughly what it cost before, with a floor.
	est := sess.SpentUSD * 1.2
	if est < 0.50 {
		est = 0.50
	}
	if est > 10.0 {
		est = 10.0
	}
	return est
}

// isTransientError checks if an error message matches known transient patterns.
func isTransientError(errMsg string) bool {
	lower := strings.ToLower(errMsg)
	for _, p := range session.TransientErrorPatterns {
		if strings.Contains(lower, strings.ToLower(p)) {
			return true
		}
	}
	return false
}

// truncateStr truncates a string to maxLen, appending "..." if needed.
func truncateStr(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 4 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// isProcessAlive checks if a PID is still running.
func isProcessAlive(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// On Unix, FindProcess always succeeds; signal 0 checks existence.
	err = proc.Signal(os.Signal(nil))
	return err == nil
}

// extractRepoFromProjectDir extracts a repo name from a Claude Code project dir name.
// Format: "-home-hg-hairglasses-studio-reponame" → "reponame"
func extractRepoFromProjectDir(dirName string) string {
	parts := strings.Split(dirName, "-")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return dirName
}
