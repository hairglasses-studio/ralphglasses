# Self-Learning Subsystems

Ralph's perpetual development loop includes 5 self-learning subsystems that improve task execution across iterations and runs.

## Subsystem Overview

| Subsystem | Purpose | Key File | State File |
|-----------|---------|----------|------------|
| **Reflexion** | Extract corrections from failures, inject into future prompts | `internal/session/reflexion.go` | `.ralph/reflections.jsonl` |
| **Episodic Memory** | Store successful approaches, retrieve similar examples | `internal/session/episodic.go` | `.ralph/episodes.jsonl` |
| **Cascade Routing** | Try cheap provider first, escalate to expensive if needed | `internal/session/cascade.go` | `.ralph/cascade_results.jsonl` |
| **Uncertainty** | Confidence scoring from verification and hedging signals | `internal/session/loopbench.go` | (inline in observations) |
| **Curriculum** | Sort tasks by difficulty (easy first), score complexity | `internal/session/curriculum.go` | (inline in observations) |

## Activation

All subsystems are opt-in via `loop_start` MCP tool parameters:

```
ralphglasses_loop_start repo=myrepo \
  enable_reflexion=true \
  enable_episodic_memory=true \
  enable_cascade=true \
  enable_uncertainty=true \
  enable_curriculum=true \
  budget_usd=20
```

Subsystems are singleton-initialized on first `loop_start` and reused across subsequent calls for the same repo. State persists across runs in `.ralph/` JSONL files.

## How Each Subsystem Works

### Reflexion

On iteration failure:
1. `ExtractReflection()` parses worker/verify output for `--- FAIL:` test names, compile errors, and error patterns
2. Stores a `Reflection` with `failure_mode`, `root_cause`, `correction`, and `files_involved`
3. On next iteration, `RecentForTask()` retrieves relevant reflections by keyword overlap
4. Injected into both planner prompt (5 most recent) and worker prompt (3 task-specific)

### Episodic Memory

On iteration success:
1. `RecordSuccess()` stores a `JournalEntry` as an episode
2. On future iterations, `FindSimilar()` retrieves top-k episodes by Jaccard similarity + recency bonus
3. `FormatExamples()` injects "here's how a similar task succeeded" context into prompts
4. Default retrieval limit: 5 episodes (configurable via `DefaultK` on `EpisodicMemory`)

### Cascade Routing

For each worker task:
1. `ShouldCascade()` checks if the task type should try the cheap provider
2. Launches cheap provider (default: Gemini) with budget/turn limits
3. `EvaluateCheapResult()` checks verification, confidence, and hedging language
4. If confidence < threshold (default 0.7) or verify fails, escalates to expensive provider (default: Claude)

**Prerequisite**: Gemini CLI must be installed and `GEMINI_API_KEY` set. See [PROVIDER-SETUP.md](PROVIDER-SETUP.md).

### Curriculum Scoring

5-signal weighted difficulty score (0.0-1.0):
1. **Task type** (weight 0.15): test=0.2, docs=0.15, bug_fix=0.45, refactor=0.65, feature=0.7
2. **Historical success rate** (weight 0.30): from FeedbackAnalyzer profile
3. **Prompt complexity** (weight 0.25): word count + breaking-change keywords
4. **Episodic evidence** (weight 0.20): average turns from similar episodes
5. **Keyword indicators** (weight 0.10): "simple"=0.15, "complex"=0.85

`ShouldDecompose()` returns true when difficulty > 0.8 and success rate < 0.5.

## Observation Pipeline

Per-iteration metrics are written to `.ralph/logs/loop_observations.jsonl`:

```json
{
  "loop_id": "abc123",
  "iteration": 1,
  "planner_latency_ms": 45000,
  "worker_latency_ms": 120000,
  "verify_latency_ms": 38000,
  "total_latency_ms": 203000,
  "total_cost_usd": 0.45,
  "confidence": 1.0,
  "difficulty_score": 0.575,
  "reflexion_applied": true,
  "episodes_used": 3,
  "cascade_escalated": false,
  "verify_passed": true,
  "task_type": "bug_fix",
  "task_title": "Propagate RefreshRepo errors to TUI"
}
```

All self-learning fields are always present (no `omitempty`) so zero values are distinguishable from missing data.

## Validation Results (3 Live Runs)

| Metric | Run 1 | Run 2 | Run 3 |
|--------|-------|-------|-------|
| Iterations | 6 | 3 | 6 |
| Completion rate | 100% | 33% | 83% |
| Avg difficulty score | 0.557 | 0.583 | 0.565 |
| Avg episodes used | 2.0 | 3.0 | 3.0 |
| Reflexions applied | 0 | 1 | 6 |
| Episodes stored | 6 | +1=7 | +5=12 |

### Key Findings

1. **Episodic memory**: Linear growth (0->1->2->3) in Run 1, cross-run persistence confirmed in Run 2-3
2. **Reflexion**: Extracted on failure, injected on next iteration, persists across runs
3. **Curriculum**: `difficulty_score` differentiates task types (test=0.505 < bug_fix=0.575)
4. **Confidence**: 1.0 for passing, 0.0 for failures, 0.5 for ambiguous
5. **Cascade**: Requires Gemini CLI installation (not tested in validation runs)

### Planner Dedup

The planner prompt includes:
- **Completed tasks** list from previous iterations (prevents repeating done work)
- **Recent git commits** from the repo (avoids tasks already merged)
- **Diversity steering** to avoid clustering on the same task type

### Known Limitations

- **Cost tracking**: `Session.SpentUSD` depends on provider stream events populating `CostUSD` — some providers don't report cost in real-time
- **Cascade**: Requires Gemini CLI on PATH; without it, all tasks go to the expensive provider
- **Episode retrieval**: Jaccard similarity is keyword-based; semantically similar but differently-worded tasks may not match
