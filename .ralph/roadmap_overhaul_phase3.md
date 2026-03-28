# Roadmap Overhaul Phase 3: Scratchpad & Finding Synthesis

## Metrics Summary

| Metric | Value |
|--------|-------|
| Go packages | 37 |
| Test files | 24,854 |
| MCP tools | 110 (13 namespaces) |
| Roadmap items (total) | 593 |
| Roadmap items (complete) | 168 (28.3%) |
| Roadmap items (unchecked) | 425 |
| R&D cycles completed | 15 |
| Total findings | 174+ |
| Finding resolution rate | 9.8% |
| Loop completion rate | 68.75% |
| Test coverage | 83.4% overall |
| Key dep versions | Go 1.26.1, mcp-go v0.45.0, bubbletea v1.3.10 |

## Recurring Multi-Cycle Findings (Regressions)

These findings appeared in 2+ cycles, indicating systemic issues:

| Finding IDs | Issue | Cycles | Category |
|-------------|-------|--------|----------|
| FINDING-148/268 | Snapshot saves to claudekit path | 4th cycle | Config |
| FINDING-160 | Session signal:killed | 3rd cycle | Stability |
| FINDING-220/262 | Provider recommend Claude-only | 3rd cycle | Multi-provider |
| FINDING-169 | Logs NO_LOG_FILE | 4th cycle | Observability |
| FINDING-216/245 | roadmap_export lacks difficulty_score | 2nd cycle | Tooling |
| FINDING-226/238 | loop_gates baseline zero-init | 2nd cycle | Loop quality |
| FINDING-207/239 | Stale loop state | 2nd cycle | Loop quality |
| FINDING-204 | Snapshot path mismatch | 3rd cycle | Config |

## Scratchpad-by-Scratchpad Analysis

### tool_improvement_scratchpad.md (Cycles 1-15)
- 36 numbered findings, many RESOLVED
- Key unresolved: worktree auto-commit (#1), cross-worktree conflicts (#2), event persistence sync (#13)
- Dominant pattern: "not valid json" response (26 occurrences) — JSON format enforcement failing

### cycle15_tool_exploration_scratchpad.md
- 33 new findings (FINDING-237 through FINDING-269)
- 25 scratchpad gaps (SG-1 through SG-25)
- 82/112 tools exercised (73%)
- Critical bugs: prompt_analyze score inflation (FINDING-240), prompt_enhance stage skipping (FINDING-243), loop_gates zero-baseline (FINDING-238), budget params silently ignored (FINDING-258/261), autonomy level not persisted (FINDING-257)

### cycle14_production_readiness_scratchpad.md
- 33 findings across 7 phases, 14 tool calls in Phase 1
- FINDING-204: snapshot saves to claudekit path (3rd cycle recurrence)
- Loop benchmark: 68.75% completion (baseline: 100%), 2.3x cost increase at P95
- Coverage: 83.4% overall, cmd/ralphglasses-mcp at 66.7%

### fleet_audit_scratchpad.md
- 523 total loop runs: 381 pending, 121 failed (109 phantom "001" repo), 6 completed
- 73% stale/phantom work
- Zero-change iteration problem: 22/23 passed iterations had 0 files changed
- Gemini 4.4x cheaper than Claude for equivalent work
- Cascade router NOT configured, bandit NOT configured

### research-audit_scratchpad.md
- roadmap_research has NO arXiv integration — only GitHub search
- Relevance scoring flat at 0.5 for ALL results (broken)
- roadmap_expand ignores research parameter
- awesome_fetch only works for default repo
- 15 findings total, several critical

### improvement_patterns.json
- Positive pattern: "not valid json" response (count: 26) — ironically "positive" classification
- Negative pattern: "signal: killed" (count: 6)
- Rules: null (no rules learned)

## Finding→Roadmap→Task Pipeline Breaks

5 critical breaks identified in the pipeline:

1. **Finding capture → Roadmap integration**: Findings accumulate in scratchpads but aren't reflected in ROADMAP.md items
2. **Roadmap item → Loop task**: No automated tool to convert roadmap items to loop-executable tasks
3. **Loop observation → Finding**: Observations record data but don't auto-generate findings
4. **Cross-cycle regression detection**: No tool correlates findings across cycles to detect recurrence
5. **Resolution verification**: No tool verifies that a "resolved" finding stays resolved in subsequent cycles

## Gap Analysis

### Internal Gaps (scratchpads reveal, roadmap doesn't address)
- JSON format enforcement (25.7% retry rate) — no roadmap item
- Zero-change iteration problem — no roadmap item
- Phantom/stale fleet work (73%) — no roadmap item
- Relevance scoring broken (flat 0.5) — no roadmap item
- Cascade/bandit not configured despite code existing — no roadmap item
- Autonomy level not persisted — no roadmap item
- Budget params silently ignored — no roadmap item

### External Gaps (competitor capabilities missing)
- No IDE integration (Cursor/Windsurf/Continue.dev all have IDE plugins)
- No web UI (cc-hub, Hive have web dashboards)
- No mobile monitoring (Hive has mobile-first dashboard)
- No Kubernetes operator (agent-sandbox K8s CRD exists in ecosystem)
- No NixOS support (StereOS, microvm.nix patterns available)

### Spec Gaps (MCP/Claude features available but not adopted)
- MCP Tasks (async with polling) — not implemented
- MCP Elicitation (server-initiated prompts) — not implemented
- MCP Streamable HTTP transport — not implemented
- MCP Progress notifications — not implemented
- Claude Code hooks integration — not leveraged
- Claude Code Agent SDK — not leveraged
- Prompt caching (cache_control) — not leveraged in MCP handlers
- Extended thinking — not leveraged
