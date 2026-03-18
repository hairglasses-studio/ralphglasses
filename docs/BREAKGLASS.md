# Breakglass Criteria

Circuit breakers that override ALL other behavior. A loop MUST terminate if ANY condition is met.

## Enforcement Hierarchy

```
Daily limits
  └── Session limits
        └── Hourly limits
              └── Per-loop limits
```

Higher-level limits are catch-alls. If per-loop limits fail to trigger, session limits catch it.

## Default Thresholds

### Per-Loop
| Criterion | Default | Configurable Key |
|---|---|---|
| Max tokens | 200,000 | `LOOP_TOKEN_BUDGET` |
| Max cost | $2.00 | `LOOP_COST_CEILING` |
| Max wall time | 15 min | `LOOP_TIME_LIMIT_MIN` |

### Per-Session
| Criterion | Default | Configurable Key |
|---|---|---|
| Total tokens | 2,000,000 | `SESSION_TOKEN_BUDGET` |
| Total cost | $10.00 | `SESSION_COST_CEILING` |
| Total time | 4 hours | `SESSION_TIME_LIMIT_HOURS` |
| Consecutive no-progress | 3 | `MAX_CONSECUTIVE_NO_PROGRESS` |

### Per-Hour
| Criterion | Default | Configurable Key |
|---|---|---|
| API calls | 80 | `MAX_CALLS_PER_HOUR` |
| Token spend | 500,000 | `HOURLY_TOKEN_BUDGET` |
| Cost spend | $5.00 | `HOURLY_COST_CEILING` |

### Per-Day
| Criterion | Default | Configurable Key |
|---|---|---|
| Total tokens | 10,000,000 | `DAILY_TOKEN_BUDGET` |
| Total cost | $100.00 | `DAILY_COST_CEILING` |
| Total time | 24 hours | `DAILY_TIME_LIMIT_HOURS` |

## Spin Detection (Immediate Breakglass)

A spin event triggers immediate loop termination:

| Signal | Threshold | Action |
|---|---|---|
| Same error repeated | ≥3 consecutive | Stop loop, log error |
| Token spike | >2x rolling 5-loop avg | Stop loop, log spike |
| No git diff | ≥60 min wall time | Stop loop, log stall |
| High loop rate | >3.0 loops/task over 5 loops | Stop loop, log inefficiency |
| Test-only loops | >30% of last 10 loops | Warning, then stop |
| Identical tool sequence | ≥2 consecutive | Stop loop, log cycle |

## Known Potential Blockers

Per-task or global blockers that cause immediate termination:

```shell
# In .ralphrc
KNOWN_BLOCKERS="permission_denied:5,api_rate_limit:10,disk_full:1"
```

Format: `error_pattern:max_occurrences`

## Enforcement

Breakglass is checked:
1. **Before** each loop iteration starts
2. **After** each tool call completes
3. **On** every status.json write

When a breakglass fires:
1. Write exit reason to `.ralph/status.json`
2. Log to `.ralph/benchmarks.jsonl` with `"exit_reason": "breakglass_TOKEN_BUDGET"`
3. Write final benchmark summary to `.ralph/benchmarks.md`
4. Signal the process manager to stop gracefully
