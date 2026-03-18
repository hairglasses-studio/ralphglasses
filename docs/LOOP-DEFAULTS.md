# Ralph Loop Default Configuration

Sane defaults for uncustomized loops. Applied when a repo has no `.ralphrc` or missing keys.

## Default `.ralphrc` Values

```shell
# Project
PROJECT_NAME="$(basename $(pwd))"

# Rate limiting
MAX_CALLS_PER_HOUR="80"
CLAUDE_TIMEOUT_MINUTES="10"

# Circuit breaker
CB_NO_PROGRESS_THRESHOLD="3"
CB_SAME_ERROR_THRESHOLD="3"
CB_PERMISSION_DENIAL_THRESHOLD="5"
CB_COOLDOWN_MINUTES="10"

# Agent identity (populated per-agent at launch)
AGENT_ROLE="worker"
AGENT_INDEX="0"
AGENT_COUNT="1"

# Budget (per-session defaults)
SESSION_TOKEN_BUDGET="2000000"
SESSION_COST_CEILING="10.00"
SESSION_TIME_LIMIT_HOURS="4"
MAX_CONSECUTIVE_NO_PROGRESS="3"

# Model routing
MODEL_PLAN="opus"
MODEL_IMPLEMENT="sonnet"
MODEL_VERIFY="haiku"
```

## Breakglass Defaults (per granularity)

| Granularity | Token Budget | Cost Ceiling | Time Limit |
|---|---|---|---|
| Per-loop | 200K | $2.00 | 15 min |
| Per-session | 2M | $10.00 | 4 hours |
| Per-hour | 500K | $5.00 | (rate limit) |
| Per-day | 10M | $100.00 | 24 hours |

## Agent Identity Schema

File: `.ralph/agent_identity.json`

```json
{
  "agent_index": 0,
  "agent_count": 1,
  "run_id": "2026-03-17-session-001",
  "seed_hash": "a1b2c3d4",
  "assigned_role": "worker",
  "persona": "implementer",
  "approach_directive": "Build the happy path first, get it working.",
  "file_ownership": [],
  "forbidden_overlap": []
}
```

Seed hash: `sha256(run_id + agent_index + task_hash)[:8]`

## Benchmark Log Schema

File: `.ralph/benchmarks.jsonl` (append per iteration)

```json
{"ts":"...","loop":1,"task_id":"...","input_tokens":0,"output_tokens":0,"duration_s":0,"result":"pass|fail|skip","cost_usd":0.0,"model":"...","spin":false}
```
