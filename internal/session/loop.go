package session

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/repofiles"
)

// StartLoop registers a new loop run for a repo.
func (m *Manager) StartLoop(_ context.Context, repoPath string, profile LoopProfile) (*LoopRun, error) {
	if strings.TrimSpace(repoPath) == "" {
		return nil, ErrRepoPathRequired
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

	if profile.MaxDurationSecs > 0 {
		d := time.Now().Add(time.Duration(profile.MaxDurationSecs) * time.Second)
		run.Deadline = &d
	}

	// Opportunistic cleanup of stale loop worktrees (best-effort).
	if _, err := CleanupStaleWorktrees(repoPath, 24*time.Hour); err != nil {
		slog.Warn("failed to cleanup stale worktrees", "repo", repoPath, "error", err)
	}

	m.mu.Lock()
	m.loops[run.ID] = run
	m.mu.Unlock()

	m.PersistLoop(run)
	return run, nil
}

// RunLoop drives a loop to completion by calling StepLoop repeatedly until
// max iterations, duration limit, retry limit, or stop signal is reached.
// It runs synchronously — callers should launch it in a goroutine if needed.
func (m *Manager) RunLoop(ctx context.Context, id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}

	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})

	run.mu.Lock()
	run.cancel = cancel
	run.done = done
	run.mu.Unlock()

	defer func() {
		cancel()
		close(done)
	}()

	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		run.mu.Lock()
		paused := run.Paused
		run.mu.Unlock()
		if paused {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
				continue
			}
		}
		err := m.StepLoop(ctx, id)
		if err != nil {
			run.mu.Lock()
			status := run.Status
			run.mu.Unlock()
			if status == "completed" || status == "stopped" {
				return nil
			}
			return err
		}
	}
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
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}

	run.mu.Lock()
	run.Status = "stopped"
	run.UpdatedAt = time.Now()
	repoPath := run.RepoPath
	cancelFn := run.cancel
	doneCh := run.done
	run.mu.Unlock()

	// Cancel the RunLoop context and wait for it to exit.
	if cancelFn != nil {
		cancelFn()
	}
	if doneCh != nil {
		<-doneCh
	}

	m.PersistLoop(run)
	if err := CleanupLoopWorktrees(repoPath, id); err != nil {
		slog.Warn("failed to cleanup loop worktrees", "loop", id, "repo", repoPath, "error", err)
	}
	return nil
}

// PauseLoop pauses auto-advance for a running loop.
func (m *Manager) PauseLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}
	run.mu.Lock()
	run.Paused = true
	run.UpdatedAt = time.Now()
	run.mu.Unlock()
	m.PersistLoop(run)
	return nil
}

// ResumeLoop resumes auto-advance for a paused loop.
func (m *Manager) ResumeLoop(id string) error {
	run, ok := m.GetLoop(id)
	if !ok {
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}
	run.mu.Lock()
	run.Paused = false
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
		return fmt.Errorf("loop %s: %w", id, ErrLoopNotFound)
	}

	run.mu.Lock()
	if run.Status == "stopped" {
		run.mu.Unlock()
		return fmt.Errorf("loop %s: %w", id, ErrLoopStopped)
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
	if run.Profile.MaxIterations > 0 && len(run.Iterations) >= run.Profile.MaxIterations {
		run.Status = "completed"
		run.mu.Unlock()
		m.PersistLoop(run)
		return fmt.Errorf("loop %s reached max iterations (%d)", id, run.Profile.MaxIterations)
	}
	if run.Deadline != nil && time.Now().After(*run.Deadline) {
		run.Status = "completed"
		run.mu.Unlock()
		m.PersistLoop(run)
		return fmt.Errorf("loop %s exceeded duration limit", id)
	}
	if converged, reason := detectConvergence(run.Iterations); converged {
		run.Status = "converged"
		run.LastError = reason
		run.mu.Unlock()
		m.PersistLoop(run)
		return fmt.Errorf("loop %s converged (%s): %w", id, reason, ErrLoopConverged)
	}

	// Snapshot previous iterations for planner dedup (while still under lock).
	prevIterations := make([]LoopIteration, len(run.Iterations))
	copy(prevIterations, run.Iterations)
	currentRunID := run.ID

	// Measure gap from previous iteration's end to this iteration's start.
	var idleBetweenMs int64
	if n := len(run.Iterations); n > 0 {
		if prev := run.Iterations[n-1].EndedAt; prev != nil {
			idleBetweenMs = time.Since(*prev).Milliseconds()
		}
	}

	iteration := LoopIteration{
		Number:        len(run.Iterations) + 1,
		Status:        "planning",
		StartedAt:     time.Now(),
		IdleBetweenMs: idleBetweenMs,
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

	// Self-test recursion guard: prevent infinite self-test loops.
	if IsSelfTestTarget(repoPath) {
		if err := RecursionGuard(); err != nil {
			return m.failLoopIteration(run, index, fmt.Errorf("self-test safety: %w", err))
		}
	}

	numWorkers := profile.MaxConcurrentWorkers
	if numWorkers <= 0 {
		numWorkers = 1
	}

	// Cross-run dedup: inject completed task titles from prior loop runs so the
	// planner avoids re-proposing tasks already done in previous loop instances.
	for _, prior := range m.ListLoops() {
		if prior.ID == currentRunID || prior.RepoPath != repoPath {
			continue
		}
		for _, iter := range prior.Iterations {
			if iter.Status != "failed" && iter.Task.Title != "" {
				prevIterations = append(prevIterations, LoopIteration{
					Status: iter.Status,
					Task:   iter.Task,
				})
			}
		}
	}

	t0 := time.Now()
	plannerPrompt, err := buildLoopPlannerPromptN(repoPath, numWorkers, prevIterations)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("build planner prompt: %w", err))
	}
	promptBuildMs := time.Since(t0).Milliseconds()

	// WS1: Inject reflexion context from previous failures into planner prompt.
	var reflexionApplied bool
	t1 := time.Now()
	if m.reflexion != nil && profile.EnableReflexion {
		if refs := m.reflexion.RecentForTask("", 5); len(refs) > 0 {
			if formatted := m.reflexion.FormatForPrompt(refs); formatted != "" {
				plannerPrompt += "\n\n" + formatted
				reflexionApplied = true
			}
		}
	}
	reflexionMs := time.Since(t1).Milliseconds()

	// WS2: Inject episodic examples of successful approaches into planner prompt.
	var episodesUsed int
	t2 := time.Now()
	if m.episodic != nil && profile.EnableEpisodicMemory {
		episodes := m.episodic.FindSimilar("", "", 0)
		if len(episodes) > 0 {
			if formatted := m.episodic.FormatExamples(episodes); formatted != "" {
				plannerPrompt += "\n\n" + formatted
				episodesUsed = len(episodes)
			}
		}
	}
	episodicMs := time.Since(t2).Milliseconds()

	// Enhance planner prompt for the planner's target provider
	var plannerEnhance enhanceResult
	t3 := time.Now()
	if m.Enhancer != nil && profile.EnablePlannerEnhancement {
		plannerEnhance = m.enhanceForProvider(ctx, plannerPrompt, profile.PlannerProvider)
		plannerPrompt = plannerEnhance.prompt
	} else {
		plannerEnhance = enhanceResult{prompt: plannerPrompt, source: "none", preScore: 0}
	}
	enhancementMs := time.Since(t3).Milliseconds()

	// Record sub-phase timing on the iteration.
	m.updateLoopIteration(run, index, "planning", func(iter *LoopIteration, loop *LoopRun) {
		iter.PromptBuildMs = promptBuildMs
		iter.ReflexionLookupMs = reflexionMs
		iter.EpisodicLookupMs = episodicMs
		iter.EnhancementMs = enhancementMs
	})

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
	plannerSession.EnhancementSource = plannerEnhance.source
	plannerSession.EnhancementPreScore = plannerEnhance.preScore

	m.updateLoopIteration(run, index, "planning", func(iter *LoopIteration, loop *LoopRun) {
		iter.PlannerSessionID = plannerSession.ID
	})

	if err := m.waitForSession(ctx, plannerSession); err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("planner session failed: %w", err))
	}

	plannerDone := time.Now()
	m.updateLoopIteration(run, index, "planning", func(iter *LoopIteration, loop *LoopRun) {
		iter.PlannerEndedAt = &plannerDone
	})

	tasks, plannerOutput, err := plannerTasksFromSession(plannerSession, numWorkers)
	if err != nil {
		return m.failLoopIteration(run, index, fmt.Errorf("parse planner output: %w", err))
	}
	if len(tasks) == 0 {
		return m.failLoopIteration(run, index, errors.New("planner returned no valid tasks"))
	}

	// Retry if planner returned freeform text instead of JSON.
	if len(tasks) > 0 && tasks[0].Source == "fallback" {
		retryPrompt := fmt.Sprintf("Your previous response was not valid JSON. Here is what you said:\n\n%s\n\nRespond with ONLY a JSON object: {\"title\":\"...\",\"prompt\":\"...\"}", plannerOutput)
		retryOpts := LaunchOptions{
			SessionName:  fmt.Sprintf("loop-plan-%s-%03d-retry", run.RepoName, iteration.Number),
			Provider:     profile.PlannerProvider,
			RepoPath:     repoPath,
			Prompt:       retryPrompt,
			Model:        profile.PlannerModel,
			MaxBudgetUSD: profile.PlannerBudgetUSD,
		}
		if retrySess, retryErr := m.launchWorkflowSession(ctx, retryOpts); retryErr == nil {
			if waitErr := m.waitForSession(ctx, retrySess); waitErr == nil {
				retryTasks, retryOutput, retryParseErr := plannerTasksFromSession(retrySess, numWorkers)
				if retryParseErr == nil && len(retryTasks) > 0 && retryTasks[0].Source != "fallback" {
					tasks = retryTasks
					plannerOutput = retryOutput
				}
			}
		}
	}

	// Near-duplicate task filtering: reject tasks whose titles are too similar
	// to already-completed work (exact match or Jaccard similarity >= 0.8).
	var completedForDedup []string
	for _, iter := range prevIterations {
		if iter.Status != "failed" && iter.Task.Title != "" {
			completedForDedup = append(completedForDedup, iter.Task.Title)
		}
	}
	if len(completedForDedup) > 0 {
		tasks = filterDuplicateTasks(tasks, completedForDedup, DefaultSimilarityThreshold)
		if len(tasks) == 0 {
			return m.failLoopIteration(run, index, errors.New("all planner tasks were near-duplicates of completed work"))
		}
	}

	// WS5: Sort tasks by estimated difficulty (easy first) and score them.
	var taskDifficulties []TaskDifficulty
	if m.curriculum != nil && profile.EnableCurriculum {
		tasks = m.curriculum.SortTasks(tasks)
		taskDifficulties = make([]TaskDifficulty, len(tasks))
		for i, t := range tasks {
			taskDifficulties[i] = m.curriculum.ScoreTask(t)
		}
	}

	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		iter.Task = tasks[0] // backwards compat: first task
		iter.Tasks = tasks
		iter.PlannerOutput = plannerOutput
	})

	// Fan out workers in parallel, each in their own worktree
	// WS3: Determine which tasks should try cheap provider first.
	useCascade := m.cascade != nil && profile.EnableCascade

	resultCh := make(chan workerResult, len(tasks))
	for i, task := range tasks {
		go func(workerIdx int, t LoopTask) {
			defer func() {
				if r := recover(); r != nil {
					resultCh <- workerResult{idx: workerIdx, err: fmt.Errorf("worker goroutine panicked: %v", r)}
				}
			}()
			resultCh <- m.runWorkerTask(workerParams{
				ctx:        ctx,
				run:        run,
				iteration:  iteration,
				workerIdx:  workerIdx,
				task:       t,
				profile:    profile,
				repoPath:   repoPath,
				useCascade: useCascade,
			})
		}(i, task)
	}

	// Collect results
	workerSessionIDs := make([]string, len(tasks))
	workerWorktrees := make([]string, len(tasks))
	workerOutputs := make([]string, len(tasks))
	var workerErrs []string
	var firstWorktree, firstBranch string
	var cascadeResults []*CascadeResult // WS3: cascade outcomes per worker
	var totalStallCount int             // WS-B: accumulated stall events across all workers

	workerCollectTimeout := time.After(15 * time.Minute)
	collected := 0
	for collected < len(tasks) {
		select {
		case res := <-resultCh:
			collected++
			if res.session != nil {
				workerSessionIDs[res.idx] = res.session.ID
			}
			workerWorktrees[res.idx] = res.worktree
			workerOutputs[res.idx] = res.output
			totalStallCount += res.stallCount // WS-B
			if res.err != nil {
				workerErrs = append(workerErrs, fmt.Sprintf("worker %d: %s", res.idx, res.err))
			}
			if res.idx == 0 {
				firstWorktree = res.worktree
				firstBranch = res.branch
			}
			if res.cascadeResult != nil {
				cascadeResults = append(cascadeResults, res.cascadeResult)
			}
		case <-workerCollectTimeout:
			workerErrs = append(workerErrs, fmt.Sprintf("timed out waiting for %d/%d workers", len(tasks)-collected, len(tasks)))
			collected = len(tasks)
		case <-ctx.Done():
			workerErrs = append(workerErrs, fmt.Sprintf("context cancelled waiting for workers: %v", ctx.Err()))
			collected = len(tasks)
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

	workersDone := time.Now()
	m.updateLoopIteration(run, index, "executing", func(iter *LoopIteration, loop *LoopRun) {
		iter.WorkersEndedAt = &workersDone
	})

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

			// WS1: Extract reflection from failed iteration for future retries.
			if m.reflexion != nil && profile.EnableReflexion {
				iterSnap := run.iterationsSnapshot()[index]
				if ref := m.reflexion.ExtractReflection(run.ID, iterSnap); ref != nil {
					ref.Applied = false
					m.reflexion.Store(*ref)
				}
			}

			emitLoopObservation(run, index, m,
				reflexionApplied, episodesUsed, cascadeResults, taskDifficulties, totalStallCount)
			m.PersistLoop(run)
			if err := writeLoopJournal(run, run.Iterations[index]); err != nil {
		slog.Warn("failed to write loop journal", "loop", run.ID, "error", err)
	}
			return verErr
		}
	}

	// Forbidden-path diff gate: if this is a self-test target, check for
	// modifications to protected files and require human review.
	postVerifyStatus := "idle"
	if IsSelfTestTarget(repoPath) {
		for _, wt := range workerWorktrees {
			if wt == "" {
				continue
			}
			diffPaths, diffErr := gitDiffPaths(wt)
			if diffErr == nil && len(diffPaths) > 0 {
				_, needsReview := ClassifyDiffPaths(diffPaths)
				if len(needsReview) > 0 {
					postVerifyStatus = "pending_review"
					break
				}
			}
		}
	}

	run.updateLoopAfterVerification(index, allVerification, postVerifyStatus, "")

	// Self-improvement acceptance gate: classify changes and route.
	if profile.SelfImprovement && postVerifyStatus == "idle" {
		accStart := time.Now()
		result, accErr := m.handleSelfImprovementAcceptance(ctx, run, index, workerWorktrees)
		if accErr != nil {
			// Log but don't fail — changes stay in worktree for manual handling.
			run.mu.Lock()
			if index < len(run.Iterations) {
				run.Iterations[index].Error = "acceptance: " + accErr.Error()
			}
			run.mu.Unlock()
		}
		if result != nil {
			run.mu.Lock()
			if index < len(run.Iterations) {
				run.Iterations[index].Acceptance = result
			}
			run.mu.Unlock()
			if result.AutoMerged && m.bus != nil {
				m.bus.Publish(events.Event{
					Type:     events.SelfImproveMerged,
					RepoName: run.RepoName,
					Data: map[string]any{
						"loop_id":    run.ID,
						"iteration":  index,
						"safe_paths": result.SafePaths,
					},
				})
			} else if result.PRCreated && m.bus != nil {
				m.bus.Publish(events.Event{
					Type:     events.SelfImprovePR,
					RepoName: run.RepoName,
					Data: map[string]any{
						"loop_id":      run.ID,
						"iteration":    index,
						"pr_url":       result.PRURL,
						"review_paths": result.ReviewPaths,
					},
				})
			}
		}
		m.updateLoopIteration(run, index, "", func(iter *LoopIteration, loop *LoopRun) {
			iter.AcceptanceMs = time.Since(accStart).Milliseconds()
		})
	}

	// WS2: Record successful iteration as episode for future retrieval.
	if m.episodic != nil && profile.EnableEpisodicMemory {
		iterSnap := run.iterationsSnapshot()[index]
		journal := JournalEntry{
			Timestamp: time.Now(),
			SessionID: iterSnap.WorkerSessionID,
			Provider:  string(profile.WorkerProvider),
			RepoName:  run.RepoName,
			Model:     profile.WorkerModel,
			TaskFocus: iterSnap.Task.Title,
			Worked:    []string{iterSnap.Task.Title},
		}
		m.episodic.RecordSuccess(journal)
	}

	emitLoopObservation(run, index, m,
		reflexionApplied, episodesUsed, cascadeResults, taskDifficulties, totalStallCount)

	// Feed cost sample to CostPredictor if wired.
	if m.costPredictor != nil {
		run.mu.Lock()
		iter := run.Iterations[index]
		provider := string(run.Profile.WorkerProvider)
		taskType := classifyTask(iter.Task.Title)
		run.mu.Unlock()
		// Gather total cost from session objects.
		var totalCost float64
		if ps, ok := m.Get(iter.PlannerSessionID); ok {
			ps.Lock()
			totalCost += ps.SpentUSD
			ps.Unlock()
		}
		for _, wid := range iter.WorkerSessionIDs {
			if ws, ok := m.Get(wid); ok {
				ws.Lock()
				totalCost += ws.SpentUSD
				ws.Unlock()
			}
		}
		m.costPredictor.Record(CostObservation{
			TaskType: taskType,
			Provider: provider,
			CostUSD:  totalCost,
		})
	}

	m.PersistLoop(run)
	if err := writeLoopJournal(run, run.Iterations[index]); err != nil {
		slog.Warn("failed to write loop journal", "loop", run.ID, "error", err)
	}
	return nil
}

func (m *Manager) failLoopIteration(run *LoopRun, index int, err error) error {
	run.updateLoopAfterVerification(index, run.iterationVerification(index), "failed", err.Error())
	m.PersistLoop(run)
	if err := writeLoopJournal(run, run.iterationsSnapshot()[index]); err != nil {
		slog.Warn("failed to write loop journal", "loop", run.ID, "error", err)
	}
	return err
}

func (m *Manager) updateLoopIteration(run *LoopRun, index int, status string, mutate func(*LoopIteration, *LoopRun)) {
	run.mu.Lock()
	defer run.mu.Unlock()

	if index < 0 || index >= len(run.Iterations) {
		return
	}
	if status != "" {
		run.Iterations[index].Status = status
	}
	if mutate != nil {
		mutate(&run.Iterations[index], run)
	}
	run.Status = "running"
	run.UpdatedAt = time.Now()
}

