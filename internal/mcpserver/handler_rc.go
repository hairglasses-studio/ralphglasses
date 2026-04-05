package mcpserver

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/session"
)

// --- Remote Control (RC) helpers ---

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return "<1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func formatCost(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}

// resolveTarget finds a session by ID or by repo name (most recent running session).
// Returns a codedError with the appropriate error code on failure.
func (s *Server) resolveTarget(target string) (*session.Session, *mcp.CallToolResult) {
	if target == "" {
		return nil, codedError(ErrInvalidParams, "target required")
	}
	// Try as session ID first
	if sess, ok := s.SessMgr.Get(target); ok {
		return sess, nil
	}
	// Try as repo name
	sessions := s.SessMgr.FindByRepo(target)
	if len(sessions) == 0 {
		return nil, codedError(ErrSessionNotFound, fmt.Sprintf("no session found for %q", target))
	}
	// Prefer running session, otherwise most recent
	var best *session.Session
	for _, sess := range sessions {
		sess.Lock()
		st := sess.Status
		la := sess.LastActivity
		sess.Unlock()
		if st == session.StatusRunning || st == session.StatusLaunching {
			if best == nil {
				best = sess
			} else {
				best.Lock()
				bestLA := best.LastActivity
				best.Unlock()
				if la.After(bestLA) {
					best = sess
				}
			}
		}
	}
	if best == nil {
		// No running session, use most recent
		best = sessions[0]
		for _, sess := range sessions[1:] {
			sess.Lock()
			la := sess.LastActivity
			sess.Unlock()
			best.Lock()
			bestLA := best.LastActivity
			best.Unlock()
			if la.After(bestLA) {
				best = sess
			}
		}
	}
	return best, nil
}

// mostActiveSession returns the most recently active session (prefers running).
func (s *Server) mostActiveSession() *session.Session {
	all := s.SessMgr.List("")
	if len(all) == 0 {
		return nil
	}
	var best *session.Session
	var bestRunning bool
	var bestTime time.Time

	for _, sess := range all {
		sess.Lock()
		st := sess.Status
		la := sess.LastActivity
		sess.Unlock()

		isRunning := st == session.StatusRunning || st == session.StatusLaunching
		if best == nil ||
			(isRunning && !bestRunning) ||
			(isRunning == bestRunning && la.After(bestTime)) {
			best = sess
			bestRunning = isRunning
			bestTime = la
		}
	}
	return best
}

func summarizeEvent(e events.Event) string {
	switch e.Type {
	case events.SessionStarted:
		return fmt.Sprintf("[start] %s/%s session %s", e.RepoName, e.Provider, shortID(e.SessionID))
	case events.SessionEnded:
		return fmt.Sprintf("[end] %s session %s", e.RepoName, shortID(e.SessionID))
	case events.SessionStopped:
		return fmt.Sprintf("[stop] %s session %s", e.RepoName, shortID(e.SessionID))
	case events.CostUpdate:
		cost := ""
		if v, ok := e.Data["cost_usd"]; ok {
			if f, ok := v.(float64); ok {
				cost = formatCost(f)
			}
		}
		return fmt.Sprintf("[cost] %s %s", e.RepoName, cost)
	case events.BudgetExceeded:
		return fmt.Sprintf("[BUDGET] %s exceeded budget", e.RepoName)
	case events.LoopStarted:
		return fmt.Sprintf("[loop] %s started", e.RepoName)
	case events.LoopStopped:
		return fmt.Sprintf("[loop] %s stopped", e.RepoName)
	case events.TeamCreated:
		return fmt.Sprintf("[team] %s created", e.RepoName)
	case events.ToolCalled:
		return fmt.Sprintf("[tool.called] %s", summarizeToolCalled(e.Data))
	case events.ScanComplete:
		return fmt.Sprintf("[scan.complete] %s", summarizeScanComplete(e.Data))
	case events.LoopIterated:
		return fmt.Sprintf("[loop.iterated] %s", summarizeLoopIterated(e.Data))
	case events.LoopRegression:
		return fmt.Sprintf("[loop.regression] %s", e.RepoName)
	case events.PromptEnhanced:
		return fmt.Sprintf("[prompt.enhanced] %s", e.RepoName)
	case events.SessionError:
		msg := dataString(e.Data, "error")
		if msg != "" {
			return fmt.Sprintf("[session.error] %s: %s", e.RepoName, msg)
		}
		return fmt.Sprintf("[session.error] %s", e.RepoName)
	default:
		return fmt.Sprintf("[%s] %s", e.Type, summarizeDefault(e))
	}
}

// dataString extracts a string value from a Data map, returning "" if missing or wrong type.
func dataString(data map[string]any, key string) string {
	if data == nil {
		return ""
	}
	v, ok := data[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	return s
}

// dataFloat extracts a float64 value from a Data map, returning 0 and false if missing or wrong type.
func dataFloat(data map[string]any, key string) (float64, bool) {
	if data == nil {
		return 0, false
	}
	v, ok := data[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

func summarizeToolCalled(data map[string]any) string {
	name := dataString(data, "tool")
	if name == "" {
		name = dataString(data, "name")
	}
	latency := dataString(data, "latency")
	if latency == "" {
		if ms, ok := dataFloat(data, "latency_ms"); ok {
			latency = fmt.Sprintf("%.0fms", ms)
		}
	}
	if name != "" && latency != "" {
		return fmt.Sprintf("%s (%s)", name, latency)
	}
	if name != "" {
		return name
	}
	return "unknown"
}

func summarizeScanComplete(data map[string]any) string {
	if count, ok := dataFloat(data, "repo_count"); ok {
		return fmt.Sprintf("found %d repos", int(count))
	}
	if count, ok := dataFloat(data, "count"); ok {
		return fmt.Sprintf("found %d repos", int(count))
	}
	return "scan finished"
}

func summarizeLoopIterated(data map[string]any) string {
	step := ""
	if n, ok := dataFloat(data, "step"); ok {
		step = fmt.Sprintf("step %d", int(n))
	}
	status := dataString(data, "status")
	if step != "" && status != "" {
		return fmt.Sprintf("%s: %s", step, status)
	}
	if step != "" {
		return step
	}
	if status != "" {
		return status
	}
	return "iteration"
}

func summarizeDefault(e events.Event) string {
	if e.RepoName != "" {
		if len(e.Data) > 0 {
			var parts []string
			for k, v := range e.Data {
				parts = append(parts, fmt.Sprintf("%s=%v", k, v))
				if len(parts) >= 3 {
					break
				}
			}
			return fmt.Sprintf("%s %s", e.RepoName, strings.Join(parts, " "))
		}
		return e.RepoName
	}
	if len(e.Data) > 0 {
		var parts []string
		for k, v := range e.Data {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
			if len(parts) >= 3 {
				break
			}
		}
		return strings.Join(parts, " ")
	}
	return ""
}

// --- Remote Control (RC) handlers ---

func (s *Server) handleRCStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	all := s.SessMgr.List("")

	var running, recent []*session.Session
	var totalCost float64
	var alerts []string
	now := time.Now()

	for _, sess := range all {
		sess.Lock()
		st := sess.Status
		cost := sess.SpentUSD
		la := sess.LastActivity
		repoName := sess.RepoName
		idleMin := now.Sub(la).Minutes()
		sess.Unlock()

		totalCost += cost

		if st == session.StatusRunning || st == session.StatusLaunching {
			running = append(running, sess)
			if idleMin > 8 {
				alerts = append(alerts, fmt.Sprintf("Session %s on %s idle %s", shortID(sess.ID), repoName, formatDuration(now.Sub(la))))
			}
		} else if now.Sub(la) < 30*time.Minute {
			recent = append(recent, sess)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%d running | %s total", len(running), formatCost(totalCost))
	if len(alerts) > 0 {
		fmt.Fprintf(&b, " | %d alert(s)", len(alerts))
	}
	b.WriteString("\n")

	for _, sess := range running {
		sess.Lock()
		repoName := sess.RepoName
		provider := sess.Provider
		cost := sess.SpentUSD
		turns := sess.TurnCount
		idle := now.Sub(sess.LastActivity)
		sess.Unlock()
		fmt.Fprintf(&b, "\n[running] %s/%s  %s  %dt  %s idle",
			repoName, provider, formatCost(cost), turns, formatDuration(idle))
	}

	for _, sess := range recent {
		sess.Lock()
		repoName := sess.RepoName
		provider := sess.Provider
		cost := sess.SpentUSD
		turns := sess.TurnCount
		st := sess.Status
		sess.Unlock()
		fmt.Fprintf(&b, "\n[%s] %s/%s  %s  %dt", st, repoName, provider, formatCost(cost), turns)
	}

	if len(running) == 0 && len(recent) == 0 {
		b.WriteString("\nNo active or recent sessions.")
	}

	if len(alerts) > 0 {
		b.WriteString("\n\nAlerts:")
		for _, a := range alerts {
			fmt.Fprintf(&b, "\n  %s", a)
		}
	}

	return textResult(b.String()), nil
}

func (s *Server) handleRCSend(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParser(argsMap(req))

	name := pp.StringOpt("repo", "")
	if name == "" {
		return codedError(ErrInvalidParams, "repo name required"), nil
	}
	if err := ValidateRepoName(name); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid repo name: %v", err)), nil
	}
	prompt := SanitizeString(pp.StringOpt("prompt", ""))
	if prompt == "" {
		return codedError(ErrInvalidParams, "prompt required"), nil
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

	provider := session.Provider(pp.StringOpt("provider", ""))
	if provider == "" {
		provider = session.DefaultPrimaryProvider()
	}
	if err := session.ValidateProvider(provider); err != nil {
		return codedError(ErrInvalidParams, fmt.Sprintf("invalid provider: %v", err)), nil
	}

	// Check for resume
	if pp.StringOpt("resume", "") == "true" {
		existing := s.SessMgr.FindByRepo(name)
		for _, sess := range existing {
			sess.Lock()
			psid := sess.ProviderSessionID
			sess.Unlock()
			if psid != "" {
				resumed, err := s.SessMgr.Resume(ctx, r.Path, provider, psid, prompt)
				if err != nil {
					return codedError(ErrInternal, fmt.Sprintf("resume failed: %v", err)), nil
				}
				return textResult(fmt.Sprintf("Resumed %s session on %s (id: %s)", provider, name, shortID(resumed.ID))), nil
			}
		}
		// No resumable session found, fall through to fresh launch
	}

	// Stop existing running sessions on this repo
	existing := s.SessMgr.FindByRepo(name)
	for _, sess := range existing {
		sess.Lock()
		st := sess.Status
		sid := sess.ID
		sess.Unlock()
		if st == session.StatusRunning || st == session.StatusLaunching {
			if err := s.SessMgr.Stop(sid); err != nil {
				slog.Warn("failed to stop session during RC act", "session", sid, "error", err)
			}
		}
	}

	budget := getNumberArg(req, "budget_usd", 5.0)
	opts := session.LaunchOptions{
		Provider:     provider,
		RepoPath:     r.Path,
		Prompt:       prompt,
		Model:        pp.StringOpt("model", ""),
		MaxBudgetUSD: budget,
	}

	// Inject journal context
	journal, _ := session.ReadRecentJournal(r.Path, 5)
	if len(journal) > 0 {
		journalCtx := session.SynthesizeContext(journal)
		if journalCtx != "" {
			opts.Prompt = journalCtx + "\n\n---\n\n" + opts.Prompt
		}
	}

	sess, err := s.SessMgr.Launch(context.Background(), opts)
	if err != nil {
		return codedError(ErrInternal, fmt.Sprintf("launch failed: %v", err)), nil
	}

	return jsonResult(map[string]any{
		"message":            fmt.Sprintf("Launched %s session on %s (budget: %s, id: %s)", provider, name, formatCost(budget), shortID(sess.ID)),
		"session_id":         sess.ID,
		"provider":           string(provider),
		"repo":               name,
		"applied_budget_usd": budget,
	}), nil
}

func (s *Server) handleRCRead(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParser(argsMap(req))
	id := pp.StringOpt("id", "")
	var sess *session.Session

	if id != "" {
		var ok bool
		sess, ok = s.SessMgr.Get(id)
		if !ok {
			return codedError(ErrSessionNotFound, fmt.Sprintf("session not found: %s", id)), nil
		}
	} else {
		sess = s.mostActiveSession()
		if sess == nil {
			return emptyResult("rc_messages"), nil
		}
	}

	lines := pp.IntOpt("lines", 10)
	if lines > 30 {
		lines = 30
	}
	if lines < 1 {
		lines = 10
	}

	sess.Lock()
	history := make([]string, len(sess.OutputHistory))
	copy(history, sess.OutputHistory)
	totalCount := sess.TotalOutputCount
	status := sess.Status
	repoName := sess.RepoName
	provider := sess.Provider
	cost := sess.SpentUSD
	turns := sess.TurnCount
	lastActivity := sess.LastActivity
	sess.Unlock()

	cursorStr := pp.StringOpt("cursor", "")
	var output []string

	if cursorStr != "" {
		cursor, err := strconv.Atoi(cursorStr)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
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
		if len(history) > lines {
			output = history[len(history)-lines:]
		} else {
			output = history
		}
	}

	idle := time.Since(lastActivity)
	var b strings.Builder
	fmt.Fprintf(&b, "[%s] %s/%s | %s | %dt | %s idle\n",
		status, repoName, provider, formatCost(cost), turns, formatDuration(idle))

	if len(output) > 0 {
		b.WriteString("\n")
		b.WriteString(strings.Join(output, "\n"))
	} else if cursorStr != "" {
		b.WriteString("\n(no new output)")
	}

	fmt.Fprintf(&b, "\n\ncursor:%d", totalCount)

	return textResult(b.String()), nil
}

func (s *Server) handleEventPoll(_ context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	if s.EventBus == nil {
		return codedError(ErrNotRunning, "event bus not initialized"), nil
	}

	pp := NewParamParser(argsMap(req))
	cursorStr := pp.StringOpt("cursor", "")
	cursor := 0
	if cursorStr != "" {
		var err error
		cursor, err = strconv.Atoi(cursorStr)
		if err != nil {
			return codedError(ErrInvalidParams, fmt.Sprintf("invalid cursor: %s", cursorStr)), nil
		}
	}

	limit := pp.IntOpt("limit", 20)
	if limit > 50 {
		limit = 50
	}
	if limit < 1 {
		limit = 20
	}

	typeFilter := events.EventType(pp.StringOpt("type", ""))

	evts, newCursor := s.EventBus.HistoryAfterCursor(cursor, limit*2) // fetch extra for filtering

	// Apply type filter
	var filtered []events.Event
	for _, e := range evts {
		if typeFilter != "" && e.Type != typeFilter {
			continue
		}
		filtered = append(filtered, e)
	}
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	type compactEvent struct {
		Type      string `json:"type"`
		Time      string `json:"time"`
		Repo      string `json:"repo,omitempty"`
		SessionID string `json:"session_id,omitempty"`
		Summary   string `json:"summary"`
	}

	out := make([]compactEvent, len(filtered))
	for i, e := range filtered {
		out[i] = compactEvent{
			Type:      string(e.Type),
			Time:      e.Timestamp.Format("15:04:05"),
			Repo:      e.RepoName,
			SessionID: shortID(e.SessionID),
			Summary:   summarizeEvent(e),
		}
	}

	return jsonResult(map[string]any{
		"events":   out,
		"count":    len(out),
		"cursor":   strconv.Itoa(newCursor),
		"has_more": len(evts) > limit,
	}), nil
}

func (s *Server) handleRCAct(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	pp := NewParamParser(argsMap(req))
	action, errResult := pp.StringErr("action")
	if errResult != nil {
		return errResult, nil
	}
	target := pp.StringOpt("target", "")

	switch action {
	case "stop":
		sess, errRes := s.resolveTarget(target)
		if errRes != nil {
			return errRes, nil
		}
		sess.Lock()
		sid := sess.ID
		repoName := sess.RepoName
		cost := sess.SpentUSD
		turns := sess.TurnCount
		sess.Unlock()

		if err := s.SessMgr.Stop(sid); err != nil {
			return codedError(ErrInternal, fmt.Sprintf("stop failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Stopped session %s on %s (%s, %dt)",
			shortID(sid), repoName, formatCost(cost), turns)), nil

	case "stop_all":
		all := s.SessMgr.List("")
		count := 0
		for _, sess := range all {
			sess.Lock()
			st := sess.Status
			sess.Unlock()
			if st == session.StatusRunning || st == session.StatusLaunching {
				count++
			}
		}
		s.SessMgr.StopAll()
		return textResult(fmt.Sprintf("Stopped %d session(s)", count)), nil

	case "pause":
		if target == "" {
			return codedError(ErrInvalidParams, "target required for pause"), nil
		}
		if s.reposNil() {
			if err := s.scan(); err != nil {
				return codedError(ErrScanFailed, fmt.Sprintf("scan failed: %v", err)), nil
			}
		}
		r := s.findRepo(target)
		if r == nil {
			return codedError(ErrRepoNotFound, fmt.Sprintf("repo not found: %s", target)), nil
		}
		nowPaused, err := s.ProcMgr.TogglePause(r.Path)
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("pause toggle failed: %v", err)), nil
		}
		if nowPaused {
			return textResult(fmt.Sprintf("Paused loop on %s", target)), nil
		}
		return textResult(fmt.Sprintf("Resumed loop on %s", target)), nil

	case "resume":
		if target == "" {
			return codedError(ErrInvalidParams, "target required for resume"), nil
		}
		sess, errRes := s.resolveTarget(target)
		if errRes != nil {
			return errRes, nil
		}
		sess.Lock()
		psid := sess.ProviderSessionID
		repoPath := sess.RepoPath
		provider := sess.Provider
		sess.Unlock()
		if psid == "" {
			return codedError(ErrInvalidParams, "session has no provider session ID to resume"), nil
		}
		newSess, err := s.SessMgr.Resume(ctx, repoPath, provider, psid, "")
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("resume failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Resumed session %s", shortID(newSess.ID))), nil

	case "retry":
		sess, errRes := s.resolveTarget(target)
		if errRes != nil {
			return errRes, nil
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
		newSess, err := s.SessMgr.Launch(context.Background(), opts)
		if err != nil {
			return codedError(ErrInternal, fmt.Sprintf("retry failed: %v", err)), nil
		}
		return textResult(fmt.Sprintf("Retried → new session %s", shortID(newSess.ID))), nil

	default:
		return codedError(ErrInvalidParams, fmt.Sprintf("unknown action: %s (expected: stop, stop_all, pause, resume, retry)", action)), nil
	}
}
