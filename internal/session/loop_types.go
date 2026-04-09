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
	PlannerProvider          Provider       `json:"planner_provider"`
	PlannerModel             string         `json:"planner_model"`
	WorkerProvider           Provider       `json:"worker_provider"`
	WorkerModel              string         `json:"worker_model"`
	VerifierProvider         Provider       `json:"verifier_provider"`
	VerifierModel            string         `json:"verifier_model"`
	MaxConcurrentWorkers     int            `json:"max_concurrent_workers"`
	RetryLimit               int            `json:"retry_limit"`
	VerifyCommands           []string       `json:"verify_commands,omitempty"`
	WorktreePolicy           string         `json:"worktree_policy,omitempty"`
	PlannerBudgetUSD         float64        `json:"planner_budget_usd,omitempty"`
	WorkerBudgetUSD          float64        `json:"worker_budget_usd,omitempty"`
	VerifierBudgetUSD        float64        `json:"verifier_budget_usd,omitempty"`
	EnableReflexion          bool           `json:"enable_reflexion"`
	EnableEpisodicMemory     bool           `json:"enable_episodic_memory"`
	EnableCascade            bool           `json:"enable_cascade"`
	CascadeConfig            *CascadeConfig `json:"cascade_config,omitempty"`
	EnableUncertainty        bool           `json:"enable_uncertainty"`
	EnableCurriculum         bool           `json:"enable_curriculum"`
	SelfImprovement          bool           `json:"self_improvement"`
	CompactionEnabled        bool           `json:"compaction_enabled"`
	CompactionThreshold      int            `json:"compaction_threshold,omitempty"` // iterations before enabling compaction
	AutoMergeAll             bool           `json:"auto_merge_all"`                 // bypass path classification, auto-merge if verify passes
	EnablePlannerEnhancement bool           `json:"enable_planner_enhancement"`     // run prompt enhancement before planner calls
	EnableWorkerEnhancement  bool           `json:"enable_worker_enhancement"`      // run prompt enhancement before worker calls
	MaxIterations            int            `json:"max_iterations,omitempty"`
	MaxDurationSecs          int            `json:"max_duration_secs,omitempty"`
	StallTimeout             time.Duration  `json:"stall_timeout,omitempty"`       // 0 = disabled, default 10min
	HardBudgetCapUSD         float64        `json:"hard_budget_cap_usd,omitempty"` // absolute spend ceiling (0 = disabled)
	NoopPlateauLimit         int            `json:"noop_plateau_limit,omitempty"`  // stop after N consecutive no-op iterations (0 = disabled)
	MaxWorkerTurns           int            `json:"max_worker_turns,omitempty"`    // absolute cap on total iterations (0 = default 20)

	// Auto-CI-fix: when verification fails, automatically generate a fix task
	// from the failure output and route it back to a worker. Capped at
	// MaxAutoFixRetries attempts per iteration (default 2, 0 = disabled).
	AutoFixOnVerifyFail bool `json:"auto_fix_on_verify_fail,omitempty"`
	MaxAutoFixRetries   int  `json:"max_auto_fix_retries,omitempty"` // 0 = disabled, default 2 when AutoFixOnVerifyFail is true
	ReviewPatience      int  `json:"review_patience,omitempty"`      // default 3
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
	Phase0SessionID   string             `json:"phase0_session_id,omitempty"`
	Phase0Output      string             `json:"phase0_output,omitempty"`
	Phase0EndedAt     *time.Time         `json:"phase0_ended_at,omitempty"`
	PlannerSessionID  string             `json:"planner_session_id,omitempty"`
	WorkerSessionID   string             `json:"worker_session_id,omitempty"`  // first/only worker (backwards compat)
	WorkerSessionIDs  []string           `json:"worker_session_ids,omitempty"` // all workers for concurrent execution
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

	// Dedup tracking: tasks filtered out by near-duplicate or content overlap detection.
	SkippedTasks []DedupSkip `json:"skipped_tasks,omitempty"`
}

// LoopRun is persisted state for a perpetual development loop.
type LoopRun struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id,omitempty"`
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
	done   chan struct{}      // closed when RunLoop exits
}

// Lock locks the loop run mutex for external callers.
func (r *LoopRun) Lock() { r.mu.Lock() }

// Unlock unlocks the loop run mutex.
func (r *LoopRun) Unlock() { r.mu.Unlock() }

// DefaultLoopProfile returns the repo-default Codex-only planner/worker setup.
// Keep this aligned with ProviderDefaults(ProviderCodex) so every runtime path
// uses the same canonical Codex default model.
func DefaultLoopProfile() LoopProfile {
	return LoopProfile{
		PlannerProvider:      ProviderCodex,
		PlannerModel:         ProviderDefaults(ProviderCodex),
		WorkerProvider:       ProviderCodex,
		WorkerModel:          ProviderDefaults(ProviderCodex),
		VerifierProvider:     ProviderCodex,
		VerifierModel:        ProviderDefaults(ProviderCodex),
		MaxConcurrentWorkers: 1,
		RetryLimit:           1,
		VerifyCommands:       []string{defaultLoopVerifyCommand},
		WorktreePolicy:       "git",
		EnableCascade:        true,
	}
}

// SelfImprovementProfile returns a profile configured for autonomous self-improvement.
// Codex is now the primary autonomy runtime, with a larger planner model and
// cheaper worker/verifier defaults.
func SelfImprovementProfile() LoopProfile {
	return LoopProfile{
		PlannerProvider:          ProviderCodex,
		PlannerModel:             ProviderDefaults(ProviderCodex),
		WorkerProvider:           ProviderCodex,
		WorkerModel:              ProviderDefaults(ProviderCodex),
		VerifierProvider:         ProviderCodex,
		VerifierModel:            ProviderDefaults(ProviderCodex),
		MaxConcurrentWorkers:     1,
		RetryLimit:               2,
		VerifyCommands:           []string{"./scripts/dev/ci.sh", "go run . selftest --gate"},
		WorktreePolicy:           "git",
		PlannerBudgetUSD:         5.0,
		WorkerBudgetUSD:          15.0,
		EnableReflexion:          true,
		EnableEpisodicMemory:     true,
		EnableUncertainty:        true,
		EnableCurriculum:         true,
		EnableCascade:            false,
		SelfImprovement:          true,
		CompactionEnabled:        true,
		AutoMergeAll:             true,
		EnablePlannerEnhancement: true,
		EnableWorkerEnhancement:  false,
		MaxIterations:            10,
		MaxDurationSecs:          14400,
	}
}

// BudgetOptimizedSelfImprovementProfile returns a self-improvement profile that
// uses cheaper Codex models to maximize iterations per dollar. Per-session
// budgets are scaled from the total budget. For a $100 total budget this yields
// ~$1.50 planner + $3.00 worker per iteration with a hard cap at 95%.
func BudgetOptimizedSelfImprovementProfile(totalBudget float64) LoopProfile {
	if totalBudget <= 0 {
		totalBudget = 100
	}
	return LoopProfile{
		PlannerProvider:          ProviderCodex,
		PlannerModel:             ProviderDefaults(ProviderCodex),
		WorkerProvider:           ProviderCodex,
		WorkerModel:              ProviderDefaults(ProviderCodex),
		VerifierProvider:         ProviderCodex,
		VerifierModel:            ProviderDefaults(ProviderCodex),
		MaxConcurrentWorkers:     1,
		RetryLimit:               2,
		VerifyCommands:           []string{"./scripts/dev/ci.sh", "go run . selftest --gate"},
		WorktreePolicy:           "git",
		PlannerBudgetUSD:         totalBudget * 0.015,
		WorkerBudgetUSD:          totalBudget * 0.030,
		VerifierBudgetUSD:        totalBudget * 0.005,
		HardBudgetCapUSD:         totalBudget * 0.95,
		NoopPlateauLimit:         3,
		EnableReflexion:          true,
		EnableEpisodicMemory:     true,
		EnableUncertainty:        true,
		EnableCurriculum:         true,
		EnableCascade:            true,
		SelfImprovement:          true,
		CompactionEnabled:        true,
		CompactionThreshold:      5,
		AutoMergeAll:             true,
		EnablePlannerEnhancement: true,
		EnableWorkerEnhancement:  false,
		MaxIterations:            int(totalBudget * 1.5), // ~150 at $100
		MaxDurationSecs:          28800,                  // 8 hours
		StallTimeout:             10 * time.Minute,
	}
}

// ResearchLoopProfile returns a budget-optimized profile for the passive research
// daemon. Uses the cheapest models available: Gemini Flash for planning, Claude
// Haiku for execution, and Gemini Flash-Lite for verification. The dailyBudget
// parameter controls per-phase budget allocation.
func ResearchLoopProfile(dailyBudget float64) LoopProfile {
	if dailyBudget <= 0 {
		dailyBudget = 25
	}
	return LoopProfile{
		PlannerProvider:      ProviderGemini,
		PlannerModel:         "gemini-3.1-flash",
		WorkerProvider:       ProviderClaude,
		WorkerModel:          "claude-3-haiku-20250401",
		VerifierProvider:     ProviderGemini,
		VerifierModel:        "gemini-3.1-flash-lite",
		MaxConcurrentWorkers: 3,
		RetryLimit:           1,
		WorktreePolicy:       "none",
		PlannerBudgetUSD:     dailyBudget * 0.10,
		WorkerBudgetUSD:      dailyBudget * 0.60,
		VerifierBudgetUSD:    dailyBudget * 0.05,
		HardBudgetCapUSD:     dailyBudget * 0.95,
		NoopPlateauLimit:     3,
		EnableCascade:        true,
		CompactionEnabled:    true,
		CompactionThreshold:  5,
		MaxIterations:        20,
		MaxDurationSecs:      3600, // 1 hour
		StallTimeout:         5 * time.Minute,
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
