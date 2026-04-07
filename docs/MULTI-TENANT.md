# Multi-Tenant Operations

ralphglasses now supports additive shared-host multi-tenancy with a canonical `tenant_id` across session, team, budget, recovery, loop, and trigger flows.

Legacy single-tenant behavior still works. When a caller omits `tenant_id`, ralphglasses resolves it to `_default`.

## What Is Scoped

- Sessions
- Teams and structured worktrees
- Loop runs
- Cost ledger and budget summaries
- Recovery operations and actions
- Trigger HTTP launch and resume flows

State is now persisted under tenant-aware paths:

- Sessions: `.session-state/sessions/<tenant_id>/<session_id>.json`
- Teams: `.session-state/teams/<tenant_id>/<team_name>.json`
- Structured integrations: `.ralph-integrations/<repo>/<tenant>/<team>`
- Structured worktrees: `.ralph-worktrees/<repo>/<tenant>/<team>/<task>/attempt-N`

Legacy flat state files are treated as `_default` and lazily rewritten into the new layout.

## Provision A Tenant

Create a tenant with an explicit allowlist of repo roots:

```bash
ralphglasses tenant create acme \
  --display-name "Acme Workspace" \
  --allowed-repo-root ~/hairglasses-studio/client-acme \
  --budget-cap-usd 250
```

Inspect the tenant:

```bash
ralphglasses tenant status acme --json
```

List all known tenants:

```bash
ralphglasses tenant list
```

## Repo Isolation

Every launch resolves the tenant before touching the repo and rejects repos outside that tenant's `allowed_repo_roots`.

This is the main shared-host safety boundary:

- identical team names can exist in different tenants
- identical repo names can exist in different tenant roots
- budget summaries and fleet views do not pool spend across tenants

## Tenant-Scoped CLI Examples

Launch a session inside one tenant:

```bash
ralphglasses session launch \
  --tenant-id acme \
  --provider codex \
  --repo ~/hairglasses-studio/client-acme/api \
  --prompt "Audit and fix flaky integration tests"
```

Check tenant-scoped fleet state:

```bash
ralphglasses tenant status acme
ralphglasses session list --tenant-id acme
ralphglasses budget status --tenant-id acme
```

## Tenant-Scoped MCP Examples

Session launch:

```json
{
  "tenant_id": "acme",
  "repo_path": "/home/hg/hairglasses-studio/client-acme/api",
  "provider": "codex",
  "prompt": "Audit and fix flaky integration tests"
}
```

Fleet status:

```json
{
  "tenant_id": "acme"
}
```

Tenant administration:

```json
{}
```

Use:

- `ralphglasses_tenant_list`
- `ralphglasses_tenant_create`
- `ralphglasses_tenant_status`
- `ralphglasses_tenant_rotate_trigger_token`

## Trigger HTTP Authentication

`POST /api/trigger` and `POST /api/resume/{run_id}` now require:

```http
Authorization: Bearer <tenant-token>
```

Rotate a tenant trigger token:

```bash
ralphglasses tenant rotate-trigger-token acme
```

Behavior:

- the bearer token resolves exactly one tenant
- request bodies cannot override tenant ownership
- resume is allowed only for runs created by the same tenant
- `/api/health` remains unauthenticated

Example trigger request:

```bash
curl -X POST http://127.0.0.1:8080/api/trigger \
  -H "Authorization: Bearer <tenant-token>" \
  -H "Content-Type: application/json" \
  -d '{
    "source": "github",
    "event": "push",
    "payload": {"ref": "refs/heads/main"}
  }'
```

## Compatibility Notes

- `_default` is auto-seeded for legacy usage
- omitted `tenant_id` continues to work through `_default`
- legacy persisted rows and JSON state are surfaced as `_default`
- this rollout is backend, CLI, MCP, and trigger-HTTP focused; TUI tenant selectors are intentionally deferred
