# Research Phase 4: Roadmap Planning & Backlog Resolution

**Date:** 2026-03-22
**Scope:** Cross-cutting analysis of roadmap methodology, dependency graph accuracy, backlog prioritization, task management automation
**Maps to:** All ROADMAP.md phases (0-8, 3.5)

---

## 1. Executive Summary

The ralphglasses ROADMAP.md contains 520 checkbox items (76 completed, 444 remaining) spanning 15 phases from foundation to AI-native features. This analysis reveals three systemic issues: (1) at least 5 Phase 0.5 items are already implemented in code but not checked off, creating a stale backlog that misleads dependency resolution; (2) the dependency graph has 7 missing edges where implicit prerequisites are not declared; and (3) the `internal/roadmap/` parser handles inline `[BLOCKED BY]` only on task lines, missing section-level dependency annotations that govern ~60% of the blocking relationships. The project would benefit from adopting structured task metadata (a la claude-task-master), automated staleness detection via the existing `roadmap_analyze` MCP tool, and a DAG validation pass to surface circular or missing edges before each planning cycle.

---

## 2. Current State Analysis

### 2.1 Roadmap Scale and Structure

| Metric | Value |
|--------|-------|
| Total checkbox items | 520 |
| Completed (`[x]`) | 76 (14.6%) |
| Remaining (`[ ]`) | 444 (85.4%) |
| Phases | 15 (0, 0.5, 1, 1.5, 2, 2.5, 2.75, 3, 3.5, 4, 5, 6, 7, 8) |
| Sections (### headings) | ~65 |
| Explicit `[BLOCKED BY]` annotations | 26 (on section headings and one task-level) |
| Item-level dependency declarations (bottom section) | 18 edges |

### 2.2 Phase Completion Status

| Phase | Status | Items | Done | Completion |
|-------|--------|-------|------|------------|
| 0: Foundation | COMPLETE | 13 | 13 | 100% |
| 0.5: Critical Fixes | In Progress | 44 | 0 | 0% (misleading -- see 2.3) |
| 1: Harden & Test | In Progress | 38 | 6 | 15.8% |
| 1.5: Developer Experience | Not Started | 37 | 0 | 0% |
| 2: Multi-Session | Not Started | 64 | 0 | 0% |
| 2.5: Multi-LLM | In Progress | 21 | 17 | 81% |
| 2.75: Architecture Extensions | COMPLETE | 25 | 25 | 100% |
| 3: i3 Multi-Monitor | Not Started | 24 | 0 | 0% |
| 3.5: Theme & Plugin | Not Started | 26 | 0 | 0% |
| 4: Bootable Thin Client | In Progress | 43 | 4 | 9.3% |
| 5: Sandboxing | Not Started | 40 | 0 | 0% |
| 6: Fleet Intelligence | Not Started | 50 | 0 | 0% |
| 7: Kubernetes | Not Started | 25 | 0 | 0% |
| 8: AI-Native | Not Started | 30 | 0 | 0% |

### 2.3 Stale Items: Implemented But Not Checked Off

Codebase inspection reveals that several Phase 0.5 items have already been implemented but remain unchecked in ROADMAP.md:

**0.5.1 -- Silent error suppression in RefreshRepo:**
- 0.5.1.1: `RefreshRepo()` in `internal/model/status.go:53-79` already returns `[]error` (not `_ =` suppression). The function collects errors into a slice and assigns them to `r.RefreshErrors`.
- 0.5.1.2: `RefreshErrorMsg` is defined in `internal/tui/app.go:53-54` and handled at line 245. Errors are propagated to the TUI.
- 0.5.1.3: `internal/tui/views/repodetail.go:142-145` displays refresh errors in the repo detail view.
- 0.5.1.4: Test exists in `internal/model/status_test.go:280-282` validating that `RefreshErrors` are stored on the repo.

**0.5.2 -- Watcher error handling:**
- 0.5.2.1: `internal/process/watcher.go` returns `WatcherErrorMsg` on errors (lines 17-71), not `return nil`.
- 0.5.2.2: `WatcherErrorMsg` is emitted and handled in `internal/tui/app.go:268`.
- 0.5.2.3: Idle timeout fallback implemented at `watcher.go:74` ("watcher idle timeout: falling back to polling").
- Tests exist in `internal/process/watcher_test.go` covering timeout, closed watcher, and failed watch scenarios.

**0.5.3 -- Process reaper exit status:**
- `ProcessErrorMsg` defined in `internal/process/manager.go:18`, exit code parsed and delivered at line 207.
- Tests in `manager_test.go:435` verify clean exit vs crash detection.

**0.5.7 -- Hardcoded version string (partially):**
- `cmd/root.go:23` defines `var version = "dev"` (in `cmd` package, not `internal/version`).
- `cmd/root.go:39` uses it for Cobra `Version` field.
- `cmd/root.go:110-111` prints version with commit and build date.
- Tests in `cmd/cmd_test.go` verify ldflags injection.
- However: `cmd/ralphglasses-mcp/main.go:24` still has hardcoded `"0.1.0"`.

**0.5.9 -- Race condition in MCP scan (partially):**
- `sync.RWMutex` added to `internal/mcpserver/tools.go:31`.
- But `go test -race` is not yet in the CI pipeline YAML (only in Makefile/docs).

**Impact:** The 0.5.1 and 0.5.2 subsections (8 items total) being unchecked makes Phase 1.8 (Custom error types, `[BLOCKED BY 0.5.1]`) appear blocked when its prerequisite is actually met. This cascading staleness affects dependency chain calculations downstream.

### 2.4 The `internal/roadmap/` Parser

The parser (`internal/roadmap/parse.go`) uses five regexes to extract structure:

| Regex | Purpose | Edge Cases |
|-------|---------|------------|
| `phaseRe` | `## Phase ...` headings | Works, but captures all `##` headings including non-phase sections like "External Projects of Interest" and "Internal Ecosystem Integration" |
| `sectionRe` | `### ...` headings | Works correctly |
| `taskRe` | `- [x]` / `- [ ]` checkboxes | Works for standard format |
| `taskIDRe` | `ID -- description` extraction | Uses `\S+` for ID, which correctly matches dotted IDs like `0.5.1.1` |
| `blockedByRe` | `[BLOCKED BY ...]` | Only matches on task description text, **not on section headings** |

**Critical gap:** The majority of `[BLOCKED BY]` annotations in ROADMAP.md are on `###` section headings (e.g., `### 2.2 -- Git worktree orchestration [BLOCKED BY 2.1]`), not on individual `- [ ]` task lines. The parser extracts section names but does not parse `[BLOCKED BY]` from them. This means:
- Section-level blocked-by relationships (governing ~150 tasks across 22 sections) are invisible to the analyzer
- The `Analyze()` function's dependency checking only works for the 1 task-level `[BLOCKED BY]` annotation (`1.4.1`)
- The `Export()` function with `respectDeps=true` will incorrectly mark section-blocked tasks as "ready"

**Additional parser issues:**
1. Non-phase `##` headings ("External Projects of Interest", "Internal Ecosystem Integration", "Dependency Chain") are captured as phases, inflating phase count and including non-task content in analysis
2. The `findEvidence()` function uses only filesystem existence checks (does the path exist?), not content-level searching. This misses cases where a feature is implemented in an existing file rather than a new directory
3. `extractKeywords()` only finds backtick-quoted identifiers and `internal/`/`cmd/`/`distro/`/`scripts/` prefixes. It misses Go package names, function names, and struct names mentioned in task descriptions
4. The orphan detection (`findOrphaned()`) only checks `internal/` subdirectories, missing orphaned code in `cmd/`, `distro/`, `scripts/`, or top-level files

---

## 3. Gap Analysis

### 3.1 Missing Dependency Edges

The following implicit dependencies are not declared anywhere in ROADMAP.md (neither in `[BLOCKED BY]` annotations nor in the "Item-Level Dependencies" section at the bottom):

| From | To | Reason |
|------|----|--------|
| 0.5.9 (race condition fix) | 1.2.1 (MCP mutex audit) | 1.2.1 says "add sync.RWMutex around repos map" -- but 0.5.9.1 already does this. Either 1.2.1 is stale or it covers additional shared state beyond what 0.5.9 addresses |
| 0.5.11 (config validation) | 1.5.4 (config schema docs) | Config schema documentation depends on having a canonical key list, which 0.5.11.1 defines |
| 1.7 (structured logging) | 2.10 (marathon Go port) | 2.10.5 specifies "structured marathon logging via slog" which depends on slog being established in the codebase first |
| 2.1 (session model) | 2.12 (telemetry) | Telemetry events reference session lifecycle (session_start, session_stop) which requires the session data model |
| 2.6 (notifications) | 2.13 (plugin system) | 2.13.3 explicitly says "extract from 2.6 as reference implementation" -- this is a hard dependency, not parallel |
| 2.75.2 (event bus) | 2.6 (notifications) | The notification system should consume events from the event bus, but 2.6 is listed as depending only on 2.1 |
| 4.1 (ISO pipeline) | 4.8 (marathon hardening) | 4.8 is marked `[PARALLEL]` but items 4.8.2 and 4.8.4 reference `/proc/meminfo` and `bc` which are Linux-specific and implicitly depend on the distro environment |

### 3.2 Phase 0.5 Items Silently Blocking Phase 1/2

Despite the ROADMAP declaring "All 0.5.x items are independent," several create implicit prerequisites:

1. **0.5.9 (race condition) silently blocks 1.2 (MCP hardening):** Item 1.2.1 says "audit all shared state in mcpserver; add sync.RWMutex." But 0.5.9.1 already added the mutex. If 0.5.9 is not completed (and checked off), a developer working on 1.2.1 will duplicate work or create conflicts.

2. **0.5.11 (config validation) silently blocks 1.5.4 (config schema):** The config schema documentation (1.5.4.1) references "all keys, types, defaults" which 0.5.11.1 defines as `config_schema.go`. Working on 1.5.4 without 0.5.11 means documenting an incomplete schema.

3. **0.5.7 (version string) explicitly blocks 1.5.2 (release automation):** This is correctly declared but 0.5.7 is partially done (version variable exists in `cmd/root.go` but not in `internal/version/version.go` as specified). The standalone MCP binary at `cmd/ralphglasses-mcp/main.go:24` still has `"0.1.0"`.

4. **0.5.10 (marathon edge cases) overlaps with 4.8 (marathon hardening):** Items 0.5.10.1-0.5.10.5 and 4.8.1-4.8.5 have significant overlap (disk space checks, memory monitoring, restart logic, bc check). Neither declares the other as a dependency, risking duplicate implementations.

5. **0.5.1 (RefreshRepo errors) already done but not checked:** This blocks 1.8 (custom error types) which says `[BLOCKED BY 0.5.1]`. Since 0.5.1 is implemented, 1.8 is actually unblocked -- but the roadmap shows it as blocked.

### 3.3 Analyzer Accuracy Issues

The `Analyze()` function in `internal/roadmap/analyze.go` has correctness gaps:

1. **Ready task calculation is redundant:** Lines 101-116 have two branches (`depsReady && len(task.DependsOn) > 0` and `depsReady && len(task.DependsOn) == 0`) that both append to `a.Ready`. The second condition means every task with no declared dependencies is marked "ready" -- including tasks under sections with section-level `[BLOCKED BY]` annotations that the parser ignores.

2. **Evidence detection is shallow:** `findEvidence()` only checks if filesystem paths exist. For example, task "Add `sync.RWMutex` to protect `repos` map" would need content-level grep to detect the mutex was added, not just that `internal/mcpserver/` exists.

3. **No phase-level dependency propagation:** The analyzer checks individual task `DependsOn` fields but does not propagate phase-level dependencies ("Phase 2 depends on Phase 1"). A Phase 2 task with no individual `DependsOn` appears ready even when Phase 1 is incomplete.

---

## 4. External Landscape

### 4.1 claude-task-master (eyaltoledano/claude-task-master)

**Stars:** 15K+ | **Language:** Node.js | **Relevance:** High

claude-task-master decomposes PRDs into structured task lists with dependency DAGs. Key patterns applicable to ralphglasses:

- **Structured task metadata:** Each task carries ID, title, status, dependencies (comma-separated IDs), priority, description, details, and test strategy as first-class fields. The ralphglasses ROADMAP.md encodes this information in free-form markdown with regex-extracted IDs.
- **Dependency validation:** `validate-dependencies` command detects circular chains and dangling references. The ralphglasses parser has no validation pass.
- **`next_task` resolution:** Returns the highest-priority task with all dependencies satisfied. The ralphglasses `Export()` with `respectDeps=true` provides similar functionality but only for task-level (not section-level) dependencies.
- **Tagged task lists (v0.16.2+):** Multiple isolated task contexts within one project. Maps to ralphglasses's multi-phase structure.
- **Autopilot TDD mode (v0.30.0+):** `tm autopilot` runs generate-test/implement/verify/commit loops. Directly relevant to Phase 6.2 (R&D cycle orchestrator).

**Transferable patterns:** Dependency validation, priority scoring, task metadata schema, autopilot loop.

### 4.2 ccpm (automazeio/ccpm)

**Stars:** 100+ | **Language:** Mixed (Bash/Markdown skills) | **Relevance:** High

ccpm provides a PRD-to-Issue pipeline for Claude Code projects using GitHub Issues as the backing store:

- **Full traceability chain:** PRD --> Epic --> Task --> Issue --> Code --> Commit. Each artifact links to its parent.
- **Deterministic operations:** Status, standup, search, and validation run as bash scripts (no LLM token cost). Only creative work (PRD authoring, task breakdown) uses the LLM.
- **GitHub Issues as database:** Enables multiple Claude instances to work on the same project simultaneously, with `parallel: true` flags for conflict-free concurrent development.
- **Guided brainstorming:** Before writing a PRD, ccpm asks about the problem, users, success criteria, constraints, and scope exclusions.

**Transferable patterns:** The PRD-to-Issue pipeline could replace the current ROADMAP.md flat-file approach for active work. The deterministic/LLM split maps to ralph's planner/worker distinction. GitHub Issues integration would give the roadmap a queryable, linkable backing store.

### 4.3 Kubernetes KEP Process

**Stars:** N/A (ecosystem process) | **Relevance:** Medium

Kubernetes manages thousands of enhancements across hundreds of contributors using KEPs (Kubernetes Enhancement Proposals):

- **One-KEP-per-feature:** Each major feature gets a structured document (motivation, proposal, alternatives, test plan) that lives in the git repo. Maps to ralphglasses sections.
- **SIG ownership:** Each KEP is owned by a SIG (Special Interest Group). Maps to the concept of phase ownership.
- **Git-based receipts process:** SIGs opt-in to releases using a specified file format. All tracking occurs via git commits rather than issue comments. Reduces spreadsheet/manual tracking overhead.
- **Lifecycle states:** `provisional --> implementable --> implemented --> deferred/withdrawn`. More nuanced than the binary `[ ]`/`[x]` checkboxes.

**Transferable patterns:** KEP-like structured documents per major feature, lifecycle states beyond done/not-done, git-based tracking artifacts.

### 4.4 Linear.app Patterns

**Relevance:** Medium

Linear's roadmap management offers patterns applicable to tool-assisted planning:

- **Quarterly planning alignment:** Teams align on what ships per quarter. The ralphglasses roadmap has no time-based milestones.
- **Automatic status transitions:** Issues move from "In Progress" to "Done" based on PR merge status. Could wire into ralph's session lifecycle events.
- **MCP server integration:** Linear's MCP server supports initiatives, milestones, and updates -- enabling product managers to update plans from within Claude Code or Cursor.

### 4.5 DAG Analysis Tooling

Tools like Apache Airflow, Prefect, and dbt demonstrate mature DAG patterns:

- **Topological sort for execution order:** Standard algorithm for determining which tasks can run in parallel vs. sequentially.
- **Cycle detection:** Tarjan's algorithm or simple DFS-based detection to catch circular dependencies at parse time.
- **Critical path analysis:** Identify the longest chain of dependent tasks to predict total project duration.
- **Visualization:** Graphviz DOT export or ASCII DAG rendering for terminal display.

---

## 5. Actionable Recommendations

### Recommendation 1: Mark Implemented Phase 0.5 Items as Complete

**Target files:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/ROADMAP.md` (lines 41-68, 83-88)

**Effort:** Small (30 min)
**Impact:** High -- unblocks dependency chain for Phase 1.8 and corrects analyzer output
**ROADMAP items:** 0.5.1.1, 0.5.1.2, 0.5.1.3, 0.5.1.4, 0.5.2.1, 0.5.2.2, 0.5.2.3, 0.5.3.1, 0.5.3.2, 0.5.3.3, 0.5.3.4

**Action:** Review each implemented item against its acceptance criteria, and change `[ ]` to `[x]` for verified items. For 0.5.7, check off the subtasks that are done (0.5.7.1-equivalent in `cmd/root.go`, 0.5.7.2 for cmd/root.go usage) but keep open 0.5.7.2 for `cmd/ralphglasses-mcp/main.go:24` (still hardcoded `"0.1.0"`). For 0.5.9, check off 0.5.9.1 (mutex added) but keep 0.5.9.2 (CI pipeline) and 0.5.9.3 (concurrent test) open.

### Recommendation 2: Add Section-Level Dependency Parsing to `internal/roadmap/parse.go`

**Target file:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/parse.go` (lines 96-102)

**Effort:** Medium (2-4 hours)
**Impact:** High -- fixes ~150 tasks incorrectly marked as "ready" by the analyzer
**ROADMAP items:** All items under sections with `[BLOCKED BY]` annotations

**Action:** When parsing `### ...` section headings, apply `blockedByRe` to the section name. Store the extracted dependencies on the `Section` struct as a new `DependsOn []string` field. In `analyze.go`, propagate section-level dependencies to all tasks within that section (a task inherits its section's dependencies unless it declares its own). Update the `Section` struct:

```go
type Section struct {
    Name       string   `json:"name"`
    Tasks      []Task   `json:"tasks"`
    Acceptance string   `json:"acceptance,omitempty"`
    DependsOn  []string `json:"depends_on,omitempty"` // NEW
}
```

Then in `Analyze()`, when checking if a task's deps are ready, also check the enclosing section's `DependsOn`. Add corresponding test cases in `parse_test.go` for section-level `[BLOCKED BY]`.

### Recommendation 3: Add Phase Filtering to the Parser to Exclude Non-Phase Sections

**Target file:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/parse.go` (lines 80-93)

**Effort:** Small (1 hour)
**Impact:** Medium -- prevents "External Projects of Interest" and "Dependency Chain" from being treated as phases
**ROADMAP items:** Relates to roadmap_parse and roadmap_analyze tool accuracy

**Action:** Add a heuristic to `phaseRe` matching: only treat `##` headings as phases if they match a pattern like `Phase \d` or contain known phase-indicating words. Alternatively, stop parsing phases when a `---` horizontal rule is encountered (the ROADMAP uses `---` separators before the non-phase sections). The simplest fix:

```go
// Stop treating ## headings as phases after the dependency chain section
if trimmed == "---" && curPhase != nil {
    // Flush current phase and stop phase parsing
    // (or set a flag to skip subsequent ## headings)
}
```

### Recommendation 4: Add DAG Validation and Missing-Edge Detection

**Target files:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/analyze.go` (new function)
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/analyze_test.go` (new tests)

**Effort:** Medium (3-5 hours)
**Impact:** High -- catches missing edges and circular dependencies before they cause planning errors
**ROADMAP items:** Supports all phases; directly relevant to roadmap_analyze MCP tool quality

**Action:** Add a `ValidateDependencyGraph()` function that:

1. **Builds the full DAG** from all task and section `DependsOn` fields plus the "Item-Level Dependencies" block (if parseable -- currently free-form text)
2. **Detects cycles** using DFS with coloring (white/gray/black)
3. **Detects dangling references** -- dependencies that reference non-existent task IDs
4. **Detects shadowed dependencies** -- a task declares `[BLOCKED BY X]` but X is a section ID, not a task ID (common format mismatch)
5. **Suggests missing edges** using heuristics:
   - If task A mentions "build on" or "extends" in description and names task B, suggest `A depends on B`
   - If two tasks reference the same file path, flag potential ordering concern

Wire this into `roadmap_analyze` MCP tool output as a new `Validation` field.

### Recommendation 5: Deduplicate Marathon Items Between Phase 0.5.10 and Phase 4.8

**Target file:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/ROADMAP.md` (lines 103-109 and 614-620)

**Effort:** Small (30 min)
**Impact:** Medium -- eliminates confusion about which phase owns marathon improvements
**ROADMAP items:** 0.5.10.1-0.5.10.5 and 4.8.1-4.8.5

**Action:** Merge overlapping items. The pattern should be:
- **0.5.10** (Phase 0.5): Keep only items that are true pre-requisites for current functionality -- `bc` check (0.5.10.1), infinite restart fix (0.5.10.3). Remove 0.5.10.2 (disk space) and 0.5.10.4 (memory monitoring) which duplicate 4.8.1 and 4.8.2.
- **4.8** (Phase 4): Keep the full hardening scope, add explicit `[DEPENDS ON 0.5.10]` to acknowledge the pre-requisite relationship.
- Add `0.5.10.5` (log rotation) to 4.8 since it's a hardening concern, not a critical fix.

### Recommendation 6: Add Content-Level Evidence Detection to `findEvidence()`

**Target file:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/analyze.go` (lines 150-166)

**Effort:** Medium (2-3 hours)
**Impact:** Medium -- dramatically improves staleness detection accuracy
**ROADMAP items:** Improves roadmap_analyze output for all phases

**Action:** Extend `findEvidence()` beyond `os.Stat()` checks. Add content-level searching:

1. Extract Go identifiers from task descriptions (function names like `RefreshRepo`, type names like `WatcherErrorMsg`, variable names like `sync.RWMutex`)
2. Use `grep`-style search (or `go/parser` for Go-specific AST analysis) to check if those identifiers exist in the codebase
3. Weight evidence: file existence = 0.3, identifier match = 0.7, test file match = 0.9

This would have caught all the Phase 0.5 stale items automatically.

### Recommendation 7: Add Phase-Level Dependency Declaration Syntax

**Target file:**
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/ROADMAP.md` (blockquote dependency annotations)
- `/Users/mitchnotmitchell/hairglasses-studio/ralphglasses/internal/roadmap/parse.go` (new regex)

**Effort:** Medium (2-3 hours)
**Impact:** Medium -- makes phase dependencies machine-parseable instead of just human-readable
**ROADMAP items:** All phases with `> **Depends on:**` blockquotes

**Action:** The ROADMAP already has `> **Depends on:** Phase X` annotations on most phases. Add a regex to the parser to extract these:

```go
var phaseDependsRe = regexp.MustCompile(`>\s*\*\*Depends on:\*\*\s*(.+)`)
```

Store as a new `DependsOn []string` field on the `Phase` struct. Propagate to all tasks within the phase for dependency resolution.

---

## 6. Risk Assessment

### High Risk

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Stale roadmap items cause developers to re-implement existing features | High | High | Recommendation 1 (mark implemented items) + Recommendation 6 (content-level evidence) |
| Section-level dependencies invisible to tooling leads to incorrect "ready" task lists | High | High | Recommendation 2 (section dependency parsing) |
| ROADMAP.md grows beyond maintainability (already 1064 lines, 520 items) | Medium | High | Consider migrating active work to GitHub Issues (ccpm pattern) while keeping ROADMAP.md as the architectural vision document |

### Medium Risk

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Duplicate marathon items lead to conflicting implementations | Medium | Medium | Recommendation 5 (dedup 0.5.10 vs 4.8) |
| Non-phase sections pollute analysis results | Medium | Low | Recommendation 3 (phase filtering) |
| No validation catches broken dependency references | Medium | Medium | Recommendation 4 (DAG validation) |

### Low Risk

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Parser regex breaks on unusual markdown formatting | Low | Low | Existing test suite covers standard cases; add edge case tests for nested lists, HTML comments, etc. |
| External tool patterns (claude-task-master, ccpm) don't translate to Go codebase | Low | Low | Selective adoption of patterns (data model, validation) rather than wholesale tool integration |

---

## 7. Implementation Priority Ordering

| Priority | Recommendation | Effort | Impact | ROADMAP IDs | Target Path |
|----------|---------------|--------|--------|-------------|-------------|
| **P0** | 1. Mark implemented 0.5 items as complete | Small (30 min) | High | 0.5.1.*, 0.5.2.*, 0.5.3.*, 0.5.7.*, 0.5.9.1 | `ROADMAP.md` |
| **P1** | 2. Section-level dependency parsing | Medium (2-4 hr) | High | Cross-cutting (all blocked sections) | `internal/roadmap/parse.go`, `internal/roadmap/analyze.go` |
| **P2** | 4. DAG validation and missing-edge detection | Medium (3-5 hr) | High | Cross-cutting | `internal/roadmap/analyze.go` |
| **P3** | 5. Deduplicate marathon items | Small (30 min) | Medium | 0.5.10.*, 4.8.* | `ROADMAP.md` |
| **P4** | 3. Phase filtering for non-phase sections | Small (1 hr) | Medium | roadmap_parse tool | `internal/roadmap/parse.go` |
| **P5** | 6. Content-level evidence detection | Medium (2-3 hr) | Medium | roadmap_analyze tool | `internal/roadmap/analyze.go` |
| **P6** | 7. Phase-level dependency syntax | Medium (2-3 hr) | Medium | Cross-cutting | `internal/roadmap/parse.go`, `ROADMAP.md` |

**Recommended first action:** P0 is a documentation-only change that immediately unblocks Phase 1.8 (custom error types) and corrects the completion metrics from 14.6% to approximately 17%. It requires no code changes and can be validated by reviewing the implementation against acceptance criteria.

**Recommended first code change:** P1 (section-level dependency parsing) should follow immediately, as it fixes a correctness bug in the `roadmap_analyze` MCP tool that affects every downstream consumer of readiness data.

---

## Appendix A: Dependency Graph Visualization

```
Phase 0 (DONE) -----> Phase 0.5 (PARTIAL) -----> Phase 1 -----> Phase 1.5
                            |                          |              |
                            | 0.5.1 --> 1.8            |              |
                            | 0.5.7 --> 1.5.2          |              |
                            | 0.5.2 --> 1.7 (implicit) |              |
                            |                          v              v
                            |                    Phase 2 <------------+
                            |                      |   |
                            |                      |   +-- 2.1 --> 2.2, 2.3, 2.4, 2.5, 2.8
                            |                      |   +-- 2.1 --> 2.12 (MISSING EDGE)
                            |                      |   +-- 2.6 --> 2.13 (MISSING EDGE)
                            |                      |   +-- 2.3 --> 5.5
                            |                      |   +-- 2.11 --> 6.4
                            |                      v
                            |                 Phase 2.5
                            |                      |
                            |                      v
                            |                Phase 2.75 (DONE)
                            |                   /       \
                            |                  v         v
                            |             Phase 3   Phase 5
                            |                |         |
                            |                v         v
                            |           Phase 4   Phase 6 <-- 2.75 event bus
                            |                |         |
                            |                +----+----+
                            |                     v
                            |                Phase 7
                            |                     |
                            |                     v
                            |                Phase 8
                            |                     ^
                            |                     |
                            |                 6.1 --> 6.2, 6.3, 8.3
                            |                 6.2 --> 8.5
                            |                 6.4 --> 6.7
```

## Appendix B: External References

- [eyaltoledano/claude-task-master](https://github.com/eyaltoledano/claude-task-master) -- Task dependency graphs, validate-dependencies, autopilot TDD
- [claude-task-master task-structure.md](https://github.com/eyaltoledano/claude-task-master/blob/main/docs/task-structure.md) -- Structured task metadata schema
- [automazeio/ccpm](https://github.com/automazeio/ccpm) -- PRD-to-Issue pipeline, GitHub Issues as database, parallel execution
- [Kubernetes KEP Process](https://github.com/kubernetes/enhancements/blob/master/keps/sig-architecture/0000-kep-process/README.md) -- Large-project enhancement tracking
- [CNCF: Enhancing the Kubernetes Enhancements Process](https://www.cncf.io/blog/2021/04/12/enhancing-the-kubernetes-enhancements-process/) -- Git-based receipts process
- [Linear.app](https://linear.app/) -- Roadmap planning, automatic status transitions, MCP server
- [Agilemania: Managing Large Product Backlogs](https://agilemania.com/how-to-manage-large-complex-product-backlog) -- Backlog prioritization patterns
- [DAG Best Practices (dbt Labs)](https://www.getdbt.com/blog/dag-use-cases-and-best-practices) -- DAG structure and validation patterns
- [Task Dependency Graphs (GeeksforGeeks/ONES)](https://ones.com/blog/mastering-task-dependency-graphs-geeksforgeeks-guide/) -- Topological sort, critical path analysis
