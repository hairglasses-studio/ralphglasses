# 32 -- Codex / Ralph Ecosystem Sweep

Generated: 2026-04-08
Sources: 18 external community links supplied for the ecosystem tranche.
Scope: Codex subagents, skills, automations, ralph-loop derivatives, Gemini extensions, review/fixer workflows, and loop-state/completion-contract patterns.

---

## Summary

### Highest-value transfers

- Codex-native subagent catalogs are now mature enough to justify a broader project-local role surface instead of relying only on generic explorer/reviewer roles.
- Skills and automation packs are converging on the same pattern: small durable entrypoints, progressive disclosure, and explicit writeback into docs or repo state.
- Ralph-loop variants are strongest when they make completion contracts explicit, keep iteration state on disk, and separate review or audit lanes from write lanes.
- Gemini variants contribute useful patterns around after-agent hooks, fresh-context loops, checkpoint files, and ghost-cleanup / mismatch-repair behavior.

### Shipped in this tranche

- Added the canonical `ralph-sweep` skill plus Claude/plugin mirrors.
- Added four new project-local Codex roles with provider mirrors: `ecosystem_researcher`, `roadmap_synthesizer`, `automation_designer`, and `review_fixer`.
- Added a durable local link index for the 18-source ecosystem tranche.
- Added this synthesized memo and a mirrored shared-docs copy.

### Highest backlog themes opened by the sweep

- broader subagent catalog growth and role-selection heuristics
- reusable automation packs for docs, CI, release, and dependency hygiene
- review/fixer/verifier and audit-only workflow lanes
- provider-neutral completion contract, loop-state, and checkpoint documentation
- scheduled ecosystem sweeps with repeatable writeback rules

---

## Source Notes

### 1. VoltAgent / awesome-codex-subagents
- Review date: 2026-04-08.
- What it is: a curated catalog of Codex-focused subagent roles.
- Reusable patterns: narrow role scopes, explicit model/sandbox pairing, and role naming that reflects task ownership.
- Immediate `ralphglasses` opportunities: keep expanding project-local role coverage beyond generic explorer/reviewer lanes; document role-selection heuristics; tag roles by read-only vs write-oriented usage.
- Immediate docs opportunities: maintain a shared role-catalog page; map common role families across Codex, Claude, and Gemini; note when a skill is a better fit than a subagent.
- Backlog opportunities: importable role-pack manifests; per-role win-rate telemetry; generated catalog resources from `.codex/agents/`.

### 2. ComposioHQ / awesome-codex-skills
- Review date: 2026-04-08.
- What it is: a curated list of Codex skill packs and supporting materials.
- Reusable patterns: `SKILL.md` as canonical entrypoint, optional `scripts/` and `references/`, progressive disclosure, and reusable operator playbooks.
- Immediate `ralphglasses` opportunities: keep `.agents/skills/` canonical; add targeted skills for repeatable workflows; keep provider mirrors thin and generated.
- Immediate docs opportunities: document when to create a skill vs an agent vs an MCP workflow; keep skill families indexed by task type; publish example skill anatomy for downstream repos.
- Backlog opportunities: skill linting for missing guardrails; skill usage telemetry; curated internal marketplace pages for high-value shared skills.

### 3. onurkanbakirci / awesome-codex-automations
- Review date: 2026-04-08.
- What it is: a catalog of Codex-friendly automations around CI, releases, documentation, and maintenance.
- Reusable patterns: small task-specific automations, scheduled checks, repo-health sweeps, and release/diff hygiene.
- Immediate `ralphglasses` opportunities: define automation-pack backlog items for docs freshness, dependency drift, AGENTS drift, and review follow-up; keep automations idempotent; pair each automation with a verification contract.
- Immediate docs opportunities: index automations by surface area; keep reusable templates for cron-driven sweeps; document operator budgets and stop conditions.
- Backlog opportunities: automation pack registry; per-pack cost/benefit telemetry; policy gates for which automations may write vs only report.

### 4. GitHub topic page: `ralph-loop`
- Review date: 2026-04-08.
- What it is: a discovery hub for active Ralph-loop-adjacent implementations.
- Reusable patterns: ecosystem naming clusters, common state-file conventions, and a broad mix of review, build, and agent-team loops.
- Immediate `ralphglasses` opportunities: treat the topic page as a recurring discovery feed; keep a standing watchlist of adjacent loop implementations; refresh the ecosystem index periodically rather than one-off.
- Immediate docs opportunities: maintain a dated ecosystem snapshot; distinguish active implementations from abandoned experiments; record which ideas were adopted vs rejected.
- Backlog opportunities: automated topic-page diffing; ecosystem freshness scoring; a recurring `ralph-sweep` schedule with writeback policy.

### 5. abhishekbhakat / ralph-loop-for-antigravity
- Review date: 2026-04-08.
- What it is: a loop tailored to Antigravity / VS Code style workflows.
- Reusable patterns: one-task-per-iteration discipline, explicit task/progress files, and simple fallback behavior.
- Immediate `ralphglasses` opportunities: document one-task iteration guidance for lightweight loops; keep task and progress state separable; prefer simple file contracts where full stores are unnecessary.
- Immediate docs opportunities: add an "editor-integrated loop" pattern note; compare task-file loops vs richer store-backed loops; show when lightweight file state is sufficient.
- Backlog opportunities: optional single-task mode for `ralphglasses_loop_step`; task-file import/export; thin-client/editor adapters for lightweight loop control.

### 6. JH427 / ralph-codex
- Review date: 2026-04-08.
- What it is: a Codex-oriented Ralph loop with immutable PRD, fresh process per iteration, and git rollback discipline.
- Reusable patterns: immutable planning artifact, fresh-session iteration reset, tests-as-reality, and append-only learnings.
- Immediate `ralphglasses` opportunities: sharpen PRD/spec immutability guidance for long loops; document when fresh-process iteration beats long-lived sessions; keep learning journals append-only.
- Immediate docs opportunities: add a PRD-first operator pattern note; contrast fresh-session and resumed-session tradeoffs; index the git rollback / checkpoint discipline.
- Backlog opportunities: explicit PRD pinning mode; plan-cache vs fresh-context experiments; per-loop rollback contract templates.

### 7. umputun / ralphex
- Review date: 2026-04-08.
- What it is: a plan-driven loop with review pipeline, worktree isolation, and dashboard/notification ideas.
- Reusable patterns: markdown plan execution, review-only/task-only modes, worktree isolation, and operator-facing status views.
- Immediate `ralphglasses` opportunities: keep review and write lanes distinct; continue investing in worktree and loop isolation; treat operator dashboards as first-class runtime surfaces.
- Immediate docs opportunities: document review-only vs write-capable lanes; capture worktree isolation patterns; add a dashboard/notifications backlog note.
- Backlog opportunities: clean-room worktree pools; richer loop dashboards; notification hooks for stuck or completed cycles.

### 8. iannuttall / ralph
- Review date: 2026-04-08.
- What it is: a mature file-state Ralph implementation with `.agents/ralph/`, `.ralph/`, and pluggable runners.
- Reusable patterns: file-backed memory, story/PRD layout, pluggable provider runners, and explicit progress/error logs.
- Immediate `ralphglasses` opportunities: keep loop state inspectable on disk; document file-state conventions more clearly; preserve provider-pluggable contracts instead of provider-specific branching.
- Immediate docs opportunities: publish a state-file glossary; explain `.ralph/` artifacts by purpose; keep a comparison chart of file-state vs database-backed state.
- Backlog opportunities: provider-neutral state schema docs; migration notes between state backends; richer state visualization resources.

### 9. breezewish / CodexPotter
- Review date: 2026-04-08.
- What it is: a Codex-first reconcile loop with clean-room rounds and a local knowledge base.
- Reusable patterns: reconcile-after-round discipline, clean-room iteration, filesystem memory, and local KB usage.
- Immediate `ralphglasses` opportunities: document reconcile checkpoints; keep local knowledge artifacts available to loops; explore when clean-room rounds reduce state contamination.
- Immediate docs opportunities: compare reconcile loops vs continuous loops; document local KB patterns; capture clean-room terminology in the research corpus.
- Backlog opportunities: plan/knowledge cache reuse; loop round summarizers; explicit reconcile-mode runtime support.

### 10. ghuntley / how-to-ralph-wiggum
- Review date: 2026-04-08.
- What it is: methodology framing and operator guidance around the Ralph Wiggum technique.
- Reusable patterns: PRD/story discipline, method branding, and clearer operator expectations around cost and iteration behavior.
- Immediate `ralphglasses` opportunities: keep PRD/story terminology explicit in docs; use consistent language for loop stages and acceptance; document cost and quality tradeoffs in plain operator terms.
- Immediate docs opportunities: maintain a methodology glossary; connect community terminology back to current repo surfaces; keep cross-links between tactic docs and roadmap planning docs.
- Backlog opportunities: methodology comparison matrix; onboarding guide for new operators; curated examples of small vs large loop setups.

### 11. DMontgomery40 Ralph audit loop gist
- Review date: 2026-04-08.
- What it is: a read-only Codex audit runner using `.codex/ralph-audit/`, `prd.json`, optional web search, and structured markdown outputs.
- Reusable patterns: audit-only lane, read-only sandboxing, report persistence, and `output-last-message` style capture.
- Immediate `ralphglasses` opportunities: add a documented read-only audit lane; keep review sweeps distinct from write sweeps; preserve report-only output contracts for low-risk audits.
- Immediate docs opportunities: index audit-only loop patterns; publish a read-only audit recipe; document when to enable web research in audit mode.
- Backlog opportunities: audit-mode skill or workflow; PRD-driven audit report templates; report collector automation for recurring sweeps.

### 12. kenryu42 / ralph-review
- Review date: 2026-04-08.
- What it is: a review-centric loop with reviewer/fixer separation and checkpoint discipline.
- Reusable patterns: reviewer/fixer split, optional simplifier, git checkpoints, and structured findings output.
- Immediate `ralphglasses` opportunities: keep review and fix roles separate; support checkpoint-first review sessions; make findings formatting consistent and actionable.
- Immediate docs opportunities: publish a review/fixer/verifier pattern note; maintain guidance for when a simplifier lane helps; tie checkpoints to recovery docs.
- Backlog opportunities: dedicated review-fix workflow; verifier lane after fix; checkpoint policy controls.

### 13. alfredolopez80 / multi-agent-ralph-loop
- Review date: 2026-04-08.
- What it is: a multi-agent Ralph variant with memory emphasis, hooks, and team roles.
- Reusable patterns: explicit team composition, memory layer, hook system, and quality gates.
- Immediate `ralphglasses` opportunities: keep team-role language concrete; document memory vs scratchpad roles; reuse hook-driven lifecycle ideas without over-coupling to a single provider.
- Immediate docs opportunities: record a multi-agent pattern catalog; compare team composition approaches; keep quality-gate terminology aligned with existing loop gates.
- Backlog opportunities: richer team templates; memory-policy experiments; hook packs for recurring loop stages.

### 14. gemini-cli-extensions / ralph
- Review date: 2026-04-08.
- What it is: a Gemini extension that emphasizes fresh-context iteration, after-agent hooks, completion promises, and ghost cleanup.
- Reusable patterns: clear completion contract, previous-context reset between iterations, hook-based lifecycle, and mismatch cleanup.
- Immediate `ralphglasses` opportunities: document completion contracts explicitly; keep ghost/mismatch cleanup as a named concern; compare resumed vs fresh-context loop tradeoffs by provider.
- Immediate docs opportunities: publish a completion-contract note; keep a glossary for ghost sessions, mismatch cleanup, and fresh-context loops; track provider-specific iteration behavior.
- Backlog opportunities: provider-aware completion contract surface; after-agent hook abstraction; automatic ghost cleanup reports.

### 15. kranthik123 / Gemini-Ralph-Loop
- Review date: 2026-04-08.
- What it is: a large Gemini command surface with checkpoints, status/history/report commands, and an MCP server implementation.
- Reusable patterns: command-oriented operator surface, explicit checkpoint/history/report files, and built-in monitoring commands.
- Immediate `ralphglasses` opportunities: keep checkpoint, history, and report concepts explicit in docs; continue exposing operator commands through clear MCP/CLI parity; treat state inspection as product surface, not debugging residue.
- Immediate docs opportunities: document state/report artifacts consistently; compare extension-style commands to MCP tools; record command families by workflow stage.
- Backlog opportunities: richer report generators; checkpoint lifecycle tooling; command-family parity audits across providers.

### 16. AsyncFuncAI / ralph-wiggum-extension
- Review date: 2026-04-08.
- What it is: a lightweight Gemini CLI extension exposing simple slash commands and local markdown-config state.
- Reusable patterns: very small command surface, easy install story, and minimal local state.
- Immediate `ralphglasses` opportunities: preserve a lightweight entry path for simple loops; avoid forcing every workflow through heavyweight configuration; keep low-friction command recipes in docs.
- Immediate docs opportunities: maintain a "lightweight extension" comparison note; distinguish minimal-loop ergonomics from full control-plane workflows; document slash-command analogs for MCP tools.
- Backlog opportunities: lightweight starter profile; minimal-state operator mode; docs-side extension comparison matrix.

### 17. gemini-cli-extensions / ralph activity page
- Review date: 2026-04-08.
- What it is: the repository activity surface showing continued maintenance through early 2026.
- Reusable patterns: active validation work, CI refinement, troubleshooting guidance, and ghost-cleanup iteration.
- Immediate `ralphglasses` opportunities: keep active maintenance visible in docs; treat troubleshooting and validation docs as part of loop quality; track operational changes separately from feature work.
- Immediate docs opportunities: record freshness signals for tracked ecosystem projects; prefer active projects when borrowing patterns; note when a source is only historically interesting.
- Backlog opportunities: ecosystem freshness scoring; stale-source warnings in future sweeps; maintenance cadence comparison across loop projects.

### 18. AsyncFuncAI / ralph-wiggum-extension `GEMINI.md`
- Review date: 2026-04-08.
- What it is: command and operator guidance for the lightweight Gemini extension path.
- Reusable patterns: slim operator docs, slash-command explanation, and minimal setup narrative.
- Immediate `ralphglasses` opportunities: keep operator docs short where the workflow is short; document command-first entrypoints cleanly; preserve minimal-path guidance alongside richer architecture docs.
- Immediate docs opportunities: add a command-surface comparison note; highlight minimal operator recipes; cross-link command docs to deeper architecture references.
- Backlog opportunities: auto-generated command cheat sheets; per-provider operator quickstarts; command-surface drift checks.

---

## Aggregated Opportunity Matrix

### Now
- Keep expanding the project-local subagent catalog with evidence-backed narrow roles.
- Keep `ralph-sweep` as the canonical recurring ecosystem sweep skill.
- Add a documented read-only audit lane for low-risk repo review work.
- Formalize reviewer/fixer/verifier workflow language in docs and agent surfaces.
- Document completion contract, ghost cleanup, and fresh-context loop terminology.
- Keep loop state, history, and report artifacts discoverable in docs rather than implicit.
- Maintain dated local and shared link indexes for external ecosystem inputs.

### Next
- Add automation packs for docs freshness, dependency drift, AGENTS drift, release checks, and review follow-up.
- Generate agent and skill catalogs from repo state so mirrors stay auditable.
- Add provider-aware guidance for when to prefer resumed vs fresh-context iterations.
- Add checkpoint and rollback policy docs for loop operations.
- Add ecosystem freshness scoring so active sources stay prioritized over stale ones.
- Build operator quickstarts for lightweight, review-only, and full control-plane modes.
- Add explicit plan/reconcile mode documentation for clean-room or reconcile-round workflows.

### Later
- Import/exportable role-pack and automation-pack manifests.
- Per-role and per-automation telemetry showing win rate, cost, and failure modes.
- Worktree-pool and clean-room isolation improvements inspired by review-first loop variants.
- Richer dashboard and notification surfaces for stuck, stale, or completed loops.
- Provider-neutral state schema and migration docs spanning file-backed and database-backed loop state.
- Recurring topic-page diff automation for the broader `ralph-loop` ecosystem.
- A community-facing methodology and pattern matrix once the internal surfaces stabilize.

---

## Recommended Roadmap Additions

1. Subagent catalog expansion with role-selection heuristics and generated catalog docs.
2. Automation pack backlog for docs, CI, release, dependency drift, and review follow-through.
3. Review/fixer/verifier workflow lane with checkpoint and audit-only variants.
4. Completion-contract and ghost-cleanup documentation spanning Codex, Claude, and Gemini loop behavior.
5. Recurring ecosystem sweep cadence with dated local/shared writeback and freshness tracking.
