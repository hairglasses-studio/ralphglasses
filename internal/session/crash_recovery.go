// crash_recovery.go — Detects crashed Claude Code sessions and orchestrates fleet-wide recovery.
//
// After a system crash, many parallel Claude Code sessions die with unsaved
// progress. This module reads ~/.claude/ state files directly (no MCP round-trip)
// to discover dead sessions, assess their state against git, and build a
// prioritized recovery plan that can be executed via Manager.Resume().
package session

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

// ---------------------------------------------------------------------------
// Claude Code state file types
// ---------------------------------------------------------------------------

// claudeSessionFile represents a session entry from ~/.claude/sessions/*.json.
type claudeSessionFile struct {
	PID        int    `json:"pid"`
	SessionID  string `json:"sessionId"`
	CWD        string `json:"cwd"`
	StartedAt  int64  `json:"startedAt"` // Unix ms
	Kind       string `json:"kind"`
	Entrypoint string `json:"entrypoint"`
	Name       string `json:"name,omitempty"`
}

// claudeTaskFile represents a task from ~/.claude/tasks/<session-uuid>/<id>.json.
type claudeTaskFile struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	Status  string `json:"status"` // pending, in_progress, completed
}

// ---------------------------------------------------------------------------
// Recovery plan types
// ---------------------------------------------------------------------------

// CrashRecoveryPlan is the output of crash detection and analysis.
type CrashRecoveryPlan struct {
	DetectedAt       time.Time            `json:"detected_at"`
	Severity         string               `json:"severity"` // none, minor, major, catastrophic
	SessionsToResume []RecoverableSession `json:"sessions_to_resume"`
	ReposToVerify    []string             `json:"repos_to_verify"`
	TotalSessions    int                  `json:"total_sessions"`
	AliveCount       int                  `json:"alive_count"`
	DeadCount        int                  `json:"dead_count"`
	RecoveryOpID     string               `json:"recovery_op_id,omitempty"`
}

// ---------------------------------------------------------------------------
// RecoveryBudgetEnvelope
// ---------------------------------------------------------------------------

// RecoveryBudgetEnvelope constrains spending on crash recovery operations.
type RecoveryBudgetEnvelope struct {
	TotalBudgetUSD float64
	SpentUSD       float64
	PerSessionCap  float64
	mu             sync.Mutex
}

// NewRecoveryBudgetEnvelope creates a recovery budget with the given limits.
func NewRecoveryBudgetEnvelope(totalBudget, perSessionCap float64) *RecoveryBudgetEnvelope {
	return &RecoveryBudgetEnvelope{
		TotalBudgetUSD: totalBudget,
		PerSessionCap:  perSessionCap,
	}
}

// CanSpend returns true if the remaining budget can accommodate the cost.
func (rb *RecoveryBudgetEnvelope) CanSpend(cost float64) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.TotalBudgetUSD-rb.SpentUSD >= cost
}

// CanSpendSession returns true if per-session cap and total budget allow the cost.
func (rb *RecoveryBudgetEnvelope) CanSpendSession(cost float64) bool {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	if rb.PerSessionCap > 0 && cost > rb.PerSessionCap {
		return false
	}
	return rb.TotalBudgetUSD-rb.SpentUSD >= cost
}

// RecordSpend adds cost to the spent total.
func (rb *RecoveryBudgetEnvelope) RecordSpend(amount float64) {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	rb.SpentUSD += amount
}

// Remaining returns remaining budget.
func (rb *RecoveryBudgetEnvelope) Remaining() float64 {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return rb.TotalBudgetUSD - rb.SpentUSD
}

// ---------------------------------------------------------------------------
// CrashRecoveryPolicy
// ---------------------------------------------------------------------------

// CrashRecoveryPolicy controls when and how crash recovery is automatically executed.
type CrashRecoveryPolicy struct {
	Enabled               bool            `json:"enabled"`
	AutoExecuteOnSeverity map[string]bool `json:"auto_execute_on_severity"`
	MaxAutoRecoveryCost   float64         `json:"max_auto_recovery_cost"`
	PerSessionBudget      float64         `json:"per_session_budget"`
	EscalationThreshold   int             `json:"escalation_threshold"`
	MaxConcurrent         int             `json:"max_concurrent"`
	CooldownAfterRecovery time.Duration   `json:"cooldown_after_recovery"`
}

// DefaultCrashRecoveryPolicy returns a conservative default policy (opt-in).
func DefaultCrashRecoveryPolicy() CrashRecoveryPolicy {
	return CrashRecoveryPolicy{
		Enabled: false,
		AutoExecuteOnSeverity: map[string]bool{
			"none":         false,
			"minor":        false,
			"major":        true,
			"catastrophic": true,
		},
		MaxAutoRecoveryCost:   5.00,
		PerSessionBudget:      1.00,
		EscalationThreshold:   3,
		MaxConcurrent:         1,
		CooldownAfterRecovery: 10 * time.Minute,
	}
}

// ShouldAutoExecute returns true if the policy allows auto-execution for the given severity.
func (p CrashRecoveryPolicy) ShouldAutoExecute(severity string) bool {
	if !p.Enabled {
		return false
	}
	allowed, ok := p.AutoExecuteOnSeverity[severity]
	return ok && allowed
}

// RecoverableSession holds enriched metadata for a dead Claude Code session.
type RecoverableSession struct {
	SessionID       string    `json:"session_id"`
	RepoPath        string    `json:"repo_path"`
	RepoName        string    `json:"repo_name"`
	SessionName     string    `json:"session_name,omitempty"`
	Priority        int       `json:"priority"` // 1 = highest
	OpenTasks       int       `json:"open_tasks"`
	TotalTasks      int       `json:"total_tasks"`
	LastActivity    time.Time `json:"last_activity"`
	PlanFile        string    `json:"plan_file,omitempty"`
	HasUncommitted  bool      `json:"has_uncommitted"`
	UnpushedCommits int       `json:"unpushed_commits"`
	ResumePrompt    string    `json:"resume_prompt"`
}

// ---------------------------------------------------------------------------
// CrashRecoveryOrchestrator
// ---------------------------------------------------------------------------

// CrashRecoveryOrchestrator detects and recovers from Claude Code session crashes.
type CrashRecoveryOrchestrator struct {
	mgr   *Manager
	bus   *events.Bus
	store Store

	mu               sync.Mutex
	lastCheck        time.Time
	lastPlan         *CrashRecoveryPlan
	recoveryActive   bool
	resumedSessions  map[string]string // claude session ID -> ralph session ID
	budget           *RecoveryBudgetEnvelope
	policy           CrashRecoveryPolicy
	failedRecoveries int
	lastRecovery     time.Time
}

// NewCrashRecoveryOrchestrator creates a new recovery orchestrator.
func NewCrashRecoveryOrchestrator(mgr *Manager, bus *events.Bus, store Store) *CrashRecoveryOrchestrator {
	return &CrashRecoveryOrchestrator{
		mgr:             mgr,
		bus:             bus,
		store:           store,
		resumedSessions: make(map[string]string),
	}
}

// SetBudget attaches a recovery budget envelope.
func (o *CrashRecoveryOrchestrator) SetBudget(b *RecoveryBudgetEnvelope) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.budget = b
}

// SetPolicy sets the crash recovery policy.
func (o *CrashRecoveryOrchestrator) SetPolicy(p CrashRecoveryPolicy) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.policy = p
}

// Policy returns the current crash recovery policy.
func (o *CrashRecoveryOrchestrator) Policy() CrashRecoveryPolicy {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.policy
}

// LastRecovery returns the time of the last recovery execution.
func (o *CrashRecoveryOrchestrator) LastRecovery() time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastRecovery
}

// FailedRecoveries returns the count of consecutive failed recovery attempts.
func (o *CrashRecoveryOrchestrator) FailedRecoveries() int {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.failedRecoveries
}

// ---------------------------------------------------------------------------
// Detection
// ---------------------------------------------------------------------------

// DetectCrash scans ~/.claude/sessions/ for dead processes within the given window.
// Returns a recovery plan if the dead session count meets the threshold.
func (o *CrashRecoveryOrchestrator) DetectCrash(ctx context.Context, window time.Duration, threshold int) (*CrashRecoveryPlan, error) {
	claudeDir := claudeBaseDir()

	// Load session metadata.
	sessions, err := loadClaudeSessions(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("load sessions: %w", err)
	}

	// Load history index for last activity times.
	lastActivity := loadClaudeHistoryIndex(claudeDir)

	cutoff := time.Now().Add(-window)

	plan := &CrashRecoveryPlan{
		DetectedAt:    time.Now(),
		TotalSessions: len(sessions),
	}

	for _, s := range sessions {
		startedAt := time.UnixMilli(s.StartedAt)
		la := lastActivity[s.SessionID]
		if la.IsZero() {
			la = startedAt
		}

		// Filter by window.
		if la.Before(cutoff) && startedAt.Before(cutoff) {
			continue
		}

		alive := pidAlive(s.PID)
		if alive {
			plan.AliveCount++
			continue
		}

		plan.DeadCount++

		// Enrich with task and git info.
		rs := RecoverableSession{
			SessionID:    s.SessionID,
			RepoPath:     s.CWD,
			RepoName:     filepath.Base(s.CWD),
			SessionName:  s.Name,
			LastActivity: la,
		}

		// Load tasks.
		tasks := loadClaudeTasks(claudeDir, s.SessionID)
		rs.TotalTasks = len(tasks)
		for _, t := range tasks {
			if t.Status != "completed" {
				rs.OpenTasks++
			}
		}

		// Check git state.
		if isGitRepo(s.CWD) {
			rs.HasUncommitted = hasUncommittedChanges(ctx, s.CWD)
			rs.UnpushedCommits = countUnpushedCommits(ctx, s.CWD)
			plan.ReposToVerify = appendUnique(plan.ReposToVerify, s.CWD)
		}

		// Find plan file.
		rs.PlanFile = findClaudePlanFile(claudeDir, s.SessionID, s.CWD)

		// Generate resume prompt.
		rs.ResumePrompt = buildResumePrompt(rs)

		plan.SessionsToResume = append(plan.SessionsToResume, rs)
	}

	// Sort by priority: most open tasks first, then most recent activity.
	sort.Slice(plan.SessionsToResume, func(i, j int) bool {
		a, b := plan.SessionsToResume[i], plan.SessionsToResume[j]
		if a.OpenTasks != b.OpenTasks {
			return a.OpenTasks > b.OpenTasks
		}
		return a.LastActivity.After(b.LastActivity)
	})
	for i := range plan.SessionsToResume {
		plan.SessionsToResume[i].Priority = i + 1
	}

	// Determine severity.
	switch {
	case plan.DeadCount >= 6:
		plan.Severity = "catastrophic"
	case plan.DeadCount >= 3:
		plan.Severity = "major"
	case plan.DeadCount >= threshold:
		plan.Severity = "minor"
	default:
		plan.Severity = "none"
	}

	// Persist recovery op if crash detected.
	if plan.DeadCount >= threshold && o.store != nil {
		op := &RecoveryOp{
			ID:            fmt.Sprintf("rec-%d", time.Now().UnixNano()),
			Severity:      plan.Severity,
			Status:        RecoveryOpDetected,
			TotalSessions: plan.TotalSessions,
			AliveCount:    plan.AliveCount,
			DeadCount:     plan.DeadCount,
			TriggerSource: "supervisor",
			DetectedAt:    plan.DetectedAt,
		}
		o.mu.Lock()
		if o.budget != nil {
			op.BudgetCapUSD = o.budget.TotalBudgetUSD
		}
		o.mu.Unlock()
		if err := o.store.SaveRecoveryOp(ctx, op); err != nil {
			slog.Warn("crash_recovery: failed to persist recovery op", "error", err)
		}
		plan.RecoveryOpID = op.ID
	}

	o.mu.Lock()
	o.lastCheck = time.Now()
	o.lastPlan = plan
	o.mu.Unlock()

	return plan, nil
}

// HasCrash returns true if the most recent detection found enough dead sessions.
func (o *CrashRecoveryOrchestrator) HasCrash(threshold int) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastPlan != nil && o.lastPlan.DeadCount >= threshold
}

// LastPlan returns the most recent recovery plan.
func (o *CrashRecoveryOrchestrator) LastPlan() *CrashRecoveryPlan {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.lastPlan
}

// ---------------------------------------------------------------------------
// Execution
// ---------------------------------------------------------------------------

// ExecuteRecovery resumes all sessions in the plan in priority order.
// It launches one session at a time, waiting for each to complete before
// starting the next. Set maxConcurrent > 1 for parallel recovery.
func (o *CrashRecoveryOrchestrator) ExecuteRecovery(ctx context.Context, plan *CrashRecoveryPlan, maxConcurrent int) error {
	if len(plan.SessionsToResume) == 0 {
		return nil
	}

	o.mu.Lock()
	if o.recoveryActive {
		o.mu.Unlock()
		return fmt.Errorf("recovery already in progress")
	}
	o.recoveryActive = true
	o.mu.Unlock()

	defer func() {
		o.mu.Lock()
		o.recoveryActive = false
		o.mu.Unlock()
	}()

	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	slog.Info("crash_recovery: starting recovery",
		"sessions", len(plan.SessionsToResume),
		"severity", plan.Severity,
		"max_concurrent", maxConcurrent,
	)

	// Update recovery op to executing.
	if plan.RecoveryOpID != "" && o.store != nil {
		if op, err := o.store.GetRecoveryOp(ctx, plan.RecoveryOpID); err == nil {
			now := time.Now()
			op.Status = RecoveryOpExecuting
			op.StartedAt = &now
			_ = o.store.SaveRecoveryOp(ctx, op)
		}
	}

	if o.bus != nil {
		o.bus.Publish(events.Event{
			Type:      events.SessionRecovered,
			Timestamp: time.Now(),
			Data: map[string]any{
				"action":   "recovery_started",
				"count":    len(plan.SessionsToResume),
				"severity": plan.Severity,
			},
		})
	}

	sem := make(chan struct{}, maxConcurrent)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errs []error
	var resumedCount, failedCount int

	for _, rs := range plan.SessionsToResume {
		if ctx.Err() != nil {
			break
		}

		// Budget check.
		o.mu.Lock()
		budget := o.budget
		o.mu.Unlock()
		if budget != nil && !budget.CanSpend(0.10) { // minimum viable cost
			slog.Warn("crash_recovery: budget exhausted, stopping recovery",
				"remaining", budget.Remaining())
			break
		}

		rs := rs

		// Create recovery action.
		var actionID string
		if plan.RecoveryOpID != "" && o.store != nil {
			action := &RecoveryAction{
				ID:              fmt.Sprintf("act-%d", time.Now().UnixNano()),
				RecoveryOpID:    plan.RecoveryOpID,
				ClaudeSessionID: rs.SessionID,
				RepoPath:        rs.RepoPath,
				RepoName:        rs.RepoName,
				Priority:        rs.Priority,
				Status:          ActionPending,
				CreatedAt:       time.Now(),
			}
			_ = o.store.SaveRecoveryAction(ctx, action)
			actionID = action.ID
		}

		wg.Add(1)
		sem <- struct{}{}

		go func() {
			defer wg.Done()
			defer func() { <-sem }()

			// Mark executing.
			if actionID != "" && o.store != nil {
				_ = o.store.UpdateRecoveryActionStatus(ctx, actionID, ActionExecuting, "")
			}

			sess, err := o.resumeSession(ctx, rs)
			if err != nil {
				slog.Warn("crash_recovery: failed to resume session",
					"session_id", rs.SessionID,
					"repo", rs.RepoName,
					"error", err,
				)
				if actionID != "" && o.store != nil {
					_ = o.store.UpdateRecoveryActionStatus(ctx, actionID, ActionFailed, err.Error())
				}
				mu.Lock()
				errs = append(errs, fmt.Errorf("%s (%s): %w", rs.RepoName, rs.SessionID, err))
				failedCount++
				mu.Unlock()
				return
			}

			slog.Info("crash_recovery: resumed session",
				"claude_session", rs.SessionID,
				"ralph_session", sess.ID,
				"repo", rs.RepoName,
			)

			if actionID != "" && o.store != nil {
				_ = o.store.UpdateRecoveryActionStatus(ctx, actionID, ActionSucceeded, "")
			}

			o.mu.Lock()
			o.resumedSessions[rs.SessionID] = sess.ID
			o.mu.Unlock()
			mu.Lock()
			resumedCount++
			mu.Unlock()
		}()
	}

	wg.Wait()

	// Update recovery op with final counts.
	if plan.RecoveryOpID != "" && o.store != nil {
		if op, err := o.store.GetRecoveryOp(ctx, plan.RecoveryOpID); err == nil {
			now := time.Now()
			op.CompletedAt = &now
			op.ResumedCount = resumedCount
			op.FailedCount = failedCount
			if failedCount > 0 && resumedCount == 0 {
				op.Status = RecoveryOpFailed
			} else {
				op.Status = RecoveryOpCompleted
			}
			if len(errs) > 0 {
				op.ErrorMsg = errs[0].Error()
			}
			_ = o.store.SaveRecoveryOp(ctx, op)
		}
	}

	// Update recovery tracking.
	o.mu.Lock()
	o.lastRecovery = time.Now()
	if failedCount > 0 && resumedCount == 0 {
		o.failedRecoveries++
	} else {
		o.failedRecoveries = 0
	}
	o.mu.Unlock()

	if len(errs) > 0 {
		return fmt.Errorf("recovery completed with %d errors: %v", len(errs), errs[0])
	}
	return nil
}

// resumeSession launches a single recovery session via Manager.Resume.
func (o *CrashRecoveryOrchestrator) resumeSession(ctx context.Context, rs RecoverableSession) (*Session, error) {
	return o.mgr.Resume(ctx, rs.RepoPath, ProviderClaude, rs.SessionID, rs.ResumePrompt)
}

// ---------------------------------------------------------------------------
// Verification
// ---------------------------------------------------------------------------

// VerifyRecovery checks that recovered sessions actually pushed their work.
func (o *CrashRecoveryOrchestrator) VerifyRecovery(ctx context.Context, plan *CrashRecoveryPlan) []VerificationResult {
	var results []VerificationResult

	for _, rs := range plan.SessionsToResume {
		result := VerificationResult{
			SessionID: rs.SessionID,
			RepoPath:  rs.RepoPath,
			RepoName:  rs.RepoName,
		}

		// Check tasks.
		tasks := loadClaudeTasks(claudeBaseDir(), rs.SessionID)
		for _, t := range tasks {
			result.TotalTasks++
			if t.Status == "completed" {
				result.CompletedTasks++
			}
		}

		// Check git state.
		if isGitRepo(rs.RepoPath) {
			result.HasUncommitted = hasUncommittedChanges(ctx, rs.RepoPath)
			result.UnpushedCommits = countUnpushedCommits(ctx, rs.RepoPath)
			result.Clean = !result.HasUncommitted && result.UnpushedCommits == 0
		}

		// Check if ralph session completed.
		o.mu.Lock()
		if ralphID, ok := o.resumedSessions[rs.SessionID]; ok {
			result.RalphSessionID = ralphID
			if o.store != nil {
				if sess, err := o.store.GetSession(ctx, ralphID); err == nil {
					result.RalphStatus = string(sess.Status)
				}
			}
		}
		o.mu.Unlock()

		results = append(results, result)
	}

	return results
}

// VerificationResult holds post-recovery state for a single session.
type VerificationResult struct {
	SessionID      string `json:"session_id"`
	RepoPath       string `json:"repo_path"`
	RepoName       string `json:"repo_name"`
	RalphSessionID string `json:"ralph_session_id,omitempty"`
	RalphStatus    string `json:"ralph_status,omitempty"`
	TotalTasks     int    `json:"total_tasks"`
	CompletedTasks int    `json:"completed_tasks"`
	HasUncommitted bool   `json:"has_uncommitted"`
	UnpushedCommits int   `json:"unpushed_commits"`
	Clean          bool   `json:"clean"` // true if all pushed, no uncommitted
}

// ---------------------------------------------------------------------------
// File reading helpers (reads ~/.claude/ directly)
// ---------------------------------------------------------------------------

func claudeBaseDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

func loadClaudeSessions(claudeDir string) ([]claudeSessionFile, error) {
	dir := filepath.Join(claudeDir, "sessions")
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var sessions []claudeSessionFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta claudeSessionFile
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		sessions = append(sessions, meta)
	}
	return sessions, nil
}

func loadClaudeHistoryIndex(claudeDir string) map[string]time.Time {
	path := filepath.Join(claudeDir, "history.jsonl")
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	index := make(map[string]time.Time)
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}
		sid, _ := entry["sessionId"].(string)
		if sid == "" {
			continue
		}
		ts, _ := entry["timestamp"].(float64)
		if ts > 0 {
			t := time.UnixMilli(int64(ts))
			if t.After(index[sid]) {
				index[sid] = t
			}
		}
	}
	return index
}

func loadClaudeTasks(claudeDir, sessionID string) []claudeTaskFile {
	dir := filepath.Join(claudeDir, "tasks", sessionID)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}

	var tasks []claudeTaskFile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var task claudeTaskFile
		if err := json.Unmarshal(data, &task); err != nil {
			continue
		}
		tasks = append(tasks, task)
	}
	return tasks
}

func findClaudePlanFile(claudeDir, sessionID, cwd string) string {
	// Search session JSONL for plan file references.
	encoded := strings.ReplaceAll(cwd, "/", "-")
	projectDir := filepath.Join(claudeDir, "projects", encoded)

	// Try direct JSONL.
	jsonlPath := filepath.Join(projectDir, sessionID+".jsonl")
	if _, err := os.Stat(jsonlPath); err != nil {
		// Try subdirectory.
		jsonlPath = filepath.Join(projectDir, sessionID, sessionID+".jsonl")
		if _, err := os.Stat(jsonlPath); err != nil {
			return ""
		}
	}

	// Read last 50 lines looking for plan references.
	f, err := os.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	start := 0
	if len(lines) > 50 {
		start = len(lines) - 50
	}

	for i := len(lines) - 1; i >= start; i-- {
		if idx := strings.Index(lines[i], "plans/"); idx >= 0 {
			rest := lines[i][idx+6:]
			endIdx := strings.IndexAny(rest, `"' `)
			if endIdx > 0 {
				name := rest[:endIdx]
				if strings.HasSuffix(name, ".md") {
					return name
				}
			}
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// Git helpers
// ---------------------------------------------------------------------------

func pidAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func isGitRepo(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

func hasUncommittedChanges(ctx context.Context, repoPath string) bool {
	out, err := gitCmd(ctx, repoPath, "status", "--porcelain")
	if err != nil {
		return false
	}
	return strings.TrimSpace(out) != ""
}

func countUnpushedCommits(ctx context.Context, repoPath string) int {
	for _, base := range []string{"origin/main", "origin/master"} {
		out, err := gitCmd(ctx, repoPath, "log", base+"..HEAD", "--oneline")
		if err != nil {
			continue
		}
		out = strings.TrimSpace(out)
		if out == "" {
			return 0
		}
		return len(strings.Split(out, "\n"))
	}
	return 0
}

func gitCmd(ctx context.Context, repoPath string, args ...string) (string, error) {
	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	fullArgs := append([]string{"-C", repoPath}, args...)
	cmd := exec.CommandContext(cmdCtx, "git", fullArgs...)
	out, err := cmd.Output()
	return string(out), err
}

// ---------------------------------------------------------------------------
// Misc helpers
// ---------------------------------------------------------------------------

func buildResumePrompt(rs RecoverableSession) string {
	var b strings.Builder
	b.WriteString("Continue from where you left off. ")

	if rs.OpenTasks > 0 {
		fmt.Fprintf(&b, "You have %d open tasks remaining (out of %d total). ", rs.OpenTasks, rs.TotalTasks)
	}
	if rs.PlanFile != "" {
		fmt.Fprintf(&b, "Your plan file is %s. ", rs.PlanFile)
	}

	b.WriteString("Check git status and review your task list before proceeding.")

	if rs.HasUncommitted {
		b.WriteString(" There are uncommitted changes in the working directory.")
	}
	if rs.UnpushedCommits > 0 {
		fmt.Fprintf(&b, " There are %d unpushed commits.", rs.UnpushedCommits)
	}

	return b.String()
}

func appendUnique(slice []string, s string) []string {
	for _, existing := range slice {
		if existing == s {
			return slice
		}
	}
	return append(slice, s)
}
