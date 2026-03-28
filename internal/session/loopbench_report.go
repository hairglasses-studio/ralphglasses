package session

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/gitutil"
)

// emitLoopObservation gathers data from the completed iteration and writes an observation.
// It also publishes a LoopIterated event to the bus if available.
func emitLoopObservation(run *LoopRun, index int, m *Manager,
	reflexionApplied bool, episodesUsed int,
	cascadeResults []*CascadeResult, taskDifficulties []TaskDifficulty,
	stallCount int,
	noopSkipped bool, consecutiveNoops int,
) {
	run.mu.Lock()
	if index < 0 || index >= len(run.Iterations) {
		run.mu.Unlock()
		return
	}
	iter := run.Iterations[index]
	profile := run.Profile
	loopID := run.ID
	repoName := run.RepoName
	repoPath := run.RepoPath
	run.mu.Unlock()

	obs := LoopObservation{
		Timestamp:        time.Now(),
		LoopID:           loopID,
		RepoName:         repoName,
		IterationNumber:  iter.Number,
		PlannerProvider:  string(profile.PlannerProvider),
		WorkerProvider:   string(profile.WorkerProvider),
		PlannerModelUsed: profile.PlannerModel,
		WorkerModelUsed:  profile.WorkerModel,
		WorkerCount:      len(iter.WorkerSessionIDs),
		Mode:             "live",
	}

	// Timing — per-stage and total
	if iter.PlannerEndedAt != nil {
		obs.PlannerLatencyMs = iter.PlannerEndedAt.Sub(iter.StartedAt).Milliseconds()
	}
	if iter.WorkersEndedAt != nil && iter.PlannerEndedAt != nil {
		obs.WorkerLatencyMs = iter.WorkersEndedAt.Sub(*iter.PlannerEndedAt).Milliseconds()
	}
	if iter.EndedAt != nil {
		obs.TotalLatencyMs = iter.EndedAt.Sub(iter.StartedAt).Milliseconds()
		if iter.WorkersEndedAt != nil {
			obs.VerifyLatencyMs = iter.EndedAt.Sub(*iter.WorkersEndedAt).Milliseconds()
		}
	}

	// Sub-phase timing
	obs.PromptBuildMs = iter.PromptBuildMs
	obs.ReflexionLookupMs = iter.ReflexionLookupMs
	obs.EpisodicLookupMs = iter.EpisodicLookupMs
	obs.EnhancementMs = iter.EnhancementMs
	obs.AcceptanceMs = iter.AcceptanceMs
	obs.IdleBetweenMs = iter.IdleBetweenMs

	// Task classification
	if iter.Task.Title != "" {
		obs.TaskTitle = iter.Task.Title
		obs.TaskType = classifyTask(iter.Task.Title)
	}
	if iter.Task.Source == "fallback" || (len(iter.Tasks) > 0 && iter.Tasks[0].Source == "fallback") {
		obs.PlannerFallback = true
	}

	// Status and verification
	obs.Status = iter.Status
	obs.VerifyPassed = iter.Status != "failed" && iter.Error == ""
	obs.Error = iter.Error

	// Eagerly capture cost from session objects via Manager.Get()
	if plannerSess, ok := m.Get(iter.PlannerSessionID); ok {
		plannerSess.Lock()
		obs.PlannerCostUSD = plannerSess.SpentUSD
		obs.PlannerTokensOut = int64(plannerSess.TurnCount) // proxy until real token tracking
		plannerSess.Unlock()
	}

	var totalWorkerCost float64
	var totalWorkerTokens int64
	for _, wid := range iter.WorkerSessionIDs {
		if wid == "" {
			continue
		}
		if ws, ok := m.Get(wid); ok {
			ws.Lock()
			totalWorkerCost += ws.SpentUSD
			totalWorkerTokens += int64(ws.TurnCount)
			ws.Unlock()
		}
	}
	obs.WorkerCostUSD = totalWorkerCost
	obs.WorkerTokensOut = totalWorkerTokens
	obs.TotalCostUSD = obs.PlannerCostUSD + obs.WorkerCostUSD

	// Git diff stats from worktrees
	for _, wt := range iter.WorktreePaths {
		if wt == "" {
			continue
		}
		files, added, removed := gitDiffStats(wt)
		obs.FilesChanged += files
		obs.LinesAdded += added
		obs.LinesRemoved += removed
	}
	// Populate structured DiffStat from the flat fields.
	if obs.FilesChanged > 0 || obs.LinesAdded > 0 || obs.LinesRemoved > 0 {
		obs.GitDiffStat = &DiffStat{
			FilesChanged: obs.FilesChanged,
			Insertions:   obs.LinesAdded,
			Deletions:    obs.LinesRemoved,
		}
	}

	// Diff path correlation — collect file paths changed across worktrees.
	var allDiffPaths []string
	seen := make(map[string]bool)
	for _, wt := range iter.WorktreePaths {
		if wt == "" {
			continue
		}
		paths, err := gitDiffPathsForWorktree(wt)
		if err != nil {
			continue
		}
		for _, p := range paths {
			if !seen[p] {
				seen[p] = true
				allDiffPaths = append(allDiffPaths, p)
			}
		}
	}
	obs.DiffPaths = allDiffPaths
	obs.DiffSummary = buildDiffSummary(allDiffPaths)

	// WS-B: Stall detection count from worker goroutines.
	obs.StallCount = stallCount

	// No-op iteration detection fields.
	obs.NoopSkipped = noopSkipped
	obs.ConsecutiveNoops = consecutiveNoops

	// Self-learning subsystem fields
	obs.ReflexionApplied = reflexionApplied
	obs.EpisodesUsed = episodesUsed

	// WS3: Cascade routing metrics — aggregate across workers.
	for _, cr := range cascadeResults {
		if cr != nil && cr.Escalated {
			obs.CascadeEscalated = true
			obs.CascadeCheapCost += cr.CheapCostUSD
		}
	}

	// WS5: Average difficulty score across tasks.
	if len(taskDifficulties) > 0 {
		var totalDiff float64
		for _, td := range taskDifficulties {
			totalDiff += td.DifficultyScore
		}
		obs.DifficultyScore = totalDiff / float64(len(taskDifficulties))
	}

	// WS4: Compute confidence from verification and worker signals.
	if obs.VerifyPassed {
		obs.Confidence = 1.0
	} else if obs.Error != "" {
		obs.Confidence = 0.0
	} else {
		obs.Confidence = 0.5
	}

	// WS11: Acceptance gate tracing — populate from iteration fields.
	obs.AcceptanceReason = iter.AcceptanceReason
	obs.StagedFilesCount = iter.StagedFilesCount
	if iter.Acceptance != nil {
		obs.AcceptanceSafeCount = len(iter.Acceptance.SafePaths)
		obs.AcceptanceReviewCount = len(iter.Acceptance.ReviewPaths)
	}

	// Derive acceptance path from the iteration's acceptance result.
	if iter.Acceptance != nil {
		switch {
		case iter.Acceptance.AutoMerged:
			obs.AcceptancePath = "auto_merge"
		case iter.Acceptance.PRCreated:
			obs.AcceptancePath = "pr"
		case iter.Acceptance.Error != "":
			obs.AcceptancePath = "rejected"
		default:
			obs.AcceptancePath = "no_change"
		}
	} else if obs.FilesChanged == 0 {
		obs.AcceptancePath = "no_change"
	}

	// Runtime diagnostics.
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	obs.MemoryUsageMB = float64(memStats.Alloc) / (1024 * 1024)
	obs.GoroutineCount = runtime.NumGoroutine()

	// Write to JSONL
	obsPath := ObservationPath(repoPath)
	if err := WriteObservation(obsPath, obs); err != nil {
		slog.Warn("failed to write loop observation", "path", obsPath, "error", err)
	}

	// Publish event
	if m.bus != nil {
		m.bus.Publish(events.Event{
			Type:     events.LoopIterated,
			RepoName: repoName,
			Data: map[string]any{
				"loop_id":    obs.LoopID,
				"iteration":  obs.IterationNumber,
				"status":     obs.Status,
				"cost_usd":   obs.TotalCostUSD,
				"latency_ms": obs.TotalLatencyMs,
				"task_type":  obs.TaskType,
			},
		})
	}
}

// emitSessionObservation writes a LoopObservation record for a standalone
// session completion (not part of a loop). This ensures fleet_analytics has
// observation data to aggregate even in standalone mode (FINDING-237).
func emitSessionObservation(sess *Session) {
	sess.mu.Lock()
	repoPath := sess.RepoPath
	repoName := sess.RepoName
	status := sess.Status
	provider := string(sess.Provider)
	spentUSD := sess.SpentUSD
	turns := sess.TurnCount
	errMsg := sess.Error
	launchedAt := sess.LaunchedAt
	endedAt := sess.EndedAt
	sessionID := sess.ID
	prompt := sess.Prompt
	sess.mu.Unlock()

	if repoPath == "" {
		return
	}

	// Only record for terminal statuses.
	if !status.IsTerminal() {
		return
	}

	var totalLatencyMs int64
	if endedAt != nil {
		totalLatencyMs = endedAt.Sub(launchedAt).Milliseconds()
	}

	obsStatus := "completed"
	if status == StatusErrored {
		obsStatus = "failed"
	} else if status == StatusStopped {
		obsStatus = "stopped"
	}

	obs := LoopObservation{
		Timestamp:       time.Now(),
		LoopID:          "session:" + sessionID,
		RepoName:        repoName,
		IterationNumber: 1,
		TotalLatencyMs:  totalLatencyMs,
		TotalCostUSD:    spentUSD,
		WorkerProvider:  provider,
		PlannerProvider: provider,
		Status:          obsStatus,
		VerifyPassed:    obsStatus == "completed",
		Error:           errMsg,
		Mode:            "standalone",
		TaskTitle:       truncateStr(prompt, 200),
	}

	// Derive confidence from status.
	switch obsStatus {
	case "completed":
		obs.Confidence = 1.0
	case "failed":
		obs.Confidence = 0.0
	default:
		obs.Confidence = 0.5
	}

	// Map cost to both planner and worker fields so aggregateObservationMetrics
	// can attribute it via either provider field.
	if turns > 0 {
		obs.WorkerCostUSD = spentUSD
		obs.WorkerTokensOut = int64(turns)
	}

	obsPath := ObservationPath(repoPath)
	if err := WriteObservation(obsPath, obs); err != nil {
		slog.Warn("failed to write session observation", "session", sessionID, "path", obsPath, "error", err)
	}
}

// gitDiffStats runs git diff --stat on a worktree and parses the summary line.
func gitDiffStats(worktreePath string) (files, added, removed int) {
	return gitutil.GitDiffStats(worktreePath)
}

// gitDiffPathsForWorktree runs git diff --name-only HEAD in the given directory
// and returns the list of changed file paths.
func gitDiffPathsForWorktree(dir string) ([]string, error) {
	return gitutil.GitDiffPaths(dir)
}

// buildDiffSummary formats a list of diff paths into a human-readable summary.
// Format: "N files: path1, path2, +M more" (shows max 3 paths).
func buildDiffSummary(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	const maxShow = 3
	shown := paths
	if len(shown) > maxShow {
		shown = shown[:maxShow]
	}
	summary := fmt.Sprintf("%d files: %s", len(paths), strings.Join(shown, ", "))
	if len(paths) > maxShow {
		summary += fmt.Sprintf(", +%d more", len(paths)-maxShow)
	}
	return summary
}
