package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/hairglasses-studio/ralphglasses/internal/events"
)

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
	TaskType         string    `json:"task_type"`
	TaskTitle        string    `json:"task_title"`
	Mode             string    `json:"mode"`
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
func emitLoopObservation(run *LoopRun, index int, m *Manager) {
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
		Timestamp:       time.Now(),
		LoopID:          loopID,
		RepoName:        repoName,
		IterationNumber: iter.Number,
		PlannerProvider: string(profile.PlannerProvider),
		WorkerProvider:  string(profile.WorkerProvider),
		WorkerCount:     len(iter.WorkerSessionIDs),
		Mode:            "live",
	}

	// Timing
	if iter.EndedAt != nil {
		obs.TotalLatencyMs = iter.EndedAt.Sub(iter.StartedAt).Milliseconds()
	}

	// Task classification
	if iter.Task.Title != "" {
		obs.TaskTitle = iter.Task.Title
		obs.TaskType = classifyTask(iter.Task.Title)
	}

	// Status and verification
	obs.Status = iter.Status
	obs.VerifyPassed = iter.Status == "idle"
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

	// Write to JSONL
	obsPath := ObservationPath(repoPath)
	_ = WriteObservation(obsPath, obs)

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

// gitDiffStats runs git diff --stat on a worktree and parses the summary line.
func gitDiffStats(worktreePath string) (files, added, removed int) {
	cmd := exec.Command("git", "diff", "--stat", "HEAD")
	cmd.Dir = worktreePath
	out, err := cmd.Output()
	if err != nil {
		return 0, 0, 0
	}

	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) == 0 {
		return 0, 0, 0
	}
	// Summary line looks like: " 3 files changed, 10 insertions(+), 5 deletions(-)"
	summary := lines[len(lines)-1]
	for _, part := range strings.Split(summary, ",") {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) < 2 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			continue
		}
		switch {
		case strings.Contains(part, "file"):
			files = n
		case strings.Contains(part, "insertion"):
			added = n
		case strings.Contains(part, "deletion"):
			removed = n
		}
	}
	return files, added, removed
}
