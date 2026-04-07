# Autonomy Levels & Self-Improvement

The session manager supports graduated autonomous decision-making via `internal/session/autonomy.go`:

| Level | Name | Behavior |
|-------|------|----------|
| 0 | Observe | Log decisions only ("would have done X") |
| 1 | AutoRecover | Auto-restart on transient errors, provider failover |
| 2 | AutoOptimize | Auto-adjust budgets, providers, and rate limits from feedback profiles; continuous supervisor monitors health and chains R&D cycles |
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

## Supervisor

At Level 2, a background supervisor (`internal/session/supervisor.go`) ticks every 60 seconds, evaluates repo health, and chains R&D cycles without human input.

```
60s tick → HealthMonitor.Assess() → SupervisorDecision → CycleChainer.Launch()
```

**Components**

- **`supervisor.go`** — Main loop. Reads the decision log, fires a health check each tick, and calls the chainer if the decision is `launch`. All decisions are appended to the JSONL audit log with rationale.
- **`health_monitor.go`** — Scores 5 metrics against configurable thresholds. Returns `healthy | degraded | critical`.
- **`cycle_chainer.go`** — Launches the next R&D cycle, attaches lineage metadata (`parent_cycle_id`, `chain_depth`), and enforces the depth cap.

**Enabling**

```bash
# Via MCP
autonomy_level set=2 repo=/path/to/repo

# Via TUI
:autonomy 2
```

**Health Thresholds** (all configurable; defaults shown)

| Metric | Default |
|--------|---------|
| Test coverage | ≥ 80% |
| Build success rate | ≥ 95% |
| Mean cycle cost USD | ≤ 2.00 |
| HITL intervention rate | ≤ 10% |
| Open critical findings | ≤ 3 |

## Productive R&D Signal

Pressure tests and autonomy status surfaces use one composite productivity score rather than treating any completed cycle as useful work.

- Research: 35 points only when the research daemon both writes durable research output and completes at least one queue topic
- Development: 35 points only when loops or cycles produce concrete development output (accepted changes, staged-file work, or accomplished cycle tasks)
- Verification: 20 points only when verifier failures stay at zero
- Anti-stall: 10 points only when the run stays below the no-op plateau threshold and does not converge on noop churn

A run is marked `productive=true` only when the score is at least 80, research and development outputs are both non-zero, verifier failures stay at zero, and the run does not end in a no-op plateau.

Dedup skips, autonomy-gated research rejections, and noop-only loop iterations are reported, but they do not contribute productivity credit.

**Cycle Chaining**

Each launched cycle carries `parent_cycle_id` and `chain_depth` in its metadata. The chainer refuses to start a new cycle if `chain_depth ≥ 10` (hard cap), preventing runaway chains. Lineage is queryable via `cycle_status` and `observation_query`.

**Safety Model**

| Gate | Behavior |
|------|----------|
| Budget | Supervisor checks remaining budget before launch; skips cycle if headroom < worker budget |
| Concurrency | At most 1 supervisor-launched cycle runs at a time per repo |
| Chain depth | Hard cap of 10; chain halts and logs `chain_depth_exceeded` |
| Cooldown | Minimum 5 minutes between launches regardless of tick rate |
| Decision audit | Every tick writes a decision record (action, rationale, metric snapshot) to the JSONL log |

**Monitoring**

```bash
# MCP
supervisor_status repo=/path/to/repo

# Returns: current health score, last decision, active chain depth, next tick ETA
```

## Claude Code Permission Mode Mapping

Our autonomy levels map naturally to Claude Code's permission modes. See [claude-code-autonomy-research.md](claude-code-autonomy-research.md) for full details.

| Autonomy Level | Name | Claude Code Permission Mode | Rationale |
|----------------|------|-----------------------------|-----------|
| 0 | Observe | Plan Mode | Read-only, no modifications — log "would have done X" |
| 1 | AutoRecover | Normal Mode + hooks | Hooks handle transient error retry; human approves risky operations |
| 2 | AutoOptimize | Auto Mode (research preview) | Model-based classifier approves safe actions, blocks risky ones |
| 3 | FullAutonomy | Bypass (`--dangerously-skip-permissions`) | Isolated environments only — containers, VMs, or dedicated machines |

**Safety recommendations by level:**
- **L0-L1**: No special isolation needed. Standard development environment.
- **L2**: Recommended: git commit before starting, budget ceiling set, cloud scheduled task for durability.
- **L3**: Required: container/VM isolation, network restrictions, hard budget + time limits, full audit logging.

**Claude Code features per level:**
- **L0**: `/loop` for monitoring, Plan Mode agents
- **L1**: Hooks for auto-recovery, `--continue` for session resumption
- **L2**: Auto Mode, `/batch` for parallel execution, cloud scheduled tasks, Agent SDK
- **L3**: `--dangerously-skip-permissions`, `--bare` for reproducibility, full Bypass mode

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
