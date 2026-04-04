# mesmer Audit Report

## Summary

mesmer is a well-structured, large-scale Go codebase (840 source files, 229 test files, 98 tool module directories) with solid architectural foundations — clean package boundaries, proper mutex usage, and comprehensive observability. The **single highest-priority improvement** is a systemic pattern of background goroutines spawned via `sync.Once` singletons that lack shutdown mechanisms, creating resource leaks in long-running or frequently-restarted deployments. Secondary concerns include 7 instances of `http.Get()` without timeout control and an access token leaked in a URL query parameter.

---

## Findings

### [1] Systemic goroutine leak: 10+ cleanup goroutines with no shutdown signal (Severity: high)

- **File(s)**: `internal/mcp/safety.go:181`, `internal/mcp/middleware.go:410`, `internal/mcp/streamable_http.go:89`, `internal/mcp/tools/async.go:105`, `internal/mcp/tools/results/module.go:148`, `internal/api/middleware.go:266`, `internal/clients/cache.go:118`, `internal/clients/operation_cooldown.go:67`, `internal/clients/session_security.go:101`, `internal/clients/tool_usage.go:128`
- **Issue**: At least 10 `go cleanup*()` goroutines are spawned at init time with infinite `for range ticker.C` loops and no `context.Context` or stop channel. In safety.go:265-277, the ticker is never stopped. While most are behind `sync.Once` (limiting to one instance), `StreamableHTTPHandler` spawns a new goroutine per handler instance (line 89). In containerized deployments with rolling restarts, these accumulate.
- **Fix**: Add a `ctx context.Context` + `cancel context.CancelFunc` to each struct. Pass `ctx` into the cleanup goroutine and select on `ctx.Done()`. Add a `Close()` or `Shutdown()` method that calls `cancel()` and `ticker.Stop()`. For the `sync.Once` singletons, wire the shutdown into the server's graceful shutdown path. Example for `SafetyManager`:
  ```go
  type SafetyManager struct {
      // ... existing fields ...
      cancel context.CancelFunc
  }
  // In GetSafetyManager():
  ctx, cancel := context.WithCancel(context.Background())
  globalSafetyManager.cancel = cancel
  go globalSafetyManager.cleanupExpiredTokens(ctx)
  // In cleanupExpiredTokens():
  func (s *SafetyManager) cleanupExpiredTokens(ctx context.Context) {
      ticker := time.NewTicker(1 * time.Minute)
      defer ticker.Stop()
      for { select { case <-ctx.Done(): return; case <-ticker.C: /* cleanup */ } }
  }
  ```
- **Effort**: medium (apply same pattern across 10 sites, ~2 hours)

---

### [2] OAuth access token passed in URL query string (Severity: high)

- **File(s)**: `internal/mcp/google_oauth.go:344`
- **Issue**: `http.Get("https://www.googleapis.com/oauth2/v2/userinfo?access_token=" + accessToken)` passes the OAuth access token as a URL query parameter. This token will appear in server access logs, proxy logs, browser history, and any intermediary that logs URLs. Google's own documentation recommends using the `Authorization: Bearer` header instead.
- **Fix**: Replace with a request using the Authorization header:
  ```go
  req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
  req.Header.Set("Authorization", "Bearer "+accessToken)
  resp, err := h.httpClient.Do(req)
  ```
  This also fixes the missing timeout (uses a client with timeout instead of `http.DefaultClient`).
- **Effort**: small

---

### [3] 7 bare `http.Get()` calls using DefaultClient (no timeout) (Severity: high)

- **File(s)**: `internal/mcp/google_oauth.go:344`, `internal/clients/obsidian_api.go:705`, `internal/commands/msp_modules.go:309`, `internal/commands/search.go:593`, `internal/mcp/tools/knowledge/module.go:2970`, `internal/mcp/tools/presentations/module.go:2441`, `cmd/mesmer-agent/main.go:210`
- **Issue**: `http.Get()` uses `http.DefaultClient` which has **no timeout**. A slow or unresponsive upstream will block the goroutine indefinitely. The codebase already has an excellent instrumented HTTP client in `internal/mcp/otel/http_client.go` with proper timeouts (30s default), connection pooling, and tracing — these calls bypass all of it.
- **Fix**: Replace each `http.Get(url)` with `otel.DefaultInstrumentedClient().Get(url)`, or better yet, construct requests with `http.NewRequestWithContext(ctx, ...)` and use the instrumented client's `Do(req)`. This adds timeout protection AND observability for free.
- **Effort**: small (mechanical replacement, ~30 min)

---

### [4] SSE response silently drops JSON marshal errors (Severity: medium)

- **File(s)**: `internal/mcp/streamable_http.go:259`, `internal/mcp/streamable_http.go:304`
- **Issue**: Two `json.Marshal()` calls discard errors with `_`. Line 259: `data, _ := json.Marshal(response)` — if marshaling fails, an empty/malformed SSE event is sent to the client. Line 304: `resultJSON, _ := json.Marshal(result)` — same pattern for the MCP server's response. While `json.Marshal` rarely fails, the `result` from `HandleMessage` could contain unmarshalable types (channels, funcs).
- **Fix**: Check the error and return a JSON-RPC error response:
  ```go
  data, err := json.Marshal(response)
  if err != nil {
      errResp := &JSONRPCResponse{JSONRPC: "2.0", ID: rpcReq.ID, Error: &JSONRPCError{Code: -32603, Message: "marshal error"}}
      data, _ = json.Marshal(errResp)
  }
  ```
- **Effort**: small

---

### [5] WebSocket upgrader accepts all origins (Severity: medium)

- **File(s)**: `internal/api/ws_agent.go:54`
- **Issue**: `CheckOrigin: func(r *http.Request) bool { return true }` with a TODO comment. This allows any website to open a WebSocket connection to the agent hub, enabling cross-site WebSocket hijacking. An attacker could connect from a malicious page and issue commands to endpoint agents.
- **Fix**: Validate origin against a configurable allowlist (can reuse `StreamableHTTPConfig.AllowedOrigins` pattern). At minimum, check origin matches the server's own hostname.
- **Effort**: small

---

### [6] `rand.Read()` errors ignored in 12+ security-sensitive locations (Severity: medium)

- **File(s)**: `internal/auth/jwt.go:80,238`, `internal/mcp/safety.go:214`, `internal/mcp/oauth.go:133,140`, `internal/mcp/google_oauth.go:360,366`, `internal/clients/collaboration.go:842`, `internal/clients/msp_identity.go:184`, `internal/clients/shared_vault_session.go:139,332,455`, `internal/clients/session_sync.go:516`, `internal/mcp/tools/operations/module.go:6155`, `internal/mcp/tools/msp_webhooks/module.go:134`
- **Issue**: `rand.Read(b)` return value discarded in 12+ locations that generate tokens, session IDs, JWT signing keys, and webhook secrets. While `crypto/rand.Read` almost never fails on Linux (reads from `/dev/urandom`), if it does fail (e.g., broken entropy source in container), all generated tokens become zero-bytes — trivially predictable. Note: some callsites like `mcp_remote.go:372` and `trust_boundaries.go:918` already handle this correctly.
- **Fix**: Check the error consistently. For the `internal/auth/jwt.go:80` case (JWT signing key generation), this is especially critical — a zero-byte signing key allows trivial JWT forgery:
  ```go
  if _, err := rand.Read(key); err != nil {
      return nil, fmt.Errorf("crypto/rand failed: %w", err)
  }
  ```
- **Effort**: small (mechanical, ~30 min across all sites)

---

### [7] RBAC `Authorize()` defaults to `Authorized: true` (Severity: medium)

- **File(s)**: `internal/mcp/auth.go:328-329`
- **Issue**: `AuthContext` is initialized with `Authorized: true` on line 329. When RBAC is not enforced (line 333), the function returns an admin-level context. The comment says "Default to authorized if RBAC not enforced" — this is intentional, but the default-allow pattern means any bug in the RBAC enforcement check silently grants full access rather than denying it.
- **Fix**: Initialize `Authorized: false` and explicitly set `true` only in the success paths. This is a defense-in-depth change:
  ```go
  ctx := &AuthContext{Authorized: false}
  if !m.config.EnforceRBAC {
      ctx.Authorized = true
      ctx.Role = RoleAdmin
      // ...
  }
  ```
- **Effort**: small (but requires verifying all callers handle the change)

---

### [8] `RateLimiter.Allow()` uses write lock for read-heavy path (Severity: low)

- **File(s)**: `internal/mcp/middleware.go:415-431`
- **Issue**: `Allow()` takes a full `mu.Lock()` on every request (line 416), even though existing users only need to update `lastSeen` (a timestamp write). For new users, a write lock is needed to insert into the map, but the common case (existing user) could use `RLock` + promote to `Lock` only on miss. Under high concurrency, this serializes all rate-limit checks.
- **Fix**: Use a two-phase approach: `RLock` to check existence, then `Lock` only for new entries. Or switch to `sync.Map` for the hot path. Given that this is a global singleton handling every request, the contention impact scales with request volume.
- **Effort**: small

---

### [9] `StreamableHTTPHandler.sessionCleanup()` goroutine leaks on handler recreation (Severity: medium)

- **File(s)**: `internal/mcp/streamable_http.go:88-93`, `internal/mcp/streamable_http.go:349-367`
- **Issue**: Unlike the `sync.Once` singletons, `NewStreamableHTTPHandler()` spawns a cleanup goroutine on every call (line 89). If the handler is recreated (e.g., config reload, test setup), the old goroutine runs forever. The cleanup goroutine (lines 349-367) correctly defers `ticker.Stop()` but has no exit condition.
- **Fix**: Accept a `context.Context` parameter in `NewStreamableHTTPHandler()` and pass it to `sessionCleanup()`. Select on `ctx.Done()` to allow graceful termination.
- **Effort**: small

---

### [10] Test file ratio: 229/840 (27%) with no coverage enforcement in CI (Severity: medium)

- **File(s)**: `.github/workflows/` (all workflow files)
- **Issue**: 229 test files for 840 source files gives a 27% file-level coverage ratio. The CI workflows run compliance checks (threshold 85 on tool patterns) but no `go test -coverprofile` with a minimum coverage gate. Critical paths like `streamable_http.go`, `google_oauth.go`, and most tool modules in `internal/mcp/tools/*/` lack any test files. The auth and safety modules have good test coverage, but the HTTP transport layer (SSE, sessions, CORS) is untested.
- **Fix**: Add a coverage gate to CI: `go test ./... -coverprofile=coverage.out && go tool cover -func=coverage.out | grep total | awk '{if ($3+0 < 40) exit 1}'`. Start with a low threshold (40%) and ratchet up. Prioritize tests for `streamable_http.go` and `google_oauth.go`.
- **Effort**: medium

---

### [11] `processRequest` round-trips JSON needlessly (Severity: low)

- **File(s)**: `internal/mcp/streamable_http.go:285-318`
- **Issue**: `processRequest()` marshals the already-parsed `JSONRPCRequest` back to JSON (line 287), passes it to `HandleMessage` as `json.RawMessage`, then marshals the result and unmarshals it into `JSONRPCResponse` (lines 304-306). This double-serialization adds latency and allocations on every request. The pattern exists because `mcp-go`'s `HandleMessage` expects raw JSON.
- **Fix**: Keep the original request body bytes from `handlePost` (line 132) and pass them directly to `HandleMessage`, skipping the unmarshal-remarshal cycle. For the response, type-assert or use `mcp-go`'s response types directly instead of round-tripping through JSON.
- **Effort**: small

---

### [12] Module directory count doesn't match CLAUDE.md claim (Severity: low)

- **File(s)**: `CLAUDE.md:5`, `internal/mcp/tools/` (98 directories)
- **Issue**: CLAUDE.md claims "70 modules" but `internal/mcp/tools/` contains 98 subdirectories. The 70 figure may refer to an older count or exclude certain directories, but it's misleading for onboarding.
- **Fix**: Update CLAUDE.md to reflect the actual count, or clarify what "module" means (e.g., only directories with a `module.go` file).
- **Effort**: small

---

## CLAUDE.md Accuracy

| Section | Issue | Suggested Fix |
|---------|-------|---------------|
| Project Overview | Claims "70 modules" — actual directory count is 98 | Update to "98 modules" or clarify counting methodology |
| Building & Running | Accurate | None |
| Key Patterns | Accurate | None |
| Tool Discovery | Claims "~766K" tokens for full load — unverifiable but plausible | None |
| Current Version | "v135.0" — matches docs/VERSION.md | None |
| Obsidian Vault | Says `~/mesmer-vault/` but Makefile installs to `~/obsidian-vaults/mesmer` | Reconcile — pick one canonical path |
| Missing | No mention of the 16 cmd/ entry points, REST API port 8080, or the Streamable HTTP transport | Add a "Binaries" section listing entry points; note the HTTP transport |
| Missing | No mention of the 7 Dockerfile variants or Helm charts | Add a "Deployment" section |

---

## Recommended Next Actions

1. **Fix bare `http.Get()` calls** (Finding #3) — 30 minutes, eliminates 7 potential indefinite hangs by switching to the existing instrumented client
2. **Move access token to Authorization header** (Finding #2) — 15 minutes, stops credential leakage in logs
3. **Add shutdown contexts to cleanup goroutines** (Finding #1) — 2 hours, fixes the systemic resource leak pattern across 10+ sites
