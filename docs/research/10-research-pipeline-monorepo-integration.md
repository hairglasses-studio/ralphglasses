# Phase 10 Research: Research Pipeline & Monorepo Integration

ROADMAP items: **6.2** (R&D Cycle Orchestrator), **8.5** (Self-Improvement Engine), **8.6** (Codebase Knowledge Graph)

---

## 1. Executive Summary

- **The R&D cycle orchestrator (6.2) has strong foundational pieces already built** -- `internal/model/benchmark.go` provides JSONL benchmark logging/summary, `internal/roadmap/` provides parse/analyze/export to rdcycle specs, and `claudekit/rdcycle/` contains the reference perpetual loop implementation with 8+ cycle specs. The gap is orchestration glue: benchmark comparison across iterations, regression detection, and auto-task-generation from regressions.
- **The self-improvement engine (8.5) requires the R&D cycle (6.2) as its foundation**, but the existing `internal/session/journal.go` (401 lines, improvement journal + pattern consolidation) and `internal/enhancer/` (7,363 lines, prompt mutation/scoring) provide substantial building blocks for pattern mining and prompt evolution.
- **The codebase knowledge graph (8.6) is a greenfield effort** with zero existing code. However, the `go/ast` + `go/types` standard library packages make Go codebase parsing straightforward, and `modernc.org/sqlite` (already proven in internal SQLite project) is the clear storage choice for a pure-Go SQLite graph store.
- **The awesome-list research pipeline (`internal/awesome/`, 1,176 lines)** is the most mature research automation subsystem: fetch, diff, analyze, report, sync -- all working and tested. It should serve as the template for R&D cycle pipeline design.
- **Monorepo integration across the hairglasses-studio ecosystem (10 repos)** currently relies on ad-hoc cross-references in CLAUDE.md. A `go.work` workspace file or shared module pattern would formalize dependency management and enable code sharing between `ralphglasses`, `mcpkit`, `claudekit`, `hg-mcp`, and internal SQLite project.

---

## 2. Current State Analysis

### 2.1 What Exists

#### `internal/awesome/` — Research Pipeline (Complete)

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `types.go` | 73 | -- | N/A (types only) | Stable |
| `fetch.go` | 140 | `fetch_test.go` (102) | Good | Stable |
| `analyze.go` | 220 | `analyze_test.go` (53) | Partial | Stable |
| `diff.go` | 32 | `diff_test.go` (64) | Good | Stable |
| `report.go` | 77 | `report_test.go` (57) | Good | Stable |
| `store.go` | 97 | `store_test.go` (97) | Good | Stable |
| `sync.go` | 150 | -- | None | Needs tests |
| `auth.go` | 14 | -- | None | Trivial |
| **Total** | **803 source / 373 test** | | | |

#### `internal/roadmap/` — Roadmap Pipeline (Complete)

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `parse.go` | 202 | `parse_test.go` (148) | Good | Stable |
| `analyze.go` | 245 | `analyze_test.go` (82) | Partial | Stable |
| `research.go` | 231 | `research_test.go` (110) | Good | Stable |
| `expand.go` | 206 | -- | None | Needs tests |
| `export.go` | 183 | `export_test.go` (138) | Good | Stable |
| **Total** | **1,067 source / 478 test** | | | |

#### `internal/enhancer/` — Prompt Enhancement (Complete)

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `enhancer.go` | 836 | `enhancer_test.go` (463) | Good | Stable |
| `scoring.go` | 536 | `scoring_test.go` (377) | Good | Stable |
| `lint.go` | 356 | `lint_test.go` (241) | Good | Stable |
| `config.go` | 262 | `config_test.go` (200) | Good | Stable |
| `context.go` | 244 | `context_test.go` (223) | Good | Stable |
| `examples.go` | 198 | `examples_test.go` (96) | Good | Stable |
| `metaprompt.go` | 191 | -- | None | Stable |
| `llmclient.go` | 177 | `llmclient_test.go` (193) | Good | Stable |
| `gemini_client.go` | 166 | `gemini_client_test.go` (159) | Good | Stable |
| `hybrid.go` | 164 | `hybrid_test.go` (248) | Good | Stable |
| `openai_client.go` | 152 | `openai_client_test.go` (136) | Good | Stable |
| `templates.go` | 276 | `templates_test.go` (110) | Good | Stable |
| `cache.go` | 103 | `cache_test.go` (104) | Good | Stable |
| `circuit.go` | 93 | `circuit_test.go` (118) | Good | Stable |
| `classifier.go` | 80 | -- | None | Stable |
| `claudemd.go` | 126 | `claudemd_test.go` (105) | Good | Stable |
| `filter.go` | 63 | `filter_test.go` (51) | Good | Stable |
| `provider.go` | 46 | `provider_test.go` (76) | Good | Stable |
| **Total** | **4,069 source / 3,294 test** | | | |

#### `internal/model/benchmark.go` — Benchmark Data Model

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `benchmark.go` | 189 | `benchmark_test.go` (exists) | Partial | Stable |

#### `internal/session/journal.go` — Improvement Journal

| File | Lines | Tests | Coverage | Status |
|------|-------|-------|----------|--------|
| `journal.go` | 401 | `journal_test.go` (305) | Good | Stable |

### 2.2 What Works Well

**Awesome-list pipeline architecture** (`internal/awesome/sync.go:31-118`): The 5-step sync pipeline (fetch -> diff -> analyze -> report -> save) is a clean, composable design. Each step is independently testable. The `SyncResult` struct provides clear pipeline metrics. This is the correct architectural template for the R&D cycle.

**Roadmap-to-task-spec export** (`internal/roadmap/export.go:26-43`): The `Export()` function converts parsed roadmap items directly into rdcycle-compatible `TaskSpec` JSON, bridging roadmap analysis to loop execution. This is a critical connector for 6.2.

**Benchmark data model** (`internal/model/benchmark.go:13-25`): `BenchmarkEntry` already captures per-iteration metrics (tokens, duration, cost, result, spin detection). `GenerateSummary()` computes per-session aggregates. This is the data foundation for 6.2.2 (self-benchmark) and 6.2.3 (regression detection).

**Improvement journal** (`internal/session/journal.go`): Append-only JSONL with worked/failed/suggest entries per session, plus `ConsolidatePatterns()` for durable pattern extraction. Direct input for 8.5.2 (pattern mining).

**Prompt mutation and scoring** (`internal/enhancer/scoring.go`, `enhancer.go`): The 10-dimension scoring system (clarity, specificity, structure, examples, tone, etc.) with letter grades and the 13-stage deterministic pipeline provide the substrate for 8.5.4 (prompt evolution).

**Concurrent analysis with semaphore** (`internal/awesome/analyze.go:44-60`): The worker-pool pattern with `sync.WaitGroup` + channel semaphore is reusable for concurrent benchmark execution and knowledge graph population.

### 2.3 What Doesn't Work

**No R&D cycle orchestrator exists** (ROADMAP 6.2): The benchmark data model and roadmap export exist in isolation. There is no loop that runs benchmark -> compare -> generate tasks -> execute tasks -> repeat. The `claudekit/rdcycle/` reference implementation exists but has not been ported.

**No regression detection** (ROADMAP 6.2.3): `GenerateSummary()` computes per-session metrics but there is no cross-session comparison. No threshold-based flagging of regressions.

**No self-improvement engine** (ROADMAP 8.5): No meta-agent concept. No automated config optimization. The improvement journal records observations but never acts on them.

**No codebase knowledge graph** (ROADMAP 8.6): Zero code exists. No AST parsing, no graph storage, no query API, no TUI view.

**No monorepo coordination**: The 10 repos under `hairglasses-studio/` are independent Go modules with no shared dependency mechanism. Pattern duplication is visible (circuit breaker in `internal/enhancer/circuit.go` vs similar in `mcpkit/resilience/`).

**Missing test coverage**: `internal/awesome/sync.go` (150 lines, no tests), `internal/roadmap/expand.go` (206 lines, no tests), `internal/enhancer/metaprompt.go` (191 lines, no tests), `internal/enhancer/classifier.go` (80 lines, no tests).

---

## 3. Gap Analysis

### 3.1 ROADMAP Target vs Current State

| ROADMAP Item | Target | Current State | Gap Size |
|-------------|--------|---------------|----------|
| 6.2.1 | Port perpetual improvement loop from claudekit rdcycle | `roadmap/export.go` exports rdcycle specs; `claudekit/rdcycle/` has reference impl | Medium -- orchestration glue needed |
| 6.2.2 | Self-benchmark: coverage, lint, build time, binary size | `model/benchmark.go` has JSONL + summary; no automated metric collection | Medium -- need `go test -cover`, `go vet`, `go build` runners |
| 6.2.3 | Regression detection across iterations | No cross-iteration comparison exists | Medium -- compare current vs previous `BenchmarkSummary` |
| 6.2.4 | Auto-generate improvement tasks from regressions | `roadmap/expand.go` generates proposals from analysis gaps | Small -- wire regression output to `Expand()` |
| 6.2.5 | Cycle dashboard in TUI | No TUI view for benchmark trends | Large -- new Bubble Tea view with ntcharts |
| 8.5.1 | Meta-agent monitoring other sessions | No meta-agent concept | Large -- new session type in `session/manager.go` |
| 8.5.2 | Pattern mining: failure modes, slow tasks, wasted budget | `session/journal.go` records; no analysis | Medium -- add pattern extraction from journal entries |
| 8.5.3 | Config optimization from observed patterns | No automated config suggestion | Medium -- analyze patterns, generate `.ralphrc` diffs |
| 8.5.4 | Prompt evolution: mutate and test prompts | `enhancer/` has mutation + scoring; no tournament selection | Medium -- add population/fitness tracking |
| 8.5.5 | Weekly performance report | No report generation | Small -- aggregate `BenchmarkSummary` + journal |
| 8.6.1 | Parse codebase: packages, types, functions, deps | No AST parsing code | Large -- new `internal/knowledge/` package |
| 8.6.2 | Store in SQLite: nodes and edges | No SQLite dependency | Large -- add `modernc.org/sqlite`, schema design |
| 8.6.3 | Query API | No query layer | Medium -- SQL queries wrapped in Go API |
| 8.6.4 | TUI graph view | No graph visualization | Large -- text-mode graph rendering |
| 8.6.5 | Context injection for agents | No context enrichment | Medium -- query graph before session launch |

### 3.2 Missing Capabilities

1. **Go standard library AST integration**: `go/ast`, `go/parser`, `go/types` are unused. These are required for 8.6.1 and would also benefit `roadmap/analyze.go` (currently uses filesystem checks only, missing function-level evidence).

2. **SQLite storage**: No SQLite dependency in `go.mod`. Required for knowledge graph (8.6.2), analytics history (6.4.1), and cross-session coordination (6.3.1). The internal SQLite project uses `modernc.org/sqlite` -- a pure-Go implementation with no CGo requirement.

3. **Cross-iteration benchmark comparison**: `model/benchmark.go` writes individual entries and computes per-session summaries but has no cross-session comparison or trend analysis.

4. **Automated metric collection**: No integration with `go test -coverprofile`, `go vet`, `staticcheck`, or `go build -o /dev/null` for binary size measurement.

5. **Population-based prompt evolution**: The enhancer can mutate and score prompts but lacks tournament/evolutionary selection (population tracking, fitness comparison, variant pruning).

6. **MCP tools for new capabilities**: The 57 existing MCP tools do not include knowledge graph queries, R&D cycle control, or self-improvement triggers.

### 3.3 Technical Debt Inventory

| Debt Item | Location | Impact | ROADMAP Item |
|-----------|----------|--------|-------------|
| `sync.go` has no tests | `internal/awesome/sync.go` | Pipeline regressions undetected | 6.2 (template for R&D pipeline) |
| `expand.go` has no tests | `internal/roadmap/expand.go` | Task generation bugs undetected | 6.2.4 |
| `classifier.go` has no tests | `internal/enhancer/classifier.go` | Task type misclassification | 8.5.4 |
| `metaprompt.go` has no tests | `internal/enhancer/metaprompt.go` | Provider-specific prompts untested | 8.5.4 |
| `rateComplexity` always returns "moderate" for Go | `internal/awesome/analyze.go:199-202` | All Go repos rated same complexity | 6.2 |
| `findEvidence` is filesystem-only | `internal/roadmap/analyze.go:150-166` | Misses function-level implementation evidence | 8.6 |
| Benchmark `HighScore` type defined but unused | `internal/model/benchmark.go:182-188` | Dead code | 6.2.2 |
| No GitHub rate limit handling in `awesome/` | `internal/awesome/fetch.go`, `analyze.go` | Pipeline fails on large lists | 6.2.1 |

---

## 4. External Landscape

### 4.1 Competitor/Peer Projects

#### 1. claude-task-master (eyaltoledano/claude-task-master, 15K+ stars, Node)
**Relevance to 6.2, 8.5**: Task dependency graph with main/research/fallback model concept. Implements automatic task decomposition and dependency ordering -- directly comparable to the R&D cycle orchestrator. Their task graph visualization is the closest peer for 6.2.5.

**Key patterns**: Task complexity scoring drives model routing (simple tasks -> cheaper models). Automatic subtask generation from high-level descriptions. PRD -> Epic -> Task -> Issue pipeline.

**Applicable to ralph**: Port the complexity-based model routing concept to the R&D cycle. Use their subtask decomposition as a reference for 6.2.4 auto-task generation.

#### 2. claude-squad (smtg-ai/claude-squad, 5K+ stars, Go)
**Relevance to 8.5**: Same Go/Bubbletea stack. Worktree isolation per agent, profile system (maps to agent definitions), multi-provider TUI. Their session monitoring provides a reference for meta-agent observation (8.5.1).

**Key patterns**: Profile-based agent specialization. Session comparison for performance optimization. Git worktree per agent prevents file conflicts.

**Applicable to ralph**: Study their per-agent performance tracking for pattern mining (8.5.2). Their profile system is a reference for prompt evolution variant management (8.5.4).

#### 3. internal-sqlite-project (internal, Go, pure SQLite)
**Relevance to 8.6**: Same organization. Uses `modernc.org/sqlite` for pure-Go SQLite with audit logging. Proven pattern for embedding SQLite in a Go binary without CGo.

**Key patterns**: Schema migration at startup, WAL mode for concurrent reads, JSON columns for flexible metadata, audit trail with timestamps.

**Applicable to ralph**: Direct template for knowledge graph SQLite schema (8.6.2). Import pattern for adding SQLite dependency to `go.mod`.

#### 4. sourcegraph/scip (Sourcegraph, Go, 600+ stars)
**Relevance to 8.6**: Source Code Intelligence Protocol. Defines a graph schema for code navigation (definitions, references, implementations, type hierarchy). Go indexer (`scip-go`) uses `go/types` to extract precise cross-references.

**Key patterns**: Protocol Buffers schema for code entities and relationships. Incremental indexing (only re-index changed files). Language-agnostic graph schema.

**Applicable to ralph**: Reference for knowledge graph schema design (8.6.1, 8.6.2). Their entity/relationship model (Symbol, Occurrence, Relationship) maps to our nodes/edges concept.

#### 5. go-callvis (ondrajz/go-callvis, 5K+ stars, Go)
**Relevance to 8.6.4**: Visualizes call graphs of Go programs using `golang.org/x/tools/go/callgraph`. Generates Graphviz dot output.

**Key patterns**: Uses `golang.org/x/tools/go/packages` for loading and `go/ssa` for call graph construction. Supports filtering by package, function, and call direction.

**Applicable to ralph**: Reference for call graph extraction (8.6.1). Their dot output format can be adapted for text-mode TUI rendering (8.6.4).

### 4.2 Patterns Worth Adopting

1. **Pipeline-as-data pattern** (from awesome sync pipeline): Each pipeline stage produces a typed output that feeds the next stage. The `SyncResult` captures metrics at each step. Apply this pattern to the R&D cycle: `BenchmarkResult -> ComparisonResult -> RegressionReport -> TaskSpec[]`.

2. **JSONL append-only telemetry** (from `model/benchmark.go`): Append-only logs with periodic summary generation. No coordination overhead. Works across concurrent sessions. Apply to all R&D cycle metrics.

3. **Pure-Go SQLite** (from internal SQLite project): `modernc.org/sqlite` avoids CGo, simplifies cross-compilation, and works on all Go-supported platforms. Use for knowledge graph and analytics history.

4. **AST visitor pattern** (from `go/ast`): `ast.Walk` + `ast.Visitor` interface for traversing Go source code. Use for knowledge graph population (8.6.1).

5. **Evolutionary prompt optimization** (academic): Tournament selection over prompt variants. Each variant gets a fitness score (from `enhancer/scoring.go`). Top-K variants survive, mutations generate new candidates. Apply to 8.5.4.

6. **Graph-as-adjacency-list in SQLite** (standard pattern): Two tables: `nodes(id, kind, name, package, file, line)` and `edges(src, dst, kind)`. With indexes on `(kind, name)` and `(src, kind)`, most queries are sub-millisecond. No need for a dedicated graph database.

### 4.3 Anti-Patterns to Avoid

1. **Over-investing in graph visualization (8.6.4)**: Text-mode graph rendering is inherently limited. Do not build a full-featured graph UI -- a simple `tree` or `list + detail` view showing callers/callees is more useful in the TUI context than an interactive force-directed layout.

2. **Real-time benchmark comparison**: The R&D cycle should be batch-oriented (run benchmarks, then compare), not streaming. Real-time dashboards add complexity without proportional value for a CLI tool.

3. **Complex ML-based pattern mining (8.5.2)**: Simple statistical analysis (frequency counting, percentile thresholds, trend detection) is more maintainable and debuggable than ML models for pattern detection. Keep it deterministic.

4. **Premature graph database adoption**: Neo4j, DGraph, or other graph databases are overkill for a single-repo code graph. SQLite adjacency lists handle millions of nodes with simple SQL queries. Adding a separate database server to a CLI tool is an anti-pattern.

5. **Monolithic knowledge graph indexer**: Do not build a single pass that indexes everything. Instead, index incrementally (only changed files since last index), and make the indexer a composable pipeline stage like the awesome-list sync.

### 4.4 Academic & Industry References

1. **"How Google Does Code Search"** (Russ Cox, 2012): Trigram indexing for code search. While ralphglasses does not need search-engine-scale, the concept of pre-computing a lightweight index that accelerates queries is directly applicable to knowledge graph queries (8.6.3).

2. **"Automated Program Repair"** (Monperrus, 2018, ACM Computing Surveys): Covers automated test generation, mutation testing, and fitness-based program improvement. Directly relevant to prompt evolution (8.5.4) -- prompts are programs in the LLM context.

3. **"The Cathedral and the Bazaar" for Monorepo** (Google, 2016): Google's monorepo paper documents the benefits of single-source-of-truth for dependency management. Relevant to hairglasses-studio monorepo integration decision.

4. **Go Workspaces** (`go.work`, Go 1.18+): Official multi-module development support. Enables `replace` directives without modifying `go.mod`, allowing cross-repo development with local checkouts.

---

## 5. Actionable Recommendations

### 5.1 Immediate Actions (can begin now, no blockers)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 1 | Add tests for `awesome/sync.go` | `internal/awesome/sync_test.go` (new) | S | High -- pipeline template for 6.2 | 6.2 |
| 2 | Add tests for `roadmap/expand.go` | `internal/roadmap/expand_test.go` (new) | S | Medium -- task generation correctness | 6.2.4 |
| 3 | Add tests for `enhancer/classifier.go` | `internal/enhancer/classifier_test.go` (new) | S | Medium -- task type accuracy for 8.5.4 | 8.5.4 |
| 4 | Fix `rateComplexity` to differentiate Go repos | `internal/awesome/analyze.go:188-203` | S | Low -- better complexity ratings | 6.2 |
| 5 | Wire `HighScore` type into `GenerateSummary()` | `internal/model/benchmark.go:182-188` | S | Low -- enable cross-session records | 6.2.3 |
| 6 | Add GitHub rate limit detection to `awesome/fetch.go` | `internal/awesome/fetch.go:39-63` | S | Medium -- prevents pipeline failures | 6.2 |

### 5.2 Near-Term Actions (require moderate effort, clear path)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 7 | Create `internal/rdcycle/` package with benchmark runner | `internal/rdcycle/runner.go` (new), `internal/rdcycle/types.go` (new) | M | High -- foundation for 6.2 | 6.2.1, 6.2.2 |
| 8 | Implement cross-iteration benchmark comparison | `internal/rdcycle/compare.go` (new) | M | High -- enables regression detection | 6.2.3 |
| 9 | Implement regression-to-task-spec generator | `internal/rdcycle/taskgen.go` (new), leveraging `roadmap/expand.go` | M | High -- closes the R&D loop | 6.2.4 |
| 10 | Create `internal/rdcycle/cycle.go` orchestrator | `internal/rdcycle/cycle.go` (new) | M | Critical -- the main R&D loop | 6.2.1 |
| 11 | Add `modernc.org/sqlite` to `go.mod` | `go.mod`, `go.sum` | S | Critical -- enables 8.6 and 6.4 | 8.6.2 |
| 12 | Create `internal/knowledge/` package with AST parser | `internal/knowledge/parser.go` (new) | L | High -- foundation for 8.6 | 8.6.1 |
| 13 | Design SQLite schema for knowledge graph | `internal/knowledge/schema.go` (new), `internal/knowledge/store.go` (new) | M | High -- storage layer for 8.6 | 8.6.2 |
| 14 | Extract improvement patterns from journal | `internal/session/patterns.go` (new, extends journal.go) | M | High -- data for 8.5.2, 8.5.3 | 8.5.2 |
| 15 | Add MCP tools: `ralphglasses_rdcycle_run`, `ralphglasses_rdcycle_status` | `internal/mcpserver/tools.go` | M | High -- expose R&D cycle via MCP | 6.2 |
| 16 | Create `go.work` workspace file for hairglasses-studio | `$HOME/hairglasses-studio/go.work` (new) | S | High -- enables cross-repo dev | Monorepo |

### 5.3 Strategic Actions (large effort, high impact, longer horizon)

| # | Action | Target Files | Effort | Impact | ROADMAP |
|---|--------|-------------|--------|--------|---------|
| 17 | Build prompt evolution engine with tournament selection | `internal/enhancer/evolution.go` (new) | L | High -- automated prompt improvement | 8.5.4 |
| 18 | Build meta-agent session type | `internal/session/metaagent.go` (new), `internal/session/manager.go` | XL | Critical -- 8.5 keystone | 8.5.1 |
| 19 | Implement knowledge graph query API | `internal/knowledge/query.go` (new) | L | High -- enables 8.6.3, 8.6.5 | 8.6.3 |
| 20 | Build TUI benchmark dashboard with ntcharts | `internal/tui/views/rdcycle.go` (new) | L | Medium -- visualization for 6.2.5 | 6.2.5 |
| 21 | Implement context injection from knowledge graph | `internal/knowledge/inject.go` (new), `internal/session/loop.go` | L | High -- smarter agent context | 8.6.5 |
| 22 | Build TUI knowledge graph explorer | `internal/tui/views/knowledge.go` (new) | L | Medium -- interactive code navigation | 8.6.4 |
| 23 | Implement weekly performance report generator | `internal/rdcycle/report.go` (new) | M | Medium -- fleet visibility | 8.5.5 |
| 24 | Build config optimization engine | `internal/session/configopt.go` (new) | M | Medium -- automated tuning | 8.5.3 |
| 25 | Shared Go library extraction from monorepo | `hairglasses-studio/hglib/` (new module) | XL | High -- eliminates duplication | Monorepo |

---

## 6. Risk Assessment

| Risk | Probability | Impact | Mitigation |
|------|------------|--------|------------|
| 6.1 (native loop engine) not ready, blocking 6.2 | Medium | Critical | 6.2 can prototype with shell-based loop runner; native engine is an optimization, not a hard dependency for benchmark collection |
| SQLite adds binary size and complexity | Low | Medium | `modernc.org/sqlite` adds ~10MB to binary; acceptable for a TUI tool. Pure Go means no CGo build issues |
| Knowledge graph indexer performance on large codebases | Medium | Medium | Index incrementally (changed files only); ralphglasses itself is ~25K lines, well within single-pass capability |
| Prompt evolution produces degenerate prompts | Medium | Low | Cap mutation rate, require minimum score threshold for survival, always keep original as baseline variant |
| Meta-agent creates runaway session costs | High | High | Hard budget cap on meta-agent sessions ($1/iteration default), circuit breaker on consecutive failures, require explicit opt-in |
| `go.work` workspace breaks CI builds | Low | Medium | CI continues to build each module independently; `go.work` is developer-local only (gitignored for individual repos, committed to monorepo root) |
| AST parsing breaks on generated code or build tags | Medium | Low | Skip `_test.go`, `_generated.go`, and conditional build files; use `go/packages` for correct build tag handling |
| Cross-repo code sharing creates tight coupling | Medium | High | Extract only stable, well-tested utilities (circuit breaker, rate limiter, SQLite helpers); keep domain logic in each repo |

---

## 7. Implementation Priority Ordering

### 7.1 Critical Path

```
Tests for existing pipeline code (actions 1-3)
    |
    v
Benchmark runner + comparison (actions 7, 8)
    |
    v
Regression-to-task generator (action 9)
    |
    v
R&D cycle orchestrator (action 10)
    |
    v
MCP tools for R&D cycle (action 15)
    |
    v
Pattern mining from journal (action 14)
    |
    v
Config optimization (action 24)
    |
    v
Meta-agent (action 18)
```

Knowledge graph has an independent critical path:

```
Add SQLite dependency (action 11)
    |
    v
AST parser (action 12)
    |
    v
SQLite schema + store (action 13)
    |
    v
Query API (action 19)
    |
    v
Context injection (action 21)
    |
    v
TUI graph explorer (action 22)
```

### 7.2 Recommended Sequence

**Sprint 1 (1 week): Foundation**
- Actions 1-6: Fill test gaps, fix trivial issues (all S effort)
- Action 11: Add SQLite dependency (S effort, unblocks 8.6)
- Action 16: Create `go.work` file (S effort, enables monorepo dev)

**Sprint 2 (2 weeks): R&D Cycle Core**
- Action 7: Benchmark runner (`internal/rdcycle/runner.go`)
- Action 8: Cross-iteration comparison (`internal/rdcycle/compare.go`)
- Action 9: Regression-to-task generator (`internal/rdcycle/taskgen.go`)
- Action 10: Cycle orchestrator (`internal/rdcycle/cycle.go`)

**Sprint 3 (2 weeks): Knowledge Graph Foundation**
- Action 12: AST parser (`internal/knowledge/parser.go`)
- Action 13: SQLite schema + store (`internal/knowledge/schema.go`, `store.go`)
- Action 14: Pattern extraction from journal (`internal/session/patterns.go`)

**Sprint 4 (2 weeks): Integration & Polish**
- Action 15: MCP tools for R&D cycle
- Action 19: Knowledge graph query API
- Action 23: Weekly performance report
- Action 24: Config optimization engine

**Sprint 5 (3 weeks): Advanced Features**
- Action 17: Prompt evolution engine
- Action 18: Meta-agent session type
- Action 20: TUI benchmark dashboard
- Action 21: Context injection from knowledge graph

**Sprint 6 (2 weeks): Visualization & Monorepo**
- Action 22: TUI knowledge graph explorer
- Action 25: Shared library extraction

### 7.3 Parallelization Opportunities

The following can be developed in parallel by separate agents/developers:

| Stream A (R&D Cycle) | Stream B (Knowledge Graph) | Stream C (Self-Improvement) |
|----------------------|--------------------------|---------------------------|
| Actions 7-10, 15, 20 | Actions 11-13, 19, 21, 22 | Actions 14, 17, 18, 23, 24 |
| ROADMAP 6.2 | ROADMAP 8.6 | ROADMAP 8.5 |
| Deps: `model/benchmark.go`, `roadmap/export.go` | Deps: `go/ast`, SQLite | Deps: `session/journal.go`, `enhancer/` |

Stream B is fully independent of A. Stream C depends on A for benchmark data (action 14 can start with existing journal data, but 24 benefits from R&D cycle metrics).

**Cross-stream integration points:**
- Knowledge graph context injection (action 21) enhances R&D cycle task generation (action 9) -- can be wired after both are independently complete.
- Meta-agent (action 18) can monitor R&D cycle status -- wire after cycle orchestrator (action 10) is stable.
- Prompt evolution (action 17) feeds into R&D cycle prompt optimization -- wire after both are stable.

---

## Appendix A: Monorepo Structure

Current `hairglasses-studio/` layout (10 repos):

```
hairglasses-studio/
  claudekit/       -- rdcycle, themekit, statusline, skills
  hg-mcp/          -- 500+ tool Go MCP server
  hgmux/           -- (purpose TBD)
  internal-ops/             -- (purpose TBD)
  mcpkit/           -- Go MCP framework, ralph loop engine, finops
  mesmer/           -- Go MCP with ralph integration
  prompt-improver/  -- archived, migrated to ralphglasses/internal/enhancer/
  ralph-claude-code/ -- (legacy)
  ralphglasses/     -- this repo
  webb/             -- (purpose TBD)
```

Recommended `go.work` for local development:
```go
go 1.26.1

use (
    ./ralphglasses
    ./mcpkit
    ./hg-mcp
    ./claudekit
)
```

This enables `import "github.com/hairglasses-studio/mcpkit/ralph"` in ralphglasses without publishing mcpkit first, accelerating the 6.2.1 port of the rdcycle engine.

## Appendix B: Proposed Knowledge Graph Schema

```sql
CREATE TABLE nodes (
    id        INTEGER PRIMARY KEY,
    kind      TEXT NOT NULL,  -- 'package', 'type', 'function', 'method', 'interface', 'variable'
    name      TEXT NOT NULL,
    package   TEXT NOT NULL,
    file      TEXT NOT NULL,
    line      INTEGER,
    signature TEXT,           -- full type signature for functions/methods
    doc       TEXT,           -- godoc comment
    exported  BOOLEAN DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE edges (
    id   INTEGER PRIMARY KEY,
    src  INTEGER NOT NULL REFERENCES nodes(id),
    dst  INTEGER NOT NULL REFERENCES nodes(id),
    kind TEXT NOT NULL,      -- 'calls', 'implements', 'embeds', 'imports', 'references', 'returns', 'depends_on'
    file TEXT,               -- where the relationship occurs
    line INTEGER
);

CREATE INDEX idx_nodes_kind_name ON nodes(kind, name);
CREATE INDEX idx_nodes_package ON nodes(package);
CREATE INDEX idx_edges_src ON edges(src, kind);
CREATE INDEX idx_edges_dst ON edges(dst, kind);
CREATE INDEX idx_edges_kind ON edges(kind);
```

## Appendix C: R&D Cycle Pipeline Design

Modeled on the `awesome/sync.go` pipeline:

```
┌─────────────┐    ┌──────────────┐    ┌────────────────┐    ┌──────────────┐    ┌─────────────┐
│  Benchmark   │───>│   Compare    │───>│   Detect       │───>│  Generate    │───>│   Execute   │
│  Collect     │    │   Iterations │    │   Regressions  │    │  TaskSpecs   │    │   Tasks     │
└─────────────┘    └──────────────┘    └────────────────┘    └──────────────┘    └─────────────┘
      │                   │                    │                    │                    │
      v                   v                    v                    v                    v
 benchmarks.jsonl   comparison.json    regressions.json      tasks.json         progress.json
```

Each stage is an exported function with typed input/output, testable in isolation.
