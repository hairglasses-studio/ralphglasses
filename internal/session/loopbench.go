package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
	"github.com/hairglasses-studio/ralphglasses/internal/gitutil"
)

// percentile computes the p-th percentile from a pre-sorted slice of float64
// using linear interpolation. Returns 0 for empty input.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := p / 100.0 * float64(len(sorted)-1)
	lower := int(idx)
	upper := lower + 1
	if upper >= len(sorted) {
		return sorted[lower]
	}
	weight := idx - float64(lower)
	return sorted[lower]*(1-weight) + sorted[upper]*weight
}

// ResolveMainRepoPath returns the top-level working directory of the main
// repository. In a worktree, this resolves back to the main checkout.
// In a normal repo or non-git directory, it returns the input path.
func ResolveMainRepoPath(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--git-common-dir")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return dir, nil // not a git dir, return as-is
	}
	commonDir := strings.TrimSpace(string(out))
	if commonDir == ".git" {
		// Normal repo — return top-level
		cmd2 := exec.Command("git", "rev-parse", "--show-toplevel")
		cmd2.Dir = dir
		out2, err2 := cmd2.Output()
		if err2 != nil {
			return dir, nil
		}
		return strings.TrimSpace(string(out2)), nil
	}
	// Worktree: commonDir is absolute path to main repo's .git dir.
	// Main repo root is its parent.
	return filepath.Dir(commonDir), nil
}

// DiffStat tracks the aggregate diff size for an iteration.
type DiffStat struct {
	FilesChanged int `json:"files_changed"`
	Insertions   int `json:"insertions"`
	Deletions    int `json:"deletions"`
}

// LoopObservation is one JSONL record per completed loop iteration,
// capturing timing, cost, outcome, and code impact for regression analysis.
type LoopObservation struct {
	Timestamp        time.Time `json:"ts"`
	LoopID           string    `json:"loop_id"`
	RepoName         string    `json:"repo_name"`
	IterationNumber  int       `json:"iteration"`
	PlannerLatencyMs int64     `json:"planner_latency_ms"`
	WorkerLatencyMs  int64     `json:"worker_latency_ms"`
	VerifyLatencyMs  int64     `json:"verify_latency_ms"`
	TotalLatencyMs   int64     `json:"total_latency_ms"`
	PlannerCostUSD   float64   `json:"planner_cost_usd"`
	WorkerCostUSD    float64   `json:"worker_cost_usd"`
	TotalCostUSD     float64   `json:"total_cost_usd"`
	PlannerProvider  string    `json:"planner_provider"`
	WorkerProvider   string    `json:"worker_provider"`
	PlannerTokensOut int64     `json:"planner_tokens_out"`
	WorkerTokensOut  int64     `json:"worker_tokens_out"`
	Status           string    `json:"status"`
	VerifyPassed     bool      `json:"verify_passed"`
	WorkerCount      int       `json:"worker_count"`
	Error            string    `json:"error,omitempty"`
	FilesChanged     int       `json:"files_changed"`
	LinesAdded       int       `json:"lines_added"`
	LinesRemoved     int       `json:"lines_removed"`
	DiffPaths        []string  `json:"diff_paths,omitempty"`
	DiffSummary      string    `json:"diff_summary,omitempty"`
	TaskType         string    `json:"task_type"`
	TaskTitle        string    `json:"task_title"`
	Mode             string    `json:"mode"`
	Confidence       float64   `json:"confidence"`
	CascadeEscalated bool      `json:"cascade_escalated"`
	CascadeCheapCost float64   `json:"cascade_cheap_cost"`
	DifficultyScore  float64   `json:"difficulty_score"`
	ReflexionApplied bool      `json:"reflexion_applied"`
	EpisodesUsed     int       `json:"episodes_used"`

	PlannerFallback  bool      `json:"planner_fallback,omitempty"`

	// GitDiffStat tracks the diff size for this iteration.
	GitDiffStat *DiffStat `json:"git_diff_stat,omitempty"`

	// PlannerModelUsed is the model ID used by the planner (e.g., "claude-opus-4-6").
	PlannerModelUsed string `json:"planner_model_used,omitempty"`

	// WorkerModelUsed is the model ID used by the worker (e.g., "claude-sonnet-4-6").
	WorkerModelUsed string `json:"worker_model_used,omitempty"`

	// AcceptancePath records how the iteration's output was handled.
	// Values: "auto_merge", "pr", "rejected", "no_change"
	AcceptancePath string `json:"acceptance_path,omitempty"`

	// StallCount is the number of stall events detected during this iteration.
	StallCount int `json:"stall_count,omitempty"`

	// Sub-phase timing (ms) — surfaces where planner/worker time is actually spent.
	PromptBuildMs     int64 `json:"prompt_build_ms,omitempty"`
	ReflexionLookupMs int64 `json:"reflexion_lookup_ms,omitempty"`
	EpisodicLookupMs  int64 `json:"episodic_lookup_ms,omitempty"`
	EnhancementMs     int64 `json:"enhancement_ms,omitempty"`
	AcceptanceMs      int64 `json:"acceptance_ms,omitempty"`
	IdleBetweenMs     int64 `json:"idle_between_ms,omitempty"`
}

// WriteObservation appends a single observation as a JSONL line.
func WriteObservation(path string, obs LoopObservation) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create observation dir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open observation file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(obs)
	if err != nil {
		return fmt.Errorf("marshal observation: %w", err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write observation: %w", err)
	}
	return nil
}

// LoadObservations reads observations from a JSONL file, filtering to those after since.
// Returns nil slice if the file does not exist.
func LoadObservations(path string, since time.Time) ([]LoopObservation, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open observations: %w", err)
	}
	defer f.Close()

	var out []LoopObservation
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var obs LoopObservation
		if err := json.Unmarshal(scanner.Bytes(), &obs); err != nil {
			continue // skip malformed lines
		}
		if !obs.Timestamp.Before(since) {
			out = append(out, obs)
		}
	}
	return out, scanner.Err()
}

// ObservationPath returns the canonical JSONL path for a repo's loop observations.
func ObservationPath(repoPath string) string {
	return filepath.Join(repoPath, ".ralph", "logs", "loop_observations.jsonl")
}

// emitLoopObservation gathers data from the completed iteration and writes an observation.
// It also publishes a LoopIterated event to the bus if available.
func emitLoopObservation(run *LoopRun, index int, m *Manager,
	reflexionApplied bool, episodesUsed int,
	cascadeResults []*CascadeResult, taskDifficulties []TaskDifficulty,
	stallCount int,
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

// LoopVelocity returns useful iterations per hour within the given window.
// An iteration is "useful" if verification passed and files were changed.
func LoopVelocity(observations []LoopObservation, windowHours float64) float64 {
	if windowHours <= 0 {
		return 0
	}
	cutoff := time.Now().Add(-time.Duration(windowHours * float64(time.Hour)))
	useful := 0
	for _, obs := range observations {
		if obs.Timestamp.After(cutoff) && obs.VerifyPassed && obs.FilesChanged > 0 {
			useful++
		}
	}
	return float64(useful) / windowHours
}

// ObservationSummary provides rolling statistics over a time window.
type ObservationSummary struct {
	WindowHours     float64            `json:"window_hours"`
	TotalIterations int                `json:"total_iterations"`
	CompletionRate  float64            `json:"completion_rate"`
	AvgCostPerIter  float64            `json:"avg_cost_per_iter"`
	CostTrend       string             `json:"cost_trend"`      // "decreasing", "stable", "increasing"
	EfficiencyScore float64            `json:"efficiency_score"` // completions per dollar
	CostByProvider  map[string]float64 `json:"cost_by_provider"`
	Velocity        float64            `json:"velocity"` // useful iterations per hour
}

// AggregateObservations computes rolling statistics from loop observations.
func AggregateObservations(observations []LoopObservation, windowHours float64) *ObservationSummary {
	if windowHours <= 0 || len(observations) == 0 {
		return &ObservationSummary{WindowHours: windowHours, CostByProvider: map[string]float64{}}
	}

	cutoff := time.Now().Add(-time.Duration(windowHours * float64(time.Hour)))
	prevCutoff := cutoff.Add(-time.Duration(windowHours * float64(time.Hour)))

	var current, previous []LoopObservation
	for _, obs := range observations {
		if obs.Timestamp.After(cutoff) {
			current = append(current, obs)
		} else if obs.Timestamp.After(prevCutoff) {
			previous = append(previous, obs)
		}
	}

	summary := &ObservationSummary{
		WindowHours:     windowHours,
		TotalIterations: len(current),
		CostByProvider:  make(map[string]float64),
	}

	if len(current) == 0 {
		return summary
	}

	var totalCost float64
	completed := 0
	for _, obs := range current {
		totalCost += obs.TotalCostUSD
		if obs.Status == "idle" {
			completed++
		}
		if obs.PlannerProvider != "" {
			summary.CostByProvider[obs.PlannerProvider] += obs.PlannerCostUSD
		}
		if obs.WorkerProvider != "" {
			summary.CostByProvider[obs.WorkerProvider] += obs.WorkerCostUSD
		}
	}

	summary.CompletionRate = float64(completed) / float64(len(current))
	summary.AvgCostPerIter = totalCost / float64(len(current))

	if totalCost > 0 {
		summary.EfficiencyScore = float64(completed) / totalCost
	}

	summary.Velocity = LoopVelocity(current, windowHours)

	// Compute cost trend by comparing current window to previous window
	if len(previous) > 0 {
		var prevCost float64
		for _, obs := range previous {
			prevCost += obs.TotalCostUSD
		}
		prevAvg := prevCost / float64(len(previous))
		curAvg := summary.AvgCostPerIter

		ratio := curAvg / prevAvg
		switch {
		case ratio < 0.85:
			summary.CostTrend = "decreasing"
		case ratio > 1.15:
			summary.CostTrend = "increasing"
		default:
			summary.CostTrend = "stable"
		}
	} else {
		summary.CostTrend = "stable"
	}

	return summary
}

// IterationSummary aggregates statistics across multiple observations.
type IterationSummary struct {
	TotalIterations   int            `json:"total_iterations"`
	CompletedCount    int            `json:"completed_count"`
	FailedCount       int            `json:"failed_count"`
	TotalStalls       int            `json:"total_stalls"`
	AvgDurationSec    float64        `json:"avg_duration_sec"`
	TotalFilesChanged int            `json:"total_files_changed"`
	TotalInsertions   int            `json:"total_insertions"`
	TotalDeletions    int            `json:"total_deletions"`
	AcceptanceCounts  map[string]int `json:"acceptance_counts"` // "auto_merge" -> N, etc.
	ModelUsage        map[string]int `json:"model_usage"`       // model ID -> count

	// Latency percentiles (seconds).
	LatencyP50 float64 `json:"latency_p50"`
	LatencyP95 float64 `json:"latency_p95"`
	LatencyP99 float64 `json:"latency_p99"`

	// Cost percentiles (USD).
	CostP50 float64 `json:"cost_p50"`
	CostP95 float64 `json:"cost_p95"`
	CostP99 float64 `json:"cost_p99"`
}

// SummarizeObservations computes aggregate statistics from a slice of observations.
func SummarizeObservations(obs []LoopObservation) IterationSummary {
	s := IterationSummary{
		AcceptanceCounts: make(map[string]int),
		ModelUsage:       make(map[string]int),
	}
	if len(obs) == 0 {
		return s
	}

	s.TotalIterations = len(obs)
	var totalDurationMs int64
	latencies := make([]float64, 0, len(obs))
	costs := make([]float64, 0, len(obs))
	for _, o := range obs {
		// Status accounting
		switch o.Status {
		case "idle":
			s.CompletedCount++
		case "failed":
			s.FailedCount++
		}

		// Stall accounting
		s.TotalStalls += o.StallCount

		// Duration
		totalDurationMs += o.TotalLatencyMs
		latencies = append(latencies, float64(o.TotalLatencyMs)/1000.0)
		costs = append(costs, o.TotalCostUSD)

		// Diff stats from DiffStat if present, otherwise from flat fields
		if o.GitDiffStat != nil {
			s.TotalFilesChanged += o.GitDiffStat.FilesChanged
			s.TotalInsertions += o.GitDiffStat.Insertions
			s.TotalDeletions += o.GitDiffStat.Deletions
		} else {
			s.TotalFilesChanged += o.FilesChanged
			s.TotalInsertions += o.LinesAdded
			s.TotalDeletions += o.LinesRemoved
		}

		// Acceptance path
		if o.AcceptancePath != "" {
			s.AcceptanceCounts[o.AcceptancePath]++
		}

		// Model usage
		if o.PlannerModelUsed != "" {
			s.ModelUsage[o.PlannerModelUsed]++
		}
		if o.WorkerModelUsed != "" {
			s.ModelUsage[o.WorkerModelUsed]++
		}
	}

	s.AvgDurationSec = float64(totalDurationMs) / float64(len(obs)) / 1000.0

	// Compute latency percentiles (seconds).
	sort.Float64s(latencies)
	s.LatencyP50 = percentile(latencies, 50)
	s.LatencyP95 = percentile(latencies, 95)
	s.LatencyP99 = percentile(latencies, 99)

	// Compute cost percentiles (USD).
	sort.Float64s(costs)
	s.CostP50 = percentile(costs, 50)
	s.CostP95 = percentile(costs, 95)
	s.CostP99 = percentile(costs, 99)

	return s
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
