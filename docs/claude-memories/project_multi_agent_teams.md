---
name: Multi-agent teams and concentric R&D initiative
description: Next major initiative — multi-agent teams with continuous concentric R&D cycles replacing sequential sprint model
type: project
---

User wants to evolve from sequential sprint model (Sprint 1→...→7) into continuous, concentric research-and-development cycles with multi-agent teams.

**Why:** Sequential sprints batch work artificially. A concentric model has research feeding development feeding research continuously — more organic, always-running.

**How to apply:**
- Leverage existing fleet/team MCP tools: `team_create`, `team_delegate`, `team_status`, `agent_define`, `agent_compose`
- Build on existing cascade routing (Claude→Gemini→Codex) for cost-optimized task distribution
- R&D cycle already TUI-initiatable via `R` key binding and `:cycle` command
- TUI overhaul Phase 3 adds: Team Orchestration view, Task Queue view, Agent Composition UI, R&D Cycle Dashboard
- Study Sprint 1-7 outcomes to identify patterns that should become continuous processes
- Blackboard pattern enables inter-agent shared state for coordinated R&D
