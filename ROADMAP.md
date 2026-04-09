# Ralphglasses Roadmap

Command-and-control TUI + bootable thin client for parallel multi-LLM agent fleets.

**Last updated:** 2026-04-08
**Codebase:** 73 packages, 222 total MCP tools (218 grouped + 4 management), 19 TUI views
**Status:** 1,143 tasks, 503 complete (44.0%), 640 remaining
**Key deps:** Go 1.26.1, mcp-go v0.45.0, bubbletea v1.3.10, anthropic-sdk-go v1.27.1
**Autonomy target:** Level 3 — fully autonomous fleet operation with self-improvement, self-healing, self-optimizing

## Core Deliverables

### Deliverable 1: `ralphglasses` Go Binary
Cross-platform Unix TUI (k9s-style) built with Charmbracelet (BubbleTea + Lip Gloss).
Manages multi-session, multi-provider LLM loops from any terminal.

### Deliverable 2: Bootable Linux Thin Client
Featherweight, low-graphics bootable Linux (Ubuntu 24.04-based) that boots into i3 + the ralphglasses TUI.
Supports multi-monitor (7-display, dual-NVIDIA-GPU) and autoboot/cron operation.

---

## Quick Wins (Phase 0.9) `[NEW]`

Immediately-actionable items derived from R&D cycle findings. Each is <30 minutes, no dependencies, high impact.

> **Parallel workstreams:** All items independent. Target: complete in 1-2 loop iterations.

- [x] **QW-1** — Fix JSON response format enforcement (25.7% retry rate, "not valid json" pattern seen 26 times across 15 cycles) `P0` `S`
  - File: `internal/session/loop_worker.go` — add JSON schema validation + retry with format reminder
  - **Acceptance:** JSON parse retry rate < 5%

- [x] **QW-2** — Enable cascade routing by default (code exists in `internal/session/cascade.go`, fleet audit shows NOT configured) `P0` `S`
  - File: `.ralphrc` default + `internal/session/manager.go` — set `CASCADE_ENABLED=true` in defaults
  - **Acceptance:** New sessions use cascade routing without explicit config

- [x] **QW-3** — Cap worker turns at 20 to prevent runaway sessions (FINDING-160: signal:killed, 3rd cycle recurrence) `P0` `S`
  - File: `internal/session/loop.go` — add `MaxWorkerTurns` with default 20
  - **Acceptance:** Sessions terminate cleanly at turn limit instead of being killed

- [x] **QW-4** — Fix prompt_analyze score inflation (FINDING-240: scores cluster at 8-9/10 regardless of quality) `P1` `S`
  - File: `internal/enhancer/scoring.go` — lowered baselines (30/25), added trivial-prompt penalties, missing-structure penalties; strict weighted average with no coherence bonus
  - **Acceptance:** Score distribution spans 3-9 range on test corpus — verified via `TestScore_TrivialPromptInflation`, `TestScore_DistributionSpan`, `TestScoringCalibration`, `TestScore_WeakDimensionsDragOverall`

- [x] **QW-5** — Fix prompt_enhance stage skipping transparency (FINDING-243: stages silently skipped) `P1` `S`
  - File: `internal/enhancer/pipeline.go` — add `SkippedStages` field to result, log skip reasons
  - **Acceptance:** Enhanced result includes list of skipped stages with reasons

- [x] **QW-6** — Fix loop_gates zero-baseline bug (FINDING-226/238: baseline zero-init, 2nd cycle recurrence) `P0` `S`
  - File: `internal/session/loop_gates.go` — ensure baseline save errors propagate, initialize from first observation
  - **Acceptance:** `loop_gates` returns meaningful deltas on first run after baseline

- [x] **QW-7** — Fix snapshot path saving to claudekit path (FINDING-148/268: 4th cycle recurrence) `P1` `S`
  - File: `internal/session/snapshot.go` — update path resolution to use ralphglasses project root
  - **Acceptance:** Snapshots save to `.ralph/snapshots/` not `claudekit/` path

- [x] **QW-8** — Fix budget params silently ignored in session_launch (FINDING-258/261) `P0` `S`
  - File: `internal/mcpserver/tools_session.go` — wire budget_usd and max_turns params through to LaunchOptions
  - **Acceptance:** `session_launch budget_usd=5.0` actually enforces $5 budget

- [x] **QW-9** — Persist autonomy level changes across restarts (FINDING-257) `P1` `S`
  - File: `internal/session/autooptimize.go` — write autonomy level to `.ralph/autonomy.json`
  - **Acceptance:** `autonomy_level` survives process restart

- [x] **QW-10** — Fix relevance scoring flat at 0.5 for all results (research-audit FINDING) `P1` `M`
  - File: `internal/roadmap/research.go` — `weightedRelevance()` combines Jaccard + coverage + star-boost
  - **Acceptance:** Relevance scores vary meaningfully (stddev > 0.15)

- [x] **QW-11** — Clean phantom fleet work (73% stale, 109 phantom "001" repo entries) `P1` `S`
  - File: `internal/fleet/coordinator.go` — add stale task reaper, validate repo paths on submission
  - **Acceptance:** `fleet_status` shows 0 phantom entries after cleanup

- [x] **QW-12** — Fix improvement_patterns.json rules always null `P2` `S`
  - File: `internal/session/reflexion.go` — rule extraction from positive/negative patterns already working
  - **Acceptance:** After 5+ cycles, `rules` array contains at least 1 learned rule

## Whiteclaw Migration Autonomy Notes `[NEW]`

Immediate roadmap notes for improving the perpetual autonomous development cycle while the whiteclaw migration program expands across the fleet.

- [x] **WM-1** — Add a tranche checkpoint emitter that writes docs-side migration updates after each completed wave instead of relying on manual session summaries `P0` `M`
  - Target: `internal/roadmap/` + roadmap export surface
  - Shipped via `ralphglasses_roadmap_export format=checkpoint` with repo, component, verification, and next-wave sections for docs-side checkpoint stubs
  - **Acceptance:** a completed tranche can emit a docs checkpoint stub with repo, component, verification, and next-wave data

- [ ] **WM-2** — Add GitHub capability probing before publish or repo-creation tasks so loops know the difference between connector install, CLI auth, push rights, and repo-creation rights `P0` `M`
  - Target: `internal/session/` + `internal/roadmap/`
  - **Acceptance:** planner can block or reroute publish/create tasks when auth or app capability is missing
  - Field note (2026-04-08): root-shell auth looked blocked while the real operator lane (`hg` over SSH) could fetch and push, so capability probes need to test the actual publish identity instead of one ambient shell context

- [ ] **WM-3** — Add a component-level migration ledger input so loops can plan from `source artifact -> target repo -> verification` instead of repo-only backlog slices `P0` `M`
  - Target: `internal/roadmap/` + docs ingest
  - **Acceptance:** roadmap tooling can ingest a machine-readable whiteclaw component map and rank next tranches from it
  - Field note (2026-04-08): `docs/projects/agent-parity/whiteclaw-component-migration-map.json` is now the first real fixture this ingest path should consume

- [ ] **WM-4** — Add existing-equivalent detection before proposing new repos; only create a new repo when no existing repo can coherently own the migrated surface `P1` `S`
  - Target: `internal/roadmap/` repo-classification heuristics
  - **Acceptance:** planner records why an existing repo was selected or why no viable owner existed

- [ ] **WM-5** — Prefer clean publish lanes via dedicated worktrees when canonical repos are dirty or ahead/behind, instead of mixing tranche commits into unstable checkouts `P1` `M`
  - Target: `internal/session/` + operator workflow docs
  - **Acceptance:** publish-oriented loops can select a safe worktree branch strategy automatically
  - Field note (2026-04-08): `surfacekit` and `docs` both required clean publish clones because the operator checkouts were dirty and one checkout was also ahead/behind `origin/main`

- [ ] **WM-6** — Keep a rolling next-tranche queue and auto-promote the next wave immediately after the prior wave verifies cleanly `P1` `M`
  - Target: `internal/roadmap/` + session loop selection
  - **Acceptance:** completing one tranche updates the ranked next-tranche backlog without manual reseeding
  - Field note (2026-04-08): the live queue advanced from docs-side ledger work to `surfacekit` contradiction evidence and then to `.github` reusable workflow rollout without re-running a fresh fleet scan

---

## Autonomous Tranche Delivery Notes [NEW]

Immediate roadmap notes captured from the Jellyfin ecosystem rollout so the perpetual autonomous build loop can patch its own delivery workflow instead of repeating operator-side mistakes.

- [ ] **ATD-1** — Add a publish-lane planner that selects `dirty checkout -> clean worktree -> detached mainline push` automatically when the canonical repo has unrelated edits `P0` `M`
  - Target: `internal/session/` + `internal/roadmap/`
  - **Acceptance:** publish tasks can choose a safe worktree strategy without manual operator intervention

- [ ] **ATD-2** — Add a durable auth bootstrap probe that verifies `gh` keyring login, SSH key presence, `gh auth setup-git`, and org push rights before any repo push or repo-create task `P0` `M`
  - Target: `internal/session/` + `internal/roadmap/`
  - **Acceptance:** planner can distinguish "not logged in", "SSH not registered", "org visible but push denied", and "fully publishable"

- [ ] **ATD-3** — Add host-override checkpoint capture so loops persist runtime discoveries like occupied ports, image-source fallbacks, and remote mount findings into reusable docs and machine-readable patch artifacts `P0` `M`
  - Target: `internal/roadmap/` + docs export + autobuild patch feed
  - **Acceptance:** a completed tranche emits both human docs updates and a machine-readable host-override record

- [ ] **ATD-4** — Add wrapper-first rollout heuristics so loops prefer building bootstrap, preflight, health, backup, restore, and unit-install surfaces before expanding service count `P1` `M`
  - Target: `internal/roadmap/` planning heuristics
  - **Acceptance:** multi-service deployment plans rank operational control-plane work ahead of lower-leverage optional integrations

- [ ] **ATD-5** — Add tranche receipt emission after every successful push, recording repo, commit SHA, verification commands, blockers cleared, and ranked next actions `P1` `S`
  - Target: `internal/roadmap/` + docs checkpoint integration
  - **Acceptance:** each tranche can leave a compact machine-ingestable receipt for the next autonomous loop

- [ ] **ATD-6** — Add automatic docs-sync suggestions when implementation changes introduce new operator-facing commands, ports, env vars, or recovery flows `P1` `M`
  - Target: `internal/roadmap/` + repo diff analysis
  - **Acceptance:** loops flag doc debt before publish when command surfaces or runbooks have drifted

- [ ] **ATD-7** — Add environment-secret boundary detection so loops keep implementing non-secret tranches while correctly deferring only the secret-gated surfaces such as VPN credentials or service API keys `P1` `M`
  - Target: `internal/session/` planning + blockers model
  - **Acceptance:** a missing secret blocks only the dependent tranche instead of freezing the full roadmap

- [x] **ATD-8** — Prioritize remote-main-reproduced commit-gate failures ahead of broader roadmap breadth so autobuild can restore a green publish lane before taking more feature work `P0` `S`
  - Target: `internal/repofiles/` + `internal/session/` + autobuild tranche selection notes
  - Shipped by repairing scaffold/test contract drift and aligning stale Gemini write expectations to the native `.gemini/agents/*.md` surface on top of current `main`
  - **Acceptance:** source-backed red commit-gate regressions are fixed and recorded before the next breadth tranche begins

- [ ] **ATD-9** — Add alternate-env validation heuristics so every new operational script is tested against a temp env file and non-default data roots before publish `P0` `M`
  - Target: `internal/roadmap/` tranche verification + autobuild patch feed
  - **Acceptance:** rollout tranches that add backup, restore, maintenance, or other host-touching scripts emit proof that `--env-file` and relocated roots behave correctly in isolation

- [ ] **ATD-10** — Add artifact-integrity gates before any retention pruning or restore recommendation `P0` `M`
  - Target: `internal/session/` execution + tranche receipt model
  - **Acceptance:** loops verify generated archives or equivalent recovery artifacts before pruning older generations or documenting restore readiness

- [x] **ATD-11** — Add a tracked-temp-artifact gate to repo verification so stray temp outputs and placeholder files fail fast before longer commit checks run `P0` `S`
  - Target: `scripts/dev/` + autobuild patch queue memory
  - Shipped via `scripts/dev/check-tracked-artifacts.sh`, `scripts/dev/test_tracked_artifacts.sh`, and commit-gate wiring in `scripts/dev/ci.sh` and `scripts/dev/pre-commit`
  - **Acceptance:** repo-owned verification names offending tracked artifact paths and fails deterministically before deeper CI stages continue

- [x] **ATD-12** — Repair generated provider-role projection drift immediately once the gate reproduces it on current `main` so the publish lane returns to green before the queue advances `P0` `S`
  - Target: `.claude/agents/` + `.gemini/agents/` + autobuild tranche sequencing
  - Shipped by regenerating provider role projections from `.agents/roles/*.json` after `scripts/sync-provider-roles.py --check` failed on current `main`
  - **Acceptance:** `python3 scripts/sync-provider-roles.py --check` and full `scripts/dev/ci.sh` pass on current `main` after the resync
- [ ] **ATD-13** — Add overlay-risk detection for automations that can hide local state, such as remote mounts or generated mirror directories `P1` `M`
  - Target: `internal/roadmap/` safety heuristics + operator prompt generation
  - **Acceptance:** plans default to explicit opt-in flags or guard rails whenever an automation would shadow non-empty local paths

- [x] **ATD-14** — Add medium-width TUI layout harness checks so nested frame chrome, help panels, and multi-select tables cannot exceed terminal width unnoticed `P1` `S`
  - Target: `internal/tui/` renderer contracts + teatest coverage
  - Shipped by truncating top-frame chrome to terminal width, wrapping help entries to the remaining line budget, accounting for multi-select prefixes in table width math, and adding medium-width Teatest assertions
  - **Acceptance:** medium-width overview and help renders stay within terminal width and deterministic tests fail on future overflow regressions

## Perpetual Development Cycle Notes `[NEW]`

Operator continuity notes captured from the Hyprland persistence rollout. These are future autopatch and autobuild follow-ups so desktop iteration can keep terminal state alive while still landing verified tranche commits continuously.

- [ ] **PDC-1** — Add a desktop destructive-action classifier so planner loops distinguish `safe_reload` from `explicit_restart` when Hyprland, launchers, or other session-bearing UI surfaces are involved `P0` `M`
  - Target: `internal/session/` planner prompts + `internal/roadmap/` task annotations
  - **Acceptance:** desktop tasks touching reload/restart flows default to safe reload lanes unless a hard restart is explicitly requested

- [ ] **PDC-2** — Add tmux continuity preflight checks to roadmap and autobuild execution: main session presence, TPM bootstrap, resurrect/continuum availability, and last persistence-health result `P0` `S`
  - Target: `internal/session/` + `internal/roadmap/`
  - **Acceptance:** loops fail fast on "persistence configured but not operational" before they schedule risky reload or restart work

- [ ] **PDC-3** — Emit verified tranche checkpoints after each commit with tests run, files touched, publish outcome, and next-tranche seed so long-running loops can resume without reconstructing session context `P1` `M`
  - Target: `internal/roadmap/` + checkpoint/journal surfaces
  - **Acceptance:** every completed tranche can produce a resumable machine-readable checkpoint entry

- [ ] **PDC-4** — Probe publish-lane capabilities before promising "push between tranches": SSH auth, connector write access, branch protection, divergence from `main`, and dirty-worktree risk `P0` `M`
  - Target: `internal/session/` publish orchestration
  - **Acceptance:** planner reports whether publish can happen via SSH, GitHub app, clean worktree, or is blocked before tranche work begins
  - Field note (2026-04-08): publish preflight also needs to classify which local identity has auth and whether the current checkout is a safe commit lane or just a source of files for a clean clone

- [ ] **PDC-5** — Prefer clean worktree publish lanes for tranche commits when live repos are dirty, then mirror the landed tranche diff back into the operator checkout without overwriting unrelated edits `P1` `M`
  - Target: `internal/session/` + operator workflow docs
  - **Acceptance:** autonomous publish flows stop depending on committing from dirty working trees
  - Field note (2026-04-08): the same fallback should cover bare-repo operator layouts like `ralphglasses`, where a normal clean clone is simpler than trying to publish from the live checkout

- [ ] **PDC-6** — Add desktop continuity regression fixtures to autobuild smoke suites: safe reload must not restart Hyprshell, dropdown terminals must not kill tmux sessions, and tmux persistence health must pass before restart lanes execute `P1` `M`
  - Target: `internal/roadmap/` + future autobuild smoke harness
  - **Acceptance:** future autonomous desktop patches catch session-destroying regressions before merge
---

## Phase 0: Foundation (COMPLETE)

- [x] Go module (`github.com/hairglasses-studio/ralphglasses`)
- [x] Cobra CLI with `--scan-path` flag
- [x] Discovery engine — scan for `.ralph/` and `.ralphrc` repos
- [x] Model layer — parsers for status.json, progress.json, circuit breaker, .ralphrc
- [x] Process manager — launch/stop/pause ralph loops via os/exec with process groups
- [x] File watcher — fsnotify with 2s polling fallback
- [x] Log streamer — tail `.ralph/live.log`
- [x] MCP server — 22 core-group tools + 196 additional grouped tools via deferred loading, plus 4 management tools (222 total across 30 tool groups)
- [x] Standalone MCP binary (`cmd/ralphglasses-mcp/`)
- [x] TUI shell — BubbleTea app with k9s-style keymap
- [x] TUI views — overview table, repo detail, log stream, config editor, help
- [x] TUI components — sortable table, breadcrumb, status bar, notifications
- [x] Styles package — Lip Gloss theme (isolated to avoid import cycles)
- [x] Marathon launcher script (`marathon.sh`)
- [x] Discovery package — `internal/discovery/` scans for `.ralph/` and `.ralphrc` repos with context support `[reconciled 2026-03-26]`
- [x] Repo files package — `internal/repofiles/` validates and optimizes repo configuration files `[reconciled 2026-03-26]`

## Phase 0.5: Critical Fixes

Pre-requisite fixes for existing bugs and silent failures. No new features. All items are independent and can be worked in parallel.

> **Parallel workstreams:** All 0.5.x items are independent. No blockers between them.

### 0.5.1 — Silent error suppression in RefreshRepo (COMPLETE)
- [x] 0.5.1.1 — Return `[]error` from `RefreshRepo()` in `internal/model/status.go:49-54` instead of discarding with `_ =`
- [x] 0.5.1.2 — Propagate errors to TUI layer: emit `RefreshErrorMsg` with repo path and error details
- [x] 0.5.1.3 — Display parse errors in repo detail view status bar (non-blocking, yellow warning)
- [x] 0.5.1.4 — Add unit test: corrupt status.json -> RefreshRepo returns error, not silent zero-value
- **Acceptance:** parse errors in `.ralph/` files visible to user, not silently dropped

### 0.5.2 — Watcher error handling (COMPLETE)
- [x] 0.5.2.1 — Replace `return nil` on watcher error (`process/watcher.go:47-48`) with error propagation
- [x] 0.5.2.2 — Emit `WatcherErrorMsg` to TUI when fsnotify errors occur
- [x] 0.5.2.3 — Auto-fallback: on watcher error, switch to polling mode and notify user
- [x] 0.5.2.4 — Add exponential backoff on repeated watcher failures (max 30s)
- **Acceptance:** watcher failures visible in TUI, automatic fallback to polling

### 0.5.3 — Process reaper exit status (COMPLETE)
- [x] 0.5.3.1 — Capture `cmd.Wait()` error in `process/manager.go:59` goroutine
- [x] 0.5.3.2 — Parse exit code: distinguish crash (non-zero) from clean exit (0)
- [x] 0.5.3.3 — Emit `ProcessExitMsg{RepoPath, ExitCode, Error}` to TUI
- [x] 0.5.3.4 — Update repo status to "crashed" or "stopped" based on exit code
- [x] 0.5.3.5 — Add unit test: simulate ralph crash, assert TUI receives crash notification
- **Acceptance:** TUI correctly reports ralph crash vs clean stop

### 0.5.4 — Getpgid fallback safety (COMPLETE)
- [x] 0.5.4.1 — Log warning when `Getpgid` fails in `manager.go:78-82` (currently silent fallback to PID-only signal)
- [x] 0.5.4.2 — Track child PIDs: on process launch, record PID + all child PIDs if available
- [x] 0.5.4.3 — Fallback kill sequence: SIGTERM to PID -> wait 5s -> SIGTERM to known children -> wait 5s -> SIGKILL
- [x] 0.5.4.4 — Post-stop audit: check for orphaned processes matching `ralph_loop` pattern
- **Acceptance:** `Stop()` reliably kills all child processes, no orphans

### 0.5.5 — Distro path mismatch
- [x] 0.5.5.1 — Align `hw-detect.service` ExecStart path with Dockerfile install location (`/usr/local/bin/`)
- [x] 0.5.5.2 — Add path consistency check to `distro/Makefile`: validate service files reference correct paths
- [x] 0.5.5.3 — Document path conventions in `distro/README.md`: scripts -> `/usr/local/bin/`, configs -> `/etc/ralphglasses/` `P2` `S`
- **Acceptance:** `hw-detect.service` starts successfully on first boot

### 0.5.6 — Grub AMD iGPU fallback
- [x] 0.5.6.1 — Add GRUB menuentry for AMD iGPU boot: `nomodeset` removed, `amdgpu.dc=1` enabled `P2` `M`
- [x] 0.5.6.2 — Add GRUB menuentry for headless/serial console boot `P2` `S`
- [x] 0.5.6.3 — Set GRUB timeout to 5s (allow human intervention on boot failure) `P2` `S`
- [x] 0.5.6.4 — Add `grub.cfg` validation to CI: parse all menuentry blocks, verify kernel image paths exist `P2` `M`
- **Acceptance:** system boots on AMD iGPU when NVIDIA unavailable

### 0.5.7 — Hardcoded version string (COMPLETE)
- [x] 0.5.7.1 — Define `var Version = "dev"` in `internal/version/version.go`
- [x] 0.5.7.2 — Replace hardcoded `"0.1.0"` in `cmd/mcp.go:19` and `cmd/ralphglasses-mcp/main.go:22`
- [x] 0.5.7.3 — Add `-ldflags "-X internal/version.Version=$(git describe)"` to build commands
- [x] 0.5.7.4 — Add `ralphglasses version` subcommand: print version, go version, build date, commit SHA
- [x] 0.5.7.5 — Display version in TUI help view and MCP server info
- **Acceptance:** `ralphglasses version` outputs correct git-derived version

### 0.5.8 — CI BATS guard (COMPLETE)
- [x] 0.5.8.1 — Guard BATS step in CI: check `scripts/test/` exists and contains `.bats` files before running
- [x] 0.5.8.2 — Add BATS install step to CI (install `bats-core` if not present)
- [x] 0.5.8.3 — Run `marathon.bats` in CI with mock ANTHROPIC_API_KEY
- [x] 0.5.8.4 — Add CI matrix: test on ubuntu-latest and macos-latest
- **Acceptance:** CI passes regardless of test directory presence

### 0.5.9 — Race condition in MCP scan (COMPLETE)
- [x] 0.5.9.1 — Add `sync.RWMutex` to protect `repos` map in `internal/mcpserver/` during concurrent scans
- [x] 0.5.9.2 — Add `go test -race` to CI pipeline for all packages
- [x] 0.5.9.3 — Write concurrent scan test: 10 goroutines scanning simultaneously
- **Acceptance:** `go test -race ./...` passes clean

### 0.5.10 — Marathon.sh edge cases
- [x] 0.5.10.1 — Add `bc` availability check at script start (budget calculation depends on it)
- [x] 0.5.10.2 — Add disk space check before marathon start (warn if < 5GB free) `P1` `S`
- [x] 0.5.10.3 — Fix infinite restart loop: cap MAX_RESTARTS, add cooldown between restarts `P1` `M`
- [x] 0.5.10.4 — Add memory pressure monitoring: check `/proc/meminfo` AvailMem, warn at < 2GB `P2` `S`
- [x] 0.5.10.5 — Add log rotation: rotate marathon logs at 100MB, keep last 3 `P2` `S`
- **Acceptance:** marathon.sh handles resource exhaustion gracefully

### 0.5.11 — Config validation strictness
- [x] 0.5.11.1 — Define canonical key list with types: `internal/model/config_schema.go` `P1` `M`
- [x] 0.5.11.2 — Warn on unknown keys in `.ralphrc` (typo detection) `P1` `S`
- [x] 0.5.11.3 — Validate numeric ranges: `MAX_CALLS_PER_HOUR` must be 1-1000, `CB_COOLDOWN_MINUTES` must be 1-60 `P1` `S`
- [x] 0.5.11.4 — Validate boolean values: only "true"/"false", reject "yes"/"1"/"on" `P2` `S`
- **Acceptance:** invalid `.ralphrc` values produce clear error messages

## Phase 0.6: Code Quality & Observability

Post-gate-pass improvements. All items are independent, parallel, and sized for single-iteration self-improvement loop completion.

> **Parallel workstreams:** All 0.6.x items are independent. No blockers between them.

### 0.6.1 — Test coverage for uncovered error paths
- [x] 0.6.1.1 — Tests for `internal/discovery/` error paths: unreadable dirs, symlink cycles, permission denied `P1` `M`
- [x] 0.6.1.2 — Tests for `internal/model/` corrupt file handling: truncated JSON, invalid UTF-8, zero-byte files, unreadable files, RefreshRepo multi-corrupt `P1` `M`
- [x] 0.6.1.3 — Tests for `internal/process/` edge cases: double-stop, stop-before-start, signal delivery to exited process, ESRCH handling `P1` `M`
- [x] 0.6.1.4 — Tests for `internal/enhancer/` pipeline stages: empty input, extremely long input, unicode-heavy prompts `P1` `M`
- **Acceptance:** each new test exercises an error path that previously had no coverage

### 0.6.2 — Observation enrichment
- [x] 0.6.2.1 — Add `GitDiffStat` field to `LoopObservation`: files changed, insertions, deletions from worker output `P1` `M`
- [x] 0.6.2.2 — Add `PlannerModelUsed` and `WorkerModelUsed` fields to `LoopObservation` for provider tracking `P1` `S`
- [x] 0.6.2.3 — Add `AcceptancePath` field to `LoopObservation`: "auto_merge", "pr", "rejected" for merge outcome tracking `P1` `M`
- [x] 0.6.2.4 — Add observation summary helper: `SummarizeObservations([]LoopObservation) ObservationSummary` with aggregate stats `P1` `M`
- **Acceptance:** new fields populated in observations, summary helper has tests

### 0.6.3 — Loop configuration validation
- [x] 0.6.3.1 — Add `ValidateLoopConfig(LoopConfig) []error` — validate all loop config fields before loop start `P0` `M`
- [x] 0.6.3.2 — Validate model names against known provider models (claude-opus-4-6, claude-sonnet-4-6, gemini-3.1-pro, etc.) `P1` `S`
- [x] 0.6.3.3 — Validate enhancement flags: warn if `enable_worker_enhancement=true` with non-Claude worker (no effect) `P1` `S`
- [x] 0.6.3.4 — Add config validation call at loop start, return clear error before spawning any sessions `P0` `S`
- **Acceptance:** invalid loop configs rejected with descriptive errors before work begins

### 0.6.4 — Gate report formatting
- [x] 0.6.4.1 — Add `FormatGateReport(*GateReport) string` — human-readable gate summary with pass/warn/fail coloring hints `P1` `M`
- [x] 0.6.4.2 — Add `FormatGateReportMarkdown(*GateReport) string` — markdown table for scratchpad/PR descriptions `P1` `S`
- [x] 0.6.4.3 — Add gate trend helper: `CompareGateReports(prev, current *GateReport) []GateTrend` showing improvement/regression per metric `P1` `M`
- [x] 0.6.4.4 — Wire `FormatGateReport` into loop status output and MCP `loop_gates` tool response `P1` `S`
- **Acceptance:** gate reports render as readable tables, trend comparison shows metric direction

### 0.6.5 — Session timeout and stall detection
- [x] 0.6.5.1 — Add `StallTimeout` field to `LoopConfig` (default: 10 minutes) — max time for a single iteration with no output `P0` `M`
- [x] 0.6.5.2 — Implement stall detector in `StepLoop`: monitor worker session output timestamp, kill and retry on timeout `P0` `L`
- [x] 0.6.5.3 — Add `StallCount` field to `LoopObservation` for tracking stall frequency `P1` `S`
- [x] 0.6.5.4 — Add stall detection tests: mock session that produces no output, assert timeout triggers `P1` `M`
- **Acceptance:** stalled iterations detected and retried, stall count tracked in observations

### 0.6.6 — Worktree cleanup robustness
- [x] 0.6.6.1 — Add `CleanupStaleWorktrees(repoRoot string, maxAge time.Duration) (int, error)` — remove worktrees older than maxAge `P1` `M`
- [x] 0.6.6.2 — Add worktree lock file detection: skip cleanup if `.lock` file present (active worktree) `P1` `S`
- [x] 0.6.6.3 — Call `CleanupStaleWorktrees` at loop start with 24h maxAge `P1` `S`
- [x] 0.6.6.4 — Add `ralphglasses_worktree_cleanup` MCP tool for manual cleanup `P2` `M`
- **Acceptance:** stale worktrees cleaned up automatically, active worktrees preserved

### 0.6.7 — Planner task deduplication improvement
- [x] 0.6.7.1 — Add Levenshtein/Jaccard similarity check to `isDuplicateTask`: catch near-duplicate titles (threshold 0.8) `P1` `M`
- [x] 0.6.7.2 — Track completed task titles in observation history, reject re-proposals of already-completed work `P1` `M`
- [x] 0.6.7.3 — `DedupSkip` with reason, matched title, similarity wired into `LoopIteration.SkippedTasks` `P2` `S`
- [x] 0.6.7.4 — Add dedup tests: exact match, near-match, and distinct task pairs `P1` `M`
- **Acceptance:** planner doesn't re-propose completed or near-duplicate tasks

### Phase 0.7 — Codebase Hardening & Observability

- [x] 0.7.1 — Observation enrichment: add GitDiffStat, model fields, AcceptancePath, StallCount, TurnCount to LoopObservation `P1` `L`
- [x] 0.7.2 — Loop config validation: ValidateLoopProfile with model-provider prefix matching, verifier validation, budget/limit bounds `P0` `M`
- [x] 0.7.3 — Stall detection: LoopStallDetector monitors iteration timestamps, wired into RunLoop `P0` `L`
- [x] 0.7.4 — Gate report formatting: FormatGateReport, FormatGateReportMarkdown, CompareGateVerdicts `P1` `M`
- [x] 0.7.5 — Gate report dedup + baseline fix: consolidated outputGateReport, fixed swallowed baseline save errors `P0` `M`
- [x] 0.7.6 — Event bus improvements: SubscribeFiltered, event type validation, async persistence `P1` `L`
- [x] 0.7.7 — Provider cost rate config: externalized to .ralph/cost_rates.json via LoadCostRatesFromDir `P1` `S`
- [x] 0.7.8 — Worktree cleanup robustness: age-based cleanup with lock file + uncommitted change detection `P1` `M`
- [x] 0.7.9 — CLI os.Exit fix: sentinel errors (ErrChecksFailed, ErrGateFailed) for cobra handling `P1` `S`
- [x] 0.7.10 — Planner task dedup: Jaccard similarity + content-overlap filtering in loop_steps.go `P1` `M`
- [x] 0.7.11 — Marathon resource monitoring: disk space, memory checks, log rotation `P2` `M`

## Phase 0.8: MCP Observability & Scratchpad Automation (COMPLETE)

New `observability` tool group (13th namespace, 11 tools). Replaces sleep anti-patterns,
adds programmatic scratchpad note-taking, surfaces observation/cost/coverage data via MCP.
Implements MCP spec features: structured output schemas, logging notifications.

> **Parallel workstreams:** 0.8.1-0.8.8 are independent (new files only). 0.8.9 wires registration.

### 0.8.1 — Observation query tools (COMPLETE)
- [x] `ralphglasses_observation_query`: filter/paginate loop observations by repo, hours, loop_id, status, provider
- [x] `ralphglasses_observation_summary`: aggregate stats via SummarizeObservations

### 0.8.2 — Scratchpad MCP tools (COMPLETE)
- [x] `ralphglasses_scratchpad_read`: read `.ralph/{name}_scratchpad.md`
- [x] `ralphglasses_scratchpad_append`: append markdown note with optional section header
- [x] `ralphglasses_scratchpad_list`: glob `.ralph/*_scratchpad.md`
- [x] `ralphglasses_scratchpad_resolve`: mark numbered item as resolved

### 0.8.3 — Loop wait/poll tools (COMPLETE)
- [x] `ralphglasses_loop_await`: blocking wait with timeout, replaces `sleep && echo done` anti-pattern
- [x] `ralphglasses_loop_poll`: non-blocking single status check

### 0.8.4 — Coverage report tool (COMPLETE)
- [x] `ralphglasses_coverage_report`: run `go test -coverprofile`, report per-package vs threshold

### 0.8.5 — Cost estimation tool (COMPLETE)
- [x] `ralphglasses_cost_estimate`: pre-launch cost estimate with historical calibration (60/40 blend)

### 0.8.6 — Merge verification tool (COMPLETE)
- [x] `ralphglasses_merge_verify`: sequential build->vet->test with 5-min timeout per step

### 0.8.7 — MCP logging integration (COMPLETE)
- [x] `MCPLogger` wrapping `*server.MCPServer` for `notifications/message` emission
- [x] `MCPLoggingMiddleware` returning `server.ToolHandlerMiddleware`
- [x] Falls back to slog when no MCP client connected

### 0.8.8 — Structured output schemas (COMPLETE)
- [x] `OutputSchemas` map for 6 high-value tools (observation_query, observation_summary, loop_benchmark, fleet_status, cost_estimate, coverage_report)
- [x] `SchemaForTool()` helper for integration

### 0.8.9 — Registration wiring & bookkeeping (COMPLETE)
- [x] `buildObservabilityGroup()` in tools_builders.go with 11 tool entries
- [x] `"observability"` added to ToolGroupNames
- [x] Test expectations updated (96->107 tools, 12->13 namespaces)
- [x] Scratchpad items #4, #5, #7, #20, #25, #26 marked RESOLVED

---

## Phase 1: Harden & Test

**Completed:**
- [x] Unit tests for all packages — 78.2% coverage (discovery, model, process, mcpserver)
- [x] TUI tests — 55.5% app coverage, view rendering, keymap, command/filter modes
- [x] CI pipeline — `go test`, `go vet`, `golangci-lint`, shellcheck, fuzz, benchmarks, BATS
- [x] Error handling — MCP scan error propagation, log stream errors, config key validation
- [x] Process manager — watcher timeout fix (no longer blocks event loop)
- [x] Config editor — key validation
- [x] End-to-end evaluation framework — `internal/e2e/` with baseline tracking, aggregate metrics, scenario stats, counterfactual analysis `[reconciled 2026-03-26]`

**Remaining (38 subtasks):**

> **Parallel workstreams:** 1.1 and 1.2 can proceed concurrently. 1.3 and 1.5 can proceed concurrently. 1.4 depends on 1.1 fixtures. 1.6 depends on all others. 1.7-1.10 can proceed in parallel with everything except 1.6.

### 1.1 — Integration test: full lifecycle (COMPLETE)
- [x] 1.1.1 — Create test fixture directory with `.ralph/` dir, mock `status.json`, and dummy `.ralphrc` — `internal/integration/helpers_test.go` `[reconciled 2026-03-26]`
- [x] 1.1.2 — Write mock `ralph_loop.sh` that simulates loop lifecycle (start, write status, exit) `[reconciled 2026-03-26]`
- [x] 1.1.3 — Implement lifecycle test: scan -> start -> poll status -> stop, assert state transitions — `internal/integration/lifecycle_test.go` `[reconciled 2026-03-26]`
- [x] 1.1.4 — Add `//go:build integration` tag and CI gate (`go test -tags=integration`) `[reconciled 2026-03-26]`
- **Acceptance:** `go test -tags=integration` passes end-to-end lifecycle

### 1.2 — MCP server hardening (COMPLETE)
- [x] 1.2.1 — Audit all shared state in `mcpserver`; add `sync.RWMutex` around `repos` map and scan results — `tools.go` line 41 `[reconciled 2026-03-26]`
- [x] 1.2.2 — Migrate all `log.Printf` calls to `slog` with structured fields (tool name, repo path, duration) — zero `log.Printf` remain in mcpserver `[reconciled 2026-03-26]`
- [x] 1.2.3 — Add request validation: reject empty repo paths, unknown config keys, malformed JSON — `internal/mcpserver/validate.go` `[reconciled 2026-03-26]`
- [x] 1.2.4 — Define MCP error codes (not-found, invalid-input, internal) and return structured errors — `internal/mcpserver/errors.go` with 17 error codes `[reconciled 2026-03-26]`
- **Acceptance:** no data races under `go test -race`, structured JSON log output

### 1.2.5 — MCP Handler Framework
- [x] 1.2.5.1 — Extract ParamParser helper: type-safe parameter extraction with validation, replacing manual `getStringArg`/`getNumberArg` calls across 81 handlers `P1` `L`
- [x] 1.2.5.2 — Standardize error codes across all handlers: migrate from `errResult()` to `errCode()` with consistent error_code values (invalid_params, not_found, internal_error) `P1` `L`
- [x] 1.2.5.3 — Handler test harness: mock Server with mock providers for table-driven tests, reducing per-handler test boilerplate `P1` `M`
- [ ] 1.2.5.4 — Handler generator: codegen tool for new MCP tool scaffolding (registration + handler + test stub) `[BLOCKED BY 1.2.5.1, 1.2.5.2]` `P2` `M`
- **Acceptance:** new handler scaffolding requires <10 LOC, all 81 handlers use ParamParser, zero raw `errResult()` calls remain

### 1.3 — TUI polish
- [x] 1.3.1 — Build `ConfirmDialog` component (y/n prompt overlay, reusable across views) — `internal/tui/components/confirm.go` `[reconciled 2026-03-26]`
- [x] 1.3.2 — Wire confirm dialog to destructive actions: stop, stop_all, config delete — wired in handlers_detail.go, handlers_loops.go, handlers_common.go `[reconciled 2026-03-26]`
- [x] 1.3.3 — SIGINT/SIGTERM shutdown handler: graceful stop of all managed processes, flush logs, clean exit `P0` `M`
- [x] 1.3.4 — Scroll bounds hardened: clampScrollPos on filter change and resize in EventLogView `P1` `M`
- **Acceptance:** destructive actions require y/n, clean exit on signals, no scroll panics on resize

### 1.4 — Process manager improvements
- [x] 1.4.1 — Define PID file format (JSON: pid, start_time, repo_path) and write on process launch `[BLOCKED BY 1.1.1]` `P1` `M`
- [x] 1.4.2 — Implement orphan scanner: on startup, check PID files against running processes, clean stale entries `P1` `M`
- [x] 1.4.3 — Add restart policy to `.ralphrc` (`RESTART_ON_CRASH=true`, `MAX_RESTARTS=3`, `RESTART_DELAY_SEC=5`) `P1` `M`
- [x] 1.4.4 — Implement health check loop: poll process status every 5s, trigger restart or circuit breaker on repeated failures `P1` `L`
- **Acceptance:** survives ralph crash with auto-restart, no orphan processes after TUI exit

### 1.5 — Config editor enhancements
- [x] 1.5.1 — Add key CRUD operations: insert new key, rename key, delete key from TUI `P2` `M`
- [x] 1.5.2 — Wire fsnotify on `.ralphrc` file; reload config on external change, emit notification `P1` `M`
- [x] 1.5.3 — Add validation rules per key type (numeric ranges, boolean, enum values) `P1` `M`
- [x] 1.5.4 — Implement undo buffer (single-level: revert last edit) `P2` `S`
- **Acceptance:** external edits reflected without restart, invalid values rejected with message

### 1.6 — Test coverage targets
- [x] 1.6.1 — Per-package coverage targets in `check-coverage.sh` + `make test-cover-strict` Makefile target `P1` `S`
- [x] 1.6.2 — Add CI enforcement step: `go test -coverprofile` -> parse -> fail if below threshold `P1` `M`
- [x] 1.6.3 — Add coverage badge to README via codecov or go-cover-treemap `P2` `S`
- [x] 1.6.4 — Write missing tests to reach 85%+ overall (focus on untested error paths) `P1` `L`
- **Acceptance:** `go test -coverprofile` meets thresholds in CI, badge visible in README

### 1.7 — Structured logging
- [x] 1.7.1 — Replace all `log.Printf` calls in `internal/mcpserver/` with `slog.Info`/`slog.Error` — zero `log.Printf` remain `[reconciled 2026-03-26]`
- [x] 1.7.2 — Replace all `log.Printf` calls in `internal/process/` with structured `slog` — uses `slog` in manager, lifecycle, orphans `[reconciled 2026-03-26]`
- [x] 1.7.3 — Add `--log-level` flag to CLI: debug, info, warn, error (default: info) `P1` `S`
- [x] 1.7.4 — Add `--log-format` flag: text (default for TTY), json (default for non-TTY) `P1` `S`
- [x] 1.7.5 — Request-scoped context fields: `tracing.WithToolName/WithRepo/WithRequestStart` + `RequestLogger(ctx)` helper `P1` `M`
- **Acceptance:** all log output is structured `slog`, configurable level and format

### 1.8 — Custom error types `[BLOCKED BY 0.5.1]`
- [x] 1.8.1 — Define sentinel errors in `internal/model/`: `ErrStatusNotFound`, `ErrConfigParseFailed`, `ErrCircuitOpen` `P1` `S`
- [x] 1.8.2 — Define sentinel errors in `internal/process/`: `ErrAlreadyRunning`, `ErrNotRunning`, `ErrNoLoopScript` — `internal/process/errors.go` `[reconciled 2026-03-26]`
- [x] 1.8.3 — Audited all `fmt.Errorf` calls: 1 instance fixed (`handler_mergeverify.go` `%v` → `%w`) `P1` `M`
- [x] 1.8.4 — Create `internal/errors/` package with error classification: transient, permanent, user-facing `P1` `M`
- [x] 1.8.5 — Add error type assertions in MCP handlers: map error types to MCP error codes `P1` `M`
- **Acceptance:** callers can use `errors.Is()` and `errors.As()` on all returned errors

### 1.9 — Context propagation
- [x] 1.9.1 — Thread `context.Context` through `discovery.Scan()` — support cancellation of long scans `[reconciled 2026-03-26]`
- [x] 1.9.2 — Thread `context.Context` through `model.Load*()` functions — timeout on stuck file reads `P1` `M`
- [x] 1.9.3 — Use incoming `ctx` in MCP tool handlers (currently received but ignored) `P0` `M`
- [x] 1.9.4 — Add `--scan-timeout` flag: max time for initial directory scan (default: 30s) `P1` `S`
- [x] 1.9.5 — Wire context cancellation to TUI shutdown: cancel in-flight operations on exit `P1` `M`
- **Acceptance:** all long-running operations respect context cancellation

### 1.10 — TUI bounds safety
- [x] 1.10.1 — SortCol out-of-bounds: clamped to valid range when columns change `P0` `S`
- [x] 1.10.2 — Add search UI to LogView: `/` to enter search, `n`/`N` for next/prev match `P2` `M`
- [x] 1.10.3 — Audited all slice access in TUI components — 19 locations verified with proper guards `P0` `M`
- [x] 1.10.4 — Add fuzz tests for table rendering with random column counts and data `P2` `M`
- [x] 1.10.5 — Zero-height terminal guard: shows "Terminal too small" for width/height < 3 `P1` `S`
- **Acceptance:** no panics on edge-case terminal sizes or empty data

### 1.11 — TUI Visual Polish Marathon

Iterative capture→analyze→fix→verify cycle across all 20 views. Eliminates wasted whitespace, improves information density, makes layouts responsive to terminal width.
Reference research: `~/hairglasses-studio/docs/research/tui-design/`

#### Tier 1: Quick Wins
- [x] 1.11.1 — Reduce double blank lines to single between sections in fleet/detail views `P2` `S`
- [x] 1.11.2 — Compact status bar separators (3-char ` │ ` to 1-char `│`) — recovers ~12 chars `P2` `S`
- [x] 1.11.3 — Dynamic event/session row counts in fleet dashboard (height-aware instead of hardcoded 10) `P1` `S`
- [x] 1.11.4 — Responsive sparkline widths — remove hardcoded caps (30/60), scale to terminal width up to 120 `P1` `M`

#### Tier 2: Responsive Layout
- [x] 1.11.5 — Column priority hiding: Priority field on Column struct, overview hides Calls/Progress/CB below 140 cols `P1` `M`
- [x] 1.11.6 — Fleet stat box wrapping: `wrapStatBoxes()` breaks to multiple rows when exceeding width `P1` `M`
- [x] 1.11.7 — Fleet panel vertical stacking: repo/session/team lists stack vertically below 90 cols `P2` `M`
- [x] 1.11.8 — Sessions table column priorities: Trend/Agent/Team hidden below 140 cols `P1` `S`
- [x] 1.11.9 — Dim inactive repos (idle/unknown) in overview table with DimStyle `P1` `S`
- [x] 1.11.10 — Dim completed/stopped/failed sessions in sessions table `P1` `S`
- [x] 1.11.11 — Stat box wrapping in recovery, forecast, coordination views `P2` `S`
- [x] 1.11.12 — Godview adaptive layout: LIVE OUTPUT shrinks when sparse, more table space `P1` `M`
- [x] 1.11.13 — Double blank line cleanup across 10+ secondary views (analytics, forecast, recovery, etc.) `P2` `S`
- [x] 1.11.14 — Responsive fleet budget gauges and repo detail cost sparkline `P1` `S`

#### Tier 3: Enhanced Layouts
- [x] 1.11.15 — Virtual scrolling for fleet list sections — height-based windowing, `[N-M of Total]` header `P2` `L`
- [x] 1.11.16 — Two-column repo detail at wide terminals (>140 cols): left status+progress, right CB+config `P2` `L`
- [x] 1.11.17 — Fleet cost trend upgrades to streamlinechart (ntcharts) at tall terminals, sparkline fallback `P3` `M`

- **Acceptance:** All views render without overflow at 80 cols; sparklines fill width at 200 cols; stat boxes wrap correctly

## Phase 1.5: Developer Experience

Tooling, release automation, and contributor workflow. All items independent of Phase 1.

> **Parallel workstreams:** All 1.5.x items are independent except 1.5.2 depends on 0.5.7 (version ldflags).

- [x] Plugin system — `internal/plugin/` with hashicorp/go-plugin gRPC interface, provider plugins, lifecycle management `[reconciled 2026-03-26]`
- [x] Batch API framework — `internal/batch/` with multi-provider batch submission (Claude, Gemini, OpenAI) `[reconciled 2026-03-26]`

### 1.5.1 — Shell completions
- [x] 1.5.1.1 — `ralphglasses completion bash|zsh|fish` via cobra built-in `GenBashCompletion` `P1` `S`
- [x] 1.5.1.2 — Add dynamic completions for `--scan-path` (directory completion) `P2` `S`
- [x] 1.5.1.3 — Add dynamic completions for repo names in `status`, `start`, `stop` subcommands `P2` `M`
- [x] 1.5.1.4 — Add install instructions for each shell to `docs/completions.md` `P2` `S`
- [x] 1.5.1.5 — Package completions in release artifacts (`.deb` installs to `/usr/share/bash-completion/`) `P2` `M`
- **Acceptance:** `ralphglasses <tab>` completes subcommands and repo names

### 1.5.2 — Release automation `[BLOCKED BY 0.5.7]`
- [x] 1.5.2.1 — Add `.goreleaser.yaml`: supported builds (linux/amd64, darwin/amd64, darwin/arm64, windows/amd64) `[reconciled 2026-03-26, narrowed 2026-04-07]`
- [x] 1.5.2.2 — GitHub Actions release workflow: tag push -> goreleaser -> GitHub Release with binaries — `.github/workflows/release.yml` `[reconciled 2026-03-26]`
- [x] 1.5.2.3 — Add changelog generation: `goreleaser` changelog from conventional commits `P2` `S`
- [x] 1.5.2.4 — Add Docker image build: `ghcr.io/hairglasses-studio/ralphglasses` multi-arch manifest `P2` `M`
- [x] 1.5.2.5 — Add Homebrew tap: `hairglasses-studio/homebrew-tap` with goreleaser auto-update `P2` `M`
- [x] 1.5.2.6 — Add AUR package: `PKGBUILD` for Arch Linux users `P2` `S`
- **Acceptance:** `git tag v0.2.0 && git push --tags` produces release with binaries, Docker image, Homebrew formula

### 1.5.3 — Pre-commit hooks
- [x] 1.5.3.1 — Add `.pre-commit-config.yaml`: golangci-lint, gofumpt, shellcheck, markdownlint `P2` `S`
- [x] 1.5.3.2 — Add `Makefile` with targets: `lint`, `test`, `build`, `install`, `bench`, `fuzz`, and more `[reconciled 2026-03-26]`
- [x] 1.5.3.3 — Add EditorConfig (`.editorconfig`) for consistent formatting across editors `P2` `S`
- [x] 1.5.3.4 — Add `CONTRIBUTING.md` with setup instructions and PR guidelines (281 lines) `[reconciled 2026-03-26]`
- **Acceptance:** `pre-commit run --all-files` passes clean

### 1.5.4 — Config schema documentation
- [x] 1.5.4.1 — Write `docs/ralphrc-reference.md`: all keys, types, defaults, descriptions, examples `P2` `M`
- [x] 1.5.4.2 — Add `ralphglasses config list-keys` subcommand: print all known keys with defaults `P2` `S`
- [x] 1.5.4.3 — Add `ralphglasses config validate` subcommand: check `.ralphrc` against schema `P1` `S`
- [x] 1.5.4.4 — Add `ralphglasses config init` subcommand: generate `.ralphrc` with all keys and defaults `P2` `S`
- [x] 1.5.4.5 — Auto-generate config docs from schema (Go code -> Markdown via `go:generate`) `P2` `M`
- **Acceptance:** `ralphglasses config list-keys` outputs all valid configuration keys

### 1.5.5 — Man page generation
- [x] 1.5.5.1 — Add `//go:generate` directive: `cobra/doc.GenManTree` for all subcommands `P2` `S`
- [x] 1.5.5.2 — Include man pages in release artifacts (`.tar.gz` includes `man/`) `P2` `S`
- [x] 1.5.5.3 — Add `make install-man` target: copy to `/usr/local/share/man/man1/` `P2` `S`
- **Acceptance:** `man ralphglasses` works after install

### 1.5.6 — Platform builds
- [x] 1.5.6.1 — Add supported cross-compilation coverage for release targets `P2` `M`
- [x] 1.5.6.2 — Keep darwin/arm64 release support for Apple Silicon `P2` `S`
- [x] 1.5.6.3 — Add amd64 smoke build coverage for Linux releases `P2` `S`
- [x] 1.5.6.4 — Remove unsupported Linux ARM/Raspberry Pi release paths and CI `P2` `S`
- **Acceptance:** supported binaries build for linux/amd64, darwin/amd64, darwin/arm64, and windows/amd64

### 1.5.7 — Nix flake (optional)
- [x] 1.5.7.1 — Add `flake.nix` with `buildGoModule` + dev shell (Go, golangci-lint, shellcheck) `P2` `M`
- [x] 1.5.7.2 — Add NixOS module: systemd service, option types for config `P2` `L`
- [x] 1.5.7.3 — Add `flake.lock` and CI check: `nix build` + `nix flake check` `P2` `S`
- **Acceptance:** `nix run github:hairglasses-studio/ralphglasses` works

### 1.5.8 — Development containers
- [x] 1.5.8.1 — Add `.devcontainer/devcontainer.json`: Go + tools, port forwarding, recommended extensions `P2` `S`
- [x] 1.5.8.2 — Add `.devcontainer/Dockerfile`: Go 1.26+, golangci-lint, BATS, shellcheck `P2` `S`
- [x] 1.5.8.3 — GitHub Codespaces support: verify `go build ./...` and `go test ./...` work `P2` `M`
- **Acceptance:** `devcontainer up` provides working dev environment

### 1.5.9 — Documentation site
- [x] 1.5.9.1 — Add `docs/` site with mdBook or mkdocs: getting started, architecture, API reference `P2` `L`
- [x] 1.5.9.2 — Add GitHub Actions: build docs on push, deploy to GitHub Pages `P2` `M`
- [x] 1.5.9.3 — Add architecture diagrams: mermaid flowcharts for data flow, component relationships `P2` `M`
- [x] 1.5.9.4 — Add MCP tool API reference: auto-generated from tool handler docstrings `P2` `L`
- **Acceptance:** docs site live at `hairglasses-studio.github.io/ralphglasses`

### 1.5.10 — Charmbracelet v2 migration
- [x] 1.5.10.1 — Migrate to Bubble Tea v2 (`charm.land/bubbletea/v2`): synchronized rendering (eliminates tearing), clipboard (OSC52), GraphicsMode, declarative Views API `P1` `XL`
- [x] 1.5.10.2 — Migrate to Lip Gloss v2 (`charm.land/lipgloss/v2`): deterministic styles (explicit `isDark` bool), explicit I/O control, SSH/Wish compat `P1` `L`
- [x] 1.5.10.3 — Update bubbles components for v2 API changes (table, viewport, list, textinput) `P1` `L`
- [ ] 1.5.10.4 — Adopt Lip Gloss v2 `table`, `tree`, `list` packages for fleet dashboard `P2` `M`
- [ ] 1.5.10.5 — Evaluate ntcharts streaming charts for real-time fleet health graphs `P2` `M`
- **Acceptance:** All 18 TUI views render without tearing; clipboard copy works; `go build ./...` clean

> **Breaking changes:** Bubble Tea v2 uses ncurses-based renderer. Lip Gloss v2 removes auto-detection side effects. Both import paths change. Must migrate together. See [Charm v2 blog](https://charm.land/blog/v2/).

### 1.5.11 — mcp-go → official SDK migration
- [ ] 1.5.11.1 — Evaluate `modelcontextprotocol/go-sdk` v1.4.1 feature parity with mcp-go v0.45.0 `P1` `M`
- [ ] 1.5.11.2 — Migrate `internal/mcpserver/` tool registration from mcp-go to official SDK `P1` `XL`
- [ ] 1.5.11.3 — Migrate transport layer (stdio + add streamable HTTP support) `P1` `L`
- [ ] 1.5.11.4 — Add OAuth support for remote MCP server mode `P2` `L`
- **Acceptance:** The full MCP tool surface registers and passes integration tests with the official SDK

### 1.5.12 — Benchmarking infrastructure
- [x] 1.5.12.1 — Add Go benchmarks for hot paths: `RefreshRepo`, `Scan`, `LoadStatus`, table rendering `P1` `M`
- [x] 1.5.12.2 — Add `benchstat` comparison in CI: detect performance regressions between commits `P1` `M`
- [x] 1.5.12.3 — Add benchmark dashboard: track p50/p99 latencies over time `P2` `L`
- [x] 1.5.12.4 — Add memory allocation benchmarks: `b.ReportAllocs()` on all benchmark functions `P1` `S`
- **Acceptance:** CI fails on >10% performance regression

## Phase 2: Multi-Session Fleet Management

> **Depends on:** Phase 1 (concurrency guards, process manager improvements)
>
> **Parallel workstreams:** 2.1 (data model) is the foundation — most items depend on it. 2.6 (notifications) and 2.7 (tmux) are independent of each other and can proceed after 2.1. 2.9 (CLI) is independent of TUI work. 2.10 (marathon port) is fully independent. 2.11-2.14 are independent.

- [x] Fleet management package — `internal/fleet/` with A2A agent cards, task offers, worker pool, DLQ, budget enforcement (38 files) `[reconciled 2026-03-26]`
- [x] Eval framework — `internal/eval/` with Bayesian A/B testing, anomaly detection, changepoint analysis, counterfactual evaluation `[reconciled 2026-03-26]`

### 2.1 — Session data model
- [x] 2.1.1 — Define `Session` struct: ID, repo path, worktree path, PID, budget, model, status, created_at, updated_at `P0` `M`
- [x] 2.1.2 — Add SQLite via `modernc.org/sqlite`: schema migrations, connection pool, WAL mode `P0` `L`
- [x] 2.1.3 — Implement Session CRUD: Create, Get, List, Update, Delete with prepared statements `P0` `M`
- [x] 2.1.4 — Implement lifecycle state machine: `created -> running -> paused -> stopped -> archived` with valid transition enforcement `P0` `M`
- [x] 2.1.5 — Add session event log table: state changes, errors, budget events with timestamps `P1` `M`
- **Acceptance:** sessions survive TUI restart, queryable via SQL

### 2.2 — Git worktree orchestration `[BLOCKED BY 2.1]`
- [x] 2.2.1 — Create `internal/worktree/` package: wrapping `git worktree add/list/remove` `P0` `M`
- [x] 2.2.2 — Auto-create worktree on session launch: branch naming convention `ralph/<session-id>` `P0` `M`
- [x] 2.2.3 — Implement merge-back: `git merge --no-ff` with conflict detection and abort-on-conflict option `P0` `L`
- [x] 2.2.4 — Add worktree cleanup on session stop/archive (remove worktree dir, prune) `P1` `S`
- [x] 2.2.5 — Handle edge cases: dirty worktree on stop, orphaned branches, worktree path conflicts `P1` `M`
- **Acceptance:** `ralphglasses worktree create <repo>` produces isolated worktree, merge-back detects conflicts

### 2.3 — Budget tracking `[BLOCKED BY 2.1]`
- [x] 2.3.1 — Per-session spend poller: read `session_spend_usd` from `.ralph/status.json` on watcher tick `P0` `M`
- [x] 2.3.2 — Implement global budget pool: total ceiling, per-session allocation, remaining calculation `P0` `M`
- [x] 2.3.3 — Add threshold alerts at 50%, 75%, 90% — emit BubbleTea message for TUI notification `P1` `S`
- [x] 2.3.4 — Auto-pause session at budget ceiling: send SIGSTOP, update session state `P0` `M`
- [x] 2.3.5 — Port budget tracking patterns from `mcpkit/finops` (cost ledger, rate calculation) `P1` `M`
- **Acceptance:** session auto-pauses when budget exhausted, alerts visible in TUI

### 2.4 — Fleet dashboard TUI view `[BLOCKED BY 2.1]`
- [x] 2.4.1 — Create `ViewFleet` in view stack with aggregate session table `P1` `M`
- [x] 2.4.2 — Columns: session name, repo, status, spend, loop count, model, uptime — sortable `P1` `M`
- [x] 2.4.3 — Live-update via watcher ticks: refresh spend/status/loop count per row `P1` `M`
- [x] 2.4.4 — Inline actions from fleet view: start/stop/pause selected session via keybinds `P1` `M`
- [x] 2.4.5 — Add fleet summary bar: total sessions, running count, total spend, aggregate throughput `P1` `S`
- **Acceptance:** fleet view shows all sessions with live-updating spend/status

### 2.5 — Session launcher `[BLOCKED BY 2.1, 2.2, 2.3]`
- [x] 2.5.1 — Implement `:launch` command: pick repo from discovered list, set session name `P1` `M`
- [x] 2.5.2 — Add budget/model selection to launch flow: dropdown or tab-complete for model, numeric input for budget `P1` `M`
- [x] 2.5.3 — Default budget from `.ralphrc` (`RALPH_SESSION_BUDGET`) or global config fallback `P1` `S`
- [x] 2.5.4 — Session templates: save current launch config as named template, load from template `P2` `M`
- [x] 2.5.5 — Validate launch preconditions: repo exists, no conflicting worktree, budget available in pool `P1` `M`
- **Acceptance:** can launch a named session with budget from TUI command mode

### 2.6 — Notification system `[PARALLEL — independent after 2.1]`
- [x] 2.6.1 — Desktop notification abstraction: `freedesktop.org` D-Bus (Linux), `osascript` (macOS) `P2` `M`
- [x] 2.6.2 — Define event types: session_complete, budget_warning, circuit_breaker_trip, crash, restart `P2` `S`
- [x] 2.6.3 — Add `.ralphrc` config keys: `NOTIFY_DESKTOP=true`, `NOTIFY_SOUND=true` `P2` `S`
- [x] 2.6.4 — Implement notification dedup/throttle: no repeat within 60s for same event type + session `P2` `M`
- **Acceptance:** desktop notification fires on circuit breaker trip

### 2.7 — tmux integration `[PARALLEL — independent after 2.1]`
- [x] 2.7.1 — `internal/tmux/` package: create/list/kill sessions, name windows, attach/detach `P2` `M`
- [x] 2.7.2 — One tmux pane per agent session: auto-create on session launch, name = session ID `P2` `M`
- [x] 2.7.3 — `ralphglasses tmux` subcommand: `list`, `attach <session>`, `detach` `P2` `S`
- [x] 2.7.4 — Headless mode: detect no TTY -> auto-use tmux instead of TUI `P1` `M`
- [x] 2.7.5 — Port patterns from claude-tools (WSL-native tmux management, `/mnt/c/` path handling) `P2` `S`
- **Acceptance:** `ralphglasses tmux list` shows active sessions, `attach` works

### 2.8 — MCP server expansion `[BLOCKED BY 2.1, 2.2, 2.3]`
- [x] 2.8.1 — Add `ralphglasses_session_launch` tool: accepts repo, budget, model, name — implemented as `session_launch` `[reconciled 2026-03-26]`
- [x] 2.8.2 — Add `ralphglasses_session_list` tool: returns all sessions with status `[reconciled 2026-03-26]`
- [x] 2.8.3 — Add `ralphglasses_worktree_create` tool: create worktree for repo `P1` `M`
- [x] 2.8.4 — Add `ralphglasses_session_budget` tool: per-session budget info `[reconciled 2026-03-26]`
- [x] 2.8.5 — Add `ralphglasses_fleet_status` tool: aggregate stats for agent-to-agent coordination `[reconciled 2026-03-26]`
- **Acceptance:** MCP tools callable from Claude Code, session lifecycle works end-to-end

### 2.9 — CLI subcommands
- [x] 2.9.1 — `ralphglasses session list|start|stop|status` — non-TUI session management `P1` `M`
- [x] 2.9.2 — `ralphglasses worktree create|list|merge|clean` — worktree operations from CLI `P1` `M`
- [x] 2.9.3 — `ralphglasses budget status|set|reset` — budget management from CLI `P2` `S`
- [x] 2.9.4 — JSON output flag (`--json`) for all subcommands for scripting/piping `P1` `S`
- **Acceptance:** all fleet operations available without TUI, JSON output parseable by `jq`

### 2.10 — Marathon.sh Go port `[PARALLEL — fully independent]`
- [x] 2.10.1 — Port `marathon.sh` to `internal/marathon/` package: duration limit, budget limit, checkpoints `P1` `L`
- [x] 2.10.2 — `ralphglasses marathon` subcommand: `--budget`, `--duration`, `--checkpoint-interval` `P1` `M`
- [x] 2.10.3 — Replace shell signal handling with Go `os/signal` (SIGINT/SIGTERM -> graceful shutdown) `P1` `M`
- [x] 2.10.4 — Git checkpoint tagging in Go: `git tag marathon-<timestamp>` at configurable interval `P1` `S`
- [x] 2.10.5 — Structured marathon logging via `slog` (replace bash `log()` function) `P1` `S`
- **Acceptance:** `ralphglasses marathon` replaces `bash marathon.sh` with identical behavior

### 2.11 — Health check endpoint `[PARALLEL]`
- [x] 2.11.1 — Add optional `--http-addr` flag (default: disabled, e.g. `:9090`) `P2` `S`
- [x] 2.11.2 — Implement `/healthz` endpoint: returns 200 if process alive, 503 if shutting down `P2` `S`
- [x] 2.11.3 — Implement `/readyz` endpoint: returns 200 if scan complete and sessions loaded `P2` `S`
- [x] 2.11.4 — Implement `/metrics` stub: placeholder for Prometheus endpoint (wired in Phase 6) `P2` `S`
- [x] 2.11.5 — Add systemd watchdog integration: `sd_notify` READY and WATCHDOG signals `P2` `M`
- **Acceptance:** `curl localhost:9090/healthz` returns 200 when TUI is running

### 2.12 — Telemetry opt-in `[PARALLEL]`
- [x] 2.12.1 — Define telemetry event schema: session_start, session_stop, crash, budget_hit, circuit_trip `P2` `S`
- [x] 2.12.2 — Local JSONL file writer: append events to `~/.ralphglasses/telemetry.jsonl` `P2` `M`
- [x] 2.12.3 — Add `--telemetry` flag and `TELEMETRY_ENABLED` config key (default: off) `P2` `S`
- [x] 2.12.4 — Optional remote POST: send anonymized events to configurable endpoint `P2` `M`
- [x] 2.12.5 — Add `ralphglasses telemetry export` subcommand: export telemetry as CSV/JSON `P2` `S`
- **Acceptance:** telemetry events written to local file when opt-in enabled

### 2.13 — Plugin system `[PARALLEL]`
- [x] 2.13.1 — Define plugin interface: `Plugin{ Name(), Init(ctx), OnEvent(event), Shutdown() }` — implemented in `internal/plugin/interfaces.go` `[reconciled 2026-03-26]`
- [x] 2.13.2 — Plugin discovery via hashicorp/go-plugin gRPC protocol (`internal/plugin/grpc.go`) `[reconciled 2026-03-26]`
- [x] 2.13.3 — Built-in plugin: `notify-desktop` (extract from 2.6 as reference implementation) `P2` `M`
- [x] 2.13.4 — Plugin lifecycle: load on startup, unload on shutdown, hot-reload on SIGHUP `P2` `M`
- [x] 2.13.5 — Plugin config: per-plugin config section in `.ralphrc` (e.g. `PLUGIN_NOTIFY_DESKTOP_SOUND=true`) `P2` `S`
- **Acceptance:** external plugin loaded and receives session events

### 2.14 — SSH remote management `[PARALLEL]`
- [x] 2.14.1 — `ralphglasses remote add <name> <host>` — register remote thin client `P2` `M`
- [x] 2.14.2 — `ralphglasses remote status` — SSH into registered hosts, collect session status `P2` `M`
- [x] 2.14.3 — `ralphglasses remote start <host> <repo>` — start ralph loop on remote host `P2` `M`
- [x] 2.14.4 — Aggregate remote sessions into fleet view (poll via SSH tunnel) `P2` `L`
- [x] 2.14.5 — SSH key management: `~/.ralphglasses/ssh/` with per-host key configuration `P2` `M`
- **Acceptance:** fleet view shows sessions from multiple physical machines

## Phase 2.5: Multi-LLM Agent Orchestration

> **Depends on:** Phase 2.1 (session model)
>
> **Parallel workstreams:** 2.5.1 (provider fixes) is foundation. 2.5.2-2.5.5 depend on it. 2.5.6 is independent.

- [x] Awesome-list analyzer — `internal/awesome/` fetches, indexes, diffs, and reports on awesome-list repos for competitive intelligence (15 files) `[reconciled 2026-03-26]`
- [x] Multi-armed bandit provider selection — `internal/bandit/` with UCB1 selector, policy framework, arm tracking for dynamic provider routing `[reconciled 2026-03-26]`
- [x] Blackboard shared state — `internal/blackboard/` with CAS-based key-value store for cross-agent coordination `[reconciled 2026-03-26]`

### 2.5.1 — Fix provider CLI command builders (COMPLETE)
- [x] 2.5.1.1 — Fix buildCodexCmd: `codex exec PROMPT --json --full-auto` (not `--quiet`)
- [x] 2.5.1.2 — Fix buildGeminiCmd: add `-p` flag and `--yolo` for headless mode
- [x] 2.5.1.3 — Fix Codex prompt delivery (positional arg, not stdin)
- [x] 2.5.1.4 — Fix npm package name in docs (`@google/gemini-cli`)
- [x] 2.5.1.5 — Update provider test suite for correct CLI flags
- **Acceptance:** codex and gemini sessions launchable via MCP tools

### 2.5.2 — Per-provider agent discovery (COMPLETE)
- [x] 2.5.2.1 — Discover `.gemini/commands/*.toml` for Gemini provider
- [x] 2.5.2.2 — Parse `AGENTS.md` sections for Codex provider
- [x] 2.5.2.3 — Add `Provider` field to `AgentDef` type
- [x] 2.5.2.4 — Wire provider param into `agent_list` and `agent_define` MCP tools
- **Acceptance:** `agent_list` returns provider-specific agent definitions

### 2.5.3 — Cross-provider team delegation (COMPLETE)
- [x] 2.5.3.1 — Add per-task provider override in `TeamTask`
- [x] 2.5.3.2 — Generate provider-aware delegation prompts for lead sessions
- [x] 2.5.3.3 — Update `team_create` with `worker_provider` default param
- [x] 2.5.3.4 — Update `team_delegate` with optional `provider` param
- **Acceptance:** provider-aware team lead delegates tasks across Gemini/Codex/Claude workers

### 2.5.4 — Provider-specific resume support (COMPLETE)
- [x] 2.5.4.1 — Probe Codex CLI resume support and allow `codex exec resume` when available
- [x] 2.5.4.2 — Verify Gemini `--resume` flag works with `stream-json`
- [x] 2.5.4.3 — Add resume tests per provider, including install-dependent Codex capability fallback
- **Acceptance:** `session_resume` works for Claude/Gemini and for Codex installs that expose `exec resume`

### 2.5.5 — Unified cost normalization `[BLOCKED BY 2.5.1]`
- [x] 2.5.5.1 — Verify Codex `--json` cost output fields, update normalizer
- [x] 2.5.5.2 — Verify Gemini `stream-json` cost output fields, update normalizer
- [x] 2.5.5.3 — Add provider-specific cost fallback (parse stderr for cost if not in JSON) `P1` `M`
- **Acceptance:** `cost_usd` tracked accurately for all providers

### 2.5.6 — Batch API integration `[PARALLEL — independent]`
- [x] 2.5.6.1 — Research: map batch API endpoints for Claude, Gemini, OpenAI (~50% cost) `[reconciled 2026-03-26]`
- [x] 2.5.6.2 — Add `BatchOptions` to `LaunchOptions` (batch mode flag, callback URL) `P1` `M`
- [x] 2.5.6.3 — Implement batch submission for Claude (Messages Batches API) — `internal/batch/claude.go` `[reconciled 2026-03-26]`
- [x] 2.5.6.4 — Implement batch submission for Gemini (Batch Prediction API) — `internal/batch/gemini.go` `[reconciled 2026-03-26]`
- [x] 2.5.6.5 — Implement batch polling/webhook for result collection `P1` `L`
- **Acceptance:** batch tasks submitted and results collected for at least one provider

## Phase 2.75: Architecture & Capability Extensions (COMPLETE)

Built across multiple implementation sessions. Extends the TUI, MCP server, and internal architecture with event-driven patterns, new tools, and interactive components.

### 2.75.1 — TUI Polish & Distribution (COMPLETE)
- [x] 4-tab layout with bubbles tab bar (Repos, Sessions, Teams, Fleet)
- [x] Sparkline charts via ntcharts for cost trends
- [x] 4 themes: k9s (default), dracula, nord, solarized (`internal/tui/styles/theme.go`)
- [x] Desktop notifications — macOS `osascript`, Linux `notify-send` (`internal/notify/`)
- [x] GoReleaser config (`.goreleaser.yaml`)
- [x] Diff view for repo git changes (`internal/tui/views/diffview.go`)

### 2.75.2 — Event Bus & Hook System (COMPLETE)
- [x] Internal pub/sub event bus (`internal/events/bus.go`) with ring buffer history (1000 events)
- [x] Event types: session lifecycle, cost updates, budget exceeded, loop started/stopped, scan complete, config changed
- [x] Bus wired into session manager, process manager, MCP server
- [x] Hook executor (`internal/hooks/hooks.go`) with sync/async hook dispatch
- [x] Hook config via `.ralph/hooks.yaml` per repo
- [x] `ralphglasses_event_list` MCP tool for querying recent events

### 2.75.3 — MCP Tool Extensions (COMPLETE, 38 total, +11 new)
- [x] `ralphglasses_event_list` — Query recent fleet events
- [x] `ralphglasses_fleet_analytics` — Cost breakdown by provider/repo/time-period
- [x] `ralphglasses_session_compare` — Compare two sessions (cost, turns, duration)
- [x] `ralphglasses_session_output` — Get recent output from running session
- [x] `ralphglasses_repo_health` — Composite health score (0-100)
- [x] `ralphglasses_session_retry` — Re-launch failed session with same params
- [x] `ralphglasses_config_bulk` — Get/set `.ralphrc` values across multiple repos
- [x] `ralphglasses_agent_compose` — Create composite agent by layering existing agents
- [x] `ralphglasses_workflow_define` — Define multi-step YAML workflows
- [x] `ralphglasses_workflow_run` — Execute workflows with dependency ordering
- [x] `ralphglasses_snapshot` — Save/list fleet state snapshots

### 2.75.4 — TUI Capability Extensions (COMPLETE)
- [x] Confirm dialog component — modal overlay for destructive actions
- [x] Multi-select in tables — space to toggle, batch stop
- [x] Actions menu — context-dependent quick actions via `a` key
- [x] Session launcher — inline form to launch sessions via `L` key
- [x] Session output streaming — real-time output view via `o` key
- [x] Timeline view — horizontal bar chart of session lifetimes via `t` key
- [x] Enhanced fleet dashboard — provider bar charts, cost-per-turn, budget gauges, top 5 expensive sessions

### 2.75.5 — Code Organization (COMPLETE)
- [x] Extracted key handlers to `internal/tui/handlers_*.go` (~1163 lines across 4 files) `[reconciled 2026-03-26]`
- [x] Extracted fleet data builder to `internal/tui/fleet_builder.go` (~200 lines)
- [x] `app.go` focused on Model/Init/Update/View (~237 lines) `[reconciled 2026-03-26]`

---

## Phase 3: i3 Multi-Monitor Integration

> **Depends on:** Phase 2 (session model, fleet dashboard)
>
> **Parallel workstreams:** 3.1 (i3 IPC) is the foundation. 3.4 (autorandr) is independent. 3.5 (Sway) can proceed in parallel with 3.2. 3.3 depends on 3.1 + 2.1 (SQLite). 3.6 (Hyprland) is independent.

### 3.1 — i3 IPC client
- [x] 3.1.1 — Create `internal/i3/` package wrapping go-i3: connect to i3 socket, subscribe to events `P1` `M`
- [x] 3.1.2 — Workspace CRUD: create named workspace, move to output, rename, close `P1` `M`
- [x] 3.1.3 — Window management: focus, move-to-workspace, set layout (splitv/splith/tabbed/stacked) `P1` `M`
- [x] 3.1.4 — Monitor enumeration: list outputs via i3 IPC (name, resolution, position) `P1` `S`
- [x] 3.1.5 — Event listener: workspace focus, window create/close, output connect/disconnect `P1` `M`
- **Acceptance:** programmatic workspace creation and window placement from Go

### 3.2 — Monitor layout manager `[BLOCKED BY 3.1]`
- [x] 3.2.1 — Define layout presets as JSON: "dev" (agents + logs), "fleet" (all sessions), "focused" (single agent) `P1` `M`
- [x] 3.2.2 — 7-monitor workspace assignment config (`distro/i3/workspaces.json`) `P1` `S`
- [ ] 3.2.3 — TUI command `:layout <name>` — apply preset `P1` `M`
- [ ] 3.2.4 — Save current layout as custom preset (`:layout save <name>`) `P2` `M`
- [x] 3.2.5 — Handle missing monitors gracefully: skip unavailable outputs, log warning, fall back `P1` `S`
- **Acceptance:** `:layout fleet` redistributes windows across monitors

### 3.3 — Multi-instance coordination `[BLOCKED BY 3.1, 2.1]`
- [x] 3.3.1 — Shared state via SQLite: same DB file, WAL mode, `PRAGMA busy_timeout` `P1` `L`
- [x] 3.3.2 — Instance discovery: Unix domain socket per instance, advertise PID and capabilities `P1` `M`
- [x] 3.3.3 — Leader election: simple file-lock based leader for fleet operations `P1` `M`
- [x] 3.3.4 — Leader failover: detect leader crash via heartbeat, re-elect `P2` `M`
- **Acceptance:** two ralphglasses instances share session state without corruption

### 3.4 — autorandr integration `[PARALLEL — independent]`
- [ ] 3.4.1 — Detect monitor connects/disconnects via i3 output events or udev `P2` `M`
- [ ] 3.4.2 — Auto-apply saved autorandr profiles on hotplug `P2` `M`
- [ ] 3.4.3 — Generate autorandr profiles from current xrandr state `P2` `M`
- [ ] 3.4.4 — Link autorandr profiles to layout presets `P2` `M`
- **Acceptance:** monitor hot-plug triggers layout restore

### 3.5 — Sway/Wayland compatibility `[PARALLEL]` — **PRIMARY COMPOSITOR**
- [x] 3.5.1 — Abstract WM interface: `internal/wm/` with i3 and Sway backends `P2` `L`
- [x] 3.5.2 — Sway IPC client: `internal/wm/sway/client.go` (208 LOC, i3-ipc protocol) `P2` `M`
- [x] 3.5.3 — Auto-detect WM at startup: check `$SWAYSOCK` vs `$I3SOCK` `P2` `S`
- [x] 3.5.4 — Test suite: `internal/wm/sway/integration_test.go` (580 LOC) `P2` `M`
- [x] 3.5.5 — Sway distro configs: `distro/sway/config`, `kiosk-config`, waybar `P1` `L`
- [x] 3.5.6 — NVIDIA Wayland env: `distro/sway/environment.d/nvidia-wayland.conf` `P1` `S`
- [x] 3.5.7 — Sway kiosk setup: `distro/scripts/sway-kiosk-setup.sh` `P1` `M`
- [x] 3.5.8 — Waybar status bar: custom fleet modules replacing i3blocks `P1` `M`
- [x] 3.5.9 — Extend sway.Output struct with Rect/CurrentMode for monitor layout `P1` `S`
- [x] 3.5.10 — ParseSwayOutputs + DetectMonitors dispatcher in `internal/wm/monitors.go` `P1` `M`
- [x] 3.5.11 — hw-detect.sh `--wayland-only` flag for Sway monitor config `P1` `S`
- [x] 3.5.12 — Manjaro Dockerfile: `distro/Dockerfile.manjaro` (Sway + NVIDIA) `P1` `L`
- [x] 3.5.13 — Systemd services updated for Wayland env vars `P1` `S`
- **Acceptance:** layout commands work on both i3 and Sway; Manjaro boots into Sway kiosk

### 3.6 — Hyprland support `[PARALLEL]`
- [x] 3.6.1 — Hyprland IPC client: `internal/wm/hyprland/` `P2` `M`
- [x] 3.6.2 — Workspace dispatch: `hyprctl dispatch workspace` `P2` `S`
- [x] 3.6.3 — Monitor configuration: `hyprctl monitors` `P2` `S`
- [ ] 3.6.4 — Dynamic workspaces: leverage Hyprland's per-monitor model `P2` `M`
- **Acceptance:** layout commands work on Hyprland

## Phase 3.5: Theme & Plugin Ecosystem

> Inspired by k9s skins + plugins system, Ghostty shader architecture,
> Starship module design, and Claude Code skills framework.

### 3.5.1 — Theme ecosystem (like k9s skins + Ghostty themes)
- [x] 3.5.1.1 — Switch theme colors from ANSI-256 integers to hex strings `P1` `M`
- [x] 3.5.1.2 — Add `snazzy` theme `P1` `S`
- [x] 3.5.1.3 — Add `catppuccin-macchiato` and `catppuccin-mocha` themes `P1` `S`
- [x] 3.5.1.4 — Add `tokyo-night` theme `P2` `S`
- [x] 3.5.1.5 — Support `~/.config/ralphglasses/themes/` external theme directory `P1` `M`
- [x] 3.5.1.6 — Add `--theme` CLI flag and `RALPH_THEME` .ralphrc key `P1` `S`
- [x] 3.5.1.7 — Add `:theme <name>` TUI command for live theme switching `P1` `M`
- **Acceptance:** `ralphglasses --theme snazzy` renders with hex-accurate palette; user themes load correctly

### 3.5.2 — Plugin system v2 (like k9s plugins.yml)
- [x] 3.5.2.1 — Define `~/.config/ralphglasses/plugins.yml` schema `P1` `M`
- [x] 3.5.2.2 — Plugin loader: parse YAML at startup, register keybinds per scope `P1` `M`
- [x] 3.5.2.3 — Variable resolver: substitute runtime context in command args `P1` `M`
- [x] 3.5.2.4 — Built-in plugins: `stern-logs`, `gh-pr`, `session-cost-report` `P2` `L`
- [x] 3.5.2.5 — Plugin shortcut display in help view `P2` `S`
- [x] 3.5.2.6 — MCP tool for plugin management `P2` `M`
- **Acceptance:** user-defined YAML plugins execute commands with variable substitution from TUI

### 3.5.3 — Resource aliases (like k9s aliases.yml)
- [x] 3.5.3.1 — Define `~/.config/ralphglasses/aliases.yml` schema `P2` `S`
- [x] 3.5.3.2 — Built-in aliases: `:rp` -> repos, `:ss` -> sessions, `:tm` -> teams, `:fl` -> fleet `P2` `S`
- [x] 3.5.3.3 — User-defined command aliases `P2` `M`
- **Acceptance:** `:alias-name` in command mode executes mapped command

### 3.5.4 — MCP skill export (like Claude Code skills)
- [x] 3.5.4.1 — Generate `.claude/skills/ralphglasses/SKILL.md` from MCP tool descriptions `P1` `M`
- [x] 3.5.4.2 — Include YAML frontmatter with allowed-tools `P1` `S`
- [x] 3.5.4.3 — Auto-update skill on `ralphglasses mcp` server start `P1` `S`
- [x] 3.5.4.4 — Mirror generated skill to `.agents/skills/ralphglasses/SKILL.md` for Codex-native skill discovery `P1` `S`
- [x] 3.5.4.5 — Add Codex plugin bundle generation for repo-local MCP affordances `P1` `M`
- **Acceptance:** provider-native skill surfaces exist for both Claude Code and Codex

### 3.5.5 — Codex-primary command/control parity `[NEW]`
- [x] 3.5.5.1 — Make Codex the default provider for session launch/resume, teams, RC, workflow launches, and fleet worker execution `P0` `M`
- [x] 3.5.5.2 — Move self-improvement and sweep defaults to Codex-first planner/worker profiles `P0` `M`
- [x] 3.5.5.3 — Update failover and cascade defaults so Codex is the primary control-plane lane and Claude is the expensive reasoning specialist `P1` `M`
- [x] 3.5.5.4 — Pin Codex developer docs and local CLI capability notes in-repo for future sessions `P1` `S`
- [x] 3.5.5.5 — Add Codex plugin/subagent export flows alongside AGENTS.md skill export `P1` `M`
- [x] 3.5.5.6 — Remove remaining Claude-biased defaults from `internal/enhancer/` and `cmd/prompt-improver/` when provider/target is omitted `P0` `M`
- [x] 3.5.5.7 — Make MCP Sampling and hybrid enhancement paths derive target prompt style from Codex/OpenAI-first runtime defaults instead of implicit Claude behavior `P1` `M`
- [x] 3.5.5.8 — Add targeted regression tests proving omitted-provider prompt improvement paths prefer OpenAI/Codex semantics over Claude-specific scoring/tone rules `P1` `S`
- **Implementation notes (2026-04-04):**
  - Codex-primary parity is complete for the currently shipped control-plane workflows: sessions, resume, RC, teams, fleet worker discovery, loops, sweeps, self-improve, prompt enhancement, skill/plugin export, and operator docs.
  - Remaining roadmap work beyond this section is general platform work, not a Codex parity blocker, unless a new shipped workflow regresses to Claude-first behavior.
  - The key ongoing risk is silent drift: future provider-default edits can reintroduce Claude-biased copy or omitted-provider behavior if code and docs are not updated together.
  - If a future session genuinely needs Claude Code to unblock a parity item, do not switch ad hoc. Write a focused Claude Code prompt, copy it to the paste buffer, record the reason in the roadmap/session notes, and keep the Codex-led branch as the source of truth.
- **Acceptance:** omitted-provider control paths default to Codex and repo docs match runtime behavior

### 3.5.6 — Theme export to terminal (like claudekit themekit)
Partially complete: `internal/themekit/` ported from claudekit `[reconciled 2026-03-27]`
- [x] 3.5.6.1 — `ralphglasses theme export ghostty` -> generate Ghostty palette config `P2` `S`
- [x] 3.5.6.2 — `ralphglasses theme export starship` -> generate Starship color overrides `P2` `S`
- [x] 3.5.6.3 — `ralphglasses theme export k9s` -> generate k9s skin.yml `P2` `S`
- [x] 3.5.6.4 — `ralphglasses theme sync` -> export to all supported tools simultaneously `P2` `M`
- **Acceptance:** `ralphglasses theme sync` updates Ghostty, Starship, and k9s to match TUI theme

---

## Phase 4: Bootable Thin Client

> **Depends on:** Phase 3 (i3 integration, monitor layout)
>
> **Parallel workstreams:** 4.1 (ISO pipeline) is the foundation. 4.3 (PXE) and 4.6 (OTA) can proceed in parallel. 4.7 (watchdog) is independent. 4.5 (install-to-disk) depends on 4.1. 4.8 (marathon hardening) is independent.

### 4.1 — Dockerfile -> ISO pipeline
**Completed:**
- [x] `distro/Dockerfile` — Ubuntu 24.04, kernel 6.12+ HWE, NVIDIA 550, i3, Go, Claude Code
- [x] `distro/scripts/hw-detect.sh` — GPU detection, GTX 1060 blacklisting, MT7927 BT blacklisting
- [x] `distro/systemd/hw-detect.service` — Oneshot first-boot hardware detection
- [x] `distro/systemd/ralphglasses.service` — TUI autostart after graphical target

**Remaining:**
- [x] 4.1.1 — `distro/Makefile` target `build`: `docker build` with build args `P1` `M`
- [x] 4.1.2 — `distro/Makefile` target `squashfs`: extract rootfs, `mksquashfs` with xz `P1` `M`
- [x] 4.1.3 — `distro/Makefile` target `iso`: `grub-mkrescue` with EFI + BIOS `P1` `M`
- [x] 4.1.4 — QEMU smoke test script: boot ISO, verify TUI starts `P1` `M`
- [ ] 4.1.5 — CI integration: build ISO in GitHub Actions, upload as artifact `P2` `L`
- [x] 4.1.6 — Fix Xorg config: remove hardcoded PCI `BusID`, use hw-detect.sh output `P1` `S`
- [ ] 4.1.7 — Fix networking priority: align Dockerfile with docs (Intel I226-V primary) `P1` `S`
- **Acceptance:** `make iso` produces bootable image, boots in QEMU

### 4.2 — i3 kiosk configuration `[BLOCKED BY 4.1]`
- [ ] 4.2.1 — `distro/i3/config` — workspace-to-output mapping for 7 monitors `P1` `M`
- [x] 4.2.2 — Strip WM chrome: `default_border none`, no desktop, no dmenu `P1` `S`
- [x] 4.2.3 — Keybindings: workspace navigation, TUI focus, emergency shell `P1` `S`
- [x] 4.2.4 — Auto-start: launch ralphglasses fullscreen on workspace 1 `P1` `S`
- [x] 4.2.5 — Lock screen: disable screen blanking, DPMS off (24/7 operation) `P1` `S`
- [ ] 4.2.6 — Template monitor names: replace hardcoded DP-1/DP-2 with hw-detect.sh values `P1` `S`
- **Acceptance:** boots to fullscreen TUI, no visible WM chrome

### 4.3 — PXE/network boot `[PARALLEL]`
- [ ] 4.3.1 — iPXE chainload config `P2` `M`
- [ ] 4.3.2 — LTSP server setup on UNRAID `P2` `L`
- [ ] 4.3.3 — Network boot squashfs overlay `P2` `M`
- [ ] 4.3.4 — Fallback: USB boot `P2` `M`
- [ ] 4.3.5 — Boot menu: select version `P2` `S`
- **Acceptance:** PXE boot from UNRAID reaches ralphglasses TUI

### 4.4 — Hardware profiles
- [x] ProArt X870E-CREATOR WIFI — primary target (documented in `distro/hardware/proart-x870e.md`)
- [ ] 4.4.1 — Generalize `hw-detect.sh`: PCI ID table with per-device actions `P1` `M`
- [ ] 4.4.2 — Hardware profile schema: JSON manifest with PCI IDs, required modules `P2` `M`
- [ ] 4.4.3 — Validate profiles against running system `P2` `M`
- [ ] 4.4.4 — Template for adding new boards `P2` `S`
- **Acceptance:** hw-detect.sh correctly identifies and configures target hardware

### 4.5 — Install-to-disk `[BLOCKED BY 4.1]`
- [ ] 4.5.1 — `install-to-disk.sh`: partition scheme (512MB ESP + ext4 rootfs) `P2` `L`
- [ ] 4.5.2 — GRUB install: UEFI mode `P2` `M`
- [ ] 4.5.3 — First-boot setup `P2` `M`
- [ ] 4.5.4 — ZFS root option `P2` `L`
- [ ] 4.5.5 — Safety: require `--confirm` flag `P2` `S`
- **Acceptance:** install-to-disk produces bootable system on NVMe

### 4.6 — OTA update mechanism `[PARALLEL]`
- [ ] 4.6.1 — Version check: compare local squashfs hash against remote manifest `P2` `M`
- [ ] 4.6.2 — Download + verify: fetch, SHA256, GPG signature `P2` `M`
- [ ] 4.6.3 — Atomic swap: A/B partition scheme `P2` `L`
- [ ] 4.6.4 — `ralphglasses update` subcommand `P2` `M`
- **Acceptance:** OTA update replaces running image, rollback works on boot failure

### 4.7 — Health watchdog service `[PARALLEL]`
- [x] 4.7.1 — Systemd watchdog unit `P1` `S`
- [ ] 4.7.2 — Hardware health checks: GPU temp, disk space, memory `P1` `M`
- [ ] 4.7.3 — Alert escalation `P2` `M`
- [x] 4.7.4 — Heartbeat file `P1` `S`
- **Acceptance:** TUI auto-restarts within 10s of crash

### 4.8 — Marathon.sh hardening `[PARALLEL]`
- [ ] 4.8.1 — Disk space monitoring `P1` `S`
- [ ] 4.8.2 — Memory pressure monitoring `P1` `S`
- [x] 4.8.3 — Fix restart logic: cap MAX_RESTARTS, exponential backoff `P0` `M`
- [ ] 4.8.4 — Add `bc` availability check `P2` `S`
- [x] 4.8.5 — Marathon summary report on completion `P2` `M`
- **Acceptance:** marathon.sh survives disk fill and memory pressure

### 4.9 — Secure boot support `[PARALLEL]`
- [ ] 4.9.1 — Sign kernel and bootloader with custom MOK `P2` `L`
- [ ] 4.9.2 — Sign NVIDIA kernel modules `P2` `M`
- [ ] 4.9.3 — MOK enrollment first-boot flow `P2` `M`
- [ ] 4.9.4 — Document Secure Boot setup `P2` `S`
- **Acceptance:** system boots with Secure Boot enabled + NVIDIA driver loaded

### 4.10 — USB provisioning tool `[BLOCKED BY 4.1]`
- [ ] 4.10.1 — `ralphglasses flash <iso> <device>` — write ISO with progress `P2` `M`
- [ ] 4.10.2 — Persistent overlay partition `P2` `M`
- [ ] 4.10.3 — Pre-seed config `P2` `M`
- [ ] 4.10.4 — Verify write: read-back checksums `P2` `S`
- **Acceptance:** `ralphglasses flash` produces bootable USB with pre-loaded config

---

## Phase 5: Agent Sandboxing & Infrastructure

> **Depends on:** Phase 2 (session model needed for container lifecycle)
>
> **Parallel workstreams:** 5.1 (Docker) and 5.2 (Incus) are parallel sandboxing approaches. 5.3 (MCP gateway) is independent. 5.4 (network) depends on 5.1 or 5.2. 5.6 (secrets) is independent. 5.7-5.8 are independent.

### 5.1 — Docker sandbox mode
- [x] 5.1.1 — `internal/sandbox/` package: create container, manage lifecycle — `docker.go` `[reconciled 2026-03-26]`
- [x] 5.1.2 — Container spec: bind-mount workspace, set `--cpus`, `--memory`, `--network` `[reconciled 2026-03-26]`
- [x] 5.1.3 — Lifecycle binding: session start -> container start, session stop -> container stop + remove `[reconciled 2026-03-26]`
- [x] 5.1.4 — Log forwarding: capture container stdout/stderr -> session log stream `P1` `M`
- [x] 5.1.5 — GPU passthrough: `--gpus` flag for NVIDIA containers `P2` `M`
- **Acceptance:** session runs inside container, cleanup on stop

### 5.2 — Incus/LXD containers
- [x] 5.2.1 — `internal/sandbox/incus/` package: Incus client, profile management `P2` `L`
- [ ] 5.2.2 — Per-container credential isolation `P2` `M`
- [ ] 5.2.3 — Workspace persistence: bind-mount + snapshot `P2` `M`
- [ ] 5.2.4 — Threat detection: suspicious file access, network, resource spikes `P2` `L`
- [ ] 5.2.5 — Port patterns from code-on-incus `P2` `M`
- **Acceptance:** session runs in Incus container with isolated credentials

### 5.3 — MCP gateway `[PARALLEL]`
- [x] 5.3.1 — Central MCP hub service `P1` `L`
- [x] 5.3.2 — Per-session tool authorization `P1` `M`
- [x] 5.3.3 — Audit logging `P1` `M`
- [x] 5.3.4 — Rate limiting `P1` `M`
- [ ] 5.3.5 — Deploy to UNRAID `P2` `M`
- **Acceptance:** agent tool calls routed through gateway with audit trail

### 5.4 — Network isolation `[BLOCKED BY 5.1 or 5.2]`
- [ ] 5.4.1 — VLAN segmentation `P2` `L`
- [ ] 5.4.2 — iptables/nftables allowlists `P2` `M`
- [ ] 5.4.3 — DNS sinkholing `P2` `M`
- [ ] 5.4.4 — Network policy config in `.ralphrc` `P2` `S`
- **Acceptance:** sandboxed session cannot reach unauthorized endpoints

### 5.5 — Budget federation `[BLOCKED BY 2.3]`
- [x] 5.5.1 — Global budget pool `P1` `M`
- [ ] 5.5.2 — Per-session limits with carry-over `P1` `M`
- [ ] 5.5.3 — Budget dashboard view `P1` `M`
- [ ] 5.5.4 — Anthropic billing API integration `P2` `L`
- [x] 5.5.5 — Budget alerts `P1` `S`
- **Acceptance:** global pool enforced across all active sessions

### 5.6 — Secret management `[PARALLEL]`
- [x] 5.6.1 — Secret provider interface: `internal/secrets/` `P2` `M`
- [x] 5.6.2 — SOPS backend `P2` `M`
- [ ] 5.6.3 — Vault backend `P2` `L`
- [ ] 5.6.4 — Secret rotation `P2` `M`
- [ ] 5.6.5 — Audit: log secret access per session `P2` `S`
- **Acceptance:** API keys loaded from Vault/SOPS, never stored in plaintext config

### 5.7 — Firecracker microVM sandbox `[PARALLEL]`

> **Research:** E2B achieves ~150ms cold start with Firecracker. Daytona achieves ~90ms with Docker. Industry consensus: Firecracker for untrusted code, gVisor for trusted agents needing syscall-level isolation.

- [x] 5.7.1 — `internal/sandbox/firecracker/` package `P2` `L`
- [ ] 5.7.2 — Boot kernel + rootfs (target: <200ms cold start, <5MiB RAM per sandbox) `P2` `L`
- [ ] 5.7.3 — virtio-fs workspace mount `P2` `M`
- [x] 5.7.4 — Resource limits (CPU, memory, network, disk I/O) `P2` `M`
- [ ] 5.7.5 — Snapshot/restore (24h sandbox lifetime for marathon sessions) `P2` `L`
- [ ] 5.7.6 — E2B-compatible sandbox API: `Create()`, `Execute()`, `Filesystem()`, `Terminate()` `P2` `M`
- **Acceptance:** session runs in Firecracker microVM with <200ms boot time

### 5.8 — gVisor runtime `[PARALLEL]`

> **Research:** gVisor provides syscall-level isolation with 10-30% I/O overhead (acceptable for CPU/network-bound agents). Google's kubernetes-sigs/agent-sandbox uses gVisor + Kata. Sweet spot for thin client's trusted-but-isolated agents.

- [x] 5.8.1 — Configure `runsc` as OCI runtime alternative `P2` `M`
- [x] 5.8.2 — gVisor sandbox profile (seccomp + AppArmor for defense-in-depth) `P2` `M`
- [ ] 5.8.3 — Performance benchmarking vs Docker `runc` (target: <30% overhead) `P2` `M`
- [ ] 5.8.4 — Fallback logic: detect gVisor, fall back to runc `P2` `S`
- [ ] 5.8.5 — Add gVisor runtime to `distro/` thin client config `P1` `S`
- **Acceptance:** sessions optionally run under gVisor; thin client defaults to gVisor

---

## Phase 6: Advanced Fleet Intelligence

> **Depends on:** Phase 2 (sessions) + Phase 5 (sandboxing). Phase 2.75 event bus provides foundation.
>
> **Parallel workstreams:** 6.1 (native loop) and 6.6 (model routing) can proceed in parallel. 6.3 (coordination) depends on 6.1. 6.4 (analytics) and 6.5 (notifications) are independent. 6.7 (replay) depends on 6.4. 6.8-6.10 are independent.

### 6.1 — Native ralph loop engine
Partially complete: `internal/session/loop.go`, `loop_worker.go`, `loop_helpers.go`, `loop_noop.go`, `loop_validate.go` implement `StepLoop` `[reconciled 2026-03-27]`
- [x] 6.1.1 — StepLoop implementation with iteration management, observation tracking `[reconciled 2026-03-27]`
- [x] 6.1.4 — Parallel execution: run independent tasks concurrently `[reconciled 2026-03-27]`
- [x] 6.1.5 — Progress telemetry: structured events to session event log `[reconciled 2026-03-27]`
- [x] 6.1.2 — Typed task specs: define task schema (inputs, outputs, dependencies) as Go structs `P1` `M`
- [ ] 6.1.3 — DAG visualization in TUI: show task graph with status `P2` `L`
- **Acceptance:** ralph loop runs natively in Go, DAG visible in TUI

### 6.2 — R&D cycle orchestrator `[BLOCKED BY 6.1]`
- [x] 6.2.1 — Port perpetual improvement loop from claudekit rdcycle `P1` `L`
- [x] 6.2.2 — Self-benchmark: coverage, lint score, build time, binary size per iteration `P1` `M`
- [x] 6.2.3 — Regression detection: compare benchmarks, flag regressions `P0` `M`
- [x] 6.2.4 — Auto-generate improvement tasks from benchmark regressions `P1` `L`
- [x] 6.2.5 — Cycle dashboard: iteration history, benchmark trends `P2` `M`
- **Acceptance:** automated benchmark -> task generation cycle runs unattended

### 6.3 — Cross-session coordination `[BLOCKED BY 6.1, 2.1]`
- [x] 6.3.1 — Shared context store `P1` `M`
- [x] 6.3.2 — Dedup engine `P1` `M`
- [x] 6.3.3 — Dependency ordering `P1` `L`
- [x] 6.3.4 — Conflict resolution `P1` `L`
- [x] 6.3.5 — Coordination dashboard `P2` `M`
- **Acceptance:** two agents targeting same repo don't conflict on same files

### 6.4 — Analytics & observability `[PARALLEL]`
- [x] 6.4.1 — Historical data model: SQLite `P1` `M`
- [x] 6.4.2 — TUI analytics view `P1` `L`
- [x] 6.4.3 — OpenTelemetry traces `P1` `L`
- [x] 6.4.4 — Prometheus metrics endpoint `P1` `M`
- [x] 6.4.5 — Grafana dashboard JSON `P2` `M`
- **Acceptance:** Grafana dashboard shows session metrics over time

### 6.5 — External notifications `[PARALLEL]`
- [x] 6.5.1 — Webhook dispatcher `P2` `M`
- [x] 6.5.2 — Discord integration `P2` `M`
- [x] 6.5.3 — Slack integration `P2` `M`
- [x] 6.5.4 — Notification templates `P2` `S`
- [x] 6.5.5 — Rate limiting and retry `P2` `M`
- **Acceptance:** Discord webhook fires on session completion

### 6.6 — Model routing
- [x] 6.6.1 — Model registry: available models with capabilities, cost/token, context window `P1` `M`
- [x] 6.6.2 — Task-type classifier: map task types to preferred models `P1` `M`
- [x] 6.6.3 — Routing rules in `.ralphrc` `P1` `S`
- [x] 6.6.4 — Dynamic routing: switch model mid-session based on task type `P1` `L`
- [x] 6.6.5 — Cost optimization: suggest cheaper model when task below threshold `P1` `M`
- **Acceptance:** different task types route to appropriate models

### 6.7 — Replay/audit trail `[BLOCKED BY 6.4]`
- [x] 6.7.1 — Session recording `P2` `L`
- [x] 6.7.2 — Replay viewer `P2` `L`
- [x] 6.7.3 — Export as Markdown/JSON `P2` `M`
- [x] 6.7.4 — Diff view: compare two session replays `P2` `L`
- [x] 6.7.5 — Retention policy `P2` `S`
- **Acceptance:** can replay a completed session step-by-step

### 6.8 — Multi-model A/B testing `[PARALLEL]`
- [x] 6.8.1 — A/B test definition `P2` `M`
- [x] 6.8.2 — Metric collection `P2` `M`
- [x] 6.8.3 — Comparison report with statistical significance `P2` `L`
- [ ] 6.8.4 — TUI A/B view `P2` `L`
- [ ] 6.8.5 — Auto-promote default model based on results `P2` `M`
- **Acceptance:** `ralphglasses ab-test --model-a opus --model-b sonnet` produces comparison

### 6.9 — Natural language fleet control `[PARALLEL]`
- [x] 6.9.1 — MCP sampling integration `P2` `L`
- [x] 6.9.2 — Command parser `P2` `L`
- [x] 6.9.3 — Intent classifier `P2` `M`
- [ ] 6.9.4 — Confirmation flow `P2` `M`
- [x] 6.9.5 — History: persist and replay commands `P2` `S`
- **Acceptance:** natural language commands execute fleet operations

### 6.10 — Cost forecasting `[PARALLEL]`
- [x] 6.10.1 — Historical cost model `P1` `M`
- [x] 6.10.2 — Budget projection `P1` `M`
- [x] 6.10.3 — TUI forecast widget `P2` `M`
- [x] 6.10.4 — Alert on anomaly: flag >2x predicted rate `P1` `S`
- [x] 6.10.5 — Recommendation engine `P2` `M`
- **Acceptance:** forecast accuracy within 20% after 10+ sessions

---

## Phase 7: Kubernetes & Cloud Fleet

> **Depends on:** Phase 5 (sandbox model) + Phase 6 (fleet intelligence)
>
> **Parallel workstreams:** 7.1 (K8s operator) is the foundation. 7.2 (autoscaling) depends on 7.1. 7.3 (multi-cloud) is independent. 7.4 (cost management) depends on 7.1. 7.5 (GitOps) is independent.

### 7.1 — Kubernetes operator
- [x] 7.1.1 — CRD definition: `RalphSession` custom resource `P2` `L`
- [x] 7.1.2 — Controller: reconcile loop `P2` `XL`
- [x] 7.1.3 — Pod template `P2` `M`
- [ ] 7.1.4 — Status subresource `P2` `M`
- [x] 7.1.5 — RBAC: minimal permissions `P2` `S`
- **Acceptance:** `kubectl apply -f session.yaml` creates and manages a ralph session

### 7.2 — Autoscaling `[BLOCKED BY 7.1]`
- [x] 7.2.1 — HPA integration `P2` `M`
- [ ] 7.2.2 — Node autoscaler hints `P2` `M`
- [ ] 7.2.3 — Budget-aware scaling `P2` `M`
- [ ] 7.2.4 — Scale-to-zero `P2` `M`
- [ ] 7.2.5 — Warm pool `P2` `L`
- **Acceptance:** session count auto-adjusts within budget

### 7.3 — Multi-cloud support `[PARALLEL]`
- [x] 7.3.1 — AWS provider `P2` `L`
- [x] 7.3.2 — GCP provider `P2` `L`
- [x] 7.3.3 — Provider interface: `internal/cloud/` `P2` `M`
- [ ] 7.3.4 — Cross-cloud fleet view `P2` `M`
- [ ] 7.3.5 — Cost comparison `P2` `M`
- **Acceptance:** sessions can launch on AWS or GCP from same TUI

### 7.4 — Cloud cost management `[BLOCKED BY 7.1]`
- [ ] 7.4.1 — Real-time cloud spend tracking `P2` `L`
- [ ] 7.4.2 — Combined budget: API + compute `P2` `M`
- [ ] 7.4.3 — Spot instance strategy `P2` `M`
- [ ] 7.4.4 — Idle resource detection `P2` `S`
- [ ] 7.4.5 — Weekly cost report `P2` `M`
- **Acceptance:** total cost (API + cloud) visible in single budget view

### 7.5 — GitOps deployment `[PARALLEL]`
- [ ] 7.5.1 — Helm chart `P2` `L`
- [ ] 7.5.2 — ArgoCD application `P2` `M`
- [ ] 7.5.3 — Kustomize overlays `P2` `M`
- [ ] 7.5.4 — Sealed secrets `P2` `M`
- [ ] 7.5.5 — Canary deployment `P2` `L`
- **Acceptance:** `git push` to deploy branch triggers automated deployment

---

## Phase 8: Advanced Orchestration & AI-Native Features

> **Depends on:** Phase 6 (fleet intelligence, native loop engine)
>
> **Parallel workstreams:** All sections are independent unless noted.

### 8.1 — Multi-agent collaboration patterns
- [x] 8.1.1 — Architect/worker pattern `P1` `L`
- [x] 8.1.2 — Review chain: code -> review -> fix `P1` `L`
- [x] 8.1.3 — Specialist routing `P1` `M`
- [x] 8.1.4 — Shared memory: cross-session knowledge base `P1` `L`
- [x] 8.1.5 — Communication protocol: structured messages via SQLite queue `P1` `M`
- **Acceptance:** architect/worker pattern produces PRs with automated code review

### 8.2 — Prompt management `[PARALLEL]`
- [x] 8.2.1 — Prompt library: `~/.ralphglasses/prompts/` `P2` `M`
- [x] 8.2.2 — Variable interpolation `P2` `M`
- [x] 8.2.3 — Prompt versioning `P2` `M`
- [ ] 8.2.4 — A/B testing `P2` `L`
- [ ] 8.2.5 — TUI prompt editor `P2` `L`
- **Acceptance:** prompt templates configurable per task type

### 8.3 — Workflow engine `[BLOCKED BY 6.1]`
- [x] 8.3.1 — YAML workflow definitions `P1` `L`
- [ ] 8.3.2 — Built-in workflows: "fix-all-lint", "increase-coverage", "migrate-dependency" `P1` `M`
- [x] 8.3.3 — Workflow executor: parse YAML -> build DAG -> assign `P1` `XL`
- [x] 8.3.4 — Conditional logic `P1` `M`
- [x] 8.3.5 — Workflow marketplace `P2` `L`
- **Acceptance:** YAML workflow runs multi-step, multi-session task to completion

### 8.4 — Code review automation `[PARALLEL]`
- [x] 8.4.1 — PR review agent `P2` `L`
- [x] 8.4.2 — Review criteria `P2` `M`
- [x] 8.4.3 — GitHub integration `P2` `M`
- [ ] 8.4.4 — Auto-approve `P2` `M`
- [ ] 8.4.5 — Review dashboard `P2` `M`
- **Acceptance:** agent-created PRs automatically reviewed

### 8.5 — Self-improvement engine `[BLOCKED BY 6.2]`
Partially complete: `internal/session/reflexion.go`, `episodic.go`, `cascade.go`, `curriculum.go`, `autooptimize.go` implement core subsystems `[reconciled 2026-03-27]`
- [x] 8.5.1 — Reflexion store: verbal reinforcement learning for self-improvement `[reconciled 2026-03-27]`
- [x] 8.5.2 — Episodic memory: Jaccard/cosine similarity for experience retrieval `[reconciled 2026-03-27]`
- [x] 8.5.3 — Cascade router: try-cheap-then-escalate routing strategy `[reconciled 2026-03-27]`
- [x] 8.5.4 — Curriculum sorter: difficulty scoring for task ordering `[reconciled 2026-03-27]`
- [x] 8.5.5 — Meta-agent: session that monitors other sessions' effectiveness `P1` `XL` `[reconciled 2026-03-29]`
  - Implemented as Supervisor in `internal/session/supervisor.go` — HealthMonitor evaluates metrics, CycleChainer feeds synthesis into next cycle
- [x] 8.5.6 — Config optimization: suggest `.ralphrc` changes based on observed patterns `P1` `L`
- [x] 8.5.7 — Prompt evolution: mutate and test prompts, keep highest-performing variants `P2` `L`
- [x] 8.5.8 — Report generation: weekly summary of fleet performance, trends, recommendations `P2` `M`
- **Acceptance:** meta-agent produces actionable configuration improvements

### 8.6 — Codebase knowledge graph `[PARALLEL]`
- [x] 8.6.1 — Parse codebase: packages, types, functions, dependencies `P2` `L`
- [x] 8.6.2 — Store in SQLite `P2` `M`
- [x] 8.6.3 — Query API `P2` `M`
- [ ] 8.6.4 — TUI graph view `P2` `XL`
- [x] 8.6.5 — Context injection: provide graph context to agents `P2` `L`
- **Acceptance:** knowledge graph queries return accurate code relationships

---

## Phase 9: R&D Cycle Automation `[NEW]`

New phase addressing the critical pipeline gap between findings and actionable work. Derived from analysis of 15 R&D cycles (174+ findings, 9.8% resolution rate). Goal: close the Finding -> Roadmap -> Task -> Execute -> Verify loop.

> **Depends on:** Phase 0.8 (scratchpad tools, observation query)
>
> **Parallel workstreams:** Tier 1 tools are critical path. Tier 2 depends on Tier 1 foundations. Tier 3 is independent.

### Architecture

```
                    +---------+
                    | OBSERVE |  <-- loop_poll, observation_query
                    +----+----+
                         |
                         v
+--------+     +----+--------+----+     +----------+
| PLAN   | <-- | ANALYZE/LEARN   | --> | SCHEDULE |
+---+----+     +-----------------+     +----+-----+
    |          scratchpad_reason,           |
    |          finding_to_task           cron/cycle_schedule
    v                                      |
+---+----+                                 v
| EXECUTE|  <-- loop_start, fleet_submit   |
+---+----+                                 |
    |                                      |
    v                                      |
+---+------+     +----------+              |
| VERIFY   | --> | BASELINE | <------------+
+----------+     +----------+
  merge_verify    cycle_baseline
```

### 9.1 — Tier 1: Critical Path (P0)

These tools close the primary pipeline breaks.

#### 9.1.1 — `finding_to_task` `P0` `M`
Convert scratchpad findings into loop-executable tasks.
- Inputs: `finding_id`, `scratchpad_name`
- Outputs: task spec with difficulty estimate, provider hint, estimated cost, file paths
- Logic: parse finding text, identify affected files via grep, classify severity, estimate effort
- Namespace: `loop`
- File: `internal/mcpserver/tools_loop.go`
- **Acceptance:** `finding_to_task FINDING-240 cycle15_tool_exploration` produces valid loop task spec

#### 9.1.2 — `cycle_merge` `P0` `L`
Auto-merge parallel worktree results from concurrent loop iterations.
- Inputs: `worktree_paths[]`, `conflict_strategy` (ours/theirs/manual)
- Outputs: merge result, conflicts list, merged branch name
- Logic: sequential merge with conflict detection, rollback on failure
- Namespace: `loop`
- File: `internal/session/merge.go`
- **Acceptance:** merges 3 parallel worktrees, reports conflicts without data loss

#### 9.1.3 — `cycle_plan` `P0` `L`
Generate next R&D cycle plan from previous observations.
- Inputs: `previous_cycle_id`, `max_tasks`, `budget_usd`
- Outputs: prioritized task list with difficulty, provider assignments, dependencies
- Logic: read observations -> identify regressions -> check unresolved findings -> generate plan
- Namespace: `loop`
- File: `internal/session/cycle_plan.go`
- **Acceptance:** generates coherent cycle plan from observation history

#### 9.1.4 — `cycle_schedule` `P1` `M`
Schedule cycles with cron triggers for unattended operation.
- Inputs: `cron_expr`, `cycle_config` (budget, max_tasks, target_repo)
- Outputs: `schedule_id`, next execution time
- Logic: register cron job, persist to `.ralph/schedules.json`, trigger via internal ticker
- Namespace: `loop`
- File: `internal/session/scheduler.go`
- **Acceptance:** `cycle_schedule "0 2 * * *" ...` runs daily cycle at 2 AM

#### 9.1.5 — `cycle_baseline` `P0` `S`
Snapshot metrics at cycle start for before/after comparison.
- Inputs: `repo_path`, `metrics[]` (coverage, lint_score, build_time, binary_size, test_count)
- Outputs: `baseline_id`, metric values, timestamp
- Logic: run `go test -cover`, `go vet`, `go build`, record results
- Namespace: `loop`
- File: `internal/session/baseline.go`
- **Acceptance:** `cycle_baseline . coverage,build_time` records reproducible snapshot

### 9.2 — Tier 2: Loop Quality (P1)

#### 9.2.1 — `loop_replay` `P1` `M`
Replay failed iterations with modified parameters.
- Inputs: `loop_id`, `iteration_index`, `overrides` (model, budget, prompt)
- Outputs: replay result, diff from original
- Namespace: `loop`
- **Acceptance:** replay produces different outcome when model/prompt changed

#### 9.2.2 — `loop_budget_forecast` `P1` `S`
Predict iteration costs before execution.
- Inputs: `task_spec`, `model`, `historical_window_hours`
- Outputs: estimated cost, confidence interval, historical comparables
- Namespace: `loop`
- **Acceptance:** forecast within 1.5x of actual cost on 80% of predictions

#### 9.2.3 — `loop_diff_review` `P1` `L`
Auto-review worker diffs against planner intent.
- Inputs: `loop_id`, `iteration_index`
- Outputs: review result (pass/warn/fail), alignment score, specific concerns
- Namespace: `loop`
- **Acceptance:** catches unrelated changes, missing test coverage, scope creep

#### 9.2.4 — `scratchpad_reason` enhancement `P1` `S`
Add LLM-powered reasoning over findings (enhance existing tool).
- Add: cross-finding correlation, root cause analysis, suggested fixes
- Namespace: `observability` (existing)
- **Acceptance:** produces root cause analysis linking related findings

#### 9.2.5 — `observation_correlate` `P1` `M`
Cross-reference observations with git commits.
- Inputs: `loop_id`, `time_range`
- Outputs: observation-commit pairs, files changed per observation, regression markers
- Namespace: `observability`
- **Acceptance:** links observations to specific git commits that caused them

### 9.3 — Tier 3: Fleet Intelligence (P2) ✅ COMPLETE

#### 9.3.1 — `fleet_capacity_plan` `P2` `S`
- [x] Recommend worker count from queue depth and budget
- Namespace: `fleet`

#### 9.3.2 — `provider_benchmark` `P2` `L`
- [x] Standardized task suite across providers (code gen, explanation, debugging, refactoring, test writing)
- Namespace: `rdcycle`

#### 9.3.3 — `session_handoff` `P2` `XL`
- [x] Transfer context between sessions with provider switching
- Namespace: `session`

#### 9.3.4 — `prompt_ab_test` `P2` `M`
- [x] A/B test prompts via 10-dimension scoring with confidence levels
- Namespace: `prompt`

#### 9.3.5 — `roadmap_prioritize` `P2` `M`
- [x] Score roadmap items by impact/effort/dependency, return prioritized sprint
- Namespace: `roadmap`

---

## Phase 9.5: Autonomous R&D Supervisor (COMPLETE)

> **Note**: 5 Tier 1 tool implementations (finding_to_task, cycle_merge, cycle_plan, cycle_schedule, cycle_baseline) have no corresponding source files. Effective completion: ~50%.

- [x] 9.5.1 — Supervisor core: persistent goroutine, 60s tick, decision dispatch via DecisionLog
- [x] 9.5.2 — Health monitor: 5-threshold evaluator (completion rate, cost rate, verify rate, idle time, iteration velocity)
- [x] 9.5.3 — Cycle chainer: synthesis → next cycle, lineage tracking, depth cap (10)
- [x] 9.5.4 — Manager wiring: SetAutonomyLevel starts/stops supervisor at level 2
- [x] 9.5.5 — MCP bridge: supervisor_status tool, autonomy_level repo_path parameter
- **Acceptance:** `autonomy_level set=2` enables fully autonomous R&D cycles

## Phase 10: Claude Code Native Integration `[NEW]`

> **Research:** See [docs/claude-code-autonomy-research.md](docs/claude-code-autonomy-research.md) for full background.
>
> **Depends on:** Phase 9.5 (supervisor), Phase 2.75 (event bus)

### 10.1 — Sprint Executor Agent & Batch Integration
- [x] 10.1.1 — Create `.claude/agents/sprint-executor.md` with `isolation: worktree`, `permissionMode: dontAsk`, `model: sonnet` `P1` `S`
- [x] 10.1.2 — Create `.claude/agents/marathon-monitor.md` with `model: haiku` for cheap status polling `P2` `S`
- [ ] 10.1.3 — Integrate `/batch` decomposition for parallel sprint execution (5 ROADMAP items → 5 batch units) `P1` `M`
- [ ] 10.1.4 — Add batch result merging to `cycle_merge.go` for worktree outputs `P1` `M`
- **Acceptance:** `/batch "Implement next 5 ROADMAP items"` produces 5 parallel agents that each implement 1 item

### 10.2 — Cloud Scheduled Tasks & Durable Marathons
- [ ] 10.2.1 — Add cloud scheduled task support for durable marathon execution (replaces `marathon.sh`) `P1` `M`
- [ ] 10.2.2 — Implement session continuation via `--resume` for multi-sprint chains `P1` `M`
- [x] 10.2.3 — Add `supervisor_state.json` restoration for cross-invocation continuity `P1` `S`
- **Acceptance:** Cloud scheduled marathon runs unattended, resumes state across invocations

### 10.3 — Hook-Based Automation
- [x] 10.3.1 — Add PostToolUse hooks for auto-`go vet` / auto-lint on Write/Edit `P2` `S`
- [x] 10.3.2 — Add PreToolUse hooks for Bash safety rules (block `rm -rf`, force push) `P2` `S`
- [x] 10.3.3 — Add Stop hook to force continuation during marathon sessions `P2` `S`
- **Acceptance:** Hooks fire on tool use, enforcing code quality and safety gates automatically

### 10.4 — Permission & Context Management
- [ ] 10.4.1 — Integrate Auto Mode permission level for L2 marathon sessions `P1` `S`
- [ ] 10.4.2 — Add Compact Instructions to CLAUDE.md for marathon context preservation `P1` `S`
- [x] 10.4.3 — Wire `CompactionEnabled` in loop profile to trigger `/compact` between sprints `P1` `M`
- **Acceptance:** Marathon sessions use Auto Mode, compact cleanly between sprints preserving critical state

### 10.5 — Cost Optimization
- [ ] 10.5.1 — Integrate token counting API for accurate pre-cycle budget forecasting `P2` `S`
- [ ] 10.5.2 — Add Batch API support for non-interactive marathon workloads (50% discount) `P2` `M`
- [x] 10.5.3.1 — Track stable repo instruction prefixes from `AGENTS.md`, `CLAUDE.md`, and `GEMINI.md` in prompt-cache analysis `P1` `S`
- [x] 10.5.3.2 — Stop assuming Claude prompt-cache savings by default in shared cache accounting `P0` `S`
- [x] 10.5.3.3 — Detect resumed-Claude cache anomalies when cache writes occur without cache reads `P0` `S`
- [x] 10.5.3.4 — Surface cache-read/cache-write ratios in fleet analytics and session status `P1` `M`
- [x] 10.5.3.5 — Add automatic reroute from Claude to Codex when repeated Claude cache anomalies are detected in long-running orchestration `P1` `M`
- **Implementation notes (2026-04-04):**
  - Claude resumed-session cache safety is now treated as suspect by default in orchestration paths unless live cache reads are actually observed.
  - Session status and fleet analytics expose cache-read/cache-write health so reroute decisions are inspectable rather than implicit.
  - Follow-on work should prefer observable cache-health metrics over theoretical savings, and any Claude-specific optimization should preserve a clean reroute path back to Codex.
- **Acceptance:** Marathon cost per sprint reduced 50-80% vs naive execution

---

## Metrics & KPIs `[NEW]`

Track project health and R&D cycle effectiveness. Data sources: loop_baseline.json (10 observations), 101 journal entries, 7 reflections, 29 episodes.

| Metric | Target | Current | Method | Key Roadmap Items |
|--------|--------|---------|--------|-------------------|
| Finding resolution rate | 40% | 9.8% (17/174+) | findings resolved / total findings | Phase 9 (finding_to_task, cycle_plan) |
| Loop completion rate | 85% | 68.75% | completed / started iterations | QW-3, QW-6, 0.6.5 (stall detection) |
| Loop P50 cost | <$0.05 | $0.0553 | loop_baseline.json | 9.2.2 (budget forecast) |
| Loop P95 cost | <$0.25 | $0.2841 | loop_baseline.json | Cost calibration, cascade routing |
| Loop P50 latency | <120s | 137.5s | loop_baseline.json | Worker turn cap, stall detection |
| Loop P95 latency | <300s | 536s | loop_baseline.json | QW-3 (turn cap), timeout tuning |
| Loop verify pass rate | 100% | 100% | loop_baseline.json | Maintain via regression gates |
| Test coverage | 85% | 83.4% | `go test -coverprofile` | 1.6 (coverage targets) |
| Cost calibration accuracy | <1.2x | 1.19x | predicted / actual cost | 9.2.2 (budget forecast) |
| JSON format retry rate | <5% | 25.7% | retries / total calls | QW-1 |
| Zero-change iteration rate | <10% | 95.6% (22/23) | 0-file-change / total passed | QW-3, 0.6.2 (observation enrichment) |
| Fleet phantom task rate | <5% | 73% (381/523 pending) | phantom / total tasks | QW-11 |
| MCP tool coverage (exercised) | 95% | 71% (82/115) | tools tested / total tools | Phase 9 (provider_benchmark) |
| Cascade routing adoption | 100% | 100% (enabled) | sessions using cascade / total | QW-2 (done) |
| R&D cycle frequency | 1/week | ~2/week | cycles completed per week | 9.1.4 (cycle_schedule) |
| Episodic memory entries | 100+ | 29 | episodes.jsonl count | Self-improvement maturity |
| Learned rules | 5+ | active (QW-12 fixed) | improvement_patterns.json | QW-12 (done) |

---

## MCP Spec Adoption Tracker `[NEW]`

Current spec: **2025-11-25** (latest stable). Official Go SDK: `modelcontextprotocol/go-sdk` v1.4.1 (supersedes mcp-go for new projects).

| Feature | Spec Version | Status | Phase | Notes |
|---------|-------------|--------|-------|-------|
| Structured Output (`outputSchema`) | 2025-06-18 | Implemented | 0.8 | Schemas for 6 tools |
| MCP Logging (`notifications/message`) | 2024-11-05 | Implemented | 0.8 | LoggingMiddleware |
| Tool Annotations (read-only, destructive) | 2025-06-18 | Implemented | 0.8 | `addToolWithMetadata()` in tools_dispatch.go |
| Tasks (async with polling) | 2025-11-25 | Not started | 9.1 | Durable state machines, `tasks/get` polling, `input_required` state |
| Elicitation (server prompts) | 2025-11-25 | Not started | 9.2 | Server→client clarification mid-execution, bridges to HITL |
| Streamable HTTP transport | 2025-11-25 | Not started | 5.3 | Replaces deprecated SSE; single `/mcp` endpoint, `Mcp-Session-Id`, resume via `Last-Event-ID` |
| Progress notifications | 2024-11-05 | Not started | 0.7 | JSON-RPC `notifications/progress` with progressToken |
| Resource subscriptions | 2025-06-18 | Not started | 6.4 | Push notifications on resource changes |
| OAuth/Auth | 2025-11-25 | Not started | 5.3 | OAuth 2.0 for remote MCP servers (required for Streamable HTTP) |
| Roots | 2025-06-18 | Not started | - | Workspace root discovery |
| Sampling | 2024-11-05 | Partial | 6.9 | MCP sampling capability is enabled; higher-level NL fleet control workflows remain roadmap work |

> **Migration note:** `mark3labs/mcp-go` v0.45.0 is current but v0.x (unstable API). GitHub MCP Server has already migrated to the official SDK. Plan migration path in Phase 1.5.

---

## Claude Code Integration Matrix `[NEW]`

Claude Code supports **24 hook events**, SKILL.md framework, Agent Teams (research preview), and Agent SDK (Python/TS, no Go SDK).

| Feature | Component | Status | Notes |
|---------|-----------|--------|-------|
| MCP Server (stdio) | `internal/mcpserver/` | Implemented | 222 total tools (218 grouped + 4 management), 30 deferred-load tool groups |
| Deferred tool loading | `internal/mcpserver/tools_dispatch.go` | Implemented | Core + management startup surface only; on-demand group loading keeps initial tool context compact |
| Hooks (internal) | `.ralph/hooks.yaml` | Implemented | Internal hook system, not CC hooks |
| CC hooks integration | - | Not started | 24 events: PreToolUse, PostToolUse, Stop, SessionStart, TeammateIdle, TaskCreated/Completed, WorktreeCreate, etc. |
| Skills export (.claude/skills/, .agents/skills/, plugin bundle) | `internal/session/skillgen.go` | Implemented | Generated on MCP startup; provider-native skill/plugin surfaces stay aligned from the live registry |
| Agent SDK bridge | - | Not started | No native Go SDK; requires sidecar/bridge pattern (Python/TS only) |
| Agent Teams | - | Not started | Research preview; teammates share findings, direct messaging; `CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS` |
| Prompt caching (`cache_control`) | `internal/enhancer/` | Partial | Enhancer has 3-provider caching; MCP handlers not cached yet |
| Adaptive thinking | - | Not started | GA Feb 2026; `thinking: {type: "adaptive"}` + `effort` param (replaces `budget_tokens`) |
| Beta headers | - | Partial | Key headers: `interleaved-thinking-2025-05-14`, `fast-mode-2026-02-01`, `compact-2026-01-12`, `mcp-client-2025-11-20`, `structured-outputs-2025-11-13`, `files-api-2025-04-14` |
| Token-efficient tool use | - | Built-in | All Claude 4 models have this natively; beta header only needed for Claude 3.7 |
| Structured outputs | - | Not started | GA Jan 2026; `output_config.format.json_schema` for guaranteed schema conformance |
| Messages Batches API | `internal/batch/` | Implemented | 50% discount, 3-provider support (Claude, Gemini, OpenAI) |
| Fast Mode | - | Not started | 2.5x faster Opus, 6x cost ($30/$150/MTok); `fast-mode-2026-02-01` |
| Compaction API | - | Not started | Server-side context summarization; `compact-2026-01-12` |
| MCP Connector (remote) | - | Not started | Messages API → remote MCP servers; `mcp-client-2025-11-20` |

---

## Provider Capability Matrix `[NEW]`

Cost comparison (March 2026, per 1M tokens, input/output):

| Tier | Claude | Gemini | OpenAI |
|------|--------|--------|--------|
| Ultra-cheap | — | Flash-Lite $0.10/$0.40 | GPT-5.4-nano $0.20/$1.25 |
| Worker | — | Flash $0.30/$2.50 | GPT-5.4-mini $0.75/$4.50 |
| Coding | Sonnet 4.6 $3/$15 | — | GPT-5.3-Codex $1.75/$14 |
| Frontier | Opus 4.6 $15/$75 | 3.1 Pro $2/$12 | GPT-5.4-pro $30/$180 |

| Capability | Claude (Anthropic) | Gemini (Google) | Codex (OpenAI) |
|------------|-------------------|-----------------|----------------|
| Models | opus-4-6, sonnet-4-6, haiku-4-5 | 3.1-pro, 2.5-flash, flash-lite | codex-mini, GPT-5.4 |
| Max context | 200K (1M via beta) | 1M tokens | 200K tokens |
| Tool use | Yes (parallel, built-in efficient) | Yes (VALIDATED mode preview) | Yes (strict schema) |
| Streaming | Yes | Yes (stream-json) | Yes (Responses API) |
| Prompt caching | cache_control (90% reads, 1.25x writes) | cachedContents ($1-4.50/hr storage) | Auto prefix (75-90%) |
| Batch API | Messages Batches (50% off) | BatchGenerateContent | POST /v1/batches (JSONL) |
| Resume | --resume | --resume | `exec resume` (install-dependent) |
| Headless mode | --print, -p | --yolo | --full-auto |
| Agent file | CLAUDE.md | .gemini/commands/*.toml | AGENTS.md |
| ralphglasses support | Full specialist lane | Full worker lane | Full primary control plane |

> **Routing research:** FrugalGPT/RouteLLM papers show 2-4x cost reduction with learned routers. Current cascade in `cascade.go` uses static thresholds — upgrade to confidence-based escalation (Phase 6.2).

---

## Tech Debt Register `[NEW]`

Derived from R&D cycle findings and scratchpad analysis.

| Issue | Impact | Effort | Components | Phase | Finding IDs |
|-------|--------|--------|------------|-------|-------------|
| JSON format enforcement failing | High — 25.7% retries waste budget | S | loop_worker.go | QW-1 | pattern_count: 26 |
| Zero-change iterations | High — 95.6% wasted compute | M | loop.go, loop_worker.go | QW-3, 0.6.2 | fleet_audit |
| Phantom fleet tasks | High — 73% stale work | S | coordinator.go | QW-11 | fleet_audit |
| Snapshot path claudekit | Medium — broken snapshots | S | snapshot.go | QW-7 | FINDING-148/268 |
| Loop gates zero baseline | Medium — misleading metrics | S | loop_gates.go | QW-6 | FINDING-226/238 |
| Budget params ignored | High — budget not enforced | S | tools_session.go | QW-8 | FINDING-258/261 |
| Provider recommend Claude-only | Medium — no multi-provider | M | tools_provider.go | 2.5 | FINDING-220/262 |
| Relevance scoring flat 0.5 | Medium — research unusable | M | tools_roadmap.go | QW-10 | research_audit |
| improvement_patterns rules null | Low — no learning | S | reflexion.go | QW-12 | pattern_analysis |
| Autonomy not persisted | Medium — state lost on restart | S | autooptimize.go | QW-9 | FINDING-257 |
| Session signal:killed | High — unclean shutdown | M | loop.go, manager.go | QW-3, 0.6.5 | FINDING-160 |
| cmd/ralphglasses-mcp 66.7% coverage | Low — undertested | L | cmd/ralphglasses-mcp/ | 1.6 | cycle14 |

---

## Dependency Chain

```
Phase 0.5 (Critical Fixes) --> Phase 1 (Harden) --> Phase 1.5 (DX)
                                      |                     |
                                      v                     v
Phase 0.9 (Quick Wins)        Phase 2 (Multi-Session) <----+
      |                              |
      v                              v
Phase 9 (R&D Cycle) <--+     Phase 2.5 (Multi-LLM)
      ^                |            |
      |                |     Phase 2.75 (Event Bus + MCP + TUI) [DONE]
Phase 0.8 [DONE] ------+            |
                               +----+----+
                               v         v
                          Phase 3 (i3)   Phase 5 (Sandbox)
                               |              |
                               v              v
                          Phase 3.5      Phase 6 (Intel) <-- 2.75 event bus
                               |              |
                          Phase 4 (Thin)      |
                               |              |
                               +------+-------+
                                      v
                               Phase 7 (K8s/Cloud)
                                      |
                                      v
                               Phase 8 (AI-Native)
```

### Item-Level Dependencies
```
0.5.1 (error fix) --> 1.8 (custom error types build on this)
0.5.2 (watcher fix) --> 1.7 (structured logging for watcher errors)
0.5.7 (version) --> 1.5.2 (release automation needs ldflags)

1.1 --> 1.4 (fixtures needed for PID file tests)
1.* --> 1.6 (coverage targets depend on all other Phase 1 work)

2.1 --> 2.2, 2.3, 2.4, 2.5, 2.8 (session model is foundation)
2.1 + 2.2 + 2.3 --> 2.5 (launcher needs worktrees + budget)
2.3 --> 5.5 (budget federation extends per-session tracking)
2.11 (health endpoint) --> 6.4 (prometheus reuses HTTP server)

3.1 --> 3.2, 3.3 (i3 IPC client needed for layout + coordination)
2.1 + 3.1 --> 3.3 (multi-instance needs SQLite + i3)

4.1 --> 4.2, 4.5, 4.10 (ISO pipeline needed before kiosk + install + USB)
5.1 or 5.2 --> 5.4 (network isolation needs a sandbox runtime)

6.1 --> 6.2, 6.3, 8.3 (native loop engine needed for orchestrator + coordination + workflows)
6.2 --> 8.5 (self-improvement needs R&D cycle)
6.4 --> 6.7 (analytics infrastructure needed for replay)

7.1 --> 7.2, 7.4 (K8s operator needed for autoscaling + cost mgmt)

0.8 (observability tools) --> 9.1 (R&D cycle tools build on scratchpad + observation query)
9.1 (finding_to_task, cycle_plan) --> 9.2 (loop quality tools)

2.75.2 (event bus) --> 6.4 (analytics builds on event history)
2.75.2 (event bus) --> 6.5 (external notifications consume events)
2.75.3 (workflow tools) --> 8.3 (workflow engine extends MCP workflows)
```

---

## R&D Cycle Architecture `[NEW]`

The self-improvement pipeline operates as a closed loop:

```
 Observations --> Journal --> Patterns --> Scratchpad --> Roadmap --> Curriculum
      ^                                                                  |
      |                                                                  v
      +--- Verify <--- Execute <--- Plan <--- finding_to_task <--- Prioritize
```

### Data flow per cycle

1. **Baseline** (`cycle_baseline`): Snapshot coverage, lint, build time, test count
2. **Plan** (`cycle_plan`): Read observations, identify regressions, generate task list
3. **Execute** (`loop_start`): Run tasks via StepLoop with cascade routing
4. **Observe** (`observation_query`): Record iteration outcomes, costs, diffs
5. **Learn** (`scratchpad_reason`): Analyze observations, extract findings
6. **Verify** (`merge_verify`, `loop_gates`): Confirm improvements, detect regressions
7. **Persist** (`scratchpad_append`): Write findings, update patterns
8. **Merge** (`cycle_merge`): Combine parallel worktree results

### Subsystem mapping

| Subsystem | Package | Key Files | Status |
|-----------|---------|-----------|--------|
| StepLoop | `internal/session/` | loop.go, loop_worker.go | Implemented |
| Cascade Router | `internal/session/` | cascade.go | Implemented, not enabled by default |
| Curriculum Sorter | `internal/session/` | curriculum.go | Implemented |
| Reflexion Store | `internal/session/` | reflexion.go | Implemented, rules null |
| Episodic Memory | `internal/session/` | episodic.go | Implemented |
| Auto-Optimize | `internal/session/` | autooptimize.go | Implemented, not persisted |
| Auto-Recovery | `internal/session/` | autorecovery.go | Implemented |
| Bandit (UCB1) | `internal/bandit/` | selector.go | Implemented, not configured |
| Blackboard | `internal/blackboard/` | blackboard.go | Implemented |
| Cost Predictor | `internal/session/` | costpredictor.go | Implemented |
| No-op Detector | `internal/session/` | convergence.go | Implemented |
| Batch Processing | `internal/batch/` | batch.go | Implemented (3-provider, 50% discount) |

### Research-informed enhancements

| Paper/Pattern | Applicable Subsystem | Enhancement | Priority |
|---------------|---------------------|-------------|----------|
| Reflexion (NeurIPS 2023) | Reflexion Store | Store structured verbal reflections, not just failure logs; inject as context on retry | P1 |
| LATS (ICML 2024) | StepLoop | For high-value tasks, spawn parallel candidate approaches (tree branches), prune by value estimate | P2 |
| DSPy/OPRO | Prompt Enhancement | Add optimizer loop: generate N candidate prompts, score with quality scorer, keep best | P1 |
| FrugalGPT/RouteLLM | Cascade Router | Upgrade from static thresholds to confidence-based escalation; learned classifier for task→provider routing | P1 |
| CrewAI memory taxonomy | Blackboard | Upgrade flat KV to structured memory: short-term, long-term, entity, contextual | P2 |
| Multi-agent scaling (Chen 2024) | Fleet | Diminishing returns above 5-7 agents; cap default fleet size, add communication overhead tracking | P1 |

### Competitive landscape positioning

| Competitor | Model | ralphglasses differentiator |
|------------|-------|-----------------------------|
| Cursor | IDE-integrated, single-session, background agents | Fleet orchestration, multi-provider, TUI/headless |
| Windsurf (→OpenAI) | IDE, "awareness" context, single-session | Multi-session, cost optimization, self-improvement loop |
| Aider | CLI single-session, architect/editor two-model | Fleet-scale, TUI dashboard, automated R&D cycles |
| CrewAI/AutoGen/LangGraph | Python frameworks, no end-user TUI | Go binary, production TUI, bootable thin client |
| E2B/Daytona | Sandbox-as-a-service | Self-hosted, integrated orchestration, no vendor lock-in |

---

## External Projects of Interest

### Multi-Session Claude Code Managers
| Project | Type | Platform | Key Feature |
|---------|------|----------|-------------|
| [craigsc/cmux](https://github.com/craigsc/cmux) | Bash | Linux/macOS/WSL | Git worktree per agent, pure bash |
| [cc-workflow](https://github.com/jrimmer/cc-workflow) | Bash | Linux/macOS | tmux + worktrees, Ubuntu provision script |
| [claude-tools](https://github.com/oreoriorosu/claude-tools) | Bash | WSL-native | tmux session management, `/mnt/c/` defaults |
| [zenportal](https://github.com/kgang/zenportal) | Python TUI | Linux | Multi-AI-CLI (Claude, Codex, Gemini) |
| [cc-hub](https://github.com/m0a/cc-hub) | Web UI | Linux | Linux x64 binary, multi-pane terminals |
| [Hive](https://github.com/latagore/hive) | Web UI | Linux | Mobile-first fleet dashboard, task queue |
| [code-on-incus](https://github.com/mensfeld/code-on-incus) | Go CLI | Linux | Incus containers, threat detection |
| [agent-runner](https://github.com/zsyu9779/agent-runner) | Go CLI | Linux/macOS | Stateful sessions, auto-commits |
| [claude-multi.nvim](https://github.com/mb6611/claude-multi.nvim) | Neovim | Any | Multi-session in Neovim |

### Agent OS & Sandboxing
| Project | Type | Maturity | Notes |
|---------|------|----------|-------|
| [StereOS](https://github.com/papercomputeco/stereOS) | NixOS agent OS | Alpha | gVisor sandboxing, produces VM images |
| Docker Sandboxes | Official | Production | Claude Code template, microVM isolation |
| [microvm.nix](https://michael.stapelberg.ch/posts/2026-02-01-coding-agent-microvm-nix/) | NixOS pattern | Documented | Stapelberg's microVM guide |
| [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox) | K8s CRD | v0.1.0 | gVisor + Kata, WarmPools |
| [alibaba/OpenSandbox](https://github.com/alibaba/OpenSandbox) | Multi-runtime | Production | Firecracker, gVisor, Kata |
| [E2B](https://e2b.dev/) | Firecracker SaaS | Production | <200ms sandbox boot |
| [Daytona](https://github.com/daytonaio/daytona) | Docker SaaS | Production | <90ms startup, state management |

### Container OS (for hosting agent workloads)
| OS | Recommendation | Notes |
|----|---------------|-------|
| **NixOS** | Top pick | microvm.nix, llm-agents.nix, claude-code-nix |
| **Fedora CoreOS** | Runner-up | Podman Quadlet systemd, no K8s required |
| **Kairos** | Build-your-own | Dockerfile -> bootable ISO |
| **Talos Linux** | K8s-only | API-only, NVIDIA extensions |

### Thin Client Base
| Distro | Size | Notes |
|--------|------|-------|
| **Ubuntu 24.04 HWE** | ~2GB | Current choice. NVIDIA 550 via apt, kernel 6.12+ |
| **Manjaro** | ~3-5GB | Current supported kiosk path with NVIDIA-friendly packaging |
| Tiny Core Linux | 16-21MB | Ultra-minimal, runs in RAM |
| ThinStation | ~50MB | PXE-native, RDP/VNC/SSH |

---

## Internal Ecosystem Integration

### From mcpkit (Go packages to port/embed)
- `mcpkit/ralph/` — Ralph Loop engine (DAG, specs, progress, cost tracking)
- `mcpkit/finops/` — FinOps cost tracking, budget management
- `mcpkit/sampling/` — LLM sampling client
- `mcpkit/registry/` — Tool registry, typed handlers
- `mcpkit/resilience/` — Circuit breakers, retries
- `mcpkit/observability/` — OpenTelemetry + Prometheus
- `mcpkit/orchestrator/` — Multi-agent orchestration

### From shell scripts (port to Go)
- `ralphglasses/marathon.sh` — 12h marathon launcher
- `dotfiles/mcp/hg-mcp/.ralph/start_session.sh` — Session launcher with budget reset
- `mesmer/.ralph/start-12hr.sh` — Pre-flight checks, budget projection
- `claudekit/scripts/perpetual-loop.sh` — Perpetual R&D cycle

### From Go MCP servers (reuse patterns)
- `hg-mcp/` — Modular tool registration pattern
- Pure-Go SQLite (modernc.org/sqlite) audit log patterns
- `claudekit/` — rdcycle perpetual loop, budget profiles

---

## Phase 10.5: Horizontal & Vertical Scaling `[NEW]`

Derived from 10-agent codebase analysis + 12-agent scaling research (2026-03-30). Each item maps to a specific bottleneck identified in the current codebase.

### 10.5.1 Session Manager Lock Splitting `P0` `L`

**Bottleneck:** Single `Manager.mu` RWMutex in `internal/session/manager.go` serializes all session operations at 100+ concurrent sessions.

- [x] Split into per-map fine-grained locks: `sessionsMu`, `workersMu`, `budgetMu`, `configMu`
- [ ] Use `sync.Map` for hot-path reads (session lookups, status queries)
- [ ] Add lock contention metrics (pprof mutex profile integration)
- [ ] Benchmark: target 70-80% contention reduction at 100 concurrent sessions
- Files: `internal/session/manager.go`, `internal/session/types.go`

### 10.5.2 MCP Server Concurrent Handler Limits `P1` `M`

**Bottleneck:** Large MCP tool surface with no concurrency limit on handlers means unbounded goroutine creation under load.

- [x] Add `semaphore.Weighted` (golang.org/x/sync) to `internal/mcpserver/middleware.go`
- [x] Default limit: 32 concurrent handlers, configurable via `MCP_MAX_CONCURRENT`
- [ ] Per-namespace rate limiting for expensive tools (fleet, session launch)
- [ ] Add handler queue depth metric
- Files: `internal/mcpserver/middleware.go`, `internal/mcpserver/server.go`

### 10.5.3 Event Bus Scaling (NATS Streaming) `P1` `L`

**Bottleneck:** In-process `events.Bus` with 1000-event ring buffer + JSONL persistence caps at single-node, single-process.

- [x] Abstract event bus behind `EventTransport` interface (in-memory default, NATS optional)
- [ ] NATS JetStream integration: persistent subjects per event type, consumer groups
- [ ] Windowed aggregation: 1m/5m/15m sliding windows for fleet metrics
- [ ] Partitioned storage: shard events by repo or session ID
- [ ] Event-driven TUI updates (replace 2s polling tick with bus subscription)
- Files: `internal/events/bus.go`, new `internal/events/nats.go`, `internal/tui/app.go`

### 10.5.4 Worker Pool Auto-Scaling `P1` `L`

**Bottleneck:** `MaxConcurrentWorkers=8` is static. No scaling based on queue depth, provider availability, or budget.

- [x] Auto-scale triggers: queue depth > 2x workers, provider rate limit headroom, budget remaining
- [ ] Provider specialization: route GPU-heavy tasks to specific workers, cost-optimize by provider
- [x] Health scoring: per-worker success rate, latency p99, stale task ratio
- [x] Priority queue with aging: prevent task starvation, priority decay over time
- [ ] Batch assignment: group related tasks (same repo, same provider) for worker affinity
- Files: `internal/fleet/coordinator.go`, `internal/fleet/worker.go`, new `internal/fleet/autoscaler.go`

### 10.5.5 Adaptive Iteration Depth `P1` `M`

**Bottleneck:** Fixed iteration limits waste compute on easy tasks and starve complex ones.

- [x] Complexity estimator: LOC, dependency depth, test count → predicted iterations
- [ ] Dynamic budget allocation: start conservative, expand on progress signals
- [ ] Deep work mode: for high-value tasks, double iteration limit + add verification passes
- [x] Smart convergence: detect diminishing returns (δ < threshold for N iterations → early stop)
- Files: `internal/session/loop.go`, `internal/session/convergence.go`, new `internal/session/depth.go`

### 10.5.6 Multi-Node Marathon Distribution `P1` `XL`

**Bottleneck:** Marathons run on a single machine. Large fleets need cross-node coordination.

- [ ] Repo sharding: partition repos across nodes by hash or explicit assignment
- [ ] Pipeline parallelism: plan on coordinator, execute on workers, verify on coordinator
- [ ] Warm-start prefetching: pre-clone repos + pre-pull models on target workers
- [ ] State replication: supervisor state sync across coordinator nodes (leader election)
- Files: `internal/fleet/coordinator.go`, `internal/session/manager_cycle.go`, new `internal/fleet/sharding.go`

### 10.5.7 Cost Optimization Engine `P1` `L`

**Bottleneck:** Static cascade routing misses 2-4x cost reduction opportunities.

- [x] Dynamic cost-tier routing via contextual bandits (extend existing UCB1 in `internal/bandit/`)
- [ ] Batch API utilization: auto-batch non-urgent tasks for 50% discount (extend `internal/batch/`)
- [ ] Fleet-wide prompt caching: shared cache prefix across sessions targeting same repo
- [ ] Budget forecasting: predict remaining marathon budget from spend velocity + task queue
- [ ] Token optimization: context pruning, tool call deduplication (target 25-35% reduction)
- Files: `internal/session/cascade.go`, `internal/batch/batch.go`, `internal/bandit/selector.go`

### 10.5.8 Git Scaling `P2` `L`

**Bottleneck:** Worktree creation overhead at scale, disk usage with many clones.

- [x] Worktree pooling: pre-create N worktrees per repo, reuse across sessions (10x creation speedup)
- [ ] Git alternates: share object store across clones (16x disk reduction for large repos)
- [ ] Merge conflict prevention: pre-check branch divergence before parallel work
- [ ] Multi-repo coordination: atomic cross-repo changes with two-phase commit
- Files: `internal/session/worktree.go`, new `internal/session/worktree_pool.go`

### 10.5.9 State Persistence (SQLite WAL) `P1` `L`

**Bottleneck:** JSON file persistence is fragile under concurrent access and doesn't scale.

- [x] SQLite WAL mode for fleet state (coordinator, sessions, observations)
- [x] Per-entity locking instead of global mutex
- [x] Session-store lock contention hardening: DSN `busy_timeout`, single-connection writes, and `SQLITE_BUSY` retries for migrate/save/update paths
- [ ] State sharding by repo for parallel writes
- [ ] Observation partitioning: time-based partitions for efficient queries
- [ ] Migration path from JSON files to SQLite (dual-write during transition)
- [ ] PostgreSQL control-plane research spike: evaluate an optional PostgreSQL backend for multi-node coordinator state when fleet deployment spans multiple hosts or network filesystems `P2` `M`
- [ ] Backend split study: map which entities remain local SQLite (`observations`, replay logs, checkpoints) versus shared control-plane state (`tenants`, sessions, loop_runs, cost_ledger, recovery_ops, fleet queue/leases) `P2` `S`
- [ ] Adapter and cutover design: confirm whether `internal/session/store.go` is sufficient for an optional PostgreSQL adapter, define config gate + bootstrap path, and document rollback/no-big-bang migration strategy `P2` `S`
- [ ] Coordination primitive evaluation: compare PostgreSQL advisory locks + LISTEN/NOTIFY against current file-lock and polling patterns, and record benchmark + ops criteria `P2` `S`
- **Acceptance:** source-backed memo recommends one of: keep SQLite-only, add optional PostgreSQL backend, or use per-node SQLite plus replication; includes schema candidates, migration sketch, lock/signaling approach, operational cost, and benchmark targets.
- Deps: `modernc.org/sqlite` (pure Go)
- Files: new `internal/store/sqlite.go`, `internal/fleet/coordinator.go`

### 10.5.10 Monitoring & Observability Stack `P2` `L`

**Bottleneck:** No external metrics export, no structured alerting.

- [x] Prometheus metrics exporter: session counts, costs, latencies, error rates
- [ ] Structured alerting: webhook, Slack, Discord notifications on fleet events
- [ ] Distributed tracing: OpenTelemetry spans for session lifecycle
- [ ] Capacity planning: predict resource needs from historical fleet data
- Files: new `internal/metrics/prometheus.go`, new `internal/metrics/alerting.go`

### 10.5.11 Autonomy Scaling `P2` `XL`

**Bottleneck:** Single-repo supervisor, no multi-repo autonomy.

- [ ] Multi-repo supervisor: coordinate R&D cycles across the hairglasses-studio org
- [ ] Contextual multi-armed bandit for task selection (per-repo arms with shared context)
- [ ] Online learning with concept drift detection (detect when past learnings become stale)
- [ ] Fleet-wide anomaly detector with kill switch (halt autonomy on regression)
- Files: `internal/session/manager_cycle.go`, `internal/session/autooptimize.go`, `internal/bandit/selector.go`

---

## Phase 11: A2A Protocol Integration `[NEW]`

Agent-to-Agent (A2A) v1.0.0 protocol integration for cross-node fleet coordination. A2A complements MCP — MCP handles model-to-tool communication, A2A handles agent-to-agent task delegation, status tracking, and artifact exchange.

**Go SDK:** `github.com/a2aproject/a2a-go/v2` (Go 1.24+, Apache 2.0)

### Current State

Partial A2A support exists in `internal/fleet/`:
- `a2a.go` — `A2AAdapter` with task offer lifecycle
- `a2a_card.go` — `AgentCard`, `BuildAgentCard()`, `DiscoverAgent()`, `RemoteA2AAdapter`
- Handlers at `/.well-known/agent.json` and `/api/v1/a2a/task/{taskID}`

### Gap Analysis

| Feature | Current | A2A v1.0 Spec | Gap |
|---------|---------|---------------|-----|
| Discovery | `/.well-known/agent.json` | `/.well-known/agent-card.json` | Path mismatch |
| Agent Card | Custom subset | Full schema (skills, capabilities, security, signatures) | Missing security, interfaces, capabilities |
| Task lifecycle | `TaskOffer` string status | `Task` with typed `TaskState`, Messages, Artifacts | Adopt `a2a.Task` type |
| Message format | Plain `Prompt` string | Multi-part (Text/File/Data) | Need Part support |
| Streaming | Custom SSE | SSE on `/tasks/sendStreaming`, `/tasks/{id}/subscribe` | A2A-compliant endpoints |
| Push notifications | Not implemented | Webhook registration with auth | New feature |
| Authentication | None (network trust) | SecuritySchemes (apiKey, bearer, OAuth2, mTLS) | Need scheme enforcement |
| Transport | REST only | REST + JSON-RPC + gRPC | Add gRPC for intra-cluster |

### 11.1 Adopt Official Go SDK `P1` `M`
- [ ] Add `github.com/a2aproject/a2a-go/v2` to `go.mod`
- [ ] Migrate `AgentCard` in `a2a_card.go` to `a2a.AgentCard` type
- [ ] Fix discovery path to `/.well-known/agent-card.json`
- [ ] Register `a2asrv.NewStaticAgentCardHandler` on fleet HTTP server
- [ ] Declare capabilities: streaming, push notifications, skills
- Files: `go.mod`, `internal/fleet/a2a_card.go`, `internal/fleet/server_handlers.go`

### 11.2 Implement FleetExecutor `P1` `L`
- [ ] Implement `a2asrv.AgentExecutor` interface wrapping `session.Manager.Launch()`
- [ ] Mount `a2asrv.NewRESTHandler()` on coordinator
- [ ] Map `session.StreamEvent` → `a2a.TaskStatusUpdateEvent` / `a2a.TaskArtifactUpdateEvent`
- [ ] Support task cancellation via `AgentExecutor.Cancel()`
- Files: new `internal/fleet/a2a_executor.go`, `internal/fleet/server.go`

### 11.3 A2A Client for Cross-Fleet Dispatch `P1` `L`
- [ ] Replace `RemoteA2AAdapter.SubmitTask()` with `a2aclient.SendMessage()`
- [ ] Use `a2aclient.SubscribeToTask()` for real-time progress tracking
- [ ] Push notification registration for marathon coordination (webhook callbacks)
- [ ] Cross-machine task delegation: thin client coordinator → cloud worker agents
- Files: `internal/fleet/a2a.go`, new `internal/fleet/a2a_dispatch.go`

### 11.4 Dynamic Capability Discovery `P2` `L`
- [ ] Workers advertise skills via Agent Card (provider type, model, capacity)
- [ ] Coordinator indexes capabilities from discovered cards
- [ ] Route tasks based on discovered skills, not hardcoded provider configs
- [ ] Supervisor health checks via A2A task status subscriptions
- Files: `internal/fleet/discovery.go`, `internal/fleet/types.go`

### 11.5 Event Bus Federation `P2` `L`
- [ ] Bridge `events.Bus` to A2A push notifications (outbound: local events → remote coordinators)
- [ ] Inbound webhook handler: A2A push → local event bus
- [ ] Security: bearer token auth on push webhooks, Tailscale network trust as base layer
- [ ] gRPC transport binding for low-latency intra-cluster communication
- Files: new `internal/fleet/a2a_federation.go`, `internal/events/bus.go`

---

## Phase 12: Tailscale Fleet Networking `[NEW]`

Tailscale-based fleet networking for secure, zero-config connectivity between thin client coordinator, cloud VM workers, and admin machines. All fleet HTTP traffic flows over WireGuard-encrypted Tailscale connections.

### Current State

Partial Tailscale support exists:
- `internal/fleet/types.go` — `WorkerInfo.TailscaleIP` field
- `internal/fleet/discovery.go` — `GetTailscaleStatus()` (shells out to `tailscale status --json`), `DiscoverCoordinator()` probes peers
- `internal/fleet/worker.go` — `RegisterPayload.TailscaleIP`, `DiscoverTailscaleIP()` stub
- `cmd/serve.go` — Auto-discovers coordinator via Tailscale peer probing

### Tag Taxonomy

| Tag | Purpose |
|-----|---------|
| `tag:ralph-fleet` | All ralphglasses nodes |
| `tag:ralph-coordinator` | Fleet coordinator (thin client) |
| `tag:ralph-worker` | Worker nodes (cloud VMs) |
| `tag:ralph-mcp` | Nodes exposing MCP server endpoints |

### Architecture

```
                    +--------------------------+
                    |     Tailnet (WireGuard)   |
                    +--------------------------+
                              |
            +-----------------+-----------------+
            |                 |                 |
  +-------------------+  +----------+  +----------+
  | Thin Client       |  | Cloud VM |  | Cloud VM |
  | ralph-coord-01    |  | ralph-   |  | ralph-   |
  | tag:coordinator   |  | worker-01|  | worker-02|
  | 7 monitors, 4090  |  | tag:     |  | tag:     |
  | Coordinator:9473  |  | worker   |  | worker   |
  | MCP Server        |  | :9473    |  | :9473    |
  +-------------------+  +----------+  +----------+
```

### 12.1 First-Boot Enrollment `P1` `M`
- [ ] Create `distro/scripts/ts-enroll.sh` — headless enrollment with pre-auth key
- [ ] Create `distro/systemd/ts-enroll.service` — oneshot, gated by marker file
- [ ] Integration with `hw-detect.sh` boot sequence: `tailscaled` → `hw-detect` → `ts-enroll` → `ralphglasses`
- [ ] Auth key provisioning: `/etc/ralphglasses/ts-authkey` injected by install-to-disk.sh or cloud-init
- [ ] Hostname derivation from hardware serial or MAC address
- Files: new `distro/scripts/ts-enroll.sh`, new `distro/systemd/ts-enroll.service`

### 12.2 ACL Policy & SSH `P1` `M`
- [ ] Define tailnet policy file with ralph tag taxonomy
- [ ] Coordinator → worker SSH: `action: accept` (machine-to-machine, no re-auth)
- [ ] Admin → fleet SSH: `action: check` with 12h re-auth period
- [ ] Auto-approve subnet routes for fleet tags
- [ ] ACL tests validating fleet connectivity rules
- Files: new `distro/tailscale/policy.json`

### 12.3 Go SDK Integration `P1` `L`
- [ ] Add `tailscale.com/tsnet` and `tailscale.com/client/local` to `go.mod`
- [ ] Replace `GetTailscaleStatus()` shell-out with `local.Client.Status()` in `discovery.go`
- [ ] Complete `DiscoverTailscaleIP()` stub in `worker.go`
- [ ] Add `WhoIs`-based auth middleware to fleet HTTP server (verify `tag:ralph-fleet`)
- [ ] MagicDNS-based coordinator discovery: resolve `ralph-coord-01` instead of peer enumeration
- Files: `go.mod`, `internal/fleet/discovery.go`, `internal/fleet/worker.go`, `internal/fleet/server.go`

### 12.4 tsnet Embedding `P2` `L`
- [ ] Embed `tsnet.Server` in coordinator for zero-config networking
- [ ] `tsnet.Listen()` for fleet API (tailnet-only)
- [ ] `tsnet.ListenFunnel()` for public MCP endpoint (HTTPS via Let's Encrypt)
- [ ] Peer identity verification via `LocalClient.WhoIs()` — replace token auth with Tailscale identity
- Files: `internal/fleet/server.go`, `cmd/serve.go`

### 12.5 Cloud VM Auto-Enrollment `P2` `M`
- [ ] cloud-init template for worker VMs: install Tailscale, enroll, start ralphglasses worker
- [ ] OAuth client credentials flow for programmatic auth key generation
- [ ] Worker systemd unit: `ralphglasses-worker.service` with `Requires=tailscaled.service`
- [ ] Fleet health monitoring via Tailscale control plane API (device last-seen, online status)
- Files: new `distro/cloud-init/worker.yaml`, new `distro/systemd/ralphglasses-worker.service`

### 12.6 Worktree Sync Over Tailscale `P2` `M`
- [ ] rsync over Tailscale SSH (no key distribution needed)
- [ ] Git-based sync: `ssh ralph@ralph-worker-01 "cd /workspace && git pull --ff-only"`
- [ ] Pre-flight repo sync before session launch on remote worker
- Files: `internal/fleet/worker.go`, new `internal/fleet/sync.go`

---

## Phase 13: Level 3 Autonomy Core `[NEW]`

Self-healing, self-optimizing, unattended operation. The agent fleet runs without human intervention for extended periods, making operational decisions autonomously while maintaining safety boundaries.

> **Prerequisite:** Phase 9.5 supervisor, Phase 10.5 scaling. **Target:** 72-hour unattended operation.

### 13.1 Self-Healing Runtime `P0` `XL`
- [ ] **SH-1** — Implement heartbeat-based session health monitor with configurable failure thresholds (3 consecutive failures = dead)
- [ ] **SH-2** — Auto-restart failed sessions with exponential backoff (1s, 2s, 4s, 8s, max 5min)
- [ ] **SH-3** — Session state snapshot/restore for crash recovery (serialize full session state to SQLite)
- [ ] **SH-4** — Circuit breaker per provider with half-open probe (fail 5 → open 60s → half-open → probe → close/reopen)
- [ ] **SH-5** — Cascading failure prevention: isolate provider outages from healthy sessions
- [ ] **SH-6** — Memory pressure detection: monitor RSS via `/proc/self/status`, shed load at 80% threshold
- [ ] **SH-7** — Disk pressure detection: monitor worktree disk usage, prune stale worktrees at 90% capacity
- [ ] **SH-8** — Orphan process reaper: scan for abandoned claude/gemini/codex child processes on startup
- [ ] **SH-9** — Lock file recovery: detect and clean stale `.lock` files from crashed sessions
- [ ] **SH-10** — Watchdog timer: kill sessions exceeding 2x expected duration with diagnostic dump
- Files: `internal/session/self_heal.go`, `internal/session/watchdog.go`, `internal/session/crash_recovery.go`

### 13.2 Config Auto-Application `P0` `L`
- [ ] **CA-1** — Config change detector: watch `.ralphrc`, `.ralph/config.json` via fsnotify, diff against running config
- [ ] **CA-2** — Hot-reload config without session restart (provider weights, budget limits, cascade thresholds)
- [ ] **CA-3** — Config validation engine: JSON Schema for `.ralphrc` with typed errors before application
- [ ] **CA-4** — Config rollback: snapshot config before change, auto-revert on degraded metrics (latency +50%, error rate +20%)
- [ ] **CA-5** — Config propagation to fleet: coordinator pushes config updates to all workers via event bus
- [ ] **CA-6** — Config drift detection: periodic reconciliation between desired and actual state
- [ ] **CA-7** — Environment-aware config: dev/staging/prod profiles with automatic detection
- Files: `internal/session/config_hotreload.go`, `internal/session/config_validator.go`

### 13.3 Autonomous Decision Engine `P0` `XL`
- [ ] **AD-1** — Decision journal: log every autonomous decision with context, reasoning, outcome, and counterfactual
- [ ] **AD-2** — Decision policy engine: OPA-style rules defining what the system can do at each autonomy level
- [ ] **AD-3** — Escalation protocol: decisions exceeding confidence threshold (< 0.7) escalate to human via notification
- [ ] **AD-4** — Decision audit trail: immutable append-only log for compliance and debugging
- [ ] **AD-5** — Rollback capability: every autonomous action has a defined undo operation
- [ ] **AD-6** — Decision replay: re-evaluate past decisions with updated policies for policy tuning
- [ ] **AD-7** — Safety boundaries: hard limits that cannot be overridden (max spend/hour, max concurrent sessions, forbidden operations)
- [ ] **AD-8** — Gradual autonomy ramp: auto-increase autonomy level after N successful unattended hours (1→2 at 4h, 2→3 at 24h)
- Files: `internal/session/decision_engine.go`, `internal/session/decision_policy.go`, `internal/session/decision_journal.go`

### 13.4 Self-Optimization Loop `P1` `L`
- [ ] **SO-1** — Performance baseline tracker: rolling 1h/24h/7d windows for latency, throughput, cost, success rate
- [ ] **SO-2** — Automatic parameter tuning: Bayesian optimization for cascade thresholds, batch sizes, retry intervals
- [ ] **SO-3** — Provider weight auto-adjustment: shift traffic based on real-time cost/quality Pareto frontier
- [ ] **SO-4** — Prompt template evolution: A/B test prompt variations, promote winners automatically
- [ ] **SO-5** — Session depth optimizer: learn optimal iteration count per task type from historical outcomes
- [ ] **SO-6** — Cost anomaly detector: alert and throttle when spend exceeds 2σ from rolling average
- [ ] **SO-7** — Quality regression detector: compare output quality scores against baseline, revert optimizations that degrade quality
- Files: `internal/session/self_optimize.go`, `internal/session/param_tuner.go`

### 13.5 Unattended Operation Mode `P1` `L`
- [ ] **UO-1** — Startup sequence: validate all providers, check disk/memory, load last-known-good config, resume interrupted sessions
- [ ] **UO-2** — Scheduled maintenance windows: pause sessions during defined windows, resume after
- [ ] **UO-3** — Daily health report: aggregate metrics, decisions made, anomalies detected, cost summary → email/Slack/file
- [ ] **UO-4** — Graceful degradation: if primary provider down, automatically route to secondary with quality warning
- [ ] **UO-5** — Nightly optimization run: re-tune parameters, prune stale data, compact databases during low-traffic hours
- [ ] **UO-6** — SLA monitoring: track uptime, mean-time-to-recovery, session success rate against defined targets
- [ ] **UO-7** — Emergency stop: hardware button / kill signal triggers graceful shutdown with state preservation
- [ ] **UO-8** — Resume after power loss: systemd unit with `Restart=always`, state recovery from last checkpoint
- Files: `internal/session/unattended.go`, `internal/session/maintenance.go`, `internal/session/sla.go`

---

## Phase 14: Agent Memory & Meta-Learning `[NEW]`

Persistent memory, experience replay, curriculum learning, and meta-cognitive capabilities that enable agents to improve across sessions and learn from fleet-wide experience.

> **Research basis:** MemGPT/Letta architecture, SELF-REFINE, Reflexion, LATS, episodic memory retrieval.

### 14.1 Persistent Agent Memory `P0` `XL`
- [ ] **PM-1** — Tiered memory architecture: working (in-context) → short-term (SQLite, 24h) → long-term (embeddings, indefinite)
- [ ] **PM-2** — Memory consolidation: nightly job merges similar short-term memories, promotes to long-term
- [ ] **PM-3** — Semantic memory retrieval: embed memories with local model (all-MiniLM-L6-v2), cosine similarity search
- [ ] **PM-4** — Episodic memory: store full session trajectories (state, action, outcome) for experience replay
- [ ] **PM-5** — Procedural memory: extract reusable patterns from successful sessions (code templates, fix recipes)
- [ ] **PM-6** — Memory eviction policy: LRU with importance weighting (high-reward memories persist longer)
- [ ] **PM-7** — Cross-session memory sharing: fleet-wide memory pool accessible by all agents
- [ ] **PM-8** — Memory search MCP tools: `memory_store`, `memory_recall`, `memory_forget`, `memory_stats`
- [ ] **PM-9** — Context window management: MemGPT-style paging — swap memory pages in/out of context window
- [ ] **PM-10** — Memory compression: summarize old memories to reduce storage while preserving key information
- Files: `internal/memory/store.go`, `internal/memory/retrieval.go`, `internal/memory/consolidation.go`, `internal/memory/embeddings.go`

### 14.2 Experience Replay & Learning `P1` `L`
- [ ] **ER-1** — Session trajectory recording: capture (state, action, reward) tuples for every session turn
- [ ] **ER-2** — Prioritized replay buffer: sample high-reward and high-surprise trajectories more frequently
- [ ] **ER-3** — Hindsight experience replay: relabel failed trajectories with achieved goals for learning from failure
- [ ] **ER-4** — Fleet-wide experience aggregation: merge replay buffers across all agents for collective learning
- [ ] **ER-5** — Pattern extraction: identify common success/failure patterns from replay buffer
- [ ] **ER-6** — Strategy library: curated collection of proven approaches per task type, updated from replay analysis
- [ ] **ER-7** — Counterfactual reasoning: "what if we had used provider X instead?" analysis from trajectory data
- Files: `internal/memory/replay.go`, `internal/memory/trajectory.go`, `internal/memory/strategy.go`

### 14.3 Curriculum Learning `P1` `L`
- [ ] **CL-1** — Task difficulty estimator: predict complexity from prompt features (length, code references, ambiguity score)
- [ ] **CL-2** — Adaptive curriculum: assign tasks from easy→hard as agent competence increases
- [ ] **CL-3** — Competence tracking per domain: separate skill levels for Go, Python, infrastructure, testing, etc.
- [ ] **CL-4** — Scaffolding: provide more hints/examples for tasks above current competence level
- [ ] **CL-5** — Mastery detection: move to harder tasks when success rate on current difficulty > 90%
- [ ] **CL-6** — Curriculum generation: automatically create training tasks from codebase patterns
- [ ] **CL-7** — Spaced repetition: re-test previously mastered skills at increasing intervals
- Files: `internal/memory/curriculum.go`, `internal/memory/competence.go`

### 14.4 Meta-Cognitive Capabilities `P2` `L`
- [ ] **MC-1** — Confidence calibration: track predicted vs actual success rates, apply Platt scaling
- [ ] **MC-2** — Uncertainty estimation: detect when agent is in unfamiliar territory (OOD detection via embedding distance)
- [ ] **MC-3** — Self-monitoring: agent evaluates own output quality before returning (SELF-REFINE loop)
- [ ] **MC-4** — Reflection triggers: automatically invoke reflection after failures, surprises, or long sessions
- [ ] **MC-5** — Learning rate tracking: measure how quickly agent improves on new task types
- [ ] **MC-6** — Cognitive load estimation: predict when context window is too full for quality output
- [ ] **MC-7** — Meta-strategy selection: choose between depth-first, breadth-first, or iterative approaches based on task type
- Files: `internal/memory/metacognition.go`, `internal/memory/confidence.go`

---

## Phase 15: Advanced Fleet Intelligence `[NEW]`

Distributed scheduling, intelligent task routing, fleet-wide optimization, and emergent coordination patterns that enable efficient operation at 100+ concurrent agents.

> **Research basis:** DeepSeek MoE, FrugalGPT/RouterLLM, swarm intelligence, stigmergy.

### 15.1 Intelligent Task Router `P0` `XL`
- [ ] **TR-1** — Multi-armed bandit router: Thompson sampling over (provider, model, prompt-strategy) arms
- [ ] **TR-2** — Contextual bandit: condition routing on task features (language, complexity, domain, time-of-day)
- [ ] **TR-3** — Cost-quality Pareto router: user specifies quality floor, system minimizes cost above that floor
- [ ] **TR-4** — Latency-aware routing: factor in current provider response times (rolling 5min P50/P99)
- [ ] **TR-5** — Rate-limit-aware routing: pre-emptively route away from providers approaching rate limits
- [ ] **TR-6** — Router learning: update bandit arms from every completed session (reward = quality / cost)
- [ ] **TR-7** — Router explainability: log why each routing decision was made (feature weights, arm values)
- [ ] **TR-8** — Fallback chains: define ordered fallback sequences per task type (Claude → Gemini → Codex)
- [ ] **TR-9** — Router A/B testing: split traffic between routing strategies, compare outcomes
- [ ] **TR-10** — Mixture-of-Experts dispatch: route sub-tasks to specialist agents based on domain expertise scores
- Files: `internal/fleet/router.go`, `internal/fleet/bandit_router.go`, `internal/fleet/pareto.go`

### 15.2 Fleet-Wide Optimization `P1` `L`
- [ ] **FO-1** — Global budget optimizer: distribute budget across agents to maximize fleet-wide output quality
- [ ] **FO-2** — Work stealing: idle agents pull tasks from overloaded agents' queues
- [ ] **FO-3** — Speculative execution: run same task on 2 providers, take first good result, cancel other
- [ ] **FO-4** — Batch coalescing: group similar tasks for batch API submission (OpenAI batch, Gemini batch)
- [ ] **FO-5** — Priority queuing: P0 tasks preempt P2 tasks, with starvation prevention (max wait 30min)
- [ ] **FO-6** — Capacity forecasting: predict fleet throughput for next hour based on current load + provider health
- [ ] **FO-7** — Resource reservation: pre-allocate capacity for scheduled high-priority work
- [ ] **FO-8** — Fleet defragmentation: consolidate sessions onto fewer workers during low-load periods
- Files: `internal/fleet/optimizer.go`, `internal/fleet/work_stealing.go`, `internal/fleet/batch.go`

### 15.3 Swarm Coordination `P2` `L`
- [ ] **SC-1** — Stigmergy: agents leave digital "pheromone trails" (task annotations) for other agents to follow
- [ ] **SC-2** — Blackboard architecture: shared knowledge space where agents post findings and read others'
- [ ] **SC-3** — Agent specialization emergence: agents gravitate toward task types they succeed at (reinforcement)
- [ ] **SC-4** — Consensus protocols: multi-agent voting on architectural decisions (majority > 2/3 required)
- [ ] **SC-5** — Division of labor: automatic task decomposition into sub-tasks assigned to specialist agents
- [ ] **SC-6** — Conflict resolution: detect when agents make contradictory changes, invoke merge arbitrator
- [ ] **SC-7** — Emergent roles: agents self-organize into reviewer, implementer, tester, documenter roles
- Files: `internal/fleet/swarm.go`, `internal/fleet/stigmergy.go`, `internal/fleet/consensus.go`

### 15.4 Distributed Scheduling `P1` `L`
- [ ] **DS-1** — DAG-based task scheduler: define task dependencies, schedule respecting topological order
- [ ] **DS-2** — Critical path analysis: identify and prioritize the longest dependency chain
- [ ] **DS-3** — Schedule visualization: Gantt chart in TUI showing task timelines, dependencies, critical path
- [ ] **DS-4** — Deadline-aware scheduling: tasks with deadlines get priority based on slack time
- [ ] **DS-5** — Resource-constrained scheduling: respect per-provider rate limits and per-agent memory limits
- [ ] **DS-6** — Preemptive scheduling: pause low-priority work when high-priority work arrives
- [ ] **DS-7** — Schedule optimization: minimize makespan using list scheduling heuristic
- Files: `internal/fleet/scheduler.go`, `internal/fleet/dag.go`, `internal/fleet/gantt.go`

---

## Phase 16: Edge & Embedded Agents `[NEW]`

Run agents on edge devices, local hardware, and hybrid cloud-edge configurations. Enable offline operation, on-device inference, and bandwidth-efficient fleet communication.

> **Research basis:** Ollama, vLLM, ExLlamaV2, ONNX Runtime, llama.cpp, TinyML.

### 16.1 Local Model Integration `P1` `XL`
- [ ] **LM-1** — Ollama provider: implement `Provider` interface for local Ollama models (llama3, codellama, deepseek-coder)
- [ ] **LM-2** — Model discovery: auto-detect available Ollama models via `ollama list` API
- [ ] **LM-3** — vLLM provider: connect to local vLLM server for high-throughput local inference
- [ ] **LM-4** — Model capability mapping: tag local models with capability scores (code, chat, reasoning, context-length)
- [ ] **LM-5** — Hybrid routing: route to local models for simple tasks (linting, formatting), cloud for complex (architecture, debugging)
- [ ] **LM-6** — Cost modeling for local inference: estimate $/token based on GPU power consumption + amortized hardware
- [ ] **LM-7** — Model quantization support: GGUF/GPTQ/AWQ format detection, quality-vs-speed tradeoff selection
- [ ] **LM-8** — Fallback to cloud: if local model confidence < threshold, escalate to cloud provider
- [ ] **LM-9** — Model warm-up: pre-load frequently used models into GPU memory on startup
- [ ] **LM-10** — Multi-GPU dispatch: distribute model layers across multiple GPUs (tensor parallelism via vLLM)
- Files: `internal/session/provider_ollama.go`, `internal/session/provider_vllm.go`, `internal/session/model_discovery.go`

### 16.2 Offline Operation `P2` `L`
- [ ] **OF-1** — Offline mode detection: check network connectivity, switch to local-only providers
- [ ] **OF-2** — Request queuing: buffer cloud API requests during offline periods, flush when connectivity returns
- [ ] **OF-3** — Local cache: cache frequent API responses for offline replay (system prompts, tool definitions)
- [ ] **OF-4** — Offline-capable tools: mark MCP tools as online/offline, disable online-only tools in offline mode
- [ ] **OF-5** — Sync-on-reconnect: reconcile offline work with fleet state when connectivity returns
- [ ] **OF-6** — Progressive enhancement: start with local model, upgrade to cloud when available
- Files: `internal/session/offline.go`, `internal/session/request_queue.go`

### 16.3 Edge Fleet Management `P2` `L`
- [ ] **EF-1** — Edge node registration: lightweight enrollment for x86_64 mini PCs, lab nodes, and appliance-class coordinators
- [ ] **EF-2** — Bandwidth-aware communication: compress fleet messages, batch status updates (delta encoding)
- [ ] **EF-3** — Split inference: run embedding/tokenization on edge, send to cloud for generation
- [ ] **EF-4** — Edge-specific task assignment: route hardware-appropriate tasks to edge devices
- [ ] **EF-5** — Remote model deployment: push GGUF models to edge nodes via fleet protocol
- [ ] **EF-6** — Edge health monitoring: temperature, memory, storage metrics with thermal throttling awareness
- [ ] **EF-7** — Mesh networking: edge nodes can relay work to each other without coordinator
- Files: `internal/fleet/edge.go`, `internal/fleet/edge_monitor.go`, `internal/fleet/mesh.go`

---

## Phase 17: AI Safety & Governance `[NEW]`

Safety boundaries, alignment techniques, audit trails, compliance frameworks, and adversarial testing to ensure autonomous agent fleets operate within defined boundaries.

> **Research basis:** Constitutional AI, DPO, process reward models, red-teaming, EU AI Act compliance.

### 17.1 Safety Boundaries & Guardrails `P0` `XL`
- [ ] **SB-1** — Operation allowlist: define permitted operations per autonomy level (L0: read-only, L1: +write, L2: +execute, L3: +deploy)
- [ ] **SB-2** — Resource limits: per-session caps on CPU time, memory, disk I/O, network bandwidth
- [ ] **SB-3** — Sensitive file protection: blocklist for `.env`, credentials, private keys — agents cannot read or modify
- [ ] **SB-4** — Network allowlist: restrict agent HTTP access to approved domains only
- [ ] **SB-5** — Git safety: prevent force-push to main/master, require PR for protected branches
- [ ] **SB-6** — Cost circuit breaker: hard stop at configurable $/hour and $/day limits (no override at L3)
- [ ] **SB-7** — Output sanitization: scan agent outputs for secrets, PII, and credentials before displaying/logging
- [ ] **SB-8** — Blast radius limits: maximum files changed per session, maximum lines changed per commit
- [ ] **SB-9** — Irreversibility detection: flag operations that cannot be undone (database migrations, published releases)
- [ ] **SB-10** — Human-in-the-loop gates: configurable checkpoints requiring human approval before proceeding
- Files: `internal/safety/guardrails.go`, `internal/safety/allowlist.go`, `internal/safety/sanitizer.go`

### 17.2 Constitutional AI for Agents `P1` `L`
- [ ] **CO-1** — Agent constitution: define principles agents must follow (helpful, harmless, honest + domain-specific rules)
- [ ] **CO-2** — Self-critique loop: agent evaluates own output against constitution before returning
- [ ] **CO-3** — Constitutional revision: propose→critique→revise cycle for outputs that violate principles
- [ ] **CO-4** — Principle priority ordering: when principles conflict, follow defined priority (safety > correctness > efficiency)
- [ ] **CO-5** — Constitution versioning: track changes to constitution over time, A/B test constitutional variants
- [ ] **CO-6** — Cross-agent constitution enforcement: agents can flag other agents' outputs for constitutional review
- Files: `internal/safety/constitution.go`, `internal/safety/self_critique.go`

### 17.3 Process Reward Models `P1` `L`
- [ ] **PR-1** — Step-level evaluation: score each reasoning step, not just final output (process vs outcome reward)
- [ ] **PR-2** — Local reward model: fine-tune small model on (step, score) pairs from successful sessions
- [ ] **PR-3** — Reward signal integration: use process reward to guide MCTS/beam search over solution steps
- [ ] **PR-4** — Reward hacking detection: monitor for agents gaming reward metrics without improving actual quality
- [ ] **PR-5** — Multi-objective reward: balance code correctness, test coverage, readability, performance
- [ ] **PR-6** — Human feedback integration: periodically sample outputs for human scoring, update reward model
- [ ] **PR-7** — Reward model calibration: ensure reward scores are well-calibrated (predicted 0.8 quality ≈ 80% human approval)
- Files: `internal/safety/reward_model.go`, `internal/safety/process_reward.go`

### 17.4 Adversarial Testing `P1` `L`
- [ ] **AT-1** — Red-team agent: adversarial agent that tries to trigger unsafe behavior in other agents
- [ ] **AT-2** — Prompt injection testing: automated injection attempts against all MCP tool inputs
- [ ] **AT-3** — Boundary probing: systematically test safety boundaries with edge cases
- [ ] **AT-4** — Regression suite: catalog of previously-found safety issues, re-test on every release
- [ ] **AT-5** — Chaos engineering: randomly inject failures (provider timeout, disk full, network partition) and verify recovery
- [ ] **AT-6** — Adversarial code review: submit intentionally buggy code, verify agents catch issues
- [ ] **AT-7** — Privilege escalation testing: verify agents cannot exceed their autonomy level
- Files: `internal/safety/redteam.go`, `internal/safety/chaos.go`, `internal/safety/injection_test.go`

### 17.5 Compliance & Audit `P2` `L`
- [ ] **AU-1** — Immutable audit log: append-only log of all agent actions, decisions, and outputs (SQLite WAL)
- [ ] **AU-2** — Data lineage: track which inputs produced which outputs, full provenance chain
- [ ] **AU-3** — Model cards: generate standardized model cards for each provider configuration in use
- [ ] **AU-4** — Risk assessment: automated risk scoring for each autonomous operation
- [ ] **AU-5** — Retention policies: configurable data retention periods, automated purge of expired data
- [ ] **AU-6** — Export compliance data: generate audit reports in standard formats (JSON, CSV, SARIF)
- [ ] **AU-7** — Access control audit: log who/what accessed which resources and when
- Files: `internal/safety/audit.go`, `internal/safety/lineage.go`, `internal/safety/model_card.go`

---

## Phase 18: World Models & Predictive Systems `[NEW]`

Predict outcomes before executing, simulate build/test results, estimate task completion times, and model codebase evolution to enable proactive optimization.

> **Research basis:** World models for code, neuro-symbolic programming, predictive code analysis, digital twins.

### 18.1 Build/Test Prediction `P1` `XL`
- [ ] **BP-1** — Build outcome predictor: given a diff, predict probability of build success (logistic regression on diff features)
- [ ] **BP-2** — Test impact analysis: predict which tests will fail from a given diff (file dependency graph + historical co-failure)
- [ ] **BP-3** — Test prioritization: run most-likely-to-fail tests first, skip low-risk tests in fast mode
- [ ] **BP-4** — Flaky test detector: identify tests with non-deterministic outcomes from historical data
- [ ] **BP-5** — Build time estimator: predict compilation + test duration from diff size and affected packages
- [ ] **BP-6** — Failure root cause predictor: given a test failure, predict most likely root cause from diff + error pattern
- [ ] **BP-7** — Merge conflict predictor: estimate conflict probability between parallel branches
- [ ] **BP-8** — CI pipeline optimizer: predict which CI stages can be skipped based on diff analysis
- Files: `internal/predict/build.go`, `internal/predict/test_impact.go`, `internal/predict/flaky.go`

### 18.2 Task Completion Estimation `P1` `L`
- [ ] **TC-1** — Duration estimator: predict task completion time from prompt features + historical data
- [ ] **TC-2** — Effort decomposition: break estimated effort into sub-components (research, implement, test, review)
- [ ] **TC-3** — Confidence intervals: provide P25/P50/P75/P95 estimates, not point estimates
- [ ] **TC-4** — Progress tracking: compare actual progress against estimate, flag tasks falling behind
- [ ] **TC-5** — Estimation calibration: track predicted vs actual durations, adjust model over time
- [ ] **TC-6** — Fleet-wide ETA: aggregate task ETAs into fleet-level completion forecast
- [ ] **TC-7** — Sprint planning support: suggest optimal task grouping to maximize sprint throughput
- Files: `internal/predict/duration.go`, `internal/predict/calibration.go`

### 18.3 Codebase Evolution Model `P2` `L`
- [ ] **CE-1** — Code complexity trend: track cyclomatic complexity, coupling, and cohesion over time per package
- [ ] **CE-2** — Technical debt forecasting: project debt accumulation rate, estimate cleanup effort
- [ ] **CE-3** — Dependency graph analysis: identify circular dependencies, suggest decoupling points
- [ ] **CE-4** — Hotspot detection: identify files with high churn + high complexity (bug magnets)
- [ ] **CE-5** — Architecture drift detection: compare actual package dependencies against intended architecture
- [ ] **CE-6** — API surface evolution: track public API changes, detect breaking changes automatically
- [ ] **CE-7** — Code clone detection: find duplicated code blocks that should be refactored
- Files: `internal/predict/evolution.go`, `internal/predict/debt.go`, `internal/predict/hotspot.go`

### 18.4 Simulation & Digital Twins `P2` `XL`
- [ ] **DT-1** — Environment simulator: model provider latency, rate limits, and costs for capacity planning
- [ ] **DT-2** — Fleet simulator: simulate N-agent workloads to test scheduling algorithms offline
- [ ] **DT-3** — Config simulator: predict impact of config changes before applying to production fleet
- [ ] **DT-4** — Failure scenario simulator: model cascading failures, verify recovery procedures
- [ ] **DT-5** — Cost simulator: project monthly costs under different fleet configurations
- [ ] **DT-6** — A/B test simulator: estimate required sample size and expected lift before running real A/B tests
- Files: `internal/predict/simulator.go`, `internal/predict/fleet_sim.go`, `internal/predict/cost_sim.go`

---

## Phase 19: Cross-Repository Orchestration `[NEW]`

Coordinate agent work across multiple repositories, manage cross-repo dependencies, and enable organization-wide code intelligence.

> **Research basis:** MetaGPT, ChatDev multi-agent repos, monorepo tooling (Bazel, Nx, Turborepo).

### 19.1 Multi-Repo Coordination `P0` `XL`
- [ ] **MR-1** — Repository registry: catalog of all managed repos with metadata (language, team, dependencies, build system)
- [ ] **MR-2** — Cross-repo dependency graph: model inter-repo dependencies (Go modules, npm packages, API contracts)
- [ ] **MR-3** — Coordinated PRs: create linked PRs across repos when a change spans boundaries
- [ ] **MR-4** — Cross-repo atomic commits: stage changes in multiple repos, merge all-or-nothing
- [ ] **MR-5** — API contract validation: when repo A changes an API, verify repo B still compiles/passes
- [ ] **MR-6** — Dependency update propagation: when library repo releases, trigger consumers to update
- [ ] **MR-7** — Cross-repo search: unified code search across all managed repositories
- [ ] **MR-8** — Repo health dashboard: aggregate build status, test coverage, dependency freshness across all repos
- Files: `internal/multirepo/registry.go`, `internal/multirepo/depgraph.go`, `internal/multirepo/coordinated_pr.go`

### 19.2 Organization-Wide Intelligence `P1` `L`
- [ ] **OI-1** — Pattern mining across repos: identify common code patterns, suggest shared libraries
- [ ] **OI-2** — Style consistency: enforce organizational coding standards across all repos
- [ ] **OI-3** — Knowledge transfer: when agent learns something in repo A, make it available in repo B
- [ ] **OI-4** — Team expertise mapping: track which agents/teams are experts in which repos/domains
- [ ] **OI-5** — Impact analysis: given a change in repo A, predict affected repos and teams
- [ ] **OI-6** — Migration coordinator: orchestrate large-scale migrations (Go version, API changes) across all repos
- [ ] **OI-7** — Org-wide metrics: aggregate LOC, test coverage, build times, PR velocity across all repos
- Files: `internal/multirepo/intelligence.go`, `internal/multirepo/migration.go`

### 19.3 Automated Dependency Management `P1` `L`
- [ ] **DM-1** — Dependency scanner: audit all repos for outdated, vulnerable, and unmaintained dependencies
- [ ] **DM-2** — Auto-update PRs: create dependency update PRs with changelog summaries and risk assessment
- [ ] **DM-3** — License compliance: scan dependency trees for license conflicts (GPL in MIT project)
- [ ] **DM-4** — Supply chain verification: verify dependency checksums, signatures, and provenance (SLSA)
- [ ] **DM-5** — Breaking change detection: analyze changelogs and diffs to predict if update will break
- [ ] **DM-6** — Dependency consolidation: identify repos using different versions of same dependency, align
- [ ] **DM-7** — Vulnerability response: when CVE published, automatically assess impact and create fix PRs
- Files: `internal/multirepo/deps.go`, `internal/multirepo/license.go`, `internal/multirepo/supply_chain.go`

### 19.4 Release Orchestration `P2` `L`
- [ ] **RO-1** — Semantic versioning automation: determine version bump from conventional commit analysis
- [ ] **RO-2** — Release train: coordinate releases across dependent repos in dependency order
- [ ] **RO-3** — Changelog generation: aggregate commit messages into structured changelogs per repo
- [ ] **RO-4** — Feature flag management: create/toggle/retire feature flags across repos
- [ ] **RO-5** — Canary releases: deploy to subset of fleet, monitor metrics, auto-promote or rollback
- [ ] **RO-6** — Release approval workflow: multi-stage approval (CI green → security scan → team lead → deploy)
- [ ] **RO-7** — Rollback automation: one-command rollback to previous known-good version across all repos
- Files: `internal/multirepo/release.go`, `internal/multirepo/changelog.go`, `internal/multirepo/canary.go`

---

## Phase 20: Agent Marketplace & Ecosystem `[NEW]`

Plugin marketplace, tool registries, agent templates, community contributions, and ecosystem integration that enable third-party extensibility.

> **Research basis:** MCP registries, tool stores, plugin architectures (Terraform providers, K8s operators).

### 20.1 Plugin Architecture `P0` `XL`
- [ ] **PA-1** — Plugin SDK: Go interface for third-party plugins (lifecycle hooks, tool registration, event subscription)
- [ ] **PA-2** — Plugin discovery: scan `~/.ralph/plugins/` and registry for available plugins
- [ ] **PA-3** — Plugin sandboxing: run plugins in WASM (Wazero) sandbox with capability-based permissions
- [ ] **PA-4** — Plugin versioning: semver for plugins, compatibility matrix with ralphglasses versions
- [ ] **PA-5** — Plugin marketplace: index of community plugins with ratings, downloads, security audit status
- [ ] **PA-6** — Hot-reload plugins: load/unload plugins without restarting ralphglasses
- [ ] **PA-7** — Plugin configuration: per-plugin config in `.ralphrc` with validation
- [ ] **PA-8** — Plugin testing framework: test harness for plugin developers with mock fleet and sessions
- Files: `internal/plugin/sdk.go`, `internal/plugin/registry.go`, `internal/plugin/sandbox.go`, `internal/plugin/marketplace.go`

### 20.2 Tool Registry `P1` `L`
- [ ] **TG-1** — MCP tool registry: public index of MCP tools with descriptions, schemas, and usage examples
- [ ] **TG-2** — Tool discovery protocol: agents can search for tools by capability ("I need a tool that runs tests")
- [ ] **TG-3** — Tool composition: chain tools into pipelines (scan → fix → test → commit)
- [ ] **TG-4** — Tool versioning: tools have versions, agents pin to compatible versions
- [ ] **TG-5** — Tool quality metrics: track success rate, latency, cost per tool invocation
- [ ] **TG-6** — Tool recommendation: suggest relevant tools based on current task context
- [ ] **TG-7** — Custom tool creation: template for creating new MCP tools from natural language description
- Files: `internal/plugin/tool_registry.go`, `internal/plugin/tool_compose.go`

### 20.3 Agent Templates `P1` `L`
- [ ] **AT-1** — Template library: pre-built agent configurations for common roles (reviewer, implementer, tester, documenter)
- [ ] **AT-2** — Template marketplace: community-contributed agent templates with ratings
- [ ] **AT-3** — Template parameterization: templates accept variables (repo, language, style guide) for customization
- [ ] **AT-4** — Template versioning: templates evolve independently, pinned versions for reproducibility
- [ ] **AT-5** — Template composition: combine multiple templates into multi-agent team configurations
- [ ] **AT-6** — Template validation: verify template produces valid agent configuration before deployment
- [ ] **AT-7** — Template performance tracking: compare outcomes across agents using different templates
- Files: `internal/plugin/templates.go`, `internal/plugin/template_marketplace.go`

### 20.4 Ecosystem Integration `P2` `L`
- [ ] **EI-1** — GitHub App: install as GitHub App for automatic PR review, issue triage, CI integration
- [ ] **EI-2** — Slack integration: fleet status, alerts, and approvals in Slack channels
- [ ] **EI-3** — Linear/Jira sync: bidirectional sync between ralphglasses tasks and project management tools
- [ ] **EI-4** — Grafana dashboard: Prometheus metrics exporter + pre-built Grafana dashboards
- [ ] **EI-5** — PagerDuty integration: alert on fleet failures, cost anomalies, safety violations
- [ ] **EI-6** — Terraform provider: manage ralphglasses fleet configuration as infrastructure-as-code
- [ ] **EI-7** — VS Code extension: launch and monitor ralphglasses sessions from VS Code
- [ ] **EI-8** — Web dashboard: read-only web UI for fleet monitoring (Go + HTMX, no JS framework)
- Files: `internal/integrations/github_app.go`, `internal/integrations/slack.go`, `internal/integrations/grafana.go`

### 20.5 WASM Plugin Sandboxing `P1` `L`
- [ ] **WS-1** — Wazero runtime integration: embed Wazero WASM runtime for plugin execution
- [ ] **WS-2** — WASI preview2 support: filesystem, network, and clock access via capability grants
- [ ] **WS-3** — Plugin capability manifest: plugins declare required capabilities (fs:read, net:http, exec:shell)
- [ ] **WS-4** — Resource limits: per-plugin memory (64MB default), CPU time (5s per call), and fuel metering
- [ ] **WS-5** — Host function exports: expose ralphglasses API to plugins via WASM host functions
- [ ] **WS-6** — Extism SDK integration: use Extism for cross-language plugin development (Go, Rust, C, AssemblyScript)
- [ ] **WS-7** — Plugin communication: inter-plugin messaging via shared memory or host-mediated channels
- Files: `internal/plugin/wasm_runtime.go`, `internal/plugin/wasm_capabilities.go`, `internal/plugin/wasm_host.go`

### 20.6 WASM Deep Integration (Research-Grounded) `P1` `XL`

> **Sources:** wazero, Extism, waPC, WASM Component Model, TinyGo WASI P2. See R2-10 research.

- [ ] **WD-1** — Implement `PluginHost` ABI bridge as exported WASM host functions (waPC-style: `ralphglasses.register_tool` with msgpack-encoded `ToolHandler`) — acceptance: integration test spins up test WASM plugin, verifies tool registration round-trip in < 5ms
- [ ] **WD-2** — Compile and publish a TinyGo guest SDK (`internal/plugin/wasm/sdk/`) exporting `RegisterPlugin(name, version, init)` — acceptance: `GOOS=wasip1 GOARCH=wasm tinygo build` produces valid `.wasm` loadable by wazero backend
- [ ] **WD-3** — Define WASI capability policy in `plugin.json` manifests with explicit allow-lists for `fs_paths`, `env_vars`, `network` (default: deny-all) — acceptance: plugin requesting `os.ReadFile` outside granted path receives permission error from runtime
- [ ] **WD-4** — WASM Component Model via WIT definitions (`internal/plugin/wit/ralphglasses.wit`) with `import ralphglasses:host/events`, `export ralphglasses:plugin/lifecycle` — acceptance: TinyGo compiles WASI P2 component against bindings; existing gRPC and raw WASM paths unchanged
- [ ] **WD-5** — Add `ralphglasses_plugin_*` MCP tool group: `plugin_list`, `plugin_reload`, `plugin_sandbox_status` — acceptance: `ralphglasses_load_tool_group plugin` loads namespace without restart
- [ ] **WD-6** — WASM sandbox security hardening guide and threat model (`docs/WASM-PLUGINS.md`) with worked attack scenario (env var exfiltration) — acceptance: CI lint step `TestWASMSandboxPolicy` verifies deny-all defaults
- Files: `internal/plugin/wasm/`, `internal/plugin/wit/`, `docs/WASM-PLUGINS.md`

---

## Phase 21: Observability & Evaluation `[NEW]`

Deep observability into agent behavior, statistical evaluation frameworks, and continuous quality monitoring.

> **Research basis:** OpenTelemetry for LLMs, Langfuse, AgentBench, tau-bench.

### 21.1 LLM Observability `P0` `L`
- [ ] **LO-1** — OpenTelemetry integration: spans for every LLM call with prompt, response, tokens, latency, cost
- [ ] **LO-2** — Trace correlation: link LLM calls to sessions, tasks, and fleet operations via trace context
- [ ] **LO-3** — Token usage dashboards: real-time token consumption by provider, model, session, and task type
- [ ] **LO-4** — Prompt/response logging: configurable logging levels (off, metadata-only, full content)
- [ ] **LO-5** — Performance regression alerts: detect when P50/P99 latency increases beyond threshold
- [ ] **LO-6** — Cost attribution: break down costs by team, project, task type, and agent
- [ ] **LO-7** — Error taxonomy: classify errors (rate limit, context overflow, malformed response, timeout) with trends
- Files: `internal/telemetry/otel.go`, `internal/telemetry/llm_spans.go`, `internal/telemetry/cost_attribution.go`

### 21.2 Agent Evaluation Framework `P1` `L`
- [ ] **AE-1** — Benchmark suite: standardized tasks for measuring agent capability (code generation, bug fixing, testing)
- [ ] **AE-2** — SWE-bench integration: run against SWE-bench tasks for external comparison
- [ ] **AE-3** — Custom eval harness: define evaluation tasks with input, expected output, scoring function
- [ ] **AE-4** — Regression testing: run benchmark suite after config changes, flag capability regressions
- [ ] **AE-5** — Provider comparison: run same tasks against Claude/Gemini/Codex, compare quality/cost/speed
- [ ] **AE-6** — Temporal analysis: track benchmark scores over time, correlate with config and model changes
- [ ] **AE-7** — Statistical significance: require p < 0.05 before declaring A/B test winners
- Files: `internal/eval/benchmark.go`, `internal/eval/harness.go`, `internal/eval/comparison.go`

### 21.3 Continuous Quality Monitoring `P1` `L`
- [ ] **CQ-1** — Output quality scoring: automated scoring of every agent output (correctness, completeness, style)
- [ ] **CQ-2** — Quality SLOs: define and monitor quality service-level objectives (e.g., 95% of outputs score > 7/10)
- [ ] **CQ-3** — Quality alerts: notify when quality drops below SLO for any provider/task-type combination
- [ ] **CQ-4** — Human evaluation pipeline: sample outputs for human review, calibrate automated scoring
- [ ] **CQ-5** — Quality dashboards: TUI view showing quality trends, outliers, and provider comparison
- [ ] **CQ-6** — Quality-cost tradeoff visualization: scatter plot of quality vs cost per provider per task type
- [ ] **CQ-7** — Quality decomposition: break quality into sub-dimensions (correctness, style, tests, docs) with independent tracking
- Files: `internal/eval/quality.go`, `internal/eval/slo.go`, `internal/eval/dashboard.go`

### 21.4 Research-Grounded Evaluation (AgentBench, tau-bench, SWE-bench) `P1` `XL`

> **Sources:** AgentBench (ICLR 2024), tau-bench pass^k, SWE-bench Verified, MLE-bench, DevBench, RouteLLM. See R1-08 research.

- [ ] **RE-1** — Pass-k reliability harness: run same task k times per provider, record full distribution (not just mean) — acceptance: `pass_k` score diverges from single-shot rate on ≥1 task class; surfaced in `fleet_analytics`
- [ ] **RE-2** — Task-type stratification: `BenchmarkCase` gains `TaskClass` enum (code_fix, test_write, refactor, doc, web_action, long_horizon); per-class scores feed UCB1 bandit arms — acceptance: `provider_recommend` returns different providers for `code_fix` vs `doc` with confidence ≥ 0.80
- [ ] **RE-3** — Difficulty-tier routing (GAIA-style): lightweight classifier estimates task difficulty from prompt features; tier-1 tasks route to cheapest provider at ≥85% success — acceptance: tier-1 routing cost drops ≥20% on 50-task replay
- [ ] **RE-4** — Functional-correctness grading via test execution: `TestExecutionGrader` runs `go test` against agent's patch in sandbox, returns pass/fail — acceptance: `loop_benchmark` reports `test_pass_rate` per provider; `eval_changepoints` detects regressions
- [ ] **RE-5** — Automated benchmark generation from roadmap items: `BenchmarkGenerator` reads task specs via `roadmap_parse`, emits `BenchmarkCase` with acceptance predicates — acceptance: `loop_benchmark` runs auto-generated cases; ≥70% have evaluable predicates
- [ ] **RE-6** — Online quality drift detection: Supervisor ticks `DetectChangepoints` over rolling 50-observation window per provider, emits `QualityRegression` event — acceptance: synthetic degradation causes event within 2 ticks; bandit routes away
- [ ] **RE-7** — Multi-shot budget selector: run task up to `max_attempts` times, return best-scoring output — acceptance: tier-2 tasks improve ≥15% success rate vs single-shot
- [ ] **RE-8** — Phase-decomposed quality scoring (DevBench): grade each R&D cycle phase independently (plan, implementation, synthesis, merge) — acceptance: `cycle_synthesize` includes per-phase scores
- [ ] **RE-9** — Cross-provider leaderboard with time-series tracking and sparklines in TUI — acceptance: leaderboard shows distinguishable rankings on ≥2 task classes after 10+ runs
- Files: `internal/eval/pass_k.go`, `internal/eval/task_class.go`, `internal/eval/test_grader.go`, `internal/eval/leaderboard.go`

---

## Phase 22: DevOps & Infrastructure Automation `[NEW]`

Agents that automate DevOps workflows, manage infrastructure, and handle operational tasks autonomously.

> **Research basis:** GitHub Actions AI, Atlantis, semantic-release, Infracost, continuous profiling.

### 22.1 CI/CD Automation `P1` `L`
- [ ] **CI-1** — GitHub Actions generator: create optimized CI workflows from project analysis (language, test framework, deploy target)
- [ ] **CI-2** — CI failure analyzer: parse CI logs, identify root cause, suggest or auto-apply fix
- [ ] **CI-3** — Pipeline optimization: identify slow CI stages, suggest caching strategies and parallelization
- [ ] **CI-4** — Test selection: run only tests affected by changes (using dependency graph from Phase 18)
- [ ] **CI-5** — Deploy automation: trigger deployments based on branch/tag patterns with rollback on failure
- [ ] **CI-6** — CI cost tracking: estimate CI compute costs, optimize runner usage
- Files: `internal/devops/ci_gen.go`, `internal/devops/ci_analyzer.go`, `internal/devops/ci_optimize.go`

### 22.2 Infrastructure Management `P2` `L`
- [ ] **IM-1** — Infrastructure scanner: detect running services, ports, containers, databases
- [ ] **IM-2** — Docker compose generator: create docker-compose.yml from project analysis
- [ ] **IM-3** — K8s manifest generator: generate Kubernetes manifests from application requirements
- [ ] **IM-4** — Cost optimization: analyze cloud resource usage, suggest right-sizing and reserved instances
- [ ] **IM-5** — Secret rotation: automated credential rotation with zero-downtime rollover
- [ ] **IM-6** — Database migration agent: generate, validate, and apply schema migrations
- Files: `internal/devops/infra.go`, `internal/devops/docker.go`, `internal/devops/k8s_gen.go`

### 22.3 Performance Engineering `P2` `L`
- [ ] **PE-1** — Continuous profiling: periodic CPU/memory profiling with flame graph generation
- [ ] **PE-2** — Performance regression detection: compare profiles across commits, flag regressions
- [ ] **PE-3** — Auto-optimization suggestions: analyze profiles, suggest specific code optimizations
- [ ] **PE-4** — Load testing automation: generate load tests from API specs, run periodically
- [ ] **PE-5** — Memory leak detection: monitor heap growth over time, identify leaking allocations
- [ ] **PE-6** — Benchmark tracking: run Go benchmarks on every commit, track performance trends
- Files: `internal/devops/profiler.go`, `internal/devops/perf_regression.go`, `internal/devops/benchmark_tracker.go`

### 22.4 Security Scanning `P1` `L`
- [ ] **SS-1** — SAST integration: run Semgrep/CodeQL on every PR, auto-fix common findings
- [ ] **SS-2** — Dependency vulnerability scanning: check dependencies against CVE databases (OSV, NVD)
- [ ] **SS-3** — Secret detection: scan for hardcoded secrets, API keys, and credentials in code and history
- [ ] **SS-4** — Container image scanning: scan Docker images for known vulnerabilities
- [ ] **SS-5** — SBOM generation: produce Software Bill of Materials in SPDX/CycloneDX format
- [ ] **SS-6** — Security review agent: automated security-focused code review on every PR
- Files: `internal/devops/sast.go`, `internal/devops/vuln_scan.go`, `internal/devops/sbom.go`

### 22.5 Documentation Automation `P2` `M`
- [ ] **DA-1** — API documentation generation: extract Go doc comments, generate OpenAPI/Swagger specs
- [ ] **DA-2** — Architecture diagram generation: produce Mermaid diagrams from package dependency graph
- [ ] **DA-3** — README maintenance: auto-update README sections (badges, install, usage) on release
- [ ] **DA-4** — Changelog generation: structured changelogs from conventional commits
- [ ] **DA-5** — Code example validation: run code examples in docs, flag broken ones
- Files: `internal/devops/docs_gen.go`, `internal/devops/arch_diagram.go`

### 22.6 Advanced Testing Automation (Research-Grounded) `P1` `L`

> **Sources:** pgregory.net/rapid, go-gremlins, native Go fuzz, teatest, pact-go, vegeta. See R4-06 research.

- [ ] **TA-1** — Property-based tests for session state machine: `rapid` generators cover all `SessionStatus` + `CyclePhase` enums; properties assert no illegal transitions under arbitrary event sequences — acceptance: CI runs with `-count=500`; ≥1 hidden invariant violation found
- [ ] **TA-2** — Mutation testing gate via `go-gremlins` on `internal/session` and `internal/enhancer` — acceptance: mutation score reported as PR comment; PR that drops score >5% blocks merge
- [ ] **TA-3** — Native Go fuzz targets for prompt pipeline input parsing and MCP tool argument unmarshalling — acceptance: `FuzzEnhancerPipeline` and `FuzzMCPToolArgs` with seed corpus; nightly CI runs 60s
- [ ] **TA-4** — TUI golden-file snapshot tests for all BubbleTea views using `teatest` — acceptance: every view has `_golden_test.go`; `UPDATE_GOLDEN=1` regenerates; CI fails on unintentional diff
- [ ] **TA-5** — Pact consumer-driven contract tests for 10 core MCP tools — acceptance: `.pact` files checked in; provider verification runs in CI
- [ ] **TA-6** — In-process fault injection harness for supervisor event bus and session runner — acceptance: cover event bus partition, provider timeout, mid-cycle crash, budget exhaustion; no panics confirmed via `-race` and `goleak`
- [ ] **TA-7** — Vegeta load tests for MCP HTTP handler: 50/100/200 RPS for 30s — acceptance: p95 ≤ 200ms at 100 RPS; results stored as CI artifacts
- [ ] **TA-8** — Chaos/resilience test suite with `txtar`-driven scenarios (stalled_session, budget_exceeded, infinite_loop_guard) — acceptance: ≥8 scenarios; all terminate within bounded wall-clock time
- Files: `internal/session/property_test.go`, `internal/session/fuzz_test.go`, `internal/tui/views/*_golden_test.go`, `testdata/chaos/*.txtar`

### 22.7 Continuous Profiling Pipeline (Research-Grounded) `P2` `L`

> **Sources:** Pyroscope, Parca, benchstat, gobenchdata, PGO (Go 1.21+, Uber 2-14% CPU reduction). See R4-07 research.

- [ ] **CP-1** — Embed `net/http/pprof` behind authenticated `/debug/pprof` endpoint in MCP server — acceptance: CPU/heap/goroutine profiles collectable; block profile rate defaults off
- [ ] **CP-2** — Integrate Pyroscope push-mode SDK (`grafana/pyroscope-go`) with goroutine labels for `session_id`, `provider`, `cycle_phase` — acceptance: overhead < 2% in 30-minute soak run
- [ ] **CP-3** — Wire `benchstat` into CI: `go test -bench=. -count=10` compared between PR and main — acceptance: CI fails when any benchmark degrades >10% at p<0.05
- [ ] **CP-4** — `gobenchdata` continuous benchmark history on `gh-pages` with regression flagging in PRs — acceptance: history JSON updated on every push to main
- [ ] **CP-5** — PGO feedback loop: `CollectPGOProfile()` harvests production CPU profiles → `make pgo-build` recompiles — acceptance: measured 2-14% CPU reduction validated by `benchstat`
- [ ] **CP-6** — Flame graph rendering in TUI `ProfileView` from embedded pprof endpoint — acceptance: scrollable flame graph follows `ViewStack` breadcrumb pattern
- [ ] **CP-7** — Auto-profile on anomaly events: subscribe to `AnomalyDetected`, capture 30s CPU + heap → `.ralph/profiles/<timestamp>/` — acceptance: profiles older than 7 days auto-pruned
- [ ] **CP-8** — `loopbench` regression gate: `IsRegressed()` check before supervisor advances `baselining` → `proposed` — acceptance: detected regression emits `BenchmarkRegression` event and halts cycle
- Files: `internal/profiler/pprof.go`, `internal/profiler/pyroscope.go`, `internal/profiler/pgo.go`, `internal/tui/views/profile_view.go`

---

## Phase 23: Advanced Prompt Engineering `[NEW]`

Automated prompt optimization, compression, caching strategies, and prompt-as-code workflows.

> **Research basis:** LLMLingua, Selective Context, prompt distillation, DSPy.

### 23.1 Prompt Optimization Pipeline `P1` `L`
- [ ] **PO-1** — Prompt compression: reduce prompt token count while preserving semantic content (LLMLingua approach)
- [ ] **PO-2** — Context window optimizer: intelligently select which context to include based on task relevance
- [ ] **PO-3** — Prompt distillation: create shorter prompts that produce equivalent outputs to longer ones
- [ ] **PO-4** — Few-shot example selection: dynamically choose the most relevant examples for each task
- [ ] **PO-5** — Prompt versioning: git-like version control for prompt templates with diff and rollback
- [ ] **PO-6** — Prompt A/B testing: automated comparison of prompt variants with statistical significance testing
- [ ] **PO-7** — DSPy-style compilation: define prompts as programs, compile to optimized prompt strings
- Files: `internal/enhancer/compression.go`, `internal/enhancer/distillation.go`, `internal/enhancer/prompt_versioning.go`

### 23.2 Advanced Caching `P1` `L`
- [ ] **AC-1** — Semantic caching: cache by semantic similarity of prompts, not just exact match
- [ ] **AC-2** — Hierarchical caching: L1 (in-memory, 100ms) → L2 (SQLite, 10ms) → L3 (cloud, 100ms)
- [ ] **AC-3** — Cache warming: pre-populate cache with common prompt prefixes on startup
- [ ] **AC-4** — Cache invalidation: automatically invalidate when referenced files change
- [ ] **AC-5** — Cache analytics: hit rate, savings, most/least cached prompts, cache size management
- [ ] **AC-6** — Cross-session cache: share cache entries across sessions for common system prompts
- Files: `internal/session/semantic_cache.go`, `internal/session/cache_hierarchy.go`

### 23.3 RAG & Long-Context Intelligence (Research-Grounded) `P1` `XL`

> **Sources:** RAPTOR (arXiv:2401.18059), GraphRAG (arXiv:2404.16130), Self-RAG (arXiv:2310.11511), LLMLingua-2, Lost-in-the-Middle (Liu et al.), Agentic Plan Caching (NeurIPS 2025). See R3-03 research.

- [ ] **RG-1** — Hybrid BM25 + dense retrieval for roadmap/task search: replace Jaccard-only `weightedRelevance()` with two-stage pipeline + Reciprocal Rank Fusion — acceptance: relevance stddev > 0.25 on 50-query eval; P90 latency < 200ms
- [ ] **RG-2** — ColBERT late-interaction re-ranking for observation/finding retrieval via ONNX runtime (no Python) — acceptance: Recall@3 improves ≥3pp; reranker adds < 50ms P95
- [ ] **RG-3** — RAPTOR hierarchical cycle summary tree: bottom-up cluster tree over `.ralph/cycles/` with LLM-generated summaries — acceptance: `observation_summary` returns coherent cross-cycle themes; tree build < 30s/cycle
- [ ] **RG-4** — GraphRAG entity-relationship index: extract entities (providers, files, error codes) and relationships from findings → adjacency list in `.ralph/knowledge_graph.json` — acceptance: `finding_reason` answers include multi-hop graph paths; build from 50 findings < 60s
- [ ] **RG-5** — Self-RAG reflection gate in enhancer pipeline (position 12): if self-check scores below threshold, trigger targeted retrieval from cycle tree — acceptance: hallucination rate drops ≥20%; gate fires on ≤30% of prompts
- [ ] **RG-6** — LLMLingua-2 context compaction: token-importance scoring via small encoder, 4-8x compression at <5% quality loss — acceptance: compacted context retains 95%+ eval answers; no external API calls
- [ ] **RG-7** — Lost-in-the-middle mitigation: relevance-ordered context assembly with U-shape sandwich (rank-1 at position 0, rank-2 at position N-1) — acceptance: accuracy variance across positions drops ≥40%
- [ ] **RG-8** — Agentic plan caching: extract `PlanTemplate` from completed sessions, match new tasks via TF-IDF cosine (≥0.6), inject cached plan — acceptance: hit rate ≥60% on repeated-pattern tasks; ≥20% turn reduction; ≥30% cost reduction
- [ ] **RG-9** — Adaptive prompt-cache prefix optimizer: instrument actual cache hit rates per provider, auto-adjust cache boundary — acceptance: hit rate ≥75% steady-state; `session_budget` includes `cache_hit_rate_pct`
- [ ] **RG-10** — Long-context provider routing: tasks with context >100K tokens auto-route to Gemini (2M window), fallback to Claude + RAPTOR compression — acceptance: `provider_recommend` returns Gemini for `context_tokens > 100000`
- Files: `internal/rag/hybrid.go`, `internal/rag/colbert.go`, `internal/rag/raptor.go`, `internal/rag/graphrag.go`, `internal/enhancer/retrieval_gate.go`

---

## Phase 24: MoE-Inspired Provider Routing (Research-Grounded) `[NEW]`

Mixture-of-Experts routing strategies adapted to multi-provider fleet dispatch. Each provider is treated as a "specialist expert" with conditional routing based on task type, difficulty, and historical performance.

> **Sources:** DeepSeekMoE (arXiv:2401.06066), Mixtral (arXiv:2401.04088), Switch Transformer (arXiv:2101.03961), GShard, Expert Choice (arXiv:2202.09368), Soft MoE (arXiv:2308.00951), RouteLLM (arXiv:2406.18665). See R3-01 research.

### 24.1 Task Classification & Stratified Routing `P0` `XL`
- [ ] **MR-1** — Task complexity classifier: assign each prompt a category from {code, math, creative, research, refactor, debug, infra, multilingual} — acceptance: classifier assigns category to ≥95% of prompts; category persisted on Session; surfaced in fleet_analytics
- [ ] **MR-2** — Per-task-type bandit arms (stratified UCB1): each (provider, task_type) pair is a distinct arm — acceptance: `bandit_status` reports per-task-type reward means; Gemini code-task arm converges differently than creative arm
- [ ] **MR-3** — Shared provider lane (DeepSeekMoE pattern): current primary control-plane provider as always-on orchestrator, secondary providers as routed specialists — acceptance: role=orchestrator always dispatches to the configured control-plane provider (currently Codex); role=worker through bandit
- [ ] **MR-4** — Expert load balancing with capacity factors (GShard pattern): per-provider concurrency caps, overflow to next-best — acceptance: `gemini_capacity=2` prevents >2 concurrent; fleet_analytics reports utilization%
- [ ] **MR-5** — Cascade cost-quality threshold as first-class config knob (0.0=cheapest, 1.0=best) — acceptance: threshold=0.2 routes ≥70% to Gemini/Codex; threshold=0.9 routes ≥70% to Claude
- Files: `internal/fleet/moe_router.go`, `internal/fleet/task_classifier.go`, `internal/fleet/capacity.go`

### 24.2 Advanced Routing Patterns `P1` `L`
- [ ] **MR-6** — Soft ensemble dispatch for high-stakes tasks: dispatch to top-2 providers, merge via enhancer scorer, return winner — acceptance: `session_launch ensemble=true` spawns 2 sessions; both outputs saved; cost attributed correctly
- [ ] **MR-7** — Router confidence score and abstention: when confidence < 0.4 (cold-start arm), route to shared Claude lane — acceptance: `routing_confidence` field on Session; confidence distribution in fleet_analytics
- [ ] **MR-8** — Task embedding cache for router warm-start: cosine similarity > 0.85 to past task seeds bandit prior — acceptance: near-duplicate prompt reuses cached routing within 50ms; LRU cache max 1000 entries
- [ ] **MR-9** — Fine-grained micro-task decomposition: split compound prompts ("write tests AND update docs AND fix bug") into 3 micro-tasks, route each independently — acceptance: decomposer splits 2-3 part prompts; disableable via `decompose=false`
- [ ] **MR-10** — Router telemetry pipeline: `RouterOutcome` struct appended to `.ralph/router_outcomes.ndjson` after every session — acceptance: MCP tool `router_outcomes` returns last N with filter; schema versioned
- Files: `internal/fleet/ensemble.go`, `internal/fleet/decomposer.go`, `internal/fleet/router_telemetry.go`

---

## Phase 25: Federated Fleet Learning (Research-Grounded) `[NEW]`

Privacy-preserving federated learning across fleet nodes. Agents improve collectively without sharing raw data, using techniques from FedAvg, FlexLoRA, DP-SGD, and secure aggregation.

> **Sources:** McMahan FedAvg (arXiv:1602.05629), FlexLoRA (arXiv:2402.11505), FedHPL, FEDGEN, Shamir secret sharing, cross-silo FL. See R3-07 research.

### 25.1 Federated Aggregation Core `P1` `XL`
- [ ] **FL-1** — FedAvg gradient aggregator for fleet-wide reflexion patterns: collect per-node weight vectors, compute weighted average, distribute global prior — acceptance: global prior reduces cold-start failure ≥20%; raw JSONL never leaves node
- [ ] **FL-2** — DP-SGD noise injection for gradient privacy: Gaussian noise calibrated via Rényi DP (ε=8, δ=1e-5 default) — acceptance: per-round ε matches configured target ±5%; budget exhaustion halts sync
- [ ] **FL-3** — Cross-silo topology map: classify Tailscale peers as silo (synchronous FedAvg) or device (async SGD with stale tolerance) — acceptance: device dropout doesn't block silo rounds; `fleet_topology` MCP tool returns classification
- [ ] **FL-4** — Secure aggregation via Shamir (k,n) threshold secret sharing: coordinator learns only summed vector — acceptance: reconstruction fails with < k shares; fallback to DP when nodes < k+1
- Files: `internal/fedlearn/aggregator.go`, `internal/fedlearn/dp.go`, `internal/fedlearn/topology.go`, `internal/fedlearn/secure_agg.go`

### 25.2 Federated Specialization `P2` `L`
- [ ] **FL-5** — LoRA-rank federated fine-tuning bridge for enhancer: SVD-decomposed adapter deltas, FlexLoRA-style redistribution — acceptance: scoring accuracy improves ≥5% after 5 rounds; adapter delta < 500KB/round
- [ ] **FL-6** — Personalized FL for per-node provider preference (FedPer split-model): global shared trunk + private personalization head — acceptance: per-node selection accuracy ≥85% within 10 iterations; head survives restart
- [ ] **FL-7** — Federated soft-prompt tuning for shared planning instructions: ≤64 token soft-prompt prefix trained collectively — acceptance: planner completion rate increases ≥10% after 10 rounds
- [ ] **FL-8** — Data-free federated distillation (FEDGEN-style): synthetic episodes for regularizing local training — acceptance: ≥15% faster curriculum convergence; generator cost < $0.10/round
- [ ] **FL-9** — Federated bandit for fleet-wide provider routing: privatized (DP-noised, ε=4) count/reward summaries to coordinator — acceptance: fleet-wide win rate converges within 5% of oracle after 20 rounds across 3 nodes
- Files: `internal/fedlearn/lora.go`, `internal/fedlearn/personalization.go`, `internal/fedlearn/prompt_tuning.go`, `internal/fedlearn/distill.go`, `internal/fedlearn/fed_bandit.go`

---

## Updated Dependency Chain (Phases 13-23) `[NEW]`

```
Phase 13 (L3 Autonomy) ──────> Phase 14 (Memory) ───────> Phase 15 (Fleet Intel)
     |                              |                            |
     v                              v                            v
  13.1 (Self-Heal) ──────> 13.3 (Decision Engine)         15.1 (Router)
  13.2 (Config Auto) ────> 13.4 (Self-Optimize)           15.2 (Fleet Optimize)
  13.5 (Unattended) ─────> 17.1 (Safety) ──────────────> 17.4 (Adversarial Test)
                                                                 |
Phase 16 (Edge) ────────> Phase 18 (Prediction)                  v
  16.1 (Local Models) ──> 18.1 (Build Predict)           17.5 (Compliance)
  16.2 (Offline) ───────> 18.2 (Task Estimate)
                                                          Phase 19 (Multi-Repo)
Phase 20 (Marketplace)                                      19.1 (Coordination)
  20.1 (Plugin SDK) ────> 20.5 (WASM Sandbox)              19.3 (Deps)
  20.2 (Tool Registry) ─> 20.3 (Agent Templates)           19.4 (Releases)
  20.4 (Ecosystem) ─────> 21.1 (Observability)
                              |                           Phase 22 (DevOps)
                              v                             22.1 (CI/CD)
                          21.2 (Eval Framework)             22.4 (Security Scan)
                          21.3 (Quality Monitor)
                                                          Phase 23 (Prompts)
                                                            23.1 (Optimization)
                                                            23.2 (Caching)
```

**Critical path to L3:** 13.1 (self-heal) → 13.3 (decision engine) → 13.5 (unattended) → 17.1 (safety) → 14.1 (memory) → 15.1 (router)

---

## Scaling Bottleneck Analysis `[NEW]`

Deep codebase analysis (2026-03-30) identified these performance bottlenecks with estimated impact at scale:

| Bottleneck | Component | Impact at 100+ Sessions | Mitigation | Phase |
|-----------|-----------|------------------------|------------|-------|
| Single `Manager.mu` RWMutex | `session/manager.go` | All session ops serialize | Per-map lock splitting | 10.5.1 |
| No MCP handler concurrency limit | `mcpserver/middleware.go` | Unbounded goroutines | Semaphore (32 default) | 10.5.2 |
| In-process event bus | `events/bus.go` | Single-node, 1000-event cap | NATS JetStream | 10.5.3 |
| Static worker pool (max=8) | `fleet/coordinator.go` | Queue backs up | Auto-scaling | 10.5.4 |
| Fixed iteration depth | `session/loop.go` | Waste on easy, starve hard | Adaptive depth | 10.5.5 |
| Single-machine marathons | `session/manager_cycle.go` | CPU/memory ceiling | Multi-node distribution | 10.5.6 |
| Static cascade routing | `session/cascade.go` | Misses 2-4x cost savings | Contextual bandits | 10.5.7 |
| Worktree creation overhead | `session/worktree.go` | I/O bottleneck | Pooling + alternates | 10.5.8 |
| JSON file persistence | Multiple | Corrupt under concurrent writes | SQLite WAL | 10.5.9 |
| OutputHistory unbounded | `session/types.go` | Memory leak over time | Ring buffer + persistence | 10.5.1 |
| TUI 2s polling tick | `tui/app.go` | 444-line rebuild every tick | Event-driven updates | 10.5.3 |
| No virtual scrolling | TUI fleet dashboard | Unusable at 100+ sessions | Virtual list component | 10.5.3 |

### Codebase Statistics (2026-03-30 Snapshot)

| Metric | Value |
|--------|-------|
| Total packages | 37 |
| MCP tools | 222 total (218 grouped + 4 management), 30 deferred-load tool groups |
| TUI views | 19 (11% migrated to Phase 2 View interface) |
| Test files | 427 (114K LOC) |
| Coverage | 84.5% (target 90%) |
| Middleware layers | 5 (trace → timeout → instrumentation → eventbus → validation) |
| Event types | 32, in-process pub/sub, 1000-event ring buffer |
| Provider rate limits | Claude 50/min, Gemini 60/min, Codex 20/min |
| Autonomy levels | 4 (observe → auto-recover → auto-optimize → full-autonomy) |
| Supervisor tick | 60s, max chain depth 10 |
| Enhancer pipeline | 13 stages, 10-dimension scoring, 11+ lint rules |

---

## Updated Dependency Chain `[NEW]`

```
Phase 10.5 (Scaling) ----> Phase 11 (A2A) ----> Phase 12 (Tailscale)
     |                          |                       |
     v                          v                       v
  10.5.1 (Lock Split)     11.1 (SDK)              12.1 (Enrollment)
  10.5.2 (Handler Limit)  11.2 (Executor)          12.2 (ACL)
  10.5.9 (SQLite) -------> 10.5.6 (Multi-Node) --> 12.3 (Go SDK)
  10.5.3 (NATS) ---------> 11.5 (Federation) ----> 12.4 (tsnet)
  10.5.4 (Auto-Scale) ---> 11.3 (A2A Dispatch) --> 12.5 (Cloud VM)
  10.5.7 (Cost Engine) --> 10.5.5 (Adaptive Depth)
  10.5.8 (Git Scale) ----> 12.6 (Worktree Sync)
  10.5.10 (Monitoring) --> 10.5.11 (Autonomy Scale)
```

**Critical path:** 10.5.1 (lock split) → 10.5.9 (SQLite) → 10.5.6 (multi-node) → 11.2 (A2A executor) → 12.3 (Tailscale SDK)

---

## Priority Legend

| Label | Meaning |
|-------|---------|
| `P0` | Critical — blocks other work or causes data loss/waste |
| `P1` | Important — significant user/developer value |
| `P2` | Nice to have — polish, optimization, future-proofing |
| `S` | Small — <1 hour, single file |
| `M` | Medium — 1-4 hours, 2-5 files |
| `L` | Large — 4-16 hours, cross-package |
| `XL` | Extra large — multi-day, architectural change |

<!-- whiteclaw-rollout:start -->
## Whiteclaw-Derived Overhaul (2026-04-08)

This tranche applies the highest-value whiteclaw findings that fit this repo's real surface: engineer briefs, bounded skills/runbooks, searchable provenance, scoped MCP packaging, and explicit verification ladders.

### Strategic Focus
- Use whiteclaw patterns to harden the repo's operator front door: engineer briefs, discovery surfaces, and explicit verification ladders.
- Prefer typed contracts and reusable control-plane machinery over more handwritten transport or prompt glue.
- Keep the repo searchable and self-describing so future sweeps do not depend on raw code spelunking.

### Recommended Work
- [ ] [Structure] Document subtree ownership and canonical source boundaries across nested modules, packages, or roadmaps.
- [ ] [CI] Add at least one smoke workflow that proves the public build/test path still matches the docs.
- [ ] [Verification] Add a layered verification ladder: build/lint, help/startup, discovery/health, and one non-destructive end-to-end check.
- [ ] [Telemetry] Tighten `.ralph` verification, cost, recovery, and improvement-journal coverage around the flows that actually matter.
- [ ] [Discovery] Add or harden a discovery-first contract layer for catalog/search/schema/health before widening the mutating tool surface.
- [ ] [Public docs] Keep release, migration, and example docs aligned with the real public workflow.

### Rationale Snapshot
- Tier / lifecycle: `tier-1` / `active`
- Language profile: `Go`
- Visibility / sensitivity: `PUBLIC` / `public`
- Surface baseline: AGENTS=yes, skills=yes, codex=yes, mcp_manifest=configured, ralph=yes, roadmap=yes
- Whiteclaw transfers in scope: verification ladder, skill/runbook splits, manifest smoke tests, engineer brief
- Live repo notes: AGENTS, skills, Codex config, configured .mcp.json, .ralph, multi-module/workspace, nested roadmaps

<!-- whiteclaw-rollout:end -->

### Session Receipt: 2026-04-08 (Surfacekit Tranches 1 & 2)
**Tranches Completed:**
1. Restore UseWhen search coverage in surfacekit_tool_discover
2. Add fleet surface explorer output resource (surfacekit://fleet/surface-explorer)

**Repos Touched:**
- `surfacekit` (cmd/surfacekit-mcp/main.go, cmd/surfacekit-mcp/main_test.go)
- `ralphglasses` (ROADMAP.md notes)

**Validation Run:**
- `GOWORK=off go test ./...` passed successfully in `surfacekit`

**Push Outcome:**
- Completed natively in local workspace after mitigating earlier bwrap mount failure.

**Workflow Lessons & Autonomous Development Loop Improvements:**
- **Broken workspace detection:** Must preflight shell launch capability and not just rely on authorization state.
- **Fallback strategy:** Direct file reads and surgical tool edits can work in environments where mutating shell commands fail.
- **Reference synchronization:** Local ref receipts can move past earlier GitHub-only observations and must be refreshed before work starts.
- **Publish readiness:** Must distinguish current checkout branch from target main, especially when a repo is on a side branch.

---

## Cline CLI Integration — Perpetual Free-Tier R&D Orchestration `[NEW]`

> **Added:** 2026-04-09
> **Context:** Cline CLI added as 4th fleet provider, positioned as the default cheap/free-tier orchestrator for perpetual L1-L3 R&D loops. Tasks of higher complexity cascade to Claude/Codex/Gemini.
>
> **Design principle:** Leverage Cline's free models for perpetual autonomous development (L3 R&D), with complexity-aware routing that assigns expensive models only when task difficulty warrants it.

### Phase 1: Core Provider Integration (DONE)

- [x] **CLINE-1** — Register `ProviderCline` type constant and defaults `P0` `S`
  - File: `internal/session/types.go`, `providers.go`
  - **Status:** Complete — `ProviderCline = "cline"`, default model = `""` (Cline-managed)

- [x] **CLINE-2** — Implement `buildClineCmd()` command builder `P0` `M`
  - File: `internal/session/provider_cline.go`
  - **Status:** Complete — maps LaunchOptions → `cline --json --yolo --taskId/--continue` flags

- [x] **CLINE-3** — Implement `normalizeClineEvent()` stream normalizer `P0` `M`
  - File: `internal/session/provider_cline.go`
  - **Status:** Complete — parses NDJSON stream events into unified session events

- [x] **CLINE-4** — Wire into cascade routing as default cheap provider `P0` `S`
  - File: `internal/session/cascade.go`, `cascade_routing.go`
  - **Status:** Complete — `CheapProvider: ProviderCline`, free tier (cost=$0), MaxCheapTurns=20

- [x] **CLINE-5** — Full capability matrix (17 capabilities mapped) `P0` `M`
  - File: `internal/session/provider_capabilities.go`
  - **Status:** Complete — native: resume, permission_mode, project_instructions, MCP client, skills, hooks

- [x] **CLINE-6** — Cost tracking at $0.00/1M for free tier `P0` `S`
  - File: `internal/session/costs.go`
  - **Status:** Complete — zero-cost rate in `getProviderCostRate()`

### Phase 1.5: CLI Parity Gap Closure (Next — Discovered 2026-04-09)

> **Context:** Research audit of Cline CLI 2.0 feature surface identified 16 gaps.
> Tranches 1-2 shipped inline (doctor, scaffold, command builder). Remaining items below.

- [x] **CLINE-7** — Doctor check: `checkCline()` binary + version + auth config detection `P0` `S`
  - File: `cmd/doctor.go`
  - **Status:** Complete — checks PATH, `cline version`, `~/.cline/data/` or `$CLINE_DIR`

- [x] **CLINE-8** — Scaffold: `.clinerules` + `.cline/mcp.json` in `ralphglasses init` `P0` `S`
  - File: `internal/repofiles/scaffold.go`
  - **Status:** Complete — generates project rules and MCP server config

- [x] **CLINE-9** — Command builder: `--thinking`, `--images`, `--config`, `--auto-approve-all`, stdin pipe, `CLINE_COMMAND_PERMISSIONS` `P0` `M`
  - File: `internal/session/provider_cline.go`, `types.go`
  - **Status:** Complete — 6 new flag mappings + complexity-based command sandboxing

- [ ] **CLINE-9a** — Add `repo_surface_audit` Cline surface check (`.clinerules`, `.cline/mcp.json`) `P1` `S`
  - File: `internal/mcpserver/tools_repo.go`
  - **Acceptance:** `repo_surface_audit` reports Cline config presence/absence

- [ ] **CLINE-9b** — Add `.cline/` projection target to `sync-provider-roles.py` `P1` `S`
  - File: `scripts/sync-provider-roles.py`
  - **Acceptance:** `python3 scripts/sync-provider-roles.py` generates `.clinerules` role hints

- [ ] **CLINE-9c** — Update `docs/PROVIDER-PARITY-OBJECTIVES.md` with Cline column `P1` `S`
  - File: `docs/PROVIDER-PARITY-OBJECTIVES.md`
  - **Acceptance:** Parity matrix includes Cline row with all capability statuses

- [ ] **CLINE-9d** — Update `docs/PROVIDER-SETUP.md` with Cline setup instructions `P1` `S`
  - File: `docs/PROVIDER-SETUP.md`
  - **Acceptance:** Setup guide covers `cline auth`, `CLINE_DIR`, free-tier onboarding

- [ ] **CLINE-9e** — `cline history` integration for session telemetry `P2` `M`
  - File: `internal/session/provider_cline.go`
  - After session completion, parse `cline history --limit 1 --json` to extract task metadata
  - **Acceptance:** Session metadata includes Cline-reported task ID and cost

- [ ] **CLINE-9f** — Programmatic Cline workspace config via `--config` dirs `P2` `M`
  - File: `internal/session/provider_cline.go`
  - Write workspace-level Cline config (auto-approve patterns, model routing) before launch
  - **Acceptance:** Concurrent Cline sessions use isolated config without conflicts

### Phase 2: Complexity-Aware Task Routing (Next)

- [ ] **CLINE-10** — Implement L1/L2/L3/L4 task complexity classifier `P0` `L`
  - File: `internal/session/task_classifier.go` (new)
  - Classify incoming prompts into complexity levels using heuristics + optional LLM pre-classification
  - L1 (trivial): lint, format, classify, simple grep → Cline free tier
  - L2 (routine): docs, refactor, simple feature → Cline free tier
  - L3 (complex): codegen, test writing, debugging → Cline free tier OR Codex (based on confidence)
  - L4 (expert): architecture, planning, security review → Claude/Codex only
  - **Acceptance:** `task_classify` MCP tool returns complexity level + suggested provider with 80%+ accuracy on test corpus

- [ ] **CLINE-11** — Adaptive escalation with confidence feedback loop `P1` `L`
  - File: `internal/session/cascade.go`
  - Track per-task-type success rates by provider; dynamically adjust escalation thresholds
  - If Cline successfully completes L3 tasks 5+ times, promote to handling them without escalation
  - If Cline fails L2 tasks, temporarily escalate to Codex until success rate recovers
  - **Acceptance:** Escalation thresholds converge after 50+ cascade decisions

- [ ] **CLINE-12** — Free-tier budget tracking and quota monitoring `P1` `M`
  - File: `internal/session/budget.go`
  - Track Cline free-tier usage: requests/day, tokens/day, rate limits hit
  - Surface quota warnings before hitting limits
  - Auto-switch to paid provider when free quota exhausted
  - **Acceptance:** `fleet_analytics` shows Cline usage stats and quota remaining

- [ ] **CLINE-13** — Task queue with priority-based provider assignment `P1` `L`
  - File: `internal/session/task_queue.go` (new)
  - Priority queue where L1-L2 tasks auto-assign to Cline, L3 tasks go to Cline with escalation fallback, L4 tasks go directly to Claude/Codex
  - Cline sessions run perpetually, picking up L1-L3 tasks from the queue
  - Expensive providers only activated when queue contains L4 tasks or Cline escalates
  - **Acceptance:** Queue processes 100+ tasks with correct provider assignment and zero wasted expensive-model calls on trivial tasks

### Phase 3: Perpetual Autonomous R&D Loop

- [ ] **CLINE-20** — Perpetual Cline R&D daemon mode `P0` `L`
  - File: `internal/session/perpetual.go` (new), `cmd/perpetual.go`
  - Long-running daemon that continuously:
    1. Pulls roadmap items tagged L1-L3
    2. Launches Cline sessions to implement them
    3. Runs verification (build + test)
    4. Escalates failures to expensive providers
    5. Records results in improvement_journal.jsonl
    6. Loops forever until stopped
  - **Acceptance:** Daemon runs 24h+ without intervention, completing 10+ roadmap items/day

- [ ] **CLINE-21** — Self-improvement loop: Cline improves Cline integration `P1` `L`
  - File: `internal/session/self_improve.go`
  - Meta-loop where Cline sessions analyze their own failure modes and generate improvement patches
  - Uses improvement_journal.jsonl patterns to identify recurring issues
  - Proposes `.clinerules` refinements and cascade threshold adjustments
  - **Acceptance:** Self-improvement loop generates at least 3 actionable improvements per week

- [ ] **CLINE-22** — Cline-led roadmap triage and task decomposition `P2` `M`
  - File: `internal/roadmap/triage.go`
  - Cline sessions analyze roadmap items, estimate complexity, suggest decomposition into subtasks
  - Pre-classify tasks before they enter the work queue
  - **Acceptance:** `roadmap_triage` produces complexity estimates within ±1 level of human assessment

- [ ] **CLINE-23** — Cross-provider session handoff: Cline → Claude/Codex `P1` `M`
  - File: `internal/session/handoff.go`
  - When Cline escalates, hand off full context (progress, files changed, test results) to expensive provider
  - Expensive provider picks up where Cline left off instead of starting from scratch
  - **Acceptance:** Escalated sessions complete 30%+ faster than cold-start on expensive provider

### Phase 4: Fleet Orchestration Enhancements

- [ ] **CLINE-30** — Multi-Cline parallel session pool `P1` `L`
  - File: `internal/session/pool.go` (new)
  - Run N concurrent Cline sessions across different repos/tasks
  - Coordinate via shared task queue; prevent duplicate work
  - Scale based on free-tier rate limits (auto-throttle when approaching quota)
  - **Acceptance:** 5+ parallel Cline sessions running without conflicts or rate limit errors

- [ ] **CLINE-31** — Cline session health monitoring and auto-restart `P1` `M`
  - File: `internal/session/health.go`
  - Monitor Cline sessions for: stuck loops, rate limit errors, context window exhaustion
  - Auto-restart with reduced scope or different prompt on failure
  - Circuit breaker: pause Cline if failure rate exceeds 50% over 10 sessions
  - **Acceptance:** Cline sessions auto-recover from transient failures without operator intervention

- [ ] **CLINE-32** — Provider cost dashboard: free vs paid utilization `P2` `S`
  - File: `internal/tui/views/cost_dashboard.go`
  - TUI view showing: Cline free-tier utilization %, tasks completed by provider, cost savings from cascade routing
  - Compare actual spend vs hypothetical spend if all tasks used expensive providers
  - **Acceptance:** Dashboard shows real-time cost savings from free-tier routing

- [ ] **CLINE-33** — Intelligent model selection within Cline `P2` `M`
  - File: `internal/session/provider_cline.go`
  - When Cline supports model selection via `--model`, route to optimal free model based on task type
  - Track per-model success rates within Cline's available models
  - **Acceptance:** Model selection improves task completion rate by 10%+ vs random model assignment

### Phase 5: Repo Surface & Developer Experience

- [x] **CLINE-40** — `.cline/` repo surface scaffolding `P1` `S`
  - File: `internal/repofiles/scaffold.go`
  - **Status:** Complete (shipped as CLINE-8) — `ralphglasses init` generates `.clinerules` + `.cline/mcp.json`

- [ ] **CLINE-41** — Cline skill projection from `.agents/skills/` `P2` `S`
  - File: `scripts/sync-provider-roles.py`
  - Extend role/skill sync to generate Cline-compatible skill definitions
  - **Acceptance:** `make skill-surface` generates `.cline/skills/` from canonical `.agents/skills/`

- [x] **CLINE-42** — Doctor check: Cline CLI availability and version `P1` `S`
  - File: `cmd/doctor.go`
  - **Status:** Complete (shipped as CLINE-7) — binary, version, `~/.cline/data/` or `$CLINE_DIR` auth check

- [ ] **CLINE-43** — TUI provider filter: show/hide Cline sessions `P2` `S`
  - File: `internal/tui/views/overview.go`
  - Filter sessions by provider; highlight free-tier sessions differently
  - **Acceptance:** TUI filter works for all 4 providers including Cline

- [ ] **CLINE-44** — Cline-specific hooks directory for ralphglasses events `P2` `M`
  - File: `internal/session/provider_cline.go`
  - Generate per-session hooks dir with pre/post hooks for: task start, verification, escalation, completion
  - Feed hook outputs back into session state
  - **Acceptance:** `--hooks-dir` populated with working hooks that log to `.ralph/hooks.log`

### Phase 6: Advanced Integration

- [ ] **CLINE-50** — A2A (Agent-to-Agent) delegation: Cline ↔ Claude/Codex `P2` `L`
  - File: `internal/session/a2a_cline.go` (new)
  - Cline sessions can delegate sub-tasks to Claude/Codex MCP servers and vice versa
  - Uses existing `a2a` tool group for protocol compatibility
  - **Acceptance:** Cline can delegate a subtask to Claude and receive results back within same session

- [ ] **CLINE-51** — Cline as MCP client consuming ralphglasses tools `P1` `M`
  - File: `.cline/mcp.json` (repo config)
  - Ensure Cline sessions launched by ralphglasses have MCP access to all 222+ ralphglasses tools
  - Validate tool accessibility in doctor check
  - **Acceptance:** Cline sessions can call `ralphglasses_loop_status`, `fleet_status`, etc.

- [ ] **CLINE-52** — Prompt optimization for free-tier models `P2` `M`
  - File: `internal/enhancer/cline_optimizer.go` (new)
  - Adapt prompts for Cline's free-tier models: shorter context, more explicit instructions, structured output hints
  - Use prompt scoring pipeline to A/B test prompt variants
  - **Acceptance:** Optimized prompts improve Cline task completion rate by 15%+ vs unoptimized

- [ ] **CLINE-53** — Fleet-wide Cline session telemetry `P2` `M`
  - File: `internal/telemetry/telemetry.go`
  - Track: tasks/day by complexity level, escalation rate, free-tier quota utilization, session duration distribution
  - Export to `.ralph/telemetry/cline_fleet.json`
  - **Acceptance:** `fleet_analytics provider=cline` returns meaningful utilization metrics
