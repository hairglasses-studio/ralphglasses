package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/ralphglasses/internal/roadmap"
)

const defaultLoopVerifyCommand = "./scripts/dev/ci.sh"

// LoopProfile configures a perpetual Codex-style planner/worker loop.
type LoopProfile struct {
	PlannerProvider      Provider `json:"planner_provider"`
	PlannerModel         string   `json:"planner_model"`
	WorkerProvider       Provider `json:"worker_provider"`
	WorkerModel          string   `json:"worker_model"`
	VerifierProvider     Provider `json:"verifier_provider"`
	VerifierModel        string   `json:"verifier_model"`
	MaxConcurrentWorkers int      `json:"max_concurrent_workers"`
	RetryLimit           int      `json:"retry_limit"`
	VerifyCommands       []string `json:"verify_commands,omitempty"`
	WorktreePolicy       string   `json:"worktree_policy,omitempty"`
	PlannerBudgetUSD     float64  `json:"planner_budget_usd,omitempty"`
	WorkerBudgetUSD      float64  `json:"worker_budget_usd,omitempty"`
	VerifierBudgetUSD    float64  `json:"verifier_budget_usd,omitempty"`
}

// LoopTask is the bounded implementation unit produced by the planner.
type LoopTask struct {
	Title  string `json:"title"`
	Prompt string `json:"prompt"`
	Source string `json:"source,omitempty"`
}

// LoopVerification captures one local verification command execution.
type LoopVerification struct {
	Command   string    `json:"command"`
	Status    string    `json:"status"`
	ExitCode  int       `json:"exit_code"`
	Output    string    `json:"output,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
}

// LoopIteration captures one planner -> worker -> verifier pass.
type LoopIteration struct {
	Number            int                `json:"number"`
	Status            string             `json:"status"`
	Task              LoopTask           `json:"task"`
	PlannerSessionID  string             `json:"planner_session_id,omitempty"`
	WorkerSessionID   string             `json:"worker_session_id,omitempty"`
	VerifierSessionID string             `json:"verifier_session_id,omitempty"`
	WorktreePath      string             `json:"worktree_path,omitempty"`
	Branch            string             `json:"branch,omitempty"`
	PlannerOutput     string             `json:"planner_output,omitempty"`
	WorkerOutput      string             `json:"worker_output,omitempty"`
	Verification      []LoopVerification `json:"verification,omitempty"`
	Error             string             `json:"error,omitempty"`
	StartedAt         time.Time          `json:"started_at"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
}

// LoopRun is persisted state for a perpetual development loop.
type LoopRun struct {
	ID         string          `json:"id"`
	RepoPath   string          `json:"repo_path"`
	RepoName   string          `json:"repo_name"`
	Status     string          `json:"status"`
	Profile    LoopProfile     `json:"profile"`
	Iterations []LoopIteration `json:"iterations"`
	LastError  string          `json:"last_error,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	mu sync.Mutex
}

// Lock locks the loop run mutex for external callers.
func (r *LoopRun) Lock() { r.mu.Lock() }

// Unlock unlocks the loop run mutex.
func (r *LoopRun) Unlock() { r.mu.Unlock() }

// DefaultLoopProfile returns the repo-default Codex-only planner/worker setup.
func DefaultLoopProfile() LoopProfile {
	return LoopProfile{
		PlannerProvider:      ProviderCodex,
		PlannerModel:         "o1-pro",
		WorkerProvider:       ProviderCodex,
		WorkerModel:          "gpt-5.4-xhigh",
		VerifierProvider:     ProviderCodex,
		VerifierModel:        "gpt-5.4-xhigh",
		MaxConcurrentWorkers: 1,
		RetryLimit:           1,
		VerifyCommands:       []string{defaultLoopVerifyCommand},
		WorktreePolicy:       "git",
	}
}

// StartLoop registers a new loop run for a repo.
func (m *Manager) StartLoop(_ context.Context, repoPath string, profile LoopProfile) (*LoopRun, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, fmt.Errorf("repo path required")
	}
	if _, err := os.Stat(repoPath); err != nil {
		return nil, fmt.Errorf("stat repo: %w", err)
	}

	profile, err := normalizeLoopProfile(profile)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	run := &LoopRun{
		ID:         uuid.NewString(),
		RepoPath:   repoPath,
		RepoName:   filepath.Base(repoPath),
		Status:     "pending",
		Profile:    profile,
		Iterations: []LoopIteration{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	m.mu.Lock()
	m.loops[run.ID] = run
	m.mu.Unlock()

	m.PersistLoop(run)
	return run, nil
}

// GetLoop returns a loop run by ID.
func (m *Manager) GetLoop(id string) (*LoopRun, bool) {
	m.LoadExternalLoops()

	m.mu.Lock()
	defer m.mu.Unlock()
	run, ok := m.loops[id]
	return run, ok
}

// ListLoops returns all known loop runs.
func (m *Manager) ListLoops() []*LoopRun {
	m.LoadExternalLoops()

	m.mu.Lock()
	defer m.mu.Unlock()

	out := make([]*LoopRun, 0, len(m.loops))
	for _, run := range m.loops {
		out = append(out, run)
	}
	return out
}

// StopLoop marks a loop run as stopped.
func (m *Manager) StopLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop not found: %s", id)
	}

	run.mu.Lock()
	run.Status = "stopped"
	run.UpdatedAt = time.Now()
	run.mu.Unlock()

	m.PersistLoop(run)
	return nil
}

// StepLoop executes one planner/worker/verify iteration.
func (m *Manager) StepLoop(ctx context.Context, id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop not found: %s", id)
	}

	run.mu.Lock()
	if run.Status == "stopped" {
		run.mu.Unlock()
		return fmt.Errorf("loop %s is stopped", id)
	}
	if len(run.Iterations) > 0 {
		last := run.Iterations[len(run.Iterations)-1]
		switch last.Status {
		case "planning", "executing", "verifying":
			run.mu.Unlock()
			return fmt.Errorf("loop %s already has an active iteration", id)
		}
	}
	if consecutiveLoopFailures(run.Iterations) > run.Profile.RetryLimit {
		run.mu.Unlock()
		return fmt.Errorf("loop %s exceeded retry limit (%d)", id, run.Profile.RetryLimit)
	}

	iteration := LoopIteration{
		Number:    len(run.Iterations) + 1,
		Status:    "planning",
		StartedAt: time.Now(),
	}
	run.Iterations = append(run.Iterations, iteration)
	index := len(run.Iterations) - 1
	repoPath := run.RepoPath
	profile := run.Profile
	run.Status = "running"
	run.LastError = ""
	run.UpdatedAt = time.Now()
	run.mu.Unlock()

	m.PersistLoop(run)

	plannerPrompt, err := buildLoopPlannerPrompt(repoPath)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("build planner prompt: %w", err))
	}

	plannerSession, err := m.launchWorkflowSession(ctx, LaunchOptions{
		Provider:     profile.PlannerProvider,
		RepoPath:     repoPath,
		Prompt:       plannerPrompt,
		Model:        profile.PlannerModel,
		MaxBudgetUSD: profile.PlannerBudgetUSD,
		SessionName:  fmt.Sprintf("loop-plan-%s-%03d", run.RepoName, iteration.Number),
	})
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("launch planner session: %w", err))
	}

	m.updateLoopIteration(run, index, "planning", func(iter *LoopIteration, loop *LoopRun) {
		iter.PlannerSessionID = plannerSession.ID
	})

	if err := m.waitForSession(ctx, plannerSession); err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("planner session failed: %w", err))
	}

	task, plannerOutput, err := plannerTaskFromSession(plannerSession)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("parse planner output: %w", err))
	}

	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		iter.Task = task
		iter.PlannerOutput = plannerOutput
	})

	worktreePath, branch, err := createLoopWorktree(ctx, repoPath, run.ID, iteration.Number)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("create worktree: %w", err))
	}

	workerSession, err := m.launchWorkflowSession(ctx, LaunchOptions{
		Provider:     profile.WorkerProvider,
		RepoPath:     worktreePath,
		Prompt:       task.Prompt,
		Model:        profile.WorkerModel,
		MaxBudgetUSD: profile.WorkerBudgetUSD,
		SessionName:  fmt.Sprintf("loop-work-%s-%03d", run.RepoName, iteration.Number),
	})
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("launch worker session: %w", err))
	}

	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		iter.WorkerSessionID = workerSession.ID
		iter.WorktreePath = worktreePath
		iter.Branch = branch
	})

	if err := m.waitForSession(ctx, workerSession); err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("worker session failed: %w", err))
	}

	workerOutput := sessionOutputSummary(workerSession)
	m.updateLoopIteration(run, index, "verifying", func(iter *LoopIteration, loop *LoopRun) {
		iter.WorkerOutput = workerOutput
	})

	verification, err := runLoopVerification(ctx, worktreePath, profile.VerifyCommands)
	if err != nil {
		run.updateLoopAfterVerification(index, verification, "failed", err.Error())
		m.PersistLoop(run)
		_ = writeLoopJournal(run, run.Iterations[index])
		return err
	}

	run.updateLoopAfterVerification(index, verification, "idle", "")
	m.PersistLoop(run)
	_ = writeLoopJournal(run, run.Iterations[index])
	return nil
}

func normalizeLoopProfile(profile LoopProfile) (LoopProfile, error) {
	def := DefaultLoopProfile()

	if profile.PlannerProvider == "" {
		profile.PlannerProvider = def.PlannerProvider
	}
	if profile.WorkerProvider == "" {
		profile.WorkerProvider = def.WorkerProvider
	}
	if profile.VerifierProvider == "" {
		profile.VerifierProvider = def.VerifierProvider
	}
	if profile.PlannerModel == "" {
		profile.PlannerModel = def.PlannerModel
	}
	if profile.WorkerModel == "" {
		profile.WorkerModel = def.WorkerModel
	}
	if profile.VerifierModel == "" {
		profile.VerifierModel = def.VerifierModel
	}
	if profile.MaxConcurrentWorkers <= 0 {
		profile.MaxConcurrentWorkers = def.MaxConcurrentWorkers
	}
	if profile.RetryLimit < 0 {
		return profile, fmt.Errorf("retry limit must be >= 0")
	}
	if len(profile.VerifyCommands) == 0 {
		profile.VerifyCommands = append([]string(nil), def.VerifyCommands...)
	}
	if profile.WorktreePolicy == "" {
		profile.WorktreePolicy = def.WorktreePolicy
	}
	if profile.MaxConcurrentWorkers != 1 {
		return profile, fmt.Errorf("max concurrent workers > 1 not implemented yet")
	}
	if profile.WorktreePolicy != "git" {
		return profile, fmt.Errorf("unsupported worktree policy %q", profile.WorktreePolicy)
	}

	for _, provider := range []Provider{
		profile.PlannerProvider,
		profile.WorkerProvider,
		profile.VerifierProvider,
	} {
		if providerBinary(provider) == "" {
			return profile, fmt.Errorf("unknown loop provider %q", provider)
		}
	}

	return profile, nil
}

func buildLoopPlannerPrompt(repoPath string) (string, error) {
	var sections []string
	sections = append(sections, `You are the planner for a perpetual development loop.

Choose exactly one bounded next task for the repo and respond with JSON only:
{"title":"short task title","prompt":"implementation prompt for the worker"}

Constraints:
- Pick the highest-impact unfinished task that is safe to execute next.
- Keep the worker task concrete and implementation-focused.
- Assume verification will run after the worker finishes.
- Do not include markdown fences or prose outside the JSON object.`)

	roadmapPath := filepath.Join(repoPath, "ROADMAP.md")
	if _, err := os.Stat(roadmapPath); err == nil {
		if rm, err := roadmap.Parse(roadmapPath); err == nil {
			analysis, analyzeErr := roadmap.Analyze(rm, repoPath)
			if analyzeErr == nil {
				var ready []string
				for i, item := range analysis.Ready {
					if i >= 5 {
						break
					}
					ready = append(ready, fmt.Sprintf("- %s: %s", item.TaskID, item.Description))
				}
				sections = append(sections, fmt.Sprintf(
					"Roadmap summary:\n- Title: %s\n- Completion: %d/%d\n- Ready tasks:\n%s",
					rm.Title,
					rm.Stats.Completed,
					rm.Stats.Total,
					joinOrPlaceholder(ready, "- none detected"),
				))
			}
		}
	}

	issueLedgerPath := filepath.Join(repoPath, "docs", "issue-ledger.json")
	if data, err := os.ReadFile(issueLedgerPath); err == nil && len(data) > 0 {
		sections = append(sections, "Issue ledger:\n"+truncateForPrompt(string(data), 2500))
	}

	journal, err := ReadRecentJournal(repoPath, 5)
	if err == nil && len(journal) > 0 {
		sections = append(sections, "Recent journal context:\n"+SynthesizeContext(journal))
	}

	return strings.Join(sections, "\n\n"), nil
}

func plannerTaskFromSession(s *Session) (LoopTask, string, error) {
	output := sessionOutputSummary(s)
	task, err := parsePlannerTask(output)
	if err == nil {
		return task, output, nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for _, candidate := range []string{s.LastOutput, strings.Join(s.OutputHistory, "\n")} {
		task, parseErr := parsePlannerTask(candidate)
		if parseErr == nil {
			return task, candidate, nil
		}
	}

	return LoopTask{}, output, err
}

func parsePlannerTask(text string) (LoopTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return LoopTask{}, errors.New("planner output is empty")
	}

	var task LoopTask
	for _, candidate := range plannerJSONCandidates(text) {
		if err := json.Unmarshal([]byte(candidate), &task); err == nil {
			task.Title = strings.TrimSpace(task.Title)
			task.Prompt = strings.TrimSpace(task.Prompt)
			if task.Title == "" && task.Prompt != "" {
				task.Title = firstLine(task.Prompt)
			}
			if task.Prompt == "" && task.Title != "" {
				task.Prompt = task.Title
			}
			if task.Title != "" && task.Prompt != "" {
				return task, nil
			}
		}
	}

	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return LoopTask{}, errors.New("planner output did not contain a task")
	}
	return LoopTask{
		Title:  firstLine(lines[0]),
		Prompt: strings.Join(lines, "\n"),
		Source: "fallback",
	}, nil
}

func plannerJSONCandidates(text string) []string {
	var out []string
	out = append(out, text)

	reFence := regexp.MustCompile("(?s)```json\\s*(\\{.*?\\})\\s*```")
	if matches := reFence.FindStringSubmatch(text); len(matches) == 2 {
		out = append(out, matches[1])
	}

	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start >= 0 && end > start {
		out = append(out, text[start:end+1])
	}

	return dedupeStrings(out)
}

func createLoopWorktree(ctx context.Context, repoPath, loopID string, iteration int) (string, string, error) {
	repoRoot, err := gitTopLevel(ctx, repoPath)
	if err != nil {
		return "", "", err
	}

	worktreePath := filepath.Join(repoRoot, ".ralph", "worktrees", "loops", sanitizeLoopName(loopID), fmt.Sprintf("%03d", iteration))
	if err := os.MkdirAll(filepath.Dir(worktreePath), 0755); err != nil {
		return "", "", fmt.Errorf("create worktree parent: %w", err)
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return "", "", fmt.Errorf("worktree path already exists: %s", worktreePath)
	}

	branch := fmt.Sprintf("ralph/%s/%03d", sanitizeLoopName(loopID), iteration)
	cmd := exec.CommandContext(ctx, "git", "-C", repoRoot, "worktree", "add", "-B", branch, worktreePath, "HEAD")
	if output, err := cmd.CombinedOutput(); err != nil {
		return "", "", fmt.Errorf("git worktree add: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return worktreePath, branch, nil
}

func gitTopLevel(ctx context.Context, repoPath string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "rev-parse", "--show-toplevel")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("resolve git repo: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func runLoopVerification(ctx context.Context, worktreePath string, commands []string) ([]LoopVerification, error) {
	results := make([]LoopVerification, 0, len(commands))
	for _, command := range commands {
		command = strings.TrimSpace(command)
		if command == "" {
			continue
		}

		started := time.Now()
		cmd := exec.CommandContext(ctx, "bash", "-lc", command)
		cmd.Dir = worktreePath
		output, err := cmd.CombinedOutput()

		result := LoopVerification{
			Command:   command,
			Status:    "completed",
			ExitCode:  0,
			Output:    truncateForPrompt(string(output), 4000),
			StartedAt: started,
			EndedAt:   time.Now(),
		}

		if err != nil {
			result.Status = "failed"
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				result.ExitCode = exitErr.ExitCode()
			} else {
				result.ExitCode = -1
			}
			results = append(results, result)
			return results, fmt.Errorf("verify command failed (%s)", command)
		}

		results = append(results, result)
	}
	return results, nil
}

func (m *Manager) failLoopIteration(run *LoopRun, index int, err error) error {
	run.updateLoopAfterVerification(index, run.iterationVerification(index), "failed", err.Error())
	m.PersistLoop(run)
	_ = writeLoopJournal(run, run.iterationsSnapshot()[index])
	return err
}

func (m *Manager) updateLoopIteration(run *LoopRun, index int, status string, mutate func(*LoopIteration, *LoopRun)) {
	run.mu.Lock()
	defer run.mu.Unlock()

	if index < 0 || index >= len(run.Iterations) {
		return
	}
	run.Iterations[index].Status = status
	if mutate != nil {
		mutate(&run.Iterations[index], run)
	}
	run.Status = "running"
	run.UpdatedAt = time.Now()
}

func (r *LoopRun) updateLoopAfterVerification(index int, verification []LoopVerification, status, errMsg string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if index < 0 || index >= len(r.Iterations) {
		return
	}
	iter := &r.Iterations[index]
	iter.Verification = append([]LoopVerification(nil), verification...)
	iter.Status = status
	iter.Error = strings.TrimSpace(errMsg)
	now := time.Now()
	iter.EndedAt = &now
	r.Status = status
	r.LastError = strings.TrimSpace(errMsg)
	r.UpdatedAt = now
}

func (r *LoopRun) iterationVerification(index int) []LoopVerification {
	r.mu.Lock()
	defer r.mu.Unlock()
	if index < 0 || index >= len(r.Iterations) {
		return nil
	}
	return append([]LoopVerification(nil), r.Iterations[index].Verification...)
}

func (r *LoopRun) iterationsSnapshot() []LoopIteration {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]LoopIteration, len(r.Iterations))
	copy(out, r.Iterations)
	return out
}

func consecutiveLoopFailures(iterations []LoopIteration) int {
	failures := 0
	for i := len(iterations) - 1; i >= 0; i-- {
		if iterations[i].Status != "failed" {
			break
		}
		failures++
	}
	return failures
}

func sessionOutputSummary(s *Session) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	var parts []string
	if len(s.OutputHistory) > 0 {
		parts = append(parts, strings.Join(s.OutputHistory, "\n"))
	}
	if s.LastOutput != "" {
		parts = append(parts, s.LastOutput)
	}
	if s.Error != "" {
		parts = append(parts, s.Error)
	}
	return strings.TrimSpace(strings.Join(dedupeStrings(parts), "\n"))
}

func writeLoopJournal(run *LoopRun, iter LoopIteration) error {
	entry := JournalEntry{
		Timestamp: time.Now(),
		SessionID: iter.WorkerSessionID,
		Provider:  string(run.Profile.WorkerProvider),
		RepoName:  run.RepoName,
		Model:     run.Profile.WorkerModel,
		TaskFocus: iter.Task.Title,
	}
	if iter.Status == "failed" {
		entry.Failed = []string{firstNonBlank(iter.Error, "loop iteration failed")}
	} else {
		entry.Worked = []string{firstNonBlank(iter.Task.Title, "loop iteration completed")}
	}
	return WriteJournalEntryManual(run.RepoPath, entry)
}

func joinOrPlaceholder(items []string, placeholder string) string {
	if len(items) == 0 {
		return placeholder
	}
	return strings.Join(items, "\n")
}

func truncateForPrompt(s string, limit int) string {
	s = strings.TrimSpace(s)
	if limit <= 0 || len(s) <= limit {
		return s
	}
	return s[:limit-3] + "..."
}

func sanitizeLoopName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	s = regexp.MustCompile(`[^a-z0-9-]+`).ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "loop"
	}
	return s
}

func nonEmptyLines(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func firstLine(text string) string {
	lines := nonEmptyLines(text)
	if len(lines) == 0 {
		return ""
	}
	return lines[0]
}

func dedupeStrings(items []string) []string {
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (m *Manager) loopStateDir() string {
	if m.stateDir == "" {
		return ""
	}
	return filepath.Join(m.stateDir, "loops")
}

// PersistLoop writes loop state to disk.
func (m *Manager) PersistLoop(run *LoopRun) {
	dir := m.loopStateDir()
	if dir == "" {
		return
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return
	}

	run.mu.Lock()
	data, err := json.Marshal(run)
	run.mu.Unlock()
	if err != nil {
		return
	}

	_ = os.WriteFile(filepath.Join(dir, run.ID+".json"), data, 0644)
}

// LoadExternalLoops merges loop runs persisted by other processes.
func (m *Manager) LoadExternalLoops() {
	dir := m.loopStateDir()
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if _, ok := m.loops[id]; ok {
			continue
		}
		data, readErr := os.ReadFile(filepath.Join(dir, entry.Name()))
		if readErr != nil {
			continue
		}
		var run LoopRun
		if err := json.Unmarshal(data, &run); err != nil {
			continue
		}
		m.loops[id] = &run
	}
}
