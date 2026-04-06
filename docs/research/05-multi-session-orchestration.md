# Research Phase 5: Multi-Session Orchestration

Research date: 2026-03-22

Covers fleet management, session lifecycle, worktree isolation, and team delegation.
Maps to ROADMAP Phases 2 (Multi-Session Fleet Management), 6.1 (Native ralph loop engine), and 6.3 (Cross-session coordination).

---

## 1. Executive Summary

Ralphglasses already has a functional multi-session orchestration layer: the `Manager` in `internal/session/manager.go` tracks concurrent sessions, teams, workflow runs, and perpetual loops. Team delegation (`LaunchTeam`), workflow execution with dependency DAGs (`executeWorkflow`), and the planner/worker/verifier loop (`StepLoop`) are operational. However, the system has five critical gaps that block ROADMAP Phases 2 and 6:

1. **Persistence is file-based JSON** -- no transactional guarantees, no queryable history, no crash recovery beyond 24-hour session files. ROADMAP 2.1 calls for SQLite with WAL mode.
2. **Worktree isolation is loop-only** -- `createLoopWorktree` in `loop.go` creates worktrees for loop iterations but regular sessions and team workers share the repo working directory. ROADMAP 2.2 calls for a general `internal/worktree/` package.
3. **Single global mutex** -- `Manager.mu` serializes all operations. At scale (10+ concurrent sessions), this creates contention on launch, status polling, team correlation, and persistence.
4. **No cross-session file collision detection** -- two sessions targeting the same repo can silently clobber each other's changes. ROADMAP 6.3 calls for dedup and conflict resolution.
5. **MaxConcurrentWorkers hardcoded to 1** -- the loop profile explicitly rejects `> 1` workers (loop.go:373-375), blocking parallel worker execution.

External projects (claude-squad, hcom, tsk) have solved several of these problems with patterns directly applicable to ralphglasses.

---

## 2. Current State Analysis

### 2.1 Session Manager Architecture

**File:** `internal/session/manager.go`

The `Manager` struct is the central orchestration point:

```go
type Manager struct {
    mu            sync.Mutex
    sessions      map[string]*Session
    teams         map[string]*TeamStatus
    workflowRuns  map[string]*WorkflowRun
    loops         map[string]*LoopRun
    bus           *events.Bus
    stateDir      string
    launchSession func(context.Context, LaunchOptions) (*Session, error)
    waitSession   func(context.Context, *Session) error
    Enhancer      *enhancer.HybridEngine
}
```

Key characteristics:
- **Single `sync.Mutex`** guards all four maps plus `stateDir`. Every `Launch`, `Stop`, `List`, `Get`, `GetTeam`, `LaunchTeam`, and `LoadExternalSessions` acquires this lock.
- **Session launch is non-blocking** -- `launch()` in `runner.go` starts a goroutine and returns immediately. The background goroutine (`runSession`) parses streaming output and updates session state under `s.mu`.
- **Persistence is fire-and-forget** -- `PersistSession` writes JSON to `~/.ralphglasses/sessions/<id>.json`. Errors are silently discarded (`_ = os.WriteFile`).
- **Cross-process discovery** via `LoadExternalSessions()` reads JSON files written by the MCP server (separate process), merging unknown sessions into memory with a 24-hour TTL.

### 2.2 Team Delegation

**File:** `internal/session/manager.go` (lines 263-395)

`LaunchTeam` creates a "lead" session with an MCP-tool-aware prompt instructing it to delegate work via `ralphglasses_session_launch`. Worker sessions are launched by the lead agent at runtime -- ralphglasses does not directly launch workers. Task correlation happens via string matching (`strings.Contains(w.Prompt, task.Description)`) in `correlateTaskStatuses`.

Limitations:
- Correlation is fragile -- if the lead rewrites the task description, the match fails.
- No structured task assignment ID links lead tasks to worker sessions.
- `DelegateTask` appends tasks but has no mechanism to assign them to specific workers.
- Worker provider is suggested in the lead prompt but not enforced.

### 2.3 Workflow Execution

**File:** `internal/session/workflow.go`

The workflow engine supports:
- YAML-defined multi-step workflows with `depends_on` edges.
- Topological sort with cycle detection (`ValidateWorkflow`).
- Parallel execution of independent steps via `runWorkflowParallelGroup` using `sync.WaitGroup`.
- Step blocking on failed dependencies (cascade failure).

The workflow engine is well-structured but not connected to worktree isolation -- all steps run in the same repo directory.

### 2.4 Perpetual Loop (Planner/Worker/Verifier)

**File:** `internal/session/loop.go`

The loop creates isolated worktrees per iteration:

```go
func createLoopWorktree(ctx, repoPath, loopID, iteration) (worktreePath, branch, error)
// Path: .ralph/worktrees/loops/<sanitized-loop-id>/<iteration-number>
// Branch: ralph/<sanitized-loop-id>/<iteration-number>
```

Each iteration:
1. Planner session produces a `LoopTask` (title + prompt).
2. Worker session runs in an isolated worktree.
3. Verification commands execute in the worktree.
4. State persists to `~/.ralphglasses/sessions/loops/<id>.json`.

The loop enforces `MaxConcurrentWorkers == 1` (line 373-375), preventing parallel workers even when the profile requests more.

### 2.5 Supporting Infrastructure

| Component | File | Purpose |
|-----------|------|---------|
| Rate limiter | `ratelimit.go` | Per-provider sliding-window (1-min) rate limiting |
| Budget enforcer | `budget.go` | Secondary budget check (90% headroom), cost ledger JSONL |
| Health checks | `health.go` | Binary availability + env var checks per provider |
| Failover | `failover.go` | Ordered provider chain with health pre-checks |
| Checkpoints | `checkpoint.go` | Git commit + tag on session boundaries |
| Git info | `gitinfo.go` | `git log` and `git diff` within time windows |

### 2.6 Concurrency Model

The current concurrency model has two layers of mutexes:

1. **`Manager.mu`** -- protects the four maps (sessions, teams, workflowRuns, loops).
2. **`Session.mu`** -- protects individual session state (status, output, cost).

Potential deadlock vectors:
- `Stop()` acquires `Manager.mu`, releases it, then acquires `Session.mu`. This is safe because the lock ordering is always Manager -> Session.
- `correlateTaskStatuses` is called with `Manager.mu` held and acquires `Session.mu` for each worker. This is safe under current lock ordering but creates contention with long-running session status updates.
- `LoadExternalSessions` holds `Manager.mu` while reading disk and unmarshaling JSON. Under heavy I/O load, this can stall all session operations.
- `PersistSession` acquires `Session.mu` for marshaling -- correctly released before file I/O. However, it fires inside `LoadExternalSessions` via `go m.PersistSession(existing)` while `Manager.mu` is held, creating a goroutine that will compete for `Session.mu`.

---

## 3. Gap Analysis

### Gap 1: File-Based Persistence vs. ROADMAP 2.1 (SQLite)

| Aspect | Current State | ROADMAP 2.1 Target |
|--------|--------------|-------------------|
| Storage | JSON files in `~/.ralphglasses/sessions/` | SQLite with WAL mode |
| Crash recovery | 24-hour TTL, no transactional writes | ACID transactions |
| Queryability | None (full scan of JSON files) | SQL queries with indexes |
| Event log | Event bus (in-memory ring buffer) | Persistent event log table |
| State machine | Implicit in code flow | Enforced transition table |
| Cross-process | File polling in `LoadExternalSessions` | WAL-mode concurrent readers |

**Effort to close:** Large. Requires schema design, migration system, connection pool, CRUD layer, and replacing all `PersistSession`/`LoadExternalSessions` calls.

### Gap 2: Worktree Isolation Limited to Loops (ROADMAP 2.2)

| Aspect | Current State | ROADMAP 2.2 Target |
|--------|--------------|-------------------|
| Loop iterations | Isolated via `createLoopWorktree` | Same |
| Regular sessions | Run in original repo directory | Isolated worktree per session |
| Team workers | Run in original repo directory | Isolated worktree per worker |
| Workflow steps | Run in original repo directory | Isolated worktree per step |
| Merge-back | Not implemented | `git merge --no-ff` with conflict detection |
| Cleanup | Not implemented (worktrees accumulate) | Auto-cleanup on session stop/archive |

**Effort to close:** Medium. The `createLoopWorktree` function already demonstrates the pattern. Need to generalize into `internal/worktree/` and wire it into `Launch`, `LaunchTeam`, and `executeWorkflow`.

### Gap 3: Single Global Mutex (Scalability)

The `Manager.mu` serializes all operations. With 10+ concurrent sessions (ROADMAP Phase 2 target), this creates:
- Launch contention: every `Launch` holds the lock for map insert + event publish.
- Polling contention: `waitForSession` polls every 200ms, and `GetTeam` holds the lock while correlating task statuses across all workers.
- I/O under lock: `LoadExternalSessions` holds the lock while reading disk.

**Effort to close:** Medium. Replace the single `sync.Mutex` with `sync.RWMutex` and consider sharding sessions by repo or using a concurrent map.

### Gap 4: No Cross-Session File Collision Detection (ROADMAP 6.3)

When multiple sessions target the same repo without worktree isolation, concurrent edits to the same files are undetected. The `correlateTaskStatuses` function matches tasks by prompt substring but never checks for file-level conflicts.

**Effort to close:** Medium-Large. Requires: (a) tracking files modified per session (git diff), (b) collision detection window (like hcom's 30-second rule), (c) notification/pause mechanism.

### Gap 5: MaxConcurrentWorkers == 1 Hardcoded

The loop profile validation rejects `MaxConcurrentWorkers > 1`:
```go
if profile.MaxConcurrentWorkers != 1 {
    return profile, fmt.Errorf("max concurrent workers > 1 not implemented yet")
}
```

This blocks parallel worker execution in the perpetual loop, which is a Phase 6.1 target.

**Effort to close:** Medium. Requires parallel worker launch with worktree isolation per worker, result aggregation, and conflict resolution on merge-back.

---

## 4. External Landscape

### 4.1 smtg-ai/claude-squad -- Worktree Isolation Model

**Architecture:** Each agent instance gets a dedicated `GitWorktree` struct with its own branch and directory. Sessions are managed through tmux, with one pane per agent.

**Key patterns applicable to ralphglasses:**

1. **Worktree lifecycle tied to session state:**
   - `NewGitWorktree()` creates a fresh worktree with branch `{prefix}{sessionName}`.
   - `Pause()` commits changes, removes worktree, preserves branch.
   - `Resume()` recreates worktree from preserved branch.
   - `Kill()` cleans up both worktree and branch (unless `isExistingBranch`).

2. **Branch naming with collision avoidance:**
   - Path: `{configDir}/worktrees/{sanitized_branch_name}_{nanosecond_timestamp}`
   - Nanosecond timestamps prevent collisions when multiple sessions use the same branch prefix.

3. **Serializable worktree state:**
   - `ToInstanceData()` / `FromInstanceData()` persist worktree metadata for crash recovery.
   - Enables session survival across process restarts.

**Applicability:** The pause/resume pattern (commit + remove worktree + preserve branch) is directly applicable to ralphglasses' session lifecycle. The current `createLoopWorktree` function should be generalized to follow this pattern.

### 4.2 aannoo/hcom -- Multi-Agent Collision Detection

**Architecture:** A hook-based pub/sub system with SQLite as the message broker. Agents communicate through `agent -> hooks -> db -> hooks -> other agent`.

**Key patterns applicable to ralphglasses:**

1. **File collision detection with 30-second window:**
   - Hooks intercept file write events and store them with timestamps in SQLite.
   - If two agents edit the same file within 30 seconds, both receive collision notifications.
   - Configurable via `--collision` event subscription.

2. **Event subscription system:**
   - Agents subscribe to events: `hcom events sub "SQL WHERE"`.
   - Built-in presets: `collision`, `created`, `stopped`, `blocked`.
   - Auto-subscription at startup via `HCOM_AUTO_SUBSCRIBE` config.

3. **SQLite event schema:**
   - Events table with type classifications: `message`, `status`, `life`.
   - Message scoping: `broadcast`, `mentions`.
   - Status contexts: `tool:X`, `deliver:X`, `approval`, `prompt`, `exit:X`.
   - JSON arrays for multi-target delivery.

4. **Structured handoffs:**
   - Messages carry bundles: title, description, events, files, transcript ranges.
   - Intent types: `request` (expect response), `inform` (FYI), `ack` (reply).
   - Thread grouping for related coordination messages.

**Applicability:** The collision detection pattern (time-windowed file edit tracking) maps directly to ROADMAP 6.3 requirements. The SQLite event schema provides a model for ROADMAP 2.1.5 (session event log table). The subscription system aligns with the existing `events.Bus` but adds persistence and queryability.

### 4.3 dtormoen/tsk -- Container-Sandboxed Parallel Workers

**Architecture:** Docker/Podman containers provide filesystem, network, and resource isolation per task. A server daemon manages worker concurrency.

**Key patterns applicable to ralphglasses:**

1. **Worker pool with configurable concurrency:**
   - `tsk server start --workers 4` sets parallel capacity.
   - Tasks queue when all workers are busy.
   - Automatic cleanup of old tasks (configurable retention).

2. **Container lifecycle per task:**
   - Copy repo + create git branch -> start proxy -> build container -> execute -> save branch.
   - Each task produces a new branch in the original repo.
   - Network isolation via proxy sidecar with domain allowlists.

3. **Task dependency chains:**
   - `tsk add --parent <taskid>` creates sequential dependencies.
   - Child tasks "start from where parent left off" -- inheriting branch state.

4. **Resource limits per container:**
   - `memory_gb` and `cpu` settings per task.
   - Separate proxy containers for different network configurations.

**Applicability:** The worker pool pattern (configurable concurrency + queue) applies to lifting the `MaxConcurrentWorkers == 1` constraint. The task dependency chain pattern (`--parent`) maps to workflow step dependencies. Container isolation is a Phase 5 concern but the worker pool pattern is Phase 2.

### 4.4 mikeyobrien/ralph-orchestrator -- Hat System Personas

**Architecture:** Rust-based RPC orchestrator with a Hat System for specialized agent personas. MCP server integration with multiple backend CLI tools.

**Key patterns applicable to ralphglasses:**

1. **Built-in persona presets:**
   - Five builtins: `code-assist`, `debug`, `research`, `review`, `pdd-to-code-assist`.
   - Each persona has specific instructions and tool permissions.
   - Configuration via `ralph.yml` variants (`ralph.bot.yml`, `ralph.reviewer.yml`).

2. **Backpressure and quality gates:**
   - Rejects incomplete work via automated checks (tests, lint, typecheck).
   - Continues iterations until `LOOP_COMPLETE` or configured limits.

3. **Human-in-the-loop integration:**
   - Telegram integration for blocking agent questions until human response.
   - Event-driven coordination where personas emit and respond to events.

**Applicability:** The persona/preset pattern aligns with ralphglasses' `AgentDef` system but adds runtime behavior constraints (backpressure). The quality gate pattern (reject incomplete work) complements the loop's `VerifyCommands`.

---

## 5. Actionable Recommendations

### Recommendation 1: Extract `internal/worktree/` Package from Loop Code

**Target file:** `internal/worktree/worktree.go` (new), refactor `internal/session/loop.go`
**Effort:** Medium (3-5 days)
**Impact:** High -- unblocks ROADMAP 2.2, enables worktree isolation for all session types
**ROADMAP item:** 2.2.1, 2.2.2, 2.2.3, 2.2.4, 2.2.5

Extract `createLoopWorktree` and `gitTopLevel` from `loop.go` into a standalone `internal/worktree/` package. Generalize the interface:

```go
package worktree

type Worktree struct {
    SessionID    string
    RepoRoot     string
    WorktreePath string
    Branch       string
    IsExisting   bool // from claude-squad: controls cleanup behavior
    CreatedAt    time.Time
}

func Create(ctx context.Context, repoRoot, sessionID string, opts CreateOpts) (*Worktree, error)
func Remove(ctx context.Context, wt *Worktree) error
func MergeBack(ctx context.Context, wt *Worktree, targetBranch string) error
func List(ctx context.Context, repoRoot string) ([]*Worktree, error)
func Cleanup(ctx context.Context, repoRoot string, olderThan time.Duration) error
```

Adopt claude-squad patterns:
- **Branch naming:** `ralph/{session-id}` (already used in loops).
- **Path structure:** `.ralph/worktrees/{session-type}/{session-id}` with nanosecond suffix for collision avoidance.
- **Pause/Resume:** On pause, commit WIP + remove worktree + preserve branch. On resume, recreate worktree from branch.
- **Cleanup policy:** Remove worktree + optionally delete branch on session archive.

Wire into `Manager.Launch()`: when `opts.Worktree != ""`, create a worktree before spawning the CLI process, set `cmd.Dir = worktreePath`.

### Recommendation 2: Add File Collision Detection Layer

**Target file:** `internal/session/collision.go` (new)
**Effort:** Medium (2-3 days)
**Impact:** High -- addresses ROADMAP 6.3.4 (conflict resolution)
**ROADMAP item:** 6.3.2, 6.3.4

Adopt hcom's time-windowed collision detection pattern:

```go
package session

type FileEdit struct {
    SessionID string
    FilePath  string
    Timestamp time.Time
}

type CollisionDetector struct {
    mu      sync.Mutex
    window  time.Duration // default 30s, from hcom
    edits   []FileEdit
    bus     *events.Bus
}

func (d *CollisionDetector) RecordEdit(sessionID, filePath string)
func (d *CollisionDetector) Check(sessionID, filePath string) []FileEdit // returns conflicting edits
```

Integration points:
- Hook into session output parsing: when `runSessionOutput` detects file modification events (tool use results mentioning file paths), call `RecordEdit`.
- On collision detection, publish `events.FileCollision` event and optionally pause the later session.
- For worktree-isolated sessions, collision detection is less critical (conflicts surface at merge-back time), but for shared-directory sessions it is essential.

### Recommendation 3: Replace Single Mutex with RWMutex + Per-Map Granularity

**Target file:** `internal/session/manager.go`
**Effort:** Small (1-2 days)
**Impact:** Medium -- reduces contention at scale, prerequisite for 10+ sessions
**ROADMAP item:** 2.1 (prerequisite)

Current state: single `sync.Mutex` for all operations.

Proposed changes:
1. Replace `sync.Mutex` with `sync.RWMutex` for the manager. Read-heavy operations (`Get`, `List`, `FindByRepo`, `IsRunning`, `GetTeam`, `ListTeams`, `GetWorkflowRun`, `ListLoops`) use `RLock`. Write operations (`Launch`, `Stop`, `DelegateTask`) use full `Lock`.
2. Move `LoadExternalSessions` disk I/O outside the lock: read files first, then acquire lock only for map updates.
3. Consider splitting into per-map mutexes (`sessionsMu`, `teamsMu`, `workflowsMu`, `loopsMu`) if profiling shows cross-map contention.

Critical constraint: maintain lock ordering `Manager.mu` -> `Session.mu` to prevent deadlocks. The current `correlateTaskStatuses` acquires `Session.mu` while holding `Manager.mu` -- this is safe but must remain consistent.

### Recommendation 4: Implement SQLite Session Store

**Target file:** `internal/session/store.go` (new), `internal/session/migrations/` (new)
**Effort:** Large (5-8 days)
**Impact:** High -- core enabler for ROADMAP 2.1, crash recovery, queryable history
**ROADMAP item:** 2.1.2, 2.1.3, 2.1.4, 2.1.5

Schema design (informed by hcom's event schema):

```sql
-- Core session table
CREATE TABLE sessions (
    id            TEXT PRIMARY KEY,
    provider      TEXT NOT NULL,
    provider_sid  TEXT,
    repo_path     TEXT NOT NULL,
    repo_name     TEXT NOT NULL,
    status        TEXT NOT NULL CHECK(status IN ('launching','running','completed','stopped','errored')),
    prompt        TEXT,
    model         TEXT,
    agent_name    TEXT,
    team_name     TEXT,
    worktree_path TEXT,
    branch        TEXT,
    budget_usd    REAL DEFAULT 0,
    spent_usd     REAL DEFAULT 0,
    turn_count    INTEGER DEFAULT 0,
    max_turns     INTEGER DEFAULT 0,
    launched_at   TEXT NOT NULL,
    last_activity TEXT NOT NULL,
    ended_at      TEXT,
    exit_reason   TEXT,
    last_output   TEXT,
    error         TEXT,
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    updated_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

-- State transition log (from hcom's event model)
CREATE TABLE session_events (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    event_type TEXT NOT NULL,
    old_status TEXT,
    new_status TEXT,
    data       TEXT, -- JSON
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

-- File edit tracking (from hcom's collision detection)
CREATE TABLE file_edits (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT NOT NULL REFERENCES sessions(id),
    file_path  TEXT NOT NULL,
    edit_type  TEXT, -- create, modify, delete
    created_at TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_sessions_status ON sessions(status);
CREATE INDEX idx_sessions_repo ON sessions(repo_path);
CREATE INDEX idx_session_events_session ON session_events(session_id);
CREATE INDEX idx_file_edits_session ON file_edits(session_id);
CREATE INDEX idx_file_edits_path_time ON file_edits(file_path, created_at);
```

Use `modernc.org/sqlite` (pure Go, no CGO) as specified in ROADMAP 2.1.2. WAL mode for concurrent readers (TUI + MCP server). The existing internal SQLite project repo in the same org already uses this library -- reuse patterns from there.

Migration strategy:
- `internal/session/migrations/001_initial.sql` with the schema above.
- `Store.Migrate()` called on startup.
- Dual-write during transition: persist to both JSON files and SQLite, read from SQLite.
- Remove JSON persistence after one release cycle.

### Recommendation 5: Lift MaxConcurrentWorkers Constraint in Loop

**Target file:** `internal/session/loop.go`
**Effort:** Medium (3-4 days)
**Impact:** Medium -- enables parallel workers in perpetual loop, prerequisite for ROADMAP 6.1
**ROADMAP item:** 6.1.4

Current blocker (loop.go:373-375):
```go
if profile.MaxConcurrentWorkers != 1 {
    return profile, fmt.Errorf("max concurrent workers > 1 not implemented yet")
}
```

Implementation plan:
1. Remove the `!= 1` guard.
2. In `StepLoop`, after the planner produces tasks, use a semaphore-gated worker pool (from tsk's pattern):
   ```go
   sem := make(chan struct{}, profile.MaxConcurrentWorkers)
   var wg sync.WaitGroup
   for _, task := range tasks {
       sem <- struct{}{} // acquire slot
       wg.Add(1)
       go func(task LoopTask) {
           defer wg.Done()
           defer func() { <-sem }() // release slot
           // create worktree, launch worker, wait, verify
       }(task)
   }
   wg.Wait()
   ```
3. Each parallel worker gets its own worktree (using Recommendation 1's `worktree.Create`).
4. After all workers complete, merge results back sequentially with conflict detection.
5. Planner must produce multiple tasks per iteration (currently produces exactly one). Extend the planner prompt to request a JSON array of tasks when `MaxConcurrentWorkers > 1`.

### Recommendation 6: Structured Task Assignment for Teams

**Target file:** `internal/session/manager.go`, `internal/session/types.go`
**Effort:** Small (1-2 days)
**Impact:** Medium -- fixes fragile string-matching correlation in `correlateTaskStatuses`
**ROADMAP item:** 2.5.3 (extension)

Current problem: `correlateTaskStatuses` matches workers to tasks via `strings.Contains(w.Prompt, task.Description)`. This breaks when the lead agent paraphrases the task.

Solution:
1. Add `TaskID string` field to `TeamTask` and `LaunchOptions`.
2. When the lead launches a worker via MCP, include a `task_id` parameter that gets stored on the `Session`.
3. Replace string matching with `w.TaskID == task.ID` in `correlateTaskStatuses`.
4. Generate task IDs deterministically: `{team_name}-{task_index}`.

### Recommendation 7: Add Session Lifecycle State Machine

**Target file:** `internal/session/types.go`, `internal/session/manager.go`
**Effort:** Small (1 day)
**Impact:** Medium -- prevents invalid state transitions, enables ROADMAP 2.1.4
**ROADMAP item:** 2.1.4

Current state: status is set ad-hoc in `runner.go` and `manager.go` without transition validation.

Add a transition table:

```go
var validTransitions = map[SessionStatus][]SessionStatus{
    StatusLaunching: {StatusRunning, StatusErrored, StatusStopped},
    StatusRunning:   {StatusCompleted, StatusErrored, StatusStopped},
    StatusCompleted: {}, // terminal
    StatusStopped:   {StatusLaunching}, // resume
    StatusErrored:   {StatusLaunching}, // retry
}

func (s *Session) TransitionTo(next SessionStatus) error {
    allowed := validTransitions[s.Status]
    for _, a := range allowed {
        if a == next { s.Status = next; return nil }
    }
    return fmt.Errorf("invalid transition: %s -> %s", s.Status, next)
}
```

---

## 6. Risk Assessment

### Risk 1: SQLite Migration Data Loss (HIGH)

**Description:** Transitioning from JSON files to SQLite requires a dual-write period. If the migration has bugs, sessions could be lost or duplicated.

**Mitigation:**
- Implement dual-write (JSON + SQLite) before cutting over.
- Add a `ralphglasses migrate` subcommand that reads existing JSON files and inserts into SQLite.
- Keep JSON files as backup for one release cycle.
- The internal SQLite project repo in the same org already uses `modernc.org/sqlite` -- reuse its connection setup patterns.

### Risk 2: Worktree Accumulation Disk Exhaustion (MEDIUM)

**Description:** Each loop iteration creates a worktree (~full repo copy). With parallel workers and long-running loops, disk space can be exhausted.

**Mitigation:**
- Implement `worktree.Cleanup` with configurable retention (default: 24 hours for completed, immediate for failed).
- Add disk space check before worktree creation (from tsk's pattern).
- Monitor `.ralph/worktrees/` size in health checks.
- Consider shallow worktrees (`git worktree add --detach`) for read-heavy verification tasks.

### Risk 3: Lock Ordering Violation in Parallel Workers (MEDIUM)

**Description:** When `MaxConcurrentWorkers > 1`, multiple goroutines will call `Manager.Launch` and `waitForSession` concurrently. If lock ordering is violated (e.g., a new code path acquires `Session.mu` before `Manager.mu`), deadlocks will occur.

**Mitigation:**
- Document lock ordering: `Manager.mu` -> `Session.mu` -> `LoopRun.mu` -> `WorkflowRun.mu`.
- Add `go test -race` to CI (ROADMAP 0.5.9.2 already calls for this).
- Consider replacing `Session.mu` with atomic operations for frequently-read fields (Status, SpentUSD, TurnCount).

### Risk 4: Collision Detection False Positives (LOW)

**Description:** Time-windowed collision detection (from hcom) may fire false positives for files that are legitimately edited by multiple agents (e.g., shared config files, go.sum).

**Mitigation:**
- Add an ignore list in `.ralphrc`: `COLLISION_IGNORE=go.sum,go.mod,package-lock.json`.
- Allow collision events to be acknowledged/dismissed without pausing.
- Only enforce hard pauses for non-generated source files.

### Risk 5: Planner Multi-Task Output Parsing Fragility (LOW)

**Description:** Lifting the single-task constraint requires the planner to output a JSON array of tasks. LLM output parsing is already fragile (`parsePlannerTask` has multiple fallback strategies).

**Mitigation:**
- Extend `parsePlannerTask` to handle both `{title, prompt}` and `[{title, prompt}, ...]` formats.
- Keep the single-task fallback: if the planner returns one task and `MaxConcurrentWorkers > 1`, run it with a single worker.
- Add structured output schema constraints to the planner prompt.

---

## 7. Implementation Priority Ordering

| Priority | Recommendation | ROADMAP ID | Effort | Impact | Dependencies |
|----------|---------------|-----------|--------|--------|--------------|
| **P0** | R3: RWMutex + granular locking | 2.1 prereq | Small | Medium | None |
| **P0** | R7: Session lifecycle state machine | 2.1.4 | Small | Medium | None |
| **P1** | R1: Extract `internal/worktree/` package | 2.2.1-2.2.5 | Medium | High | None |
| **P1** | R6: Structured task assignment for teams | 2.5.3 ext | Small | Medium | None |
| **P2** | R4: SQLite session store | 2.1.2-2.1.5 | Large | High | R7 (state machine) |
| **P2** | R2: File collision detection | 6.3.2, 6.3.4 | Medium | High | R1 (worktrees reduce need) |
| **P3** | R5: Parallel workers in loop | 6.1.4 | Medium | Medium | R1 (worktree isolation) |

**Suggested implementation order:**
1. **Sprint 1 (P0):** R3 (RWMutex) + R7 (state machine) -- foundational improvements, low risk, can be done in parallel.
2. **Sprint 2 (P1):** R1 (worktree package) + R6 (task assignment) -- generalize existing patterns, unblock session isolation.
3. **Sprint 3 (P2):** R4 (SQLite store) -- major persistence overhaul, requires R7 for state machine enforcement.
4. **Sprint 4 (P2):** R2 (collision detection) -- important but partially mitigated by worktree isolation from R1.
5. **Sprint 5 (P3):** R5 (parallel workers) -- depends on R1 for worktree isolation per worker.

---

## Appendix A: External Project Reference Summary

| Project | Language | Key Pattern | Relevance to ralphglasses |
|---------|----------|-------------|--------------------------|
| [smtg-ai/claude-squad](https://github.com/smtg-ai/claude-squad) | Go | Worktree per session, pause/resume via commit+remove, nanosecond path suffixes | Direct model for `internal/worktree/` (R1) |
| [aannoo/hcom](https://github.com/aannoo/hcom) | Python | 30s collision window, SQLite event schema, hook-based pub/sub, structured handoffs | Direct model for collision detection (R2), SQLite schema (R4) |
| [dtormoen/tsk](https://github.com/dtormoen/tsk) | Go | Container sandbox, semaphore worker pool, task dependency chains, auto-cleanup | Worker pool pattern for parallel loop workers (R5) |
| [mikeyobrien/ralph-orchestrator](https://github.com/mikeyobrien/ralph-orchestrator) | Rust | Hat System personas, backpressure quality gates, event-driven coordination | Persona pattern enhances AgentDef system |

## Appendix B: Key Source Files Referenced

| File | Role |
|------|------|
| `internal/session/manager.go` | Session/team/workflow/loop lifecycle management |
| `internal/session/runner.go` | Session launch, streaming output parsing, process lifecycle |
| `internal/session/types.go` | Session, LaunchOptions, TeamConfig, TeamStatus, AgentDef types |
| `internal/session/loop.go` | Perpetual planner/worker/verifier loop with worktree creation |
| `internal/session/workflow.go` | Multi-step workflow DAG execution |
| `internal/session/agents.go` | Agent definition discovery (Claude, Gemini, Codex) |
| `internal/session/templates.go` | Provider-specific prompt templates |
| `internal/session/failover.go` | Provider failover chain |
| `internal/session/checkpoint.go` | Git checkpoint (commit + tag) |
| `internal/session/budget.go` | Budget enforcement, cost ledger |
| `internal/session/ratelimit.go` | Per-provider rate limiting |
| `internal/session/health.go` | Provider health checks |
| `internal/session/gitinfo.go` | Git log/diff queries |
