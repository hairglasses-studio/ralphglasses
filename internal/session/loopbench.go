package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
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

	PlannerFallback bool `json:"planner_fallback,omitempty"`

	// GitDiffStat tracks the diff size for this iteration.
	GitDiffStat *DiffStat `json:"git_diff_stat,omitempty"`

	// PlannerModelUsed is the model ID used by the planner (e.g., "claude-opus-4-6").
	PlannerModelUsed string `json:"planner_model_used,omitempty"`

	// WorkerModelUsed is the model ID used by the worker (e.g., "claude-sonnet-4-6").
	WorkerModelUsed string `json:"worker_model_used,omitempty"`

	// AcceptancePath records how the iteration's output was handled.
	// Values: "auto_merge", "pr", "rejected", "no_change"
	AcceptancePath string `json:"acceptance_path,omitempty"`

	// WS11: Acceptance gate tracing fields for diagnosing silent rejections.
	AcceptanceReason      string `json:"acceptance_reason,omitempty"`       // "auto_merged", "pr_created", "no_staged_files", "worker_no_changes"
	StagedFilesCount      int    `json:"staged_files_count,omitempty"`      // files staged after git add (post-exclude)
	AcceptanceSafeCount   int    `json:"acceptance_safe_count,omitempty"`   // paths classified as safe
	AcceptanceReviewCount int    `json:"acceptance_review_count,omitempty"` // paths classified as needing review

	// WorkerEnhancementSource records how the worker prompt was enhanced.
	// Values: "none", "local", "api"
	WorkerEnhancementSource string `json:"worker_enhancement_source,omitempty"`

	// StallCount is the number of stall events detected during this iteration.
	StallCount int `json:"stall_count,omitempty"`

	// NoopSkipped indicates this iteration was skipped due to consecutive no-op detection.
	NoopSkipped bool `json:"noop_skipped,omitempty"`

	// ConsecutiveNoops is the running count of consecutive no-op iterations for this loop.
	ConsecutiveNoops int `json:"consecutive_noops,omitempty"`

	// TurnCount is the total number of LLM turns (planner + workers) for this iteration.
	TurnCount int `json:"turn_count,omitempty"`

	// Sub-phase timing (ms) — surfaces where planner/worker time is actually spent.
	PromptBuildMs     int64 `json:"prompt_build_ms,omitempty"`
	ReflexionLookupMs int64 `json:"reflexion_lookup_ms,omitempty"`
	EpisodicLookupMs  int64 `json:"episodic_lookup_ms,omitempty"`
	EnhancementMs     int64 `json:"enhancement_ms,omitempty"`
	AcceptanceMs      int64 `json:"acceptance_ms,omitempty"`
	IdleBetweenMs     int64 `json:"idle_between_ms,omitempty"`

	// Runtime diagnostics captured at observation time.
	MemoryUsageMB  float64 `json:"memory_usage_mb,omitempty"`
	GoroutineCount int     `json:"goroutine_count,omitempty"`

	// Red-signal provenance keeps repo-owned patch selection from treating
	// branch-local or dirty local state as source-backed repo debt.
	SignalBranches      []string `json:"signal_branches,omitempty"`
	SignalBranchLocal   bool     `json:"signal_branch_local,omitempty"`
	SignalDirtyWorktree bool     `json:"signal_dirty_worktree,omitempty"`
	RemoteMainVerified  bool     `json:"remote_main_verified,omitempty"`
	RedSignalEvidence   string   `json:"red_signal_evidence,omitempty"`
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

// ObservationEligibleForCycle returns whether an observation should influence
// loop self-improvement task generation. Standalone ad-hoc sessions are
// excluded because they are not loop iterations.
func ObservationEligibleForCycle(obs LoopObservation) bool {
	switch obs.Mode {
	case "", "live":
		return ObservationEligibleForRepoPatchQueue(obs)
	default:
		return false
	}
}

// ObservationEligibleForBaseline returns whether an observation should
// influence regression baselines and gates. Standalone ad-hoc sessions are
// excluded, while live and mock loop observations are retained.
func ObservationEligibleForBaseline(obs LoopObservation) bool {
	switch obs.Mode {
	case "", "live", "mock":
		return true
	default:
		return false
	}
}

// ObservationEligibleForRepoPatchQueue returns whether an observation should be
// allowed to create repo-owned repair work. Red signals must carry explicit
// source-backed evidence instead of relying on local branch or worktree state.
func ObservationEligibleForRepoPatchQueue(obs LoopObservation) bool {
	if !ObservationRequiresRedSignalEvidence(obs.Status) {
		return true
	}
	if obs.RemoteMainVerified {
		return true
	}
	switch obs.RedSignalEvidence {
	case "remote_main", "ci", "source_integrity":
		return true
	default:
		return false
	}
}

// ObservationRequiresRedSignalEvidence reports whether a status represents a
// red signal that should not create repo-owned repair work without source-
// backed verification metadata.
func ObservationRequiresRedSignalEvidence(status string) bool {
	switch status {
	case "failed", "regressed", "cycle_failed":
		return true
	default:
		return false
	}
}

func captureObservationRepoState(obs *LoopObservation, repoPath string, worktreePaths []string) {
	if obs == nil {
		return
	}

	branches, dirty := detectObservationRepoState(repoPath, worktreePaths)
	obs.SignalBranches = branches
	obs.SignalDirtyWorktree = dirty
	obs.SignalBranchLocal = observationBranchesIncludeLocal(branches)

	if obs.RemoteMainVerified && obs.RedSignalEvidence == "" {
		obs.RedSignalEvidence = "remote_main"
	}
	if obs.RedSignalEvidence == "remote_main" {
		obs.RemoteMainVerified = true
	}
	if ObservationRequiresRedSignalEvidence(obs.Status) && obs.RedSignalEvidence == "" {
		obs.RedSignalEvidence = "local_only"
	}
}

func detectObservationRepoState(repoPath string, worktreePaths []string) ([]string, bool) {
	paths := observationSignalPaths(repoPath, worktreePaths)
	if len(paths) == 0 {
		return nil, false
	}

	var branches []string
	seenBranches := make(map[string]struct{}, len(paths))
	dirty := false
	for _, path := range paths {
		branch := observationGitBranch(path)
		if branch != "" {
			if _, seen := seenBranches[branch]; !seen {
				seenBranches[branch] = struct{}{}
				branches = append(branches, branch)
			}
		}
		if worktreeIsDirty(path) {
			dirty = true
		}
	}
	sort.Strings(branches)
	return branches, dirty
}

func observationSignalPaths(repoPath string, worktreePaths []string) []string {
	seen := make(map[string]struct{}, len(worktreePaths)+1)
	paths := make([]string, 0, len(worktreePaths)+1)
	addPath := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		paths = append(paths, path)
	}

	addPath(repoPath)
	for _, path := range worktreePaths {
		addPath(path)
	}
	return paths
}

func observationGitBranch(path string) string {
	cmd := exec.Command("git", "-C", path, "rev-parse", "--abbrev-ref", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func observationBranchesIncludeLocal(branches []string) bool {
	for _, branch := range branches {
		switch branch {
		case "", "main", "master":
			continue
		default:
			return true
		}
	}
	return false
}
