# Sweep Audit Cost Analysis — 2026-04-03

## Session Cost Breakdown

| Repo | Input$ | CacheRead$ | CacheCreate$ | Output$ | Total$ | Fresh? |
|------|--------|------------|--------------|---------|--------|--------|
| claudekit | $0.00 | $2.84 | $5.82 | $0.54 | **$9.20** | yes |
| cr8-cli | $0.00 | $1.98 | $3.33 | $0.57 | **$5.88** | yes |
| crabravee | $0.00 | $2.16 | $4.39 | $0.53 | **$7.08** | yes |
| dotfiles | $0.25 | $87.03 | $56.43 | $5.99 | $149.70 | no (414 turns) |
| hg-mcp | $0.22 | $3.98 | $3.46 | $0.78 | **$8.45** | yes |
| hgmux | $0.53 | $2.41 | $8.39 | $0.61 | **$11.94** | yes |
| jobb | $0.74 | $1189.23 | $451.46 | $29.20 | $1670.64 | no (1793 turns) |
| mcpkit | $0.00 | $1.78 | $5.30 | $0.55 | **$7.62** | yes |
| mesmer | $0.21 | $2.19 | $3.85 | $0.57 | **$6.81** | yes |
| ralphglasses | $0.92 | $194.36 | $135.64 | $11.60 | $342.51 | no (528 turns) |

## Actual Audit Cost

- **7 fresh audit sessions**: $56.97
- **3 appended to pre-existing sessions**: ~$30 estimated
- **Total audit cost**: ~$87

## Key Insight

jobb ($1,671), dotfiles ($150), and ralphglasses ($343) costs are from their
entire multi-day session histories, NOT from the audit. `claude -p` reused
existing session files rather than creating fresh ones for these repos.

## Lessons

1. `claude -p` appends to existing project session histories — costs accumulate
2. Cache read tokens ($1.50/M) are cheap; cache creation ($18.75/M) dominates
3. Average fresh audit cost: ~$8/repo with Opus 4.6 1M
4. Should set `--max-budget-usd 15` per session to cap runaway costs
5. Need `--no-session-persistence` flag for ephemeral audit sessions
