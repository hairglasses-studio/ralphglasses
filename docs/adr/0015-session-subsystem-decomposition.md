# ADR 0015: Session Subsystem Decomposition

## Status

Accepted

## Context

The `internal/session/` package has grown to 406 Go files (192 non-test, 214 test) in a single flat package. This creates several problems:

- **Navigation** -- developers must scan 192 source files to find the right type or function. Prefixed filenames (`cost_engine.go`, `cost_ledger.go`, `cost_model.go`, ...) are a code smell indicating latent sub-packages.
- **Compilation** -- Go compiles packages as a unit. A single 406-file package means every change recompiles everything in the package, even when the change is isolated to one domain (e.g., cost tracking).
- **Testing** -- `go test ./internal/session/` runs all 214 test files regardless of what changed. Test parallelism is limited to a single package, and failure noise from unrelated subsystems slows debugging.
- **Ownership** -- with everything in one package, there is no clear boundary between the cost engine, provider adapters, autonomy system, event log, and persistence layer. Changes in one domain risk unintended coupling to another.
- **12-Factor Agents (F10)** -- the 12-factor agents analysis reinforces "Small, Focused Agents" at the code level: each subsystem should be independently testable, deployable, and reasoned about.

### File inventory by domain

| Domain | Non-test files | Examples |
|--------|---------------|----------|
| Provider adapters | 5 | `providers.go`, `provider_amp.go`, `provider_goose.go`, `provider_crush.go` |
| Cost / budget / FinOps | 18 | `costs.go`, `cost_engine.go`, `budget.go`, `budget_pool.go`, `spend_monitor.go` |
| Context budget / errors | 10 | `context_budget.go`, `context_pruner.go`, `prefetch.go`, `error_context.go` |
| Event log / replay | 6 | `event_log.go`, `journal.go`, `replay.go`, `replay_diff.go`, `snapshot.go` |
| Autonomy / reflexion | 13 | `autonomy.go`, `reflexion.go`, `episodic.go`, `decision_model.go`, `curriculum.go` |
| Store / persistence | 10 | `store.go`, `store_sqlite.go`, `checkpoint.go`, `compaction.go`, `retention.go` |
| Other (core, loop, routing, ...) | 130 | `manager.go`, `types.go`, `loop.go`, `cascade.go`, `supervisor.go`, ... |

The six prefix-clustered domains (provider, cost, context, eventlog, autonomy, store) account for 62 non-test files -- a clean first wave of extraction. The remaining 130 files in "other" include the core session types, the R&D loop engine, cascade routing, supervisor, and team coordination, which can be addressed in subsequent waves.

## Decision

Split `internal/session/` into sub-packages, extracted one domain at a time. The initial target sub-packages are:

### `session/provider`
Provider interface, adapter implementations (Claude, Codex, Gemini, Goose, Amp, Crush), and provider normalization logic.

**Files to migrate:** `providers.go`, `providers_normalize.go`, `provider_amp.go`, `provider_crush.go`, `provider_goose.go`

### `session/cost`
Cost tracking, budget enforcement, FinOps cost models, spend monitoring, budget federation and pooling, rate calculation, and retry budgets.

**Files to migrate:** `costs.go`, `costnorm.go`, `costpredictor.go`, `cost_anomaly.go`, `cost_engine.go`, `cost_events.go`, `cost_ledger.go`, `cost_model.go`, `cost_optimizer.go`, `cost_routing.go`, `budget.go`, `budget_federation.go`, `budget_forecast.go`, `budget_poller.go`, `budget_pool.go`, `spend_monitor.go`, `rate_calc.go`, `retry_budget.go`

### `session/ctxbudget`
Context budget management, context pruning, prefetch hooks, error context enrichment, and token counting. Named `ctxbudget` (not `context`) to avoid shadowing the stdlib `context` package.

**Files to migrate:** `context_budget.go`, `context_pruner.go`, `contextstore.go`, `prefetch.go`, `prefetch_hooks.go`, `error_compactor.go`, `error_context.go`, `errors.go`, `token_counter.go`, `truncation.go`

### `session/eventlog`
Append-only event log (from ADR 0014), event types, journal, replay, replay-diff, and snapshot infrastructure.

**Files to migrate:** `event_log.go`, `journal.go`, `replay.go`, `replay_diff.go`, `replay_export.go`, `snapshot.go`

### `session/autonomy`
Autonomy levels, decision logging, reflexion loops, episodic memory, tiered memory, learning transfer, curriculum, adaptive depth, and signal classification.

**Files to migrate:** `autonomy.go`, `auto_mode.go`, `adaptive_depth.go`, `curriculum.go`, `decision_model.go`, `depth.go`, `episodic.go`, `improvement_metrics.go`, `learning_transfer.go`, `reflexion.go`, `signal_classify.go`, `trajectory_distill.go`, `uncertainty.go`

### `session/store`
Persistence layer: SQLite store, in-memory store, migrations, checkpoint, compaction, retention, log rotation, and audit/metrics JSON.

**Files to migrate:** `store.go`, `store_sqlite.go`, `store_memory.go`, `store_migrate.go`, `checkpoint.go`, `compaction.go`, `retention.go`, `logrotate.go`, `json_audit.go`, `json_metrics.go`

### Migration strategy

Each sub-package is extracted in a separate phase:

1. **Create the sub-package** with a `doc.go` establishing the package identity.
2. **Move types and functions** from the flat session package into the sub-package. Exported symbols keep their names; unexported symbols are promoted to exported where needed.
3. **Add backward-compatible re-exports** in the parent session package: type aliases (`type Provider = provider.Provider`) and thin wrapper functions that delegate to the sub-package. This preserves all existing call sites.
4. **Migrate call sites** in subsequent commits, removing re-exports once no external caller depends on the session-package version.
5. **Delete re-exports** when all callers have been updated.

This phased approach means no commit breaks the build, and each phase can be reviewed and merged independently.

### Ordering

1. `session/provider` -- smallest (5 files), fewest internal dependencies, cleanest cut
2. `session/eventlog` -- self-contained (6 files), only depends on types
3. `session/cost` -- largest (18 files) but has clear boundaries
4. `session/ctxbudget` -- moderate (10 files), some coupling to cost for budget checks
5. `session/autonomy` -- moderate (13 files), depends on event log and cost
6. `session/store` -- extracted last because other sub-packages may need store interfaces

## Consequences

**Positive:**

- Faster incremental compilation: changing `cost_engine.go` only recompiles `session/cost`, not all 406 files
- Faster targeted testing: `go test ./internal/session/cost/` runs only cost-related tests
- Clearer ownership: each sub-package has a focused responsibility and can be reviewed independently
- Reduced coupling: forced to define explicit interfaces between domains instead of reaching into package-private state
- Easier onboarding: new contributors can understand one sub-package at a time
- Aligns with 12-factor agents F10 (Small, Focused Agents) at the code structure level

**Negative:**

- Increased import verbosity during the transition (callers may need to import both `session` and `session/cost`)
- Re-export shims add temporary code that must be cleaned up
- Potential for circular dependencies if domain boundaries are drawn incorrectly (mitigated by the ordering above)
- The 130 "other" files remain in the flat package until subsequent waves

**Mitigations:**

- Backward-compatible re-exports ensure zero breakage during migration
- Each extraction is a self-contained PR that can be reverted independently
- `go vet ./...` and `GOWORK=off go build ./...` gate every phase
- Circular dependency risk is low: the ordering extracts leaf domains first (provider, eventlog) before domains with cross-cutting concerns (autonomy, store)
