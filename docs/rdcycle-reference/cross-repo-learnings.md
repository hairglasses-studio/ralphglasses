# Cross-Repo Ralph Loop Learnings (2026-03-16)

Source: mesmer, ralphglasses, mcpkit, hg-mcp parallel research

## Fixes Relevant to Claudekit

### 1. Full Environment Cleanup (mesmer discovery)
Three env vars need clearing, not just CLAUDECODE:
```bash
unset CLAUDECODE CLAUDE_CODE_ENABLE_TELEMETRY CLAUDE_CODE_ENTRYPOINT
```

### 2. ANTHROPIC_API_KEY Conflict (ralphglasses discovery)
Claude Code uses OAuth. ANTHROPIC_API_KEY overrides it with stale key → auth failures.
```bash
unset ANTHROPIC_API_KEY
```
Note: claudekit's APISamplingClient intentionally uses API keys for direct calls — this only applies to Claude Code CLI invocations.

## Patterns Worth Adopting

### From mesmer: Pre-flight Validation Script
Before launching a 12h run, validate:
- Binary has expected symbols (strings check)
- Task plan has remaining work
- Disk space > 5GB
- Budget projection fits within cap
- Dry-run mode to test without launching

### From mesmer: Dual-Condition Exit Detection
Prevents false exits. Requires BOTH:
1. Heuristic indicators ≥ 2 (no file changes, tasks all checked, short responses)
2. Explicit model confirmation (EXIT_SIGNAL: true + STATUS: COMPLETE)

### From mesmer: Task Batching (3x speedup)
Batch 2-3 similar tasks per iteration. Single commit per feature set. Config in .ralphrc: BATCH_SIMILAR_TASKS=true, MAX_TASKS_PER_BATCH=3

### From mcpkit: Static Task DAG with LLM Selection
Instead of sequential lists, use dependency DAG. `ReadyTasks()` returns frontier, LLM picks optimal next task. Enables parallel execution.

### From mcpkit: Stuck Detection Module
Monitors execution patterns for repetitive behavior, injects contextual hints. More sophisticated than simple circuit breaker — could enhance claudekit's verify-stuck-loop fix.

### From ralphglasses: Circuit Breaker with 3 Independent Failure Modes
Track separately: no-progress, same-error, permission-denial. Each with own threshold and cooldown. More granular than single-mode breakers.

## Claudekit-Specific: Known Open Issues from Research
1. **Verify task stuck-loop** (Tier 8): mark_done when make check passes with no changes
2. **Duplicate hollow notes**: Add notesWritten flag to skip fallback if HandleNotes already ran
3. **APISamplingClient is unique** — no other repo has this pattern. Consider extracting to shared library.
4. **SetSampler() deferred wiring** — novel pattern. Document as reference implementation.
