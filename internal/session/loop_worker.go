package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// workerResult captures the outcome of a single worker goroutine.
type workerResult struct {
	idx           int
	session       *Session
	worktree      string
	branch        string
	output        string
	err           error
	cascadeResult *CascadeResult // WS3: non-nil if cascade routing was attempted
	stallCount    int            // WS-B: stall events detected by StallDetector
	pooled        bool           // Phase 10.5.8: true if worktree came from pool
}

// workerParams bundles the inputs for runWorkerTask to avoid a long parameter list.
type workerParams struct {
	ctx        context.Context
	run        *LoopRun
	iteration  LoopIteration
	workerIdx  int
	task       LoopTask
	profile    LoopProfile
	repoPath   string
	useCascade bool
}

// runWorkerTask executes a single worker task in its own worktree.
// It is called from a goroutine inside StepLoop.
func (m *Manager) runWorkerTask(p workerParams) workerResult {
	// WS-B: Create stall detector for this worker.
	var detector *StallDetector
	if p.profile.StallTimeout > 0 {
		detector = NewStallDetector(p.profile.StallTimeout)
	}

	// Check for cancellation before expensive worktree creation.
	if err := p.ctx.Err(); err != nil {
		return workerResult{idx: p.workerIdx, err: fmt.Errorf("cancelled before worktree creation: %w", err)}
	}

	// Phase 10.5.8: Try worktree pool first, fall back to direct creation.
	var wt, br string
	var wtErr error
	var pooled bool
	if pool := m.WorktreePool(); pool != nil {
		wt, br, wtErr = pool.Acquire(p.ctx, p.repoPath)
		if wtErr == nil {
			pooled = true
		} else {
			slog.Debug("worktree pool acquire failed, falling back to direct creation",
				"repo", p.repoPath, "error", wtErr)
		}
	}
	if !pooled {
		wt, br, wtErr = createLoopWorktree(p.ctx, p.repoPath, p.run.ID, p.iteration.Number*100+p.workerIdx)
	}
	if wtErr != nil {
		return workerResult{idx: p.workerIdx, err: fmt.Errorf("create worktree: %w", wtErr)}
	}

	taskType := classifyTask(p.task.Title)

	// WS2: Inject episodic examples into worker prompt for similar tasks.
	workerPrompt := p.task.Prompt
	if m.episodic != nil && p.profile.EnableEpisodicMemory {
		eps := m.episodic.FindSimilar(taskType, p.task.Title, 0)
		if len(eps) > 0 {
			if examples := m.episodic.FormatExamples(eps); examples != "" {
				workerPrompt = examples + "\n\n" + workerPrompt
			}
		}
	}

	// WS1: Inject reflexion corrections into worker prompt for similar tasks.
	if m.reflexion != nil && p.profile.EnableReflexion {
		refs := m.reflexion.RecentForTask(p.task.Title, 3)
		if len(refs) > 0 {
			if formatted := m.reflexion.FormatForPrompt(refs); formatted != "" {
				workerPrompt = formatted + "\n\n" + workerPrompt
			}
		}
	}

	// Enhance worker prompt for the worker's target provider
	var workerEnhance enhanceResult
	if m.Enhancer != nil && p.profile.EnableWorkerEnhancement {
		workerEnhance = m.enhanceForProvider(p.ctx, workerPrompt, p.profile.WorkerProvider)
		workerPrompt = workerEnhance.prompt
	} else {
		workerEnhance = enhanceResult{prompt: workerPrompt, source: "none", preScore: 0}
	}

	baseOpts := LaunchOptions{
		Provider:     p.profile.WorkerProvider,
		RepoPath:     wt,
		Prompt:       workerPrompt,
		Model:        p.profile.WorkerModel,
		MaxBudgetUSD: p.profile.WorkerBudgetUSD,
		SessionName:  fmt.Sprintf("loop-work-%s-%03d-%d", p.run.RepoName, p.iteration.Number, p.workerIdx),
	}

	// Enable compaction beta for long-running loops once iteration
	// count exceeds the configured threshold.
	if p.profile.CompactionEnabled && p.iteration.Number > p.profile.CompactionThreshold {
		baseOpts.Betas = append(baseOpts.Betas, "compact-2026-01-12")
	}

	// Check for cancellation before cascade/launch phase.
	if err := p.ctx.Err(); err != nil {
		return workerResult{idx: p.workerIdx, worktree: wt, pooled: pooled, err: fmt.Errorf("cancelled before session launch: %w", err)}
	}

	// WS3: Try cheap provider first if cascade routing is enabled.
	var cascadeRes *CascadeResult
	if p.useCascade && m.cascade.ShouldCascade(taskType, p.task.Prompt) {
		// Use SelectTier to refine model choice based on task complexity.
		tier := m.cascade.SelectTier(taskType, 0)
		if tier.Model != "" {
			baseOpts.Model = tier.Model
			baseOpts.Provider = tier.Provider
		}
		cheapOpts := m.cascade.CheapLaunchOpts(baseOpts)
		cheapSess, cheapErr := m.launchWorkflowSession(p.ctx, cheapOpts)
		if cheapErr == nil {
			cheapSess.EnhancementSource = workerEnhance.source
			cheapSess.EnhancementPreScore = workerEnhance.preScore
			// WS-B: Monitor for stalls during cheap cascade session.
			if detector != nil {
				detector.RecordActivity()
				stallCh := detector.Start()
				go func() {
					for range stallCh {
						// Drain stall notifications; count is tracked in detector.
					}
				}()
			}
			if waitErr := m.waitForSession(p.ctx, cheapSess); waitErr != nil {
				slog.Warn("cheap cascade session wait failed", "session", cheapSess.ID, "error", waitErr)
			}
			if detector != nil {
				detector.Stop()
				detector.RecordActivity() // reset for next phase
			}

			// Run quick verification to assess cheap result
			cheapVerify, _ := runLoopVerification(p.ctx, wt, p.profile.VerifyCommands)
			escalate, conf, reason := m.cascade.EvaluateCheapResult(cheapSess, 10, cheapVerify)

			cheapSess.Lock()
			cheapCost := cheapSess.SpentUSD
			cheapSess.Unlock()

			cr := CascadeResult{
				UsedProvider:    cheapOpts.Provider,
				CheapConfidence: conf,
				CheapCostUSD:    cheapCost,
				TotalCostUSD:    cheapCost,
			}

			if !escalate {
				// Cheap provider succeeded — skip expensive launch
				cr.Escalated = false
				cascadeRes = &cr
				m.cascade.RecordResult(cr)
				out := sessionOutputSummary(cheapSess)
				workerStalls := 0
				if detector != nil {
					workerStalls = detector.StallCount()
				}
				return workerResult{
					idx: p.workerIdx, session: cheapSess, worktree: wt,
					branch: br, output: out, cascadeResult: &cr,
					stallCount: workerStalls, pooled: pooled,
				}
			}
			// Escalate: continue to expensive provider
			cr.Escalated = true
			cr.Reason = reason
			cascadeRes = &cr
		}
	}

	// Check for cancellation before expensive main session launch.
	if err := p.ctx.Err(); err != nil {
		return workerResult{idx: p.workerIdx, worktree: wt, pooled: pooled, err: fmt.Errorf("cancelled before main session launch: %w", err), cascadeResult: cascadeRes}
	}

	ws, launchErr := m.launchWorkflowSession(p.ctx, baseOpts)
	if launchErr != nil {
		return workerResult{idx: p.workerIdx, worktree: wt, pooled: pooled, err: fmt.Errorf("launch worker: %w", launchErr), cascadeResult: cascadeRes}
	}
	ws.EnhancementSource = workerEnhance.source
	ws.EnhancementPreScore = workerEnhance.preScore

	// WS-B: Monitor for stalls during main worker session.
	if detector != nil {
		detector.RecordActivity()
		stallCh := detector.Start()
		go func() {
			for range stallCh {
				// Drain stall notifications; count is tracked in detector.
			}
		}()
	}
	waitErr := m.waitForSession(p.ctx, ws)
	if detector != nil {
		detector.Stop()
	}
	// Check if productive work happened despite timeout
	if waitErr != nil && errors.Is(waitErr, context.DeadlineExceeded) && hasGitChanges(wt) {
		waitErr = nil // Worker made progress; treat as success
	}

	// WS3: Record cascade outcome with total cost.
	if cascadeRes != nil {
		ws.Lock()
		cascadeRes.TotalCostUSD = cascadeRes.CheapCostUSD + ws.SpentUSD
		cascadeRes.UsedProvider = baseOpts.Provider
		ws.Unlock()
		m.cascade.RecordResult(*cascadeRes)
	}

	out := sessionOutputSummary(ws)
	workerStalls := 0
	if detector != nil {
		workerStalls = detector.StallCount()
	}
	return workerResult{idx: p.workerIdx, session: ws, worktree: wt, branch: br, output: out, err: waitErr, cascadeResult: cascadeRes, stallCount: workerStalls, pooled: pooled}
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

// recentGitLog returns the last n commit subjects from the repo's git log.
func recentGitLog(repoPath string, n int) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "log", "--oneline", fmt.Sprintf("-%d", n))
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", err
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
