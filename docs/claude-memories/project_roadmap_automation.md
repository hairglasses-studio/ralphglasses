---
name: Roadmap automation tools
description: MCP tools for automated roadmap parsing, analysis, research, expansion, and export — feeds into ralph loop task discovery
type: reference
---

7 MCP tools for roadmap automation:

**Roadmap tools (5):** `roadmap_parse`, `roadmap_analyze`, `roadmap_research`, `roadmap_expand`, `roadmap_export` — in `internal/roadmap/` package. Pipeline: ROADMAP.md → structured analysis → GitHub research → expansion proposals → rdcycle/fix_plan specs.

**Repo file tools (2):** `repo_scaffold`, `repo_optimize` — in `internal/repofiles/` package. Scaffold creates .ralph/, .ralphrc, PROMPT.md with project-type defaults. Optimize detects misconfigs and stale plans.

**Why:** Closes the loop: ROADMAP.md → structured task spec → ralph loop iterations. Any repo with a ROADMAP.md becomes a target for autonomous iteration.
