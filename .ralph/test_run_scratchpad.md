# Self-Learning Test Run Scratchpad

## Tool Improvement Opportunities

### MCP Tool Gaps
- `loop_start` had no self-learning toggle params — just added (enable_reflexion, enable_episodic_memory, enable_cascade, enable_uncertainty, enable_curriculum)
- No budget_usd param on loop_start — budget enforcement happens at marathon.sh level, not loop level. Consider adding per-loop budget cap.
- Subsystems are re-initialized on every loop_start — no singleton/reuse. If multiple loops share the same repo, they'll load duplicate state. Consider lazy init on Manager.
- CurriculumSorter gets nil for both feedback and episodic — should wire to actual FeedbackAnalyzer and EpisodicMemory when available for better difficulty scoring.
- Cascade defaults MaxCheapBudgetUSD=0.50 which may be too low for real tasks. Plan says $2.00.
- **Hot reload gap**: MCP server runs as a long-lived process (`go run . mcp`). Source changes require server restart. No `--watch` or signal-based reload. This blocked self-learning activation during Run 1 — profile flags were missing because the binary was stale.
- **Profile response should always include Enable flags** even when false, for debuggability. Current `omitempty` hides them, making it hard to verify subsystem activation.

### Observation Pipeline
- **`omitempty` hides zero-valued self-learning fields**: `reflexion_applied=false`, `episodes_used=0`, `cascade_escalated=false`, `confidence=0.0` all omitted from JSONL. Cannot distinguish "not populated" from "populated with zero." Fix: remove `omitempty` from self-learning observation fields.
- Observations written to `.ralph/logs/loop_observations.jsonl` not `.ralph/observations.jsonl` — plan had the wrong path. Update plan/docs.
- `total_cost_usd: 0` despite real API calls. Cost tracking may not be integrated with the loop observation writer. BudgetEnforcer tracks costs separately in `cost_ledger.jsonl`.

### Reflexion
- Reflexion extracted on iter 1 failure — good! But `correction` field is generic: "Ensure all verification commands pass before completing." Not actionable for retry.
- `files_involved` has junk entries (`app.go`, `.mcp.js`, `github.c`) — extraction is parsing worker output too aggressively, pulling partial strings.
- `root_cause` is just the error string (`verify command failed`), not a semantic analysis. Consider parsing verify output for the actual failing command.
- **BUG: `RecentForTask("")` always returns nil**. Called at planner stage before task is known. `toWordSet("")` → empty → nil. Reflexion never injects into planner prompt. Fix: return N most recent when task title is empty.
- Both reflections have `applied=false`. The `applied` flag is never set to `true` because the injection code path never runs (due to the bug above).
- Planner keeps picking the same ROADMAP item (RefreshRepo) because it doesn't see reflexion context telling it the task already failed.

### Episodic Memory
- No episodes yet (expected — no successes). Will validate on iter 2+ or Run 2.

### Cascade Routing
- Cascade didn't trigger on iter 1. Need to verify: does cascade apply to the planner phase or only workers? Worker was Claude/sonnet (the "expensive" provider). Cascade may need Gemini as cheap provider to be on PATH/configured.
- **Gemini CLI not installed** (doctor.sh showed `gemini warn provider CLI missing`). Cascade routing cannot work without a cheap provider binary. This is a blocker.

### Uncertainty
- No confidence field in observation (omitempty hides 0.0). The `ExtractConfidence` function wasn't called visibly. May only run during cascade evaluation.

### Curriculum
- **`difficulty_score: 0.55` populated!** Curriculum sorter is working. Task was `bug_fix` type.
- Only 1 task in this iteration, so sorting has no effect. Need multi-task iterations to see ordering.

---

## Run 1 Notes

### Pre-run
- Had to add MCP tool params + handler wiring before we could even start — this was a missing integration piece.
- Codex CLI not on PATH — DefaultLoopProfile uses codex provider. Had to override to claude/sonnet.

### During Run
- ci.sh exits 1 due to pre-existing bats flake: `marathon.sh fails without ANTHROPIC_API_KEY` — test expects failure but ANTHROPIC_API_KEY is set. All Go tests pass (22/22 E2E). Worker changes are correct but verify marks as failed.
- Planner picks same ROADMAP item both iterations because reflexion context isn't injected (RecentForTask bug).
- Worker on iter 2 correctly says "already returns []error" — the task was already done. Planner doesn't know this because it lacks context about previous iterations.

### During Run (post-fix, loop 78c271b9)
- Iter 1 PASSED: "Propagate RefreshRepo errors to TUI" — ci.sh exit 0
  - difficulty_score=0.575, confidence=1.0, verify_passed=true
  - Episodic memory stored 1 success episode
  - No reflections (correct — no failure)
  - reflexion_applied/episodes_used/cascade_escalated hidden by omitempty (all zero/false)
  - total_cost_usd=0 — cost not tracked in observation (cost tracking gap persists)
  - total_latency_ms=203653 (~3.4 min planner+worker+verify)

- Iter 3 PASSED: "unit test: RefreshErrors warning section" — ci.sh exit 0
  - difficulty_score=0.505 (test < bug_fix 0.575 — correct ordering)
  - confidence=1.0, verify_passed=true
  - episodes_used=2 (growing: iter1=0, iter2=1, iter3=2)
  - task_type=test (correctly classified)
  - Episodic memory retrieval scaling linearly with stored episodes
  - total_cost_usd=0 still (cost tracking gap persists)
  - total_latency_ms=209425 (~3.5 min)

### Observations So Far
- **Episodic memory**: Working well. Linear growth in episodes_used (0→1→2). Jaccard similarity retrieval finding relevant prior tasks.
- **Curriculum**: difficulty_score differentiates task types (test=0.505, bug_fix=0.575). No multi-task iteration to test sorting yet.
- **Confidence**: Always 1.0 (all passing). Need a failure to validate non-1.0 scores.
- **Reflexion**: Still untested in live run (no failures). This is actually good — loop is producing correct code.
- **Cascade**: Not exercisable (Gemini CLI not installed). All tasks go to expensive provider.
- **Planner quality**: Each iteration picks a new, meaningful ROADMAP item. No repeats (reflexion fix working).
- **Planner keeps picking related items**: Iters 1-3 all from the 0.5.1.x error propagation cluster. Good coherence but limits diversity of task types tested.

### Post-run Metrics
- **Iterations**: 6/6 passed (100% completion)
- **Total latency**: 21.5 min (avg 215s/iter = 3.6 min planner+worker+verify)
- **Total cost**: $0 tracked (cost tracking gap — real cost ~$3-6 estimated from API usage)
- **Difficulty scores**: [0.575, 0.575, 0.505, 0.55, 0.55, 0.59] — range 0.505-0.59
- **Episodes used**: [0, 1, 2, 3, 3, 3] — capped at 3, linear growth until cap
- **Confidence**: all 1.0 (no failures to produce lower scores)
- **Task types**: 5x bug_fix, 1x test — planner heavily biases toward error propagation tasks
- **Reflexions**: 0 (no failures to trigger extraction)
- **Cascade**: 0 (Gemini CLI not installed)
- **Files changed per iter**: [1, 1, 1, 0, 1, 3] — mostly single-file changes
- **Planner behavior**: Coherent task selection from ROADMAP 0.5.1.x cluster, no repeats, good prompt quality

### Run 1 Tool Improvement Notes
- **Episodes cap at 3**: `FindSimilar` returns max 3 by default. All 6 episodes stored but only 3 retrieved. Consider configurable retrieval limit.
- **No task type diversity**: Planner picks same ROADMAP cluster (0.5.1.x error propagation). Need to steer toward mixed types for curriculum testing.
- **Worker on iter 4 found existing code**: `files_changed=0` — the feature already existed. Planner didn't check current state before assigning. Consider adding pre-check stage.
- **Latency per-stage not tracked**: `planner_latency_ms=0, worker_latency_ms=0, verify_latency_ms=0` — only total_latency_ms populated. Per-stage timing would help identify bottlenecks.

---

## Run 2 Notes

### During Run
- Iter 1 PASSED: "Propagate watcher.Close() errors" — same task as Run 1 iter 5 (dedup gap). episodes_used=3 (cross-run memory working!)
- Iter 2 FAILED: "Return []error from RefreshRepo()" — task already done. Worker correctly said "no changes needed" but verify hit flaky `TestEdge_LargeInputs/Enhance_large` (1.06s > 1s threshold). Reflexion extracted.
- Iter 3 FAILED: `reflexion_applied=true` — reflexion injection confirmed working! But same flaky test blocked. Task title parsing bug: observation task_title contains full JSON object.
- Iter 4: Retry limit (1) hit, loop stopped.

### Key Findings
- **Cross-run episodic memory**: CONFIRMED. Run 2 iter 1 had episodes_used=3 from Run 1's stored episodes.
- **Reflexion injection**: CONFIRMED. `reflexion_applied=true` on iter 3 (the iteration after the first failure).
- **Reflexion quality**: Still generic. "Ensure all verification commands pass" doesn't help with flaky timing test.
- **Flaky test blocker**: `TestEdge_LargeInputs` timing threshold (1s for 100K input enhance) is environment-dependent. Causes cascading failures unrelated to worker changes.
- **Planner dedup gap**: Planner picks tasks that are already done or were done in Run 1. No awareness of completed work.
- **Task title parsing bug**: When planner returns JSON with nested fields, observation records full JSON as task_title instead of extracting the title field.
- **Difficulty score increases on failure**: 0.575 → 0.6 (iter 3 after 2 failures). Curriculum responding to failure history.

### Post-run Metrics
- **Iterations**: 3 (1 pass, 2 fail = 33% completion)
- **Total latency**: 7.7 min (avg 154s/iter — faster than Run 1's 215s because no-change iters are quick)
- **Reflexions generated**: 2 (1 per failure)
- **Reflexion applied**: 1 (on iter 3)
- **Episodes stored**: 7 total (6 from Run 1 + 1 from Run 2)
- **Episodes used**: all 3 (cross-run retrieval working)
- **Cascade**: still untested (no Gemini CLI)

---

## Run 3 Notes

### During Run
- Iter 1 FAILED: "Return []error from RefreshRepo()" — same task (dedup gap). Flaky `TestEdge_LargeInputs` again. `reflexion_applied=true` (carry-forward from Run 2).
- Iter 2 PASSED: "Display RefreshErrors in repo detail view" — `reflexion_applied=true`, episodes_used=3
- Iter 3 PASSED: "Propagate watcher errors instead of returning nil" — no-change iter, worker correctly found already done
- Iter 4 PASSED: "0.5.2.1: Propagate partial watcher errors" — new task variant!
- Iter 5 PASSED: "0.5.2.1: propagate watcher error instead of return nil" — another variant
- Iter 6 PASSED: "Task 0.5.2.1 is already complete" — task title contains full sentence, not extracted title

### Key Findings
- **Reflexion carry-forward**: All 6 iterations had `reflexion_applied=true` — Run 2's reflections persist into Run 3
- **Episodes saturated**: All iterations `episodes_used=3` (hit retrieval cap). 9 total episodes stored across 3 runs.
- **Retry limit 2 worked**: Loop survived the flaky test (failed iter 1, recovered on iter 2). Retry_limit=1 was too aggressive.
- **Task title extraction degrading**: Later iterations have full sentences or JSON as task_title instead of clean titles
- **Planner dedup still the #1 gap**: Keeps picking already-done tasks from the same ROADMAP cluster

### Post-run Metrics
- **Iterations**: 6 (5 pass, 1 fail = 83% completion)
- **Total latency**: 25.2 min (avg 252s/iter)
- **Reflexions applied**: 6/6 (100% — all inherited from Run 2)
- **Episodes used**: all 3 (saturated)
- **Difficulty scores**: [0.55, 0.575, 0.525, 0.575, 0.575, 0.59]

---

## Cross-Run Analysis

| Metric | Run 1 | Run 2 | Run 3 | Trend |
|--------|-------|-------|-------|-------|
| Total iterations | 6 | 3 | 6 | — |
| Passed | 6 | 1 | 5 | — |
| Failed | 0 | 2 | 1 | Flaky test impact varies |
| Completion rate | 100% | 33% | 83% | Improved with retry_limit=2 |
| Total latency (min) | 21.5 | 7.7 | 25.2 | — |
| Avg latency/iter (s) | 215 | 154 | 252 | No-change iters are faster |
| Avg difficulty score | 0.557 | 0.583 | 0.565 | Stable 0.55-0.59 range |
| Avg episodes used | 2.0 | 3.0 | 3.0 | Saturated at cap |
| Reflexions applied | 0 | 1 | 6 | Accumulates across runs |
| Reflexions generated | 0 | 2 | 1 | Generated on failures |
| Episodes stored | 6 | +1=7 | +5=12 | Growing |
| Cascade escalations | 0 | 0 | 0 | Blocked (no Gemini CLI) |
| Cost tracked ($) | 0 | 0 | 0 | Gap persists |

### Key Conclusions

1. **Episodic memory works end-to-end**: Linear growth (0→1→2→3) in Run 1, saturated at cap in Runs 2-3. Cross-run persistence confirmed.
2. **Reflexion works end-to-end**: Extracted on failure, injected on next iteration, persists across runs. All Run 3 iterations had `reflexion_applied=true`.
3. **Curriculum scoring works**: `difficulty_score` differentiates task types (test=0.505 < bug_fix=0.575). No multi-task iteration to test sorting.
4. **Confidence scoring works**: 1.0 for all passing iterations. 0.0 (hidden by omitempty) for failures.
5. **Cascade routing NOT tested**: Gemini CLI not installed. All tasks go to expensive provider.
6. **Cost tracking broken**: `total_cost_usd=0` across all 15 iterations despite real API calls.

### Top Improvement Priorities

1. **Planner task dedup**: Planner keeps picking already-completed ROADMAP items. Needs access to git log or a "completed tasks" registry.
2. **Flaky test tolerance**: `TestEdge_LargeInputs` timing threshold (1s) is environment-dependent. Either relax threshold or exclude from ci.sh.
3. **Cost tracking pipeline**: Wire BudgetEnforcer's cost_ledger.jsonl into observation writer.
4. **Remove omitempty on self-learning fields**: Can't distinguish "not populated" from "populated with zero."
5. **Episode retrieval cap**: Default `FindSimilar` returns max 3. Consider configurable limit or relevance-weighted retrieval.
6. **Reflexion correction quality**: Corrections are generic ("Ensure all verification commands pass"). Parse verify output for specific failing test/command.
7. **Task title parsing**: Later iterations have full JSON or sentences as task_title instead of clean extracted titles.
8. **Per-stage latency**: Only `total_latency_ms` populated; planner/worker/verify per-stage timing all 0.
