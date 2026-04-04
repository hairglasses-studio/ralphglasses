# hgmux Audit Report

## Summary

hgmux is a well-structured Swift/AppKit macOS terminal multiplexer with ~130 source files across session monitoring, remote monitoring, orchestration, workspace isolation, and panel subsystems. The codebase is generally healthy with clear module boundaries and consistent patterns. The single highest-priority improvement is **fixing the WebSocket connection memory/CPU leak** in `MonitorHTTPServer`, which grows unboundedly with every client connection cycle. Beyond that, a config parsing bug silently drops values containing `=` (including base64 auth tokens), and a data race in `ClaudeSessionWatcher` will break under Swift 6 strict concurrency.

---

## Findings

### [1] WebSocket connections never cleaned up (Severity: high)
- **File(s)**: `Sources/RemoteMonitor/MonitorHTTPServer.swift:277-289`
- **Issue**: `startWSHeartbeat` recurses infinitely via `asyncAfter`. When a WebSocket client disconnects, the connection stays in `wsConnections` and the heartbeat loop continues sending JSON every 2 seconds to a dead socket forever. Every browser refresh, tab sleep/wake, or network change creates a new zombie. Over hours this causes unbounded memory and CPU growth.
- **Fix**: Check `connection.state` in `startWSHeartbeat` — break the loop if not `.ready`. Remove dead connections from `wsConnections` on send failure in `sendWSFrame`. Add a `connection.stateUpdateHandler` to proactively clean up on `.cancelled`/`.failed`.
- **Effort**: small

### [2] Config "=" splitting drops valid values (Severity: high)
- **File(s)**: `Sources/RemoteMonitor/MonitorHTTPServer.swift:506`, `Sources/Orchestrator/Watchdog.swift:177`, `Sources/WorkspaceIsolation/ProcessSandbox.swift:170`, `Sources/SessionMonitor/CostTracker.swift:70`
- **Issue**: All four config parsers use `components(separatedBy: "=")` and check `parts.count == 2`. Any config value containing `=` (base64 tokens, URLs with query params) is silently ignored. This directly affects `hgmux-remote-monitor-token` when users set tokens manually.
- **Fix**: Replace `components(separatedBy: "=")` with `split(separator: "=", maxSplits: 1)` to split only on the first `=`. Better yet, extract a shared `HgmuxConfig` parser (see Finding 9).
- **Effort**: small

### [3] ClaudeSessionWatcher data race across actor boundaries (Severity: high)
- **File(s)**: `Sources/SessionMonitor/ClaudeSessionWatcher.swift:11-15, 24-27, 30-39, 68-69, 96-101`
- **Issue**: Class is `@MainActor` but `fileWatchers`, `fileOffsets`, and `debounceTimers` are mutated exclusively from `watchQueue` closures via `[weak self]` capture, bypassing actor isolation. Under Swift 6 strict concurrency this won't compile. Under Swift 5 it's an unsound data race — if `stop()` runs on the main actor while `scanExistingFiles()` reads `fileWatchers` on `watchQueue`, behavior is undefined.
- **Fix**: Either make this a dedicated `actor` (not `@MainActor`), or move the mutable state into a lock-protected struct accessed only from `watchQueue`, keeping only `@Published` state on the main actor.
- **Effort**: medium

### [4] ISO8601DateFormatter allocated on every JSON call (Severity: medium)
- **File(s)**: `Sources/SessionMonitor/CostTracker.swift:172`, `Sources/Orchestrator/TaskBoard.swift:72-74,148,193`, `Sources/SessionMonitor/AgentTeamTracker.swift:147-148`, `Sources/RemoteMonitor/MonitorHTTPServer.swift:435`, `Sources/WorkspaceIsolation/GitDivergenceTracker.swift:179`, `Sources/WorkspaceIsolation/ProcessSandbox.swift:93`, `Sources/Orchestrator/Watchdog.swift:143`, `Sources/Orchestrator/AgentOrchestrator.swift:95-115`
- **Issue**: `ISO8601DateFormatter()` is allocated on every serialization call across 9+ files. This is one of the most expensive Foundation formatters to construct. The WebSocket heartbeat calls `sessionsJSON()` every 2 seconds, creating multiple formatter instances per tick.
- **Fix**: Add a `private static let isoFormatter = ISO8601DateFormatter()` in each class (safe because all are `@MainActor`-isolated) or create a shared extension.
- **Effort**: small

### [5] Duplicate config loading boilerplate (Severity: medium)
- **File(s)**: `Sources/RemoteMonitor/MonitorHTTPServer.swift:498-520`, `Sources/Orchestrator/Watchdog.swift:169-191`, `Sources/WorkspaceIsolation/ProcessSandbox.swift:162-184`, `Sources/SessionMonitor/CostTracker.swift:59-94`
- **Issue**: Four classes independently parse `~/.config/hgmux/config` with near-identical boilerplate (read file, split lines, skip comments, split on `=`, switch on key). This duplication is the root cause amplifier for Finding 2 — the `=` bug exists identically in all four.
- **Fix**: Extract a shared `HgmuxConfig` struct that parses the config file once and exposes typed accessors. Each consumer reads from the shared config instead of parsing independently.
- **Effort**: medium

### [6] CLAUDE.md still references cmux paths (Severity: medium)
- **File(s)**: `CLAUDE.md:1-80` (header and early sections)
- **Issue**: The file header says "cmux agent notes", references `cmux DEV`, `cmux-macos.dmg`, `/tmp/cmux-*`, `com.cmux.*` bundle IDs. The hgmux-specific section at the bottom contradicts these with `hgmux-*` paths. Agents reading CLAUDE.md may use wrong socket paths or bundle IDs.
- **Fix**: Audit the entire CLAUDE.md and update the pre-hgmux references. The top section describes the upstream cmux workflow (which is still valid for building), so mark that context clearly and ensure the hgmux additions section takes precedence.
- **Effort**: small

### [7] RingBuffer uses O(n) removeFirst (Severity: medium)
- **File(s)**: `Sources/SessionMonitor/SessionRegistry.swift:61-66`
- **Issue**: `RingBuffer.append()` calls `storage.removeFirst()` which shifts all elements — O(n) per append once at capacity. With capacity=1000 this means shifting 999 elements per JSONL event. The impact is moderate (small n, infrequent calls) but the fix is trivial.
- **Fix**: Replace with a proper circular buffer using head/tail index and modular arithmetic. Or use `Collections.Deque` which has O(1) `removeFirst`.
- **Effort**: small

### [8] Watchdog context estimation is fundamentally wrong (Severity: medium)
- **File(s)**: `Sources/Orchestrator/Watchdog.swift:66-86`
- **Issue**: Context usage is estimated as `(inputTokens + outputTokens) / 200000`. But `inputTokens + outputTokens` is the cumulative session total, not current context window contents. A session that has processed 500k tokens total may only have 80k in context due to compression. This produces false positive warnings on any non-trivial session.
- **Fix**: Either track context window state by detecting compaction events in the JSONL stream, or remove the feature until reliable signal is available. At minimum, document the limitation.
- **Effort**: medium

### [9] FileHandle not closed with defer (Severity: low)
- **File(s)**: `Sources/SessionMonitor/CostTracker.swift:180-183`, `Sources/WorkspaceIsolation/ProcessSandbox.swift:104-106`
- **Issue**: `FileHandle(forWritingAtPath:)` is opened, `seekToEndOfFile()` and `write()` called, then `try? handle.close()`. If the process is interrupted between open and close, the handle leaks. The `write()` API used here doesn't throw, so this is low risk, but the pattern is fragile.
- **Fix**: Wrap in `defer { try? handle.close() }` immediately after opening.
- **Effort**: small

### [10] CI workflow doesn't build Swift project (Severity: medium)
- **File(s)**: `.github/workflows/ci.yml`
- **Issue**: The main CI triggered on PRs only runs Node.js checks (`npm ci`, `npm test`, `npm run lint`, `npm run build`). It never builds or tests the Swift/Xcode project. Swift CI exists in other workflows (`ci-macos-compat.yml`, `test-e2e.yml`) but they aren't triggered on every PR.
- **Fix**: Either add a Swift build step to `ci.yml` (requires macOS runner), or ensure `ci-macos-compat.yml` is triggered on PR events. Note: macOS runners are expensive on GitHub Actions.
- **Effort**: medium

### [11] CORS wildcard on authenticated endpoints (Severity: low)
- **File(s)**: `Sources/RemoteMonitor/MonitorHTTPServer.swift:232`
- **Issue**: `Access-Control-Allow-Origin: *` on all responses including authenticated API endpoints. A malicious website could probe the local API if it obtains the auth token. Low practical risk for a local dev tool, but violates defense-in-depth.
- **Fix**: Remove the CORS header from authenticated endpoints, or restrict to `localhost` origins.
- **Effort**: small

### [12] RingBuffer lacks Sendable conformance (Severity: low)
- **File(s)**: `Sources/SessionMonitor/SessionRegistry.swift:47-67`
- **Issue**: `RingBuffer<T>` doesn't conform to `Sendable`. Since all access is `@MainActor`-isolated this is safe today, but will produce warnings under strict concurrency checking and blocks Swift 6 adoption.
- **Fix**: Add `extension RingBuffer: @unchecked Sendable where T: Sendable {}` (or make it a value type with `Sendable` conformance).
- **Effort**: small

### [13] Sandbox command check is trivially bypassable (Severity: low)
- **File(s)**: `Sources/WorkspaceIsolation/ProcessSandbox.swift:40-41`
- **Issue**: `isCommandAllowed` splits on space and checks only the first token against the allowlist. Absolute paths (`/usr/bin/git`), relative paths (`./git`), or shell builtins bypass the check. Provides false confidence.
- **Fix**: Resolve the executable to its basename, handle shell wrappers. However, this is an advisory governance feature, not a security boundary — document that limitation rather than trying to make it airtight.
- **Effort**: medium

### [14] Non-constant-time token comparison (Severity: low)
- **File(s)**: `Sources/RemoteMonitor/MonitorHTTPServer.swift:122`
- **Issue**: Auth token comparison uses `!=` (short-circuit string equality), theoretically vulnerable to timing attacks. Practically irrelevant for a local dev server over loopback TCP.
- **Fix**: Use `HMAC`-based comparison or a byte-by-byte constant-time check. Low priority given threat model.
- **Effort**: small

---

## CLAUDE.md Accuracy

| Section | Status | Issue |
|---------|--------|-------|
| Header "cmux agent notes" | **Outdated** | Should say "hgmux agent notes" or clarify dual identity |
| `reload.sh` / tagged builds | **Partially outdated** | References `/tmp/cmux-debug-<tag>.log` but hgmux section says `/tmp/hgmux-debug-<tag>.log` |
| Bundle identifiers | **Contradictory** | Early sections imply `com.cmux.*`, hgmux section at bottom says `com.hairglasses-studio.hgmux.*` |
| Socket paths | **Contradictory** | Top references `/tmp/cmux-*.sock`, bottom says `/tmp/hgmux-*.sock` |
| `hgmux-sandbox-enabled` config key | **Present but undocumented behavior** | CLAUDE.md lists the key but doesn't mention the trivial bypass limitation |
| Watchdog context threshold | **Misleading** | Documents `hgmux-watchdog-context-threshold` without noting the estimation is cumulative tokens, not current context |
| Missing: `hgmux-pricing-*` keys | **Incomplete** | Config keys mentioned in CostTracker code but not listed in the supported keys section |

---

## Recommended Next Actions

1. **Fix WebSocket connection leak** — add connection state checks and cleanup in `MonitorHTTPServer.swift` (30 min, prevents unbounded resource growth)
2. **Extract shared config parser and fix `=` splitting** — create `HgmuxConfig` struct, replace 4 independent parsers (1-2 hours, fixes silent config bug + eliminates duplication)
3. **Fix ClaudeSessionWatcher data race** — restructure to proper actor isolation before Swift 6 migration (2-3 hours, prevents undefined behavior)
