# Security Audit — 2026-04-05

Pre-release security audit for ralphglasses Wave 3 public release.

## Summary

| Check | Status | Notes |
|-------|--------|-------|
| .env tracked in git | PASS | Not tracked; `.gitignore` covers `.env`, `.env.*`, `.env.local`, `.envrc` |
| .env committed to history | PASS | Never committed (verified via `git log --all --diff-filter=A -- ".env"`) |
| Hardcoded API keys in source | PASS | Only placeholder patterns (`sk-ant-...`) and test fixtures with fake keys |
| Local path leaks (`/home/hg`) | PASS | No leaks in tracked source; `.ralph/` runtime state (untracked) contains paths but is gitignored |
| `go.mod` replace directives | PASS | No local `replace` directives |
| LICENSE file | PASS | MIT, Copyright 2024-2026 hairglasses-studio |
| `.env.example` | MISSING | CONTRIBUTING.md and README.md show `.env` setup inline; no `.env.example` file exists |

## Git Author Email Exposure

**841 commits** use `mitch@galileo.ai` as the author email. This leaks an employer association.

An additional **6 commits** use `mixellburk@gmail.com` (GitHub merge commits via web UI).

Only recent commits use the correct `mitch@hairglasses.studio` email.

**Remediation options:**
1. **Rewrite history** with `git-filter-repo` to replace all non-standard emails — required if employer association is a concern for the public repo
2. **Accept as-is** — the galileo.ai domain is visible but does not contain secrets

## Tracked `.ralph/` Files (20 files)

The following `.ralph/` files are tracked and will be public:

- `.ralph/AGENT.md`, `.ralph/PROMPT.md`, `.ralph/SCRATCHPAD.md` — agent instruction files
- `.ralph/ab_tests/` (3 JSON files) — A/B test results
- `.ralph/benchmarks/` (2 JSON files) — performance benchmarks
- `.ralph/cost_observations.json` — cost tracking data
- `.ralph/coverage.txt` — test coverage snapshot
- `.ralph/fix_plan.md`, `.ralph/roadmap_overhaul_phase3.md`, `.ralph/sprint7_audit_report.md` — planning docs
- `.ralph/handoffs/` (1 JSON file) — session handoff data
- `.ralph/plans/` (2 MD files) — stage plans
- `.ralph/workflows/` (4 YAML files) — workflow definitions

No sensitive content found in any of these files (verified: no API keys, no personal paths, no email addresses).

## Billing Data in History

Commit `c48fc6f` ("chore: add billing analysis data for support ticket") added:
- `claude-code-billing-analysis.pdf` (8.6 KB)
- `.ralph/sweep-audit-2026-04-03/usage-evidence.jsonl`
- `.ralph/sweep-audit-2026-04-03/usage-summary.json`

These files are **no longer tracked** (gitignored via `.ralph/sweep-*/`), but they remain in git history. The usage-summary JSON files contain `mixellburk@gmail.com` as the account identifier.

**Remediation:** If the billing dispute data and personal email must not be public, history rewriting is required for this commit.

## Sensitive File Patterns in `.gitignore`

The `.gitignore` correctly covers:
- `.env`, `.env.*`, `.env.local`, `.envrc`
- `*.key`, `*.pem`
- `.ralph/` runtime state (logs, cycles, sessions, sweep data, JSON state files)
- `.tools/` (local Go toolchain)
- `.claude/settings.local.json`

## API Key Patterns in Source (Safe)

The following files reference API key prefixes but only as documentation placeholders or test fixtures:
- `README.md`, `CONTRIBUTING.md`, `AGENTS.md`, `GEMINI.md`, `docs/PROVIDER-SETUP.md`, `docs/getting-started.md` — all use `sk-ant-...` placeholder syntax
- `cmd/debugbundle.go` — key masking logic (sanitizes keys in debug output)
- `cmd/debugbundle_test.go`, `cmd/doctor_test.go` — test fixtures with obviously fake keys
- `internal/batch/batch_test.go` — test fixtures (`sk-ant-test-key`, `sk-ant-poll`, etc.)
- `internal/awesome/auth_test.go` — test fixture (`ghp_test123`)

No real API keys exist in any tracked file.

## Recommendations

### Required Before Public Release

1. **Decide on history rewrite** — 841 galileo.ai commits + billing data in history. If either is unacceptable for the public repo:
   - Use `git-filter-repo` to rewrite author emails to `mitch@hairglasses.studio`
   - Use `git-filter-repo` to remove billing data files from history
   - This requires force-push and collaborator re-clone
2. **Rotate API keys** — Even though no keys were found in git, rotate all keys referenced in the research finding (Anthropic, OpenAI, Google, GitHub PAT) as a precaution
3. **Add `.env.example`** — Create a template file so contributors know which variables to set

### Optional Improvements

4. **Remove tracked `.ralph/` files** — Consider whether A/B test results, benchmarks, cost observations, and sprint audit reports should be public. They contain no secrets but expose internal development process details.
5. **Pre-commit hook** — Add a git pre-commit hook that scans for key patterns (`sk-ant-`, `sk-svcacct-`, `AIzaSy`, `ghp_`) in staged files

## Verification Commands

```bash
# Confirm .env not tracked
git ls-files .env

# Confirm .env never in history
git log --all --diff-filter=A -- ".env" ".env*" --oneline

# Scan for real API keys
grep -rn 'sk-ant-api\|sk-svcacct-\|ghp_[a-zA-Z0-9]\{36\}' $(git ls-files) | grep -v _test.go | grep -v '\.\.\.

# Check author emails
git log --all --format='%ae' | sort | uniq -c | sort -rn

# Scan for personal paths
grep -rn '/home/hg' $(git ls-files) | grep -v .ralph/
```
