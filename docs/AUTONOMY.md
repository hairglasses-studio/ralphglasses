# Autonomy Levels & Self-Improvement

The session manager supports graduated autonomous decision-making via `internal/session/autonomy.go`:

| Level | Name | Behavior |
|-------|------|----------|
| 0 | Observe | Log decisions only ("would have done X") |
| 1 | AutoRecover | Auto-restart on transient errors, provider failover |
| 2 | AutoOptimize | Auto-adjust budgets, providers, and rate limits from feedback profiles |
| 3 | FullAutonomy | Auto-launch from roadmap, scale teams, apply config changes |

All decisions are recorded in a JSONL decision log with rationale, inputs, and outcomes. Human overrides are tracked by the HITL subsystem.

## Self-Improvement Subsystems

- **`autonomy.go`**: DecisionLog with 4-level gating — decisions below current level are logged but not executed
- **`autooptimize.go`**: Feedback-driven provider/budget selection using journal-derived performance profiles
- **`autorecovery.go`**: Transient error retry with exponential backoff (connection reset, timeout, rate limit, 429/503), provider failover on persistent failures
- **`contextstore.go`**: Cross-session file conflict detection — prevents concurrent workers from editing the same files
- **`feedback.go`**: Provider/task performance profiling (avg cost, turns, duration, completion rate per task type)
- **`hitl.go`**: Human-in-the-loop metric tracking — manual interventions vs autonomous actions, trend scoring (improving/stable/degrading)
- **`promptcache.go`**: Prompt prefix caching — identifies stable preamble (CLAUDE.md, system prompts) for cost savings across sessions
