# Autonomy Levels & Self-Improvement

The session manager supports graduated autonomous decision-making via `internal/session/autonomy.go`:

| Level | Name | Behavior |
|-------|------|----------|
| 0 | Observe | Log decisions only ("would have done X") |
| 1 | AutoRecover | Auto-restart on transient errors, provider failover |
| 2 | AutoOptimize | Auto-adjust budgets, providers, and rate limits from feedback profiles |
| 3 | FullAutonomy | Auto-launch from roadmap, scale teams, apply config changes |

All decisions are recorded in a JSONL decision log with rationale, inputs, and outcomes. Human overrides are tracked by the HITL subsystem.

## Self-Improvement Subsystems

- **`autonomy.go`**: DecisionLog with 4-level gating — decisions below current level are logged but not executed
- **`autooptimize.go`**: Feedback-driven provider/budget selection using journal-derived performance profiles
- **`autorecovery.go`**: Transient error retry with exponential backoff (connection reset, timeout, rate limit, 429/503), provider failover on persistent failures
- **`contextstore.go`**: Cross-session file conflict detection — prevents concurrent workers from editing the same files
- **`feedback.go`**: Provider/task performance profiling (avg cost, turns, duration, completion rate per task type)
- **`hitl.go`**: Human-in-the-loop metric tracking — manual interventions vs autonomous actions, trend scoring (improving/stable/degrading)
- **`promptcache.go`**: Prompt prefix caching — identifies stable preamble (CLAUDE.md, system prompts) for cost savings across sessions

## Loop Profiles

`LoopProfile` (defined in `internal/session/loop.go`) configures a perpetual planner/worker loop.

| Field | Type | Default | Description |
|---|---|---|---|
| `PlannerProvider` | `Provider` | `""` | Provider used for the planner role |
| `PlannerModel` | `string` | `""` | Model name for the planner |
| `WorkerProvider` | `Provider` | `""` | Provider used for worker sessions |
| `WorkerModel` | `string` | `""` | Model name for workers |
| `VerifierProvider` | `Provider` | `""` | Provider used to run verification |
| `VerifierModel` | `string` | `""` | Model name for the verifier |
| `MaxConcurrentWorkers` | `int` | `0` | Maximum parallel worker sessions |
| `RetryLimit` | `int` | `0` | Maximum retries per task before skipping |
| `VerifyCommands` | `[]string` | `nil` | Shell commands run to verify each task |
| `WorktreePolicy` | `string` | `""` | How git worktrees are allocated per task |
| `PlannerBudgetUSD` | `float64` | `0` | Per-iteration USD budget for the planner |
| `WorkerBudgetUSD` | `float64` | `0` | Per-task USD budget for each worker |
| `VerifierBudgetUSD` | `float64` | `0` | Per-task USD budget for the verifier |
| `EnableReflexion` | `bool` | `false` | Enable reflexion-style self-critique feedback |
| `EnableEpisodicMemory` | `bool` | `false` | Persist task outcomes as episodic memory |
| `EnableCascade` | `bool` | `false` | Enable cascade multi-step task expansion |
| `CascadeConfig` | `*CascadeConfig` | `nil` | Parameters for cascade expansion |
| `EnableUncertainty` | `bool` | `false` | Gate task execution on uncertainty estimates |
| `EnableCurriculum` | `bool` | `false` | Order tasks by difficulty (curriculum learning) |
| `SelfImprovement` | `bool` | `false` | Allow the loop to propose self-improvement tasks |
| `CompactionEnabled` | `bool` | `false` | Enable context compaction for long-running loops |
| `CompactionThreshold` | `int` | `0` | Iterations before compaction is activated |
| `AutoMergeAll` | `bool` | `false` | Bypass path classification and auto-merge any PR whose verify step passes |
| `MaxIterations` | `int` | `0` | Stop after this many iterations (0 = unlimited) |
| `MaxDurationSecs` | `int` | `0` | Stop after this many seconds (0 = unlimited) |

When `AutoMergeAll` is `true`, the loop skips the normal path-classification gate that would otherwise hold risky changes for human review. Instead, any PR whose `VerifyCommands` all pass is merged immediately and the loop continues to the next task. This makes fully unattended self-improvement possible: the planner queues roadmap tasks, workers implement them in isolated worktrees, and every green PR lands on the main branch without a human ever approving it.

```
planner picks task
       │
       ▼
worker executes in worktree
       │
       ▼
VerifyCommands pass?
       │  yes
       ▼
PR created → AutoMergeAll merges
       │
       ▼
loop continues (next task)
```
