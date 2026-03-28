package session

import (
	"context"
	"strings"
	"sync"
	"time"
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
	EnableReflexion      bool     `json:"enable_reflexion"`
	EnableEpisodicMemory bool     `json:"enable_episodic_memory"`
	EnableCascade        bool           `json:"enable_cascade"`
	CascadeConfig        *CascadeConfig `json:"cascade_config,omitempty"`
	EnableUncertainty    bool     `json:"enable_uncertainty"`
	EnableCurriculum     bool     `json:"enable_curriculum"`
	SelfImprovement      bool     `json:"self_improvement"`
	CompactionEnabled    bool     `json:"compaction_enabled"`
	CompactionThreshold  int      `json:"compaction_threshold,omitempty"` // iterations before enabling compaction
	AutoMergeAll         bool     `json:"auto_merge_all"`                // bypass path classification, auto-merge if verify passes
	EnablePlannerEnhancement bool  `json:"enable_planner_enhancement"` // run prompt enhancement before planner calls
	EnableWorkerEnhancement  bool  `json:"enable_worker_enhancement"`  // run prompt enhancement before worker calls
	MaxIterations        int           `json:"max_iterations,omitempty"`
	MaxDurationSecs      int           `json:"max_duration_secs,omitempty"`
	StallTimeout         time.Duration `json:"stall_timeout,omitempty"`      // 0 = disabled, default 10min
	HardBudgetCapUSD     float64       `json:"hard_budget_cap_usd,omitempty"` // absolute spend ceiling (0 = disabled)
	NoopPlateauLimit     int           `json:"noop_plateau_limit,omitempty"`  // stop after N consecutive no-op iterations (0 = disabled)
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
	Acceptance        *AcceptanceResult  `json:"acceptance,omitempty"`
	StartedAt         time.Time          `json:"started_at"`
	EndedAt           *time.Time         `json:"ended_at,omitempty"`
	PlannerEndedAt    *time.Time         `json:"planner_ended_at,omitempty"`
	WorkersEndedAt    *time.Time         `json:"workers_ended_at,omitempty"`

	// Sub-phase timing (milliseconds) — surfaces where time is actually spent.
	PromptBuildMs     int64 `json:"prompt_build_ms,omitempty"`
	ReflexionLookupMs int64 `json:"reflexion_lookup_ms,omitempty"`
	EpisodicLookupMs  int64 `json:"episodic_lookup_ms,omitempty"`
	EnhancementMs     int64 `json:"enhancement_ms,omitempty"`
	WorktreeSetupMs   int64 `json:"worktree_setup_ms,omitempty"`
	AcceptanceMs      int64 `json:"acceptance_ms,omitempty"`
	IdleBetweenMs     int64 `json:"idle_between_ms,omitempty"` // gap from previous iteration's EndedAt

	// WS11: Acceptance gate tracing fields for diagnosing silent rejections.
	AcceptanceReason string `json:"acceptance_reason,omitempty"` // "auto_merged", "pr_created", "no_staged_files", "worker_no_changes"
	StagedFilesCount int    `json:"staged_files_count,omitempty"`
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
	Deadline   *time.Time      `json:"deadline,omitempty"`

	Paused bool `json:"paused,omitempty"`

	mu     sync.Mutex
	cancel context.CancelFunc // set by RunLoop; called by StopLoop
	done   chan struct{}       // closed when RunLoop exits
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

// SelfImprovementProfile returns a profile configured for autonomous self-improvement.
// Serial execution (1 worker), all self-learning enabled, ci.sh + selftest --gate verify.
func SelfImprovementProfile() LoopProfile {
	return LoopProfile{
		PlannerProvider:      ProviderClaude,
		PlannerModel:         "claude-opus-4-6", // opus for deep research/planning
		WorkerProvider:       ProviderClaude,
		WorkerModel:          "claude-sonnet-4-6", // sonnet for execution (cost-effective)
		VerifierProvider:     ProviderClaude,
		VerifierModel:        "claude-sonnet-4-6",
		MaxConcurrentWorkers: 1,
		RetryLimit:           2,
		VerifyCommands:       []string{"./scripts/dev/ci.sh", "go run . selftest --gate"},
		WorktreePolicy:       "git",
		PlannerBudgetUSD:     5.0,  // opus planner needs higher budget
		WorkerBudgetUSD:      15.0, // sonnet worker budget
		EnableReflexion:      true,
		EnableEpisodicMemory: true,
		EnableUncertainty:    true,
		EnableCurriculum:     true,
		EnableCascade:        false,
		SelfImprovement:      true,
		CompactionEnabled:    true,
		AutoMergeAll:         true,  // unattended: auto-merge when ci.sh passes
		EnablePlannerEnhancement: true,  // opus planner benefits from enhanced prompts
		EnableWorkerEnhancement:  false, // worker prompts already well-structured by planner
		MaxIterations:        10,
		MaxDurationSecs:      14400, // 4 hours
	}
}

// BudgetOptimizedSelfImprovementProfile returns a self-improvement profile that
// uses Sonnet-only (no Opus) to maximize iterations per dollar. Per-session
// budgets are scaled from the total budget. For a $100 total budget this yields
// ~$1.50 planner + $3.00 worker per iteration with a hard cap at 95%.
func BudgetOptimizedSelfImprovementProfile(totalBudget float64) LoopProfile {
	if totalBudget <= 0 {
		totalBudget = 100
	}
	return LoopProfile{
		PlannerProvider:      ProviderClaude,
		PlannerModel:         "claude-sonnet-4-6",
		WorkerProvider:       ProviderClaude,
		WorkerModel:          "claude-sonnet-4-6",
		VerifierProvider:     ProviderClaude,
		VerifierModel:        "claude-sonnet-4-6",
		MaxConcurrentWorkers: 1,
		RetryLimit:           2,
		VerifyCommands:       []string{"./scripts/dev/ci.sh", "go run . selftest --gate"},
		WorktreePolicy:       "git",
		PlannerBudgetUSD:     totalBudget * 0.015, // $1.50 per planner call at $100
		WorkerBudgetUSD:      totalBudget * 0.030, // $3.00 per worker call at $100
		VerifierBudgetUSD:    totalBudget * 0.005, // $0.50 per verifier call at $100
		HardBudgetCapUSD:     totalBudget * 0.95,  // hard stop at 95%, preserves 5% buffer
		NoopPlateauLimit:     3,
		EnableReflexion:      true,
		EnableEpisodicMemory: true,
		EnableUncertainty:    true,
		EnableCurriculum:     true,
		EnableCascade:        false,
		SelfImprovement:      true,
		CompactionEnabled:    true,
		CompactionThreshold:  5,
		AutoMergeAll:         true,
		EnablePlannerEnhancement: false,
		EnableWorkerEnhancement:  false,
		MaxIterations:        int(totalBudget * 1.5), // ~150 at $100
		MaxDurationSecs:      28800,                   // 8 hours
		StallTimeout:         10 * time.Minute,
	}
}

// enhanceResult holds the enhanced prompt plus metadata for training loop capture.
type enhanceResult struct {
	prompt   string
	source   string // "local", "llm", "none"
	preScore int    // 0-100 quality score before enhancement
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
