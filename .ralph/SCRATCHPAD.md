# Ralph MCP Tool & Skill Improvement Scratchpad

Living document — updated as we discover opportunities during development.

## MCP Tool Improvements

### Session Management
- [ ] **Default verifier to planner's provider** — Currently defaults to Codex even when planner is Claude. Fix in `tools_builders_session.go`
- [ ] **Codex model fallback chain** — Detect ChatGPT vs API account, fallback `gpt-5.4-xhigh` → `gpt-5.4` → `o4-mini` → error
- [ ] **Session output streaming** — `session_tail` should support WebSocket/SSE for real-time TUI updates
- [ ] **Session fork** — Allow forking a running session to try a different approach in parallel

### Loop System
- [ ] **Loop step async status** — `loop_step` returns immediately with 0 iterations; needs async polling or callback
- [ ] **Loop resume from checkpoint** — Checkpoint system exists but resume path untested at scale
- [ ] **Multi-repo loop orchestration** — Launch loops across N repos simultaneously from one command
- [ ] **Loop convergence detection** — Detect when iterations stop producing meaningful changes

### Fleet Management
- [ ] **Fleet-wide budget enforcement** — Aggregate spend across 77 repos, hard cap at fleet level
- [ ] **Stale loop cleanup** — 97 stale "001" loop entries from tests need pruning (prune tool exists but dry-run only)
- [ ] **Provider health dashboard** — Show which providers are up, rate-limited, or erroring
- [ ] **Fleet cost forecast** — Predict daily/weekly spend based on active loops × avg cost/iteration

### Quality Gates
- [ ] **Selftest build snapshot** — `go build` with multiple packages fails targeting single binary. Fix to build `./cmd/ralphglasses` only
- [ ] **Gate baseline auto-refresh** — Baseline gets stale; auto-refresh after N iterations
- [ ] **Cross-repo gate comparison** — Compare gate health across all 77 repos

### Roadmap Integration (NEW — see below)
- [ ] **roadmap_read** — Read ROADMAP.md from any repo, parse into structured phases/tasks
- [ ] **roadmap_next_task** — Get next actionable task from a repo's roadmap
- [ ] **roadmap_update_status** — Mark a roadmap task as complete/in-progress
- [ ] **meta_roadmap_sync** — Sync repo roadmaps with docs/strategy/META-ROADMAP.md
- [ ] **roadmap_assign_to_loop** — Create R&D loop from a roadmap task

### Docs Integration (NEW)
- [ ] **docs_search** — Full-text search across docs repo from ralph context
- [ ] **docs_write_finding** — Write research finding to docs/research/<domain>/
- [ ] **docs_check_existing** — Check if topic already researched before starting loop
- [ ] **docs_push** — Commit and push docs changes from within ralph session

## Skill Improvements

### Existing Skills
- [ ] **audit-sweep** — Add docs repo consultation before starting audit
- [ ] **fix-sweep** — Reference knowledge-base.md for known fix patterns
- [ ] **start-supervisor** — Add fleet budget awareness (don't start if 90%+ spent)
- [ ] **monitor-supervisor** — Add per-provider health checks

### New Skills Needed
- [ ] **/roadmap-sprint** — Execute a roadmap phase across multiple repos
- [ ] **/fleet-audit** — Audit all 77 repos for health, coverage, secrets
- [ ] **/cost-report** — Daily cost breakdown by provider, repo, loop
- [ ] **/research-sync** — Sync research findings between ralph sessions and docs repo

## Architecture Notes

### Observed During Testing (2026-04-04)
- Marathon shell script crashes without terminal (exit code 1, 5 restarts)
- Codex `gpt-5.4-xhigh` model not available on ChatGPT accounts
- Loop step returns immediately — sessions run async, no blocking wait option
- Journal consolidation threshold (100 entries) may be too low for 77 repos
- Fleet status returns all loop runs (97!) — needs pagination or filtering
- Cost tracking lost on restart (in-memory only) — needs disk persistence

### Performance Observations
- `mcp-call ralphglasses_scan` takes ~3s for 77 repos (acceptable)
- `mcp-call ralphglasses_fleet_status` returns ~50KB JSON (could be large at scale)
- Claude sessions: $0.004-$0.116/session, 1-21 turns
- Codex sessions: Failed due to model incompatibility
- Gemini sessions: Launch OK, model auto-detected as gemini-3-pro
