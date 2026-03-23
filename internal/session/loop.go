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

	"github.com/hairglasses-studio/ralphglasses/internal/enhancer"
	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
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
	Tasks             []LoopTask         `json:"tasks,omitempty"` // multiple tasks for concurrent execution
	PlannerSessionID  string             `json:"planner_session_id,omitempty"`
	WorkerSessionID   string             `json:"worker_session_id,omitempty"`    // first/only worker (backwards compat)
	WorkerSessionIDs  []string           `json:"worker_session_ids,omitempty"`   // all workers for concurrent execution
	VerifierSessionID string             `json:"verifier_session_id,omitempty"`
	WorktreePath      string             `json:"worktree_path,omitempty"`
	WorktreePaths     []string           `json:"worktree_paths,omitempty"` // per-worker worktrees
	Branch            string             `json:"branch,omitempty"`
	PlannerOutput     string             `json:"planner_output,omitempty"`
	WorkerOutput      string             `json:"worker_output,omitempty"`
	HasQuestions      bool               `json:"has_questions,omitempty"`
	WorkerOutputs     []string           `json:"worker_outputs,omitempty"` // per-worker outputs
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
// When MaxConcurrentWorkers > 1, the planner is asked for multiple tasks
// and workers execute in parallel, each in their own git worktree.
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

	// Validate that critical ralph files still exist before proceeding.
	// Claude can accidentally delete .ralph/ during cleanup tasks.
	if err := repofiles.ValidateIntegrity(repoPath); err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("integrity check: %w", err))
	}

	numWorkers := profile.MaxConcurrentWorkers
	if numWorkers <= 0 {
		numWorkers = 1
	}

	plannerPrompt, err := buildLoopPlannerPromptN(repoPath, numWorkers)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("build planner prompt: %w", err))
	}

	// Enhance planner prompt for the planner's target provider
	if m.Enhancer != nil {
		plannerPrompt = m.enhanceForProvider(ctx, plannerPrompt, profile.PlannerProvider)
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

	tasks, plannerOutput, err := plannerTasksFromSession(plannerSession, numWorkers)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("parse planner output: %w", err))
	}

	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		iter.Task = tasks[0] // backwards compat: first task
		iter.Tasks = tasks
		iter.PlannerOutput = plannerOutput
	})

	// Fan out workers in parallel, each in their own worktree
	type workerResult struct {
		idx       int
		session   *Session
		worktree  string
		branch    string
		output    string
		err       error
	}

	resultCh := make(chan workerResult, len(tasks))
	for i, task := range tasks {
		go func(workerIdx int, t LoopTask) {
			wt, br, wtErr := createLoopWorktree(ctx, repoPath, run.ID, iteration.Number*100+workerIdx)
			if wtErr != nil {
				resultCh <- workerResult{idx: workerIdx, err: fmt.Errorf("create worktree: %w", wtErr)}
				return
			}

			// Enhance worker prompt for the worker's target provider
			workerPrompt := t.Prompt
			if m.Enhancer != nil {
				workerPrompt = m.enhanceForProvider(ctx, workerPrompt, profile.WorkerProvider)
			}

			ws, launchErr := m.launchWorkflowSession(ctx, LaunchOptions{
				Provider:     profile.WorkerProvider,
				RepoPath:     wt,
				Prompt:       workerPrompt,
				Model:        profile.WorkerModel,
				MaxBudgetUSD: profile.WorkerBudgetUSD,
				SessionName:  fmt.Sprintf("loop-work-%s-%03d-%d", run.RepoName, iteration.Number, workerIdx),
			})
			if launchErr != nil {
				resultCh <- workerResult{idx: workerIdx, worktree: wt, err: fmt.Errorf("launch worker: %w", launchErr)}
				return
			}

			waitErr := m.waitForSession(ctx, ws)
			// Check if productive work happened despite timeout
			if waitErr != nil && errors.Is(waitErr, context.DeadlineExceeded) && hasGitChanges(wt) {
				waitErr = nil // Worker made progress; treat as success
			}
			out := sessionOutputSummary(ws)
			resultCh <- workerResult{idx: workerIdx, session: ws, worktree: wt, branch: br, output: out, err: waitErr}
		}(i, task)
	}

	// Collect results
	workerSessionIDs := make([]string, len(tasks))
	workerWorktrees := make([]string, len(tasks))
	workerOutputs := make([]string, len(tasks))
	var workerErrs []string
	var firstWorktree, firstBranch string

	for range tasks {
		res := <-resultCh
		if res.session != nil {
			workerSessionIDs[res.idx] = res.session.ID
		}
		workerWorktrees[res.idx] = res.worktree
		workerOutputs[res.idx] = res.output
		if res.err != nil {
			workerErrs = append(workerErrs, fmt.Sprintf("worker %d: %s", res.idx, res.err))
		}
		if res.idx == 0 {
			firstWorktree = res.worktree
			firstBranch = res.branch
		}
	}

	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		if len(workerSessionIDs) > 0 {
			iter.WorkerSessionID = workerSessionIDs[0]
		}
		iter.WorkerSessionIDs = workerSessionIDs
		iter.WorktreePath = firstWorktree
		iter.WorktreePaths = workerWorktrees
		iter.Branch = firstBranch
		iter.WorkerOutputs = workerOutputs
		if len(workerOutputs) > 0 {
			iter.WorkerOutput = workerOutputs[0]
		}
	})

	if len(workerErrs) > 0 {
		errMsg := strings.Join(workerErrs, "; ")
		return m.failLoopIteration(run, index, fmt.Errorf("worker(s) failed: %s", errMsg))
	}

	// Detect if any worker asked questions instead of acting autonomously.
	hasQ := false
	for _, wo := range workerOutputs {
		if q, _ := DetectQuestions(wo); q {
			hasQ = true
			break
		}
	}

	// Verify: run verification on each worktree
	m.updateLoopIteration(run, index, "verifying", func(iter *LoopIteration, loop *LoopRun) {
		iter.HasQuestions = hasQ
	})

	var allVerification []LoopVerification
	for _, wt := range workerWorktrees {
		if wt == "" {
			continue
		}
		verification, verErr := runLoopVerification(ctx, wt, profile.VerifyCommands)
		allVerification = append(allVerification, verification...)
		if verErr != nil {
			run.updateLoopAfterVerification(index, allVerification, "failed", verErr.Error())
			m.PersistLoop(run)
			_ = writeLoopJournal(run, run.Iterations[index])
			return verErr
		}
	}

	run.updateLoopAfterVerification(index, allVerification, "idle", "")
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
	if profile.MaxConcurrentWorkers > 8 {
		return profile, fmt.Errorf("max concurrent workers capped at 8, got %d", profile.MaxConcurrentWorkers)
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

// buildLoopPlannerPromptN builds a planner prompt requesting N parallel tasks.
func buildLoopPlannerPromptN(repoPath string, numTasks int) (string, error) {
	if numTasks <= 1 {
		return buildLoopPlannerPrompt(repoPath, nil)
	}
	prompt, err := buildLoopPlannerPrompt(repoPath, nil)
	if err != nil {
		return "", err
	}
	// Replace the single-task instruction with multi-task
	prompt = strings.Replace(prompt,
		`Choose exactly one bounded next task for the repo and respond with JSON only:
{"title":"short task title","prompt":"implementation prompt for the worker"}`,
		fmt.Sprintf(`Choose up to %d independent tasks that can run in parallel (no file conflicts).
Respond with a JSON array only:
[{"title":"task 1","prompt":"implementation prompt"},{"title":"task 2","prompt":"implementation prompt"}]

Each task runs in its own git worktree, so they must not modify the same files.`, numTasks),
		1)
	return prompt, nil
}

func buildLoopPlannerPrompt(repoPath string, prevIterations []LoopIteration) (string, error) {
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

	// Inject corrective guidance from previous iterations.
	if len(prevIterations) > 0 {
		last := prevIterations[len(prevIterations)-1]
		sections = append(sections, fmt.Sprintf(
			"Previous iteration: task=%q status=%s", last.Task.Title, last.Status))
		if last.HasQuestions {
			sections = append(sections,
				`IMPORTANT: The previous worker asked questions instead of acting autonomously.
In headless mode, no human will answer. Re-task with explicit instructions to make autonomous decisions using conservative defaults.`)
		}
	}

	return strings.Join(sections, "\n\n"), nil
}

// plannerTasksFromSession extracts up to maxTasks from the planner output.
// It tries to parse a JSON array first; if that fails, falls back to single task.
func plannerTasksFromSession(s *Session, maxTasks int) ([]LoopTask, string, error) {
	output := sessionOutputSummary(s)

	// Try multi-task parse first (JSON array)
	if maxTasks > 1 {
		tasks, err := parsePlannerTasks(output)
		if err == nil && len(tasks) > 0 {
			if len(tasks) > maxTasks {
				tasks = tasks[:maxTasks]
			}
			return tasks, output, nil
		}

		// Try from session fields
		s.mu.Lock()
		for _, candidate := range []string{s.LastOutput, strings.Join(s.OutputHistory, "\n")} {
			tasks, parseErr := parsePlannerTasks(candidate)
			if parseErr == nil && len(tasks) > 0 {
				s.mu.Unlock()
				if len(tasks) > maxTasks {
					tasks = tasks[:maxTasks]
				}
				return tasks, candidate, nil
			}
		}
		s.mu.Unlock()
	}

	// Fall back to single task
	task, out, err := plannerTaskFromSession(s)
	if err != nil {
		return nil, out, err
	}
	return []LoopTask{task}, out, nil
}

// parsePlannerTasks tries to parse a JSON array of tasks from planner output.
func parsePlannerTasks(text string) ([]LoopTask, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, errors.New("empty output")
	}

	// Try direct parse
	var tasks []LoopTask
	for _, candidate := range plannerJSONArrayCandidates(text) {
		if err := json.Unmarshal([]byte(candidate), &tasks); err == nil && len(tasks) > 0 {
			valid := make([]LoopTask, 0, len(tasks))
			for _, t := range tasks {
				t.Title = strings.TrimSpace(t.Title)
				t.Prompt = strings.TrimSpace(t.Prompt)
				if t.Title != "" && t.Prompt != "" {
					valid = append(valid, t)
				}
			}
			if len(valid) > 0 {
				return valid, nil
			}
		}
	}

	return nil, errors.New("no task array found in planner output")
}

func plannerJSONArrayCandidates(text string) []string {
	var out []string
	out = append(out, text)

	reFence := regexp.MustCompile("(?s)```json\\s*(\\[.*?\\])\\s*```")
	if matches := reFence.FindStringSubmatch(text); len(matches) == 2 {
		out = append(out, matches[1])
	}

	start := strings.IndexByte(text, '[')
	end := strings.LastIndexByte(text, ']')
	if start >= 0 && end > start {
		out = append(out, text[start:end+1])
	}

	return dedupeStrings(out)
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

// enhanceForProvider runs hybrid prompt enhancement targeting the given session provider.
// Uses ModeAuto so LLM failures fall back to the local pipeline — never blocks the loop.
func (m *Manager) enhanceForProvider(ctx context.Context, prompt string, provider Provider) string {
	target := mapProvider(provider)
	cfg := enhancer.Config{TargetProvider: target}
	result := enhancer.EnhanceHybrid(ctx, prompt, "", cfg, m.Enhancer, enhancer.ModeAuto, target)
	if result.Enhanced != prompt && m.bus != nil {
		m.bus.Publish(events.Event{
			Type: events.PromptEnhanced,
			Data: map[string]any{
				"target_provider": string(target),
				"source":          result.Source,
				"stages_run":      result.StagesRun,
			},
		})
	}
	return result.Enhanced
}

// mapProvider converts a session Provider to the enhancer's ProviderName.
func mapProvider(p Provider) enhancer.ProviderName {
	switch p {
	case ProviderGemini:
		return enhancer.ProviderGemini
	case ProviderCodex:
		return enhancer.ProviderOpenAI
	default:
		return enhancer.ProviderClaude
	}
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

// hasGitChanges checks whether the given repo path has uncommitted or new
// changes relative to HEAD, indicating productive work despite a timeout.
func hasGitChanges(repoPath string) bool {
	cmd := exec.Command("git", "-C", repoPath, "diff", "--stat", "HEAD")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(output))) > 0
}
