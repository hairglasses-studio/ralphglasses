---
paths:
  - ".claude/**"
---

Model routing for ralphglasses workloads:
- Fleet status/discovery: use sonnet (fast, cheap) — these are read-only lookups
- Self-improvement/R&D cycles: use opus (deep reasoning needed for code changes)
- Session orchestration: use sonnet unless debugging complex provider interactions
- Subagent model is sonnet globally (CLAUDE_CODE_SUBAGENT_MODEL=sonnet)
- Budget awareness: ralphglasses tracks per-provider costs; check `ralphglasses_session_budget` before long operations
- Compaction at 70% — keep critical context near top and bottom of prompts
